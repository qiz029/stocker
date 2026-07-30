package engine

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestForumPostsDeterministic(t *testing.T) {
	sc := scenario.Synthetic()
	w1, err := GenerateWorld(sc, 42)
	if err != nil {
		t.Fatal(err)
	}
	w2, err := GenerateWorld(sc, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(w1.Forum) == 0 {
		t.Fatal("no forum posts generated")
	}
	if !reflect.DeepEqual(w1.Forum, w2.Forum) {
		t.Fatal("same seed produced different forum posts")
	}
	// A different seed should produce a different room (persona draws differ).
	w3, err := GenerateWorld(sc, 43)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(w1.Forum, w3.Forum) {
		t.Fatal("different seeds produced identical forums")
	}
}

func TestForumCadenceBoundsAndPersonaConsistency(t *testing.T) {
	sc := scenario.Synthetic()
	w, err := GenerateWorld(sc, 42)
	if err != nil {
		t.Fatal(err)
	}
	perDay := map[int]int{}
	personaByNPC := map[string]string{}
	for _, p := range w.Forum {
		perDay[p.Day]++
		if p.NPCName == "" || p.Body == "" {
			t.Fatalf("empty field in post: %+v", p)
		}
		if prev, ok := personaByNPC[p.NPCName]; ok && prev != p.Persona {
			t.Fatalf("NPC %s changed persona: %s → %s", p.NPCName, prev, p.Persona)
		}
		personaByNPC[p.NPCName] = p.Persona
	}
	for d := 0; d < sc.Days; d++ {
		if n := perDay[d]; n < 2 || n > 6 {
			t.Fatalf("day %d has %d posts, want 2..6", d, n)
		}
	}
}

var digitRe = regexp.MustCompile(`[0-9]`)
var floatRe = regexp.MustCompile(`\d+\.\d+`)

func TestForumTemplatesNoRealNamesOrNumbers(t *testing.T) {
	pools := map[string][]string{
		"npc":       npcNames,
		"up":        forumReactionUp,
		"down":      forumReactionDown,
		"mixed":     forumReactionMixed,
		"bandwagon": forumBandwagon,
		"skeptic":   forumSkeptic,
		"rumor":     forumRumor,
		"chatter":   forumChatter,
	}
	for family, pool := range pools {
		for _, s := range pool {
			if digitRe.MatchString(s) {
				t.Fatalf("%s template carries a digit: %q", family, s)
			}
		}
	}
	// Generated bodies must never carry float-like numbers (shock values),
	// even though synthetic aliases legitimately contain digits ("Syn S1").
	sc := scenario.Synthetic()
	w, err := GenerateWorld(sc, 42)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range w.Forum {
		if floatRe.MatchString(p.Body) {
			t.Fatalf("forum post carries a number: %q", p.Body)
		}
	}
}

func tinyRecapScenario() *scenario.Scenario {
	return &scenario.Scenario{
		ID: "tiny", Days: 4,
		Factors: []scenario.Factor{
			{ID: "MKT", Name: "market", Kind: scenario.KindMarket},
			{ID: "SEC", Name: "明星板块", Kind: scenario.KindSector},
		},
		Instruments: []scenario.Instrument{
			{ID: "A", Alias: "阿尔法科技", Beta: map[string]float64{"MKT": 1, "SEC": 1}},
			{ID: "B", Alias: "贝塔重工", Beta: map[string]float64{"MKT": 1}},
			{ID: "C", Alias: "伽马能源", Beta: map[string]float64{"MKT": 1}},
		},
	}
}

func flatOHLC(closes ...float64) []scenario.OHLC {
	out := make([]scenario.OHLC, len(closes))
	prev := closes[0]
	for i, c := range closes {
		open := prev
		if i == 0 {
			open = c
		}
		out[i] = scenario.OHLC{Open: open, High: c, Low: c, Close: c}
		prev = c
	}
	return out
}

func TestRecapNewsNamesBiggestMovers(t *testing.T) {
	sc := tinyRecapScenario()
	// Day 1: A gaps up, B gaps down, C flat. SEC holds only A.
	prices := map[string][]scenario.OHLC{
		"A": flatOHLC(100, 120, 120, 120),
		"B": flatOHLC(100, 80, 80, 80),
		"C": flatOHLC(100, 100, 100, 100),
	}
	evs := RecapNews(sc, prices, 7)
	if !reflect.DeepEqual(evs, RecapNews(sc, prices, 7)) {
		t.Fatal("recap news not deterministic")
	}
	// One recap per covered day, published the next day, zero impact.
	if len(evs) != sc.Days-1 {
		t.Fatalf("recaps = %d, want %d", len(evs), sc.Days-1)
	}
	byDay := map[int]NewsEvent{}
	for _, ev := range evs {
		if ev.Track != TrackHistorical || !ev.Recap || len(ev.TrueShock) != 0 || len(ev.ReportShock) != 0 {
			t.Fatalf("recap must be zero-impact historical: %+v", ev)
		}
		byDay[ev.Day] = ev
	}
	// The recap published on day 2 summarizes day 1 and must name the
	// actual biggest gainer/loser (aliases) and the strongest sector.
	h := byDay[2].Headline
	for _, want := range []string{"阿尔法科技", "贝塔重工", "明星板块"} {
		if !strings.Contains(h, want) {
			t.Fatalf("day-2 recap missing %q: %q", want, h)
		}
	}
	// Flat days still recap the (tied, declaration-order) movers.
	if byDay[3].Headline == "" {
		t.Fatal("missing recap for flat day")
	}
}

func TestNoiseNewsTemplateVariety(t *testing.T) {
	if len(noiseTemplates) < 20 {
		t.Fatalf("noise template pool = %d, want 20+", len(noiseTemplates))
	}
	for _, s := range noiseTemplates {
		if digitRe.MatchString(s) {
			t.Fatalf("noise template carries a digit: %q", s)
		}
	}
	sc := scenario.Synthetic()
	evs := NoiseNews(sc, 42)
	if len(evs) == 0 {
		t.Fatal("no noise news")
	}
	for i := 1; i < len(evs); i++ {
		if evs[i].Headline == evs[i-1].Headline {
			t.Fatalf("consecutive noise items repeat %q", evs[i].Headline)
		}
	}
}
