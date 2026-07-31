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
// news-copy pipeline (aliases and factor display names only, never IDs). It
// returns parallel zh/en maps; factors without NameEn fall back to Name, and
// the resolved alias (already English) serves both languages.
func displayNames(sc *scenario.Scenario, seed uint64) (zh, en map[string]string) {
	zh = map[string]string{}
	en = map[string]string{}
	for _, f := range sc.Factors {
		zh[f.ID] = f.Name
		en[f.ID] = f.Name
		if f.NameEn != "" {
			en[f.ID] = f.NameEn
		}
	}
	for _, inst := range sc.Instruments {
		alias := ResolveAlias(seed, inst.ID, inst.Alias, inst.Aliases)
		zh["IDIO:"+inst.ID] = alias
		en["IDIO:"+inst.ID] = alias
	}
	return zh, en
}

// dayMovers returns the zh/en display aliases of day d's biggest gainer and
// loser from pre-generated prices. Ties break toward the earlier-declared
// instrument (strict comparisons), keeping the pick deterministic.
func dayMovers(sc *scenario.Scenario, prices map[string][]scenario.OHLC, d int, zh, en map[string]string) (up, down, upEn, downEn string, ok bool) {
	var best, worst float64
	var upID, downID string
	for _, inst := range sc.Instruments {
		ret, valid := dayLogRet(prices[inst.ID], d)
		if !valid {
			continue
		}
		if !ok || ret > best {
			best, upID = ret, inst.ID
		}
		if !ok || ret < worst {
			worst, downID = ret, inst.ID
		}
		ok = true
	}
	if ok {
		up, down = zh["IDIO:"+upID], zh["IDIO:"+downID]
		upEn, downEn = en["IDIO:"+upID], en["IDIO:"+downID]
	}
	return up, down, upEn, downEn, ok
}

// strongestSector returns the zh/en display names of the sector factor whose
// member instruments (beta map carries the sector ID) had the best mean
// log return on day d. Declaration order breaks ties.
func strongestSector(sc *scenario.Scenario, prices map[string][]scenario.OHLC, d int, zh, en map[string]string) (string, string, bool) {
	best := math.Inf(-1)
	bestID := ""
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
			best, bestID, found = sum/float64(n), f.ID, true
		}
	}
	if !found {
		return "", "", false
	}
	return zh[bestID], en[bestID], true
}

var recapTemplates = []string{
	"昨日复盘：{up}领涨，{down}垫底，{sec}板块最强",
	"收盘综述：{up}涨幅居前，{down}走势最弱，资金聚焦{sec}板块",
	"市场回顾：{up}一枝独秀，{down}明显承压，{sec}板块整体走强",
}

// English parallels of recapTemplates, same index ↔ same meaning. The
// "{up}/{down}/{sec}" placeholder keys are shared with the zh templates.
var recapTemplatesEn = []string{
	"Daily recap: {up} led the tape, {down} finished last, and {sec} was the strongest group",
	"Closing wrap: {up} topped the gainers, {down} was the weakest, with money rotating into {sec}",
	"Market review: {up} stood alone at the top, {down} came under clear pressure, and {sec} firmed across the board",
}

var recapTemplatesNoSector = []string{
	"昨日复盘：{up}领涨，{down}垫底",
	"收盘综述：{up}涨幅居前，{down}走势最弱",
}

var recapTemplatesNoSectorEn = []string{
	"Daily recap: {up} led the tape, {down} finished last",
	"Closing wrap: {up} topped the gainers, {down} was the weakest",
}

// RecapNews emits the daily market recap: a zero-impact, historical-track
// item published on day d+1 summarizing day d — biggest gainer/loser and
// the strongest sector, all read from the pre-generated (public) prices.
func RecapNews(sc *scenario.Scenario, prices map[string][]scenario.OHLC, seed uint64) []NewsEvent {
	rng := Stream(seed, "recap-news")
	zh, en := displayNames(sc, seed)
	var evs []NewsEvent
	last, lastNoSec := -1, -1
	for d := 0; d+1 < sc.Days; d++ {
		up, down, upEn, downEn, ok := dayMovers(sc, prices, d, zh, en)
		if !ok {
			continue
		}
		var headline, headlineEn string
		if sec, secEn, hasSec := strongestSector(sc, prices, d, zh, en); hasSec {
			idx := pickNoRepeat(rng, len(recapTemplates), last)
			last = idx
			headline = strings.NewReplacer(
				"{up}", up, "{down}", down, "{sec}", sec,
			).Replace(recapTemplates[idx])
			headlineEn = strings.NewReplacer(
				"{up}", upEn, "{down}", downEn, "{sec}", secEn,
			).Replace(recapTemplatesEn[idx])
		} else {
			idx := pickNoRepeat(rng, len(recapTemplatesNoSector), lastNoSec)
			lastNoSec = idx
			headline = strings.NewReplacer(
				"{up}", up, "{down}", down,
			).Replace(recapTemplatesNoSector[idx])
			headlineEn = strings.NewReplacer(
				"{up}", upEn, "{down}", downEn,
			).Replace(recapTemplatesNoSectorEn[idx])
		}
		evs = append(evs, NewsEvent{
			Day: d + 1, Track: TrackHistorical, MediaID: "wire",
			Recap: true, Headline: headline, HeadlineEn: headlineEn,
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

// English parallels of noiseTemplates, same index ↔ same meaning.
var noiseTemplatesEn = []string{
	// rumor
	"An unverified takeover rumor is making the rounds on trading desks",
	"Word is a major institution is quietly reshuffling positions, direction unknown",
	"People close to the regulators say the new rules are still under study",
	"Talk of policy tailwinds for an entire industry is everywhere, with no hard evidence yet",
	"Rumor has it a well-known hot-money player has been lying low for months; targets vary by telling",
	// sentiment
	"A prominent analyst warned valuations are stretched, then walked it back to long-term bullish",
	"TV pundit debates heat up as bulls and bears talk past each other",
	"Weekend finance column: should ordinary investors panic? Experts are split",
	"Traders say this tape is costing them sleep",
	"Brokerage lobbies are filling up again, and veterans are back to sharing wisdom",
	"Sentiment gauges diverge again; optimists and pessimists refuse to blink",
	// joke
	"A hedge fund manager's lavish art purchase has desks gossiping",
	"A brokerage research note so dense readers dubbed it astrology",
	"A market-show guest's streak of wrong calls has viewers calling them a contrarian indicator",
	"A trader posted their statement online with a four-word caption: just here to participate",
	"A company renamed itself to chase a hot theme; the stock went nowhere but the jokes went viral",
	// question
	"Bounce or true reversal? Institutional views are sharply divided",
	"Whether volume keeps building is the tape's biggest open question",
	"After this chop at the highs: take profits or keep holding?",
	"Whether the market's leadership will rotate has everyone arguing",
	"Is the low-volume drift consolidation or exhaustion? Nobody agrees",
	"Risk aversion is rising; where money goes next is the focus",
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
				Headline: noiseTemplates[idx], HeadlineEn: noiseTemplatesEn[idx],
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

// English parallels of impactTemplates: %s placeholders are the subject's
// English display name and a direction phrase ("catches a bid" /
// "comes under pressure"). The zh "板块" suffix is folded into natural
// English instead of being concatenated.
var impactTemplatesEn = []string{
	"Mixed read on %s as it %s",
	"News out of %s has multiple sources saying it %s, with opinion clearly split",
	"Talk around %s is heating up; reports suggest it %s",
	"Reports that %s %s drew a cautious market reaction",
}

// FillFallbackCopy gives every headline-less event a template line in zh
// and en. LLM-generated copy (plan 4) overwrites these when available.
func FillFallbackCopy(sc *scenario.Scenario, evs []NewsEvent, seed uint64) {
	rng := Stream(seed, "fallback-copy")
	zhName := map[string]string{}
	enName := map[string]string{}
	for _, f := range sc.Factors {
		zhName[f.ID] = f.Name
		enName[f.ID] = f.Name
		if f.NameEn != "" {
			enName[f.ID] = f.NameEn
		}
	}
	lastImpact, lastNoise := -1, -1
	for i := range evs {
		if evs[i].Headline != "" {
			continue
		}
		switch evs[i].Track {
		case TrackHistorical:
			evs[i].Headline = "市场剧烈波动，交易员情绪紧张"
			evs[i].HeadlineEn = "Wild swings put traders on edge"
		case TrackImpact:
			// 用 ReportShock（而非 TrueShock）措辞, 保持真实/报道解耦
			var top string
			var mag float64
			for f, v := range evs[i].ReportShock {
				if math.Abs(v) > math.Abs(mag) {
					top, mag = f, v
				}
			}
			tone, toneEn := "承压", "comes under pressure"
			if mag > 0 {
				tone, toneEn = "获得提振", "catches a bid"
			}
			idx := pickNoRepeat(rng, len(impactTemplates), lastImpact)
			lastImpact = idx
			headline := fmt.Sprintf(impactTemplates[idx], zhName[top], tone)
			headlineEn := fmt.Sprintf(impactTemplatesEn[idx], enName[top], toneEn)
			if evs[i].ClusterID != 0 && evs[i].TrueShock == nil {
				prefix, prefixEn := "【追踪】", "【Follow-up】"
				if isRumor(evs, i) {
					prefix, prefixEn = "【传闻】", "【Rumor】"
				}
				headline = prefix + headline
				headlineEn = prefixEn + headlineEn
			}
			evs[i].Headline = headline
			evs[i].HeadlineEn = headlineEn
		default:
			idx := pickNoRepeat(rng, len(noiseTemplates), lastNoise)
			lastNoise = idx
			evs[i].Headline = noiseTemplates[idx]
			evs[i].HeadlineEn = noiseTemplatesEn[idx]
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
