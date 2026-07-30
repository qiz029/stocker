package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// mkBareRoom inserts a rooms row directly (no world generation) so tests
// can construct room_news / room_prices by hand.
func mkBareRoom(t *testing.T, pool *pgxpool.Pool, sc *scenario.Scenario, hostID int64, code string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO rooms (invite_code, scenario_id, days, seed, day_duration_secs, host_user_id)
		VALUES ($1, $2, $3, 1, 60, $4) RETURNING id`,
		code, sc.ID, sc.Days, hostID).Scan(&id)
	if err != nil {
		t.Fatalf("mkBareRoom: %v", err)
	}
	return id
}

func insertCloses(t *testing.T, pool *pgxpool.Pool, roomID int64, inst string, closes []float64) {
	t.Helper()
	ctx := context.Background()
	for d, c := range closes {
		if _, err := pool.Exec(ctx, `
			INSERT INTO room_prices (room_id, instrument_id, day, open, high, low, close)
			VALUES ($1, $2, $3, $4, $4, $4, $4)`, roomID, inst, d, c); err != nil {
			t.Fatalf("insert price %s day %d: %v", inst, d, err)
		}
	}
}

func insertReport(t *testing.T, pool *pgxpool.Pool, roomID int64, day int, media, track string, report map[string]float64) {
	t.Helper()
	var rs any
	if report != nil {
		b, _ := json.Marshal(report)
		rs = string(b)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO room_news (room_id, day, media_id, headline, track, report_shock)
		VALUES ($1, $2, $3, 'h', $4, $5)`, roomID, day, media, track, rs); err != nil {
		t.Fatalf("insert news: %v", err)
	}
}

// accuracyPrices: S1 drifts up, S6 drifts down, everything else flat.
// Synthetic: MarketProxy = S1, TECH = S1..S5, OLD = S6..S8.
func accuracyPrices(t *testing.T, pool *pgxpool.Pool, roomID int64) {
	t.Helper()
	up := []float64{100, 100, 101, 103, 106, 106}
	down := []float64{100, 100, 99, 97, 94, 94}
	flat := []float64{100, 100, 100, 100, 100, 100}
	for i := 1; i <= 8; i++ {
		id := fmt.Sprintf("S%d", i)
		switch id {
		case "S1":
			insertCloses(t, pool, roomID, id, up)
		case "S6":
			insertCloses(t, pool, roomID, id, down)
		default:
			insertCloses(t, pool, roomID, id, flat)
		}
	}
}

func TestMediaAccuracy(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	roomID := mkBareRoom(t, pool, sc, host.ID, "ACC1")
	accuracyPrices(t, pool, roomID)

	const curDay = 5                                                                         // observable reports: day <= 3
	insertReport(t, pool, roomID, 1, "wire", "impact", map[string]float64{"IDIO:S6": -0.02}) // S6 falls → hit
	insertReport(t, pool, roomID, 1, "wire", "impact", map[string]float64{"IDIO:S1": 0.02})  // S1 rises → hit
	insertReport(t, pool, roomID, 1, "paper", "impact", map[string]float64{"MKT": -0.02})    // proxy S1 rises → miss
	insertReport(t, pool, roomID, 1, "paper", "impact", map[string]float64{"TECH": 0.02})    // S1 up, S2-S5 flat → hit
	insertReport(t, pool, roomID, 4, "wire", "impact", map[string]float64{"IDIO:S1": 0.02})  // too recent → not counted
	insertReport(t, pool, roomID, 1, "wire", "noise", nil)                                   // noise → not counted
	insertReport(t, pool, roomID, 1, "wire", "impact", nil)                                  // no report → not counted

	acc, err := MediaAccuracy(ctx, pool, roomID, curDay)
	if err != nil {
		t.Fatal(err)
	}
	if got := acc["wire"]; got.Reports != 2 || got.Hits != 2 {
		t.Fatalf("wire = %+v, want {2 2}", got)
	}
	if got := acc["paper"]; got.Reports != 2 || got.Hits != 1 {
		t.Fatalf("paper = %+v, want {2 1}", got)
	}
	if len(acc) != 2 {
		t.Fatalf("unexpected media keys: %+v", acc)
	}

	// Before any outcome window exists there is nothing to score.
	early, err := MediaAccuracy(ctx, pool, roomID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(early) != 0 {
		t.Fatalf("curDay=1 accuracy = %+v, want empty", early)
	}
}

func TestMediaAccuracyBasketFallback(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := scenario.Synthetic()
	sc.ID = "synthetic-basket"
	sc.MarketProxy = "" // force the equal-weighted basket path
	if err := SaveScenario(ctx, pool, sc); err != nil {
		t.Fatal(err)
	}
	roomID := mkBareRoom(t, pool, sc, host.ID, "ACC2")
	accuracyPrices(t, pool, roomID)

	// Basket mean of ln(1.06) + ln(0.94) + 6×0 over 8 names is slightly
	// negative: a bearish MKT report hits, a bullish one misses.
	insertReport(t, pool, roomID, 1, "wire", "impact", map[string]float64{"MKT": 0.02})
	insertReport(t, pool, roomID, 1, "wire", "impact", map[string]float64{"MKT": -0.02})

	acc, err := MediaAccuracy(ctx, pool, roomID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got := acc["wire"]; got.Reports != 2 || got.Hits != 1 {
		t.Fatalf("basket wire = %+v, want {2 1}", got)
	}
}
