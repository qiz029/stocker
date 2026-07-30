package store

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAnnualRateFormula(t *testing.T) {
	// Fewer than 2 return observations → base rate.
	if r := AnnualRate(nil); r != 0.03 {
		t.Fatalf("empty closes: rate = %v, want 0.03", r)
	}
	if r := AnnualRate([]float64{100, 101}); r != 0.03 {
		t.Fatalf("single return: rate = %v, want 0.03", r)
	}
	// Flat prices → zero vol → base rate.
	flat := make([]float64, 25)
	for i := range flat {
		flat[i] = 100
	}
	if r := AnnualRate(flat); r != 0.03 {
		t.Fatalf("flat closes: rate = %v, want 0.03", r)
	}
	// ±1% daily alternation: std ≈ 1% → vol ≈ 0.1587 → rate ≈ 0.1252.
	vol := make([]float64, 22)
	vol[0] = 100
	for i := 1; i < len(vol); i++ {
		if i%2 == 1 {
			vol[i] = vol[i-1] * 1.01
		} else {
			vol[i] = vol[i-1] / 1.01
		}
	}
	r := AnnualRate(vol)
	want := 0.03 + 0.6*0.01*math.Sqrt(252)
	if math.Abs(r-want) > 0.005 {
		t.Fatalf("volatile closes: rate = %v, want ≈%v", r, want)
	}
	// Wild swings clamp at 60%.
	wild := make([]float64, 22)
	wild[0] = 100
	for i := 1; i < len(wild); i++ {
		if i%2 == 1 {
			wild[i] = wild[i-1] * 2
		} else {
			wild[i] = wild[i-1] / 2
		}
	}
	if r := AnnualRate(wild); r != 0.60 {
		t.Fatalf("wild closes: rate = %v, want clamp 0.60", r)
	}
}

func debtOf(t *testing.T, pool *pgxpool.Pool, roomID, userID int64) (int64, *int, *int) {
	t.Helper()
	var debt int64
	var through, bankrupt *int
	if err := pool.QueryRow(context.Background(), `
		SELECT debt_cents, interest_through_day, bankrupt_day
		FROM room_players WHERE room_id = $1 AND user_id = $2`,
		roomID, userID).Scan(&debt, &through, &bankrupt); err != nil {
		t.Fatal(err)
	}
	return debt, through, bankrupt
}

func TestBorrowRepayGuards(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Bad amounts.
	if _, err := Borrow(ctx, pool, room, guest.ID, 0, t0); !errors.Is(err, ErrBadLoanAmount) {
		t.Fatalf("borrow 0: %v", err)
	}
	if _, err := Repay(ctx, pool, room, guest.ID, -5, t0); !errors.Is(err, ErrBadLoanAmount) {
		t.Fatalf("repay -5: %v", err)
	}
	// Repay with no debt.
	if _, err := Repay(ctx, pool, room, guest.ID, 100, t0); !errors.Is(err, ErrBadLoanAmount) {
		t.Fatalf("repay without debt: %v", err)
	}

	// Borrow up to the cap is allowed; one cent more is not.
	st, err := Borrow(ctx, pool, room, guest.ID, MaxDebtCents, t0)
	if err != nil {
		t.Fatalf("borrow to cap: %v", err)
	}
	if st.DebtCents != MaxDebtCents || st.CashCents != InitialCashCents+MaxDebtCents {
		t.Fatalf("state after max borrow: %+v", st)
	}
	if _, err := Borrow(ctx, pool, room, guest.ID, 1, t0); !errors.Is(err, ErrDebtCapExceeded) {
		t.Fatalf("borrow past cap: %v", err)
	}
	debt, through, _ := debtOf(t, pool, room.ID, guest.ID)
	if debt != MaxDebtCents || through == nil || *through != 0 {
		t.Fatalf("debt=%d through=%v, want %d/0", debt, through, MaxDebtCents)
	}

	// loan_txns recorded the borrow.
	var kind string
	var day int
	if err := pool.QueryRow(ctx, `
		SELECT kind, day FROM loan_txns WHERE room_id = $1 AND user_id = $2`,
		room.ID, guest.ID).Scan(&kind, &day); err != nil {
		t.Fatal(err)
	}
	if kind != "borrow" || day != 0 {
		t.Fatalf("loan_txn: %s day %d, want borrow day 0", kind, day)
	}

	// Repay more than the debt; then repay the full debt (same day, no
	// interest yet) and confirm a fresh loan restarts the interest clock.
	if _, err := Repay(ctx, pool, room, guest.ID, MaxDebtCents+1, t0); !errors.Is(err, ErrBadLoanAmount) {
		t.Fatalf("over-repay: %v", err)
	}
	st, err = Repay(ctx, pool, room, guest.ID, MaxDebtCents, t0)
	if err != nil {
		t.Fatalf("full repay: %v", err)
	}
	if st.DebtCents != 0 || st.CashCents != InitialCashCents {
		t.Fatalf("state after full repay: %+v", st)
	}

	at3 := t0.Add(3*61*time.Second + time.Second) // day 3
	if _, err := Borrow(ctx, pool, room, guest.ID, 1_000_000, at3); err != nil {
		t.Fatalf("re-borrow: %v", err)
	}
	_, through, _ = debtOf(t, pool, room.ID, guest.ID)
	if through == nil || *through != 3 {
		t.Fatalf("re-borrow interest clock = %v, want day 3", through)
	}
}

func TestBorrowInLobbyAndAfterEnd(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Borrow(ctx, pool, room, host.ID, 100, time.Now()); !errors.Is(err, ErrRoomNotRunning) {
		t.Fatalf("borrow in lobby: %v", err)
	}
	t0 := time.Now().Truncate(time.Second)
	room, err = StartRoom(ctx, pool, room.ID, host.ID, t0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Borrow(ctx, pool, room, host.ID, 100, t0.Add(400*60*time.Second)); !errors.Is(err, ErrRoomEnded) {
		t.Fatalf("borrow after end: %v", err)
	}
}

// expectedDebt compounds principal day-by-day exactly like SettleTx,
// reading the same room_prices through the exported AnnualRate.
func expectedDebt(t *testing.T, pool *pgxpool.Pool, roomID int64, principal int64, fromDay, toDay int) int64 {
	t.Helper()
	ctx := context.Background()
	debt := principal
	for d := fromDay + 1; d <= toDay; d++ {
		rows, err := pool.Query(ctx, `
			SELECT close FROM room_prices
			WHERE room_id = $1 AND instrument_id = 'S1' AND day >= $2 AND day < $3
			ORDER BY day`, roomID, d-21, d)
		if err != nil {
			t.Fatal(err)
		}
		var closes []float64
		for rows.Next() {
			var c float64
			if err := rows.Scan(&c); err != nil {
				t.Fatal(err)
			}
			closes = append(closes, c)
		}
		rows.Close()
		rate := AnnualRate(closes)
		debt = int64(math.Round(float64(debt) * (1 + rate/252)))
	}
	return debt
}

func TestInterestCompoundsDaily(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	if _, err := Borrow(ctx, pool, room, guest.ID, 1_000_000, t0); err != nil {
		t.Fatalf("borrow: %v", err)
	}
	at5 := t0.Add(5*61*time.Second + time.Second) // day 5
	if _, _, err := SettleRoom(ctx, pool, room, at5); err != nil {
		t.Fatal(err)
	}
	want := expectedDebt(t, pool, room.ID, 1_000_000, 0, 5)
	debt, through, bankrupt := debtOf(t, pool, room.ID, guest.ID)
	if debt != want {
		t.Fatalf("debt = %d, want %d (compounded days 1-5)", debt, want)
	}
	if debt <= 1_000_000 {
		t.Fatalf("debt did not grow: %d", debt)
	}
	if through == nil || *through != 5 || bankrupt != nil {
		t.Fatalf("through=%v bankrupt=%v, want 5/nil", through, bankrupt)
	}

	// Host never borrowed: untouched, no interest clock.
	debt, through, _ = debtOf(t, pool, room.ID, room.HostUserID)
	if debt != 0 || through != nil {
		t.Fatalf("never-borrowed host: debt=%d through=%v", debt, through)
	}
}

func TestBankruptcyAtCapCrossing(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Borrow exactly to the cap, then park $10k in a buy. Fills run
	// before accrual within a settle, so this order EXECUTES at the
	// bankrupting settle (day-1 open) rather than being refunded.
	if _, err := Borrow(ctx, pool, room, guest.ID, MaxDebtCents, t0); err != nil {
		t.Fatalf("borrow: %v", err)
	}
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 1_000_000}); err != nil {
		t.Fatalf("place order: %v", err)
	}

	// Day 1: one day of interest pushes debt past the cap → bankrupt.
	at1 := t0.Add(61 * time.Second)
	if _, _, err := SettleRoom(ctx, pool, room, at1); err != nil {
		t.Fatal(err)
	}
	debt, through, bankrupt := debtOf(t, pool, room.ID, guest.ID)
	if debt <= MaxDebtCents {
		t.Fatalf("debt = %d, want > cap %d", debt, MaxDebtCents)
	}
	if bankrupt == nil || *bankrupt != 1 || through == nil || *through != 1 {
		t.Fatalf("bankrupt=%v through=%v, want 1/1", bankrupt, through)
	}

	// The buy filled (no refund) and the position survives bankruptcy
	// untouched.
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != InitialCashCents+MaxDebtCents-1_000_000 {
		t.Fatalf("cash after bankruptcy = %d, want %d", cash, InitialCashCents+MaxDebtCents-1_000_000)
	}
	var shares float64
	if err := pool.QueryRow(ctx, `
		SELECT shares FROM positions WHERE room_id = $1 AND user_id = $2 AND instrument_id = 'S1'`,
		room.ID, guest.ID).Scan(&shares); err != nil {
		t.Fatal(err)
	}
	if shares <= 0 {
		t.Fatalf("position lost on bankruptcy: shares = %v", shares)
	}
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM orders WHERE room_id = $1 AND user_id = $2`,
		room.ID, guest.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "filled" {
		t.Fatalf("order status = %s, want filled", status)
	}

	// The bankruptcy is announced once.
	var n int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM room_events WHERE room_id = $1 AND kind = 'bankrupt'`,
		room.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("bankrupt events = %d, want 1", n)
	}

	// Everything money-moving is refused now; chat stays open (not tested here).
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, at1, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 100}); !errors.Is(err, ErrPlayerBankrupt) {
		t.Fatalf("bankrupt order: %v", err)
	}
	if _, err := Borrow(ctx, pool, room, guest.ID, 100, at1); !errors.Is(err, ErrPlayerBankrupt) {
		t.Fatalf("bankrupt borrow: %v", err)
	}
	if _, err := Repay(ctx, pool, room, guest.ID, 100, at1); !errors.Is(err, ErrPlayerBankrupt) {
		t.Fatalf("bankrupt repay: %v", err)
	}

	// Settling again changes nothing (idempotent): same debt, still one event.
	if _, _, err := SettleRoom(ctx, pool, room, at1); err != nil {
		t.Fatal(err)
	}
	debt2, _, _ := debtOf(t, pool, room.ID, guest.ID)
	if debt2 != debt {
		t.Fatalf("debt changed on re-settle: %d → %d", debt, debt2)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM room_events WHERE room_id = $1 AND kind = 'bankrupt'`,
		room.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("bankrupt events after re-settle = %d, want 1", n)
	}
}
