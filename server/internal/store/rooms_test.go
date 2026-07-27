package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

// mkUser / mkScenario are shared by later store tests too.
func mkUser(t *testing.T, pool *pgxpool.Pool, name string) *User {
	t.Helper()
	u, err := CreateUser(context.Background(), pool, name, "hash")
	if err != nil {
		t.Fatalf("mkUser(%s): %v", name, err)
	}
	return u
}

func mkScenario(t *testing.T, pool *pgxpool.Pool) *scenario.Scenario {
	t.Helper()
	sc := scenario.Synthetic()
	if err := SaveScenario(context.Background(), pool, sc); err != nil {
		t.Fatalf("mkScenario: %v", err)
	}
	return sc
}

func TestCreateRoomPersistsWorld(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)

	room, err := CreateRoom(ctx, pool, sc, host.ID, 3600, nil)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.Status != "lobby" || room.Days != sc.Days || room.InviteCode == "" {
		t.Fatalf("bad room: %+v", room)
	}

	var nPrices int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM room_prices WHERE room_id = $1`, room.ID).Scan(&nPrices); err != nil {
		t.Fatal(err)
	}
	if want := len(sc.Instruments) * sc.Days; nPrices != want {
		t.Fatalf("room_prices rows = %d, want %d", nPrices, want)
	}

	// The stored world must equal a regeneration from the stored seed —
	// proves the seed persisted is the seed used.
	world, err := engine.GenerateWorld(sc, room.Seed)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	var nNews int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM room_news WHERE room_id = $1`, room.ID).Scan(&nNews); err != nil {
		t.Fatal(err)
	}
	if nNews != len(world.News) {
		t.Fatalf("room_news rows = %d, want %d", nNews, len(world.News))
	}
	var gotClose float64
	if err := pool.QueryRow(ctx, `
		SELECT close FROM room_prices
		WHERE room_id = $1 AND instrument_id = 'S1' AND day = 299`, room.ID).Scan(&gotClose); err != nil {
		t.Fatal(err)
	}
	if want := world.Prices["S1"][299].Close; gotClose != want {
		t.Fatalf("S1 day299 close = %v, want %v", gotClose, want)
	}

	// Host is seated with initial cash.
	var cash int64
	if err := pool.QueryRow(ctx,
		`SELECT cash_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		room.ID, host.ID).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	if cash != InitialCashCents {
		t.Fatalf("host cash = %d, want %d", cash, InitialCashCents)
	}
}

func TestCreateRoomValidatesDayDuration(t *testing.T) {
	pool := TestDB(t, "store")
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	for _, secs := range []int{0, 59, 86401, -5} {
		if _, err := CreateRoom(context.Background(), pool, sc, host.ID, secs, nil); !errors.Is(err, ErrBadDayDuration) {
			t.Errorf("duration %d: got %v, want ErrBadDayDuration", secs, err)
		}
	}
}

func TestJoinStartAndClock(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	guest := mkUser(t, pool, "guest")
	late := mkUser(t, pool, "late")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Truncate(time.Second)

	// Clock before start.
	if _, _, err := room.CurrentDay(t0); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("CurrentDay before start: %v, want ErrNotStarted", err)
	}

	// Lobby join.
	day, err := JoinRoom(ctx, pool, room, guest.ID, t0)
	if err != nil || day != 0 {
		t.Fatalf("lobby join: day=%d err=%v", day, err)
	}
	if _, err := JoinRoom(ctx, pool, room, guest.ID, t0); !errors.Is(err, ErrAlreadyJoined) {
		t.Fatalf("double join: %v, want ErrAlreadyJoined", err)
	}

	// Only host can start.
	if _, err := StartRoom(ctx, pool, room.ID, guest.ID, t0); !errors.Is(err, ErrCannotStart) {
		t.Fatalf("guest start: %v, want ErrCannotStart", err)
	}
	room, err = StartRoom(ctx, pool, room.ID, host.ID, t0)
	if err != nil || room.Status != "running" || room.StartedAt == nil {
		t.Fatalf("host start: %+v, %v", room, err)
	}
	if _, err := StartRoom(ctx, pool, room.ID, host.ID, t0); !errors.Is(err, ErrCannotStart) {
		t.Fatalf("double start: %v, want ErrCannotStart", err)
	}

	// Deterministic clock (60s per day, 300 days).
	for _, tc := range []struct {
		at    time.Time
		day   int
		ended bool
	}{
		{t0.Add(-time.Hour), 0, false}, // clock skew clamps, never panics
		{t0, 0, false},
		{t0.Add(59 * time.Second), 0, false},
		{t0.Add(61 * time.Second), 1, false},
		{t0.Add(150 * 60 * time.Second), 150, false},
		{t0.Add(300 * 60 * time.Second), 299, true},
		{t0.Add(9999 * 60 * time.Second), 299, true},
	} {
		day, ended, err := room.CurrentDay(tc.at)
		if err != nil || day != tc.day || ended != tc.ended {
			t.Errorf("CurrentDay(%v) = (%d,%v,%v), want (%d,%v)", tc.at.Sub(t0), day, ended, err, tc.day, tc.ended)
		}
	}

	// Mid-game join stamps joined_day; post-game join refused.
	day, err = JoinRoom(ctx, pool, room, late.ID, t0.Add(5*60*time.Second))
	if err != nil || day != 5 {
		t.Fatalf("late join: day=%d err=%v", day, err)
	}
	extra := mkUser(t, pool, "extra")
	if _, err := JoinRoom(ctx, pool, room, extra.ID, t0.Add(400*60*time.Second)); !errors.Is(err, ErrRoomEnded) {
		t.Fatalf("post-game join: %v, want ErrRoomEnded", err)
	}

	// Lookup helpers.
	byInvite, err := GetRoomByInvite(ctx, pool, room.InviteCode)
	if err != nil || byInvite.ID != room.ID {
		t.Fatalf("GetRoomByInvite: %+v, %v", byInvite, err)
	}
	rooms, err := ListRoomsForUser(ctx, pool, guest.ID)
	if err != nil || len(rooms) != 1 || rooms[0].ID != room.ID {
		t.Fatalf("ListRoomsForUser: %+v, %v", rooms, err)
	}
	member, err := IsMember(ctx, pool, room.ID, guest.ID)
	if err != nil || !member {
		t.Fatalf("IsMember(guest): %v, %v", member, err)
	}
	member, err = IsMember(ctx, pool, room.ID, extra.ID)
	if err != nil || member {
		t.Fatalf("IsMember(extra): %v, %v", member, err)
	}
}

func scenarioMustLoad(t *testing.T, pool *pgxpool.Pool) *scenario.Scenario {
	t.Helper()
	sc, err := LoadScenario(context.Background(), pool, "synthetic-v1")
	if err != nil {
		t.Fatalf("scenarioMustLoad: %v", err)
	}
	return sc
}

type fakeFiller struct{ calls int }

func (f *fakeFiller) FillCopy(_ context.Context, _ *scenario.Scenario, evs []engine.NewsEvent) {
	f.calls++
	for i := range evs {
		evs[i].Headline = "AI标题"
		evs[i].Body = "AI正文。"
	}
}

func TestCreateRoomAppliesCopyFiller(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)

	filler := &fakeFiller{}
	room, err := CreateRoom(ctx, pool, sc, host.ID, 3600, filler)
	if err != nil {
		t.Fatal(err)
	}
	if filler.calls != 1 {
		t.Fatalf("filler calls: %d", filler.calls)
	}
	var headline, body string
	if err := pool.QueryRow(ctx, `
		SELECT headline, body FROM room_news WHERE room_id = $1 ORDER BY id LIMIT 1`,
		room.ID).Scan(&headline, &body); err != nil {
		t.Fatal(err)
	}
	if headline != "AI标题" || body != "AI正文。" {
		t.Fatalf("copy not persisted: %q %q", headline, body)
	}
	// Clusters persisted (synthetic worlds form clusters — engine Task 2).
	var clustered int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM room_news WHERE room_id = $1 AND cluster_id > 0`,
		room.ID).Scan(&clustered); err != nil {
		t.Fatal(err)
	}
	if clustered == 0 {
		t.Fatal("no clustered news persisted")
	}
}

func TestCreateRoomNilFillerKeepsTemplates(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 3600, nil)
	if err != nil {
		t.Fatal(err)
	}
	var headline, body string
	if err := pool.QueryRow(ctx, `
		SELECT headline, body FROM room_news WHERE room_id = $1 ORDER BY id LIMIT 1`,
		room.ID).Scan(&headline, &body); err != nil {
		t.Fatal(err)
	}
	if headline == "" || body != "" {
		t.Fatalf("template state wrong: %q %q", headline, body)
	}
}
