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
		if ev.HeadlineEn == "" {
			t.Fatalf("event %d has empty English headline", i)
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
			if !strings.HasPrefix(ev.HeadlineEn, "【Rumor】") {
				t.Fatalf("zh rumor prefix without en 【Rumor】: %q", ev.HeadlineEn)
			}
		}
		if strings.HasPrefix(ev.Headline, "【追踪】") {
			sawFollow = true
			if !strings.HasPrefix(ev.HeadlineEn, "【Follow-up】") {
				t.Fatalf("zh follow-up prefix without en 【Follow-up】: %q", ev.HeadlineEn)
			}
		}
	}
	if !sawRumor || !sawFollow {
		t.Fatalf("cluster copy prefixes missing: rumor=%v follow=%v", sawRumor, sawFollow)
	}
}

// TestTemplatePoolsParallel pins the zh/en template pools to equal lengths,
// so the shared pick index always has a translation to land on.
func TestTemplatePoolsParallel(t *testing.T) {
	pairs := map[string][2][]string{
		"recap":      {recapTemplates, recapTemplatesEn},
		"recapNoSec": {recapTemplatesNoSector, recapTemplatesNoSectorEn},
		"noise":      {noiseTemplates, noiseTemplatesEn},
		"impact":     {impactTemplates, impactTemplatesEn},
	}
	for name, p := range pairs {
		if len(p[0]) != len(p[1]) {
			t.Fatalf("%s pool: zh=%d en=%d", name, len(p[0]), len(p[1]))
		}
		for i, s := range p[1] {
			if s == "" {
				t.Fatalf("%s en template %d is empty", name, i)
			}
		}
	}
}

// TestNoiseNewsEnMatchesZhIndex verifies the English headline is the
// same-index translation of the Chinese one (same pick, no extra draws).
func TestNoiseNewsEnMatchesZhIndex(t *testing.T) {
	sc := scenario.Synthetic()
	evs := NoiseNews(sc, 42)
	if len(evs) == 0 {
		t.Fatal("no noise news")
	}
	for _, ev := range evs {
		idx := -1
		for k, tpl := range noiseTemplates {
			if tpl == ev.Headline {
				idx = k
				break
			}
		}
		if idx < 0 {
			t.Fatalf("zh headline not from pool: %q", ev.Headline)
		}
		if ev.HeadlineEn != noiseTemplatesEn[idx] {
			t.Fatalf("en headline not the same-index translation of %q: %q", ev.Headline, ev.HeadlineEn)
		}
	}
}
