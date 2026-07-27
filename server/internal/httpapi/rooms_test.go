package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/toddzheng/stocker/server/internal/scenario"
	"github.com/toddzheng/stocker/server/internal/store"
)

func seedScenario(t *testing.T, s *Server) *scenario.Scenario {
	t.Helper()
	sc := scenario.Synthetic()
	if err := store.SaveScenario(context.Background(), s.DB, sc); err != nil {
		t.Fatalf("seed scenario: %v", err)
	}
	return sc
}

// fakeClock pins the server's clock and returns a function to move it.
func fakeClock(s *Server, start time.Time) func(d time.Duration) {
	now := start
	s.Now = func() time.Time { return now }
	return func(d time.Duration) { now = now.Add(d) }
}

func TestRoomLifecycleAndState(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	guest := registerClient(t, s, "guest")
	outsider := registerClient(t, s, "outsider")

	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	invite := created["invite_code"].(string)
	if created["status"] != "lobby" || invite == "" {
		t.Fatalf("created room: %v", created)
	}

	// Bad scenario / bad duration.
	resp, _ := host.do("POST", "/api/rooms", map[string]any{"scenario_id": "nope", "day_duration_secs": 60})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown scenario: %d", resp.StatusCode)
	}
	resp, _ = host.do("POST", "/api/rooms", map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 5})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad duration: %d", resp.StatusCode)
	}

	// Join by invite; non-members are locked out of room reads.
	guest.mustJSON("POST", "/api/rooms/join", map[string]any{"invite_code": invite}, http.StatusOK)
	resp, _ = outsider.do("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider room read: %d", resp.StatusCode)
	}

	// Guest cannot start; host can.
	resp, _ = guest.do("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("guest start: %d", resp.StatusCode)
	}
	started := host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	if started["status"] != "running" {
		t.Fatalf("started room: %v", started)
	}

	// Advance two historical days; state reflects the deterministic clock.
	advance(2*60*time.Second + time.Second)
	state := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil, http.StatusOK)
	room := state["room"].(map[string]any)
	if room["current_day"].(float64) != 2 || room["ended"].(bool) {
		t.Fatalf("room state: %v", room)
	}
	instruments := state["instruments"].([]any)
	if len(instruments) != 8 {
		t.Fatalf("instruments: %d, want 8", len(instruments))
	}
	quotes := state["quotes"].([]any)
	if len(quotes) != 8 {
		t.Fatalf("quotes: %d, want 8", len(quotes))
	}
	lb := state["leaderboard"].([]any)
	if len(lb) != 2 {
		t.Fatalf("leaderboard: %v", lb)
	}
	row := lb[0].(map[string]any)
	if _, hasUID := row["user_id"]; hasUID {
		t.Fatalf("leaderboard leaks user_id: %v", row)
	}

	// Price series is truncated at the current day (no future peeking).
	prices := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/prices/S1", roomID), nil, http.StatusOK)
	if n := len(prices["days"].([]any)); n != 3 {
		t.Fatalf("price days = %d, want 3 (day 0..2)", n)
	}
	resp, _ = guest.do("GET", fmt.Sprintf("/api/rooms/%d/prices/NOPE", roomID), nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown instrument prices: %d", resp.StatusCode)
	}

	// My rooms.
	mine := guest.mustJSON("GET", "/api/rooms", nil, http.StatusOK)
	if n := len(mine["rooms"].([]any)); n != 1 {
		t.Fatalf("my rooms = %d, want 1", n)
	}
}

func TestNewsIsBlindBoxSafe(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	advance(10*60*time.Second + time.Second) // day 10

	resp, body := host.do("GET", fmt.Sprintf("/api/rooms/%d/news?after=0", roomID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("news: %d %s", resp.StatusCode, body)
	}
	raw := string(body)
	for _, leak := range []string{`"track"`, `"true_shock"`, `"report_shock"`, `"real_name"`} {
		if strings.Contains(raw, leak) {
			t.Fatalf("news response leaks %s: %s", leak, raw[:min(400, len(raw))])
		}
	}

	news := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/news?after=0", roomID), nil, http.StatusOK)
	items := news["items"].([]any)
	if len(items) == 0 {
		t.Fatal("no news items by day 10")
	}
	maxID := 0.0
	for _, it := range items {
		m := it.(map[string]any)
		if d := m["day"].(float64); d > 10 {
			t.Fatalf("future news leaked: day %v", d)
		}
		if m["headline"].(string) == "" || m["media_id"].(string) == "" {
			t.Fatalf("bad news item: %v", m)
		}
		maxID = max(maxID, m["id"].(float64))
	}

	// Incremental fetch returns nothing new.
	again := host.mustJSON("GET",
		fmt.Sprintf("/api/rooms/%d/news?after=%d", roomID, int(maxID)), nil, http.StatusOK)
	if n := len(again["items"].([]any)); n != 0 {
		t.Fatalf("incremental fetch returned %d items, want 0", n)
	}
}

func TestScenarioListAndIsHost(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	if err := store.SetScenarioMeta(context.Background(), s.DB, "synthetic-v1", "合成测试剧本", ""); err != nil {
		t.Fatal(err)
	}
	host := registerClient(t, s, "host")
	guest := registerClient(t, s, "guest")

	scen := host.mustJSON("GET", "/api/scenarios", nil, http.StatusOK)
	items := scen["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("scenarios: %v", items)
	}
	first := items[0].(map[string]any)
	if first["id"] != "synthetic-v1" || first["name"] != "合成测试剧本" || first["days"].(float64) != 300 {
		t.Fatalf("scenario info: %v", first)
	}

	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	if created["is_host"] != true {
		t.Fatalf("creator not host: %v", created)
	}
	roomID := int64(created["id"].(float64))
	joined := guest.mustJSON("POST", "/api/rooms/join",
		map[string]any{"invite_code": created["invite_code"]}, http.StatusOK)
	if joined["is_host"] != false {
		t.Fatalf("guest is host: %v", joined)
	}
	state := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil, http.StatusOK)
	if state["room"].(map[string]any)["is_host"] != false {
		t.Fatalf("state is_host wrong for guest")
	}
}
