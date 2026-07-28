package pipeline

// Fidelity diagnostics for the dotcom-2000 scenario. Unlike the release gate
// (TestDotcomFidelityAcrossSeeds), which stops at the FIRST VerifyFidelity
// error per seed, this sweep checks EVERY instrument on EVERY seed and prints
// the complete failure map plus a plateau probe (top-5 highest/lowest baseline
// closes) for each instrument with extremum failures. It was this sweep that
// revealed the extremum failures masked by first-error reporting in the
// round-1/2 diagnosis (see task-5-report.md, round 3). Run with:
//
//	STOCKER_FIDELITY_DIAG=1 go test ./internal/pipeline/ -run TestDiagFidelity -count=1 -v
//
// The test itself never fails — it is an instrument, not a gate.

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

func lr(p []scenario.OHLC) []float64 {
	r := make([]float64, len(p)-1)
	for i := 1; i < len(p); i++ {
		r[i-1] = math.Log(p[i].Close / p[i-1].Close)
	}
	return r
}

func pear(a, b []float64) float64 {
	n := float64(len(a))
	var sa, sb, saa, sbb, sab float64
	for i := range a {
		sa += a[i]
		sb += b[i]
		saa += a[i] * a[i]
		sbb += b[i] * b[i]
		sab += a[i] * b[i]
	}
	return (sab - sa*sb/n) / math.Sqrt((saa-sa*sa/n)*(sbb-sb*sb/n))
}

func argExt(p []scenario.OHLC, max bool) int {
	best := 0
	for i := range p {
		if (max && p[i].Close > p[best].Close) || (!max && p[i].Close < p[best].Close) {
			best = i
		}
	}
	return best
}

func TestDiagFidelity(t *testing.T) {
	if os.Getenv("STOCKER_FIDELITY_DIAG") == "" {
		t.Skip("diagnostic sweep; set STOCKER_FIDELITY_DIAG=1 to run")
	}
	sc, err := BuildScenario("dotcom-2000")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("== calibration ==")
	for _, inst := range sc.Instruments {
		fmt.Printf("%s idioScale=%.4f bMKT=%.4f\n", inst.ID, inst.IdioScale, inst.Beta["MKT"])
	}
	worstCorr := map[string]float64{}
	extFail := map[string]int{}
	corrFail := map[string]int{}
	for seed := uint64(1); seed <= 12; seed++ {
		shocks := engine.ExpandClusters(sc, seed, engine.GenerateShockTimeline(sc, seed))
		states, err := engine.EvolveFactorStates(sc, shocks)
		if err != nil {
			t.Fatal(err)
		}
		prices := engine.SynthesizePrices(sc, states, seed)
		for _, inst := range sc.Instruments {
			base, disp := sc.Baseline[inst.ID], prices[inst.ID]
			corr := pear(lr(base), lr(disp))
			if w, ok := worstCorr[inst.ID]; !ok || corr < w {
				worstCorr[inst.ID] = corr
			}
			if corr < 0.85 {
				corrFail[inst.ID]++
				fmt.Printf("seed=%d %s CORR %.4f\n", seed, inst.ID, corr)
			}
			baseNet := math.Log(base[len(base)-1].Close / base[0].Close)
			if math.Abs(baseNet) >= 0.18 { // mirrors engine minNetLogForDirection (final round)
				bDir := base[len(base)-1].Close >= base[0].Close
				dDir := disp[len(disp)-1].Close >= disp[0].Close
				if bDir != dDir {
					dispNet := math.Log(disp[len(disp)-1].Close / disp[0].Close)
					fmt.Printf("seed=%d %s DIR baseNet=%.4f dispNet=%.4f\n", seed, inst.ID, baseNet, dispNet)
				}
			}
			for _, mx := range []bool{true, false} {
				bd, dd := argExt(base, mx), argExt(disp, mx)
				if d := bd - dd; d > 10 || d < -10 {
					// Mirror engine.twinExtremumOK's dual condition and
					// report the two gaps it measures: aGap = how far the
					// baseline at the display's extremum day sits from the
					// baseline's true extreme; bGap = how far the display's
					// best value near the baseline extremum day sits from
					// the display's own global extreme.
					lo, hi := bd-10, bd+10
					if lo < 0 {
						lo = 0
					}
					if hi > len(disp)-1 {
						hi = len(disp) - 1
					}
					nb := disp[lo].Close
					for i := lo + 1; i <= hi; i++ {
						if (mx && disp[i].Close > nb) || (!mx && disp[i].Close < nb) {
							nb = disp[i].Close
						}
					}
					var aGap, bGap float64
					if mx {
						aGap = 1 - base[dd].Close/base[bd].Close
						bGap = 1 - nb/disp[dd].Close
					} else {
						aGap = base[dd].Close/base[bd].Close - 1
						bGap = nb/disp[dd].Close - 1
					}
					const tol = 0.18 // mirrors engine extremumValueTol (round 5)
					if aGap > tol || bGap > tol {
						extFail[inst.ID]++
						fmt.Printf("seed=%d %s EXT max=%v base=%d disp=%d moved=%d aGap=%.4f bGap=%.4f\n",
							seed, inst.ID, mx, bd, dd, d, aGap, bGap)
					}
				}
			}
		}
	}
	fmt.Println("== worst corr per instrument ==")
	ids := make([]string, 0)
	for _, inst := range sc.Instruments {
		ids = append(ids, inst.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		fmt.Printf("%s worstCorr=%.4f corrFails=%d extFails=%d\n", id, worstCorr[id], corrFail[id], extFail[id])
	}
	// Plateau probe for any instrument with extremum failures: top-5 closes.
	for _, id := range ids {
		if extFail[id] == 0 {
			continue
		}
		base := sc.Baseline[id]
		type dc struct {
			day int
			c   float64
		}
		all := make([]dc, len(base))
		for i, b := range base {
			all[i] = dc{i, b.Close}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].c < all[j].c })
		fmt.Printf("%s lowest5: ", id)
		for i := 0; i < 5; i++ {
			fmt.Printf("(d%d %.3f) ", all[i].day, all[i].c)
		}
		sort.Slice(all, func(i, j int) bool { return all[i].c > all[j].c })
		fmt.Printf(" highest5: ")
		for i := 0; i < 5; i++ {
			fmt.Printf("(d%d %.3f) ", all[i].day, all[i].c)
		}
		fmt.Println()
	}
}
