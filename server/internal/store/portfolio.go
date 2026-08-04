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
	// Average-cost P&L, replayed from the player's fills: buy adds its
	// amount to the cost basis, sell removes basis at the running average.
	AvgCost  float64 // dollars per share (0 when shares or basis is 0)
	PnLCents int64
	PnLPct   float64
}

type PendingOrder struct {
	ID           int64
	InstrumentID string
	Side         string
	AmountCents  int64
	Shares       float64
	ExecDay      int
}

// OptionPosition is an open option holding marked to the current day's BS
// price, with average-cost P&L tracked through option_positions.premium_paid
// (buys add the premium, sells shrink the basis proportionally).
type OptionPosition struct {
	OptionID     int64
	InstrumentID string
	Kind         string
	Strike       float64
	ExpiryDay    int
	Contracts    float64
	Price        float64 // current BS price, dollars per contract
	ValueCents   int64
	AvgCost      float64 // dollars per contract (0 when basis is 0)
	PnLCents     int64
	PnLPct       float64
}

type Portfolio struct {
	CashCents  int64
	DebtCents  int64
	Bankrupt   bool
	TotalCents int64 // net of debt
	Positions  []PortfolioPosition
	Options    []OptionPosition
	Pending    []PendingOrder
}

// GetPortfolio values a player's holdings at curDay's close. TotalCents
// counts frozen order money too, so placing an order never dents totals,
// and is NET of debt.
func GetPortfolio(ctx context.Context, q Querier, room *Room, userID int64, curDay int) (*Portfolio, error) {
	p := &Portfolio{}
	if err := q.QueryRow(ctx,
		`SELECT cash_cents, debt_cents, bankrupt_day IS NOT NULL
		FROM room_players WHERE room_id = $1 AND user_id = $2`,
		room.ID, userID).Scan(&p.CashCents, &p.DebtCents, &p.Bankrupt); err != nil {
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

	basis, err := avgCostBasis(ctx, q, room.ID, userID)
	if err != nil {
		return nil, err
	}
	for i := range p.Positions {
		pos := &p.Positions[i]
		b := basis[pos.InstrumentID]
		pos.AvgCost = b / pos.Shares / 100 // dollars per share; ≤0 is possible
		// after realized gains outran the original basis
		pos.PnLCents = pos.ValueCents - int64(math.Round(b))
		if b > 0 {
			pos.PnLPct = float64(pos.ValueCents)/b - 1
		}
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

	optRows, err := q.Query(ctx, `
		SELECT o.id, o.instrument_id, o.kind, o.strike, o.expiry_day, o.listed_day,
			p.contracts, p.premium_paid
		FROM option_positions p
		JOIN room_options o ON o.id = p.option_id
		WHERE p.room_id = $1 AND p.user_id = $2 AND p.contracts > 0
		ORDER BY o.expiry_day, o.instrument_id, o.strike, o.kind`, room.ID, userID)
	if err != nil {
		return nil, err
	}
	type optHolding struct {
		opt       RoomOption
		contracts float64
		basis     float64 // remaining premium paid, dollars
	}
	var holdings []optHolding
	for optRows.Next() {
		var h optHolding
		if err := optRows.Scan(&h.opt.ID, &h.opt.InstrumentID, &h.opt.Kind,
			&h.opt.Strike, &h.opt.ExpiryDay, &h.opt.ListedDay,
			&h.contracts, &h.basis); err != nil {
			optRows.Close()
			return nil, err
		}
		holdings = append(holdings, h)
	}
	optRows.Close()
	if err := optRows.Err(); err != nil {
		return nil, err
	}
	for _, h := range holdings {
		price, err := optionPriceAt(ctx, q, room.ID, &h.opt, curDay)
		if err != nil {
			return nil, err
		}
		op := OptionPosition{
			OptionID: h.opt.ID, InstrumentID: h.opt.InstrumentID,
			Kind: h.opt.Kind, Strike: h.opt.Strike, ExpiryDay: h.opt.ExpiryDay,
			Contracts: h.contracts, Price: price,
			ValueCents: int64(math.Round(price * h.contracts * 100)),
			AvgCost:    h.basis / h.contracts,
		}
		basisCents := int64(math.Round(h.basis * 100))
		op.PnLCents = op.ValueCents - basisCents
		if basisCents > 0 {
			op.PnLPct = float64(op.ValueCents)/float64(basisCents) - 1
		}
		p.Options = append(p.Options, op)
	}

	total, err := assetsCents(ctx, q, room.ID, userID, curDay, "close")
	if err != nil {
		return nil, err
	}
	p.TotalCents = total
	return p, nil
}

// avgCostBasis replays the player's fills chronologically and returns the
// remaining average-method cost basis (cents) per instrument. Buy: basis
// += amount, shares += s. Sell: basis -= avg × s, shares -= s, where
// avg = basis/shares at that moment.
func avgCostBasis(ctx context.Context, q Querier, roomID, userID int64) (map[string]float64, error) {
	rows, err := q.Query(ctx, `
		SELECT instrument_id, side, shares, amount_cents
		FROM trades
		WHERE room_id = $1 AND user_id = $2
		ORDER BY day, id`, roomID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type lot struct{ basis, shares float64 }
	lots := map[string]*lot{}
	for rows.Next() {
		var inst, side string
		var shares float64
		var amountCents int64
		if err := rows.Scan(&inst, &side, &shares, &amountCents); err != nil {
			return nil, err
		}
		l := lots[inst]
		if l == nil {
			l = &lot{}
			lots[inst] = l
		}
		if side == "buy" {
			l.basis += float64(amountCents)
			l.shares += shares
		} else if l.shares > 0 {
			l.basis -= l.basis / l.shares * shares
			l.shares -= shares
			if l.shares <= 1e-9 { // fully sold: drop float dust
				l.shares, l.basis = 0, 0
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(lots))
	for inst, l := range lots {
		out[inst] = l.basis
	}
	return out, nil
}

type LeaderboardRow struct {
	UserID     int64
	Username   string
	IsAgent    bool
	AvatarID   string
	TotalCents int64 // net of debt
	JoinedDay  int
	Bankrupt   bool
	Curve      []int64 // daily net totals, ordered by day
}

// Leaderboard values every player at curDay's close. Holdings stay
// hidden (spec §2.2): only totals are exposed. Active players sort
// first (net total desc), bankrupt players last (net total desc).
func Leaderboard(ctx context.Context, q Querier, room *Room, curDay int) ([]LeaderboardRow, error) {
	rows, err := q.Query(ctx, `
		SELECT rp.user_id,
			CASE WHEN u.is_agent THEN u.agent_name ELSE COALESCE(NULLIF(u.display_name, ''), u.username) END,
			u.is_agent, u.avatar_id, rp.joined_day, rp.bankrupt_day IS NOT NULL
		FROM room_players rp JOIN users u ON u.id = rp.user_id
		WHERE rp.room_id = $1`, room.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeaderboardRow
	for rows.Next() {
		var r LeaderboardRow
		if err := rows.Scan(&r.UserID, &r.Username, &r.IsAgent, &r.AvatarID, &r.JoinedDay, &r.Bankrupt); err != nil {
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
		curve, err := dailyCurve(ctx, q, room.ID, out[i].UserID)
		if err != nil {
			return nil, err
		}
		out[i].Curve = curve
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bankrupt != out[j].Bankrupt {
			return !out[i].Bankrupt // actives first
		}
		if out[i].TotalCents != out[j].TotalCents {
			return out[i].TotalCents > out[j].TotalCents
		}
		return out[i].Username < out[j].Username
	})
	return out, nil
}

// dailyCurve returns a player's snapshotted net totals ordered by day
// (empty before the first settlement snapshot).
func dailyCurve(ctx context.Context, q Querier, roomID, userID int64) ([]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT total_cents FROM room_player_daily
		WHERE room_id = $1 AND user_id = $2
		ORDER BY day`, roomID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
