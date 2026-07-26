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
func SettleTx(ctx context.Context, tx pgx.Tx, room *Room, curDay int, ended bool) error {
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

	if !ended {
		return nil
	}
	// Game over: whatever is still pending can never execute — refund it.
	rows, err = tx.Query(ctx, `
		SELECT id, user_id, instrument_id, side, amount_cents, shares, exec_day
		FROM orders
		WHERE room_id = $1 AND status = 'pending'
		ORDER BY id
		FOR UPDATE`, room.ID)
	if err != nil {
		return err
	}
	leftovers, err := collect(rows)
	if err != nil {
		return err
	}
	for _, o := range leftovers {
		if o.side == "buy" {
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents + $1
				WHERE room_id = $2 AND user_id = $3`,
				o.amountCents, room.ID, o.userID); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `
				INSERT INTO positions (room_id, user_id, instrument_id, shares)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (room_id, user_id, instrument_id)
				DO UPDATE SET shares = positions.shares + EXCLUDED.shares`,
				room.ID, o.userID, o.instrumentID, o.shares); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET status = 'cancelled' WHERE id = $1`, o.id); err != nil {
			return err
		}
	}
	return nil
}

// priceCols whitelists the interpolated column name in assetsCents.
var priceCols = map[string]bool{"open": true, "close": true}

// assetsCents values a player's total assets at the given day: cash +
// positions + frozen buy cash + frozen sell shares, using the day's open
// or close. Frozen amounts count — freezing must not dent the leaderboard.
func assetsCents(ctx context.Context, q Querier, roomID, userID int64, day int, priceCol string) (int64, error) {
	if !priceCols[priceCol] {
		return 0, fmt.Errorf("assetsCents: bad price column %q", priceCol)
	}
	var cash int64
	if err := q.QueryRow(ctx,
		`SELECT cash_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		roomID, userID).Scan(&cash); err != nil {
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
	return cash + frozenBuy + int64(math.Round((posVal+frozenSellVal)*100)), nil
}
