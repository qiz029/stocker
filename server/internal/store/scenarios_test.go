package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestScenarioRoundTrip(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	orig := scenario.Synthetic()

	if err := SaveScenario(ctx, pool, orig); err != nil {
		t.Fatalf("SaveScenario: %v", err)
	}
	// Saving again must not error or duplicate (upsert).
	if err := SaveScenario(ctx, pool, orig); err != nil {
		t.Fatalf("second SaveScenario: %v", err)
	}

	loaded, err := LoadScenario(ctx, pool, orig.ID)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	if !reflect.DeepEqual(orig, loaded) {
		t.Fatalf("round-trip mismatch:\norig:   %+v\nloaded: %+v", orig, loaded)
	}

	if _, err := LoadScenario(ctx, pool, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing scenario: got %v, want ErrNotFound", err)
	}
}

// The determinism gate: a world generated from the DB-loaded scenario must
// be byte-identical (same prices, same news) to one generated from the
// in-memory scenario. float64 survives DOUBLE PRECISION and JSONB exactly
// with Go's encoders, so DeepEqual is the right check.
func TestLoadedScenarioGeneratesIdenticalWorld(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	orig := scenario.Synthetic()
	if err := SaveScenario(ctx, pool, orig); err != nil {
		t.Fatalf("SaveScenario: %v", err)
	}
	loaded, err := LoadScenario(ctx, pool, orig.ID)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}

	w1, err := engine.GenerateWorld(orig, 42)
	if err != nil {
		t.Fatalf("GenerateWorld(orig): %v", err)
	}
	w2, err := engine.GenerateWorld(loaded, 42)
	if err != nil {
		t.Fatalf("GenerateWorld(loaded): %v", err)
	}
	if !reflect.DeepEqual(w1.Prices, w2.Prices) {
		t.Fatal("prices differ between original and DB-loaded scenario")
	}
	if !reflect.DeepEqual(w1.News, w2.News) {
		t.Fatal("news differ between original and DB-loaded scenario")
	}
}

func TestScenarioMetaAndInfos(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	sc := scenario.Synthetic()
	if err := SaveScenario(ctx, pool, sc); err != nil {
		t.Fatal(err)
	}
	if err := SetScenarioMeta(ctx, pool, sc.ID, "合成测试剧本", ""); err != nil {
		t.Fatalf("SetScenarioMeta: %v", err)
	}
	if err := SetScenarioMeta(ctx, pool, "nope", "x", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing scenario: %v", err)
	}
	infos, err := ScenarioInfos(ctx, pool)
	if err != nil || len(infos) != 1 {
		t.Fatalf("infos: %+v err=%v", infos, err)
	}
	if infos[0].ID != sc.ID || infos[0].Name != "合成测试剧本" || infos[0].Days != sc.Days {
		t.Fatalf("info: %+v", infos[0])
	}
}

func TestEraHintRoundTrip(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	sc := scenario.Synthetic()
	sc.EraHint = "测试年代"
	if err := SaveScenario(ctx, pool, sc); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadScenario(ctx, pool, sc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.EraHint != "测试年代" {
		t.Fatalf("EraHint round-trip: got %q, want %q", loaded.EraHint, "测试年代")
	}
}

func TestIdioScaleAndRealNameRoundTrip(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	sc := scenario.Synthetic()
	sc.Instruments[0].IdioScale = 1.7
	sc.Instruments[0].Reconstructed = true
	if err := SaveScenario(ctx, pool, sc); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadScenario(ctx, pool, sc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Instruments[0].IdioScale != 1.7 || !loaded.Instruments[0].Reconstructed {
		t.Fatalf("round-trip lost calibration fields: %+v", loaded.Instruments[0])
	}
	if err := SetInstrumentDisplay(ctx, pool, sc.ID, map[string]InstrumentDisplay{
		"S1": {Alias: "Ridgeline Networks", Desc: "d", RealName: "Cisco Systems", Business: "b", Bull: "u", Bear: "r"},
	}); err != nil {
		t.Fatal(err)
	}
	var realName string
	if err := pool.QueryRow(ctx,
		`SELECT real_name FROM instruments WHERE scenario_id=$1 AND id='S1'`, sc.ID).Scan(&realName); err != nil {
		t.Fatal(err)
	}
	if realName != "Cisco Systems" {
		t.Fatalf("real_name: %q", realName)
	}
}
