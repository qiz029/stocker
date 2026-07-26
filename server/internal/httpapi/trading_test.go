package httpapi

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// End-to-end: register → create/join/start → trade → settle-on-read →
// whale alert → leaderboard, all through HTTP with a fake clock.
func TestTradingFlow(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	guest := registerClient(t, s, "guest")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	guest.mustJSON("POST", "/api/rooms/join",
		map[string]any{"invite_code": created["invite_code"]}, http.StatusOK)
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)

	ordersPath := fmt.Sprintf("/api/rooms/%d/orders", roomID)
	portfolioPath := fmt.Sprintf("/api/rooms/%d/portfolio", roomID)

	// Day 0: guest goes 90% into S1 — big enough to trip the whale alert.
	order := guest.mustJSON("POST", ordersPath, map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 9_000_000}, http.StatusOK)
	if order["exec_day"].(float64) != 1 || order["status"] != "pending" {
		t.Fatalf("order: %v", order)
	}

	// Insufficient cash through the API maps to 400.
	resp, _ := guest.do("POST", ordersPath, map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 5_000_000})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("overspend via API: %d", resp.StatusCode)
	}

	// Frozen but unsettled: total still equals initial cash.
	p := guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	if p["total_cents"].(float64) != 10_000_000 {
		t.Fatalf("frozen total: %v", p["total_cents"])
	}

	// Day 1: reading the portfolio settles the order.
	advance(61 * time.Second)
	p = guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	positions := p["positions"].([]any)
	if len(positions) != 1 {
		t.Fatalf("positions after settle: %v", positions)
	}
	pos := positions[0].(map[string]any)
	if pos["instrument_id"] != "S1" || pos["shares"].(float64) <= 0 {
		t.Fatalf("position: %v", pos)
	}

	// The anonymous whale alert is in the feed; payload names no player.
	events := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/events?after=0", roomID), nil, http.StatusOK)
	items := events["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("events: %v", items)
	}
	ev := items[0].(map[string]any)
	payload := ev["payload"].(map[string]any)
	if ev["kind"] != "whale" || payload["instrument_id"] != "S1" || payload["side"] != "buy" {
		t.Fatalf("whale event: %v", ev)
	}
	if _, leaked := payload["user_id"]; leaked {
		t.Fatalf("whale event leaks user: %v", payload)
	}

	// Cancel flow: place then cancel restores cash.
	o2 := guest.mustJSON("POST", ordersPath, map[string]any{
		"instrument_id": "S2", "side": "buy", "amount_cents": 200_000}, http.StatusOK)
	guest.mustJSON("DELETE", fmt.Sprintf("%s/%d", ordersPath, int64(o2["id"].(float64))), nil, http.StatusOK)
	p = guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	if n := len(p["pending"].([]any)); n != 0 {
		t.Fatalf("pending after cancel: %d", n)
	}
	// Cancelling someone else's (or a filled) order is a 409.
	resp, _ = host.do("DELETE", fmt.Sprintf("%s/%d", ordersPath, int64(o2["id"].(float64))), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("cancel foreign order: %d", resp.StatusCode)
	}

	// Orders after game end are refused with 409.
	advance(400 * 60 * time.Second)
	resp, _ = guest.do("POST", ordersPath, map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 100})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("order after end: %d", resp.StatusCode)
	}
}
