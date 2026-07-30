package store

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaxDebtCents caps total debt (principal + accrued interest) per player:
// borrows that would cross it are refused, and interest that pushes debt
// past it triggers bankruptcy.
const MaxDebtCents int64 = 20_000_000 // $200,000

const (
	baseAnnualRate = 0.03 // used when fewer than 2 return observations
	minAnnualRate  = 0.01
	maxAnnualRate  = 0.60
	volRateSlope   = 0.6
	tradingDays    = 252.0
	rateWindowDays = 20 // trailing sim days of returns feeding vol20
)

// LoanState is a player's debt position after a borrow/repay.
type LoanState struct {
	CashCents int64
	DebtCents int64
}

// logReturns turns a close series into consecutive daily log returns.
func logReturns(closes []float64) []float64 {
	if len(closes) < 2 {
		return nil
	}
	rets := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		if closes[i-1] <= 0 || closes[i] <= 0 {
			continue
		}
		rets = append(rets, math.Log(closes[i]/closes[i-1]))
	}
	return rets
}

// sampleStddev returns the n-1 standard deviation; len < 2 yields 0.
func sampleStddev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var mean float64
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	var ss float64
	for _, x := range xs {
		d := x - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(xs)-1))
}

// realizedVolFromReturns annualizes daily log returns into a volatility:
// stddev(rets) × √252. ok=false when there are fewer than 2 observations.
func realizedVolFromReturns(rets []float64) (vol float64, ok bool) {
	if len(rets) < 2 {
		return 0, false
	}
	return sampleStddev(rets) * math.Sqrt(tradingDays), true
}

// realizedVol annualizes a trailing close series (oldest first) into
// realized volatility. ok=false when there are fewer than 2 return
// observations.
func realizedVol(closes []float64) (vol float64, ok bool) {
	return realizedVolFromReturns(logReturns(closes))
}

// annualRateFromReturns maps daily log returns to the annual loan rate:
// clamp(0.03 + 0.6 × stddev(rets) × √252, 0.01, 0.60). Fewer than two
// observations keep the 3% base rate.
func annualRateFromReturns(rets []float64) float64 {
	vol, ok := realizedVolFromReturns(rets)
	if !ok {
		return baseAnnualRate
	}
	rate := baseAnnualRate + volRateSlope*vol
	return math.Min(math.Max(rate, minAnnualRate), maxAnnualRate)
}

// AnnualRate maps a trailing close series (oldest first) to the annual
// loan rate. Pure function; the DB-backed lookup is annualRateAt.
func AnnualRate(closes []float64) float64 {
	return annualRateFromReturns(logReturns(closes))
}

// marketProxyForRoom resolves the scenario's market-proxy instrument id
// for a room ('' = unresolved → equal-weighted basket fallback).
func marketProxyForRoom(ctx context.Context, q Querier, roomID int64) (string, error) {
	var proxy string
	err := q.QueryRow(ctx, `
		SELECT s.market_proxy FROM rooms r
		JOIN scenarios s ON s.id = r.scenario_id
		WHERE r.id = $1`, roomID).Scan(&proxy)
	return proxy, err
}

// basketReturns builds the equal-weighted basket's daily log returns over
// the days before day: per instrument, per day, the mean of log returns.
func basketReturns(ctx context.Context, q Querier, roomID int64, day int) ([]float64, error) {
	rows, err := q.Query(ctx, `
		SELECT instrument_id, day, close FROM room_prices
		WHERE room_id = $1 AND day >= $2 AND day < $3
		ORDER BY instrument_id, day`, roomID, day-rateWindowDays-1, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var prevInst string
	var prevClose float64
	sum := map[int]float64{}
	cnt := map[int]int{}
	first := true
	for rows.Next() {
		var inst string
		var d int
		var close float64
		if err := rows.Scan(&inst, &d, &close); err != nil {
			return nil, err
		}
		if first || inst != prevInst {
			first = false
			prevInst = inst
		} else if prevClose > 0 && close > 0 {
			sum[d] += math.Log(close / prevClose)
			cnt[d]++
		}
		prevClose = close
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rets := make([]float64, 0, len(sum))
	for d := day - rateWindowDays; d < day; d++ {
		if cnt[d] > 0 {
			rets = append(rets, sum[d]/float64(cnt[d]))
		}
	}
	return rets, nil
}

// annualRateAt computes the annual loan rate for (room, day) from
// room_prices alone: vol20 of the market proxy's daily log close returns
// over the trailing 20 sim days before day. Falls back to the
// equal-weighted basket when the proxy is unresolvable, and to the 3%
// base rate when there are fewer than 2 return observations.
func annualRateAt(ctx context.Context, q Querier, roomID int64, day int) (float64, error) {
	proxy, err := marketProxyForRoom(ctx, q, roomID)
	if err != nil {
		return 0, err
	}
	if proxy != "" {
		rows, err := q.Query(ctx, `
			SELECT close FROM room_prices
			WHERE room_id = $1 AND instrument_id = $2 AND day >= $3 AND day < $4
			ORDER BY day`, roomID, proxy, day-rateWindowDays-1, day)
		if err != nil {
			return 0, err
		}
		var closes []float64
		for rows.Next() {
			var c float64
			if err := rows.Scan(&c); err != nil {
				rows.Close()
				return 0, err
			}
			closes = append(closes, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return 0, err
		}
		if len(closes) > 0 {
			return AnnualRate(closes), nil
		}
	}
	rets, err := basketReturns(ctx, q, roomID, day)
	if err != nil {
		return 0, err
	}
	return annualRateFromReturns(rets), nil
}

// CurrentAnnualRate exposes annualRateAt for the HTTP layer (today's rate).
func CurrentAnnualRate(ctx context.Context, q Querier, roomID int64, day int) (float64, error) {
	return annualRateAt(ctx, q, roomID, day)
}

// Borrow lends amountCents to a player in a running room: cash rises,
// debt rises, and a 'borrow' loan_txns row records it. Settlement runs
// first so accrued interest is part of the debt the cap is checked
// against. A fresh loan (debt was 0) starts accruing the NEXT day.
func Borrow(ctx context.Context, db *pgxpool.Pool, room *Room, userID, amountCents int64, now time.Time) (*LoanState, error) {
	return loanTx(ctx, db, room, userID, amountCents, now, "borrow")
}

// Repay pays amountCents against a player's debt (≤ min(cash, debt)).
func Repay(ctx context.Context, db *pgxpool.Pool, room *Room, userID, amountCents int64, now time.Time) (*LoanState, error) {
	return loanTx(ctx, db, room, userID, amountCents, now, "repay")
}

func loanTx(ctx context.Context, db *pgxpool.Pool, room *Room, userID, amountCents int64, now time.Time, kind string) (*LoanState, error) {
	if room.Status != "running" {
		return nil, ErrRoomNotRunning
	}
	curDay, ended, err := room.CurrentDay(now)
	if err != nil {
		return nil, err
	}
	if ended {
		return nil, ErrRoomEnded
	}
	if amountCents <= 0 {
		return nil, ErrBadLoanAmount
	}

	var out *LoanState
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := SettleTx(ctx, tx, room, curDay, false); err != nil {
			return err
		}
		var cash, debt int64
		var bankruptDay *int
		if err := tx.QueryRow(ctx, `
			SELECT cash_cents, debt_cents, bankrupt_day FROM room_players
			WHERE room_id = $1 AND user_id = $2 FOR UPDATE`,
			room.ID, userID).Scan(&cash, &debt, &bankruptDay); err != nil {
			return err
		}
		if bankruptDay != nil {
			return ErrPlayerBankrupt
		}

		switch kind {
		case "borrow":
			if debt+amountCents > MaxDebtCents {
				return ErrDebtCapExceeded
			}
			// interest_through_day: a fresh loan (debt was 0) starts
			// accruing tomorrow; an ongoing loan keeps its schedule.
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET
					cash_cents = cash_cents + $1,
					debt_cents = debt_cents + $1,
					interest_through_day = CASE
						WHEN debt_cents = 0 THEN $4
						ELSE interest_through_day END
				WHERE room_id = $2 AND user_id = $3`,
				amountCents, room.ID, userID, curDay); err != nil {
				return err
			}
			cash += amountCents
			debt += amountCents
		case "repay":
			if amountCents > debt {
				return ErrBadLoanAmount
			}
			tag, err := tx.Exec(ctx, `
				UPDATE room_players SET
					cash_cents = cash_cents - $1,
					debt_cents = debt_cents - $1
				WHERE room_id = $2 AND user_id = $3 AND cash_cents >= $1`,
				amountCents, room.ID, userID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrInsufficientCash
			}
			cash -= amountCents
			debt -= amountCents
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO loan_txns (room_id, user_id, day, kind, amount_cents)
			VALUES ($1, $2, $3, $4, $5)`,
			room.ID, userID, curDay, kind, amountCents); err != nil {
			return err
		}
		out = &LoanState{CashCents: cash, DebtCents: debt}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
