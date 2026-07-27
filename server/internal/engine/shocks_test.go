package engine

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestShockTimelineDeterministic(t *testing.T) {
	sc := scenario.Synthetic()
	a := GenerateShockTimeline(sc, 42)
	b := GenerateShockTimeline(sc, 42)
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Day != b[i].Day || a[i].MediaID != b[i].MediaID {
			t.Fatalf("event %d differs", i)
		}
		for f, v := range a[i].TrueShock {
			if b[i].TrueShock[f] != v {
				t.Fatalf("event %d shock differs on %s", i, f)
			}
		}
	}
	c := GenerateShockTimeline(sc, 43)
	if len(c) == len(a) {
		same := true
		for i := range a {
			if a[i].Day != c[i].Day {
				same = false
				break
			}
		}
		if same {
			t.Fatal("different seeds produced identical timelines")
		}
	}
}

func TestShockTimelineRates(t *testing.T) {
	sc := scenario.Synthetic()
	n := 0
	for s := uint64(0); s < 20; s++ {
		n += len(GenerateShockTimeline(sc, s))
	}
	perDay := float64(n) / (20 * float64(sc.Days))
	// 期望 ≈ 0.15 + 0.25 + 8*0.05 = 0.80 事件/日
	if perDay < 0.5 || perDay > 1.2 {
		t.Fatalf("event rate %f/day out of range", perDay)
	}
}

func TestKeyWindowSuppression(t *testing.T) {
	sc := scenario.Synthetic()
	kw := sc.KeyWindows[0] // 崩盘窗口, Direction=-1
	inWinPos, inWinNeg := 0, 0
	for s := uint64(0); s < 50; s++ {
		for _, ev := range GenerateShockTimeline(sc, s) {
			if ev.Day < kw.StartDay || ev.Day > kw.EndDay {
				continue
			}
			for _, v := range ev.TrueShock {
				if math.Abs(v) <= 0.02 {
					continue
				}
				if v > 0 {
					inWinPos++
				} else {
					inWinNeg++
				}
			}
		}
	}
	if inWinPos*3 >= inWinNeg {
		t.Fatalf("suppression too weak in crash window: %d big-positive vs %d big-negative", inWinPos, inWinNeg)
	}
}

func TestReportShockDecoupled(t *testing.T) {
	sc := scenario.Synthetic()
	evs := GenerateShockTimeline(sc, 42)
	diff := false
	for _, ev := range evs {
		for f, v := range ev.TrueShock {
			if ev.ReportShock[f] != v {
				diff = true
			}
		}
	}
	if !diff {
		t.Fatal("ReportShock identical to TrueShock everywhere; rho/noise not applied")
	}
}

func TestNoEmptyKeyShocks(t *testing.T) {
	// Test that instruments without idio factors don't produce empty-key shocks
	sc := &scenario.Scenario{
		ID:   "test",
		Days: 100,
		Factors: []scenario.Factor{
			{ID: "MKT", Name: "Market", Kind: scenario.KindMarket},
		},
		Instruments: []scenario.Instrument{
			{ID: "stock1", Alias: "S1", Desc: "Stock without idio", Beta: map[string]float64{"MKT": 1.0}},
		},
		KeyWindows: []scenario.KeyWindow{},
	}
	evs := GenerateShockTimeline(sc, 42)
	for _, ev := range evs {
		for f := range ev.TrueShock {
			if f == "" {
				t.Fatalf("found empty-key shock in event: %+v", ev)
			}
		}
	}
}

func TestIdioScaleScalesOnlyIdioShocks(t *testing.T) {
	sc := scenario.Synthetic()
	base := GenerateShockTimeline(sc, 7)

	scaled := scenario.Synthetic()
	for i := range scaled.Instruments {
		scaled.Instruments[i].IdioScale = 2.0
	}
	got := GenerateShockTimeline(scaled, 7)

	if len(got) != len(base) {
		t.Fatalf("event count changed: %d vs %d", len(got), len(base))
	}
	var checkedIdio, checkedShared int
	for i := range base {
		for f, v := range base[i].TrueShock {
			gv := got[i].TrueShock[f]
			if strings.HasPrefix(f, "IDIO:") {
				checkedIdio++
				if math.Abs(gv-2*v) > 1e-12 {
					t.Fatalf("idio shock not doubled: %v vs %v", gv, v)
				}
			} else {
				checkedShared++
				if gv != v {
					t.Fatalf("market/sector shock changed: %v vs %v", gv, v)
				}
			}
		}
	}
	if checkedIdio == 0 || checkedShared == 0 {
		t.Fatal("test exercised nothing")
	}
}

func TestIdioScaleDefaultsToOne(t *testing.T) {
	inst := scenario.Instrument{}
	if inst.IdioScaleOrDefault() != 1 {
		t.Fatalf("zero value must default to 1, got %v", inst.IdioScaleOrDefault())
	}
	inst.IdioScale = 0.5
	if inst.IdioScaleOrDefault() != 0.5 {
		t.Fatalf("explicit scale ignored")
	}
}

func TestExpandClustersShape(t *testing.T) {
	sc := scenario.Synthetic()
	base := GenerateShockTimeline(sc, 42)
	out := ExpandClusters(sc, 42, base)

	if len(out) < len(base) {
		t.Fatalf("clusters removed events: %d < %d", len(out), len(base))
	}
	byCluster := map[int][]NewsEvent{}
	for _, ev := range out {
		if ev.ClusterID != 0 {
			byCluster[ev.ClusterID] = append(byCluster[ev.ClusterID], ev)
		}
	}
	if len(byCluster) == 0 {
		t.Fatal("no clusters formed over 300 days at pCluster=0.6 — implausible")
	}
	lowRho := map[string]bool{"tabloid": true, "forum": true}
	highRho := map[string]bool{"wire": true, "paper": true}
	for id, evs := range byCluster {
		var main, rumor, follow int
		var mainDay int
		for _, ev := range evs {
			if ev.TrueShock != nil {
				main++
				mainDay = ev.Day
				// only market/sector events cluster
				for f := range ev.TrueShock {
					if strings.HasPrefix(f, "IDIO:") {
						t.Fatalf("cluster %d built around idio event", id)
					}
					if math.Abs(ev.TrueShock[f]) < bigShock {
						t.Fatalf("cluster %d around small shock %v", id, ev.TrueShock[f])
					}
				}
			}
		}
		if main != 1 {
			t.Fatalf("cluster %d has %d main events", id, main)
		}
		for _, ev := range evs {
			if ev.TrueShock != nil {
				continue
			}
			if ev.ReportShock == nil {
				t.Fatalf("cluster %d companion missing report shock", id)
			}
			switch {
			case ev.Day == mainDay-1:
				rumor++
				if !lowRho[ev.MediaID] {
					t.Fatalf("rumor from %s", ev.MediaID)
				}
			case ev.Day == mainDay+1:
				follow++
				if !highRho[ev.MediaID] {
					t.Fatalf("follow-up from %s", ev.MediaID)
				}
			default:
				t.Fatalf("companion on unexpected day %d (main %d)", ev.Day, mainDay)
			}
		}
		if rumor > 1 || follow > 1 {
			t.Fatalf("cluster %d duplicated companions", id)
		}
	}
	// Determinism.
	again := ExpandClusters(sc, 42, GenerateShockTimeline(sc, 42))
	if len(again) != len(out) {
		t.Fatal("ExpandClusters not deterministic")
	}
}

func TestClusterCompanionsDoNotMovePrices(t *testing.T) {
	sc := scenario.Synthetic()
	shocks := GenerateShockTimeline(sc, 42)
	statesPlain, err := EvolveFactorStates(sc, shocks)
	if err != nil {
		t.Fatal(err)
	}
	statesClustered, err := EvolveFactorStates(sc, ExpandClusters(sc, 42, shocks))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(statesPlain, statesClustered) {
		t.Fatal("zero-impact companions changed factor states")
	}
}
