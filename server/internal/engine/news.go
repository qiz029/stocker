package engine

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"

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

// pickNoRepeat returns an index in [0,n) different from last using exactly
// one draw (n > 1); pass last = -1 initially. Keeps consecutive template
// picks from repeating while staying deterministic.
func pickNoRepeat(rng *rand.Rand, n, last int) int {
	if n <= 1 {
		return 0
	}
	i := rng.IntN(n - 1)
	if last >= 0 && i >= last {
		i++
	}
	return i
}

// dayLogRet returns an instrument's log return on day d from pre-generated
// prices; day 0 is measured open→close (there is no prior close).
func dayLogRet(p []scenario.OHLC, d int) (float64, bool) {
	if d < 0 || d >= len(p) {
		return 0, false
	}
	base := p[d].Open
	if d > 0 {
		base = p[d-1].Close
	}
	if base <= 0 || p[d].Close <= 0 {
		return 0, false
	}
	return math.Log(p[d].Close / base), true
}

// displayNames maps factor IDs to display names and "IDIO:<instID>" keys to
// the room's resolved instrument alias — the same naming discipline as the
// news-copy pipeline (aliases and factor display names only, never IDs).
func displayNames(sc *scenario.Scenario, seed uint64) map[string]string {
	name := map[string]string{}
	for _, f := range sc.Factors {
		name[f.ID] = f.Name
	}
	for _, inst := range sc.Instruments {
		name["IDIO:"+inst.ID] = ResolveAlias(seed, inst.ID, inst.Alias, inst.Aliases)
	}
	return name
}

// dayMovers returns the display aliases of day d's biggest gainer and loser
// from pre-generated prices. Ties break toward the earlier-declared
// instrument (strict comparisons), keeping the pick deterministic.
func dayMovers(sc *scenario.Scenario, prices map[string][]scenario.OHLC, d int, name map[string]string) (up, down string, ok bool) {
	var best, worst float64
	for _, inst := range sc.Instruments {
		ret, valid := dayLogRet(prices[inst.ID], d)
		if !valid {
			continue
		}
		alias := name["IDIO:"+inst.ID]
		if !ok || ret > best {
			best, up = ret, alias
		}
		if !ok || ret < worst {
			worst, down = ret, alias
		}
		ok = true
	}
	return up, down, ok
}

// strongestSector returns the display name of the sector factor whose
// member instruments (beta map carries the sector ID) had the best mean
// log return on day d. Declaration order breaks ties.
func strongestSector(sc *scenario.Scenario, prices map[string][]scenario.OHLC, d int) (string, bool) {
	best := math.Inf(-1)
	bestName := ""
	found := false
	for _, f := range sc.Factors {
		if f.Kind != scenario.KindSector {
			continue
		}
		var sum float64
		n := 0
		for _, inst := range sc.Instruments {
			if inst.Beta[f.ID] == 0 {
				continue
			}
			if ret, ok := dayLogRet(prices[inst.ID], d); ok {
				sum += ret
				n++
			}
		}
		if n > 0 && sum/float64(n) > best {
			best, bestName, found = sum/float64(n), f.Name, true
		}
	}
	return bestName, found
}

var recapTemplates = []string{
	"昨日复盘：{up}领涨，{down}垫底，{sec}板块最强",
	"收盘综述：{up}涨幅居前，{down}走势最弱，资金聚焦{sec}板块",
	"市场回顾：{up}一枝独秀，{down}明显承压，{sec}板块整体走强",
}

var recapTemplatesNoSector = []string{
	"昨日复盘：{up}领涨，{down}垫底",
	"收盘综述：{up}涨幅居前，{down}走势最弱",
}

// RecapNews emits the daily market recap: a zero-impact, historical-track
// item published on day d+1 summarizing day d — biggest gainer/loser and
// the strongest sector, all read from the pre-generated (public) prices.
func RecapNews(sc *scenario.Scenario, prices map[string][]scenario.OHLC, seed uint64) []NewsEvent {
	rng := Stream(seed, "recap-news")
	name := displayNames(sc, seed)
	var evs []NewsEvent
	last, lastNoSec := -1, -1
	for d := 0; d+1 < sc.Days; d++ {
		up, down, ok := dayMovers(sc, prices, d, name)
		if !ok {
			continue
		}
		var headline string
		if sec, hasSec := strongestSector(sc, prices, d); hasSec {
			idx := pickNoRepeat(rng, len(recapTemplates), last)
			last = idx
			headline = strings.NewReplacer(
				"{up}", up, "{down}", down, "{sec}", sec,
			).Replace(recapTemplates[idx])
		} else {
			idx := pickNoRepeat(rng, len(recapTemplatesNoSector), lastNoSec)
			lastNoSec = idx
			headline = strings.NewReplacer(
				"{up}", up, "{down}", down,
			).Replace(recapTemplatesNoSector[idx])
		}
		evs = append(evs, NewsEvent{
			Day: d + 1, Track: TrackHistorical, MediaID: "wire",
			Recap: true, Headline: headline,
		})
	}
	return evs
}

const pNoiseDaily = 0.15

// noiseTemplates are zero-impact tabloid lines in four families (传闻 /
// 情绪 / 段子 / 设问). No real names, no numbers — ever.
var noiseTemplates = []string{
	// 传闻
	"华尔街交易大厅流传一则未经证实的并购传闻",
	"传闻某大型机构正在悄悄调仓，方向成谜",
	"接近监管层的人士透露，新规仍在研究之中",
	"坊间盛传某行业将迎来政策暖风，目前尚无实锤",
	"有传言称知名游资已潜伏多时，目标众说纷纭",
	// 情绪
	"知名分析师警告市场估值过高，随后改口称长期仍看好",
	"财经名嘴电视辩论升温，多空双方各执一词",
	"周末财经专栏：普通投资者应该恐慌吗？专家意见不一",
	"交易员们纷纷表示，最近的行情让人睡不好觉",
	"营业部大厅人气回暖，老股民又开始讲解心得",
	"市场情绪指标再度陷入分歧，乐观与悲观者互不相让",
	// 段子
	"某对冲基金经理豪掷千金购入艺术品，引发市场闲谈",
	"某券商研报因措辞过于晦涩，被读者调侃为占星术",
	"财经节目嘉宾连续看错方向，被网友封为反向指标",
	"有股民在论坛晒出交割单，配文只有四个字：重在参与",
	"某公司改名蹭热点，股价没动，段子先火了",
	// 设问
	"本轮行情是反弹还是反转？机构观点针锋相对",
	"量能能否持续放大，成为盘面最大悬念",
	"高位震荡之下，落袋为安还是继续持有？",
	"市场风格会否切换，各方争论不休",
	"缩量盘整意味着蓄势还是乏力？众说纷纭",
	"避险情绪升温，资金下一站去向引关注",
}

// NoiseNews sprinkles zero-impact tabloid chatter. Consecutive noise items
// never reuse the same template (pickNoRepeat).
func NoiseNews(sc *scenario.Scenario, seed uint64) []NewsEvent {
	rng := Stream(seed, "noise-news")
	var evs []NewsEvent
	last := -1
	for d := 0; d < sc.Days; d++ {
		if rng.Float64() < pNoiseDaily {
			m := MediaTable[rng.IntN(len(MediaTable))]
			idx := pickNoRepeat(rng, len(noiseTemplates), last)
			last = idx
			evs = append(evs, NewsEvent{
				Day: d, Track: TrackNoise, MediaID: m.ID,
				Headline: noiseTemplates[idx],
			})
		}
	}
	return evs
}

// impactTemplates are the fallback headline variants for the impact track;
// %s placeholders are the subject's display name and a direction word.
var impactTemplates = []string{
	"消息面变化，%s板块%s，市场解读不一",
	"%s板块传来消息，多方信源称%s，分歧明显",
	"围绕%s板块的讨论升温，消息面指其%s",
	"有消息称%s板块%s，市场反应谨慎",
}

// FillFallbackCopy gives every headline-less event a Chinese template line.
// LLM-generated copy (plan 4) overwrites these when available.
func FillFallbackCopy(sc *scenario.Scenario, evs []NewsEvent, seed uint64) {
	rng := Stream(seed, "fallback-copy")
	name := map[string]string{}
	for _, f := range sc.Factors {
		name[f.ID] = f.Name
	}
	lastImpact, lastNoise := -1, -1
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
			idx := pickNoRepeat(rng, len(impactTemplates), lastImpact)
			lastImpact = idx
			headline := fmt.Sprintf(impactTemplates[idx], name[top], tone)
			if evs[i].ClusterID != 0 && evs[i].TrueShock == nil {
				prefix := "【追踪】"
				if isRumor(evs, i) {
					prefix = "【传闻】"
				}
				headline = prefix + headline
			}
			evs[i].Headline = headline
		default:
			idx := pickNoRepeat(rng, len(noiseTemplates), lastNoise)
			lastNoise = idx
			evs[i].Headline = noiseTemplates[idx]
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
