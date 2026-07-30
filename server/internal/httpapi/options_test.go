package httpapi

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"
)

// getChain fetches the bare-array option chain response.
func getChain(t *testing.T, c *client, path string, wantStatus int) []map[string]any {
	t.Helper()
	resp, data := c.do("GET", path, nil)
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s: status %d, want %d; body: %s", path, resp.StatusCode, wantStatus, data)
	}
	var out []map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("GET %s: bad JSON %q: %v", path, data, err)
	}
	return out
}

// Full option flow: priced chain → buy → portfolio position → mark moves
// with the clock → partial sell-to-close → remainder cash-settles at expiry.
func TestOptionFlow(t *testing.T) {
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

	chainPath := fmt.Sprintf("/api/rooms/%d/options", roomID)
	ordersPath := fmt.Sprintf("/api/rooms/%d/options/orders", roomID)
	portfolioPath := fmt.Sprintf("/api/rooms/%d/portfolio", roomID)

	// Lobby: the chain is not available yet.
	if resp, _ := guest.do("GET", chainPath+"?instrument_id=S1", nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("lobby chain: %d, want 400", resp.StatusCode)
	}
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)

	// Missing instrument_id is a 400; an unknown one yields an empty array.
	if resp, _ := guest.do("GET", chainPath, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("no instrument_id: %d, want 400", resp.StatusCode)
	}
	if chain := getChain(t, guest, chainPath+"?instrument_id=NOPE", http.StatusOK); len(chain) != 0 {
		t.Fatalf("unknown instrument chain = %v, want empty", chain)
	}

	// The chain is priced and sorted by expiry then strike; the nearest
	// expiry is day 5 and every strike has a call and a put.
	chain := getChain(t, guest, chainPath+"?instrument_id=S1", http.StatusOK)
	if len(chain) == 0 {
		t.Fatal("empty chain for S1")
	}
	prevExpiry, prevStrike := -1.0, -1.0
	kinds := map[float64]map[string]bool{}
	for _, o := range chain {
		expiry := o["expiry_day"].(float64)
		strike := o["strike"].(float64)
		if o["price"].(float64) < 0 {
			t.Fatalf("negative price: %v", o)
		}
		if expiry < prevExpiry || (expiry == prevExpiry && strike < prevStrike) {
			t.Fatalf("chain not sorted: %v after (%v, %v)", o, prevExpiry, prevStrike)
		}
		prevExpiry, prevStrike = expiry, strike
		if kinds[expiry] == nil {
			kinds[expiry] = map[string]bool{}
		}
		kinds[expiry][o["kind"].(string)] = true
	}
	if prevExpiry != 5 && kinds[5] == nil {
		t.Fatalf("nearest expiry = %v, want 5", chain[0]["expiry_day"])
	}
	if !kinds[5]["call"] || !kinds[5]["put"] {
		t.Fatalf("expiry 5 kinds = %v, want call+put", kinds[5])
	}

	// Pick the first expiry-5 call and buy 2 contracts.
	var target map[string]any
	for _, o := range chain {
		if o["expiry_day"].(float64) == 5 && o["kind"] == "call" {
			target = o
			break
		}
	}
	optionID := int64(target["option_id"].(float64))
	strike := target["strike"].(float64)

	if resp, _ := guest.do("POST", ordersPath,
		map[string]any{"option_id": optionID, "action": "buy", "contracts": 0}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("zero-contract buy: %d, want 400", resp.StatusCode)
	}
	fill := guest.mustJSON("POST", ordersPath,
		map[string]any{"option_id": optionID, "action": "buy", "contracts": 2}, http.StatusOK)
	premium := int64(fill["amount_cents"].(float64))
	if fill["action"] != "buy" || fill["contracts"].(float64) != 2 || premium <= 0 {
		t.Fatalf("buy fill: %v", fill)
	}
	if int64(fill["cash_cents"].(float64)) != 10_000_000-premium {
		t.Fatalf("cash after buy: %v", fill["cash_cents"])
	}

	// Portfolio shows the position; total is unchanged (premium → MTM).
	p := guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	opts := p["options"].([]any)
	if len(opts) != 1 {
		t.Fatalf("portfolio options = %v, want 1", opts)
	}
	o0 := opts[0].(map[string]any)
	if int64(o0["option_id"].(float64)) != optionID || o0["contracts"].(float64) != 2 {
		t.Fatalf("portfolio option: %v", o0)
	}
	if o0["pnl_cents"].(float64) != 0 {
		t.Fatalf("same-day pnl = %v, want 0", o0["pnl_cents"])
	}
	if p["total_cents"].(float64) != 10_000_000 {
		t.Fatalf("total after buy = %v, want 10000000", p["total_cents"])
	}

	// Three days pass: the BS mark moves and P&L tracks it.
	advance(3*61*time.Second + time.Second)
	p = guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	o3 := p["options"].([]any)[0].(map[string]any)
	if o3["price"].(float64) == o0["price"].(float64) {
		t.Fatalf("price did not move: day0 %v day3 %v", o0["price"], o3["price"])
	}
	wantValue := int64(math.Round(o3["price"].(float64) * 2 * 100))
	if int64(o3["value_cents"].(float64)) != wantValue {
		t.Fatalf("day-3 value = %v, want %d", o3["value_cents"], wantValue)
	}
	if int64(o3["pnl_cents"].(float64)) != wantValue-premium {
		t.Fatalf("day-3 pnl = %v, want %d", o3["pnl_cents"], wantValue-premium)
	}

	// Sell one contract to close: cash is credited at the same mark.
	sell := guest.mustJSON("POST", ordersPath,
		map[string]any{"option_id": optionID, "action": "sell", "contracts": 1}, http.StatusOK)
	sellAmount := int64(sell["amount_cents"].(float64))
	if sell["price"].(float64) != o3["price"].(float64) {
		t.Fatalf("sell price %v, want day-3 mark %v", sell["price"], o3["price"])
	}
	cashAfterSell := int64(sell["cash_cents"].(float64))
	if cashAfterSell != 10_000_000-premium+sellAmount {
		t.Fatalf("cash after sell = %d", cashAfterSell)
	}
	p = guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	if p["options"].([]any)[0].(map[string]any)["contracts"].(float64) != 1 {
		t.Fatalf("contracts after partial close: %v", p["options"])
	}

	// Past expiry (day 5): the remaining contract cash-settles at the
	// expiry day's close and disappears from the portfolio.
	advance(3 * 61 * time.Second) // day 6
	prices := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/prices/S1", roomID), nil, http.StatusOK)
	close5 := prices["days"].([]any)[5].(map[string]any)["close"].(float64)
	payoff := math.Max(close5-strike, 0)
	wantCash := cashAfterSell + int64(math.Round(payoff*100))

	p = guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	if len(p["options"].([]any)) != 0 {
		t.Fatalf("options after expiry = %v, want none", p["options"])
	}
	if int64(p["cash_cents"].(float64)) != wantCash {
		t.Fatalf("cash after expiry = %v, want %d (payoff %v)", p["cash_cents"], wantCash, payoff)
	}
	if int64(p["total_cents"].(float64)) != wantCash {
		t.Fatalf("total after expiry = %v, want %d", p["total_cents"], wantCash)
	}
}
