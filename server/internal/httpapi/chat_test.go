package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestChatFlow(t *testing.T) {
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
	guest.mustJSON("POST", "/api/rooms/join",
		map[string]any{"invite_code": created["invite_code"]}, http.StatusOK)
	chatPath := fmt.Sprintf("/api/rooms/%d/chat", roomID)

	// Lobby chat is allowed, stamped day 0.
	host.mustJSON("POST", chatPath, map[string]any{"text": "开局前聊两句"}, http.StatusOK)

	// Start, advance to day 2, chat again.
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	advance(2*60*time.Second + time.Second)
	guest.mustJSON("POST", chatPath, map[string]any{"text": "科技股什么情况"}, http.StatusOK)

	got := guest.mustJSON("GET", chatPath+"?after=0", nil, http.StatusOK)
	items := got["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("chat items: %v", items)
	}
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	if first["username"] != "host" || first["day"].(float64) != 0 {
		t.Fatalf("first message: %v", first)
	}
	if second["username"] != "guest" || second["day"].(float64) != 2 {
		t.Fatalf("second message: %v", second)
	}

	// Incremental fetch.
	after := int64(second["id"].(float64))
	tail := guest.mustJSON("GET", fmt.Sprintf("%s?after=%d", chatPath, after), nil, http.StatusOK)
	if n := len(tail["items"].([]any)); n != 0 {
		t.Fatalf("incremental returned %d", n)
	}

	// Validation and membership.
	resp, _ := guest.do("POST", chatPath, map[string]any{"text": "   "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("blank chat: %d", resp.StatusCode)
	}
	resp, _ = guest.do("POST", chatPath, map[string]any{"text": strings.Repeat("啊", 501)})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized chat: %d", resp.StatusCode)
	}
	resp, _ = outsider.do("GET", chatPath+"?after=0", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider chat read: %d", resp.StatusCode)
	}
}

func TestRoomStateCarriesProfileAndNewsBody(t *testing.T) {
	s := newServer(t)
	sc := seedScenario(t, s)
	if err := storeSetDisplayForTest(s, sc.ID); err != nil {
		t.Fatalf("set display: %v", err)
	}
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	advance(61 * time.Second)

	state := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil, http.StatusOK)
	instruments := state["instruments"].([]any)
	s1 := instruments[0].(map[string]any)
	if s1["alias"] != "郊狼网络" {
		t.Fatalf("alias not applied: %v", s1)
	}
	profile, ok := s1["profile"].(map[string]any)
	if !ok || profile["bull"] != "卖铲人" {
		t.Fatalf("profile missing: %v", s1)
	}
	// Instruments without display data have null profile, not a crash.
	s2 := instruments[1].(map[string]any)
	if _, hasKey := s2["profile"]; !hasKey {
		t.Fatalf("profile key absent on undisplayed instrument: %v", s2)
	}

	// News items carry a body field (empty until plan 4).
	news := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/news?after=0", roomID), nil, http.StatusOK)
	items := news["items"].([]any)
	if len(items) == 0 {
		t.Fatal("no news by day 1")
	}
	if _, hasBody := items[0].(map[string]any)["body"]; !hasBody {
		t.Fatalf("news item missing body: %v", items[0])
	}
}
