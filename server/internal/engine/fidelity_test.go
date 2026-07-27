package engine

import (
	"math"
	"strings"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestFidelityHoldsAcrossSeeds(t *testing.T) {
	sc := scenario.Synthetic()
	for s := uint64(0); s < 30; s++ {
		evs := GenerateShockTimeline(sc, s)
		states, err := EvolveFactorStates(sc, evs)
		if err != nil {
			t.Fatalf("seed %d: %v", s, err)
		}
		prices := SynthesizePrices(sc, states, s)
		if err := VerifyFidelity(sc, prices); err != nil {
			t.Fatalf("seed %d violates fidelity: %v", s, err)
		}
	}
}

func TestFidelityRejectsGarbage(t *testing.T) {
	sc := scenario.Synthetic()
	// 基线取反(镜像)必然违反相关性/方向条款
	bad := map[string][]scenario.OHLC{}
	for id, p := range sc.Baseline {
		rev := make([]scenario.OHLC, len(p))
		for i := range p {
			rev[i] = p[len(p)-1-i]
		}
		bad[id] = rev
	}
	if err := VerifyFidelity(sc, bad); err == nil {
		t.Fatal("reversed prices should fail fidelity")
	}
}

func TestFidelityRejectsMissingInstrument(t *testing.T) {
	sc := scenario.Synthetic()
	prices := map[string][]scenario.OHLC{}
	for id, p := range sc.Baseline {
		prices[id] = p
	}
	// 移除一个标的的价格, 不应 panic, 必须返回描述性错误
	delete(prices, sc.Instruments[0].ID)
	err := VerifyFidelity(sc, prices)
	if err == nil {
		t.Fatal("missing instrument prices should fail fidelity, got nil error")
	}
}

func TestFidelityRejectsLengthMismatch(t *testing.T) {
	sc := scenario.Synthetic()
	prices := map[string][]scenario.OHLC{}
	for id, p := range sc.Baseline {
		prices[id] = p
	}
	// 截断一个标的的价格序列, 不应 panic, 必须返回描述性错误
	id := sc.Instruments[0].ID
	prices[id] = prices[id][:len(prices[id])-5]
	err := VerifyFidelity(sc, prices)
	if err == nil {
		t.Fatal("length-mismatched prices should fail fidelity, got nil error")
	}
}

// oneInstScenario builds a minimal scenario around a single baseline series.
func oneInstScenario(base []scenario.OHLC) *scenario.Scenario {
	return &scenario.Scenario{
		ID: "twin", Days: len(base),
		Factors:     []scenario.Factor{{ID: "MKT", Kind: scenario.KindMarket}},
		Instruments: []scenario.Instrument{{ID: "F1", Beta: map[string]float64{"MKT": 1}, IdioScale: 1}},
		Baseline:    map[string][]scenario.OHLC{"F1": base},
	}
}

// piecewiseLinear interpolates closes through (day, value) knots.
func piecewiseLinear(days int, knots [][2]float64) []scenario.OHLC {
	out := make([]scenario.OHLC, days)
	k := 0
	for d := 0; d < days; d++ {
		for k+1 < len(knots)-1 && float64(d) >= knots[k+1][0] {
			k++
		}
		a, b := knots[k], knots[k+1]
		v := a[1] + (b[1]-a[1])*(float64(d)-a[0])/(b[0]-a[0])
		out[d] = scenario.OHLC{Open: v, High: v + 1, Low: v - 1, Close: v}
	}
	return out
}

// Round-4 amendment (twin-extreme equivalence): a display whose global max
// flips to a near-equal historical twin peak far outside the slack window
// must be accepted — spec §4.6 requires the historical extremum day to remain
// a LOCAL extremum, not that the global argmax day is preserved.
func TestFidelityTwinExtremumFlipAccepted(t *testing.T) {
	// Twin peaks 0.3% apart, 150 days apart: d50=130 vs d200=129.61.
	base := piecewiseLinear(260, [][2]float64{
		{0, 100}, {50, 130}, {125, 110}, {200, 129.61}, {259, 120},
	})
	sc := oneInstScenario(base)
	disp := make([]scenario.OHLC, len(base))
	copy(disp, base)
	// Nudge the far twin 0.5% higher: display's global max flips to d200.
	c := base[200].Close * 1.005
	disp[200] = scenario.OHLC{Open: c, High: c + 1, Low: c - 1, Close: c}
	if err := VerifyFidelity(sc, map[string][]scenario.OHLC{"F1": disp}); err != nil {
		t.Fatalf("twin-peak flip must be accepted, got: %v", err)
	}
}

// A display extreme landing on a day where the baseline is nowhere near its
// own extreme is a genuine narrative violation and must still be rejected.
func TestFidelityGenuineExtremumDisplacementRejected(t *testing.T) {
	// Baseline peak d50=150; d200 sits ~30% below it — well outside even the
	// round-5 envelope-grounded tolerance (0.18). Small deterministic
	// wiggles give the return series enough variance that the broad bump
	// below does not drag return correlation under the 0.85 floor — the
	// extremum clause must be what rejects this display.
	base := piecewiseLinear(260, [][2]float64{
		{0, 100}, {50, 150}, {120, 110}, {200, 105}, {259, 120},
	})
	for d := range base {
		w := 1 + 0.02*math.Sin(2.1*float64(d))
		base[d].Close *= w
		base[d].Open, base[d].High, base[d].Low = base[d].Close, base[d].Close+1, base[d].Close-1
	}
	sc := oneInstScenario(base)
	// Broad multiplicative bump centered on d200 lifts the display's global
	// max to ~160 there while leaving returns nearly identical day-to-day.
	disp := make([]scenario.OHLC, len(base))
	for d := range base {
		m := math.Exp(0.44 * math.Exp(-math.Pow(float64(d)-200, 2)/(2*25*25)))
		c := base[d].Close * m
		disp[d] = scenario.OHLC{Open: c, High: c + 1, Low: c - 1, Close: c}
	}
	err := VerifyFidelity(sc, map[string][]scenario.OHLC{"F1": disp})
	if err == nil {
		t.Fatal("genuine extremum displacement must be rejected")
	}
	if !strings.Contains(err.Error(), "extremum moved") {
		t.Fatalf("expected the extremum error, got: %v", err)
	}
}

func TestFidelityDirectionExemptForNearFlat(t *testing.T) {
	// A baseline with ~zero net move must not fail on direction sign alone.
	sc := &scenario.Scenario{
		ID: "flat", Days: 260,
		Factors:     []scenario.Factor{{ID: "MKT", Kind: scenario.KindMarket}},
		Instruments: []scenario.Instrument{{ID: "F1", Beta: map[string]float64{"MKT": 1}, IdioScale: 1}},
		Baseline:    map[string][]scenario.OHLC{},
	}
	base := make([]scenario.OHLC, sc.Days)
	prices := make([]scenario.OHLC, sc.Days)
	for d := 0; d < sc.Days; d++ {
		// A single smooth oscillation (one full sine period across the
		// window) so the series has one unique global max and one unique
		// global min — no repeated-tie plateaus for argExtremum to latch
		// onto. base[0] == base[Days-1] exactly, so net ≈ 0 (« 0.10).
		b := 100 + 2*math.Sin(2*math.Pi*float64(d)/float64(sc.Days-1))
		base[d] = scenario.OHLC{Open: b, High: b + 1, Low: b - 1, Close: b}
		prices[d] = scenario.OHLC{Open: b, High: b + 1, Low: b - 1, Close: b}
	}
	// Flip the display's net direction: end 1% below start while tracking base.
	for d := range prices {
		f := 1.0 - 0.02*float64(d)/float64(sc.Days-1)
		prices[d].Close = base[d].Close * f
		prices[d].Open, prices[d].High, prices[d].Low = prices[d].Close, prices[d].Close+1, prices[d].Close-1
	}
	sc.Baseline["F1"] = base
	if err := VerifyFidelity(sc, map[string][]scenario.OHLC{"F1": prices}); err != nil {
		t.Fatalf("near-flat direction flip must be exempt, got: %v", err)
	}
}
