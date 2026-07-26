package engine

import (
	"math"
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
