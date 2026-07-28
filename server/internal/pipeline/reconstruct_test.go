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
		{cal[0].Format("2006-01-02"), 60, 0},
		{cal[150].Format("2006-01-02"), 90, 0},
		{cal[399].Format("2006-01-02"), 12, 0},
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
		{cal[0].Format("2006-01-02"), 60, 0},
		{cal[399].Format("2006-01-02"), 20, 0},
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
		{{cal[0].Format("2006-01-02"), 60, 0}},                                       // one anchor
		{{cal[3].Format("2006-01-02"), 60, 0}, {cal[1].Format("2006-01-02"), 50, 0}}, // unordered
		{{cal[0].Format("2006-01-02"), -1, 0}, {cal[9].Format("2006-01-02"), 50, 0}}, // bad price
		{{"1990-01-01", 60, 0}, {cal[9].Format("2006-01-02"), 50, 0}},                // outside calendar
		{{cal[2].Format("2006-01-02"), 60, 0}, {cal[7].Format("2006-01-02"), 50, 0}}, // doesn't pin calendar endpoints
	}
	for i, a := range bad {
		if _, err := Reconstruct(a, cal, 1); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestReconstructSegmentSigma(t *testing.T) {
	// Two-segment series: day 0→200 default sigma, day 200→400 with Sigma: 0.15
	cal := tradingCalendar(time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC), 400)
	anchors := []Anchor{
		{Date: cal[0].Format("2006-01-02"), Price: 50, Sigma: 0},      // first anchor, Sigma ignored
		{Date: cal[200].Format("2006-01-02"), Price: 50, Sigma: 0},    // end of first segment
		{Date: cal[399].Format("2006-01-02"), Price: 50, Sigma: 0.15}, // end of second segment with high sigma
	}
	bars, err := Reconstruct(anchors, cal, 42)
	if err != nil {
		t.Fatal(err)
	}

	// Verify anchors hit exactly
	if math.Abs(bars[0].Close-50) > 1e-9 {
		t.Fatalf("anchor 0: close %v want 50", bars[0].Close)
	}
	if math.Abs(bars[200].Close-50) > 1e-9 {
		t.Fatalf("anchor 200: close %v want 50", bars[200].Close)
	}
	if math.Abs(bars[399].Close-50) > 1e-9 {
		t.Fatalf("anchor 399: close %v want 50", bars[399].Close)
	}

	// Compute daily vol for head segment (0→200, default sigma 0.025)
	var headSum, headSum2 float64
	for i := 1; i <= 200; i++ {
		r := math.Log(bars[i].Close / bars[i-1].Close)
		headSum += r
		headSum2 += r * r
	}
	headVol := math.Sqrt(headSum2/200 - (headSum/200)*(headSum/200))

	// Compute daily vol for tail segment (200→399, high sigma 0.15)
	var tailSum, tailSum2 float64
	for i := 201; i < 400; i++ {
		r := math.Log(bars[i].Close / bars[i-1].Close)
		tailSum += r
		tailSum2 += r * r
	}
	tailVol := math.Sqrt(tailSum2/199 - (tailSum/199)*(tailSum/199))

	// Head segment should use default sigma → vol ∈ [0.015, 0.045]
	if headVol < 0.015 || headVol > 0.045 {
		t.Fatalf("head vol %.4f outside [0.015, 0.045]", headVol)
	}

	// Tail segment should use 0.15 sigma → vol ∈ [0.10, 0.22]
	if tailVol < 0.10 || tailVol > 0.22 {
		t.Fatalf("tail vol %.4f outside [0.10, 0.22]", tailVol)
	}

	// Verify determinism
	again, _ := Reconstruct(anchors, cal, 42)
	if again[250].Close != bars[250].Close {
		t.Fatal("not deterministic")
	}
}
