package store

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	optionRiskFreeRate  = 0.03
	optionFallbackVol   = 0.30 // when neither instrument nor basket vol is computable
	optionExpirySpacing = 5    // expiries land every 5 sim days
	optionChainAhead    = 2    // upcoming expiries kept listed
)

// strikeMults anchor a listing's strikes to the listing day's close.
var strikeMults = []float64{0.80, 0.90, 1.00, 1.10, 1.20}

// RoomOption is one listed contract (an instrument × kind × strike × expiry).
type RoomOption struct {
	ID           int64
	InstrumentID string
	Kind         string // "call" | "put"
	Strike       float64
	ExpiryDay    int
	ListedDay    int
}

// OptionFill reports an executed buy/sell: the BS price per contract, the
// premium exchanged (cents), and the player's cash after the fill.
type OptionFill struct {
	Action      string
	Contracts   float64
	Price       float64
	AmountCents int64
	CashCents   int64
}

// normCDF is the standard normal CDF via math.Erf (deterministic).
func normCDF(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt2))
}

// BlackScholes prices a European call/put (long side, dollars per share).
// T is years to expiry; T <= 0 (or degenerate inputs) yields intrinsic value.
func BlackScholes(kind string, S, K, T, r, sigma float64) float64 {
	if T <= 0 || sigma <= 0 || S <= 0 || K <= 0 {
		if kind == "call" {
			return math.Max(S-K, 0)
		}
		return math.Max(K-S, 0)
	}
	d1 := (math.Log(S/K) + (r+0.5*sigma*sigma)*T) / (sigma * math.Sqrt(T))
	d2 := d1 - sigma*math.Sqrt(T)
	if kind == "call" {
		return S*normCDF(d1) - K*math.Exp(-r*T)*normCDF(d2)
	}
	return K*math.Exp(-r*T)*normCDF(-d2) - S*normCDF(-d1)
}

// upcomingExpiries returns the next n expiry days strictly after day:
// multiples of optionExpirySpacing below totalDays.
func upcomingExpiries(day, totalDays, n int) []int {
	var out []int
	for e := (day/optionExpirySpacing + 1) * optionExpirySpacing; e < totalDays && len(out) < n; e += optionExpirySpacing {
		out = append(out, e)
	}
	return out
}

// listOptionsTx ensures contracts exist for the next optionChainAhead
// expiries after day: every instrument's close on the listing day ×
// strikeMults (rounded to cents, deduped) × {call, put}. Each expiry is
// listed once — the first day it enters the 2-expiry lookahead — so the
// strike anchor stays put; the UNIQUE key keeps the insert idempotent.
func listOptionsTx(ctx context.Context, tx pgx.Tx, room *Room, day int) error {
	expiries := upcomingExpiries(day, room.Days, optionChainAhead)
	if len(expiries) == 0 {
		return nil
	}
	// Skip expiries that already have a chain listed.
	pending := expiries[:0]
	for _, expiry := range expiries {
		var listed bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM room_options WHERE room_id = $1 AND expiry_day = $2)`,
			room.ID, expiry).Scan(&listed); err != nil {
			return err
		}
		if !listed {
			pending = append(pending, expiry)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT instrument_id, close FROM room_prices
		WHERE room_id = $1 AND day = $2 ORDER BY instrument_id`, room.ID, day)
	if err != nil {
		return err
	}
	type quote struct {
		instrumentID string
		close        float64
	}
	var quotes []quote
	for rows.Next() {
		var q quote
		if err := rows.Scan(&q.instrumentID, &q.close); err != nil {
			rows.Close()
			return err
		}
		quotes = append(quotes, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, expiry := range pending {
		for _, q := range quotes {
			if q.close <= 0 {
				continue
			}
			seen := map[float64]bool{}
			for _, m := range strikeMults {
				strike := math.Round(q.close*m*100) / 100
				if seen[strike] {
					continue
				}
				seen[strike] = true
				for _, kind := range []string{"call", "put"} {
					if _, err := tx.Exec(ctx, `
						INSERT INTO room_options (room_id, instrument_id, kind, strike, expiry_day, listed_day)
						VALUES ($1, $2, $3, $4, $5, $6)
						ON CONFLICT (room_id, instrument_id, kind, strike, expiry_day) DO NOTHING`,
						room.ID, q.instrumentID, kind, strike, expiry, day); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// optionSigmaAt resolves the BS volatility for (instrument, day): the
// instrument's trailing-20-day realized vol (same window convention as the
// loan rate), falling back to the whole-room equal-weighted basket vol, and
// finally to a fixed 30%.
func optionSigmaAt(ctx context.Context, q Querier, roomID int64, instrumentID string, day int) (float64, error) {
	rows, err := q.Query(ctx, `
		SELECT close FROM room_prices
		WHERE room_id = $1 AND instrument_id = $2 AND day >= $3 AND day < $4
		ORDER BY day`, roomID, instrumentID, day-rateWindowDays-1, day)
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
	if vol, ok := realizedVol(closes); ok {
		return vol, nil
	}
	rets, err := basketReturns(ctx, q, roomID, day)
	if err != nil {
		return 0, err
	}
	if vol, ok := realizedVolFromReturns(rets); ok {
		return vol, nil
	}
	return optionFallbackVol, nil
}

// optionPriceAt values a contract at the given day: S = that day's close,
// T = (expiry_day − day)/252 years (clamped at 0 → intrinsic), r = 3%,
// σ = optionSigmaAt. Deterministic in (room, day).
func optionPriceAt(ctx context.Context, q Querier, roomID int64, opt *RoomOption, day int) (float64, error) {
	var close float64
	if err := q.QueryRow(ctx, `
		SELECT close FROM room_prices
		WHERE room_id = $1 AND instrument_id = $2 AND day = $3`,
		roomID, opt.InstrumentID, day).Scan(&close); err != nil {
		return 0, err
	}
	t := float64(opt.ExpiryDay-day) / tradingDays
	if t < 0 {
		t = 0
	}
	sigma, err := optionSigmaAt(ctx, q, roomID, opt.InstrumentID, day)
	if err != nil {
		return 0, err
	}
	return BlackScholes(opt.Kind, close, opt.Strike, t, optionRiskFreeRate, sigma), nil
}

// loadOptionTx fetches a contract of this room (FOR UPDATE).
func loadOptionTx(ctx context.Context, tx pgx.Tx, roomID, optionID int64) (*RoomOption, error) {
	opt := &RoomOption{ID: optionID}
	err := tx.QueryRow(ctx, `
		SELECT instrument_id, kind, strike, expiry_day, listed_day
		FROM room_options WHERE id = $1 AND room_id = $2 FOR UPDATE`,
		optionID, roomID).Scan(&opt.InstrumentID, &opt.Kind, &opt.Strike, &opt.ExpiryDay, &opt.ListedDay)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return opt, nil
}

// BuyOption opens (or adds to) a long position at the current day's BS
// price, debiting the premium immediately. SellOption closes contracts at
// the same mark, crediting the premium. Both settle first so the mark and
// the player's cash are current.
func BuyOption(ctx context.Context, db *pgxpool.Pool, room *Room, userID, optionID int64, contracts float64, now time.Time) (*OptionFill, error) {
	return optionOrderTx(ctx, db, room, userID, optionID, contracts, now, "buy")
}

func SellOption(ctx context.Context, db *pgxpool.Pool, room *Room, userID, optionID int64, contracts float64, now time.Time) (*OptionFill, error) {
	return optionOrderTx(ctx, db, room, userID, optionID, contracts, now, "sell")
}

func optionOrderTx(ctx context.Context, db *pgxpool.Pool, room *Room, userID, optionID int64, contracts float64, now time.Time, action string) (*OptionFill, error) {
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
	if contracts <= 0 || math.IsNaN(contracts) || math.IsInf(contracts, 0) {
		return nil, ErrBadOptionOrder
	}

	var out *OptionFill
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := SettleTx(ctx, tx, room, curDay, false); err != nil {
			return err
		}
		opt, err := loadOptionTx(ctx, tx, room.ID, optionID)
		if err != nil {
			return err
		}
		// Expired (or expiring-today) contracts are not tradeable.
		if opt.ExpiryDay <= curDay {
			return ErrBadOptionOrder
		}
		var cash int64
		var bankrupt bool
		if err := tx.QueryRow(ctx, `
			SELECT cash_cents, bankrupt_day IS NOT NULL FROM room_players
			WHERE room_id = $1 AND user_id = $2 FOR UPDATE`,
			room.ID, userID).Scan(&cash, &bankrupt); err != nil {
			return err
		}
		if bankrupt {
			return ErrPlayerBankrupt
		}
		price, err := optionPriceAt(ctx, tx, room.ID, opt, curDay)
		if err != nil {
			return err
		}
		amountCents := int64(math.Round(price * contracts * 100))

		switch action {
		case "buy":
			tag, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents - $1
				WHERE room_id = $2 AND user_id = $3 AND cash_cents >= $1`,
				amountCents, room.ID, userID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrInsufficientCash
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO option_positions (room_id, user_id, option_id, contracts, premium_paid)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (room_id, user_id, option_id)
				DO UPDATE SET contracts = option_positions.contracts + EXCLUDED.contracts,
					premium_paid = option_positions.premium_paid + EXCLUDED.premium_paid`,
				room.ID, userID, optionID, contracts, float64(amountCents)/100); err != nil {
				return err
			}
			cash -= amountCents
		case "sell":
			// Average-cost close: the cost basis shrinks proportionally.
			tag, err := tx.Exec(ctx, `
				UPDATE option_positions SET
					premium_paid = premium_paid * (contracts - $1) / contracts,
					contracts = contracts - $1
				WHERE room_id = $2 AND user_id = $3 AND option_id = $4 AND contracts >= $1`,
				contracts, room.ID, userID, optionID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrInsufficientContracts
			}
			if _, err := tx.Exec(ctx, `
				DELETE FROM option_positions
				WHERE room_id = $1 AND user_id = $2 AND option_id = $3 AND contracts <= 1e-9`,
				room.ID, userID, optionID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents + $1
				WHERE room_id = $2 AND user_id = $3`,
				amountCents, room.ID, userID); err != nil {
				return err
			}
			cash += amountCents
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO option_trades (room_id, user_id, option_id, day, action, contracts, price, amount_cents)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			room.ID, userID, optionID, curDay, action, contracts, price, amountCents); err != nil {
			return err
		}
		out = &OptionFill{Action: action, Contracts: contracts, Price: price, AmountCents: amountCents, CashCents: cash}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListOptions returns the tradeable chain (expiry after day) for one
// instrument, priced at the day, ordered by expiry then strike.
func ListOptions(ctx context.Context, q Querier, roomID int64, instrumentID string, day int) ([]RoomOption, []float64, error) {
	rows, err := q.Query(ctx, `
		SELECT id, instrument_id, kind, strike, expiry_day, listed_day
		FROM room_options
		WHERE room_id = $1 AND instrument_id = $2 AND expiry_day > $3
		ORDER BY expiry_day, strike, kind`, roomID, instrumentID, day)
	if err != nil {
		return nil, nil, err
	}
	var opts []RoomOption
	var ids []int64
	for rows.Next() {
		var o RoomOption
		if err := rows.Scan(&o.ID, &o.InstrumentID, &o.Kind, &o.Strike, &o.ExpiryDay, &o.ListedDay); err != nil {
			rows.Close()
			return nil, nil, err
		}
		opts = append(opts, o)
		ids = append(ids, o.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	prices := make([]float64, len(opts))
	for i := range opts {
		p, err := optionPriceAt(ctx, q, roomID, &opts[i], day)
		if err != nil {
			return nil, nil, err
		}
		prices[i] = p
	}
	return opts, prices, nil
}

// settleExpiredOptionsTx cash-settles every position whose expiry day has
// arrived, at the expiry day's close: payoff = max(S−K, 0) for calls,
// max(K−S, 0) for puts. The position is deleted and an 'expiry'
// option_trades row records it (amount 0 when worthless), which makes
// re-settling idempotent.
func settleExpiredOptionsTx(ctx context.Context, tx pgx.Tx, room *Room, curDay int) error {
	type holding struct {
		userID       int64
		optionID     int64
		contracts    float64
		instrumentID string
		kind         string
		strike       float64
		expiryDay    int
	}
	rows, err := tx.Query(ctx, `
		SELECT p.user_id, p.option_id, p.contracts, o.instrument_id, o.kind, o.strike, o.expiry_day
		FROM option_positions p
		JOIN room_options o ON o.id = p.option_id
		WHERE p.room_id = $1 AND o.expiry_day <= $2 AND p.contracts > 0
		ORDER BY o.expiry_day, p.option_id, p.user_id
		FOR UPDATE OF p`, room.ID, curDay)
	if err != nil {
		return err
	}
	var holdings []holding
	for rows.Next() {
		var h holding
		if err := rows.Scan(&h.userID, &h.optionID, &h.contracts,
			&h.instrumentID, &h.kind, &h.strike, &h.expiryDay); err != nil {
			rows.Close()
			return err
		}
		holdings = append(holdings, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, h := range holdings {
		var close float64
		if err := tx.QueryRow(ctx, `
			SELECT close FROM room_prices
			WHERE room_id = $1 AND instrument_id = $2 AND day = $3`,
			room.ID, h.instrumentID, h.expiryDay).Scan(&close); err != nil {
			return err
		}
		payoff := math.Max(close-h.strike, 0)
		if h.kind == "put" {
			payoff = math.Max(h.strike-close, 0)
		}
		amountCents := int64(math.Round(payoff * h.contracts * 100))
		if amountCents > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents + $1
				WHERE room_id = $2 AND user_id = $3`,
				amountCents, room.ID, h.userID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM option_positions
			WHERE room_id = $1 AND user_id = $2 AND option_id = $3`,
			room.ID, h.userID, h.optionID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO option_trades (room_id, user_id, option_id, day, action, contracts, price, amount_cents)
			VALUES ($1, $2, $3, $4, 'expiry', $5, $6, $7)`,
			room.ID, h.userID, h.optionID, h.expiryDay, h.contracts, payoff, amountCents); err != nil {
			return err
		}
	}
	return nil
}

// optionValueAt marks a player's option book to market at day (BS price ×
// contracts, dollars). Used by assetsCents and the portfolio.
func optionValueAt(ctx context.Context, q Querier, roomID, userID int64, day int) (float64, error) {
	rows, err := q.Query(ctx, `
		SELECT o.instrument_id, o.kind, o.strike, o.expiry_day, o.listed_day, p.contracts
		FROM option_positions p
		JOIN room_options o ON o.id = p.option_id
		WHERE p.room_id = $1 AND p.user_id = $2 AND p.contracts > 0`, roomID, userID)
	if err != nil {
		return 0, err
	}
	type pos struct {
		opt       RoomOption
		contracts float64
	}
	var positions []pos
	for rows.Next() {
		var p pos
		if err := rows.Scan(&p.opt.InstrumentID, &p.opt.Kind, &p.opt.Strike,
			&p.opt.ExpiryDay, &p.opt.ListedDay, &p.contracts); err != nil {
			rows.Close()
			return 0, err
		}
		positions = append(positions, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var total float64
	for _, p := range positions {
		price, err := optionPriceAt(ctx, q, roomID, &p.opt, day)
		if err != nil {
			return 0, err
		}
		total += price * p.contracts
	}
	return total, nil
}
