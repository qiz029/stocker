package engine

import (
	"math"
	"strings"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// ForumPost is one NPC forum entry. Persona is a tone bucket used only as a
// hint for the LLM copy pass — it is neither persisted nor served.
type ForumPost struct {
	Day     int
	NPCName string
	Persona string
	Body    string
	// English copies; empty falls back to the Chinese field.
	NPCNameEn string
	BodyEn    string
}

// npcNames is the fixed handle pool (~30 internet-forum-style names, no
// real names, no numbers). Each room assigns personas to handles from its
// seed, so a handle keeps one voice within a room.
var npcNames = []string{
	"满仓踏空王", "抄底小能手", "涨停敢死队", "韭菜本菜", "空仓观望中",
	"价值老周", "看图炒股", "消息灵通人士", "反向指标本标", "稳健理财阿姨",
	"梭哈研究院院长", "追涨杀跌专业户", "贴吧老潜水员", "财报练习生", "心态崩了呀",
	"躺平收息佬", "波段狂魔", "钻石手本手", "止损纪律委员", "盘感流选手",
	"内幕绝缘体", "理性吃瓜群众", "杠杆小王子", "套牢盘房主", "复盘狂魔",
	"左侧交易爱好者", "右侧跟风群众", "打板小霸王", "长线价投信徒", "短线提款机",
}

// npcNamesEn is the English handle pool, index-parallel with npcNames
// (npcNamesEn[i] is the English voice of npcNames[i]). Same discipline:
// forum-style handles, no real names, no numbers.
var npcNamesEn = []string{
	"MissedItAllMax", "BuyTheDipDan", "LimitUpLenny", "BagHolderBob", "SidelinesSam",
	"ValueInvestorVic", "ChartWatcherWes", "PluggedInPete", "ContrarianCarl", "SteadySaverSue",
	"AllInAcademic", "ChaseAndDumpChad", "LurkerLou", "EarningsTraineeEd", "MeltingDownMoe",
	"DividendDave", "SwingTraderSid", "DiamondHandsDana", "StopLossSteve", "TapeReaderTess",
	"NoInsiderNed", "JustWatchingJune", "LeverageLeo", "UnderwaterUrsula", "RecapRandy",
	"EarlyEntryEli", "BandwagonBeth", "MomentumMike", "LongTermLarry", "ScalpMachineSal",
}

// Persona buckets; assigned to handles per room seed.
const (
	personaBull   = "多头"
	personaBear   = "空头"
	personaSnark  = "阴阳师"
	personaGossip = "消息通"
	personaChill  = "吃瓜群众"
)

var forumPersonas = []string{personaBull, personaBear, personaSnark, personaGossip, personaChill}

// Forum template families. {up}/{down} are instrument aliases, {subj} a
// news subject's display name. No real names, no numbers — the leak
// discipline matches the news copy.
var forumReactionUp = []string{
	"{up}今天也太猛了，大腿已拍肿",
	"{up}这是要起飞？我先上车为敬",
	"早上是谁喊的{up}，出来我谢谢你",
	"{up}涨得我心慌，拿不住啊",
}

// English template families are index-parallel with their zh counterparts
// (same index ↔ same meaning); the {up}/{down}/{subj} placeholder keys are
// shared.
var forumReactionUpEn = []string{
	"{up} was insane today, kicking myself for not loading up",
	"Is {up} about to take off? Hopping on first",
	"Whoever called {up} this morning, come out so I can thank you",
	"{up} is ripping so hard it's making me nervous, I can't hold on",
}

var forumReactionDown = []string{
	"{down}跌麻了，抄底的老铁还好吗",
	"{down}这走势，搁谁谁不迷糊",
	"割在{down}最低点的举个手",
	"{down}还能拿吗，在线等，挺急的",
}

var forumReactionDownEn = []string{
	"{down} got crushed, you dip-buyers holding up?",
	"That price action on {down} would confuse anyone",
	"Raise your hand if you sold {down} at the exact bottom",
	"Can I still hold {down}? Asking online, kinda urgent",
}

var forumReactionMixed = []string{
	"今天{up}一骑绝尘，{down}却在地板上摩擦",
	"{up}和{down}走出了两个世界",
}

var forumReactionMixedEn = []string{
	"{up} left everyone in the dust while {down} got dragged on the floor today",
	"{up} and {down} are living in two different worlds",
}

var forumBandwagon = []string{
	"看到{subj}的消息我直接加仓，跟上节奏",
	"{subj}这消息靠谱，我已满仓",
	"利好{subj}？冲了再说，犹豫就会败北",
}

var forumBandwagonEn = []string{
	"Saw the {subj} news and added immediately, keeping pace",
	"The {subj} news checks out, I'm all in",
	"Bullish for {subj}? Buy first, think later — hesitation is defeat",
}

var forumSkeptic = []string{
	"又是{subj}的传闻，上次也是这么说的",
	"{subj}利好？骗我接盘罢了",
	"你们吹{subj}的样子，像极了上一轮",
}

var forumSkepticEn = []string{
	"Another {subj} rumor, that's what they said last time too",
	"Good news for {subj}? Just bait for my exit liquidity",
	"The way you all hype {subj} feels exactly like the last cycle",
}

var forumRumor = []string{
	"听说{subj}那边有情况，懂的都懂",
	"{subj}的小道消息都传到这了？",
	"说得有鼻子有眼，{subj}怕不是真有动作",
}

var forumRumorEn = []string{
	"Hearing something is up with {subj}, iykyk",
	"Even the {subj} whispers made it all the way here?",
	"It sounds awfully specific, maybe {subj} really is making a move",
}

var forumChatter = []string{
	"今天又是看戏的一天",
	"群里都在赚钱，就我在站岗？",
	"这行情没法玩，删软件保平安",
	"各位今天战况如何，报个到",
	"收盘了，喝口茶压压惊",
	"我悟了，反着买就对了",
	"谁能告诉我现在到底是牛是熊",
	"别看盘了，越看越亏",
}

var forumChatterEn = []string{
	"Another day of just watching the show",
	"Everyone in the chat is making money, am I the only one holding the bag?",
	"This market is unplayable, deleting the app for peace of mind",
	"How did everyone do today? Roll call",
	"Market's closed, time for some tea to calm the nerves",
	"I've achieved enlightenment: just buy the opposite",
	"Can someone tell me whether this is a bull or a bear",
	"Stop staring at the tape, the more you look the more you lose",
}

// newsSubject is one display-worthy subject from a day's impact news.
type newsSubject struct {
	name    string
	nameEn  string
	isRumor bool
}

// ForumPosts generates the room's NPC forum: 2-6 posts per sim day,
// deterministic in (scenario, prices, news, seed). Content mixes reactions
// to the previous day's movers (public prices), bandwagon/skeptic replies
// to the day's news and cluster rumors (aliases/factor display names
// only), and pure chatter. Bodies are template text; the LLM copy pass may
// polish them later (templates must read acceptably on their own).
func ForumPosts(sc *scenario.Scenario, prices map[string][]scenario.OHLC, news []NewsEvent, seed uint64) []ForumPost {
	rng := Stream(seed, "forum")
	// Persona assignment consumes the stream first, so a handle's voice is
	// stable regardless of how many posts the room ends up with.
	personaOf := make([]string, len(npcNames))
	for i := range personaOf {
		personaOf[i] = forumPersonas[rng.IntN(len(forumPersonas))]
	}
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
			persona := personaOf[npc]
			roll := rng.Float64()
			var body, bodyEn string
			switch {
			case d >= 1 && roll < 0.40:
				body, bodyEn = forumReaction(sc, prices, d-1, zh, en, pick)
			case len(subjects) > 0 && roll < 0.75:
				body, bodyEn = forumReply(subjects[rng.IntN(len(subjects))], persona, pick)
			default:
				body, bodyEn = pick("chatter", forumChatter, forumChatterEn)
			}
			posts = append(posts, ForumPost{
				Day: d, NPCName: npcNames[npc], Persona: persona,
				Body: body, NPCNameEn: npcNamesEn[npc], BodyEn: bodyEn,
			})
		}
	}
	return posts
}

// forumReaction renders a zh/en post pair about day d's biggest mover(s).
func forumReaction(sc *scenario.Scenario, prices map[string][]scenario.OHLC, d int, zh, en map[string]string, pick func(string, []string, []string) (string, string)) (string, string) {
	up, down, upEn, downEn, ok := dayMovers(sc, prices, d, zh, en)
	if !ok || up == down { // no data, or a single-instrument world
		return pick("chatter", forumChatter, forumChatterEn)
	}
	repl := strings.NewReplacer("{up}", up, "{down}", down)
	replEn := strings.NewReplacer("{up}", upEn, "{down}", downEn)
	// Vary the family by day parity so reactions don't all read alike.
	var body, bodyEn string
	switch d % 3 {
	case 0:
		body, bodyEn = pick("mixed", forumReactionMixed, forumReactionMixedEn)
	case 1:
		body, bodyEn = pick("up", forumReactionUp, forumReactionUpEn)
	default:
		body, bodyEn = pick("down", forumReactionDown, forumReactionDownEn)
	}
	return repl.Replace(body), replEn.Replace(bodyEn)
}

// forumReply renders a bandwagon/skeptic/rumor zh/en post pair about a news
// subject; the NPC's persona steers the family.
func forumReply(subj newsSubject, persona string, pick func(string, []string, []string) (string, string)) (string, string) {
	repl := strings.NewReplacer("{subj}", subj.name)
	replEn := strings.NewReplacer("{subj}", subj.nameEn)
	render := func(family string, pool, poolEn []string) (string, string) {
		body, bodyEn := pick(family, pool, poolEn)
		return repl.Replace(body), replEn.Replace(bodyEn)
	}
	if subj.isRumor && (persona == personaGossip || persona == personaSnark) {
		return render("rumor", forumRumor, forumRumorEn)
	}
	switch persona {
	case personaBull:
		return render("bandwagon", forumBandwagon, forumBandwagonEn)
	case personaBear, personaSnark:
		return render("skeptic", forumSkeptic, forumSkepticEn)
	case personaGossip:
		if subj.isRumor {
			return render("rumor", forumRumor, forumRumorEn)
		}
		return render("bandwagon", forumBandwagon, forumBandwagonEn)
	default:
		// 吃瓜群众 split evenly between the two stances.
		return render("skeptic", forumSkeptic, forumSkepticEn)
	}
}

// ManipulationFollowUps renders 1-3 NPC forum posts reacting to a freshly
// planted rumor about subject (the instrument's per-room display alias),
// recycling the rumor/bandwagon/skeptic families of the pre-generated forum.
// The stream labels must make the draw unique per action (e.g. the acting
// user id and the action count), so repeat manipulations read differently.
func ManipulationFollowUps(seed uint64, day int, subject string, labels ...string) []ForumPost {
	rng := Stream(seed, append([]string{"hype-forum"}, labels...)...)
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
		persona := forumPersonas[rng.IntN(len(forumPersonas))]
		body, bodyEn := forumReply(subj, persona, pick)
		posts = append(posts, ForumPost{
			Day: day, NPCName: npcNames[npc], Persona: persona,
			Body: body, NPCNameEn: npcNamesEn[npc], BodyEn: bodyEn,
		})
	}
	return posts
}
