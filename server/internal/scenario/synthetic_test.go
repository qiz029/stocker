package scenario

import "testing"

func TestSyntheticShape(t *testing.T) {
	sc := Synthetic()
	if sc.Days != 300 || len(sc.Instruments) != 8 {
		t.Fatalf("shape: days=%d instruments=%d", sc.Days, len(sc.Instruments))
	}
	nIdio := 0
	for _, f := range sc.Factors {
		if f.Kind == "idio" {
			nIdio++
		}
	}
	if nIdio != len(sc.Instruments) {
		t.Fatalf("want one idio factor per instrument, got %d", nIdio)
	}
	for _, inst := range sc.Instruments {
		prices := sc.Baseline[inst.ID]
		if len(prices) != sc.Days {
			t.Fatalf("%s: %d days", inst.ID, len(prices))
		}
		if prices[0].Open != 100 {
			t.Fatalf("%s: start open %f, want 100", inst.ID, prices[0].Open)
		}
		for d, p := range prices {
			if p.Low > p.Open || p.Low > p.Close || p.High < p.Open || p.High < p.Close || p.Low <= 0 {
				t.Fatalf("%s day %d: invalid OHLC %+v", inst.ID, d, p)
			}
		}
	}
	if len(sc.KeyWindows) == 0 {
		t.Fatal("synthetic scenario must include a crash KeyWindow")
	}
}

func TestSyntheticDeterministic(t *testing.T) {
	a, b := Synthetic(), Synthetic()
	if a.Baseline["S1"][123] != b.Baseline["S1"][123] {
		t.Fatal("synthetic scenario must be deterministic")
	}
}

func TestSyntheticHasBoomAndCrash(t *testing.T) {
	sc := Synthetic()
	p := sc.Baseline["S1"]
	peak := p[150].Close
	if peak < 150 { // 前 150 天泡沫至少 +50%
		t.Fatalf("boom too weak: peak %f", peak)
	}
	if p[220].Close > peak*0.55 { // 崩盘至少 -45%
		t.Fatalf("crash too weak: %f vs peak %f", p[220].Close, peak)
	}
}
