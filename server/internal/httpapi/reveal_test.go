package httpapi

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestRevealOnlyAfterGameEnds(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	revealPath := fmt.Sprintf("/api/rooms/%d/reveal", roomID)

	// Lobby: no reveal.
	resp, _ := host.do("GET", revealPath, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("reveal in lobby: %d", resp.StatusCode)
	}

	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/orders", roomID), map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 1_000_000}, http.StatusOK)

	// Mid-game: still no reveal.
	advance(10 * 60 * time.Second)
	resp, _ = host.do("GET", revealPath, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("reveal mid-game: %d", resp.StatusCode)
	}

	// Past the end: reveal opens up and carries real identity + trades.
	advance(400 * 60 * time.Second)
	got := host.mustJSON("GET", revealPath, nil, http.StatusOK)
	instruments := got["instruments"].([]any)
	if len(instruments) != 8 {
		t.Fatalf("reveal instruments: %d", len(instruments))
	}
	if _, hasKey := instruments[0].(map[string]any)["real_name"]; !hasKey {
		t.Fatalf("reveal missing real_name: %v", instruments[0])
	}
	trades := got["trades"].([]any)
	if len(trades) != 1 {
		t.Fatalf("reveal trades: %v", trades)
	}
	tr := trades[0].(map[string]any)
	if tr["username"] != "host" || tr["instrument_id"] != "S1" || tr["day"].(float64) != 1 {
		t.Fatalf("trade: %v", tr)
	}
	if len(got["leaderboard"].([]any)) != 1 {
		t.Fatalf("reveal leaderboard: %v", got["leaderboard"])
	}
}
