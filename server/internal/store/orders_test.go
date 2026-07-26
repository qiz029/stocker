package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// mkRunningRoom creates a started room with a host and one guest and
// returns (room, guest, start time). 60s per historical day.
func mkRunningRoom(t *testing.T, pool *pgxpool.Pool) (*Room, *User, time.Time) {
	t.Helper()
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	guest := mkUser(t, pool, "guest")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := JoinRoom(ctx, pool, room, guest.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Truncate(time.Second)
	room, err = StartRoom(ctx, pool, room.ID, host.ID, t0)
	if err != nil {
		t.Fatal(err)
	}
	return room, guest, t0
}

func cashOf(t *testing.T, pool *pgxpool.Pool, roomID, userID int64) int64 {
	t.Helper()
	var cash int64
	if err := pool.QueryRow(context.Background(),
		`SELECT cash_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		roomID, userID).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	return cash
}

func TestPlaceBuyFreezesCash(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	o, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 4_000_000})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if o.ExecDay != 1 || o.Status != "pending" {
		t.Fatalf("order: %+v", o)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != 6_000_000 {
		t.Fatalf("cash after freeze = %d, want 6000000", cash)
	}

	// Cannot overspend what is left.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 7_000_000}); !errors.Is(err, ErrInsufficientCash) {
		t.Fatalf("overspend: %v, want ErrInsufficientCash", err)
	}

	// Cancel refunds in full.
	if err := CancelOrder(ctx, pool, room, guest.ID, o.ID, t0); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != InitialCashCents {
		t.Fatalf("cash after cancel = %d, want %d", cash, InitialCashCents)
	}
	// A cancelled order cannot be cancelled again.
	if err := CancelOrder(ctx, pool, room, guest.ID, o.ID, t0); !errors.Is(err, ErrNotCancellable) {
		t.Fatalf("double cancel: %v, want ErrNotCancellable", err)
	}
}

func TestPlaceSellRequiresShares(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// No position yet.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "sell", Shares: 1}); !errors.Is(err, ErrInsufficientShares) {
		t.Fatalf("sell without position: %v, want ErrInsufficientShares", err)
	}

	// Seed a position directly, then a sell freezes it.
	if _, err := pool.Exec(ctx, `
		INSERT INTO positions (room_id, user_id, instrument_id, shares)
		VALUES ($1, $2, 'S1', 100)`, room.ID, guest.ID); err != nil {
		t.Fatal(err)
	}
	o, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "sell", Shares: 60})
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	var left float64
	if err := pool.QueryRow(ctx, `
		SELECT shares FROM positions WHERE room_id=$1 AND user_id=$2 AND instrument_id='S1'`,
		room.ID, guest.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 40 {
		t.Fatalf("frozen position = %v, want 40", left)
	}
	// Cancel restores the shares.
	if err := CancelOrder(ctx, pool, room, guest.ID, o.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT shares FROM positions WHERE room_id=$1 AND user_id=$2 AND instrument_id='S1'`,
		room.ID, guest.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 100 {
		t.Fatalf("restored position = %v, want 100", left)
	}
}

func TestPlaceOrderValidation(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	cases := []struct {
		req  OrderReq
		want error
	}{
		{OrderReq{InstrumentID: "NOPE", Side: "buy", AmountCents: 100}, ErrUnknownInstrument},
		{OrderReq{InstrumentID: "S1", Side: "hold", AmountCents: 100}, ErrBadOrder},
		{OrderReq{InstrumentID: "S1", Side: "buy", AmountCents: 0}, ErrBadOrder},
		{OrderReq{InstrumentID: "S1", Side: "buy", AmountCents: -5}, ErrBadOrder},
		{OrderReq{InstrumentID: "S1", Side: "sell", Shares: 0}, ErrBadOrder},
	}
	for _, tc := range cases {
		if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, tc.req); !errors.Is(err, tc.want) {
			t.Errorf("req %+v: got %v, want %v", tc.req, err, tc.want)
		}
	}

	// Orders on ended rooms are refused.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0.Add(400*60*time.Second), OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 100}); !errors.Is(err, ErrRoomEnded) {
		t.Errorf("ended room: %v, want ErrRoomEnded", err)
	}

	// Orders on lobby rooms are refused.
	host2 := mkUser(t, pool, "host2")
	sc := scenarioMustLoad(t, pool)
	lobby, err := CreateRoom(ctx, pool, sc, host2.ID, 60)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PlaceOrder(ctx, pool, lobby, host2.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 100}); !errors.Is(err, ErrRoomNotRunning) {
		t.Errorf("lobby room: %v, want ErrRoomNotRunning", err)
	}
}
