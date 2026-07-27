package engine

import (
	"fmt"
	"math"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

const bigMoveLogRet = 0.06

// HistoricalNews emits zero-impact explanatory coverage for large baseline
// moves, so the world reacts to history instead of staying silent on crash days.
func HistoricalNews(sc *scenario.Scenario) []NewsEvent {
	seen := map[int]bool{}
	var evs []NewsEvent
	for _, inst := range sc.Instruments {
		p := sc.Baseline[inst.ID]
		for d := 1; d < sc.Days; d++ {
			ret := math.Log(p[d].Close / p[d-1].Close)
			if math.Abs(ret) <= bigMoveLogRet || seen[d] {
				continue
			}
			seen[d] = true
			evs = append(evs, NewsEvent{
				Day: d, Track: TrackHistorical, MediaID: "wire",
			})
		}
	}
	return evs
}

const pNoiseDaily = 0.10

var noiseTemplates = []string{
	"知名分析师警告市场估值过高，随后改口称长期仍看好",
	"华尔街交易大厅流传一则未经证实的并购传闻",
	"财经名嘴电视辩论升温，多空双方各执一词",
	"某对冲基金经理豪掷千金购入艺术品，引发市场闲谈",
	"周末财经专栏：普通投资者应该恐慌吗？专家意见不一",
}

// NoiseNews sprinkles zero-impact tabloid chatter.
func NoiseNews(sc *scenario.Scenario, seed uint64) []NewsEvent {
	rng := Stream(seed, "noise-news")
	var evs []NewsEvent
	for d := 0; d < sc.Days; d++ {
		if rng.Float64() < pNoiseDaily {
			m := MediaTable[rng.IntN(len(MediaTable))]
			evs = append(evs, NewsEvent{
				Day: d, Track: TrackNoise, MediaID: m.ID,
				Headline: noiseTemplates[rng.IntN(len(noiseTemplates))],
			})
		}
	}
	return evs
}

// FillFallbackCopy gives every headline-less event a Chinese template line.
// LLM-generated copy (plan 4) overwrites these when available.
func FillFallbackCopy(sc *scenario.Scenario, evs []NewsEvent, seed uint64) {
	rng := Stream(seed, "fallback-copy")
	name := map[string]string{}
	for _, f := range sc.Factors {
		name[f.ID] = f.Name
	}
	for i := range evs {
		if evs[i].Headline != "" {
			continue
		}
		switch evs[i].Track {
		case TrackHistorical:
			evs[i].Headline = "市场剧烈波动，交易员情绪紧张"
		case TrackImpact:
			// 用 ReportShock（而非 TrueShock）措辞, 保持真实/报道解耦
			var top string
			var mag float64
			for f, v := range evs[i].ReportShock {
				if math.Abs(v) > math.Abs(mag) {
					top, mag = f, v
				}
			}
			tone := "承压"
			if mag > 0 {
				tone = "获得提振"
			}
			headline := fmt.Sprintf("消息面变化，%s板块%s，市场解读不一", name[top], tone)
			if evs[i].ClusterID != 0 && evs[i].TrueShock == nil {
				prefix := "【追踪】"
				if isRumor(evs, i) {
					prefix = "【传闻】"
				}
				headline = prefix + headline
			}
			evs[i].Headline = headline
		default:
			evs[i].Headline = noiseTemplates[rng.IntN(len(noiseTemplates))]
		}
	}
}

// isRumor reports whether the cluster companion at index i precedes its
// cluster's main event (events are not yet day-sorted when copy is filled,
// so find the main event by ClusterID).
func isRumor(evs []NewsEvent, i int) bool {
	for j := range evs {
		if evs[j].ClusterID == evs[i].ClusterID && evs[j].TrueShock != nil {
			return evs[i].Day < evs[j].Day
		}
	}
	return false
}
