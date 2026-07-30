package store

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestPortfolioValuation(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Frozen-but-unsettled money still counts: total == initial exactly.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 4_000_000}); err != nil {
		t.Fatal(err)
	}
	p, err := GetPortfolio(ctx, pool, room, guest.ID, 0)
	if err != nil {
		t.Fatalf("GetPortfolio: %v", err)
	}
	if p.CashCents != 6_000_000 || p.TotalCents != InitialCashCents {
		t.Fatalf("frozen portfolio: cash=%d total=%d, want 6000000 / %d",
			p.CashCents, p.TotalCents, InitialCashCents)
	}
	if len(p.Pending) != 1 || p.Pending[0].Side != "buy" || p.Pending[0].ExecDay != 1 {
		t.Fatalf("pending: %+v", p.Pending)
	}

	// After settlement the position is valued at the current day's close.
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}
	p, err = GetPortfolio(ctx, pool, room, guest.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Positions) != 1 || p.Positions[0].InstrumentID != "S1" {
		t.Fatalf("positions: %+v", p.Positions)
	}
	var open1, close1 float64
	if err := pool.QueryRow(ctx, `
		SELECT open, close FROM room_prices
		WHERE room_id=$1 AND instrument_id='S1' AND day=1`, room.ID).Scan(&open1, &close1); err != nil {
		t.Fatal(err)
	}
	wantShares := 40_000.0 / open1
	if math.Abs(p.Positions[0].Shares-wantShares) > 1e-9 || p.Positions[0].Close != close1 {
		t.Fatalf("position detail: %+v, want shares %v close %v", p.Positions[0], wantShares, close1)
	}
	wantTotal := 6_000_000 + int64(math.Round(wantShares*close1*100))
	if p.TotalCents != wantTotal {
		t.Fatalf("total = %d, want %d", p.TotalCents, wantTotal)
	}
}

func TestLeaderboardOrdering(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// guest goes all-in on S1 at day-1 open; host stays in cash.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: InitialCashCents}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}
	rows, err := Leaderboard(ctx, pool, room, 1)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("leaderboard rows = %d, want 2", len(rows))
	}
	// S1 is the tech-bubble instrument: its day-1 close is above its
	// day-1 open in the synthetic scenario's boom phase, so the all-in
	// player leads; regardless, ordering must match the totals.
	if rows[0].TotalCents < rows[1].TotalCents {
		t.Fatalf("leaderboard not sorted desc: %+v", rows)
	}
	for _, r := range rows {
		if r.Username == "" {
			t.Fatalf("missing username: %+v", r)
		}
	}
}

func TestPortfolioNetOfDebtAndAvgCostPnL(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Day 0: buy $40,000 of S1 and borrow $10,000.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 4_000_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := Borrow(ctx, pool, room, guest.ID, 1_000_000, t0); err != nil {
		t.Fatal(err)
	}
	p, err := GetPortfolio(ctx, pool, room, guest.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Cash = 10M − 4M frozen + 1M borrowed; net total stays at initial.
	if p.CashCents != 7_000_000 || p.DebtCents != 1_000_000 || p.Bankrupt {
		t.Fatalf("day-0 portfolio: %+v", p)
	}
	if p.TotalCents != InitialCashCents {
		t.Fatalf("net total = %d, want %d", p.TotalCents, InitialCashCents)
	}

	// Day 1: the buy filled; avg cost is the fill price and P&L marks to close.
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}
	p, err = GetPortfolio(ctx, pool, room, guest.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Positions) != 1 {
		t.Fatalf("positions: %+v", p.Positions)
	}
	pos := p.Positions[0]
	var open1, close1 float64
	if err := pool.QueryRow(ctx, `
		SELECT open, close FROM room_prices
		WHERE room_id=$1 AND instrument_id='S1' AND day=1`, room.ID).Scan(&open1, &close1); err != nil {
		t.Fatal(err)
	}
	if math.Abs(pos.AvgCost-open1) > 1e-9 {
		t.Fatalf("avg cost = %v, want day-1 open %v", pos.AvgCost, open1)
	}
	wantShares := 40_000.0 / open1
	wantValue := int64(math.Round(wantShares * close1 * 100))
	if pos.ValueCents != wantValue {
		t.Fatalf("value = %d, want %d", pos.ValueCents, wantValue)
	}
	if pos.PnLCents != wantValue-4_000_000 {
		t.Fatalf("pnl = %d, want %d", pos.PnLCents, wantValue-4_000_000)
	}
	wantPct := float64(wantValue)/4_000_000 - 1
	if math.Abs(pos.PnLPct-wantPct) > 1e-9 {
		t.Fatalf("pnl pct = %v, want %v", pos.PnLPct, wantPct)
	}
	// Total is net of debt (debt also accrued one day of interest).
	debt, _, _ := debtOf(t, pool, room.ID, guest.ID)
	if want := 7_000_000 + wantValue - debt; p.TotalCents != want {
		t.Fatalf("net total = %d, want %d", p.TotalCents, want)
	}
}

func TestLeaderboardBankruptSortingAndCurve(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Guest borrows to the cap; day-1 settle bankrupts them. Host stays clean.
	if _, err := Borrow(ctx, pool, room, guest.ID, MaxDebtCents, t0); err != nil {
		t.Fatal(err)
	}
	at1 := t0.Add(61 * time.Second)
	if _, _, err := SettleRoom(ctx, pool, room, at1); err != nil {
		t.Fatal(err)
	}
	// Gift the bankrupt guest extra cash so their NET total beats the
	// host's: bankrupt-last must dominate total-desc. Re-settle so the
	// day-1 snapshot reflects it.
	if _, err := pool.Exec(ctx, `
		UPDATE room_players SET cash_cents = cash_cents + 10_000_000
		WHERE room_id = $1 AND user_id = $2`, room.ID, guest.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SettleRoom(ctx, pool, room, at1); err != nil {
		t.Fatal(err)
	}
	rows, err := Leaderboard(ctx, pool, room, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	host, guestRow := rows[0], rows[1]
	if host.UserID != room.HostUserID || host.Bankrupt {
		t.Fatalf("first row: %+v, want solvent host", host)
	}
	if guestRow.UserID != guest.ID || !guestRow.Bankrupt {
		t.Fatalf("last row: %+v, want bankrupt guest", guestRow)
	}
	// The bankrupt guest has the higher net total, yet sorts last.
	if guestRow.TotalCents <= host.TotalCents {
		t.Fatalf("expected bankrupt guest total (%d) > host (%d) to prove sort is not by total",
			guestRow.TotalCents, host.TotalCents)
	}
	// Curve: snapshots for days 0 and 1; last entry equals the current total.
	if len(guestRow.Curve) != 2 {
		t.Fatalf("curve = %v, want 2 entries", guestRow.Curve)
	}
	if guestRow.Curve[1] != guestRow.TotalCents {
		t.Fatalf("curve tail = %d, want current total %d", guestRow.Curve[1], guestRow.TotalCents)
	}
	if len(host.Curve) != 2 || host.Curve[0] != InitialCashCents || host.Curve[1] != InitialCashCents {
		t.Fatalf("host curve = %v", host.Curve)
	}
}

func TestPortfolioShowsOptions(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)
	optID := firstOptionID(t, pool, room.ID, "call", 10)

	fill, err := BuyOption(ctx, pool, room, guest.ID, optID, 2, t0)
	if err != nil {
		t.Fatal(err)
	}
	p, err := GetPortfolio(ctx, pool, room, guest.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Options) != 1 {
		t.Fatalf("options = %+v, want 1 entry", p.Options)
	}
	o := p.Options[0]
	if o.OptionID != optID || o.InstrumentID != "S1" || o.Kind != "call" ||
		o.ExpiryDay != 10 || o.Contracts != 2 {
		t.Fatalf("option detail: %+v", o)
	}
	// Same-day mark: value == premium paid, P&L zero, total untouched.
	if o.Price != fill.Price || o.ValueCents != fill.AmountCents {
		t.Fatalf("option mark: price %v value %d, want %v / %d", o.Price, o.ValueCents, fill.Price, fill.AmountCents)
	}
	if math.Abs(o.AvgCost-fill.Price) > 0.01 {
		t.Fatalf("avg cost %v, want ≈%v", o.AvgCost, fill.Price)
	}
	if o.PnLCents != 0 || o.PnLPct != 0 {
		t.Fatalf("same-day pnl = %d / %v, want 0", o.PnLCents, o.PnLPct)
	}
	if p.TotalCents != InitialCashCents {
		t.Fatalf("total = %d, want %d", p.TotalCents, InitialCashCents)
	}

	// Three days on, the BS mark moves; value and P&L track it.
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(3*61*time.Second+time.Second)); err != nil {
		t.Fatal(err)
	}
	p, err = GetPortfolio(ctx, pool, room, guest.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Options) != 1 {
		t.Fatalf("options day 3 = %+v, want 1 entry", p.Options)
	}
	o = p.Options[0]
	var opt RoomOption
	if err := pool.QueryRow(ctx, `
		SELECT instrument_id, kind, strike, expiry_day FROM room_options WHERE id = $1`,
		optID).Scan(&opt.InstrumentID, &opt.Kind, &opt.Strike, &opt.ExpiryDay); err != nil {
		t.Fatal(err)
	}
	wantPrice, err := optionPriceAt(ctx, pool, room.ID, &opt, 3)
	if err != nil {
		t.Fatal(err)
	}
	if o.Price != wantPrice {
		t.Fatalf("day-3 price = %v, want BS %v", o.Price, wantPrice)
	}
	wantValue := int64(math.Round(wantPrice * 2 * 100))
	if o.ValueCents != wantValue {
		t.Fatalf("day-3 value = %d, want %d", o.ValueCents, wantValue)
	}
	if o.PnLCents != wantValue-fill.AmountCents {
		t.Fatalf("day-3 pnl = %d, want %d", o.PnLCents, wantValue-fill.AmountCents)
	}
	wantTotal := p.CashCents + wantValue
	if p.TotalCents != wantTotal {
		t.Fatalf("day-3 total = %d, want cash+value %d", p.TotalCents, wantTotal)
	}
}
