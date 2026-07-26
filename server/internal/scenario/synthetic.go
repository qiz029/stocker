package scenario

import (
	"fmt"
	"math"
	"math/rand/v2"
)

// Synthetic builds a deterministic 8-instrument, 300-day test scenario with a
// scripted bubble (day 0-150), crash (150-220) and flat recovery, so engine
// tests can assert fidelity against known structure. Fixed internal seed —
// NOT derived from any room seed; this is test data, not gameplay data.
//
// Extremum-pinning notes (see fidelity.go / TestFidelityHoldsAcrossSeeds):
// argExtremum works on absolute Close level, and for a random-walk-style log
// price, close[d]/close[e] for d > e is exp(sum of returns (e, d]) — it never
// includes ret[e] itself. That means a scripted return on day e only ever
// dominates comparisons where e is the SMALLER index (i.e. it can pin e as a
// floor/ceiling for everything AFTER it), never comparisons where e is the
// larger index. So:
//   - to pin day 0 as the series-wide low, the lever is ret[1] (the earliest
//     index shared by every close[d]/close[0] ratio, d>=1), not ret[0].
//   - to pin day 299 as the series-wide high, the lever is ret[299] itself
//     (the latest index shared by every close[299]/close[d] ratio, d<=298).
//   - to pin day 219 (capitulation) as the series-wide low, BOTH sides are
//     needed: ret[219] dominates every close[d]/close[219] ratio for d<219
//     (the pre-crash side), and ret[220] dominates every close[d]/close[219]
//     ratio for d>219 (the post-crash recovery side) — symmetric to the two
//     points above.
func Synthetic() *Scenario {
	const days = 300
	factors := []Factor{
		{ID: "MKT", Name: "market", Kind: KindMarket},
		{ID: "TECH", Name: "tech sector", Kind: KindSector},
		{ID: "OLD", Name: "old economy", Kind: KindSector},
	}
	sc := &Scenario{
		ID:         "synthetic-v1",
		Days:       days,
		KeyWindows: []KeyWindow{{StartDay: 150, EndDay: 220, Direction: -1}},
		Baseline:   map[string][]OHLC{},
	}
	// Fixed PCG constant (determinism only requires *some* fixed constant —
	// this pair was picked, via an offline search over the same scripted
	// gaps below, for giving every instrument's pinned extremum/direction a
	// comfortable margin against the ±10-day slack instead of a knife-edge
	// one; see task-7-report.md for the search method).
	rng := rand.New(rand.NewPCG(3711, 31))
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("S%d", i+1)
		idioID := "IDIO:" + id
		factors = append(factors, Factor{ID: idioID, Name: id, Kind: KindIdio})
		techy := i < 5 // S1-S5 科技股吃泡沫剧情, S6-S8 传统行业走平
		beta := map[string]float64{"MKT": 1.0, idioID: 1.0}
		if techy {
			beta["TECH"] = 1.2
		} else {
			beta["OLD"] = 0.8
		}
		sc.Instruments = append(sc.Instruments, Instrument{
			ID: id, Alias: "Syn " + id, Desc: "synthetic", Beta: beta,
		})
		prices := make([]OHLC, days)
		logp := math.Log(100)
		for d := 0; d < days; d++ {
			drift := 0.004
			if techy {
				switch {
				case d < 150:
					drift = 0.010 // 泡沫
				case d == 150:
					drift = -0.15 // scripted crash-start gap-down
				case d < 219:
					drift = -0.020 // 崩盘
				case d == 219:
					drift = -0.15 // capitulation gap: pins day 219 as the
					// low against every earlier day (see doc comment above)
				case d == 220:
					drift = 0.50 // recovery anchor: pins day 219 as the low
					// against every later (post-crash) day too
				default:
					drift = 0.008 // 恢复
				}
			} else {
				switch d {
				case 1:
					drift = 0.30 // opening rally: pins day 0 as the
					// series-wide low (day 0's own return can't do this —
					// see doc comment above)
				case 299:
					drift = 0.50 // finishing pop: pins day 299 as the
					// series-wide high
				}
			}
			ret := drift + rng.NormFloat64()*0.035
			open := math.Exp(logp)
			logp += ret
			cls := math.Exp(logp)
			hi := math.Max(open, cls) * (1 + rng.Float64()*0.01)
			lo := math.Min(open, cls) * (1 - rng.Float64()*0.01)
			prices[d] = OHLC{Open: open, High: hi, Low: lo, Close: cls}
		}
		// 归一化：起始 Open 精确为 100
		k := 100 / prices[0].Open
		for d := range prices {
			prices[d].Open *= k
			prices[d].High *= k
			prices[d].Low *= k
			prices[d].Close *= k
		}
		sc.Baseline[id] = prices
	}
	sc.Factors = factors
	return sc
}
