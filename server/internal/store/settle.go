package store

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SettleRoom lazily settles all due orders, then reports the current day.
// Every read path (room state, portfolio, leaderboard, events) calls this
// first, so whoever arrives first triggers settlement and everyone sees
// the same result (spec §5.4). Lobby rooms are a no-op.
func SettleRoom(ctx context.Context, db *pgxpool.Pool, room *Room, now time.Time) (int, bool, error) {
	if room.StartedAt == nil {
		return 0, false, nil
	}
	day, ended, err := room.CurrentDay(now)
	if err != nil {
		return 0, false, err
	}
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		return SettleTx(ctx, tx, room, day, ended)
	})
	return day, ended, err
}

// SettleTx fills every pending order whose exec day has arrived, at that
// day's open. Concurrency-safe: FOR UPDATE serializes settlers, and under
// READ COMMITTED a blocked competitor re-evaluates the WHERE clause after
// the lock is released, so orders another transaction just filled drop
// out of its result set — settlement is idempotent.
//
// After fills it accrues loan interest day-by-day (bankrupting anyone
// whose debt crosses MaxDebtCents), refunds still-pending orders once the
// game has ended, and finally upserts the per-player daily net-asset
// snapshot feeding the leaderboard curve.
func SettleTx(ctx context.Context, tx pgx.Tx, room *Room, curDay int, ended bool) error {
	// All settlement entry points share this room-row lock. curDay is usually
	// computed before the transaction begins and may be stale after waiting
	// for another request, so clamp it to the persisted monotonic watermark.
	var settledThrough *int
	if err := tx.QueryRow(ctx, `
		SELECT settled_through_day FROM rooms WHERE id = $1 FOR UPDATE`, room.ID).
		Scan(&settledThrough); err != nil {
		return err
	}
	if settledThrough != nil && *settledThrough > curDay {
		curDay = *settledThrough
	}

	type due struct {
		id           int64
		userID       int64
		instrumentID string
		side         string
		amountCents  int64
		shares       float64
		execDay      int
	}
	collect := func(rows pgx.Rows) ([]due, error) {
		defer rows.Close()
		var out []due
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.id, &d.userID, &d.instrumentID, &d.side,
				&d.amountCents, &d.shares, &d.execDay); err != nil {
				return nil, err
			}
			out = append(out, d)
		}
		return out, rows.Err()
	}

	rows, err := tx.Query(ctx, `
		SELECT id, user_id, instrument_id, side, amount_cents, shares, exec_day
		FROM orders
		WHERE room_id = $1 AND status = 'pending' AND exec_day <= $2
		ORDER BY exec_day, id
		FOR UPDATE`, room.ID, curDay)
	if err != nil {
		return err
	}
	dueOrders, err := collect(rows)
	if err != nil {
		return err
	}

	for _, o := range dueOrders {
		var open float64
		if err := tx.QueryRow(ctx, `
			SELECT open FROM room_prices
			WHERE room_id = $1 AND instrument_id = $2 AND day = $3`,
			room.ID, o.instrumentID, o.execDay).Scan(&open); err != nil {
			return fmt.Errorf("settle order %d (day %d): %w", o.id, o.execDay, err)
		}
		var tradeShares float64
		var tradeCents int64
		if o.side == "buy" {
			tradeShares = float64(o.amountCents) / 100 / open
			tradeCents = o.amountCents
			if _, err := tx.Exec(ctx, `
				INSERT INTO positions (room_id, user_id, instrument_id, shares)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (room_id, user_id, instrument_id)
				DO UPDATE SET shares = positions.shares + EXCLUDED.shares`,
				room.ID, o.userID, o.instrumentID, tradeShares); err != nil {
				return err
			}
		} else {
			tradeShares = o.shares
			tradeCents = int64(math.Round(o.shares * open * 100))
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents + $1
				WHERE room_id = $2 AND user_id = $3`,
				tradeCents, room.ID, o.userID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO trades (order_id, room_id, user_id, instrument_id, side, day, price, shares, amount_cents)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			o.id, room.ID, o.userID, o.instrumentID, o.side, o.execDay,
			open, tradeShares, tradeCents); err != nil {
			return err
		}
		// Mark filled BEFORE the whale valuation so the order no longer
		// counts as frozen in assetsCents (no double counting).
		if _, err := tx.Exec(ctx, `UPDATE orders SET status = 'filled' WHERE id = $1`, o.id); err != nil {
			return err
		}
		assets, err := assetsCents(ctx, tx, room.ID, o.userID, o.execDay, "open")
		if err != nil {
			return err
		}
		// Whale alert: trade ≥ 20% of the player's total assets (spec §2.3).
		if assets > 0 && tradeCents*5 >= assets {
			if _, err := tx.Exec(ctx, `
				INSERT INTO room_events (room_id, day, kind, payload)
				VALUES ($1, $2, 'whale', jsonb_build_object('instrument_id', $3::text, 'side', $4::text))`,
				room.ID, o.execDay, o.instrumentID, o.side); err != nil {
				return err
			}
		}
	}

	if err := accrueInterestTx(ctx, tx, room, curDay); err != nil {
		return err
	}

	// Options: keep the rolling chain listed for the current day, then
	// cash-settle every expired position — both before the daily snapshot
	// so snapshots reflect the payoffs.
	if err := listOptionsTx(ctx, tx, room, curDay); err != nil {
		return err
	}
	if err := settleExpiredOptionsTx(ctx, tx, room, curDay); err != nil {
		return err
	}

	if ended {
		// Game over: whatever is still pending can never execute — refund it.
		if err := cancelPendingOrdersTx(ctx, tx, room.ID, nil); err != nil {
			return err
		}
	}

	if err := snapshotDailyTotalsTx(ctx, tx, room, curDay); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE rooms SET settled_through_day = GREATEST(
			COALESCE(settled_through_day, $2), $2)
		WHERE id = $1`, room.ID, curDay)
	return err
}

// accrueInterestTx compounds every active borrower's debt once per sim
// day, from interest_through_day+1 through curDay, at that day's
// market-linked rate (利滚利). Idempotent via interest_through_day.
// Debt crossing MaxDebtCents bankrupts the player at the crossing day.
func accrueInterestTx(ctx context.Context, tx pgx.Tx, room *Room, curDay int) error {
	type borrower struct {
		userID  int64
		debt    int64
		through int
	}
	rows, err := tx.Query(ctx, `
		SELECT user_id, debt_cents, interest_through_day FROM room_players
		WHERE room_id = $1 AND debt_cents > 0 AND bankrupt_day IS NULL
		ORDER BY user_id
		FOR UPDATE`, room.ID)
	if err != nil {
		return err
	}
	var borrowers []borrower
	for rows.Next() {
		var b borrower
		if err := rows.Scan(&b.userID, &b.debt, &b.through); err != nil {
			rows.Close()
			return err
		}
		borrowers = append(borrowers, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// The rate depends only on (room, day): compute it once per day.
	rateCache := map[int]float64{}
	rateFor := func(day int) (float64, error) {
		r, ok := rateCache[day]
		if !ok {
			var err error
			r, err = annualRateAt(ctx, tx, room.ID, day)
			if err != nil {
				return 0, err
			}
			rateCache[day] = r
		}
		return r, nil
	}

	for _, b := range borrowers {
		debt := b.debt
		through := curDay
		bankruptAt := -1
		for d := b.through + 1; d <= curDay; d++ {
			r, err := rateFor(d)
			if err != nil {
				return err
			}
			debt = int64(math.Round(float64(debt) * (1 + r/tradingDays)))
			if debt > MaxDebtCents {
				bankruptAt = d
				through = d
				break
			}
		}
		if bankruptAt < 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET debt_cents = $3, interest_through_day = $4
				WHERE room_id = $1 AND user_id = $2`,
				room.ID, b.userID, debt, through); err != nil {
				return err
			}
			continue
		}
		// Bankruptcy: freeze the debt at the crossing day, refund the
		// player's pending orders (same semantics as the end-of-game
		// refund), and announce it. Positions are untouched.
		if _, err := tx.Exec(ctx, `
			UPDATE room_players SET debt_cents = $3, interest_through_day = $4, bankrupt_day = $4
			WHERE room_id = $1 AND user_id = $2`,
			room.ID, b.userID, debt, bankruptAt); err != nil {
			return err
		}
		if err := cancelPendingOrdersTx(ctx, tx, room.ID, &b.userID); err != nil {
			return err
		}
		var alias string
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(NULLIF(display_name, ''), 'Player') FROM users WHERE id = $1`, b.userID).Scan(&alias); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO room_events (room_id, day, kind, payload)
			VALUES ($1, $2, 'bankrupt', jsonb_build_object('day', $3::int, 'username', $4::text))`,
			room.ID, bankruptAt, bankruptAt, alias); err != nil {
			return err
		}
	}
	return nil
}

// cancelPendingOrdersTx cancels pending orders and refunds the frozen
// side (cash for buys, shares for sells). userID nil cancels the whole
// room's leftovers at game end; otherwise just one player's (bankruptcy).
func cancelPendingOrdersTx(ctx context.Context, tx pgx.Tx, roomID int64, userID *int64) error {
	query := `
		SELECT id, user_id, instrument_id, side, amount_cents, shares
		FROM orders
		WHERE room_id = $1 AND status = 'pending'`
	args := []any{roomID}
	if userID != nil {
		query += ` AND user_id = $2`
		args = append(args, *userID)
	}
	query += ` ORDER BY id FOR UPDATE`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	type pending struct {
		id           int64
		userID       int64
		instrumentID string
		side         string
		amountCents  int64
		shares       float64
	}
	var orders []pending
	for rows.Next() {
		var o pending
		if err := rows.Scan(&o.id, &o.userID, &o.instrumentID, &o.side,
			&o.amountCents, &o.shares); err != nil {
			rows.Close()
			return err
		}
		orders = append(orders, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, o := range orders {
		if o.side == "buy" {
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents + $1
				WHERE room_id = $2 AND user_id = $3`,
				o.amountCents, roomID, o.userID); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `
				INSERT INTO positions (room_id, user_id, instrument_id, shares)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (room_id, user_id, instrument_id)
				DO UPDATE SET shares = positions.shares + EXCLUDED.shares`,
				roomID, o.userID, o.instrumentID, o.shares); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET status = 'cancelled' WHERE id = $1`, o.id); err != nil {
			return err
		}
	}
	return nil
}

// snapshotDailyTotalsTx upserts every player's net-asset total for curDay
// into room_player_daily (the leaderboard equity curve). Idempotent.
func snapshotDailyTotalsTx(ctx context.Context, tx pgx.Tx, room *Room, curDay int) error {
	rows, err := tx.Query(ctx,
		`SELECT user_id FROM room_players WHERE room_id = $1 ORDER BY user_id`, room.ID)
	if err != nil {
		return err
	}
	var userIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		userIDs = append(userIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, userID := range userIDs {
		total, err := assetsCents(ctx, tx, room.ID, userID, curDay, "close")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO room_player_daily (room_id, user_id, day, total_cents)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (room_id, user_id, day)
			DO UPDATE SET total_cents = EXCLUDED.total_cents`,
			room.ID, userID, curDay, total); err != nil {
			return err
		}
	}
	return nil
}

// priceCols whitelists the interpolated column name in assetsCents.
var priceCols = map[string]bool{"open": true, "close": true}

// assetsCents values a player's NET total assets at the given day: cash +
// positions + frozen buy cash + frozen sell shares + option marks − debt,
// using the day's open or close. Frozen amounts count — freezing must not
// dent the leaderboard. Debt-free players are unaffected by the subtraction.
func assetsCents(ctx context.Context, q Querier, roomID, userID int64, day int, priceCol string) (int64, error) {
	if !priceCols[priceCol] {
		return 0, fmt.Errorf("assetsCents: bad price column %q", priceCol)
	}
	var cash, debt int64
	if err := q.QueryRow(ctx,
		`SELECT cash_cents, debt_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		roomID, userID).Scan(&cash, &debt); err != nil {
		return 0, err
	}
	var posVal float64
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(SUM(p.shares * rp.%s), 0)
		FROM positions p
		JOIN room_prices rp ON rp.room_id = p.room_id
			AND rp.instrument_id = p.instrument_id AND rp.day = $3
		WHERE p.room_id = $1 AND p.user_id = $2`, priceCol),
		roomID, userID, day).Scan(&posVal); err != nil {
		return 0, err
	}
	var frozenBuy int64
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_cents), 0) FROM orders
		WHERE room_id = $1 AND user_id = $2 AND status = 'pending' AND side = 'buy'`,
		roomID, userID).Scan(&frozenBuy); err != nil {
		return 0, err
	}
	var frozenSellVal float64
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(SUM(o.shares * rp.%s), 0)
		FROM orders o
		JOIN room_prices rp ON rp.room_id = o.room_id
			AND rp.instrument_id = o.instrument_id AND rp.day = $3
		WHERE o.room_id = $1 AND o.user_id = $2 AND o.status = 'pending' AND o.side = 'sell'`, priceCol),
		roomID, userID, day).Scan(&frozenSellVal); err != nil {
		return 0, err
	}
	// Open option positions count at that day's Black-Scholes mark.
	optVal, err := optionValueAt(ctx, q, roomID, userID, day)
	if err != nil {
		return 0, err
	}
	return cash + frozenBuy + int64(math.Round((posVal+frozenSellVal+optVal)*100)) - debt, nil
}
