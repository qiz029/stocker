package engine

import (
	"math"
	"strings"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestHistoricalNewsCoversBigMoves(t *testing.T) {
	sc := scenario.Synthetic()
	evs := HistoricalNews(sc)
	byDay := map[int]bool{}
	for _, ev := range evs {
		if ev.Track != TrackHistorical {
			t.Fatal("wrong track")
		}
		if len(ev.TrueShock) != 0 {
			t.Fatal("historical news must not inject shocks")
		}
		byDay[ev.Day] = true
	}
	// 找一个基线大波动日, 断言有报道
	found := false
	for _, inst := range sc.Instruments {
		p := sc.Baseline[inst.ID]
		for d := 1; d < sc.Days; d++ {
			if math.Abs(math.Log(p[d].Close/p[d-1].Close)) > 0.06 {
				found = true
				if !byDay[d] {
					t.Fatalf("big move day %d has no historical news", d)
				}
			}
		}
	}
	if !found {
		t.Skip("synthetic scenario produced no >6% day; loosen synthetic noise")
	}
}

func TestNoiseNewsZeroImpactAndDeterministic(t *testing.T) {
	sc := scenario.Synthetic()
	a := NoiseNews(sc, 42)
	b := NoiseNews(sc, 42)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("noise news not deterministic: %d vs %d", len(a), len(b))
	}
	for _, ev := range a {
		if ev.Track != TrackNoise || len(ev.TrueShock) != 0 {
			t.Fatal("noise news must have zero impact")
		}
	}
}

func TestFillFallbackCopy(t *testing.T) {
	sc := scenario.Synthetic()
	evs := GenerateShockTimeline(sc, 42)
	evs = append(evs, NoiseNews(sc, 42)...)
	FillFallbackCopy(sc, evs, 42)
	for i, ev := range evs {
		if ev.Headline == "" {
			t.Fatalf("event %d has empty headline", i)
		}
	}
}

func TestClusterFallbackCopyPrefixes(t *testing.T) {
	sc := scenario.Synthetic()
	evs := ExpandClusters(sc, 42, GenerateShockTimeline(sc, 42))
	FillFallbackCopy(sc, evs, 42)
	var sawRumor, sawFollow bool
	for _, ev := range evs {
		if ev.ClusterID == 0 || ev.TrueShock != nil {
			continue
		}
		if strings.HasPrefix(ev.Headline, "【传闻】") {
			sawRumor = true
		}
		if strings.HasPrefix(ev.Headline, "【追踪】") {
			sawFollow = true
		}
	}
	if !sawRumor || !sawFollow {
		t.Fatalf("cluster copy prefixes missing: rumor=%v follow=%v", sawRumor, sawFollow)
	}
}
