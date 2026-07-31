package engine

import (
	"math"
	"math/rand/v2"
	"strings"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// Factor-ID conventions for NewsEvent.TrueShock / ReportShock keys:
//   - Market shocks always target the literal factor ID "MKT".
//   - Sector shocks target a scenario-defined sector factor ID (e.g. "TECH").
//   - Idiosyncratic (per-instrument) shocks target "IDIO:<instrumentID>",
//     e.g. "IDIO:S1" for instrument "S1" (see scenario.Synthetic and
//     Instrument.Beta, which keys betas the same way).
//
// Every scenario must define both a "MKT" factor and, for each instrument
// that should carry idiosyncratic shocks, an "IDIO:<instrumentID>" factor —
// EvolveFactorStates rejects any TrueShock key that isn't a declared
// scenario.Factor.ID.
type Track string

const (
	TrackHistorical Track = "historical"
	TrackImpact     Track = "impact"
	TrackNoise      Track = "noise"
)

type Media struct {
	ID  string
	Rho float64
}

// MediaTable are the built-in virtual outlets; Rho is reporting fidelity.
var MediaTable = []Media{
	{ID: "wire", Rho: 0.9},    // 通讯社
	{ID: "paper", Rho: 0.75},  // 大报
	{ID: "tv", Rho: 0.6},      // 财经电视
	{ID: "tabloid", Rho: 0.4}, // 小报
	{ID: "forum", Rho: 0.25},  // 论坛传闻
}

type NewsEvent struct {
	Day         int
	Track       Track
	MediaID     string
	TrueShock   map[string]float64
	ReportShock map[string]float64
	Headline    string
	Body        string // LLM copy (plan 4); empty on template fallback
	// English copies (template or LLM); empty falls back to the Chinese field.
	HeadlineEn string
	BodyEn     string
	ClusterID  int // 0 = standalone; shared by 传闻/主事件/追踪 triplets
	// Recap marks the daily market-recap item (historical track, zero
	// shock): the LLM copy pipeline gives it the objective market-wrap
	// persona instead of the on-scene report persona. Not persisted.
	Recap bool
}

const (
	pMarketDaily = 0.15
	pSectorDaily = 0.25
	pIdioDaily   = 0.05
	shockMean    = 0.022
	pPositive    = 0.45
	bigShock     = 0.02
	pFlipInWin   = 0.8
	lamFast      = 0.7
	lamSlow      = 0.99
	fracFast     = 0.65
	clampX       = 0.30
	epsSigma     = 0.004
)

// gamma2 samples Gamma(shape=2, mean=mean) as the sum of two exponentials.
func gamma2(rng *rand.Rand, mean float64) float64 {
	theta := mean / 2
	return -theta * (math.Log(1-rng.Float64()) + math.Log(1-rng.Float64()))
}

func inWindow(sc *scenario.Scenario, day int) *scenario.KeyWindow {
	for i := range sc.KeyWindows {
		w := &sc.KeyWindows[i]
		if day >= w.StartDay && day <= w.EndDay {
			return w
		}
	}
	return nil
}

func signedShock(rng *rand.Rand, sc *scenario.Scenario, day int) float64 {
	mag := gamma2(rng, shockMean)
	sign := -1.0
	if rng.Float64() < pPositive {
		sign = 1.0
	}
	if w := inWindow(sc, day); w != nil && mag > bigShock &&
		int(sign) != w.Direction && rng.Float64() < pFlipInWin {
		sign = float64(w.Direction)
	}
	return sign * mag
}

func report(rng *rand.Rand, rho float64, trueShock map[string]float64) map[string]float64 {
	rep := make(map[string]float64, len(trueShock))
	for f, v := range trueShock {
		rep[f] = rho*v + rng.NormFloat64()*(1-rho)*0.01
	}
	return rep
}

// GenerateShockTimeline builds the impact-track news events for one room.
// Fully deterministic in (scenario, seed).
func GenerateShockTimeline(sc *scenario.Scenario, seed uint64) []NewsEvent {
	rng := Stream(seed, "shocks")
	var sectors []string
	idioOf := map[string]string{}
	for _, f := range sc.Factors {
		if f.Kind == scenario.KindSector {
			sectors = append(sectors, f.ID)
		}
	}
	for _, inst := range sc.Instruments {
		for fid := range inst.Beta {
			if len(fid) > 5 && fid[:5] == "IDIO:" {
				idioOf[inst.ID] = fid
			}
		}
	}
	var evs []NewsEvent
	// Note: a shock emitted on the last scenario day (sc.Days-1) still
	// publishes as news, but EvolveFactorStates applies effects on day+1,
	// which is out of range — such a shock can never move prices. This is
	// intentional flavor (a headline that "hasn't been priced in yet"), not
	// a bug.
	emit := func(day int, shock map[string]float64) {
		m := MediaTable[rng.IntN(len(MediaTable))]
		evs = append(evs, NewsEvent{
			Day: day, Track: TrackImpact, MediaID: m.ID,
			TrueShock: shock, ReportShock: report(rng, m.Rho, shock),
		})
	}
	for d := 0; d < sc.Days; d++ {
		if rng.Float64() < pMarketDaily {
			emit(d, map[string]float64{"MKT": signedShock(rng, sc, d)})
		}
		if len(sectors) > 0 && rng.Float64() < pSectorDaily {
			emit(d, map[string]float64{sectors[rng.IntN(len(sectors))]: signedShock(rng, sc, d)})
		}
		for i := range sc.Instruments {
			inst := &sc.Instruments[i]
			if fid, ok := idioOf[inst.ID]; ok {
				if rng.Float64() < pIdioDaily {
					emit(d, map[string]float64{
						fid: signedShock(rng, sc, d) * inst.IdioScaleOrDefault(),
					})
				}
			}
		}
	}
	return evs
}

const pCluster = 0.6

// ExpandClusters turns big market/sector impact events into 传闻→主事件→追踪
// narratives (spec §4.4). Companions carry only ReportShock (TrueShock nil),
// so factor states and prices are untouched — clusters are pure narrative.
func ExpandClusters(sc *scenario.Scenario, seed uint64, evs []NewsEvent) []NewsEvent {
	rng := Stream(seed, "clusters")
	lowRho := []string{"tabloid", "forum"}
	highRho := []string{"wire", "paper"}
	out := make([]NewsEvent, 0, len(evs)+len(evs)/2)
	nextCluster := 1
	for _, ev := range evs {
		big := false
		if ev.Track == TrackImpact && len(ev.TrueShock) == 1 {
			for f, v := range ev.TrueShock {
				if !strings.HasPrefix(f, "IDIO:") && math.Abs(v) >= bigShock {
					big = true
				}
			}
		}
		if !big || rng.Float64() >= pCluster {
			out = append(out, ev)
			continue
		}
		id := nextCluster
		nextCluster++
		ev.ClusterID = id
		if ev.Day > 0 {
			rumorReport := make(map[string]float64, 1)
			mismatch := 0.5 + rng.Float64() // 幅度错配: ×[0.5, 1.5)
			for f, v := range ev.ReportShock {
				rumorReport[f] = v * mismatch
			}
			out = append(out, NewsEvent{
				Day: ev.Day - 1, Track: TrackImpact,
				MediaID:     lowRho[rng.IntN(len(lowRho))],
				ReportShock: rumorReport, ClusterID: id,
			})
		}
		out = append(out, ev)
		if ev.Day+1 < sc.Days {
			followReport := make(map[string]float64, 1)
			for f, v := range ev.TrueShock {
				followReport[f] = v * 0.9
			}
			out = append(out, NewsEvent{
				Day: ev.Day + 1, Track: TrackImpact,
				MediaID:     highRho[rng.IntN(len(highRho))],
				ReportShock: followReport, ClusterID: id,
			})
		}
	}
	return out
}
