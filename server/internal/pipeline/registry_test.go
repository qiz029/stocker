package pipeline

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

// realNamePattern is the blind-box spot list: real company names must never
// leak into alias, desc, or EraHint (spec, plan-5 Global Constraints).
// Covers dotcom-2000 + crash-1987 + nifty-1972 + the upcoming gfc-2008 names.
var realNamePattern = regexp.MustCompile(
	`Microsoft|Cisco|Intel|IBM|Apple|Amazon|Lehman|Goldman|Kodak|Xerox|` +
		`Coca-Cola|McDonald|Disney|Johnson|Procter|Gamble|Caterpillar|Exxon|Avon|` +
		`Merck|Boeing|American Express|Morgan|Citigroup|Wells Fargo|Bear Stearns|AIG|Lennar|Chevron`)

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
				// build.go's degenerate volatility-budget branch (see its
				// doc comment) zeros MKT/sector betas when an instrument's
				// own historical vol is too low to budget even at zero
				// beta contribution. That's expected — and news-inert by
				// design — for a universe's own market-proxy/index
				// instrument (nifty-1972's N15 is the first case), but if
				// it ever fires for an ordinary instrument, that's a real
				// modeling bug (a stock silently going deaf to MKT/sector
				// news) worth catching here rather than letting it pass
				// silently.
				if inst.Beta["MKT"] == 0 && inst.ID != u.MarketProxy {
					t.Errorf("degenerate vol budget fired for non-index instrument %s in %s — review the universe composition",
						inst.ID, id)
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

			// Blind-box scan of the authored dossier PROSE (Business/Bull/
			// Bear), not just Alias/Desc/EraHint. RealName is deliberately
			// exempt — it's the one field that's supposed to hold the real
			// name, surfaced only on reveal.
			meta, err := BuildMeta(id)
			if err != nil {
				t.Fatalf("BuildMeta(%s): %v", id, err)
			}
			for instID, d := range meta.Dossiers {
				for _, field := range []struct {
					name, val string
				}{
					{"Alias", d.Alias}, {"Desc", d.Desc},
					{"Business", d.Business}, {"Bull", d.Bull}, {"Bear", d.Bear},
				} {
					if realNamePattern.MatchString(field.val) {
						t.Errorf("%s: instrument %s dossier field %s %q leaks a real name",
							id, instID, field.name, field.val)
					}
				}
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

// TestCrash1987SingleDayDrop is crash-1987's scenario-specific sanity check
// (plan-5 Task 4): the real -20%+ single-day crash must survive alignment
// and normalization, landing inside the scripted crash-week key window.
func TestCrash1987SingleDayDrop(t *testing.T) {
	sc, err := BuildScenario("crash-1987")
	if err != nil {
		t.Fatalf("BuildScenario(crash-1987): %v", err)
	}
	base := sc.Baseline["Y17"]
	if base == nil {
		t.Fatal("crash-1987: no baseline for Y17")
	}
	worstDay := -1
	worstRet := 0.0
	for d := 1; d < len(base); d++ {
		ret := math.Log(base[d].Close / base[d-1].Close)
		if worstDay == -1 || ret < worstRet {
			worstDay = d
			worstRet = ret
		}
	}
	if worstRet > -0.18 {
		t.Fatalf("crash-1987: worst single-day log return %.4f (day %d) does not exceed -0.18", worstRet, worstDay)
	}
	inWindow := false
	for _, w := range sc.KeyWindows {
		if w.Direction == -1 && worstDay >= w.StartDay && worstDay <= w.EndDay {
			inWindow = true
		}
	}
	if !inWindow {
		t.Fatalf("crash-1987: worst day %d (ret %.4f) not inside the crash-week key window; windows=%+v",
			worstDay, worstRet, sc.KeyWindows)
	}
}

// maxLogDrawdown returns the worst (most negative) log peak-to-trough
// drawdown of a baseline close series: max over t of log(close[t]/peakSoFar).
func maxLogDrawdown(base []scenario.OHLC) float64 {
	worst := 0.0
	peak := base[0].Close
	for _, b := range base {
		if b.Close > peak {
			peak = b.Close
		}
		dd := math.Log(b.Close / peak)
		if dd < worst {
			worst = dd
		}
	}
	return worst
}

// TestNifty1972BearMarketDepth is nifty-1972's scenario-specific sanity
// check (plan-5 Task 5): the "最优质的公司也跌八成" texture the scenario exists
// to teach must survive alignment/normalization/reconstruction — the SPX
// proxy itself must show a deep bear market, and at least 4 of the NIFT
// (一流成长/"nifty fifty") sector instruments must show a baseline drawdown
// severe enough to make the point.
func TestNifty1972BearMarketDepth(t *testing.T) {
	sc, err := BuildScenario("nifty-1972")
	if err != nil {
		t.Fatalf("BuildScenario(nifty-1972): %v", err)
	}
	u, ok := ByID("nifty-1972")
	if !ok {
		t.Fatal("nifty-1972 not registered")
	}

	proxyBase := sc.Baseline[u.MarketProxy]
	if proxyBase == nil {
		t.Fatalf("nifty-1972: no baseline for market proxy %s", u.MarketProxy)
	}
	proxyDD := maxLogDrawdown(proxyBase)
	if proxyDD > -0.5 {
		t.Fatalf("nifty-1972: SPX proxy %s max log drawdown %.4f does not exceed -0.5",
			u.MarketProxy, proxyDD)
	}

	niftIDs := map[string]bool{}
	for _, spec := range u.Instruments {
		if spec.Sector == "NIFT" {
			niftIDs[spec.ID] = true
		}
	}
	deepCount := 0
	var report []string
	for id := range niftIDs {
		base := sc.Baseline[id]
		if base == nil {
			t.Fatalf("nifty-1972: no baseline for NIFT instrument %s", id)
		}
		dd := maxLogDrawdown(base)
		// -60% drawdown in price terms is log(0.4) ≈ -0.9163.
		if dd <= math.Log(0.4) {
			deepCount++
		}
		report = append(report, fmt.Sprintf("%s=%.4f", id, dd))
	}
	if deepCount < 4 {
		t.Fatalf("nifty-1972: only %d/%d NIFT instruments have baseline drawdown >= 60%% (need >= 4); drawdowns: %v",
			deepCount, len(niftIDs), report)
	}
	t.Logf("nifty-1972: SPX proxy drawdown=%.4f, NIFT drawdowns: %v (>=60%% count=%d/%d)",
		proxyDD, report, deepCount, len(niftIDs))
}

// TestGfc2008CrisisShape is gfc-2008's scenario-specific sanity check
// (plan-5 Task 6): G03 (Lehman, reconstructed to zero) must show an
// enormous baseline net log return, G09 (AIG) must show a near-total
// baseline drawdown, the DEFC ("防御消费") sector instruments must hold up
// (drawdown < 40%, "防御股确实防御"), and the +1 (绝望底反转) key window must
// actually start at the SPX proxy's global baseline trough — 2009-03-09
// really was the bottom.
func TestGfc2008CrisisShape(t *testing.T) {
	sc, err := BuildScenario("gfc-2008")
	if err != nil {
		t.Fatalf("BuildScenario(gfc-2008): %v", err)
	}
	u, ok := ByID("gfc-2008")
	if !ok {
		t.Fatal("gfc-2008 not registered")
	}

	// G03 (LEH): baseline net log return <= -5.
	lehBase := sc.Baseline["G03"]
	if lehBase == nil {
		t.Fatal("gfc-2008: no baseline for G03")
	}
	lehNet := math.Log(lehBase[len(lehBase)-1].Close / lehBase[0].Close)
	if lehNet > -5 {
		t.Fatalf("gfc-2008: G03 (LEH) net log return %.4f does not go below -5 (雷曼归零)", lehNet)
	}

	// G09 (AIG): baseline drawdown >= 90% (log <= log(0.1) ~= -2.3026).
	aigBase := sc.Baseline["G09"]
	if aigBase == nil {
		t.Fatal("gfc-2008: no baseline for G09")
	}
	aigDD := maxLogDrawdown(aigBase)
	if aigDD > math.Log(0.1) {
		t.Fatalf("gfc-2008: G09 (AIG) max log drawdown %.4f does not reach -90%% (log <= %.4f)",
			aigDD, math.Log(0.1))
	}

	// DEFC sector: every instrument's baseline drawdown < 40% (log > log(0.6)).
	var defcReport []string
	for _, spec := range u.Instruments {
		if spec.Sector != "DEFC" {
			continue
		}
		base := sc.Baseline[spec.ID]
		if base == nil {
			t.Fatalf("gfc-2008: no baseline for DEFC instrument %s", spec.ID)
		}
		dd := maxLogDrawdown(base)
		defcReport = append(defcReport, fmt.Sprintf("%s=%.4f", spec.ID, dd))
		if dd <= math.Log(0.6) {
			t.Errorf("gfc-2008: DEFC instrument %s baseline drawdown %.4f exceeds 40%% (log <= %.4f)",
				spec.ID, dd, math.Log(0.6))
		}
	}

	// +1 key window's start day must sit within 10 days of the SPX proxy's
	// global baseline trough (the trough itself must be 2009-03-09).
	proxyBase := sc.Baseline[u.MarketProxy]
	if proxyBase == nil {
		t.Fatalf("gfc-2008: no baseline for market proxy %s", u.MarketProxy)
	}
	troughDay, troughClose := -1, proxyBase[0].Close
	for i, b := range proxyBase {
		if b.Close < troughClose {
			troughClose = b.Close
			troughDay = i
		}
	}
	var reboundStart = -1
	for _, w := range sc.KeyWindows {
		if w.Direction == 1 {
			reboundStart = w.StartDay
		}
	}
	if reboundStart == -1 {
		t.Fatal("gfc-2008: no +1 key window found")
	}
	if diff := reboundStart - troughDay; diff < -10 || diff > 10 {
		t.Fatalf("gfc-2008: +1 key window start day %d is not within 10 days of SPX proxy trough day %d (diff %d)",
			reboundStart, troughDay, diff)
	}
	t.Logf("gfc-2008: G03 net=%.4f, G09 drawdown=%.4f, DEFC drawdowns: %v, SPX trough day=%d, +1 window start day=%d",
		lehNet, aigDD, defcReport, troughDay, reboundStart)
}
