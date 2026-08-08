package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

var actionsT0 = time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

// mkRunningRoom creates a room (60s days) and starts it at actionsT0; the
// returned clock function yields `now` for any sim day.
func mkActionRoom(t *testing.T, pool *pgxpool.Pool, sc *scenario.Scenario, hostID int64) (*Room, func(day int) time.Time) {
	t.Helper()
	ctx := context.Background()
	room, err := CreateRoom(ctx, pool, sc, hostID, 60, nil)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	room, err = StartRoom(ctx, pool, room.ID, hostID, actionsT0)
	if err != nil {
		t.Fatalf("StartRoom: %v", err)
	}
	return room, func(day int) time.Time {
		return actionsT0.Add(time.Duration(day)*60*time.Second + time.Second)
	}
}

// allPrices snapshots room_prices as (instrument, day) → OHLC.
func allPrices(t *testing.T, pool *pgxpool.Pool, roomID int64) map[string]scenario.OHLC {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT instrument_id, day, open, high, low, close FROM room_prices
		WHERE room_id = $1`, roomID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]scenario.OHLC{}
	for rows.Next() {
		var inst string
		var d int
		var p scenario.OHLC
		if err := rows.Scan(&inst, &d, &p.Open, &p.High, &p.Low, &p.Close); err != nil {
			t.Fatal(err)
		}
		out[fmt.Sprintf("%s/%d", inst, d)] = p
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestResynthesizeIdenticalWithoutNewShock(t *testing.T) {
	pool := TestDB(t, "store")
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, _ := mkActionRoom(t, pool, sc, host.ID)

	before := allPrices(t, pool, room.ID)
	if err := ResynthesizePrices(context.Background(), pool, room, sc, 0); err != nil {
		t.Fatalf("ResynthesizePrices: %v", err)
	}
	after := allPrices(t, pool, room.ID)
	if len(before) != len(after) {
		t.Fatalf("row count changed: %d → %d", len(before), len(after))
	}
	for k, p := range before {
		if after[k] != p {
			t.Fatalf("%s changed without new shock: %+v → %+v", k, p, after[k])
		}
	}
}

func TestResynthesizeConcurrentSerialized(t *testing.T) {
	pool := TestDB(t, "store")
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, _ := mkActionRoom(t, pool, sc, host.ID)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ResynthesizePrices(context.Background(), pool, room, sc, 0); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent resynth: %v", err)
	}

	// The result must equal a single deterministic recompute.
	world, err := engine.GenerateWorld(sc, room.Seed)
	if err != nil {
		t.Fatalf("GenerateWorld: %v", err)
	}
	got := allPrices(t, pool, room.ID)
	for _, inst := range sc.Instruments {
		for d := 0; d < sc.Days; d++ {
			if want := world.Prices[inst.ID][d]; got[fmt.Sprintf("%s/%d", inst.ID, d)] != want {
				t.Fatalf("%s day %d = %+v, want %+v", inst.ID, d, got[fmt.Sprintf("%s/%d", inst.ID, d)], want)
			}
		}
	}
}

func TestHypePlantsRealShock(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, at := mkActionRoom(t, pool, sc, host.ID)
	const day = 5

	before := allPrices(t, pool, room.ID)
	var forumBefore int64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(max(id), 0) FROM room_forum_posts WHERE room_id = $1`,
		room.ID).Scan(&forumBefore); err != nil {
		t.Fatal(err)
	}
	res, err := Hype(ctx, pool, room, sc, host.ID, at(day), "S1", "up", 2)
	if err != nil {
		t.Fatalf("Hype: %v", err)
	}
	if res.FeeCents != HypeTier2FeeCents {
		t.Fatalf("fee = %d, want %d", res.FeeCents, HypeTier2FeeCents)
	}
	wantCash := InitialCashCents - HypeTier2FeeCents
	if res.Caught {
		wantCash -= min(3*HypeTier2FeeCents, wantCash+MaxDebtCents)
	}
	if res.CashCents != wantCash {
		t.Fatalf("cash = %d, want %d", res.CashCents, wantCash)
	}

	// The planted story: impact track, tabloid, true shock of exactly the
	// tier magnitude on IDIO:S1, driven by the host (server-private).
	var mediaID, track, headline, headlineEn, bodyEn string
	var shock map[string]float64
	var drivenBy *int64
	var exposed bool
	if err := pool.QueryRow(ctx, `
		SELECT media_id, track, headline, true_shock, driven_by_user_id, exposed, headline_en, body_en
		FROM room_news WHERE room_id = $1 AND day = $2 AND driven_by_user_id IS NOT NULL`,
		room.ID, day).Scan(&mediaID, &track, &headline, &shock, &drivenBy, &exposed, &headlineEn, &bodyEn); err != nil {
		t.Fatalf("planted news row: %v", err)
	}
	if mediaID != "tabloid" || track != "impact" {
		t.Fatalf("media/track = %s/%s", mediaID, track)
	}
	if headlineEn == "" || bodyEn == "" {
		t.Fatalf("planted story missing English copy: headline_en=%q body_en=%q", headlineEn, bodyEn)
	}
	if got := shock["IDIO:S1"]; got != HypeTier2Shock {
		t.Fatalf("true_shock = %v, want %v", got, HypeTier2Shock)
	}
	if drivenBy == nil || *drivenBy != host.ID {
		t.Fatalf("driven_by_user_id = %v, want %d", drivenBy, host.ID)
	}
	if exposed != res.Caught {
		t.Fatalf("exposed = %v, caught = %v", exposed, res.Caught)
	}

	// Prices: history (≤ day) untouched, future moved only for S1, up.
	after := allPrices(t, pool, room.ID)
	for _, inst := range sc.Instruments {
		for d := 0; d < sc.Days; d++ {
			k := fmt.Sprintf("%s/%d", inst.ID, d)
			oldP, newP := before[k], after[k]
			switch {
			case d <= day:
				if newP != oldP {
					t.Fatalf("history rewritten at %s: %+v → %+v", k, oldP, newP)
				}
			case inst.ID != "S1":
				if newP != oldP {
					t.Fatalf("bystander %s moved: %+v → %+v", k, oldP, newP)
				}
			default:
				if newP.Close <= oldP.Close {
					t.Fatalf("S1 day %d did not move up: %v → %v", d, oldP.Close, newP.Close)
				}
			}
		}
	}
	// First affected day carries the full shock: ×exp(0.03) (±0.5% slack).
	ratio := after["S1/6"].Close / before["S1/6"].Close
	if want := math.Exp(HypeTier2Shock); math.Abs(ratio/want-1) > 0.005 {
		t.Fatalf("S1 day6 ratio = %v, want ≈%v", ratio, want)
	}

	// NPC forum follow-ups: 1-3 posts on the action day.
	var nForum int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM room_forum_posts
		WHERE room_id = $1 AND day = $2 AND id > $3`,
		room.ID, day, forumBefore).Scan(&nForum); err != nil {
		t.Fatal(err)
	}
	if nForum < 1 || nForum > 3 {
		t.Fatalf("forum follow-ups = %d, want 1-3", nForum)
	}

	// Action recorded with the fee.
	var fee int64
	var payload map[string]any
	if err := pool.QueryRow(ctx, `
		SELECT fee_cents, payload FROM player_actions
		WHERE room_id = $1 AND user_id = $2 AND kind = 'hype'`,
		room.ID, host.ID).Scan(&fee, &payload); err != nil {
		t.Fatalf("player_actions: %v", err)
	}
	if fee != HypeTier2FeeCents || payload["instrument_id"] != "S1" || payload["direction"] != "up" {
		t.Fatalf("action row: fee=%d payload=%v", fee, payload)
	}
}

func TestHypeDiminishingReturns(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, at := mkActionRoom(t, pool, sc, host.ID)

	if _, err := Hype(ctx, pool, room, sc, host.ID, at(5), "S1", "up", 1); err != nil {
		t.Fatalf("hype 1: %v", err)
	}
	if _, err := Hype(ctx, pool, room, sc, host.ID, at(6), "S1", "up", 1); err != nil {
		t.Fatalf("hype 2: %v", err)
	}
	rows, err := pool.Query(ctx, `
		SELECT (true_shock ->> 'IDIO:S1')::float8 FROM room_news
		WHERE room_id = $1 AND driven_by_user_id IS NOT NULL ORDER BY id`, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var shocks []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		shocks = append(shocks, v)
	}
	if len(shocks) != 2 {
		t.Fatalf("planted shocks = %v, want 2", shocks)
	}
	if shocks[0] != HypeTier1Shock {
		t.Fatalf("first shock = %v, want %v", shocks[0], HypeTier1Shock)
	}
	if want := HypeTier1Shock * HypeDiminishFactor; math.Abs(shocks[1]/want-1) > 1e-9 {
		t.Fatalf("second shock = %v, want %v", shocks[1], want)
	}
}

func TestHypeGuards(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, at := mkActionRoom(t, pool, sc, host.ID)

	// Bad tier / direction / instrument.
	if _, err := Hype(ctx, pool, room, sc, host.ID, at(1), "S1", "up", 4); !errors.Is(err, ErrBadAction) {
		t.Fatalf("tier 4: %v", err)
	}
	if _, err := Hype(ctx, pool, room, sc, host.ID, at(1), "S1", "sideways", 1); !errors.Is(err, ErrBadAction) {
		t.Fatalf("bad direction: %v", err)
	}
	if _, err := Hype(ctx, pool, room, sc, host.ID, at(1), "NOPE", "up", 1); !errors.Is(err, ErrUnknownInstrument) {
		t.Fatalf("unknown instrument: %v", err)
	}

	// Lobby room.
	lobby, err := CreateRoom(ctx, pool, sc, host.ID, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Hype(ctx, pool, lobby, sc, host.ID, at(1), "S1", "up", 1); !errors.Is(err, ErrRoomNotRunning) {
		t.Fatalf("lobby: %v", err)
	}

	// Ended room.
	if _, err := Hype(ctx, pool, room, sc, host.ID, at(sc.Days+1), "S1", "up", 1); !errors.Is(err, ErrRoomEnded) {
		t.Fatalf("ended: %v", err)
	}

	// Insufficient cash.
	poor := mkUser(t, pool, "poor")
	if _, err := JoinRoom(ctx, pool, room, poor.ID, at(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE room_players SET cash_cents = 1 WHERE room_id = $1 AND user_id = $2`,
		room.ID, poor.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Hype(ctx, pool, room, sc, poor.ID, at(1), "S1", "up", 1); !errors.Is(err, ErrInsufficientCash) {
		t.Fatalf("poor: %v", err)
	}

	// Bankrupt.
	if _, err := pool.Exec(ctx, `
		UPDATE room_players SET bankrupt_day = 1 WHERE room_id = $1 AND user_id = $2`,
		room.ID, poor.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Hype(ctx, pool, room, sc, poor.ID, at(1), "S1", "up", 1); !errors.Is(err, ErrPlayerBankrupt) {
		t.Fatalf("bankrupt: %v", err)
	}

	// Daily limit: one hype per player per day.
	if _, err := Hype(ctx, pool, room, sc, host.ID, at(2), "S1", "up", 1); err != nil {
		t.Fatalf("first hype: %v", err)
	}
	if _, err := Hype(ctx, pool, room, sc, host.ID, at(2), "S2", "down", 1); !errors.Is(err, ErrActionLimit) {
		t.Fatalf("second hype same day: %v", err)
	}
}

// caughtUser returns a fresh joined user whose first tier-3 hype is
// regulator-caught (per the deterministic manipulation stream), plus one
// whose isn't.
func caughtUsers(t *testing.T, pool *pgxpool.Pool, room *Room, at func(int) time.Time, prefix string) (caught, clean *User) {
	t.Helper()
	for i := 0; i < 100 && (caught == nil || clean == nil); i++ {
		u := mkUser(t, pool, fmt.Sprintf("%s%d", prefix, i))
		if _, err := JoinRoom(context.Background(), pool, room, u.ID, at(1)); err != nil {
			t.Fatal(err)
		}
		isCaught := engine.Stream(room.Seed, "manipulation", fmt.Sprint(u.ID), "0").Float64() < 0.30
		if isCaught && caught == nil {
			caught = u
		}
		if !isCaught && clean == nil {
			clean = u
		}
	}
	if caught == nil || clean == nil {
		t.Fatal("no caught/clean user found in 100 draws")
	}
	return caught, clean
}

func TestHypeBustFineEventExposed(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, at := mkActionRoom(t, pool, sc, host.ID)
	caught, clean := caughtUsers(t, pool, room, at, "roller")
	setTestAlias(t, pool, caught, "Caught Trader")

	// Determinism: the roll matches the engine stream exactly.
	res, err := Hype(ctx, pool, room, sc, clean.ID, at(3), "S1", "up", 3)
	if err != nil {
		t.Fatalf("clean hype: %v", err)
	}
	if res.Caught || res.FineCents != 0 {
		t.Fatalf("clean user busted: %+v", res)
	}

	res, err = Hype(ctx, pool, room, sc, caught.ID, at(3), "S1", "down", 3)
	if err != nil {
		t.Fatalf("caught hype: %v", err)
	}
	if !res.Caught {
		t.Fatal("caught user walked")
	}
	wantFine := int64(3 * HypeTier3FeeCents)
	if res.FineCents != wantFine {
		t.Fatalf("fine = %d, want %d", res.FineCents, wantFine)
	}
	// Cash after the fee (6M¢) can't cover the 12M¢ fine: cash goes to
	// zero and the 6M¢ shortfall becomes debt.
	if res.CashCents != 0 {
		t.Fatalf("cash = %d, want 0 (shortfall goes to debt)", res.CashCents)
	}
	var caughtDebt int64
	if err := pool.QueryRow(ctx, `
		SELECT debt_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		room.ID, caught.ID).Scan(&caughtDebt); err != nil {
		t.Fatal(err)
	}
	if wantDebt := wantFine - (InitialCashCents - HypeTier3FeeCents); caughtDebt != wantDebt {
		t.Fatalf("debt = %d, want %d", caughtDebt, wantDebt)
	}

	// Public bust event.
	var payload map[string]any
	if err := pool.QueryRow(ctx, `
		SELECT payload FROM room_events
		WHERE room_id = $1 AND kind = 'manipulation_bust'`, room.ID).Scan(&payload); err != nil {
		t.Fatalf("bust event: %v", err)
	}
	if payload["username"] != caught.DisplayName ||
		int64(payload["fine_cents"].(float64)) != wantFine ||
		payload["instrument_id"] != "S1" ||
		int(payload["day"].(float64)) != 3 {
		t.Fatalf("bust payload: %v", payload)
	}

	// The caught user's planted story is exposed; the clean user's isn't.
	exposedOf := func(userID int64) bool {
		var exposed bool
		if err := pool.QueryRow(ctx, `
			SELECT exposed FROM room_news
			WHERE room_id = $1 AND driven_by_user_id = $2`,
			room.ID, userID).Scan(&exposed); err != nil {
			t.Fatal(err)
		}
		return exposed
	}
	if !exposedOf(caught.ID) || exposedOf(clean.ID) {
		t.Fatalf("exposed flags wrong: caught=%v clean=%v", exposedOf(caught.ID), exposedOf(clean.ID))
	}
}

func TestHypeFineUnderLowCash(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, at := mkActionRoom(t, pool, sc, host.ID)
	caught, _ := caughtUsers(t, pool, room, at, "lowcash")

	// Cash exactly covers the fee: the whole fine becomes debt.
	if _, err := pool.Exec(ctx, `
		UPDATE room_players SET cash_cents = $3 WHERE room_id = $1 AND user_id = $2`,
		room.ID, caught.ID, HypeTier3FeeCents); err != nil {
		t.Fatal(err)
	}
	res, err := Hype(ctx, pool, room, sc, caught.ID, at(3), "S1", "up", 3)
	if err != nil {
		t.Fatalf("hype: %v", err)
	}
	wantFine := int64(3 * HypeTier3FeeCents)
	if !res.Caught || res.FineCents != wantFine || res.CashCents != 0 {
		t.Fatalf("result: %+v, want caught with fine %d and cash 0", res, wantFine)
	}
	var debt int64
	if err := pool.QueryRow(ctx, `
		SELECT debt_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		room.ID, caught.ID).Scan(&debt); err != nil {
		t.Fatal(err)
	}
	if debt != wantFine {
		t.Fatalf("debt = %d, want %d (fine shortfall)", debt, wantFine)
	}

	// Near the debt cap: the fine is capped by the remaining headroom.
	caught2, _ := caughtUsers(t, pool, room, at, "capped")
	if _, err := pool.Exec(ctx, `
		UPDATE room_players SET cash_cents = $3, debt_cents = $4, interest_through_day = 3
		WHERE room_id = $1 AND user_id = $2`,
		room.ID, caught2.ID, HypeTier3FeeCents, MaxDebtCents-100); err != nil {
		t.Fatal(err)
	}
	res, err = Hype(ctx, pool, room, sc, caught2.ID, at(3), "S1", "up", 3)
	if err != nil {
		t.Fatalf("hype 2: %v", err)
	}
	if res.FineCents != 100 || res.CashCents != 0 {
		t.Fatalf("capped fine: %+v, want fine 100 cash 0", res)
	}
	if err := pool.QueryRow(ctx, `
		SELECT debt_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		room.ID, caught2.ID).Scan(&debt); err != nil {
		t.Fatal(err)
	}
	if debt != MaxDebtCents {
		t.Fatalf("debt = %d, want cap %d", debt, MaxDebtCents)
	}
}

func TestDebunkVerdictsAndGuards(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, at := mkActionRoom(t, pool, sc, host.ID)
	const day = 10

	// Debunk every published item; count fidelity against the truth.
	type item struct {
		id          int64
		trueShock   map[string]float64
		reportShock map[string]float64
	}
	rows, err := pool.Query(ctx, `
		SELECT id, true_shock, report_shock FROM room_news
		WHERE room_id = $1 AND day <= $2 ORDER BY id`, room.ID, day)
	if err != nil {
		t.Fatal(err)
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.trueShock, &it.reportShock); err != nil {
			t.Fatal(err)
		}
		items = append(items, it)
	}
	rows.Close()
	if len(items) == 0 {
		t.Fatal("no published news by day 10")
	}

	truthful, checked, noSubstance := 0, 0, 0
	for _, it := range items {
		if checked+noSubstance >= 40 {
			break // host cash covers 50 debunks; keep headroom
		}
		res, err := Debunk(ctx, pool, room, host.ID, at(day), it.id)
		if err != nil {
			t.Fatalf("debunk %d: %v", it.id, err)
		}
		if res.FeeCents != DebunkFeeCents {
			t.Fatalf("fee = %d, want %d", res.FeeCents, DebunkFeeCents)
		}
		// Second debunk of the same item is refused.
		if _, err := Debunk(ctx, pool, room, host.ID, at(day), it.id); !errors.Is(err, ErrAlreadyDisputed) {
			t.Fatalf("re-debunk %d: %v", it.id, err)
		}
		if len(it.trueShock) == 0 {
			if res.Verdict != "no_substance" {
				t.Fatalf("item %d verdict = %s, want no_substance", it.id, res.Verdict)
			}
			noSubstance++
			continue
		}
		trueDir := sign(dominant(it.trueShock))
		repDir := trueDir
		if len(it.reportShock) > 0 {
			repDir = sign(dominant(it.reportShock))
		}
		base := "likely_true"
		if trueDir != repDir {
			base = "likely_false"
		}
		if res.Verdict != "likely_true" && res.Verdict != "likely_false" {
			t.Fatalf("item %d verdict = %s", it.id, res.Verdict)
		}
		if res.Verdict == base {
			truthful++
		}
		checked++
	}
	if checked == 0 || noSubstance == 0 {
		t.Fatalf("fixture lacks impact/noise items: checked=%d noSubstance=%d", checked, noSubstance)
	}
	// 85% fidelity: loose band to avoid flake (checked ≈ 30-60 items).
	if got := float64(truthful) / float64(checked); got < 0.65 || got > 1.0 {
		t.Fatalf("verdict fidelity = %v (%d/%d), want ≈0.85", got, truthful, checked)
	}

	// All debunked items are publicly disputed.
	var nDisputed int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM room_news WHERE room_id = $1 AND disputed`,
		room.ID).Scan(&nDisputed); err != nil {
		t.Fatal(err)
	}
	if nDisputed != checked+noSubstance {
		t.Fatalf("disputed = %d, want %d", nDisputed, checked+noSubstance)
	}

	// Unpublished and foreign items don't exist for players.
	var futureID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM room_news WHERE room_id = $1 AND day > $2 LIMIT 1`,
		room.ID, day).Scan(&futureID); err != nil {
		t.Fatal(err)
	}
	if _, err := Debunk(ctx, pool, room, host.ID, at(day), futureID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("future news: %v", err)
	}
	if _, err := Debunk(ctx, pool, room, host.ID, at(day), 999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing news: %v", err)
	}
}

// intelRoll reports whether the intel stream corrupts this lookup.
func intelRoll(room *Room, userID int64, instrumentID string, day int) float64 {
	return engine.Stream(room.Seed, "intel",
		fmt.Sprint(userID), instrumentID, fmt.Sprint(day)).Float64()
}

func TestIntelPaths(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, at := mkActionRoom(t, pool, sc, host.ID)

	// Find a (day, instrument) with a true idio shock, and a quiet one.
	type evt struct {
		day   int
		inst  string
		shock float64
	}
	var withEvent, quietDay *evt
	for d := 0; d < 60 && (withEvent == nil || quietDay == nil); d++ {
		found := false
		for _, inst := range sc.Instruments {
			var v float64
			err := pool.QueryRow(ctx, `
				SELECT (true_shock ->> $3)::float8 FROM room_news
				WHERE room_id = $1 AND day = $2 AND track = 'impact' AND true_shock ? $3
				ORDER BY abs((true_shock ->> $3)::float8) DESC LIMIT 1`,
				room.ID, d, "IDIO:"+inst.ID).Scan(&v)
			if err == nil && !found {
				found = true
				if withEvent == nil {
					withEvent = &evt{day: d, inst: inst.ID, shock: v}
				}
			}
		}
		if !found && quietDay == nil {
			quietDay = &evt{day: d, inst: "S1"}
		}
	}
	if withEvent == nil || quietDay == nil {
		t.Fatalf("fixture lacks event/quiet days: %+v %+v", withEvent, quietDay)
	}

	// Uncorrupted event lookup: true direction, correct bucket.
	res, err := Intel(ctx, pool, room, sc, host.ID, at(withEvent.day), withEvent.inst)
	if err != nil {
		t.Fatalf("intel: %v", err)
	}
	corrupted := intelRoll(room, host.ID, withEvent.inst, withEvent.day) < intelNoiseProb
	if !corrupted {
		wantOutlook := "up"
		if withEvent.shock < 0 {
			wantOutlook = "down"
		}
		if res.Outlook != wantOutlook || res.Strength != bucketStrength(math.Abs(withEvent.shock)) {
			t.Fatalf("outlook = %s/%s, want %s/%s",
				res.Outlook, res.Strength, wantOutlook, bucketStrength(math.Abs(withEvent.shock)))
		}
	} else if res.Outlook != "quiet" &&
		!(withEvent.shock > 0 && res.Outlook == "down") &&
		!(withEvent.shock < 0 && res.Outlook == "up") {
		t.Fatalf("corrupted tip must be quiet or flipped, got %s", res.Outlook)
	}
	if res.FeeCents != IntelFeeCents {
		t.Fatalf("fee = %d, want %d", res.FeeCents, IntelFeeCents)
	}

	// Per-player per-instrument per-day limit; another instrument is fine.
	if _, err := Intel(ctx, pool, room, sc, host.ID, at(withEvent.day), withEvent.inst); !errors.Is(err, ErrActionLimit) {
		t.Fatalf("repeat intel: %v", err)
	}
	other := "S2"
	if withEvent.inst == "S2" {
		other = "S3"
	}
	if _, err := Intel(ctx, pool, room, sc, host.ID, at(withEvent.day), other); err != nil {
		t.Fatalf("other instrument same day: %v", err)
	}

	// Quiet day, uncorrupted: quiet with null strength.
	quietUser := host
	for i := 0; intelRoll(room, quietUser.ID, quietDay.inst, quietDay.day) < intelNoiseProb; i++ {
		quietUser = mkUser(t, pool, fmt.Sprintf("quiet%d", i))
		if _, err := JoinRoom(ctx, pool, room, quietUser.ID, at(1)); err != nil {
			t.Fatal(err)
		}
	}
	res, err = Intel(ctx, pool, room, sc, quietUser.ID, at(quietDay.day), quietDay.inst)
	if err != nil {
		t.Fatalf("quiet intel: %v", err)
	}
	if res.Outlook != "quiet" || res.Strength != "" {
		t.Fatalf("quiet day = %s/%s, want quiet/''", res.Outlook, res.Strength)
	}

	// Quiet day, corrupted: a tip is fabricated.
	fabUser := host
	for i := 0; intelRoll(room, fabUser.ID, quietDay.inst, quietDay.day) >= intelNoiseProb; i++ {
		fabUser = mkUser(t, pool, fmt.Sprintf("fab%d", i))
		if _, err := JoinRoom(ctx, pool, room, fabUser.ID, at(1)); err != nil {
			t.Fatal(err)
		}
	}
	res, err = Intel(ctx, pool, room, sc, fabUser.ID, at(quietDay.day), quietDay.inst)
	if err != nil {
		t.Fatalf("fabricated intel: %v", err)
	}
	if res.Outlook == "quiet" || res.Strength == "" {
		t.Fatalf("fabricated tip = %s/%s, want a direction and strength", res.Outlook, res.Strength)
	}
}

func TestIntelNoiseDistribution(t *testing.T) {
	// The corruption draw itself: ~25% over many independent streams.
	const n = 4000
	corrupt := 0
	for i := 0; i < n; i++ {
		if engine.Stream(42, "intel", fmt.Sprint(i), "S1", "3").Float64() < intelNoiseProb {
			corrupt++
		}
	}
	if got := float64(corrupt) / n; got < 0.22 || got > 0.28 {
		t.Fatalf("corruption rate = %v, want ≈0.25", got)
	}
}
