package pipeline

import (
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
)

// The plan-1 fidelity gate must hold for the real scenario across seeds —
// this is the release gate for the whole pipeline (spec §4.6).
func TestDotcomFidelityAcrossSeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-seed world generation is slow")
	}
	sc, err := BuildScenario("dotcom-2000")
	if err != nil {
		t.Fatal(err)
	}
	for seed := uint64(1); seed <= 12; seed++ {
		if _, err := engine.GenerateWorld(sc, seed); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}
