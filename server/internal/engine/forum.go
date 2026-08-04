package engine

import (
	"math"
	"strings"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// ForumPost is one disclosed Agent-persona forum entry. Persona is a tone
// bucket used only as a hint for the LLM copy pass.
type ForumPost struct {
	Day     int
	NPCName string
	Persona string
	Body    string
	// English copies; empty falls back to the Chinese field.
	NPCNameEn string
	BodyEn    string
	IsAgent   bool
}

// The forum voices are the same five fictional identities seated as built-in
// competitors. They evoke investing archetypes without impersonating real
// people, and remain explicitly labeled as Agents in the client.
var npcNames = []string{
	"西雅图价值客", "硅谷链上哥", "奥马哈复利派", "华尔街逆向姐", "湾区趋势侠",
}

// npcNamesEn is the English handle pool, index-parallel with npcNames
// (npcNamesEn[i] is the English voice of npcNames[i]). Same discipline:
// forum-style handles, no real names, no numbers.
var npcNamesEn = []string{
	"Seattle Value Sage", "Silicon Valley Chain Bull", "Omaha Compounder", "Wall Street Contrarian", "Bay Area Momentum",
}

// A forum voice is a social personality, independent from an Agent's trading
// strategy. Each room assigns all five voices to the five handles without
// replacement, so personalities vary between rooms but remain stable within
// one room. English pools are index-parallel with their Chinese counterparts.
type forumVoice struct {
	Persona              string
	Reaction, ReactionEn []string
	News, NewsEn         []string
	Rumor, RumorEn       []string
	Chatter, ChatterEn   []string
}

const (
	personaCunning  = "狡猾试探型：不轻易亮明观点，喜欢用反问观察别人，但不编造内幕"
	personaFriendly = "友善鼓励型：先共情再讨论，愿意听不同观点，不嘲讽亏损的人"
	personaMentor   = "复盘大哥型：喜欢讲判断框架、仓位纪律和风险，不直接替别人做决定"
	personaCautious = "谨慎风控型：重视证据和下行风险，遇到热闹会提醒大家慢一点"
	personaJester   = "幽默吐槽型：用轻松的玩笑活跃气氛，但不攻击其他玩家"
)

// Template placeholders are public display aliases only. No real names or
// numbers are allowed, matching the news-copy leak discipline.
var forumVoices = []forumVoice{
	{
		Persona: personaCunning,
		Reaction: []string{
			"大家都盯着{up}，我倒想知道是谁最希望我们忽略{down}",
			"{up}这么热闹，先别急着亮底牌，看看{down}那边怎么走",
			"都说{up}强，那谁愿意先讲讲这判断最怕什么？",
		},
		ReactionEn: []string{
			"Everyone is watching {up}; I wonder who benefits from us ignoring {down}",
			"{up} is getting loud, but keep your cards close and watch {down}",
			"Everyone says {up} is strong, so who wants to name what could break that view?",
		},
		News: []string{
			"{subj}这条消息有意思，不过先看看谁在带节奏",
			"关于{subj}，我更想听还没站队的人怎么拆",
			"先别急着给{subj}下结论，评论区里谁最着急？",
		},
		NewsEn: []string{
			"The {subj} story is interesting, but first notice who is pushing it",
			"On {subj}, I would rather hear from someone who has not picked a side",
			"Do not rush to label {subj}; who in here seems most eager?",
		},
		Rumor: []string{
			"{subj}的传闻先放桌上，别急着让别人看见你的牌",
			"谁最早开始传{subj}的？顺着这条线想想挺有意思",
			"{subj}说得这么像真的，反而值得多问一句为什么",
		},
		RumorEn: []string{
			"Leave the {subj} rumor on the table, but do not show everyone your hand yet",
			"Who started the {subj} whisper? Following that trail is more interesting",
			"The {subj} rumor sounds almost too polished, which makes me ask why",
		},
		Chatter: []string{
			"今天大家都很有信心啊，那我先听听不说话的人",
			"观点可以先讲，仓位嘛，还是各自留点神秘感",
			"别急着跟票，先看看最会喊的人自己在怕什么",
		},
		ChatterEn: []string{
			"Everyone sounds confident today; I want to hear from the quiet ones",
			"Share the thesis if you like, but keeping positions private is healthy",
			"Do not follow the loudest voice yet; first ask what they are afraid of",
		},
	},
	{
		Persona: personaFriendly,
		Reaction: []string{
			"拿着{down}的朋友先别慌，咱们也听听看懂{up}的人怎么复盘",
			"{up}走强挺提气，错过也没关系，机会不会只来一次",
			"今天{up}和{down}分化挺大，大家都说说自己的观察吧",
		},
		ReactionEn: []string{
			"If you hold {down}, do not panic; let us hear how the {up} crowd reads today",
			"The strength in {up} is encouraging, but missing it is fine; opportunities return",
			"{up} and {down} split sharply today; I would love to hear everyone's read",
		},
		News: []string{
			"{subj}这条大家怎么看？不同观点都可以讲讲",
			"关注{subj}的朋友别着急，咱们把逻辑一条条捋清楚",
			"{subj}有分歧很正常，欢迎多分享你们看到的依据",
		},
		NewsEn: []string{
			"How does everyone read {subj}? Different views are welcome",
			"If you are following {subj}, no rush; let us unpack the thesis together",
			"Disagreement on {subj} is healthy, so please share what supports your view",
		},
		Rumor: []string{
			"{subj}目前还是传闻，大家可以聊，但别因为焦虑互相带节奏",
			"听到{subj}的消息先记下来，有新证据再一起更新",
			"{subj}这事谁有不同看法都可以说，别有压力",
		},
		RumorEn: []string{
			"{subj} is still a rumor; discuss it, but do not feed each other's anxiety",
			"Note the {subj} whisper for now, and we can update when evidence appears",
			"Any view on {subj} is welcome here, so do not feel pressured",
		},
		Chatter: []string{
			"各位今天还好吗，赚亏都可以说，复盘比面子重要",
			"没抓住行情也别懊恼，先把今天看懂就很有收获",
			"群里有新朋友的话随时提问，大家一起研究",
		},
		ChatterEn: []string{
			"How is everyone doing? Wins or losses are both welcome; reflection matters more than pride",
			"Do not beat yourself up for missing a move; understanding today is already progress",
			"If anyone is new here, ask away and we can study it together",
		},
	},
	{
		Persona: personaMentor,
		Reaction: []string{
			"复盘{up}别只看涨得多，先找驱动，再拿{down}做对照",
			"{up}强、{down}弱只是结果，真正要学的是两边逻辑怎么分叉",
			"看懂{up}之后也别追着跑，先写下失效条件和能承受的风险",
		},
		ReactionEn: []string{
			"When reviewing {up}, do not stop at the gain; find the driver and compare it with {down}",
			"{up} strong and {down} weak are outcomes; the lesson is where their theses diverged",
			"Even after understanding {up}, do not chase; write down invalidation and tolerable risk first",
		},
		News: []string{
			"看{subj}别只看标题，先分清事实、预期和价格",
			"分析{subj}可以先问三件事：证据是什么，市场信了多少，错了怎么办",
			"{subj}的方向不是重点，重点是你的判断有没有可验证的条件",
		},
		NewsEn: []string{
			"Do not judge {subj} by the headline; separate facts, expectations, and price",
			"For {subj}, ask what the evidence is, how much is priced in, and what happens if you are wrong",
			"Direction is not the key with {subj}; a testable thesis is",
		},
		Rumor: []string{
			"传闻看{subj}，先标记来源和能被证伪的节点，别急着下结论",
			"{subj}消息真假未定，先想清楚如果错了怎么控制损失",
			"把{subj}当作待验证假设，不要当成答案，这就是基本功",
		},
		RumorEn: []string{
			"For a {subj} rumor, mark the source and what could disprove it before concluding",
			"The {subj} claim is unverified, so first decide how to limit damage if it is wrong",
			"Treat {subj} as a hypothesis to test, not an answer; that is the basic discipline",
		},
		Chatter: []string{
			"收盘后别先看盈亏，先写下今天哪条判断对、哪条只是运气",
			"仓位是判断的安全带，逻辑再好也别忘了系上",
			"谁愿意发个复盘？我帮你看推理，不替你猜涨跌",
		},
		ChatterEn: []string{
			"After the close, review which calls were sound and which were luck before checking profit",
			"Position sizing is the seat belt for a thesis, so wear it even when the idea looks great",
			"Anyone want to share a review? I can examine the reasoning, not predict the next move",
		},
	},
	{
		Persona: personaCautious,
		Reaction: []string{
			"{up}越热越要看风险，{down}也别因为便宜就急着接",
			"{up}和{down}都走得极端，今天更该慢一点",
			"别被{up}的热闹催着追，也别让{down}的下跌逼着赌反弹",
		},
		ReactionEn: []string{
			"The hotter {up} gets, the more risk matters; do not grab {down} just because it looks cheap",
			"Both {up} and {down} moved to extremes, which is a reason to slow down",
			"Do not let excitement in {up} force a chase or weakness in {down} force a rebound bet",
		},
		News: []string{
			"{subj}先看证据，标题越热闹越要留点余地",
			"讨论{subj}可以，但最好把最坏情况也摆上桌",
			"{subj}如果不及预期会怎样？先回答这个再谈机会",
		},
		NewsEn: []string{
			"Start with evidence on {subj}; the louder the headline, the more room for doubt",
			"We can discuss {subj}, but the downside case belongs on the table too",
			"What if {subj} disappoints? Answer that before discussing opportunity",
		},
		Rumor: []string{
			"{subj}还只是传闻，没证据之前先按不确定处理",
			"听见{subj}的风声可以关注，但别拿情绪当确认",
			"{subj}真假未明，最安全的动作有时就是再等等",
		},
		RumorEn: []string{
			"{subj} is still a rumor, so treat it as uncertainty until evidence arrives",
			"The {subj} whisper is worth noting, but emotion is not confirmation",
			"With {subj} unverified, sometimes the safest move is simply to wait",
		},
		Chatter: []string{
			"今天情绪有点满，大家下单前多留一秒想想退路",
			"看不懂的时候少做一点，也是一种判断",
			"热闹归热闹，现金和耐心也都是仓位",
		},
		ChatterEn: []string{
			"Sentiment feels crowded today; take one more moment to consider the exit before acting",
			"Doing less when the market is unclear is still a decision",
			"The room can be loud, but cash and patience are positions too",
		},
	},
	{
		Persona: personaJester,
		Reaction: []string{
			"{up}在天上飞，{down}在地上躺，今天的市场很会安排节目",
			"看完{up}再看{down}，我的自选列表像在演悲喜剧",
			"{up}负责让人拍腿，{down}负责让人拍桌，分工明确",
		},
		ReactionEn: []string{
			"{up} is flying while {down} lies on the floor; the market booked a full show today",
			"Watching {up} and then {down} turns my watchlist into a tragicomedy",
			"{up} makes people slap their knees and {down} makes them slap the desk; perfect teamwork",
		},
		News: []string{
			"{subj}这标题一出，评论区的键盘都开始冒烟了",
			"{subj}还没走两步，大家已经把大结局写完了",
			"围观{subj}可以，别把表情包当研究报告就行",
		},
		NewsEn: []string{
			"That {subj} headline landed and every keyboard in here started smoking",
			"{subj} barely moved and the room already wrote the season finale",
			"Watching {subj} is fine; just do not mistake memes for research",
		},
		Rumor: []string{
			"{subj}的小道消息跑得比行情快，腿脚真好",
			"{subj}传闻已经进群，再等一会儿可能就要出周边了",
			"听说{subj}有故事，我先搬个凳子，不急着搬仓位",
		},
		RumorEn: []string{
			"The {subj} rumor runs faster than the market; impressive cardio",
			"The {subj} whisper just entered the room and merchandise cannot be far behind",
			"Apparently {subj} has a story; I am moving a chair, not my position",
		},
		Chatter: []string{
			"今天谁赚钱谁请喝茶，谁亏钱我陪你一起假装没看盘",
			"我的交易系统很稳定：买完紧张，卖完后悔",
			"收盘钟一响，所有人的逻辑突然都清晰了",
		},
		ChatterEn: []string{
			"Winners buy the tea today; if you lost, I will help pretend we never checked the market",
			"My trading system is consistent: nervous after buying and regretful after selling",
			"The closing bell rings and suddenly everyone's thesis becomes crystal clear",
		},
	},
}

func forumVoiceAssignment(seed uint64) []forumVoice {
	voices := append([]forumVoice(nil), forumVoices...)
	rng := Stream(seed, "forum-personalities")
	for i := len(voices) - 1; i > 0; i-- {
		j := rng.IntN(i + 1)
		voices[i], voices[j] = voices[j], voices[i]
	}
	return voices
}

// newsSubject is one display-worthy subject from a day's impact news.
type newsSubject struct {
	name    string
	nameEn  string
	isRumor bool
}

// ForumPosts generates the room's fictional Agent forum: 2-6 posts per sim day,
// deterministic in (scenario, prices, news, seed). Content mixes reactions
// to the previous day's movers (public prices), bandwagon/skeptic replies
// to the day's news and cluster rumors (aliases/factor display names
// only), and pure chatter. Bodies are template text; the LLM copy pass may
// polish them later (templates must read acceptably on their own).
func ForumPosts(sc *scenario.Scenario, prices map[string][]scenario.OHLC, news []NewsEvent, seed uint64) []ForumPost {
	rng := Stream(seed, "forum")
	voices := forumVoiceAssignment(seed)
	zh, en := displayNames(sc, seed)

	subjectsByDay := map[int][]newsSubject{}
	for _, ev := range news {
		if ev.Track != TrackImpact || len(ev.ReportShock) == 0 {
			continue
		}
		var top string
		var mag float64
		for f, v := range ev.ReportShock {
			if math.Abs(v) > math.Abs(mag) {
				top, mag = f, v
			}
		}
		subj := zh[top]
		if subj == "" {
			continue
		}
		subjectsByDay[ev.Day] = append(subjectsByDay[ev.Day], newsSubject{
			name:    subj,
			nameEn:  en[top],
			isRumor: ev.ClusterID != 0 && ev.TrueShock == nil,
		})
	}

	lastPick := map[string]int{}
	// pick draws one index per family (stream consumption unchanged) and
	// returns the zh/en template pair at that index.
	pick := func(family string, pool, poolEn []string) (string, string) {
		idx := pickNoRepeat(rng, len(pool), lastPick[family]-1)
		// lastPick defaults to 0 → -1 initial; store idx+1 so 0 is a valid last.
		lastPick[family] = idx + 1
		return pool[idx], poolEn[idx]
	}

	posts := make([]ForumPost, 0, sc.Days*4)
	for d := 0; d < sc.Days; d++ {
		n := 2 + rng.IntN(5) // 2..6 posts per day
		subjects := subjectsByDay[d]
		for k := 0; k < n; k++ {
			npc := rng.IntN(len(npcNames))
			voice := voices[npc]
			roll := rng.Float64()
			var body, bodyEn string
			switch {
			case d >= 1 && roll < 0.40:
				body, bodyEn = forumReaction(sc, prices, d-1, zh, en, voice, pick)
			case len(subjects) > 0 && roll < 0.75:
				body, bodyEn = forumReply(subjects[rng.IntN(len(subjects))], voice, pick)
			default:
				body, bodyEn = forumChatter(voice, pick)
			}
			posts = append(posts, ForumPost{
				Day: d, NPCName: npcNames[npc], Persona: voice.Persona,
				Body: body, NPCNameEn: npcNamesEn[npc], BodyEn: bodyEn, IsAgent: true,
			})
		}
	}
	return posts
}

// forumReaction renders a zh/en post pair about day d's biggest mover(s).
func forumReaction(sc *scenario.Scenario, prices map[string][]scenario.OHLC, d int, zh, en map[string]string, voice forumVoice, pick func(string, []string, []string) (string, string)) (string, string) {
	up, down, upEn, downEn, ok := dayMovers(sc, prices, d, zh, en)
	if !ok || up == down { // no data, or a single-instrument world
		return forumChatter(voice, pick)
	}
	repl := strings.NewReplacer("{up}", up, "{down}", down)
	replEn := strings.NewReplacer("{up}", upEn, "{down}", downEn)
	body, bodyEn := pick("reaction|"+voice.Persona, voice.Reaction, voice.ReactionEn)
	return repl.Replace(body), replEn.Replace(bodyEn)
}

// forumReply renders a news or rumor reply in the Agent's assigned social
// voice. Personality changes the framing, not the public facts in the draft.
func forumReply(subj newsSubject, voice forumVoice, pick func(string, []string, []string) (string, string)) (string, string) {
	repl := strings.NewReplacer("{subj}", subj.name)
	replEn := strings.NewReplacer("{subj}", subj.nameEn)
	family := "news|" + voice.Persona
	pool, poolEn := voice.News, voice.NewsEn
	if subj.isRumor {
		family = "rumor|" + voice.Persona
		pool, poolEn = voice.Rumor, voice.RumorEn
	}
	body, bodyEn := pick(family, pool, poolEn)
	return repl.Replace(body), replEn.Replace(bodyEn)
}

func forumChatter(voice forumVoice, pick func(string, []string, []string) (string, string)) (string, string) {
	return pick("chatter|"+voice.Persona, voice.Chatter, voice.ChatterEn)
}

// ManipulationFollowUps renders 1-3 fictional Agent forum posts reacting to a freshly
// planted rumor about subject (the instrument's per-room display alias),
// recycling the rumor/bandwagon/skeptic families of the pre-generated forum.
// The stream labels must make the draw unique per action (e.g. the acting
// user id and the action count), so repeat manipulations read differently.
func ManipulationFollowUps(seed uint64, day int, subject string, labels ...string) []ForumPost {
	rng := Stream(seed, append([]string{"hype-forum"}, labels...)...)
	voices := forumVoiceAssignment(seed)
	subj := newsSubject{name: subject, nameEn: subject, isRumor: true}
	lastPick := map[string]int{}
	pick := func(family string, pool, poolEn []string) (string, string) {
		idx := pickNoRepeat(rng, len(pool), lastPick[family]-1)
		lastPick[family] = idx + 1
		return pool[idx], poolEn[idx]
	}
	n := 1 + rng.IntN(3)
	posts := make([]ForumPost, 0, n)
	for k := 0; k < n; k++ {
		npc := rng.IntN(len(npcNames))
		voice := voices[npc]
		body, bodyEn := forumReply(subj, voice, pick)
		posts = append(posts, ForumPost{
			Day: day, NPCName: npcNames[npc], Persona: voice.Persona,
			Body: body, NPCNameEn: npcNamesEn[npc], BodyEn: bodyEn, IsAgent: true,
		})
	}
	return posts
}
