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
	rng := rand.New(rand.NewPCG(7, 7)) // fixed: determinism required by tests
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
			drift := 0.0005
			if techy {
				switch {
				case d < 150:
					drift = 0.006 // 泡沫
				case d == 150:
					drift = -0.12 // scripted crash-start gap-down
				case d < 220:
					drift = -0.013 // 崩盘
				default:
					drift = 0.0
				}
			}
			ret := drift + rng.NormFloat64()*0.015
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
