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

func TestFidelityRejectsMissingInstrument(t *testing.T) {
	sc := scenario.Synthetic()
	prices := map[string][]scenario.OHLC{}
	for id, p := range sc.Baseline {
		prices[id] = p
	}
	// 移除一个标的的价格, 不应 panic, 必须返回描述性错误
	delete(prices, sc.Instruments[0].ID)
	err := VerifyFidelity(sc, prices)
	if err == nil {
		t.Fatal("missing instrument prices should fail fidelity, got nil error")
	}
}

func TestFidelityRejectsLengthMismatch(t *testing.T) {
	sc := scenario.Synthetic()
	prices := map[string][]scenario.OHLC{}
	for id, p := range sc.Baseline {
		prices[id] = p
	}
	// 截断一个标的的价格序列, 不应 panic, 必须返回描述性错误
	id := sc.Instruments[0].ID
	prices[id] = prices[id][:len(prices[id])-5]
	err := VerifyFidelity(sc, prices)
	if err == nil {
		t.Fatal("length-mismatched prices should fail fidelity, got nil error")
	}
}
