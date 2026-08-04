// Package llm batch-generates bilingual (Chinese + English) news copy at
// room creation through any OpenAI-compatible chat-completions endpoint
// (DeepSeek, OpenAI, local proxies). It is the game's only external
// dependency; every failure mode degrades to the engine's template copy,
// never to a failed room.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

type Config struct {
	BaseURL, APIKey, Model string
	Concurrency            int
	Timeout                time.Duration
	// DisableThinking adds DeepSeek's {"thinking":{"type":"disabled"}} to
	// requests. Reasoning models otherwise burn most of the room-creation
	// budget "thinking" about each batch; copy quality stays adequate
	// without it. Off by default — other OpenAI-compatible providers may
	// reject the unknown field.
	DisableThinking bool
}

// FromEnv reads LLM_* variables; nil disables generation entirely.
func FromEnv() *Config {
	base := os.Getenv("LLM_BASE_URL")
	model := os.Getenv("LLM_MODEL")
	if base == "" || model == "" {
		return nil
	}
	cfg := &Config{BaseURL: strings.TrimRight(base, "/"), APIKey: os.Getenv("LLM_API_KEY"),
		Model: model, Concurrency: 4, Timeout: 90 * time.Second}
	if v, err := strconv.Atoi(os.Getenv("LLM_CONCURRENCY")); err == nil && v > 0 {
		cfg.Concurrency = v
	}
	if v, err := strconv.Atoi(os.Getenv("LLM_TIMEOUT_SECS")); err == nil && v > 0 {
		cfg.Timeout = time.Duration(v) * time.Second
	}
	if os.Getenv("LLM_DISABLE_THINKING") == "1" {
		cfg.DisableThinking = true
	}
	return cfg
}

type Generator struct {
	cfg   Config
	httpc *http.Client
}

func New(cfg Config) *Generator {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	return &Generator{cfg: cfg, httpc: &http.Client{Timeout: cfg.Timeout}}
}

const chunkSize = 12

var mediaPersona = map[string]string{
	"wire":    "通讯社快讯：克制、简短、只述事实与官方口径",
	"paper":   "财经大报：分析性，引用多方观点，谨慎下结论",
	"tv":      "财经电视：口播感，短句，有现场氛围",
	"tabloid": "市场小报：耸动标题党，细节夸张但留有余地",
	"forum":   "股民论坛：口语、传闻腔、表情丰富、可信度存疑",
}

// systemPromptTmpl's %s is filled with the scenario's EraHint (see
// systemPromptFor), falling back to a neutral "架空年代" when the scenario
// doesn't set one (e.g. the synthetic test scenario).
const systemPromptTmpl = `你是一款股票模拟游戏的新闻引擎。游戏背景是一个%s的虚构平行世界。为给定的新闻条目同时撰写中文与英文的标题和正文。

铁律：
1. 只使用条目中给出的化名与板块名，绝不出现任何真实公司名、真实人名、具体年份或日期。
2. 按每条的媒体人设写作，风格差异要明显。
3. 措辞含糊、多信源、对冲："据传""接近人士""另有分析师认为"。禁止"应声大涨/大跌"式的看图说话。
4. 条目给出的只是"报道倾向"（利好/利空 + 强弱），不是事实；标题份量与倾向强弱可以错配。
5. 角色为"传闻"的条目要留悬念；"追踪"条目要呼应同组事件并给出多方复盘。
6. 输出严格 JSON 数组，元素形如 {"idx":<原样返回>,"headline":"≤40字","body":"80-160字","headline_en":"English, ≤120 chars","body_en":"English, 160-320 chars"}，不要任何多余文本或代码围栏。

英文写作指引：headline_en 与 body_en 不是中文的逐字翻译，而是同一事实在同一媒体人设下的自然英文写法——通讯社快讯对应 terse wire copy，财经大报对应 measured broadsheet analysis，财经电视对应 punchy broadcast patter，市场小报对应 sensational tabloid headlines，股民论坛对应 slangy forum chatter。英文部分同样遵守盲盒纪律：只用化名与板块名，不出现真实公司名、真实人名、具体年份或数字。`

// defaultEraHint is used when a scenario's EraHint is empty.
const defaultEraHint = "架空年代"

// eraHintOf resolves the scenario's era flavor for prompt formatting.
func eraHintOf(sc *scenario.Scenario) string {
	if sc.EraHint != "" {
		return sc.EraHint
	}
	return defaultEraHint
}

// systemPromptFor formats the system prompt once per FillCopy call with the
// scenario's era flavor.
func systemPromptFor(sc *scenario.Scenario) string {
	return fmt.Sprintf(systemPromptTmpl, eraHintOf(sc))
}

// chat posts one chat-completions request and returns the (code-fence
// stripped) message content; false on any failure — callers degrade to
// template copy.
func (g *Generator) chat(ctx context.Context, sysPrompt, userJSON string) (string, bool) {
	body := map[string]any{
		"model": g.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": userJSON},
		},
		"temperature": 0.9,
	}
	if g.cfg.DisableThinking {
		body["thinking"] = map[string]string{"type": "disabled"}
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return "", false
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		g.cfg.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", false
	}
	req.Header.Set("Content-Type", "application/json")
	if g.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)
	}
	resp, err := g.httpc.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", false
	}
	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil || len(cr.Choices) == 0 {
		return "", false
	}
	content := strings.TrimSpace(cr.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content), true
}

type promptItem struct {
	Idx     int          `json:"idx"`
	Kind    string       `json:"类型"`
	Persona string       `json:"媒体人设"`
	Role    string       `json:"角色,omitempty"`
	Tilt    []promptTilt `json:"报道倾向,omitempty"`
	// Note carries the recap item's template facts (alias/sector names
	// only, no numbers) so the model expands rather than invents them.
	Note string `json:"要点,omitempty"`
}

type promptTilt struct {
	Subject   string `json:"对象"`
	Direction string `json:"方向"`
	Strength  string `json:"强度"`
}

type copyOut struct {
	Idx        int    `json:"idx"`
	Headline   string `json:"headline"`
	Body       string `json:"body"`
	HeadlineEn string `json:"headline_en"`
	BodyEn     string `json:"body_en"`
}

// FillCopy generates copy for all events, chunked with cluster members kept
// together, bounded by cfg.Concurrency in-flight requests. Mutates evs.
func (g *Generator) FillCopy(ctx context.Context, sc *scenario.Scenario, evs []engine.NewsEvent) {
	sysPrompt := systemPromptFor(sc)
	displayName := map[string]string{}
	for _, f := range sc.Factors {
		displayName[f.ID] = f.Name
	}
	for _, inst := range sc.Instruments {
		displayName["IDIO:"+inst.ID] = inst.Alias
	}
	dayDir := dayDirections(sc)

	// Order indexes so cluster members are adjacent, then chunk.
	order := make([]int, 0, len(evs))
	seen := make(map[int]bool, len(evs))
	for i := range evs {
		if seen[i] {
			continue
		}
		if c := evs[i].ClusterID; c != 0 {
			for j := i; j < len(evs); j++ {
				if evs[j].ClusterID == c && !seen[j] {
					order = append(order, j)
					seen[j] = true
				}
			}
		} else {
			order = append(order, i)
			seen[i] = true
		}
	}

	// Build runs: each run is one cluster's indexes (or a single standalone).
	var runs [][]int
	{
		i := 0
		for i < len(order) {
			j := i + 1
			c := evs[order[i]].ClusterID
			if c != 0 {
				for j < len(order) && evs[order[j]].ClusterID == c {
					j++
				}
			}
			runs = append(runs, order[i:j])
			i = j
		}
	}
	// Pack runs into chunks of ≤chunkSize without splitting a run
	// (a run larger than chunkSize gets its own oversized chunk).
	var chunks [][]int
	var cur []int
	for _, run := range runs {
		if len(cur) > 0 && len(cur)+len(run) > chunkSize {
			chunks = append(chunks, cur)
			cur = nil
		}
		cur = append(cur, run...)
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}

	sem := make(chan struct{}, g.cfg.Concurrency)
	done := make(chan int, len(chunks))
	filled := 0
	for _, ch := range chunks {
		sem <- struct{}{}
		go func(ch []int) {
			defer func() { <-sem }()
			done <- g.fillChunk(ctx, sysPrompt, displayName, evs, ch, dayDir)
		}(ch)
	}
	for range chunks {
		filled += <-done
	}
	log.Printf("llm: filled %d/%d news items", filled, len(evs))
}

// minDayDirLog is the mean-log-return floor below which a day's overall
// direction is too tame to call "上涨"/"下跌"; such days get no parenthetical.
const minDayDirLog = 0.002

// dayDirections computes, for each day in the scenario baseline, a
// qualitative market direction word from the mean log return across all
// instruments that day: positive → "上涨", negative → "下跌", empty when the
// magnitude is below minDayDirLog (near-flat day, no direction to report).
// Index 0 is always empty (no prior day to return from).
func dayDirections(sc *scenario.Scenario) []string {
	dirs := make([]string, sc.Days)
	for d := 1; d < sc.Days; d++ {
		var sum float64
		n := 0
		for _, inst := range sc.Instruments {
			p := sc.Baseline[inst.ID]
			if d >= len(p) {
				continue
			}
			sum += math.Log(p[d].Close / p[d-1].Close)
			n++
		}
		if n == 0 {
			continue
		}
		mean := sum / float64(n)
		switch {
		case mean >= minDayDirLog:
			dirs[d] = "上涨"
		case mean <= -minDayDirLog:
			dirs[d] = "下跌"
		}
	}
	return dirs
}

// fillChunk generates copy for one chunk of events in place. Returns the
// number of chunk entries that got at least one field updated: the Chinese
// pair (headline/body) and the English pair (headline_en/body_en) are each
// validated and written back independently, so an entry counts when either
// language lands and the other may stay empty for the engine's template
// fallback.
func (g *Generator) fillChunk(ctx context.Context, sysPrompt string, displayName map[string]string, evs []engine.NewsEvent, idxs []int, dayDir []string) int {
	items := make([]promptItem, 0, len(idxs))
	for _, i := range idxs {
		ev := &evs[i]
		it := promptItem{Idx: i, Persona: mediaPersona[ev.MediaID]}
		switch ev.Track {
		case engine.TrackHistorical:
			if ev.Recap {
				// Daily market recap: objective market-wrap persona, and
				// the template headline carries the facts (biggest
				// gainer/loser aliases, strongest sector) to expand on.
				it.Persona = "市场复盘专栏：客观克制的收盘综述，只陈述公开行情"
				it.Kind = "每日复盘：依据要点扩写客观的市场回顾，不添加观点与预测"
				it.Note = ev.Headline
			} else {
				it.Kind = "行情解读：当日市场出现剧烈波动，为其撰写现场报道"
				if ev.Day > 0 && ev.Day < len(dayDir) && dayDir[ev.Day] != "" {
					it.Kind = fmt.Sprintf("行情解读：当日市场剧烈波动（整体%s），为其撰写现场报道", dayDir[ev.Day])
				}
			}
		case engine.TrackNoise:
			it.Kind = "花边闲谈：与行情无直接关系的市场八卦"
		default:
			it.Kind = "消息面报道"
			for f, v := range ev.ReportShock {
				dir := "利空"
				if v > 0 {
					dir = "利好"
				}
				strength := "弱"
				if math.Abs(v) >= 0.025 {
					strength = "强"
				} else if math.Abs(v) >= 0.01 {
					strength = "中"
				}
				name := displayName[f]
				if name == "" {
					name = f
				}
				it.Tilt = append(it.Tilt, promptTilt{Subject: name, Direction: dir, Strength: strength})
			}
			if ev.ClusterID != 0 {
				switch {
				case ev.TrueShock != nil:
					it.Role = fmt.Sprintf("事件组%d：主事件", ev.ClusterID)
				default:
					it.Role = fmt.Sprintf("事件组%d：传闻或追踪（按前后关系判断）", ev.ClusterID)
				}
			}
		}
		items = append(items, it)
	}
	userJSON, err := json.Marshal(items)
	if err != nil {
		return 0
	}
	content, ok := g.chat(ctx, sysPrompt, string(userJSON))
	if !ok {
		return 0
	}
	var outs []copyOut
	if err := json.Unmarshal([]byte(content), &outs); err != nil {
		return 0
	}
	valid := map[int]bool{}
	for _, i := range idxs {
		valid[i] = true
	}
	n := 0
	for _, o := range outs {
		if !valid[o.Idx] {
			continue
		}
		touched := false
		h := strings.TrimSpace(o.Headline)
		b := strings.TrimSpace(o.Body)
		if h != "" && b != "" && len([]rune(h)) <= 60 && len([]rune(b)) <= 400 {
			evs[o.Idx].Headline = h
			evs[o.Idx].Body = b
			touched = true
		}
		he := strings.TrimSpace(o.HeadlineEn)
		be := strings.TrimSpace(o.BodyEn)
		if he != "" && be != "" && len([]rune(he)) <= 120 && len([]rune(be)) <= 600 {
			evs[o.Idx].HeadlineEn = he
			evs[o.Idx].BodyEn = be
			touched = true
		}
		if touched {
			n++
		}
	}
	return n
}

// forumSystemPromptTmpl's %s is the scenario era hint, same convention as
// systemPromptTmpl. The forum pass rewrites template drafts into fictional
// Agent-persona replies;
// drafts carry the only facts (aliases, sector names) allowed.
const forumSystemPromptTmpl = `你是一款股票模拟游戏的股民论坛写手。游戏背景是一个%s的虚构平行世界。把每条论坛草稿改写成自然的中文与英文回帖。

铁律：
1. 股民论坛回帖语气，20-80字，可阴阳怪气。
2. 禁止真实公司名、真实人名与任何数字。
3. 保留草稿中提到的化名与板块名，不要新增事实、行情判断或具体标的。
4. 按每条的虚构 Agent 性格写作，让狡猾、友善、指导型等口吻有明显区别，但不要声称自己是真人或真实名人。
5. 重点是回应观点、提问和讨论；不得虚构自己买卖、持仓、收益或掌握内幕。
6. 输出严格 JSON 数组，元素形如 {"idx":<原样返回>,"body":"20-80字","body_en":"English, natural forum voice"}，不要任何多余文本或代码围栏。

英文写作指引：body_en 不是中文的逐字翻译，而是同一意思在自然英文论坛腔下的写法（trading-forum slang、sarcasm 均可），同样禁止真实公司名、真实人名与任何数字。`

type forumPromptItem struct {
	Idx     int    `json:"idx"`
	NPC     string `json:"昵称"`
	Persona string `json:"人设"`
	Draft   string `json:"草稿"`
}

type forumCopyOut struct {
	Idx    int    `json:"idx"`
	Body   string `json:"body"`
	BodyEn string `json:"body_en"`
}

// FillForumCopy polishes fictional Agent forum-post bodies (Chinese and English) in
// place, chunked like FillCopy with the same concurrency bound and
// degrade-to-template behavior: any failure leaves the template body
// untouched.
func (g *Generator) FillForumCopy(ctx context.Context, sc *scenario.Scenario, posts []engine.ForumPost) {
	sysPrompt := fmt.Sprintf(forumSystemPromptTmpl, eraHintOf(sc))
	var chunks [][]int
	for i := 0; i < len(posts); i += chunkSize {
		end := i + chunkSize
		if end > len(posts) {
			end = len(posts)
		}
		chunks = append(chunks, []int{i, end})
	}
	sem := make(chan struct{}, g.cfg.Concurrency)
	done := make(chan int, len(chunks))
	filled := 0
	for _, ch := range chunks {
		sem <- struct{}{}
		go func(lo, hi int) {
			defer func() { <-sem }()
			done <- g.fillForumChunk(ctx, sysPrompt, posts, lo, hi)
		}(ch[0], ch[1])
	}
	for range chunks {
		filled += <-done
	}
	log.Printf("llm: polished %d/%d forum posts", filled, len(posts))
}

// fillForumChunk polishes posts[lo:hi] in place; like fillChunk it returns
// the number of posts that got at least one field updated (Chinese body and
// English body validated and written back independently).
func (g *Generator) fillForumChunk(ctx context.Context, sysPrompt string, posts []engine.ForumPost, lo, hi int) int {
	items := make([]forumPromptItem, 0, hi-lo)
	for i := lo; i < hi; i++ {
		items = append(items, forumPromptItem{
			Idx: i, NPC: posts[i].NPCName, Persona: posts[i].Persona, Draft: posts[i].Body,
		})
	}
	userJSON, err := json.Marshal(items)
	if err != nil {
		return 0
	}
	content, ok := g.chat(ctx, sysPrompt, string(userJSON))
	if !ok {
		return 0
	}
	var outs []forumCopyOut
	if err := json.Unmarshal([]byte(content), &outs); err != nil {
		return 0
	}
	n := 0
	for _, o := range outs {
		if o.Idx < lo || o.Idx >= hi {
			continue
		}
		touched := false
		b := strings.TrimSpace(o.Body)
		if b != "" && len([]rune(b)) <= 120 {
			posts[o.Idx].Body = b
			touched = true
		}
		be := strings.TrimSpace(o.BodyEn)
		if be != "" && len([]rune(be)) <= 300 {
			posts[o.Idx].BodyEn = be
			touched = true
		}
		if touched {
			n++
		}
	}
	return n
}
