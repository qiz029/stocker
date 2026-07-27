package pipeline

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestBuildScenarioShape(t *testing.T) {
	sc, err := BuildScenario()
	if err != nil {
		t.Fatalf("BuildScenario: %v", err)
	}
	if sc.ID != "dotcom-2000" {
		t.Fatalf("id: %s", sc.ID)
	}
	if sc.Days < 730 || sc.Days > 770 {
		t.Fatalf("days: %d (window 1999-01-04..2001-12-31 ≈ 750 trading days)", sc.Days)
	}
	if len(sc.Instruments) != 22 {
		t.Fatalf("instruments: %d", len(sc.Instruments))
	}
	// factors: MKT + SENT + 7 sectors + 3 macros + 22 idio = 34
	if len(sc.Factors) != 34 {
		t.Fatalf("factors: %d", len(sc.Factors))
	}
	for _, inst := range sc.Instruments {
		base := sc.Baseline[inst.ID]
		if len(base) != sc.Days {
			t.Fatalf("%s: baseline length %d", inst.ID, len(base))
		}
		if math.Abs(base[0].Close-100) > 1e-9 {
			t.Fatalf("%s: not normalized, day0 close %v", inst.ID, base[0].Close)
		}
		if inst.Beta["IDIO:"+inst.ID] != 1 {
			t.Fatalf("%s: idio beta missing", inst.ID)
		}
		// Vol-budget amendment (task-5 round 3): post-budget IdioScale is γ·[0.4,3.0], floor 0.1.
		if inst.IdioScale < 0.1 || inst.IdioScale > 3.0 {
			t.Fatalf("%s: idio scale %v outside clamp", inst.ID, inst.IdioScale)
		}
		if b := inst.Beta["MKT"]; b < -0.5 || b > 3 {
			t.Fatalf("%s: market beta %v outside clamp", inst.ID, b)
		}
	}
	// Reconstructed flags.
	recon := 0
	for _, inst := range sc.Instruments {
		if inst.Reconstructed {
			recon++
		}
	}
	if recon != 4 {
		t.Fatalf("reconstructed count: %d", recon)
	}
	// The market proxy instrument tracks the market factor strongly.
	for i := range sc.Instruments {
		if sc.Instruments[i].ID == "X22" {
			// Vol-budget amendment (task-5 round 3): X22's β_MKT=1.0 is scaled by its γ, range (0.2, 1.3].
			if b := sc.Instruments[i].Beta["MKT"]; b <= 0.2 || b > 1.3 {
				t.Fatalf("SPX-tracking instrument MKT beta %v", b)
			}
		}
	}
	// Key windows resolved to sane day indexes.
	if len(sc.KeyWindows) != 3 {
		t.Fatalf("key windows: %d", len(sc.KeyWindows))
	}
	for _, w := range sc.KeyWindows {
		if w.StartDay <= 0 || w.EndDay >= sc.Days || w.StartDay >= w.EndDay || w.Direction != -1 {
			t.Fatalf("bad key window: %+v", w)
		}
	}
	// Determinism.
	sc2, err := BuildScenario()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sc, sc2) {
		t.Fatal("BuildScenario not deterministic")
	}
	// Blind box: no real names anywhere in the scenario object.
	for _, inst := range sc.Instruments {
		if strings.Contains(inst.Alias, "Microsoft") || strings.Contains(inst.Desc, "Cisco") {
			t.Fatalf("real name leaked into scenario: %+v", inst)
		}
	}
}

func TestBuildMeta(t *testing.T) {
	meta := BuildMeta()
	if meta.Name != "2000 互联网泡沫" || meta.RealPeriod != "1999-01 ~ 2001-12" {
		t.Fatalf("meta: %+v", meta)
	}
	if len(meta.Dossiers) != 22 {
		t.Fatalf("dossiers: %d", len(meta.Dossiers))
	}
	d, ok := meta.Dossiers["X02"]
	if !ok || d.RealName == "" || d.Alias == "" || d.Bull == "" || d.Bear == "" || d.Business == "" {
		t.Fatalf("X02 dossier incomplete: %+v", d)
	}
}
