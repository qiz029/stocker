package pipeline

import (
	"math"
	"sort"
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
)

// Monte Carlo calibration gate (spec §4.2, ported from the plan-1 Python
// study): across seeds and instruments, the perturbation layer must stay
// a *seasoning* on real history — bounded deviation, zero clamp hits.
func TestPerturbationCalibrationStats(t *testing.T) {
	if testing.Short() {
		t.Skip("Monte Carlo calibration is slow")
	}
	sc, err := BuildScenario()
	if err != nil {
		t.Fatal(err)
	}
	var devs []float64      // |log(display/baseline)| samples
	var extraRets []float64 // daily display-vs-baseline excess log returns
	clampHits := 0
	for seed := uint64(100); seed < 110; seed++ {
		world, err := engine.GenerateWorld(sc, seed)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		for _, inst := range sc.Instruments {
			base := sc.Baseline[inst.ID]
			disp := world.Prices[inst.ID]
			prevDev := 0.0
			for d := 0; d < sc.Days; d++ {
				dev := math.Log(disp[d].Close / base[d].Close)
				devs = append(devs, math.Abs(dev))
				if d > 0 {
					extraRets = append(extraRets, dev-prevDev)
				}
				prevDev = dev
				if math.Abs(dev) > 0.30+1e-9 {
					t.Fatalf("%s day %d: deviation %.4f exceeds clamp", inst.ID, d, dev)
				}
				if math.Abs(dev) > 0.299 {
					clampHits++
				}
			}
		}
	}
	sort.Float64s(devs)
	median := devs[len(devs)/2]
	p95 := devs[int(float64(len(devs))*0.95)]
	var s2 float64
	for _, r := range extraRets {
		s2 += r * r
	}
	extraVol := math.Sqrt(s2 / float64(len(extraRets)))

	// Spec §4.2 targets with tolerance for the per-stock-calibrated real
	// scenario: 额外日波动约 1.6%（区间放宽）; 偏离中位约 3%; p95 约 9%;
	// clamp 零触发（纯保险丝）.
	if extraVol < 0.006 || extraVol > 0.030 {
		t.Errorf("extra daily vol %.4f outside [0.006, 0.030]", extraVol)
	}
	if median < 0.008 || median > 0.08 {
		t.Errorf("median |deviation| %.4f outside [0.008, 0.08]", median)
	}
	if p95 > 0.15 {
		t.Errorf("p95 |deviation| %.4f > 0.15", p95)
	}
	if frac := float64(clampHits) / float64(len(devs)); frac > 0.001 {
		t.Errorf("clamp near-hits fraction %.5f > 0.1%% — clamp is not a fuse anymore", frac)
	}
	t.Logf("calibration: extraVol=%.4f median=%.4f p95=%.4f clampNearHits=%d/%d",
		extraVol, median, p95, clampHits, len(devs))
}
