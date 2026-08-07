package store

import (
	"context"
	"errors"
	"strings"
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

	room, err := CreateNamedRoom(ctx, pool, sc, host.ID, 3600, " Opening Bell ", nil)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.Name != "Opening Bell" || room.Status != "lobby" || room.Days != sc.Days || room.InviteCode == "" {
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

	// Every room starts with five system-controlled competitors in addition
	// to its human host. Agent identity is persisted, not inferred from names.
	var players, agents int
	if err := pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE u.is_agent)
		FROM room_players rp JOIN users u ON u.id = rp.user_id
		WHERE rp.room_id = $1`, room.ID).Scan(&players, &agents); err != nil {
		t.Fatal(err)
	}
	if players != 6 || agents != AgentPlayerCount {
		t.Fatalf("room players = %d (%d agents), want 6 (%d agents)",
			players, agents, AgentPlayerCount)
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

func TestCreateNamedRoomValidatesName(t *testing.T) {
	pool := TestDB(t, "store")
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	for _, name := range []string{"", " ", "x", "bad\nname", strings.Repeat("x", RoomNameMaxLength+1)} {
		if _, err := CreateNamedRoom(context.Background(), pool, sc, host.ID, 60, name, nil); !errors.Is(err, ErrBadRoomName) {
			t.Errorf("name %q: got %v, want ErrBadRoomName", name, err)
		}
	}
}

func TestEraLeaderboardUsesFinalSnapshotsAcrossEras(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

	makeCompletedRoom := func(scenarioID, hostName string) (*Room, *User) {
		t.Helper()
		sc := scenario.Synthetic()
		sc.ID = scenarioID
		if err := SaveScenario(ctx, pool, sc); err != nil {
			t.Fatal(err)
		}
		host := mkUser(t, pool, hostName)
		room, err := CreateRoom(ctx, pool, sc, host.ID, 60, nil, "public")
		if err != nil {
			t.Fatal(err)
		}
		startedAt := now.Add(-time.Duration(sc.Days+1) * time.Minute)
		if _, err := pool.Exec(ctx, `UPDATE rooms SET status = 'running', started_at = $2 WHERE id = $1`, room.ID, startedAt); err != nil {
			t.Fatal(err)
		}
		return room, host
	}

	roomA, hostA := makeCompletedRoom("era-a", "winner-a")
	roomB, hostB := makeCompletedRoom("era-b", "winner-b")
	for _, result := range []struct {
		room  *Room
		host  *User
		total int64
	}{{roomA, hostA, 12_000_000}, {roomB, hostB, 11_000_000}} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO room_player_daily (room_id, user_id, day, total_cents)
			VALUES ($1, $2, $3, $4)`, result.room.ID, result.host.ID, result.room.Days-1, result.total); err != nil {
			t.Fatal(err)
		}
	}

	// A large but non-final snapshot must not leak into the completed leaderboard.
	stale := mkUser(t, pool, "stale-player")
	if _, err := pool.Exec(ctx, `INSERT INTO room_players (room_id, user_id, cash_cents) VALUES ($1, $2, $3)`,
		roomA.ID, stale.ID, InitialCashCents); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO room_player_daily (room_id, user_id, day, total_cents) VALUES ($1, $2, $3, $4)`,
		roomA.ID, stale.ID, roomA.Days-2, 99_000_000); err != nil {
		t.Fatal(err)
	}

	rows, err := EraLeaderboard(ctx, pool, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("leaderboard rows = %+v, want one final result per era", rows)
	}
	got := map[string]string{}
	for _, row := range rows {
		got[row.ScenarioID] = row.Username
		if row.Username == stale.Username {
			t.Fatalf("stale snapshot entered leaderboard: %+v", row)
		}
	}
	if got["era-a"] != hostA.Username || got["era-b"] != hostB.Username {
		t.Fatalf("leaderboard by era = %v", got)
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
		evs[i].HeadlineEn = "AI headline"
		evs[i].BodyEn = "AI body."
	}
}

func TestCreateRoomQueuesCopyWithoutCallingFiller(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)

	filler := &fakeFiller{}
	room, err := CreateRoom(ctx, pool, sc, host.ID, 3600, filler)
	if err != nil {
		t.Fatal(err)
	}
	var headline, body, headlineEn, bodyEn string
	if err := pool.QueryRow(ctx, `
		SELECT headline, body, headline_en, body_en FROM room_news WHERE room_id = $1 ORDER BY id LIMIT 1`,
		room.ID).Scan(&headline, &body, &headlineEn, &bodyEn); err != nil {
		t.Fatal(err)
	}
	if filler.calls != 0 {
		t.Fatalf("room creation called filler %d times; want async queue only", filler.calls)
	}
	if headline == "" || body != "" || headlineEn == "" || bodyEn != "" {
		t.Fatalf("template state wrong: zh=%q/%q en=%q/%q", headline, body, headlineEn, bodyEn)
	}
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM room_copy_jobs WHERE room_id = $1`, room.ID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != sc.Days {
		t.Fatalf("copy jobs = %d, want %d", jobs, sc.Days)
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
