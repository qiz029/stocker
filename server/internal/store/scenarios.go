package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
			`INSERT INTO scenarios (id, days, factors, key_windows, era_hint, market_proxy) VALUES ($1, $2, $3, $4, $5, $6)`,
			sc.ID, sc.Days, string(factors), string(keyWindows), sc.EraHint, sc.MarketProxy); err != nil {
			return err
		}
		for ord, inst := range sc.Instruments {
			beta, err := json.Marshal(inst.Beta)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO instruments (scenario_id, id, ord, alias, descr, descr_en, beta, idio_scale, reconstructed)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				sc.ID, inst.ID, ord, inst.Alias, inst.Desc, inst.DescEn, string(beta),
				inst.IdioScale, inst.Reconstructed); err != nil {
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
		`SELECT days, factors, key_windows, era_hint, market_proxy FROM scenarios WHERE id = $1`, id).
		Scan(&sc.Days, &factors, &keyWindows, &sc.EraHint, &sc.MarketProxy)
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
		SELECT id, alias, descr, descr_en, beta, idio_scale, reconstructed, aliases FROM instruments
		WHERE scenario_id = $1 ORDER BY ord`, id)
	if err != nil {
		return nil, err
	}
	defer instRows.Close()
	for instRows.Next() {
		var inst scenario.Instrument
		var beta []byte
		if err := instRows.Scan(&inst.ID, &inst.Alias, &inst.Desc, &inst.DescEn, &beta,
			&inst.IdioScale, &inst.Reconstructed, &inst.Aliases); err != nil {
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

type InstrumentDisplay struct {
	Alias    string `json:"-"`
	Desc     string `json:"-"`
	RealName string `json:"-"`
	// Aliases is the candidate set for the per-room alias pick; empty
	// leaves the aliases column NULL (readers fall back to Alias).
	Aliases  []string `json:"-"`
	Business string   `json:"business"`
	Bull     string   `json:"bull"`
	Bear     string   `json:"bear"`
	// English copies of the display copy; empty means "no translation",
	// readers fall back to the Chinese field. The *En fields are stored in
	// the descr_en / profile_en columns, never inside the profile JSONB.
	DescEn     string `json:"-"`
	BusinessEn string `json:"-"`
	BullEn     string `json:"-"`
	BearEn     string `json:"-"`
}

// SetInstrumentDisplay overwrites display-only columns (alias, descr,
// descr_en, profile, profile_en, real_name, aliases) for existing
// instruments. World generation never reads these, so applying them after
// SaveScenario cannot affect determinism.
func SetInstrumentDisplay(ctx context.Context, db *pgxpool.Pool, scenarioID string, display map[string]InstrumentDisplay) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		for id, d := range display {
			profile, err := json.Marshal(d)
			if err != nil {
				return err
			}
			var profileEn any // nil → NULL column: no English profile
			if d.BusinessEn != "" || d.BullEn != "" || d.BearEn != "" {
				b, err := json.Marshal(struct {
					Business string `json:"business"`
					Bull     string `json:"bull"`
					Bear     string `json:"bear"`
				}{d.BusinessEn, d.BullEn, d.BearEn})
				if err != nil {
					return err
				}
				profileEn = string(b)
			}
			var aliases any // nil → NULL column: no candidates recorded
			if len(d.Aliases) > 0 {
				b, err := json.Marshal(d.Aliases)
				if err != nil {
					return err
				}
				aliases = string(b)
			}
			tag, err := tx.Exec(ctx, `
				UPDATE instruments SET alias = $3, descr = $4, descr_en = $5, profile = $6, profile_en = $7, real_name = $8, aliases = $9
				WHERE scenario_id = $1 AND id = $2`,
				scenarioID, id, d.Alias, d.Desc, d.DescEn, string(profile), profileEn, d.RealName, aliases)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return fmt.Errorf("set display: unknown instrument %q in scenario %q", id, scenarioID)
			}
		}
		return nil
	})
}

func SetScenarioMeta(ctx context.Context, q Querier, id, name, nameEn, realPeriod string) error {
	tag, err := q.Exec(ctx,
		`UPDATE scenarios SET name = $2, name_en = $3, real_period = $4 WHERE id = $1`,
		id, name, nameEn, realPeriod)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type ScenarioInfo struct {
	ID, Name, NameEn string
	Days             int
}

func ScenarioInfos(ctx context.Context, q Querier) ([]ScenarioInfo, error) {
	rows, err := q.Query(ctx, `SELECT id, name, name_en, days FROM scenarios ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScenarioInfo
	for rows.Next() {
		var s ScenarioInfo
		if err := rows.Scan(&s.ID, &s.Name, &s.NameEn, &s.Days); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
