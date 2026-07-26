package store

import (
	"context"
	"math"
	"sort"
)

type PortfolioPosition struct {
	InstrumentID string
	Shares       float64
	Close        float64
	ValueCents   int64
}

type PendingOrder struct {
	ID           int64
	InstrumentID string
	Side         string
	AmountCents  int64
	Shares       float64
	ExecDay      int
}

type Portfolio struct {
	CashCents  int64
	TotalCents int64
	Positions  []PortfolioPosition
	Pending    []PendingOrder
}

// GetPortfolio values a player's holdings at curDay's close. TotalCents
// counts frozen order money too, so placing an order never dents totals.
func GetPortfolio(ctx context.Context, q Querier, room *Room, userID int64, curDay int) (*Portfolio, error) {
	p := &Portfolio{}
	if err := q.QueryRow(ctx,
		`SELECT cash_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		room.ID, userID).Scan(&p.CashCents); err != nil {
		return nil, err
	}

	posRows, err := q.Query(ctx, `
		SELECT p.instrument_id, p.shares, rp.close
		FROM positions p
		JOIN room_prices rp ON rp.room_id = p.room_id
			AND rp.instrument_id = p.instrument_id AND rp.day = $3
		WHERE p.room_id = $1 AND p.user_id = $2 AND p.shares > 0
		ORDER BY p.instrument_id`, room.ID, userID, curDay)
	if err != nil {
		return nil, err
	}
	defer posRows.Close()
	for posRows.Next() {
		var pos PortfolioPosition
		if err := posRows.Scan(&pos.InstrumentID, &pos.Shares, &pos.Close); err != nil {
			return nil, err
		}
		pos.ValueCents = int64(math.Round(pos.Shares * pos.Close * 100))
		p.Positions = append(p.Positions, pos)
	}
	if err := posRows.Err(); err != nil {
		return nil, err
	}

	ordRows, err := q.Query(ctx, `
		SELECT id, instrument_id, side, amount_cents, shares, exec_day
		FROM orders
		WHERE room_id = $1 AND user_id = $2 AND status = 'pending'
		ORDER BY id`, room.ID, userID)
	if err != nil {
		return nil, err
	}
	defer ordRows.Close()
	for ordRows.Next() {
		var o PendingOrder
		if err := ordRows.Scan(&o.ID, &o.InstrumentID, &o.Side, &o.AmountCents, &o.Shares, &o.ExecDay); err != nil {
			return nil, err
		}
		p.Pending = append(p.Pending, o)
	}
	if err := ordRows.Err(); err != nil {
		return nil, err
	}

	total, err := assetsCents(ctx, q, room.ID, userID, curDay, "close")
	if err != nil {
		return nil, err
	}
	p.TotalCents = total
	return p, nil
}

type LeaderboardRow struct {
	UserID     int64
	Username   string
	TotalCents int64
	JoinedDay  int
}

// Leaderboard values every player at curDay's close. Holdings stay
// hidden (spec §2.2): only totals are exposed.
func Leaderboard(ctx context.Context, q Querier, room *Room, curDay int) ([]LeaderboardRow, error) {
	rows, err := q.Query(ctx, `
		SELECT rp.user_id, u.username, rp.joined_day
		FROM room_players rp JOIN users u ON u.id = rp.user_id
		WHERE rp.room_id = $1`, room.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeaderboardRow
	for rows.Next() {
		var r LeaderboardRow
		if err := rows.Scan(&r.UserID, &r.Username, &r.JoinedDay); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		total, err := assetsCents(ctx, q, room.ID, out[i].UserID, curDay, "close")
		if err != nil {
			return nil, err
		}
		out[i].TotalCents = total
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalCents != out[j].TotalCents {
			return out[i].TotalCents > out[j].TotalCents
		}
		return out[i].Username < out[j].Username
	})
	return out, nil
}
