package pipeline

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// Anchor pins a reconstructed series to a documented price on a date.
type Anchor struct {
	Date  string // "2006-01-02"
	Price float64
}

const (
	reconNoiseSigma = 0.025 // daily log noise — must be representative of
	// 2000-era single-stock vol, or the fidelity gate's correlation floor
	// becomes unreachable (plan-1 lesson: corr ceiling = σb/√(σb²+σp²)).
	reconWickSigma = 0.008
)

// Reconstruct builds a daily OHLC series through the anchors: log-space
// linear interpolation plus Brownian-bridge noise that vanishes exactly at
// anchor days, so every anchor price is hit to the digit. Deterministic in
// (anchors, calendar, seed).
func Reconstruct(anchors []Anchor, calendar []time.Time, seed uint64) ([]Bar, error) {
	if len(anchors) < 2 {
		return nil, fmt.Errorf("need at least 2 anchors, got %d", len(anchors))
	}
	idxOf := make(map[string]int, len(calendar))
	for i, d := range calendar {
		idxOf[d.Format("2006-01-02")] = i
	}
	type pin struct {
		idx  int
		logP float64
	}
	pins := make([]pin, 0, len(anchors))
	for _, a := range anchors {
		if a.Price <= 0 {
			return nil, fmt.Errorf("anchor %s has non-positive price %v", a.Date, a.Price)
		}
		idx, ok := idxOf[a.Date]
		if !ok {
			return nil, fmt.Errorf("anchor %s not a calendar trading day", a.Date)
		}
		if len(pins) > 0 && idx <= pins[len(pins)-1].idx {
			return nil, fmt.Errorf("anchors not strictly ascending at %s", a.Date)
		}
		pins = append(pins, pin{idx, math.Log(a.Price)})
	}
	if pins[0].idx != 0 || pins[len(pins)-1].idx != len(calendar)-1 {
		return nil, fmt.Errorf("anchors must cover the full calendar (first day and last day)")
	}

	rng := rand.New(rand.NewPCG(seed, 0x9E3779B97F4A7C15))
	logC := make([]float64, len(calendar))
	for seg := 0; seg+1 < len(pins); seg++ {
		a, b := pins[seg], pins[seg+1]
		n := b.idx - a.idx
		// Random walk over the segment, then subtract the linear drift of
		// its own endpoint so the bridge is exactly zero at both anchors.
		walk := make([]float64, n+1)
		for i := 1; i <= n; i++ {
			walk[i] = walk[i-1] + rng.NormFloat64()*reconNoiseSigma
		}
		for i := 0; i <= n; i++ {
			frac := float64(i) / float64(n)
			trend := a.logP + (b.logP-a.logP)*frac
			bridge := walk[i] - walk[n]*frac
			logC[a.idx+i] = trend + bridge
		}
	}

	bars := make([]Bar, len(calendar))
	for i := range calendar {
		c := math.Exp(logC[i])
		o := c
		if i > 0 {
			o = bars[i-1].Close
		}
		wick := math.Abs(rng.NormFloat64() * reconWickSigma)
		hi := math.Max(o, c) * (1 + wick)
		lo := math.Min(o, c) * (1 - wick)
		bars[i] = Bar{Date: calendar[i], Open: o, High: hi, Low: lo, Close: c}
	}
	return bars, nil
}
