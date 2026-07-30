package pipeline

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

type Meta struct {
	Name, RealPeriod string
	Dossiers         map[string]Dossier
}

func BuildMeta(id string) (Meta, error) {
	u, ok := ByID(id)
	if !ok {
		return Meta{}, fmt.Errorf("unknown scenario %q", id)
	}
	m := Meta{Name: u.Name, RealPeriod: u.RealPeriod,
		Dossiers: map[string]Dossier{}}
	for _, spec := range u.Instruments {
		m.Dossiers[spec.ID] = spec.Dossier
	}
	return m, nil
}

const (
	maxMissingFrac = 0.15
	// Perturbation vol decomposition under the spec-locked engine constants
	// (measured directly from engine.GenerateShockTimeline/EvolveFactorStates
	// via a 24-seed Monte Carlo probe over this scenario's own calendar
	// length, spec §4.2): the day-to-day change of a factor's state has
	// std ≈ mktVolPerBeta for MKT, ≈ sectorVolPerBeta for any sector factor
	// (both measured at unit beta; SENT/macro factors are never shocked by
	// the current engine — see internal/engine/shocks.go's `sectors` /
	// `idioOf` collection, which only walks KindSector and idio factors —
	// so betas on SENT/GOLD/OIL/RATE contribute zero perturbation and are
	// correctly excluded here), the fixed per-day display noise epsSigma=
	// 0.004 in engine/shocks.go contributes epsVol = sqrt(2)*epsSigma
	// regardless of beta, and the idio channel at IdioScale=1 contributes
	// idioBaseVol.
	//
	// An earlier version of this formula used one flat sharedVol=0.013
	// constant for every instrument, implicitly assuming β_MKT≈1 and no
	// sector beta. That undercounts the shared channel for real high-beta
	// dot-com names (β_MKT often 1.5–2.4 here) — for those the formula
	// then ADDED idio noise on top of an already-too-large shared
	// component, driving the multi-seed fidelity gate's correlation floor
	// below 0.85 (measured: X02 β_MKT=2.16 → worst-seed corr 0.806..0.841
	// across 12 seeds). It also overcounts the shared channel for
	// low-vol, β_MKT-near-1 real instruments whose own historical vol is
	// close to or below the MKT channel's own unavoidable noise (X14, X21,
	// X22 — see build_test.go / the pipeline README on the fidelity gate
	// diagnosis for this scenario). Weighting the shared term by each
	// instrument's own β_MKT/β_sector (below) fixes the former case
	// without changing behavior for the (majority) near-β=1 instruments it
	// already worked for.
	mktVolPerBeta    = 0.011
	sectorVolPerBeta = 0.0053
	epsVol           = 0.005657 // sqrt(2) * engine.epsSigma(0.004)
	idioBaseVol      = 0.00585
	targetFrac       = 0.40
	// idioScaleMin is spec-locked to 0.4 (Task 5's "Produces" interface:
	// "IdioScale from target = 0.40·σ_i (clamped [0.4, 3])"), enforced by
	// TestBuildScenarioShape. The playbook's round-1 correlation-floor
	// remedy ("reduce idioScaleMin to 0.3 for all") was tried and reverted:
	// it violates this contract, and — since the idio channel is a small
	// contributor next to the shared MKT/sector channel for every
	// instrument this scenario's fidelity gate still fails on (see round-2
	// note above) — measured to have no material effect on any of them.
	idioScaleMin = 0.4
	idioScaleMax = 3.0
	betaClampLo  = -0.5
	betaClampHi  = 3.0
	// budgetFrac is the round-3 per-instrument volatility budget (plan
	// amendment, see task-5-report.md "Round 3"). After betas and IdioScale
	// are computed, each instrument's model-implied TOTAL perturbation vol
	//   v_i = sqrt((β_MKT·mktVolPerBeta)² + (β_sec·sectorVolPerBeta)²
	//              + (IdioScale·idioBaseVol)² + epsVol²)
	// is capped at b_i = budgetFrac·σ_i (σ_i = baseline daily log-return
	// stddev). The return-correlation ceiling σ_i/sqrt(σ_i²+v_i²) then sits
	// at 1/sqrt(1+budgetFrac²) ≈ 0.894, above the 0.85 fidelity gate with
	// margin. When v_i exceeds the budget, every beta-routed channel of that
	// instrument is shrunk by
	//   γ_i = sqrt((b_i² − epsVol²) / (v_i² − epsVol²))
	// — the exact solution for "post-budget total vol = b_i" given that
	// epsVol (the engine's fixed per-day display noise) is NOT routed
	// through betas and therefore cannot be scaled down; in the epsVol→0
	// limit this reduces to the plain γ = b_i/v_i form. γ_i multiplies every
	// Beta entry of the instrument; its "IDIO:<id>" share is carried by
	// IdioScale instead (engine's idio price contribution is
	// Beta[IDIO]·IdioScale·shock, so IdioScale *= γ is identical to
	// Beta[IDIO] *= γ, and keeps the Beta[IDIO]==1 shape invariant). This
	// preserves the factor mix and is game-semantically sound: low-vol
	// instruments respond proportionally less to game news. Estimation is
	// deterministic — plain arithmetic over the round-2 Monte-Carlo-measured
	// channel constants above; no sampling at build time.
	budgetFrac = 0.50
	// minIdioScaleAfterBudget is the degenerate-budget floor for IdioScale
	// (see the "bi <= epsVol" branch below): the shape gate's actual
	// enforced bound (TestAllScenariosShape checks IdioScale ∈ [0.1, 3],
	// not idioScaleMin's 0.4 — see that test). Only reached when an
	// instrument's own historical vol is so low that even zero beta/idio
	// contribution still leaves the engine's fixed per-day noise (epsVol)
	// above its budget target — doesn't happen for any dotcom-2000/
	// crash-1987 instrument, only nifty-1972's N15 (real 1972-75 SPX daily
	// vol 0.0103, below epsVol/budgetFrac; task-5-report.md "N15 vol
	// budget").
	minIdioScaleAfterBudget = 0.1
)

func parseDay(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic("universe date " + s + ": " + err.Error()) // static data; fail loud
	}
	return t
}

// BuildScenario assembles the named scenario from embedded raw data.
// Deterministic and offline. Unknown ids are an error.
func BuildScenario(id string) (*scenario.Scenario, error) {
	u, ok := ByID(id)
	if !ok {
		return nil, fmt.Errorf("unknown scenario %q", id)
	}
	start, end := parseDay(u.WindowStart), parseDay(u.WindowEnd)

	// 1. Trading calendar = the market proxy's trading days inside the window.
	var proxySpec *InstrumentSpec
	for i := range u.Instruments {
		if u.Instruments[i].ID == u.MarketProxy {
			proxySpec = &u.Instruments[i]
			break
		}
	}
	if proxySpec == nil {
		return nil, fmt.Errorf("market proxy %q not found among instruments", u.MarketProxy)
	}
	spxBars, err := RawSeries(proxySpec.Raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", proxySpec.Raw, err)
	}
	var calendar []time.Time
	for _, b := range spxBars {
		if !b.Date.Before(start) && !b.Date.After(end) {
			calendar = append(calendar, b.Date)
		}
	}
	days := len(calendar)
	if days < 700 {
		return nil, fmt.Errorf("calendar too short: %d days", days)
	}
	dayIndex := make(map[string]int, days)
	for i, d := range calendar {
		dayIndex[d.Format("2006-01-02")] = i
	}

	// 2. Per-instrument aligned series, level-scaled into a plausible
	// trading band (see levelScale).
	closes := map[string][]float64{} // instrument id -> scaled closes
	baseline := map[string][]scenario.OHLC{}
	for _, spec := range u.Instruments {
		var bars []Bar
		if spec.Raw != "" {
			raw, err := RawSeries(spec.Raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", spec.ID, err)
			}
			bars, err = alignToCalendar(raw, calendar)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", spec.ID, err)
			}
		} else {
			seed := uint64(0xD07C0)
			for _, c := range spec.ID {
				seed = seed*131 + uint64(c)
			}
			bars, err = Reconstruct(spec.Anchors, calendar, seed)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", spec.ID, err)
			}
		}
		scale := levelScale(bars[0].Close)
		series := make([]scenario.OHLC, days)
		cl := make([]float64, days)
		for i, b := range bars {
			series[i] = scenario.OHLC{
				Open: b.Open * scale, High: b.High * scale,
				Low: b.Low * scale, Close: b.Close * scale,
			}
			cl[i] = series[i].Close
		}
		baseline[spec.ID] = series
		closes[spec.ID] = cl
	}

	// 3. Log-return series and factor proxies.
	rets := map[string][]float64{}
	for id, cl := range closes {
		rets[id] = logReturns(cl)
	}
	mkt := rets[u.MarketProxy]
	sectorRet := map[string][]float64{}
	for _, sec := range u.Sectors {
		var members [][]float64
		for _, spec := range u.Instruments {
			if spec.Sector == sec.ID {
				members = append(members, rets[spec.ID])
			}
		}
		if len(members) == 0 {
			return nil, fmt.Errorf("sector %s has no members", sec.ID)
		}
		sectorRet[sec.ID] = meanSeries(members)
	}

	// 4. Betas: market OLS, then sector OLS on residuals.
	sectorResid := map[string][]float64{}
	for id, sr := range sectorRet {
		b := olsBeta(sr, mkt)
		sectorResid[id] = residual(sr, mkt, b)
	}
	instruments := make([]scenario.Instrument, 0, len(u.Instruments))
	factorIDs := map[string]bool{}
	for _, spec := range u.Instruments {
		r := rets[spec.ID]
		bMkt := clampF(olsBeta(r, mkt), betaClampLo, betaClampHi)
		beta := map[string]float64{"MKT": bMkt, "IDIO:" + spec.ID: 1}
		var bSec float64
		if spec.Sector != "" {
			res := residual(r, mkt, bMkt)
			bSec = clampF(olsBeta(res, sectorResid[spec.Sector]), betaClampLo, betaClampHi)
			beta[spec.Sector] = bSec
		}
		for f, v := range spec.ExtraBeta {
			beta[f] = v
		}
		sigma := stddev(r)
		target := targetFrac * sigma
		// Instrument-specific shared-channel noise: the MKT and sector
		// factor states are shared across every instrument, so their
		// contribution to THIS instrument's perturbation scales with its
		// own beta (see the const block above for the measurement this is
		// based on). epsVol is the engine's fixed per-day display noise,
		// present regardless of beta.
		sharedEff := math.Sqrt(bMkt*bMkt*mktVolPerBeta*mktVolPerBeta +
			bSec*bSec*sectorVolPerBeta*sectorVolPerBeta + epsVol*epsVol)
		var scale float64
		if target*target > sharedEff*sharedEff {
			scale = math.Sqrt(target*target-sharedEff*sharedEff) / idioBaseVol
		} else {
			scale = idioScaleMin
		}
		scale = clampF(scale, idioScaleMin, idioScaleMax)
		// Round-3 volatility budgeting (see budgetFrac above). scalable2 is
		// the beta-routed part of the perturbation variance, i.e. v_i²−epsVol².
		scalable2 := bMkt*bMkt*mktVolPerBeta*mktVolPerBeta +
			bSec*bSec*sectorVolPerBeta*sectorVolPerBeta +
			scale*scale*idioBaseVol*idioBaseVol
		vi := math.Sqrt(scalable2 + epsVol*epsVol)
		bi := budgetFrac * sigma
		if vi > bi && scalable2 > 0 {
			if bi > epsVol {
				gamma := math.Sqrt((bi*bi - epsVol*epsVol) / scalable2)
				for f := range beta {
					if f == "IDIO:"+spec.ID {
						continue // carried by IdioScale below, see budgetFrac doc
					}
					beta[f] *= gamma
				}
				scale *= gamma
			} else {
				// Round-4 amendment (task-5-report.md, nifty-1972's N15):
				// bi is analytically unreachable via any uniform gamma
				// (bi²−epsVol² < 0 — the engine's fixed display noise alone
				// already exceeds this instrument's budget target). Rather
				// than erroring, or shrinking every channel by the same
				// factor, zero the beta-routed channels (MKT/sector — these
				// carry no lower-bound contract) entirely and let IdioScale
				// settle at the shape gate's enforced floor
				// (minIdioScaleAfterBudget, not further reducible). Since
				// mktVolPerBeta/sectorVolPerBeta > idioBaseVol, this
				// minimizes total perturbation vol — and hence maximizes
				// the achievable return-correlation — subject to the one
				// binding constraint (IdioScale ∈ [0.1, 3]) the shape gate
				// imposes; a uniform gamma wastes headroom shrinking the
				// already-small idio channel instead of fully eliminating
				// the larger MKT/sector ones.
				//
				// This branch has no other notice mechanism, so a future
				// scenario tripping it on a NON-index instrument (whose
				// MKT/sector betas silently going to zero would be a real
				// modeling bug, not the expected outcome) would otherwise
				// pass silently. Log it here; TestAllScenariosShape asserts
				// it only ever fires for a universe's own MarketProxy.
				log.Printf("pipeline: %s/%s vol budget below engine noise floor — betas zeroed, idio floored (news-inert)",
					u.ScenarioID, spec.ID)
				beta["MKT"] = 0
				if spec.Sector != "" {
					beta[spec.Sector] = 0
				}
				scale = minIdioScaleAfterBudget
			}
		}
		instruments = append(instruments, scenario.Instrument{
			ID: spec.ID, Alias: spec.Dossier.Alias, Desc: spec.Dossier.Desc,
			Aliases: spec.Dossier.Aliases,
			Beta:    beta, IdioScale: scale, Reconstructed: spec.Raw == "",
		})
		for f := range beta {
			factorIDs[f] = true
		}
	}

	// 5. Factor declarations in stable order.
	factors := []scenario.Factor{
		{ID: "MKT", Name: "大盘", Kind: scenario.KindMarket},
		{ID: "SENT", Name: "风险情绪", Kind: scenario.KindSentiment},
	}
	for _, sec := range u.Sectors {
		factors = append(factors, scenario.Factor{ID: sec.ID, Name: sec.Name, Kind: scenario.KindSector})
	}
	for _, mac := range u.Macros {
		factors = append(factors, scenario.Factor{ID: mac.ID, Name: mac.Name, Kind: scenario.KindMacro})
	}
	for _, spec := range u.Instruments {
		factors = append(factors, scenario.Factor{
			ID: "IDIO:" + spec.ID, Name: spec.Dossier.Alias, Kind: scenario.KindIdio,
		})
	}

	// 6. Key windows → day indexes.
	var windows []scenario.KeyWindow
	for _, w := range u.KeyWindows {
		si, ok1 := dayIndex[w.Start]
		ei, ok2 := dayIndex[w.End]
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("key window %s..%s not on trading calendar", w.Start, w.End)
		}
		windows = append(windows, scenario.KeyWindow{StartDay: si, EndDay: ei, Direction: w.Direction})
	}

	// 7. Every beta key must reference a declared factor (the engine
	// rejects unknown factor ids at world-generation time — catch it here
	// with a clearer message).
	declared := map[string]bool{}
	for _, f := range factors {
		declared[f.ID] = true
	}
	for f := range factorIDs {
		if !declared[f] {
			return nil, fmt.Errorf("beta references undeclared factor %q", f)
		}
	}

	return &scenario.Scenario{
		ID: u.ScenarioID, Days: days, EraHint: u.EraHint,
		MarketProxy: u.MarketProxy,
		Factors:     factors, Instruments: instruments,
		KeyWindows: windows, Baseline: baseline,
	}, nil
}

// levelScale picks the whole-series scale factor for an instrument's
// baseline so the day-0 close sits at a plausible trading level, replacing
// the old "every stock opens at exactly 100" normalization:
//
//   - raw day-0 close inside [2, 500] → factor 1: keep the real level as-is.
//   - below 2 (penny stock)           → scale up so the day-0 close lands
//     in [2, 10): target = 2 + 8*frac.
//   - above 500 (e.g. a split-less index or Berkshire-style price) → scale
//     down into [100, 500): target = 100 + 400*frac.
//
// frac is the raw close's fractional part (close - floor(close)) — a pure
// function of the input, so the result is deterministic with no rng.
// Reconstructed (anchor-based) series pass through the same clamp, keeping
// whatever level their anchors produce when it is already inside the band.
// This is a pure level change: every downstream consumer of the baseline
// (log returns, betas, vol, news thresholds, fidelity gates) is
// ratio-based and unaffected.
func levelScale(close float64) float64 {
	const lo, hi = 2.0, 500.0
	if close >= lo && close <= hi {
		return 1
	}
	frac := close - math.Floor(close)
	if close < lo {
		return (lo + 8*frac) / close
	}
	return (100 + 400*frac) / close
}

func alignToCalendar(bars []Bar, calendar []time.Time) ([]Bar, error) {
	byDate := make(map[string]Bar, len(bars))
	for _, b := range bars {
		byDate[b.Date.Format("2006-01-02")] = b
	}
	out := make([]Bar, len(calendar))
	missing := 0
	var last Bar
	for i, d := range calendar {
		if b, ok := byDate[d.Format("2006-01-02")]; ok {
			out[i] = b
			last = b
		} else {
			if i == 0 {
				return nil, fmt.Errorf("no data on window start %s", d.Format("2006-01-02"))
			}
			missing++
			ff := last
			ff.Date = d
			ff.Open, ff.High, ff.Low = last.Close, last.Close, last.Close
			out[i] = ff
		}
	}
	if frac := float64(missing) / float64(len(calendar)); frac > maxMissingFrac {
		return nil, fmt.Errorf("%.0f%% of days missing", frac*100)
	}
	return out, nil
}

func logReturns(cl []float64) []float64 {
	out := make([]float64, len(cl)-1)
	for i := 1; i < len(cl); i++ {
		out[i-1] = math.Log(cl[i] / cl[i-1])
	}
	return out
}

func meanSeries(series [][]float64) []float64 {
	out := make([]float64, len(series[0]))
	for i := range out {
		var s float64
		for _, sr := range series {
			s += sr[i]
		}
		out[i] = s / float64(len(series))
	}
	return out
}

func olsBeta(y, x []float64) float64 {
	var sx, sy, sxx, sxy float64
	n := float64(len(x))
	for i := range x {
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
	}
	den := sxx - sx*sx/n
	if den == 0 {
		return 0
	}
	return (sxy - sx*sy/n) / den
}

func residual(y, x []float64, beta float64) []float64 {
	out := make([]float64, len(y))
	for i := range y {
		out[i] = y[i] - beta*x[i]
	}
	return out
}

func stddev(x []float64) float64 {
	var s, s2 float64
	n := float64(len(x))
	for _, v := range x {
		s += v
		s2 += v * v
	}
	return math.Sqrt(s2/n - (s/n)*(s/n))
}

func clampF(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}
