package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/toddzheng/stocker/server/internal/store"
)

func TestRevealOnlyAfterGameEnds(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	if err := store.SetScenarioMeta(context.Background(), s.DB, "synthetic-v1", "合成测试剧本", "Synthetic Test Scenario", "1999-01 ~ 2001-12"); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	host.mustJSON("PUT", "/api/me/profile",
		map[string]any{"display_name": "Market Owl", "avatar_id": "owl"}, http.StatusOK)
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
	if tr["username"] != "Market Owl" || tr["instrument_id"] != "S1" || tr["day"].(float64) != 1 {
		t.Fatalf("trade: %v", tr)
	}
	if len(got["leaderboard"].([]any)) != 1+store.AgentPlayerCount {
		t.Fatalf("reveal leaderboard: %v", got["leaderboard"])
	}
	if got["real_period"] != "1999-01 ~ 2001-12" {
		t.Fatalf("real_period: %v", got["real_period"])
	}
}
