package engine

import "testing"

func TestResolveAliasDeterministic(t *testing.T) {
	candidates := []string{"Ridgeline Networks", "Vantor Networks", "Copperline Communications"}
	first := ResolveAlias(42, "S1", "Ridgeline Networks", candidates)
	for i := 0; i < 100; i++ {
		if got := ResolveAlias(42, "S1", "Ridgeline Networks", candidates); got != first {
			t.Fatalf("same seed diverged: %q then %q", first, got)
		}
	}
}

func TestResolveAliasStaysInCandidates(t *testing.T) {
	candidates := []string{"A", "B", "C"}
	for seed := uint64(0); seed < 200; seed++ {
		got := ResolveAlias(seed, "X01", "A", candidates)
		ok := false
		for _, c := range candidates {
			if got == c {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("seed %d: pick %q not in candidates", seed, got)
		}
	}
	// Distinct labels pick independently: some seed must differentiate
	// instruments (not a guarantee per seed, just a sanity sweep).
	picks := map[string]bool{}
	for seed := uint64(0); seed < 200; seed++ {
		picks[ResolveAlias(seed, "X01", "A", candidates)] = true
	}
	if len(picks) < 2 {
		t.Fatalf("200 seeds collapsed to one candidate: %v", picks)
	}
}

func TestResolveAliasEmptyCandidatesFallsBack(t *testing.T) {
	if got := ResolveAlias(7, "X01", "Northgate Systems", nil); got != "Northgate Systems" {
		t.Fatalf("nil candidates: got %q", got)
	}
	if got := ResolveAlias(7, "X01", "Northgate Systems", []string{}); got != "Northgate Systems" {
		t.Fatalf("empty candidates: got %q", got)
	}
}
