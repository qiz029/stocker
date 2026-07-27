package pipeline

import (
	"math"
	"reflect"
	"regexp"
	"sort"
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
)

// realNamePattern is the blind-box spot list: real company names must never
// leak into alias, desc, or EraHint (spec, plan-5 Global Constraints).
var realNamePattern = regexp.MustCompile(
	`Microsoft|Cisco|Intel|IBM|Apple|Amazon|Lehman|Goldman|Kodak|Xerox`)

func TestRegistryLists(t *testing.T) {
	ids := Universes()
	if len(ids) == 0 {
		t.Fatal("no scenarios registered")
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("Universes() not sorted: %v", ids)
	}
	found := false
	for _, id := range ids {
		if id == "dotcom-2000" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dotcom-2000 missing from registry: %v", ids)
	}

	u, ok := ByID("dotcom-2000")
	if !ok || u == nil || u.ScenarioID != "dotcom-2000" {
		t.Fatalf("ByID(dotcom-2000) roundtrip failed: %+v ok=%v", u, ok)
	}

	if u2, ok := ByID("no-such-scenario"); ok || u2 != nil {
		t.Fatalf("ByID(unknown) should miss, got %+v ok=%v", u2, ok)
	}
}

// TestAllScenariosShape runs the plan-4 shape invariants over every
// registered scenario, so a future universe_<era>.go automatically gets the
// same gate dotcom-2000 has always had.
func TestAllScenariosShape(t *testing.T) {
	for _, id := range Universes() {
		id := id
		t.Run(id, func(t *testing.T) {
			u, ok := ByID(id)
			if !ok {
				t.Fatalf("ByID(%s) missing", id)
			}

			sc, err := BuildScenario(id)
			if err != nil {
				t.Fatalf("BuildScenario(%s): %v", id, err)
			}
			if sc.Days <= 200 {
				t.Fatalf("%s: days %d not > 200", id, sc.Days)
			}

			declared := map[string]bool{}
			for _, f := range sc.Factors {
				declared[f.ID] = true
			}

			for _, inst := range sc.Instruments {
				if inst.Alias == "" {
					t.Errorf("%s: instrument %s has empty alias", id, inst.ID)
				}
				if inst.Beta["IDIO:"+inst.ID] != 1 {
					t.Errorf("%s: instrument %s IDIO:self beta != 1 (got %v)",
						id, inst.ID, inst.Beta["IDIO:"+inst.ID])
				}
				if inst.IdioScale < 0.1 || inst.IdioScale > 3 {
					t.Errorf("%s: instrument %s idio scale %v outside [0.1, 3]",
						id, inst.ID, inst.IdioScale)
				}
				base := sc.Baseline[inst.ID]
				if len(base) != sc.Days {
					t.Errorf("%s: instrument %s baseline length %d != Days %d",
						id, inst.ID, len(base), sc.Days)
				}
				if len(base) == 0 || math.Abs(base[0].Close-100) > 1e-9 {
					t.Errorf("%s: instrument %s not normalized to 100", id, inst.ID)
				}
				for f := range inst.Beta {
					if !declared[f] {
						t.Errorf("%s: instrument %s beta key %q references undeclared factor",
							id, inst.ID, f)
					}
				}
				if realNamePattern.MatchString(inst.Alias) {
					t.Errorf("%s: instrument %s alias %q leaks a real name", id, inst.ID, inst.Alias)
				}
				if realNamePattern.MatchString(inst.Desc) {
					t.Errorf("%s: instrument %s desc %q leaks a real name", id, inst.ID, inst.Desc)
				}
			}
			if realNamePattern.MatchString(sc.EraHint) {
				t.Errorf("%s: EraHint %q leaks a real name", id, sc.EraHint)
			}

			for _, w := range sc.KeyWindows {
				if w.StartDay < 0 || w.EndDay >= sc.Days || w.StartDay >= w.EndDay {
					t.Errorf("%s: key window out of range: %+v (Days=%d)", id, w, sc.Days)
				}
			}

			sc2, err := BuildScenario(id)
			if err != nil {
				t.Fatalf("second BuildScenario(%s): %v", id, err)
			}
			if !reflect.DeepEqual(sc, sc2) {
				t.Errorf("%s: BuildScenario is not deterministic", id)
			}

			_ = u // u.FidelitySeeds/FetchSpecs are exercised by the other registry tests
		})
	}
}

// TestAllScenariosFidelity is the release gate (spec §4.6) run for every
// registered scenario, each through its own FidelitySeeds count of seeds.
func TestAllScenariosFidelity(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-seed world generation is slow")
	}
	for _, id := range Universes() {
		id := id
		t.Run(id, func(t *testing.T) {
			u, ok := ByID(id)
			if !ok {
				t.Fatalf("ByID(%s) missing", id)
			}
			sc, err := BuildScenario(id)
			if err != nil {
				t.Fatalf("BuildScenario(%s): %v", id, err)
			}
			for seed := uint64(1); seed <= uint64(u.FidelitySeeds); seed++ {
				if _, err := engine.GenerateWorld(sc, seed); err != nil {
					t.Fatalf("seed %d: %v", seed, err)
				}
			}
		})
	}
}

// TestAllScenariosCalibration mirrors TestPerturbationCalibrationStats
// (plan-4's calibration gate) across every registered scenario, with a
// lighter 4-seed sample per scenario (dotcom-2000 additionally keeps its own
// dedicated 10-seed calibration test).
func TestAllScenariosCalibration(t *testing.T) {
	if testing.Short() {
		t.Skip("Monte Carlo calibration is slow")
	}
	for _, id := range Universes() {
		id := id
		t.Run(id, func(t *testing.T) {
			sc, err := BuildScenario(id)
			if err != nil {
				t.Fatalf("BuildScenario(%s): %v", id, err)
			}
			var devs []float64
			var extraRets []float64
			clampHits := 0
			for seed := uint64(100); seed < 104; seed++ {
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

			// Same bands as plan-4's TestPerturbationCalibrationStats.
			if extraVol < 0.006 || extraVol > 0.030 {
				t.Errorf("%s: extra daily vol %.4f outside [0.006, 0.030]", id, extraVol)
			}
			if median < 0.008 || median > 0.08 {
				t.Errorf("%s: median |deviation| %.4f outside [0.008, 0.08]", id, median)
			}
			if p95 > 0.15 {
				t.Errorf("%s: p95 |deviation| %.4f > 0.15", id, p95)
			}
			if frac := float64(clampHits) / float64(len(devs)); frac > 0.001 {
				t.Errorf("%s: clamp near-hits fraction %.5f > 0.1%%", id, frac)
			}
			t.Logf("%s calibration: extraVol=%.4f median=%.4f p95=%.4f clampNearHits=%d/%d",
				id, extraVol, median, p95, clampHits, len(devs))
		})
	}
}
