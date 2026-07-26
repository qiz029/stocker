package engine

import (
	"math"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestPricesNoShocksTrackBaseline(t *testing.T) {
	sc := scenario.Synthetic()
	states := EvolveFactorStates(sc, nil)
	prices := SynthesizePrices(sc, states, 42)
	for _, inst := range sc.Instruments {
		for d := 0; d < sc.Days; d++ {
			ratio := prices[inst.ID][d].Close / sc.Baseline[inst.ID][d].Close
			// 仅剩 ε 噪声, |logratio| 应远小于 5σ=2%
			if math.Abs(math.Log(ratio)) > 0.02 {
				t.Fatalf("%s day %d deviates %f without shocks", inst.ID, d, ratio)
			}
		}
	}
}

func TestPricesClampBound(t *testing.T) {
	sc := scenario.Synthetic()
	// 构造巨量冲击, 验证 clamp
	evs := []NewsEvent{}
	for d := 0; d < 50; d++ {
		evs = append(evs, NewsEvent{Day: d, Track: TrackImpact,
			TrueShock: map[string]float64{"MKT": 0.5}})
	}
	states := EvolveFactorStates(sc, evs)
	prices := SynthesizePrices(sc, states, 42)
	for _, inst := range sc.Instruments {
		for d := 0; d < sc.Days; d++ {
			ratio := prices[inst.ID][d].Close / sc.Baseline[inst.ID][d].Close
			if ratio > math.Exp(clampX)+1e-9 {
				t.Fatalf("clamp violated: ratio %f", ratio)
			}
		}
	}
}

func TestPricesOHLCConsistent(t *testing.T) {
	sc := scenario.Synthetic()
	states := EvolveFactorStates(sc, GenerateShockTimeline(sc, 42))
	prices := SynthesizePrices(sc, states, 42)
	for _, inst := range sc.Instruments {
		for d, p := range prices[inst.ID] {
			b := sc.Baseline[inst.ID][d]
			// 四值同乘一个系数 → 比例关系保持
			r1, r2 := p.Open/b.Open, p.Close/b.Close
			if math.Abs(r1-r2) > 1e-9 {
				t.Fatalf("%s day %d: OHLC not uniformly scaled", inst.ID, d)
			}
			if p.Low > p.Open || p.High < p.Close || p.Low <= 0 {
				t.Fatalf("%s day %d: invalid OHLC", inst.ID, d)
			}
		}
	}
}
