package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The share page is public (no session cookie), read-only, and keeps the
// blind box closed while the room is running.
func TestSharePage(t *testing.T) {
	s := newServer(t)
	sc := seedScenario(t, s)
	// Era names live in the scenarios table; set one so the blind-box leak
	// check has something to detect.
	if _, err := s.DB.Exec(context.Background(),
		`UPDATE scenarios SET name = '秘密时代', name_en = 'Secret Era' WHERE id = $1`, sc.ID); err != nil {
		t.Fatalf("name scenario: %v", err)
	}
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"name": "Opening Bell", "scenario_id": sc.ID, "day_duration_secs": 60}, http.StatusOK)
	token, _ := created["share_token"].(string)
	if token == "" {
		t.Fatalf("created room has no share_token: %v", created)
	}

	getShare := func(acceptLang string) (int, string) {
		t.Helper()
		req := httptest.NewRequest("GET", "/share/"+token, nil)
		if acceptLang != "" {
			req.Header.Set("Accept-Language", acceptLang)
		}
		rec := httptest.NewRecorder()
		s.Router().ServeHTTP(rec, req)
		resp := rec.Result()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(data)
	}

	// No auth needed. Lobby state renders without a leaderboard.
	code, html := getShare("")
	if code != http.StatusOK {
		t.Fatalf("share page: %d; body: %s", code, html)
	}
	for _, want := range []string{"Opening Bell", "og:title", "/og.png"} {
		if !strings.Contains(html, want) {
			t.Fatalf("lobby share page missing %q", want)
		}
	}

	// Unknown token → branded 404, and never a room leak.
	req := httptest.NewRequest("GET", "/share/DOESNOTEXIST", nil)
	rec := httptest.NewRecorder()
	s.Router().ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("bad token: %d, want 404", rec.Result().StatusCode)
	}

	// Running room: leaderboard shows, era stays hidden (blind box).
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", int64(created["id"].(float64))), nil, http.StatusOK)
	advance(2 * time.Minute)
	code, html = getShare("")
	if code != http.StatusOK {
		t.Fatalf("running share page: %d", code)
	}
	if !strings.Contains(html, "host") {
		t.Fatalf("running share page missing leaderboard: %.300s", html)
	}
	if strings.Contains(html, "秘密时代") || strings.Contains(html, "Secret Era") {
		t.Fatalf("running share page leaks the hidden era")
	}
	if !strings.Contains(html, `http-equiv="refresh"`) {
		t.Fatalf("running share page should auto-refresh")
	}

	// zh Accept-Language switches the copy.
	code, html = getShare("zh-CN,zh;q=0.9")
	if code != http.StatusOK || !strings.Contains(html, `lang="zh-CN"`) || !strings.Contains(html, "净资产排行") {
		t.Fatalf("zh share page: %d", code)
	}

	// After the timeline ends the blind box opens: the era is named.
	advance(time.Duration(sc.Days+1) * time.Minute)
	code, html = getShare("zh-CN,zh;q=0.9")
	if code != http.StatusOK || !strings.Contains(html, "秘密时代") {
		t.Fatalf("ended share page should reveal the era: %d %.300s", code, html)
	}
}

// Non-members never receive the share token through the API.
func TestShareTokenMembership(t *testing.T) {
	s := newServer(t)
	sc := seedScenario(t, s)
	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"name": "Token Room", "scenario_id": sc.ID, "day_duration_secs": 60}, http.StatusOK)
	if created["share_token"] == "" {
		t.Fatalf("host should see share_token")
	}
	// Public-room read path for a logged-in non-member must not leak tokens.
	pub := host.mustJSON("POST", "/api/rooms",
		map[string]any{"name": "Public Room", "scenario_id": sc.ID, "day_duration_secs": 60, "visibility": "public"}, http.StatusOK)
	pubID := int64(pub["id"].(float64))
	outsider := registerClient(t, s, "outsider")
	view := outsider.mustJSON("GET", fmt.Sprintf("/api/rooms/%d", pubID), nil, http.StatusOK)
	room := view["room"].(map[string]any)
	if _, ok := room["share_token"]; ok {
		t.Fatalf("non-member room view leaks share_token: %v", room)
	}
}
