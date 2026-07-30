package httpapi

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// End-to-end loan flow: borrow → portfolio debt/rate → interest accrues
// over days → repay → borrow to the cap → bankruptcy → order rejected →
// leaderboard shows the bankrupt row last with a curve.
func TestLoanFlow(t *testing.T) {
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

	loansPath := fmt.Sprintf("/api/rooms/%d/loans", roomID)
	portfolioPath := fmt.Sprintf("/api/rooms/%d/portfolio", roomID)

	// Bad action and bad amounts map to 400.
	if resp, _ := guest.do("POST", loansPath, map[string]any{"action": "nope", "amount_cents": 1}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad action: %d", resp.StatusCode)
	}
	if resp, _ := guest.do("POST", loansPath, map[string]any{"action": "borrow", "amount_cents": 0}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("zero borrow: %d", resp.StatusCode)
	}

	// Day 0: borrow $50,000.
	r := guest.mustJSON("POST", loansPath,
		map[string]any{"action": "borrow", "amount_cents": 5_000_000}, http.StatusOK)
	if r["debt_cents"].(float64) != 5_000_000 || r["cash_cents"].(float64) != 15_000_000 {
		t.Fatalf("borrow response: %v", r)
	}
	if r["max_debt_cents"].(float64) != 20_000_000 {
		t.Fatalf("max debt: %v", r["max_debt_cents"])
	}

	// Portfolio shows the debt; net total is unchanged; day-0 rate is the
	// 3% base (no trailing history yet).
	p := guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	if p["debt_cents"].(float64) != 5_000_000 || p["bankrupt"].(bool) {
		t.Fatalf("portfolio after borrow: %v", p)
	}
	if p["total_cents"].(float64) != 10_000_000 {
		t.Fatalf("net total: %v", p["total_cents"])
	}
	if p["interest_rate_annual_bp"].(float64) != 300 {
		t.Fatalf("day-0 rate: %v bp, want 300", p["interest_rate_annual_bp"])
	}

	// Three days pass: interest compounds into the debt.
	advance(3*61*time.Second + time.Second)
	p = guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	debt := int64(p["debt_cents"].(float64))
	if debt <= 5_000_000 {
		t.Fatalf("debt did not accrue: %d", debt)
	}
	if int64(p["total_cents"].(float64)) != 15_000_000-debt {
		t.Fatalf("net total: %v, want %d", p["total_cents"], 15_000_000-debt)
	}
	if bp := p["interest_rate_annual_bp"].(float64); bp < 100 || bp > 6000 {
		t.Fatalf("rate out of clamp range: %v bp", bp)
	}

	// Repay $20,000; over-repaying the remaining debt is a 400.
	r = guest.mustJSON("POST", loansPath,
		map[string]any{"action": "repay", "amount_cents": 2_000_000}, http.StatusOK)
	debt = int64(r["debt_cents"].(float64))
	if debt <= 3_000_000 {
		t.Fatalf("debt after repay: %d", debt)
	}
	if resp, _ := guest.do("POST", loansPath,
		map[string]any{"action": "repay", "amount_cents": debt + 1}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("over-repay: %d", resp.StatusCode)
	}

	// Borrow up to exactly the cap; one cent more is refused.
	r = guest.mustJSON("POST", loansPath,
		map[string]any{"action": "borrow", "amount_cents": 20_000_000 - debt}, http.StatusOK)
	if r["debt_cents"].(float64) != 20_000_000 {
		t.Fatalf("debt at cap: %v", r["debt_cents"])
	}
	if resp, _ := guest.do("POST", loansPath,
		map[string]any{"action": "borrow", "amount_cents": 1}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("borrow past cap: %d", resp.StatusCode)
	}

	// The next day of interest crosses the cap → bankruptcy.
	advance(61 * time.Second)
	p = guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	if !p["bankrupt"].(bool) {
		t.Fatalf("expected bankruptcy: %v", p)
	}
	if p["debt_cents"].(float64) <= 20_000_000 {
		t.Fatalf("debt past cap: %v", p["debt_cents"])
	}

	// Orders and further loans are rejected; the error is a 400.
	if resp, _ := guest.do("POST", fmt.Sprintf("/api/rooms/%d/orders", roomID), map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 100}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bankrupt order: %d", resp.StatusCode)
	}
	for _, action := range []string{"borrow", "repay"} {
		if resp, _ := guest.do("POST", loansPath,
			map[string]any{"action": action, "amount_cents": 100}); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("bankrupt %s: %d", action, resp.StatusCode)
		}
	}

	// The bankruptcy is announced in the event feed.
	events := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/events?after=0", roomID), nil, http.StatusOK)
	items := events["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("events: %v", items)
	}
	ev := items[0].(map[string]any)
	payload := ev["payload"].(map[string]any)
	if ev["kind"] != "bankrupt" || payload["day"].(float64) != 4 || payload["username"] != "guest" {
		t.Fatalf("bankrupt event: %v", ev)
	}

	// Leaderboard: solvent host first, bankrupt guest last with a curve.
	state := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil, http.StatusOK)
	board := state["leaderboard"].([]any)
	if len(board) != 2 {
		t.Fatalf("leaderboard: %v", board)
	}
	first := board[0].(map[string]any)
	last := board[1].(map[string]any)
	if first["username"] != "host" || first["bankrupt"].(bool) {
		t.Fatalf("leaderboard first: %v", first)
	}
	if last["username"] != "guest" || !last["bankrupt"].(bool) {
		t.Fatalf("leaderboard last: %v", last)
	}
	curve := last["curve"].([]any)
	if len(curve) == 0 {
		t.Fatalf("bankrupt row has no curve: %v", last)
	}
	if int64(curve[len(curve)-1].(float64)) != int64(last["total_cents"].(float64)) {
		t.Fatalf("curve tail %v != total %v", curve[len(curve)-1], last["total_cents"])
	}
}
