package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Order struct {
	ID           int64
	RoomID       int64
	UserID       int64
	InstrumentID string
	Side         string  // "buy" | "sell"
	AmountCents  int64   // buy: frozen cash to spend at next open
	Shares       float64 // sell: frozen shares to liquidate at next open
	ExecDay      int
	Status       string // "pending" | "filled" | "cancelled"
}

type OrderReq struct {
	InstrumentID string
	Side         string
	AmountCents  int64
	Shares       float64
}

// PlaceOrder freezes the paying side immediately (spec §2.2 下单即冻结)
// and schedules execution at the next day's open. Placement is refused
// once the game has ended.
func PlaceOrder(ctx context.Context, db *pgxpool.Pool, room *Room, userID int64, now time.Time, req OrderReq) (*Order, error) {
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
	if req.Side != "buy" && req.Side != "sell" {
		return nil, ErrBadOrder
	}
	if req.Side == "buy" && req.AmountCents <= 0 {
		return nil, ErrBadOrder
	}
	if req.Side == "sell" && req.Shares <= 0 {
		return nil, ErrBadOrder
	}

	var out *Order
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := SettleTx(ctx, tx, room, curDay, false); err != nil {
			return err
		}
		var bankrupt bool
		if err := tx.QueryRow(ctx, `
			SELECT bankrupt_day IS NOT NULL FROM room_players
			WHERE room_id = $1 AND user_id = $2`,
			room.ID, userID).Scan(&bankrupt); err != nil {
			return err
		}
		if bankrupt {
			return ErrPlayerBankrupt
		}
		var one int
		err := tx.QueryRow(ctx, `
			SELECT 1 FROM room_prices
			WHERE room_id = $1 AND instrument_id = $2 AND day = 0`,
			room.ID, req.InstrumentID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownInstrument
		}
		if err != nil {
			return err
		}

		switch req.Side {
		case "buy":
			tag, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents - $1
				WHERE room_id = $2 AND user_id = $3 AND cash_cents >= $1`,
				req.AmountCents, room.ID, userID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrInsufficientCash
			}
			req.Shares = 0
		case "sell":
			tag, err := tx.Exec(ctx, `
				UPDATE positions SET shares = shares - $1
				WHERE room_id = $2 AND user_id = $3 AND instrument_id = $4 AND shares >= $1`,
				req.Shares, room.ID, userID, req.InstrumentID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrInsufficientShares
			}
			req.AmountCents = 0
		}

		o := &Order{
			RoomID: room.ID, UserID: userID, InstrumentID: req.InstrumentID,
			Side: req.Side, AmountCents: req.AmountCents, Shares: req.Shares,
			ExecDay: curDay + 1, Status: "pending",
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO orders (room_id, user_id, instrument_id, side, amount_cents, shares, exec_day)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			o.RoomID, o.UserID, o.InstrumentID, o.Side, o.AmountCents, o.Shares, o.ExecDay).Scan(&o.ID); err != nil {
			return err
		}
		out = o
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CancelOrder cancels a still-pending order and returns the frozen side.
// Once the exec day has been settled the order is filled and no longer
// cancellable (spec §2.2 成交日到来前可撤单).
func CancelOrder(ctx context.Context, db *pgxpool.Pool, room *Room, userID, orderID int64, now time.Time) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if room.Status == "running" {
			curDay, ended, err := room.CurrentDay(now)
			if err != nil {
				return err
			}
			if err := SettleTx(ctx, tx, room, curDay, ended); err != nil {
				return err
			}
		}
		var side, instrumentID string
		var amountCents int64
		var shares float64
		err := tx.QueryRow(ctx, `
			UPDATE orders SET status = 'cancelled'
			WHERE id = $1 AND room_id = $2 AND user_id = $3 AND status = 'pending'
			RETURNING side, instrument_id, amount_cents, shares`,
			orderID, room.ID, userID).Scan(&side, &instrumentID, &amountCents, &shares)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotCancellable
		}
		if err != nil {
			return err
		}
		if side == "buy" {
			_, err = tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents + $1
				WHERE room_id = $2 AND user_id = $3`, amountCents, room.ID, userID)
		} else {
			_, err = tx.Exec(ctx, `
				INSERT INTO positions (room_id, user_id, instrument_id, shares)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (room_id, user_id, instrument_id)
				DO UPDATE SET shares = positions.shares + EXCLUDED.shares`,
				room.ID, userID, instrumentID, shares)
		}
		return err
	})
}
