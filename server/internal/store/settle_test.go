package store

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestSettleBuyAndSell(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Day 0: buy $40,000 of S1, executes at day 1 open.
	o, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 4_000_000})
	if err != nil {
		t.Fatal(err)
	}

	// Advance to day 1 and settle via a read path.
	at := t0.Add(61 * time.Second)
	day, ended, err := SettleRoom(ctx, pool, room, at)
	if err != nil || day != 1 || ended {
		t.Fatalf("SettleRoom: day=%d ended=%v err=%v", day, ended, err)
	}

	var open float64
	if err := pool.QueryRow(ctx, `
		SELECT open FROM room_prices WHERE room_id=$1 AND instrument_id='S1' AND day=1`,
		room.ID).Scan(&open); err != nil {
		t.Fatal(err)
	}
	wantShares := 40_000.0 / open // $40,000 at day-1 open

	var gotShares float64
	if err := pool.QueryRow(ctx, `
		SELECT shares FROM positions WHERE room_id=$1 AND user_id=$2 AND instrument_id='S1'`,
		room.ID, guest.ID).Scan(&gotShares); err != nil {
		t.Fatal(err)
	}
	if math.Abs(gotShares-wantShares) > 1e-9 {
		t.Fatalf("shares = %v, want %v", gotShares, wantShares)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1`, o.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "filled" {
		t.Fatalf("order status = %s, want filled", status)
	}
	var nTrades int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM trades WHERE room_id=$1`, room.ID).Scan(&nTrades); err != nil {
		t.Fatal(err)
	}
	if nTrades != 1 {
		t.Fatalf("trades = %d, want 1", nTrades)
	}

	// Settling again changes nothing (idempotence).
	if _, _, err := SettleRoom(ctx, pool, room, at); err != nil {
		t.Fatal(err)
	}
	var nTrades2 int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM trades WHERE room_id=$1`, room.ID).Scan(&nTrades2)
	if nTrades2 != 1 {
		t.Fatalf("trades after resettle = %d, want 1", nTrades2)
	}

	// Day 1: sell everything, executes at day 2 open.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, at, OrderReq{
		InstrumentID: "S1", Side: "sell", Shares: gotShares}); err != nil {
		t.Fatal(err)
	}
	at2 := t0.Add(121 * time.Second)
	if _, _, err := SettleRoom(ctx, pool, room, at2); err != nil {
		t.Fatal(err)
	}
	var open2 float64
	if err := pool.QueryRow(ctx, `
		SELECT open FROM room_prices WHERE room_id=$1 AND instrument_id='S1' AND day=2`,
		room.ID).Scan(&open2); err != nil {
		t.Fatal(err)
	}
	wantCash := 6_000_000 + int64(math.Round(gotShares*open2*100))
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != wantCash {
		t.Fatalf("cash after sell = %d, want %d", cash, wantCash)
	}
}

func TestWhaleEventThreshold(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// 90% of assets -> whale. 5% of assets -> silent.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 9_000_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S2", Side: "buy", AmountCents: 500_000}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}

	rows, err := pool.Query(ctx, `
		SELECT payload->>'instrument_id', payload->>'side'
		FROM room_events WHERE room_id=$1 AND kind='whale'`, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var whales [][2]string
	for rows.Next() {
		var inst, side string
		if err := rows.Scan(&inst, &side); err != nil {
			t.Fatal(err)
		}
		whales = append(whales, [2]string{inst, side})
	}
	if len(whales) != 1 || whales[0] != [2]string{"S1", "buy"} {
		t.Fatalf("whale events = %v, want exactly [S1 buy]", whales)
	}
}

func TestEndOfGameRefundsPendingOrders(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Place on the last day: exec_day == 300 which never comes.
	lastDay := t0.Add(299 * 60 * time.Second)
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, lastDay, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 2_000_000}); err != nil {
		t.Fatal(err)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != 8_000_000 {
		t.Fatalf("frozen cash = %d", cash)
	}

	// Past the end: settlement refunds instead of filling.
	day, ended, err := SettleRoom(ctx, pool, room, t0.Add(400*60*time.Second))
	if err != nil || day != 299 || !ended {
		t.Fatalf("SettleRoom at end: day=%d ended=%v err=%v", day, ended, err)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != InitialCashCents {
		t.Fatalf("cash after refund = %d, want %d", cash, InitialCashCents)
	}
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM orders WHERE room_id=$1 ORDER BY id DESC LIMIT 1`, room.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" {
		t.Fatalf("order status = %s, want cancelled", status)
	}
}

func TestPlaceOrderSettlesFirst(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Sell needs the shares a due-but-unsettled buy will deliver:
	// placement itself must settle first for this to succeed.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 4_000_000}); err != nil {
		t.Fatal(err)
	}
	at := t0.Add(61 * time.Second) // buy is now due but nothing has settled it yet
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, at, OrderReq{
		InstrumentID: "S1", Side: "sell", Shares: 1}); err != nil {
		t.Fatalf("sell after due buy: %v (PlaceOrder must run SettleTx first)", err)
	}
}

func TestSettleAccrualIdempotentAndSnapshots(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// The borrow itself settles day 0: a snapshot exists for both players.
	if _, err := Borrow(ctx, pool, room, guest.ID, 1_000_000, t0); err != nil {
		t.Fatalf("borrow: %v", err)
	}
	at3 := t0.Add(3*61*time.Second + time.Second) // day 3
	if _, _, err := SettleRoom(ctx, pool, room, at3); err != nil {
		t.Fatal(err)
	}
	debt1, _, _ := debtOf(t, pool, room.ID, guest.ID)
	if debt1 <= 1_000_000 {
		t.Fatalf("debt did not accrue: %d", debt1)
	}

	// Re-settling the same day is a no-op for debt and snapshots.
	if _, _, err := SettleRoom(ctx, pool, room, at3); err != nil {
		t.Fatal(err)
	}
	debt2, _, _ := debtOf(t, pool, room.ID, guest.ID)
	if debt2 != debt1 {
		t.Fatalf("accrual not idempotent: %d → %d", debt1, debt2)
	}

	// Snapshots: days 0 and 3 for each of the 2 players — 4 rows total,
	// exactly one row per (player, day), values stable across re-settle.
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM room_player_daily WHERE room_id = $1`, room.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("snapshot rows = %d, want 4", n)
	}
	var hostDay3 int64
	if err := pool.QueryRow(ctx, `
		SELECT total_cents FROM room_player_daily
		WHERE room_id = $1 AND user_id = $2 AND day = 3`,
		room.ID, room.HostUserID).Scan(&hostDay3); err != nil {
		t.Fatal(err)
	}
	if hostDay3 != InitialCashCents {
		t.Fatalf("host day-3 snapshot = %d, want %d", hostDay3, InitialCashCents)
	}
	var guestDay3 int64
	if err := pool.QueryRow(ctx, `
		SELECT total_cents FROM room_player_daily
		WHERE room_id = $1 AND user_id = $2 AND day = 3`,
		room.ID, guest.ID).Scan(&guestDay3); err != nil {
		t.Fatal(err)
	}
	// Net of debt: 10M initial + 1M borrowed cash − accrued debt.
	if want := InitialCashCents + 1_000_000 - debt1; guestDay3 != want {
		t.Fatalf("guest day-3 snapshot = %d, want %d", guestDay3, want)
	}

	// The never-borrowed host accrued nothing and has no interest clock.
	hostDebt, hostThrough, _ := debtOf(t, pool, room.ID, room.HostUserID)
	if hostDebt != 0 || hostThrough != nil {
		t.Fatalf("host: debt=%d through=%v, want 0/nil", hostDebt, hostThrough)
	}
}

func TestSettleTxExpiresOptionsAndSnapshots(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// A deep-ITM call expiring day 5; the payoff must land via plain
	// SettleRoom — no options-specific call — and show up in the snapshot.
	close5 := closeAt(t, pool, room.ID, "S1", 5)
	strike := math.Round(close5*50) / 100
	callID := insertOption(t, pool, room.ID, "S1", "call", strike, 5)
	fill, err := BuyOption(ctx, pool, room, guest.ID, callID, 1, t0)
	if err != nil {
		t.Fatal(err)
	}

	at5 := t0.Add(5*61*time.Second + time.Second)
	if _, _, err := SettleRoom(ctx, pool, room, at5); err != nil {
		t.Fatal(err)
	}
	payoffCents := int64(math.Round((close5 - strike) * 100))
	wantTotal := InitialCashCents - fill.AmountCents + payoffCents
	var snap int64
	if err := pool.QueryRow(ctx, `
		SELECT total_cents FROM room_player_daily
		WHERE room_id = $1 AND user_id = $2 AND day = 5`,
		room.ID, guest.ID).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	if snap != wantTotal {
		t.Fatalf("day-5 snapshot = %d, want %d (cash − premium + payoff)", snap, wantTotal)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != wantTotal {
		t.Fatalf("cash = %d, want %d", cash, wantTotal)
	}

	// Idempotent: re-settling keeps cash, snapshot, and expiry rows stable.
	if _, _, err := SettleRoom(ctx, pool, room, at5); err != nil {
		t.Fatal(err)
	}
	var snap2 int64
	if err := pool.QueryRow(ctx, `
		SELECT total_cents FROM room_player_daily
		WHERE room_id = $1 AND user_id = $2 AND day = 5`,
		room.ID, guest.ID).Scan(&snap2); err != nil {
		t.Fatal(err)
	}
	if snap2 != wantTotal || cashOf(t, pool, room.ID, guest.ID) != wantTotal {
		t.Fatalf("re-settle changed state: snap=%d cash=%d", snap2, cashOf(t, pool, room.ID, guest.ID))
	}
	var nExpiry int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM option_trades WHERE room_id = $1 AND action = 'expiry'`,
		room.ID).Scan(&nExpiry); err != nil {
		t.Fatal(err)
	}
	if nExpiry != 1 {
		t.Fatalf("expiry rows = %d, want 1", nExpiry)
	}
}
