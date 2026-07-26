package engine

import (
	"math"
	"sort"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// SynthesizePrices produces the per-room display prices:
// display = baseline × exp(clamp(Σ β·X + ε)).
func SynthesizePrices(sc *scenario.Scenario, states [][]float64, seed uint64) map[string][]scenario.OHLC {
	ids := sc.FactorIDs()
	idx := make(map[string]int, len(ids))
	for i, id := range ids {
		idx[id] = i
	}
	out := make(map[string][]scenario.OHLC, len(sc.Instruments))
	for _, inst := range sc.Instruments {
		rng := Stream(seed, "eps", inst.ID)
		prices := make([]scenario.OHLC, sc.Days)
		// Sort factors once per instrument (inst.Beta doesn't change across
		// days) to ensure deterministic iteration order.
		factors := make([]string, 0, len(inst.Beta))
		for f := range inst.Beta {
			factors = append(factors, f)
		}
		sort.Strings(factors)
		for d := 0; d < sc.Days; d++ {
			x := rng.NormFloat64() * epsSigma
			for _, f := range factors {
				beta := inst.Beta[f]
				x += beta * states[d][idx[f]]
			}
			x = math.Max(-clampX, math.Min(clampX, x))
			m := math.Exp(x)
			b := sc.Baseline[inst.ID][d]
			prices[d] = scenario.OHLC{
				Open: b.Open * m, High: b.High * m, Low: b.Low * m, Close: b.Close * m,
			}
		}
		out[inst.ID] = prices
	}
	return out
}
