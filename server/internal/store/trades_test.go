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
		"S1": {Alias: "Ridgeline Networks", Desc: "网络设备巨头",
			Aliases:  []string{"Ridgeline Networks", "Vantor Networks"},
			Business: "路由器", Bull: "卖铲人", Bear: "客户烧钱"},
	})
	if err != nil {
		t.Fatalf("SetInstrumentDisplay: %v", err)
	}
	var alias, descr string
	var profile []byte
	var aliases []string
	if err := pool.QueryRow(ctx, `
		SELECT alias, descr, profile, aliases FROM instruments WHERE scenario_id=$1 AND id='S1'`,
		sc.ID).Scan(&alias, &descr, &profile, &aliases); err != nil {
		t.Fatal(err)
	}
	if alias != "Ridgeline Networks" || descr != "网络设备巨头" || len(profile) == 0 {
		t.Fatalf("display not applied: %s %s %s", alias, descr, profile)
	}
	if len(aliases) != 2 || aliases[0] != "Ridgeline Networks" || aliases[1] != "Vantor Networks" {
		t.Fatalf("aliases not applied: %v", aliases)
	}

	// LoadScenario round-trips the candidate set (CreateRoom resolves the
	// per-room alias from it) and leaves it nil where none was recorded.
	loaded, err := LoadScenario(ctx, pool, sc.ID)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	if len(loaded.Instruments[0].Aliases) != 2 {
		t.Fatalf("LoadScenario aliases: %+v", loaded.Instruments[0].Aliases)
	}
	if loaded.Instruments[1].Aliases != nil {
		t.Fatalf("aliases should be nil when unset: %+v", loaded.Instruments[1].Aliases)
	}

	// Unknown instrument id errors instead of silently no-oping.
	if err := SetInstrumentDisplay(ctx, pool, sc.ID, map[string]InstrumentDisplay{
		"NOPE": {Alias: "x"}}); err == nil {
		t.Fatal("unknown instrument should error")
	}
}
