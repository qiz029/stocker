package pipeline

import (
	"math"
	"testing"
	"time"
)

func tradingCalendar(start time.Time, days int) []time.Time {
	cal := make([]time.Time, 0, days)
	d := start
	for len(cal) < days {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			cal = append(cal, d)
		}
		d = d.AddDate(0, 0, 1)
	}
	return cal
}

func TestReconstructHitsAnchorsExactly(t *testing.T) {
	cal := tradingCalendar(time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC), 400)
	anchors := []Anchor{
		{cal[0].Format("2006-01-02"), 60},
		{cal[150].Format("2006-01-02"), 90},
		{cal[399].Format("2006-01-02"), 12},
	}
	bars, err := Reconstruct(anchors, cal, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 400 {
		t.Fatalf("bars: %d", len(bars))
	}
	for _, tc := range []struct {
		idx  int
		want float64
	}{{0, 60}, {150, 90}, {399, 12}} {
		if math.Abs(bars[tc.idx].Close-tc.want) > 1e-9 {
			t.Fatalf("anchor at %d: close %v want %v", tc.idx, bars[tc.idx].Close, tc.want)
		}
	}
}

func TestReconstructHasRealisticVolAndValidOHLC(t *testing.T) {
	cal := tradingCalendar(time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC), 400)
	anchors := []Anchor{
		{cal[0].Format("2006-01-02"), 60},
		{cal[399].Format("2006-01-02"), 20},
	}
	bars, err := Reconstruct(anchors, cal, 7)
	if err != nil {
		t.Fatal(err)
	}
	var sum, sum2 float64
	for i := 1; i < len(bars); i++ {
		r := math.Log(bars[i].Close / bars[i-1].Close)
		sum += r
		sum2 += r * r
	}
	n := float64(len(bars) - 1)
	vol := math.Sqrt(sum2/n - (sum/n)*(sum/n))
	if vol < 0.015 || vol > 0.045 {
		t.Fatalf("daily vol %.4f outside [0.015, 0.045]", vol)
	}
	for i, b := range bars {
		if b.Low <= 0 || b.High < b.Low || b.Close > b.High || b.Close < b.Low ||
			b.Open > b.High || b.Open < b.Low {
			t.Fatalf("invalid OHLC at %d: %+v", i, b)
		}
	}
	// Determinism.
	again, _ := Reconstruct(anchors, cal, 7)
	if again[123].Close != bars[123].Close {
		t.Fatal("not deterministic")
	}
	// Different seed differs.
	other, _ := Reconstruct(anchors, cal, 8)
	if other[123].Close == bars[123].Close {
		t.Fatal("seed has no effect")
	}
}

func TestReconstructValidation(t *testing.T) {
	cal := tradingCalendar(time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC), 10)
	bad := [][]Anchor{
		{{cal[0].Format("2006-01-02"), 60}},                                    // one anchor
		{{cal[3].Format("2006-01-02"), 60}, {cal[1].Format("2006-01-02"), 50}}, // unordered
		{{cal[0].Format("2006-01-02"), -1}, {cal[9].Format("2006-01-02"), 50}}, // bad price
		{{"1990-01-01", 60}, {cal[9].Format("2006-01-02"), 50}},                // outside calendar
	}
	for i, a := range bad {
		if _, err := Reconstruct(a, cal, 1); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}
