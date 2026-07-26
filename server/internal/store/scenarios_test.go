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
