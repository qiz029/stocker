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
		map[string]any{"name": "Opening Bell", "scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	invite := created["invite_code"].(string)
	if created["status"] != "lobby" || created["name"] != "Opening Bell" || invite == "" {
		t.Fatalf("created room: %v", created)
	}
	resp, _ := host.do("POST", "/api/rooms", map[string]any{
		"name": " ", "scenario_id": "synthetic-v1", "day_duration_secs": 60,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("blank room name: %d, want 400", resp.StatusCode)
	}

	// Bad scenario / bad duration.
	resp, _ = host.do("POST", "/api/rooms", map[string]any{"scenario_id": "nope", "day_duration_secs": 60})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown scenario: %d", resp.StatusCode)
	}
	resp, _ = host.do("POST", "/api/rooms", map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 1})
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
	if len(lb) != 2+store.AgentPlayerCount {
		t.Fatalf("leaderboard: %v", lb)
	}
	agents := 0
	for _, item := range lb {
		row := item.(map[string]any)
		if _, hasUID := row["user_id"]; hasUID {
			t.Fatalf("leaderboard leaks user_id: %v", row)
		}
		if row["is_agent"].(bool) {
			agents++
		}
	}
	if agents != store.AgentPlayerCount {
		t.Fatalf("leaderboard agents = %d, want %d", agents, store.AgentPlayerCount)
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

func TestPublicRoomSpectateProfileAndJoin(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	fakeClock(s, time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC))
	host := registerClient(t, s, "publichost")
	viewer := registerIncompleteClient(t, s, "viewer")
	incompleteCreator := registerIncompleteClient(t, s, "newhost")
	resp, _ := incompleteCreator.do("POST", "/api/rooms", map[string]any{
		"scenario_id": "synthetic-v1", "day_duration_secs": 60, "visibility": "public",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("create without profile = %d, want 422", resp.StatusCode)
	}

	created := host.mustJSON("POST", "/api/rooms", map[string]any{
		"scenario_id": "synthetic-v1", "day_duration_secs": 60, "visibility": "public",
	}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	invite := created["invite_code"].(string)

	listing := viewer.mustJSON("GET", "/api/rooms/public", nil, http.StatusOK)
	rooms := listing["rooms"].([]any)
	if len(rooms) != 1 {
		t.Fatalf("public rooms = %v", rooms)
	}
	listed := rooms[0].(map[string]any)
	if created["name"] == "" || listed["name"] != created["name"] {
		t.Fatalf("public room name: created=%v listed=%v", created["name"], listed["name"])
	}
	if _, leaksInvite := listed["invite_code"]; leaksInvite {
		t.Fatalf("public listing leaks invite code: %v", listed)
	}
	if listed["human_players"].(float64) != 1 || listed["agent_players"].(float64) != store.AgentPlayerCount {
		t.Fatalf("public counts: %v", listed)
	}

	state := viewer.mustJSON("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil, http.StatusOK)
	publicRoom := state["room"].(map[string]any)
	if publicRoom["is_member"] != false {
		t.Fatalf("viewer marked as member: %v", publicRoom)
	}
	if _, leaksInvite := publicRoom["invite_code"]; leaksInvite {
		t.Fatalf("spectator state leaks invite code: %v", publicRoom)
	}

	resp, _ = viewer.do("POST", fmt.Sprintf("/api/rooms/%d/join", roomID), nil)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("join without profile = %d, want 422", resp.StatusCode)
	}
	resp, _ = viewer.do("POST", "/api/rooms/join", map[string]any{"invite_code": invite})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invite join without profile = %d, want 422", resp.StatusCode)
	}
	profile := viewer.mustJSON("PUT", "/api/me/profile", map[string]any{
		"display_name": "Market Fox", "avatar_id": "fox",
	}, http.StatusOK)
	if profile["profile_complete"] != true || profile["display_name"] != "Market Fox" {
		t.Fatalf("profile response: %v", profile)
	}
	joined := viewer.mustJSON("POST", "/api/rooms/join", map[string]any{"invite_code": invite}, http.StatusOK)
	if joined["is_member"] != true || joined["invite_code"] == "" {
		t.Fatalf("joined room response: %v", joined)
	}

	// Read-only was real enforcement, not only hidden frontend controls.
	outsider := registerClient(t, s, "watchonly")
	resp, _ = outsider.do("POST", fmt.Sprintf("/api/rooms/%d/chat", roomID), map[string]any{"text": "hello"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("spectator chat = %d, want 403", resp.StatusCode)
	}

	private := host.mustJSON("POST", "/api/rooms", map[string]any{
		"scenario_id": "synthetic-v1", "day_duration_secs": 60,
	}, http.StatusOK)
	resp, _ = outsider.do("GET", fmt.Sprintf("/api/rooms/%d", int64(private["id"].(float64))), nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("private room spectator read = %d, want 403", resp.StatusCode)
	}
}

func TestPublicRoomCapacityIsAtomic(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	host := registerClient(t, s, "capacity_host")
	created := host.mustJSON("POST", "/api/rooms", map[string]any{
		"scenario_id": "synthetic-v1", "day_duration_secs": 60, "visibility": "public",
	}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	joinPath := fmt.Sprintf("/api/rooms/%d/join", roomID)

	// Host plus ten successful joins leaves exactly one human seat.
	for i := 0; i < store.MaxHumanPlayers-2; i++ {
		player := registerClient(t, s, fmt.Sprintf("seat_%02d", i))
		player.mustJSON("POST", joinPath, nil, http.StatusOK)
	}
	racers := []*client{
		registerClient(t, s, "last_seat_a"),
		registerClient(t, s, "last_seat_b"),
	}
	start := make(chan struct{})
	statuses := make(chan int, len(racers))
	for _, racer := range racers {
		go func(c *client) {
			<-start
			resp, _ := c.do("POST", joinPath, nil)
			statuses <- resp.StatusCode
		}(racer)
	}
	close(start)
	ok, conflict := 0, 0
	for range racers {
		switch <-statuses {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("concurrent final-seat results: ok=%d conflict=%d", ok, conflict)
	}
	humans, err := store.HumanPlayerCount(context.Background(), s.DB, roomID)
	if err != nil {
		t.Fatal(err)
	}
	if humans != store.MaxHumanPlayers {
		t.Fatalf("human players = %d, want %d", humans, store.MaxHumanPlayers)
	}
}

func TestNewsIsBlindBoxSafe(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	outsider := registerClient(t, s, "outsider")
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
	for _, leak := range []string{`"track"`, `"true_shock"`, `"report_shock"`, `"real_name"`, `"driven_by_user_id"`} {
		if strings.Contains(raw, leak) {
			t.Fatalf("news response leaks %s: %s", leak, raw[:min(400, len(raw))])
		}
	}

	news := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/news?after=0", roomID), nil, http.StatusOK)
	items := news["items"].([]any)
	if len(items) == 0 {
		t.Fatal("no news items by day 10")
	}
	first := items[0].(map[string]any)
	newsID := int64(first["id"].(float64))
	detailPath := fmt.Sprintf("/api/rooms/%d/news/%d", roomID, newsID)
	detail := host.mustJSON("GET", detailPath, nil, http.StatusOK)
	if detail["body"] != first["body"] || detail["headline"] != first["headline"] {
		t.Fatalf("news detail does not match feed item: detail=%v feed=%v", detail, first)
	}
	detailRaw := fmt.Sprintf("%v", detail)
	for _, leak := range []string{"track", "true_shock", "report_shock", "real_name", "driven_by_user_id"} {
		if strings.Contains(detailRaw, leak) {
			t.Fatalf("news detail leaks %s: %s", leak, detailRaw)
		}
	}
	resp, _ = outsider.do("GET", detailPath, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider news detail: %d", resp.StatusCode)
	}
	var futureID int64
	if err := s.DB.QueryRow(context.Background(), `
		SELECT id FROM room_news WHERE room_id = $1 AND day > 10 ORDER BY day, id LIMIT 1`, roomID).Scan(&futureID); err != nil {
		t.Fatal(err)
	}
	resp, _ = host.do("GET", fmt.Sprintf("/api/rooms/%d/news/%d", roomID, futureID), nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("future news detail status = %d, want 404", resp.StatusCode)
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
		// English copies ride along (empty until the bilingual copy fill
		// lands); the keys must always be present.
		for _, k := range []string{"headline_en", "body_en"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("news item missing %s: %v", k, m)
			}
		}
		// cluster_id is exposed (nullable): it only groups already-published
		// items; track/shock vectors and the planting user remain forbidden
		// (checked above). disputed/exposed are public action flags.
		if _, ok := m["cluster_id"]; !ok {
			t.Fatalf("news item missing cluster_id: %v", m)
		}
		if _, ok := m["disputed"]; !ok {
			t.Fatalf("news item missing disputed: %v", m)
		}
		if _, ok := m["exposed"]; !ok {
			t.Fatalf("news item missing exposed: %v", m)
		}
		maxID = max(maxID, m["id"].(float64))
	}

	// media_accuracy carries aggregate counts only.
	acc, ok := news["media_accuracy"].(map[string]any)
	if !ok {
		t.Fatalf("media_accuracy missing or wrong shape: %v", news["media_accuracy"])
	}
	for media, v := range acc {
		entry := v.(map[string]any)
		if len(entry) != 2 {
			t.Fatalf("media_accuracy[%s] carries extra fields: %v", media, entry)
		}
		if _, ok := entry["reports"]; !ok {
			t.Fatalf("media_accuracy[%s] missing reports: %v", media, entry)
		}
		if _, ok := entry["hits"]; !ok {
			t.Fatalf("media_accuracy[%s] missing hits: %v", media, entry)
		}
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
	if err := store.SetScenarioMeta(context.Background(), s.DB, "synthetic-v1", "合成测试剧本", "Synthetic Test Scenario", ""); err != nil {
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
	// name_en rides along (empty until an English name is set).
	if _, ok := first["name_en"]; !ok {
		t.Fatalf("scenario info missing name_en: %v", first)
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

func TestForumEndpoint(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))

	// Lobby: not started yet.
	resp, _ := host.do("GET", fmt.Sprintf("/api/rooms/%d/forum?after=0", roomID), nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("lobby forum: %d", resp.StatusCode)
	}

	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	advance(5*60*time.Second + time.Second) // day 5

	resp, body := host.do("GET", fmt.Sprintf("/api/rooms/%d/forum?after=0", roomID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("forum: %d %s", resp.StatusCode, body)
	}
	// Blind box: the persona hint never leaves the server.
	if strings.Contains(string(body), "persona") {
		t.Fatalf("forum response leaks persona: %s", body[:min(400, len(body))])
	}

	forum := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/forum?after=0", roomID), nil, http.StatusOK)
	items := forum["items"].([]any)
	if len(items) == 0 {
		t.Fatal("no forum posts by day 5")
	}
	maxID := 0.0
	for _, it := range items {
		m := it.(map[string]any)
		if len(m) != 7 { // bilingual post fields plus disclosed Agent identity
			t.Fatalf("forum item carries extra fields: %v", m)
		}
		if d := m["day"].(float64); d > 5 {
			t.Fatalf("future forum post leaked: day %v", d)
		}
		if m["npc_name"].(string) == "" || m["body"].(string) == "" {
			t.Fatalf("bad forum item: %v", m)
		}
		if m["is_agent"] != true {
			t.Fatalf("forum persona is not disclosed as Agent: %v", m)
		}
		maxID = max(maxID, m["id"].(float64))
	}

	// Cursor pagination: incremental fetch returns nothing new.
	again := host.mustJSON("GET",
		fmt.Sprintf("/api/rooms/%d/forum?after=%d", roomID, int(maxID)), nil, http.StatusOK)
	if n := len(again["items"].([]any)); n != 0 {
		t.Fatalf("incremental fetch returned %d items, want 0", n)
	}
}
