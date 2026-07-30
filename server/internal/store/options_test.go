package store

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBlackScholesMath(t *testing.T) {
	// T → 0 collapses to intrinsic value.
	if p := BlackScholes("call", 110, 100, 0, 0.03, 0.30); p != 10 {
		t.Fatalf("ITM call at expiry = %v, want 10", p)
	}
	if p := BlackScholes("call", 90, 100, 0, 0.03, 0.30); p != 0 {
		t.Fatalf("OTM call at expiry = %v, want 0", p)
	}
	if p := BlackScholes("put", 90, 100, 0, 0.03, 0.30); p != 10 {
		t.Fatalf("ITM put at expiry = %v, want 10", p)
	}

	// Put-call parity: C − P = S − K·e^(−rT).
	S, K, T, r, sigma := 100.0, 105.0, 0.25, 0.03, 0.35
	c := BlackScholes("call", S, K, T, r, sigma)
	p := BlackScholes("put", S, K, T, r, sigma)
	if want := S - K*math.Exp(-r*T); math.Abs(c-p-want) > 1e-9 {
		t.Fatalf("parity: C−P = %v, want %v", c-p, want)
	}

	// Calls increase in S; prices never go negative.
	prev := -1.0
	for s := 80.0; s <= 120.0; s += 5 {
		c := BlackScholes("call", s, 100, 0.5, 0.03, 0.30)
		if c < 0 {
			t.Fatalf("negative call price %v at S=%v", c, s)
		}
		if c <= prev {
			t.Fatalf("call not increasing: S=%v price %v (prev %v)", s, c, prev)
		}
		prev = c
	}
	if p := BlackScholes("put", 200, 50, 1, 0.03, 0.10); p < 0 {
		t.Fatalf("negative put price %v", p)
	}
}

// closeAt reads one instrument's close for exact payoff math.
func closeAt(t *testing.T, pool *pgxpool.Pool, roomID int64, inst string, day int) float64 {
	t.Helper()
	var c float64
	if err := pool.QueryRow(context.Background(), `
		SELECT close FROM room_prices
		WHERE room_id = $1 AND instrument_id = $2 AND day = $3`,
		roomID, inst, day).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

// countOptions rows, optionally restricted to one expiry (−1 = all).
func countOptions(t *testing.T, pool *pgxpool.Pool, roomID int64, expiry int) int {
	t.Helper()
	q := `SELECT count(*) FROM room_options WHERE room_id = $1`
	args := []any{roomID}
	if expiry >= 0 {
		q += ` AND expiry_day = $2`
		args = append(args, expiry)
	}
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// expectedChainRows computes the rows one listing pass should create:
// instruments × deduped rounded strikes × 2 kinds.
func expectedChainRows(t *testing.T, pool *pgxpool.Pool, roomID int64, day int) int {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT close FROM room_prices WHERE room_id = $1 AND day = $2`, roomID, day)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var c float64
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		seen := map[float64]bool{}
		for _, m := range strikeMults {
			seen[math.Round(c*m*100)/100] = true
		}
		total += len(seen) * 2
	}
	return total
}

func TestOptionChainListing(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)

	room, err := CreateRoom(ctx, pool, sc, host.ID, 60, nil)
	if err != nil {
		t.Fatal(err)
	}

	// CreateRoom lists the first two expiries (5 and 10) anchored to the
	// day-0 close.
	want0 := expectedChainRows(t, pool, room.ID, 0)
	for _, expiry := range []int{5, 10} {
		if n := countOptions(t, pool, room.ID, expiry); n != want0 {
			t.Fatalf("expiry %d rows = %d, want %d", expiry, n, want0)
		}
	}
	var minE, maxE int
	if err := pool.QueryRow(ctx, `
		SELECT min(expiry_day), max(expiry_day) FROM room_options WHERE room_id = $1`,
		room.ID).Scan(&minE, &maxE); err != nil {
		t.Fatal(err)
	}
	if minE != 5 || maxE != 10 {
		t.Fatalf("expiry range = %d..%d, want 5..10", minE, maxE)
	}
	var badListed int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM room_options WHERE room_id = $1 AND listed_day != 0`,
		room.ID).Scan(&badListed); err != nil {
		t.Fatal(err)
	}
	if badListed != 0 {
		t.Fatalf("listed_day != 0 on %d rows", badListed)
	}

	// Strikes anchor to the listing day's close, rounded to 2 decimals,
	// each strike × {call, put}.
	close0 := closeAt(t, pool, room.ID, "S1", 0)
	for _, m := range strikeMults {
		strike := math.Round(close0*m*100) / 100
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM room_options
			WHERE room_id = $1 AND instrument_id = 'S1' AND expiry_day = 5 AND strike = $2`,
			room.ID, strike).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Fatalf("strike %v (mult %v): %d rows, want 2 (call+put)", strike, m, n)
		}
	}

	// Start and settle day 0: listing is idempotent, nothing new.
	t0 := time.Now().Truncate(time.Second)
	if room, err = StartRoom(ctx, pool, room.ID, host.ID, t0); err != nil {
		t.Fatal(err)
	}
	before := countOptions(t, pool, room.ID, -1)
	if _, _, err := SettleRoom(ctx, pool, room, t0); err != nil {
		t.Fatal(err)
	}
	if n := countOptions(t, pool, room.ID, -1); n != before {
		t.Fatalf("re-settle day 0 listed %d new rows", n-before)
	}

	// Day 5 rolls the chain forward: expiry 15 appears, anchored to the
	// day-5 close; repeating the settle adds nothing.
	at5 := t0.Add(5*61*time.Second + time.Second)
	if _, _, err := SettleRoom(ctx, pool, room, at5); err != nil {
		t.Fatal(err)
	}
	want5 := expectedChainRows(t, pool, room.ID, 5)
	if n := countOptions(t, pool, room.ID, 15); n != want5 {
		t.Fatalf("expiry 15 rows = %d, want %d", n, want5)
	}
	if n := countOptions(t, pool, room.ID, -1); n != before+want5 {
		t.Fatalf("total rows = %d, want %d", n, before+want5)
	}
	var listed15 int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM room_options WHERE room_id = $1 AND expiry_day = 15 AND listed_day = 5`,
		room.ID).Scan(&listed15); err != nil {
		t.Fatal(err)
	}
	if listed15 != want5 {
		t.Fatalf("expiry 15 listed_day=5 rows = %d, want %d", listed15, want5)
	}
	if _, _, err := SettleRoom(ctx, pool, room, at5); err != nil {
		t.Fatal(err)
	}
	if n := countOptions(t, pool, room.ID, -1); n != before+want5 {
		t.Fatalf("re-settle day 5 changed row count: %d, want %d", n, before+want5)
	}
}

// firstOptionID returns one listed contract for S1.
func firstOptionID(t *testing.T, pool *pgxpool.Pool, roomID int64, kind string, expiry int) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `
		SELECT id FROM room_options
		WHERE room_id = $1 AND instrument_id = 'S1' AND kind = $2 AND expiry_day = $3
		ORDER BY strike LIMIT 1`, roomID, kind, expiry).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBuySellOptionGuards(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)
	optID := firstOptionID(t, pool, room.ID, "call", 10)

	// Bad contract counts and unknown contracts.
	if _, err := BuyOption(ctx, pool, room, guest.ID, optID, 0, t0); !errors.Is(err, ErrBadOptionOrder) {
		t.Fatalf("buy 0 contracts: %v", err)
	}
	if _, err := BuyOption(ctx, pool, room, guest.ID, optID, -2, t0); !errors.Is(err, ErrBadOptionOrder) {
		t.Fatalf("buy -2 contracts: %v", err)
	}
	if _, err := BuyOption(ctx, pool, room, guest.ID, 999999, 1, t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("buy unknown option: %v", err)
	}

	// Cash guard: an absurd size must exceed the initial $100k.
	if _, err := BuyOption(ctx, pool, room, guest.ID, optID, 1e9, t0); !errors.Is(err, ErrInsufficientCash) {
		t.Fatalf("oversized buy: %v", err)
	}

	// Sell guard: nothing held, then more than held.
	if _, err := SellOption(ctx, pool, room, guest.ID, optID, 1, t0); !errors.Is(err, ErrInsufficientContracts) {
		t.Fatalf("sell without position: %v", err)
	}
	fill, err := BuyOption(ctx, pool, room, guest.ID, optID, 2, t0)
	if err != nil {
		t.Fatalf("buy 2: %v", err)
	}
	if fill.Action != "buy" || fill.Contracts != 2 || fill.AmountCents <= 0 {
		t.Fatalf("buy fill: %+v", fill)
	}
	if fill.CashCents != InitialCashCents-fill.AmountCents {
		t.Fatalf("cash after buy = %d, want %d", fill.CashCents, InitialCashCents-fill.AmountCents)
	}
	if _, err := SellOption(ctx, pool, room, guest.ID, optID, 3, t0); !errors.Is(err, ErrInsufficientContracts) {
		t.Fatalf("oversell: %v", err)
	}

	// Sell-to-close credits the mark and leaves no dust row behind.
	sell, err := SellOption(ctx, pool, room, guest.ID, optID, 2, t0)
	if err != nil {
		t.Fatalf("sell 2: %v", err)
	}
	if sell.CashCents != InitialCashCents-fill.AmountCents+sell.AmountCents {
		t.Fatalf("cash after sell = %d", sell.CashCents)
	}
	var nPos int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM option_positions WHERE room_id = $1 AND user_id = $2`,
		room.ID, guest.ID).Scan(&nPos); err != nil {
		t.Fatal(err)
	}
	if nPos != 0 {
		t.Fatalf("position rows after full close = %d, want 0", nPos)
	}
	var nTrades int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM option_trades WHERE room_id = $1 AND user_id = $2`,
		room.ID, guest.ID).Scan(&nTrades); err != nil {
		t.Fatal(err)
	}
	if nTrades != 2 {
		t.Fatalf("option_trades = %d, want 2 (buy+sell)", nTrades)
	}

	// Not running (lobby) and ended rooms refuse.
	sc, err := LoadScenario(ctx, pool, room.ScenarioID)
	if err != nil {
		t.Fatal(err)
	}
	lobby, err := CreateRoom(ctx, pool, sc, room.HostUserID, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuyOption(ctx, pool, lobby, room.HostUserID, optID, 1, t0); !errors.Is(err, ErrRoomNotRunning) {
		t.Fatalf("buy in lobby: %v", err)
	}
	if _, err := BuyOption(ctx, pool, room, guest.ID, optID, 1,
		t0.Add(400*60*time.Second)); !errors.Is(err, ErrRoomEnded) {
		t.Fatalf("buy after end: %v", err)
	}
}

func TestBankruptCannotTradeOptions(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)
	optID := firstOptionID(t, pool, room.ID, "call", 10)

	// One day of interest on a maxed-out loan crosses the cap → bankrupt.
	if _, err := Borrow(ctx, pool, room, guest.ID, MaxDebtCents, t0); err != nil {
		t.Fatal(err)
	}
	at1 := t0.Add(61 * time.Second)
	if _, _, err := SettleRoom(ctx, pool, room, at1); err != nil {
		t.Fatal(err)
	}
	if _, err := BuyOption(ctx, pool, room, guest.ID, optID, 1, at1); !errors.Is(err, ErrPlayerBankrupt) {
		t.Fatalf("bankrupt buy: %v", err)
	}
	if _, err := SellOption(ctx, pool, room, guest.ID, optID, 1, at1); !errors.Is(err, ErrPlayerBankrupt) {
		t.Fatalf("bankrupt sell: %v", err)
	}
}

// insertOption adds a custom contract (tests pick strikes after peeking at
// the deterministic future close).
func insertOption(t *testing.T, pool *pgxpool.Pool, roomID int64, inst, kind string, strike float64, expiry int) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO room_options (room_id, instrument_id, kind, strike, expiry_day, listed_day)
		VALUES ($1, $2, $3, $4, $5, 0) RETURNING id`,
		roomID, inst, kind, strike, expiry).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOptionExpirySettlement(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Peek at the deterministic day-5 close, then list a deep-ITM call and
	// a deep-OTM put expiring that day.
	close5 := closeAt(t, pool, room.ID, "S1", 5)
	strike := math.Round(close5*50) / 100 // half the close
	callID := insertOption(t, pool, room.ID, "S1", "call", strike, 5)
	putID := insertOption(t, pool, room.ID, "S1", "put", strike, 5)

	if _, err := BuyOption(ctx, pool, room, guest.ID, callID, 3, t0); err != nil {
		t.Fatalf("buy call: %v", err)
	}
	if _, err := BuyOption(ctx, pool, room, guest.ID, putID, 2, t0); err != nil {
		t.Fatalf("buy put: %v", err)
	}
	cashAfterBuys := cashOf(t, pool, room.ID, guest.ID)

	at5 := t0.Add(5*61*time.Second + time.Second)
	if _, _, err := SettleRoom(ctx, pool, room, at5); err != nil {
		t.Fatal(err)
	}

	// ITM call pays max(S−K, 0) × contracts at the expiry day's close; the
	// OTM put expires worthless (amount 0). Both positions are gone.
	payoff := close5 - strike
	wantCash := cashAfterBuys + int64(math.Round(payoff*3*100))
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != wantCash {
		t.Fatalf("cash after expiry = %d, want %d", cash, wantCash)
	}
	var nPos int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM option_positions WHERE room_id = $1 AND user_id = $2`,
		room.ID, guest.ID).Scan(&nPos); err != nil {
		t.Fatal(err)
	}
	if nPos != 0 {
		t.Fatalf("positions after expiry = %d, want 0", nPos)
	}
	rows, err := pool.Query(ctx, `
		SELECT o.kind, t.contracts, t.amount_cents
		FROM option_trades t JOIN room_options o ON o.id = t.option_id
		WHERE t.room_id = $1 AND t.action = 'expiry' ORDER BY o.kind`, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	expiries := map[string]int64{}
	for rows.Next() {
		var kind string
		var contracts float64
		var amount int64
		if err := rows.Scan(&kind, &contracts, &amount); err != nil {
			t.Fatal(err)
		}
		expiries[kind] = amount
	}
	if len(expiries) != 2 {
		t.Fatalf("expiry trade rows = %v, want call+put", expiries)
	}
	if expiries["call"] != int64(math.Round(payoff*3*100)) {
		t.Fatalf("call payoff = %d, want %d", expiries["call"], int64(math.Round(payoff*3*100)))
	}
	if expiries["put"] != 0 {
		t.Fatalf("put payoff = %d, want 0 (OTM)", expiries["put"])
	}

	// Re-settling is a no-op: positions are deleted once settled.
	if _, _, err := SettleRoom(ctx, pool, room, at5); err != nil {
		t.Fatal(err)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != wantCash {
		t.Fatalf("cash after re-settle = %d, want %d", cash, wantCash)
	}
	var nExpiry int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM option_trades WHERE room_id = $1 AND action = 'expiry'`,
		room.ID).Scan(&nExpiry); err != nil {
		t.Fatal(err)
	}
	if nExpiry != 2 {
		t.Fatalf("expiry rows after re-settle = %d, want 2", nExpiry)
	}
}

func TestAssetsCentsIncludesOptionMTM(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)
	optID := firstOptionID(t, pool, room.ID, "call", 10)

	fill, err := BuyOption(ctx, pool, room, guest.ID, optID, 2, t0)
	if err != nil {
		t.Fatal(err)
	}
	// Same-day mark == premium paid, so net assets are exactly the initial
	// cash (premium moved from cash into the option's value).
	assets, err := assetsCents(ctx, pool, room.ID, guest.ID, 0, "close")
	if err != nil {
		t.Fatal(err)
	}
	if assets != InitialCashCents {
		t.Fatalf("assets = %d, want %d (cash + option MTM)", assets, InitialCashCents)
	}
	if assets != fill.CashCents+fill.AmountCents {
		t.Fatalf("assets = %d, want cash %d + premium %d", assets, fill.CashCents, fill.AmountCents)
	}
}
