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

var forumReactionDown = []string{
	"{down}跌麻了，抄底的老铁还好吗",
	"{down}这走势，搁谁谁不迷糊",
	"割在{down}最低点的举个手",
	"{down}还能拿吗，在线等，挺急的",
}

var forumReactionMixed = []string{
	"今天{up}一骑绝尘，{down}却在地板上摩擦",
	"{up}和{down}走出了两个世界",
}

var forumBandwagon = []string{
	"看到{subj}的消息我直接加仓，跟上节奏",
	"{subj}这消息靠谱，我已满仓",
	"利好{subj}？冲了再说，犹豫就会败北",
}

var forumSkeptic = []string{
	"又是{subj}的传闻，上次也是这么说的",
	"{subj}利好？骗我接盘罢了",
	"你们吹{subj}的样子，像极了上一轮",
}

var forumRumor = []string{
	"听说{subj}那边有情况，懂的都懂",
	"{subj}的小道消息都传到这了？",
	"说得有鼻子有眼，{subj}怕不是真有动作",
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

// newsSubject is one display-worthy subject from a day's impact news.
type newsSubject struct {
	name    string
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
	name := displayNames(sc, seed)

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
		subj := name[top]
		if subj == "" {
			continue
		}
		subjectsByDay[ev.Day] = append(subjectsByDay[ev.Day], newsSubject{
			name:    subj,
			isRumor: ev.ClusterID != 0 && ev.TrueShock == nil,
		})
	}

	lastPick := map[string]int{}
	pick := func(family string, pool []string) string {
		idx := pickNoRepeat(rng, len(pool), lastPick[family]-1)
		// lastPick defaults to 0 → -1 initial; store idx+1 so 0 is a valid last.
		lastPick[family] = idx + 1
		return pool[idx]
	}

	posts := make([]ForumPost, 0, sc.Days*4)
	for d := 0; d < sc.Days; d++ {
		n := 2 + rng.IntN(5) // 2..6 posts per day
		subjects := subjectsByDay[d]
		for k := 0; k < n; k++ {
			npc := rng.IntN(len(npcNames))
			persona := personaOf[npc]
			roll := rng.Float64()
			var body string
			switch {
			case d >= 1 && roll < 0.40:
				body = forumReaction(sc, prices, d-1, name, pick)
			case len(subjects) > 0 && roll < 0.75:
				body = forumReply(subjects[rng.IntN(len(subjects))], persona, pick)
			default:
				body = pick("chatter", forumChatter)
			}
			posts = append(posts, ForumPost{
				Day: d, NPCName: npcNames[npc], Persona: persona, Body: body,
			})
		}
	}
	return posts
}

// forumReaction renders a post about day d's biggest mover(s).
func forumReaction(sc *scenario.Scenario, prices map[string][]scenario.OHLC, d int, name map[string]string, pick func(string, []string) string) string {
	up, down, ok := dayMovers(sc, prices, d, name)
	if !ok || up == down { // no data, or a single-instrument world
		return pick("chatter", forumChatter)
	}
	repl := strings.NewReplacer("{up}", up, "{down}", down)
	// Vary the family by day parity so reactions don't all read alike.
	switch d % 3 {
	case 0:
		return repl.Replace(pick("mixed", forumReactionMixed))
	case 1:
		return repl.Replace(pick("up", forumReactionUp))
	default:
		return repl.Replace(pick("down", forumReactionDown))
	}
}

// forumReply renders a bandwagon/skeptic/rumor post about a news subject;
// the NPC's persona steers the family.
func forumReply(subj newsSubject, persona string, pick func(string, []string) string) string {
	repl := strings.NewReplacer("{subj}", subj.name)
	if subj.isRumor && (persona == personaGossip || persona == personaSnark) {
		return repl.Replace(pick("rumor", forumRumor))
	}
	switch persona {
	case personaBull:
		return repl.Replace(pick("bandwagon", forumBandwagon))
	case personaBear, personaSnark:
		return repl.Replace(pick("skeptic", forumSkeptic))
	case personaGossip:
		if subj.isRumor {
			return repl.Replace(pick("rumor", forumRumor))
		}
		return repl.Replace(pick("bandwagon", forumBandwagon))
	default:
		// 吃瓜群众 split evenly between the two stances.
		return repl.Replace(pick("skeptic", forumSkeptic))
	}
}

// ManipulationFollowUps renders 1-3 NPC forum posts reacting to a freshly
// planted rumor about subject (the instrument's per-room display alias),
// recycling the rumor/bandwagon/skeptic families of the pre-generated forum.
// The stream labels must make the draw unique per action (e.g. the acting
// user id and the action count), so repeat manipulations read differently.
func ManipulationFollowUps(seed uint64, day int, subject string, labels ...string) []ForumPost {
	rng := Stream(seed, append([]string{"hype-forum"}, labels...)...)
	subj := newsSubject{name: subject, isRumor: true}
	lastPick := map[string]int{}
	pick := func(family string, pool []string) string {
		idx := pickNoRepeat(rng, len(pool), lastPick[family]-1)
		lastPick[family] = idx + 1
		return pool[idx]
	}
	n := 1 + rng.IntN(3)
	posts := make([]ForumPost, 0, n)
	for k := 0; k < n; k++ {
		npc := rng.IntN(len(npcNames))
		persona := forumPersonas[rng.IntN(len(forumPersonas))]
		posts = append(posts, ForumPost{
			Day: day, NPCName: npcNames[npc], Persona: persona,
			Body: forumReply(subj, persona, pick),
		})
	}
	return posts
}
