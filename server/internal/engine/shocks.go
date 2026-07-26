package engine

import (
	"math"
	"math/rand/v2"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

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

func report(rng *rand.Rand, rho float64, true_ map[string]float64) map[string]float64 {
	rep := make(map[string]float64, len(true_))
	for f, v := range true_ {
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
	emit := func(day int, shock map[string]float64) {
		m := MediaTable[rng.IntN(len(MediaTable))]
		// Sort factors for deterministic JSON marshaling
		shockCopy := make(map[string]float64)
		for f, v := range shock {
			shockCopy[f] = v
		}
		evs = append(evs, NewsEvent{
			Day: day, Track: TrackImpact, MediaID: m.ID,
			TrueShock: shockCopy, ReportShock: report(rng, m.Rho, shock),
		})
	}
	for d := 0; d < sc.Days; d++ {
		if rng.Float64() < pMarketDaily {
			emit(d, map[string]float64{"MKT": signedShock(rng, sc, d)})
		}
		if len(sectors) > 0 && rng.Float64() < pSectorDaily {
			emit(d, map[string]float64{sectors[rng.IntN(len(sectors))]: signedShock(rng, sc, d)})
		}
		for _, inst := range sc.Instruments {
			if fid, ok := idioOf[inst.ID]; ok {
				if rng.Float64() < pIdioDaily {
					emit(d, map[string]float64{fid: signedShock(rng, sc, d)})
				}
			}
		}
	}
	return evs
}
