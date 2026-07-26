package engine

import "testing"

func TestStreamDeterministic(t *testing.T) {
	a, b := Stream(42, "shocks"), Stream(42, "shocks")
	for i := 0; i < 100; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatalf("same seed+label diverged at %d", i)
		}
	}
}

func TestStreamIndependentLabels(t *testing.T) {
	a, b := Stream(42, "shocks"), Stream(42, "eps", "CSCO")
	same := 0
	for i := 0; i < 100; i++ {
		if a.Uint64() == b.Uint64() {
			same++
		}
	}
	if same > 2 {
		t.Fatalf("labels not independent: %d/100 collisions", same)
	}
}
