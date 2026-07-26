package engine

import (
	"math"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestSingleShockDecay(t *testing.T) {
	sc := scenario.Synthetic()
	shock := 0.10
	evs := []NewsEvent{{Day: 9, Track: TrackImpact,
		TrueShock: map[string]float64{"MKT": shock}}}
	states, err := EvolveFactorStates(sc, evs)
	if err != nil {
		t.Fatal(err)
	}
	mkt := 0 // "MKT" 是 FactorIDs()[0]
	if states[9][mkt] != 0 {
		t.Fatal("shock must take effect on Day+1, not publish day")
	}
	x10 := states[10][mkt]
	want10 := shock // 生效首日：fast 0.65+slow 0.35 = 全额
	if math.Abs(x10-want10) > 1e-9 {
		t.Fatalf("day10 X=%f want %f", x10, want10)
	}
	x11 := states[11][mkt]
	want11 := shock*0.65*lamFast + shock*0.35*lamSlow
	if math.Abs(x11-want11) > 1e-9 {
		t.Fatalf("day11 X=%f want %f", x11, want11)
	}
	// 长期回归 0
	if math.Abs(states[299][mkt]) > 0.01 {
		t.Fatalf("X should decay toward 0, got %f", states[299][mkt])
	}
}

func TestNoEventsZeroStates(t *testing.T) {
	sc := scenario.Synthetic()
	states, err := EvolveFactorStates(sc, nil)
	if err != nil {
		t.Fatal(err)
	}
	for d := range states {
		for i, x := range states[d] {
			if x != 0 {
				t.Fatalf("day %d factor %d nonzero without events", d, i)
			}
		}
	}
}

func TestEvolveFactorStatesUnknownFactorID(t *testing.T) {
	sc := scenario.Synthetic()
	evs := []NewsEvent{{Day: 9, Track: TrackImpact,
		TrueShock: map[string]float64{"NOPE_NOT_A_FACTOR": 0.05}}}
	_, err := EvolveFactorStates(sc, evs)
	if err == nil {
		t.Fatal("expected error for unknown factor id, got nil")
	}
}
