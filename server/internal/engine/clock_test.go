package engine

import (
	"testing"
	"time"
)

func TestCurrentDay(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	day27 := 27 * time.Minute
	cases := []struct {
		name  string
		now   time.Time
		day   int
		ended bool
	}{
		{"at start", start, 0, false},
		{"just before day 1", start.Add(day27 - time.Second), 0, false},
		{"exactly day 1", start.Add(day27), 1, false},
		{"mid game", start.Add(100*day27 + time.Minute), 100, false},
		{"last day", start.Add(749 * day27), 749, false},
		{"past end clamps", start.Add(3000 * day27), 749, true},
	}
	for _, c := range cases {
		day, ended := CurrentDay(start, day27, 750, c.now)
		if day != c.day || ended != c.ended {
			t.Errorf("%s: got (%d,%v) want (%d,%v)", c.name, day, ended, c.day, c.ended)
		}
	}
}
