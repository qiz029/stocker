package engine

import (
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestFidelityHoldsAcrossSeeds(t *testing.T) {
	sc := scenario.Synthetic()
	for s := uint64(0); s < 30; s++ {
		evs := GenerateShockTimeline(sc, s)
		states := EvolveFactorStates(sc, evs)
		prices := SynthesizePrices(sc, states, s)
		if err := VerifyFidelity(sc, prices); err != nil {
			t.Fatalf("seed %d violates fidelity: %v", s, err)
		}
	}
}

func TestFidelityRejectsGarbage(t *testing.T) {
	sc := scenario.Synthetic()
	// 基线取反(镜像)必然违反相关性/方向条款
	bad := map[string][]scenario.OHLC{}
	for id, p := range sc.Baseline {
		rev := make([]scenario.OHLC, len(p))
		for i := range p {
			rev[i] = p[len(p)-1-i]
		}
		bad[id] = rev
	}
	if err := VerifyFidelity(sc, bad); err == nil {
		t.Fatal("reversed prices should fail fidelity")
	}
}
