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
