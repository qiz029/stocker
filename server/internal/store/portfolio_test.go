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
