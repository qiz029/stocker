package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// SaveScenario upserts a full scenario (metadata, instruments, baseline
// prices) in one transaction. Existing rows for the same id are replaced.
func SaveScenario(ctx context.Context, db *pgxpool.Pool, sc *scenario.Scenario) error {
	factors, err := json.Marshal(sc.Factors)
	if err != nil {
		return err
	}
	keyWindows, err := json.Marshal(sc.KeyWindows)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// ON DELETE CASCADE wipes instruments and scenario_prices too.
		if _, err := tx.Exec(ctx, `DELETE FROM scenarios WHERE id = $1`, sc.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO scenarios (id, days, factors, key_windows) VALUES ($1, $2, $3, $4)`,
			sc.ID, sc.Days, string(factors), string(keyWindows)); err != nil {
			return err
		}
		for ord, inst := range sc.Instruments {
			beta, err := json.Marshal(inst.Beta)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO instruments (scenario_id, id, ord, alias, descr, beta)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				sc.ID, inst.ID, ord, inst.Alias, inst.Desc, string(beta)); err != nil {
				return err
			}
		}
		rows := make([][]any, 0, len(sc.Instruments)*sc.Days)
		for _, inst := range sc.Instruments {
			for d, p := range sc.Baseline[inst.ID] {
				rows = append(rows, []any{sc.ID, inst.ID, d, p.Open, p.High, p.Low, p.Close})
			}
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"scenario_prices"},
			[]string{"scenario_id", "instrument_id", "day", "open", "high", "low", "close"},
			pgx.CopyFromRows(rows))
		return err
	})
}

func LoadScenario(ctx context.Context, q Querier, id string) (*scenario.Scenario, error) {
	sc := &scenario.Scenario{ID: id, Baseline: map[string][]scenario.OHLC{}}
	var factors, keyWindows []byte
	err := q.QueryRow(ctx,
		`SELECT days, factors, key_windows FROM scenarios WHERE id = $1`, id).
		Scan(&sc.Days, &factors, &keyWindows)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(factors, &sc.Factors); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(keyWindows, &sc.KeyWindows); err != nil {
		return nil, err
	}

	instRows, err := q.Query(ctx, `
		SELECT id, alias, descr, beta FROM instruments
		WHERE scenario_id = $1 ORDER BY ord`, id)
	if err != nil {
		return nil, err
	}
	defer instRows.Close()
	for instRows.Next() {
		var inst scenario.Instrument
		var beta []byte
		if err := instRows.Scan(&inst.ID, &inst.Alias, &inst.Desc, &beta); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(beta, &inst.Beta); err != nil {
			return nil, err
		}
		sc.Instruments = append(sc.Instruments, inst)
	}
	if err := instRows.Err(); err != nil {
		return nil, err
	}

	for i := range sc.Instruments {
		sc.Baseline[sc.Instruments[i].ID] = make([]scenario.OHLC, sc.Days)
	}
	priceRows, err := q.Query(ctx, `
		SELECT instrument_id, day, open, high, low, close
		FROM scenario_prices WHERE scenario_id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer priceRows.Close()
	for priceRows.Next() {
		var instID string
		var day int
		var p scenario.OHLC
		if err := priceRows.Scan(&instID, &day, &p.Open, &p.High, &p.Low, &p.Close); err != nil {
			return nil, err
		}
		sc.Baseline[instID][day] = p // assign by index: row order is irrelevant
	}
	return sc, priceRows.Err()
}
