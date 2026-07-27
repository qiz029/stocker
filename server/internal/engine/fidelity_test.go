package engine

import (
	"math"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestFidelityHoldsAcrossSeeds(t *testing.T) {
	sc := scenario.Synthetic()
	for s := uint64(0); s < 30; s++ {
		evs := GenerateShockTimeline(sc, s)
		states, err := EvolveFactorStates(sc, evs)
		if err != nil {
			t.Fatalf("seed %d: %v", s, err)
		}
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

func TestFidelityDirectionExemptForNearFlat(t *testing.T) {
	// A baseline with ~zero net move must not fail on direction sign alone.
	sc := &scenario.Scenario{
		ID: "flat", Days: 260,
		Factors:     []scenario.Factor{{ID: "MKT", Kind: scenario.KindMarket}},
		Instruments: []scenario.Instrument{{ID: "F1", Beta: map[string]float64{"MKT": 1}, IdioScale: 1}},
		Baseline:    map[string][]scenario.OHLC{},
	}
	base := make([]scenario.OHLC, sc.Days)
	prices := make([]scenario.OHLC, sc.Days)
	for d := 0; d < sc.Days; d++ {
		// A single smooth oscillation (one full sine period across the
		// window) so the series has one unique global max and one unique
		// global min — no repeated-tie plateaus for argExtremum to latch
		// onto. base[0] == base[Days-1] exactly, so net ≈ 0 (« 0.10).
		b := 100 + 2*math.Sin(2*math.Pi*float64(d)/float64(sc.Days-1))
		base[d] = scenario.OHLC{Open: b, High: b + 1, Low: b - 1, Close: b}
		prices[d] = scenario.OHLC{Open: b, High: b + 1, Low: b - 1, Close: b}
	}
	// Flip the display's net direction: end 1% below start while tracking base.
	for d := range prices {
		f := 1.0 - 0.02*float64(d)/float64(sc.Days-1)
		prices[d].Close = base[d].Close * f
		prices[d].Open, prices[d].High, prices[d].Low = prices[d].Close, prices[d].Close+1, prices[d].Close-1
	}
	sc.Baseline["F1"] = base
	if err := VerifyFidelity(sc, map[string][]scenario.OHLC{"F1": prices}); err != nil {
		t.Fatalf("near-flat direction flip must be exempt, got: %v", err)
	}
}
