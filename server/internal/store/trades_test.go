package store

import (
	"context"
	"testing"
	"time"
)

func TestTradesForUser(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 3_000_000}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}

	trades, err := TradesForUser(ctx, pool, room.ID, guest.ID)
	if err != nil {
		t.Fatalf("TradesForUser: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("trades: %+v", trades)
	}
	tr := trades[0]
	if tr.InstrumentID != "S1" || tr.Side != "buy" || tr.Day != 1 ||
		tr.AmountCents != 3_000_000 || tr.Shares <= 0 || tr.Price <= 0 {
		t.Fatalf("trade: %+v", tr)
	}

	// Other players' trades are not visible.
	host, err := GetUserByUsername(ctx, pool, "host")
	if err != nil {
		t.Fatal(err)
	}
	hostTrades, err := TradesForUser(ctx, pool, room.ID, host.ID)
	if err != nil || len(hostTrades) != 0 {
		t.Fatalf("host trades: %+v err=%v", hostTrades, err)
	}
}

func TestSetInstrumentDisplay(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	sc := mkScenario(t, pool)

	err := SetInstrumentDisplay(ctx, pool, sc.ID, map[string]InstrumentDisplay{
		"S1": {Alias: "郊狼网络", Desc: "网络设备巨头", Business: "路由器", Bull: "卖铲人", Bear: "客户烧钱"},
	})
	if err != nil {
		t.Fatalf("SetInstrumentDisplay: %v", err)
	}
	var alias, descr string
	var profile []byte
	if err := pool.QueryRow(ctx, `
		SELECT alias, descr, profile FROM instruments WHERE scenario_id=$1 AND id='S1'`,
		sc.ID).Scan(&alias, &descr, &profile); err != nil {
		t.Fatal(err)
	}
	if alias != "郊狼网络" || descr != "网络设备巨头" || len(profile) == 0 {
		t.Fatalf("display not applied: %s %s %s", alias, descr, profile)
	}

	// Unknown instrument id errors instead of silently no-oping.
	if err := SetInstrumentDisplay(ctx, pool, sc.ID, map[string]InstrumentDisplay{
		"NOPE": {Alias: "x"}}); err == nil {
		t.Fatal("unknown instrument should error")
	}
}
