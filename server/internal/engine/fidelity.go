package engine

import (
	"fmt"
	"math"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

const (
	minReturnCorr = 0.85
	extremumSlack = 10
)

func logReturns(p []scenario.OHLC) []float64 {
	r := make([]float64, len(p)-1)
	for i := 1; i < len(p); i++ {
		r[i-1] = math.Log(p[i].Close / p[i-1].Close)
	}
	return r
}

func pearson(a, b []float64) float64 {
	n := float64(len(a))
	var sa, sb, saa, sbb, sab float64
	for i := range a {
		sa += a[i]
		sb += b[i]
		saa += a[i] * a[i]
		sbb += b[i] * b[i]
		sab += a[i] * b[i]
	}
	cov := sab - sa*sb/n
	va := saa - sa*sa/n
	vb := sbb - sb*sb/n
	return cov / math.Sqrt(va*vb)
}

func argExtremum(p []scenario.OHLC, max bool) int {
	best := 0
	for i := range p {
		if (max && p[i].Close > p[best].Close) || (!max && p[i].Close < p[best].Close) {
			best = i
		}
	}
	return best
}

// VerifyFidelity enforces spec §4.6: perturbed prices must preserve the
// broad historical narrative. Also used by the data pipeline (plan 4) as a
// scenario acceptance gate.
func VerifyFidelity(sc *scenario.Scenario, prices map[string][]scenario.OHLC) error {
	for _, inst := range sc.Instruments {
		base := sc.Baseline[inst.ID]
		disp, ok := prices[inst.ID]
		if !ok {
			return fmt.Errorf("%s: missing prices", inst.ID)
		}
		if len(disp) != len(base) {
			return fmt.Errorf("%s: length mismatch (base=%d, disp=%d)", inst.ID, len(base), len(disp))
		}
		if len(base) < 2 {
			return fmt.Errorf("%s: too few days (%d) to verify fidelity", inst.ID, len(base))
		}
		corr := pearson(logReturns(base), logReturns(disp))
		if math.IsNaN(corr) {
			return fmt.Errorf("%s: return correlation is NaN (degenerate series)", inst.ID)
		}
		if corr < minReturnCorr {
			return fmt.Errorf("%s: return correlation %.3f < %.2f", inst.ID, corr, minReturnCorr)
		}
		bDir := base[len(base)-1].Close >= base[0].Close
		dDir := disp[len(disp)-1].Close >= disp[0].Close
		if bDir != dDir {
			return fmt.Errorf("%s: cumulative direction flipped", inst.ID)
		}
		for _, max := range []bool{true, false} {
			bd, dd := argExtremum(base, max), argExtremum(disp, max)
			if abs(bd-dd) > extremumSlack {
				return fmt.Errorf("%s: extremum moved %d days (max=%v)", inst.ID, abs(bd-dd), max)
			}
		}
	}
	return nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
