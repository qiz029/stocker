package store

import "context"

type Trade struct {
	InstrumentID string
	Side         string
	Day          int
	Price        float64
	Shares       float64
	AmountCents  int64
}

// TradesForUser returns the caller's own settled trades — the frontend
// rebuilds the personal asset curve from these. Other players' trades
// stay hidden until reveal (spec §2.2).
func TradesForUser(ctx context.Context, q Querier, roomID, userID int64) ([]Trade, error) {
	rows, err := q.Query(ctx, `
		SELECT instrument_id, side, day, price, shares, amount_cents
		FROM trades WHERE room_id = $1 AND user_id = $2
		ORDER BY day, id`, roomID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trade
	for rows.Next() {
		var t Trade
		if err := rows.Scan(&t.InstrumentID, &t.Side, &t.Day, &t.Price, &t.Shares, &t.AmountCents); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
