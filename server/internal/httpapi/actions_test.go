package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/store"
)

// actionsFixture starts a room at day 0 and returns the host client, the
// room id, the invite code, and a clock-advance func.
func actionsFixture(t *testing.T, s *Server) (*client, int64, string, func(time.Duration)) {
	t.Helper()
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)
	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	return host, roomID, created["invite_code"].(string), advance
}

func roomSeed(t *testing.T, s *Server, roomID int64) uint64 {
	t.Helper()
	var seed int64
	if err := s.DB.QueryRow(context.Background(),
		`SELECT seed FROM rooms WHERE id = $1`, roomID).Scan(&seed); err != nil {
		t.Fatal(err)
	}
	return uint64(seed)
}

func priceCloseAt(t *testing.T, s *Server, roomID int64, inst string, day int) float64 {
	t.Helper()
	var close float64
	if err := s.DB.QueryRow(context.Background(), `
		SELECT close FROM room_prices
		WHERE room_id = $1 AND instrument_id = $2 AND day = $3`,
		roomID, inst, day).Scan(&close); err != nil {
		t.Fatal(err)
	}
	return close
}

func TestHypeFlowMovesTomorrowPrices(t *testing.T) {
	s := newServer(t)
	host, roomID, _, advance := actionsFixture(t, s)
	advance(5*60*time.Second + time.Second) // day 5

	path := fmt.Sprintf("/api/rooms/%d", roomID)
	beforePrices := host.mustJSON("GET", path+"/prices/S1", nil, http.StatusOK)["days"].([]any)
	futureCloseBefore := priceCloseAt(t, s, roomID, "S1", 6)

	res := host.mustJSON("POST", path+"/actions/hype",
		map[string]any{"instrument_id": "S1", "direction": "up", "tier": 2}, http.StatusOK)
	if res["fee_cents"].(float64) != float64(store.HypeTier2FeeCents) {
		t.Fatalf("fee_cents = %v", res["fee_cents"])
	}
	caught := res["caught"].(bool)
	wantCash := float64(store.InitialCashCents - store.HypeTier2FeeCents)
	if caught {
		// Fine takes cash first; any shortfall is debt (cash bottoms at 0).
		wantCash -= float64(min(3*store.HypeTier2FeeCents, int64(wantCash)))
	}
	if res["cash_cents"].(float64) != wantCash {
		t.Fatalf("cash_cents = %v, want %v (caught=%v)", res["cash_cents"], wantCash, caught)
	}

	// The planted story shows up in the public feed, blind-box safe.
	resp, body := host.do("GET", path+"/news?after=0", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("news: %d", resp.StatusCode)
	}
	if strings.Contains(string(body), "driven_by_user_id") {
		t.Fatalf("news leaks driven_by_user_id: %s", body[:min(400, len(body))])
	}
	news := host.mustJSON("GET", path+"/news?after=0", nil, http.StatusOK)
	var planted map[string]any
	for _, it := range news["items"].([]any) {
		m := it.(map[string]any)
		if _, ok := m["disputed"]; !ok {
			t.Fatalf("news item missing disputed flag: %v", m)
		}
		if _, ok := m["exposed"]; !ok {
			t.Fatalf("news item missing exposed flag: %v", m)
		}
		if strings.Contains(m["headline"].(string), "重磅利好正在酝酿") {
			planted = m
		}
	}
	if planted == nil {
		t.Fatal("planted hype story not in the news feed")
	}
	if planted["day"].(float64) != 5 || planted["media_id"].(string) != "tabloid" {
		t.Fatalf("planted item: %v", planted)
	}
	if planted["exposed"].(bool) != caught {
		t.Fatalf("exposed = %v, caught = %v", planted["exposed"], caught)
	}

	// Second hype the same day hits the per-day limit.
	resp, _ = host.do("POST", path+"/actions/hype",
		map[string]any{"instrument_id": "S2", "direction": "down", "tier": 1})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("second hype: %d", resp.StatusCode)
	}

	// Tomorrow: history identical, day-6 close lifted vs the pre-hype world.
	advance(60 * time.Second) // day 6
	after := host.mustJSON("GET", path+"/prices/S1", nil, http.StatusOK)["days"].([]any)
	if len(after) != len(beforePrices)+1 {
		t.Fatalf("price days = %d, want %d", len(after), len(beforePrices)+1)
	}
	for i := range beforePrices {
		if fmt.Sprint(after[i]) != fmt.Sprint(beforePrices[i]) {
			t.Fatalf("history day %d changed: %v → %v", i, beforePrices[i], after[i])
		}
	}
	day6 := after[6].(map[string]any)["close"].(float64)
	if day6 <= futureCloseBefore {
		t.Fatalf("day-6 close = %v, want > pre-hype %v", day6, futureCloseBefore)
	}
}

func TestHypeBustFlowsToEvents(t *testing.T) {
	s := newServer(t)
	host, roomID, invite, advance := actionsFixture(t, s)
	advance(3*60*time.Second + time.Second) // day 3
	seed := roomSeed(t, s, roomID)

	// Find a user whose first tier-3 hype is caught (deterministic stream).
	var busted *client
	var bustedName string
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("roller%d", i)
		c := registerClient(t, s, name)
		c.mustJSON("POST", "/api/rooms/join", map[string]any{"invite_code": invite}, http.StatusOK)
		var uid int64
		if err := s.DB.QueryRow(context.Background(),
			`SELECT id FROM users WHERE username = $1`, name).Scan(&uid); err != nil {
			t.Fatal(err)
		}
		if engine.Stream(seed, "manipulation", fmt.Sprint(uid), "0").Float64() < 0.30 {
			busted, bustedName = c, name
			break
		}
	}
	if busted == nil {
		t.Fatal("no caught user found")
	}

	path := fmt.Sprintf("/api/rooms/%d", roomID)
	res := busted.mustJSON("POST", path+"/actions/hype",
		map[string]any{"instrument_id": "S1", "direction": "down", "tier": 3}, http.StatusOK)
	if !res["caught"].(bool) {
		t.Fatalf("expected bust: %v", res)
	}
	if res["fine_cents"].(float64) != float64(3*store.HypeTier3FeeCents) {
		t.Fatalf("fine_cents = %v", res["fine_cents"])
	}

	events := host.mustJSON("GET", path+"/events?after=0", nil, http.StatusOK)
	var bust map[string]any
	for _, it := range events["items"].([]any) {
		m := it.(map[string]any)
		if m["kind"].(string) == "manipulation_bust" {
			bust = m
		}
	}
	if bust == nil {
		t.Fatal("manipulation_bust not in events feed")
	}
	payload := bust["payload"].(map[string]any)
	if payload["username"].(string) != bustedName || payload["instrument_id"].(string) != "S1" {
		t.Fatalf("bust payload: %v", payload)
	}

	// The busted story is publicly exposed.
	news := host.mustJSON("GET", path+"/news?after=0", nil, http.StatusOK)
	exposedFound := false
	for _, it := range news["items"].([]any) {
		m := it.(map[string]any)
		if strings.Contains(m["headline"].(string), "暗藏隐忧") {
			exposedFound = m["exposed"].(bool)
		}
	}
	if !exposedFound {
		t.Fatal("busted story not flagged exposed")
	}
}

func TestDebunkAndIntelFlows(t *testing.T) {
	s := newServer(t)
	host, roomID, _, advance := actionsFixture(t, s)
	advance(5*60*time.Second + time.Second) // day 5
	path := fmt.Sprintf("/api/rooms/%d", roomID)

	news := host.mustJSON("GET", path+"/news?after=0", nil, http.StatusOK)
	items := news["items"].([]any)
	if len(items) == 0 {
		t.Fatal("no news by day 5")
	}
	newsID := int64(items[0].(map[string]any)["id"].(float64))

	res := host.mustJSON("POST", path+"/actions/debunk",
		map[string]any{"news_id": newsID}, http.StatusOK)
	verdict := res["verdict"].(string)
	switch verdict {
	case "likely_true", "likely_false", "no_substance":
	default:
		t.Fatalf("verdict = %q (must carry no numbers)", verdict)
	}
	if res["fee_cents"].(float64) != float64(store.DebunkFeeCents) {
		t.Fatalf("fee_cents = %v", res["fee_cents"])
	}
	if res["cash_cents"].(float64) != float64(store.InitialCashCents-store.DebunkFeeCents) {
		t.Fatalf("cash_cents = %v", res["cash_cents"])
	}

	// The item is publicly disputed; a second debunk is refused.
	news = host.mustJSON("GET", path+"/news?after=0", nil, http.StatusOK)
	for _, it := range news["items"].([]any) {
		m := it.(map[string]any)
		if int64(m["id"].(float64)) == newsID && !m["disputed"].(bool) {
			t.Fatalf("item %d not disputed after debunk", newsID)
		}
	}
	resp, _ := host.do("POST", path+"/actions/debunk", map[string]any{"news_id": newsID})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("re-debunk: %d", resp.StatusCode)
	}
	resp, _ = host.do("POST", path+"/actions/debunk", map[string]any{"news_id": 999999})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("debunk missing news: %d", resp.StatusCode)
	}

	// Intel: shape + per-day limit.
	res = host.mustJSON("POST", path+"/actions/intel",
		map[string]any{"instrument_id": "S1"}, http.StatusOK)
	outlook := res["outlook"].(string)
	if outlook != "up" && outlook != "down" && outlook != "quiet" {
		t.Fatalf("outlook = %q", outlook)
	}
	if outlook == "quiet" && res["strength"] != nil {
		t.Fatalf("quiet outlook with strength %v", res["strength"])
	}
	if res["fee_cents"].(float64) != float64(store.IntelFeeCents) {
		t.Fatalf("fee_cents = %v", res["fee_cents"])
	}
	resp, _ = host.do("POST", path+"/actions/intel", map[string]any{"instrument_id": "S1"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("repeat intel: %d", resp.StatusCode)
	}
}

func TestActionGuardsOverHTTP(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)
	host := registerClient(t, s, "host")
	outsider := registerClient(t, s, "outsider")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	path := fmt.Sprintf("/api/rooms/%d", roomID)

	// Lobby: actions refused.
	resp, _ := host.do("POST", path+"/actions/hype",
		map[string]any{"instrument_id": "S1", "direction": "up", "tier": 1})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("lobby hype: %d", resp.StatusCode)
	}

	host.mustJSON("POST", path+"/start", nil, http.StatusOK)
	advance(time.Second)

	// Non-member locked out.
	resp, _ = outsider.do("POST", path+"/actions/hype",
		map[string]any{"instrument_id": "S1", "direction": "up", "tier": 1})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider hype: %d", resp.StatusCode)
	}

	// Bad tier / direction / instrument.
	for _, body := range []map[string]any{
		{"instrument_id": "S1", "direction": "up", "tier": 9},
		{"instrument_id": "S1", "direction": "flat", "tier": 1},
		{"instrument_id": "NOPE", "direction": "up", "tier": 1},
	} {
		resp, _ = host.do("POST", path+"/actions/hype", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("hype %v: %d", body, resp.StatusCode)
		}
	}
	resp, _ = host.do("POST", path+"/actions/intel", map[string]any{"instrument_id": "NOPE"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("intel unknown instrument: %d", resp.StatusCode)
	}
}
