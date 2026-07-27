// Package llm batch-generates news copy at room creation through any
// OpenAI-compatible chat-completions endpoint (DeepSeek, OpenAI, local
// proxies). It is the game's only external dependency; every failure mode
// degrades to the engine's template copy, never to a failed room.
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

const systemPrompt = `你是一款股票模拟游戏的新闻引擎。游戏背景是一个类似上世纪末科技股狂热的虚构平行世界。为给定的新闻条目撰写中文标题和正文。

铁律：
1. 只使用条目中给出的化名与板块名，绝不出现任何真实公司名、真实人名、具体年份或日期。
2. 按每条的媒体人设写作，风格差异要明显。
3. 措辞含糊、多信源、对冲："据传""接近人士""另有分析师认为"。禁止"应声大涨/大跌"式的看图说话。
4. 条目给出的只是"报道倾向"（利好/利空 + 强弱），不是事实；标题份量与倾向强弱可以错配。
5. 角色为"传闻"的条目要留悬念；"追踪"条目要呼应同组事件并给出多方复盘。
6. 输出严格 JSON 数组，元素形如 {"idx":<原样返回>,"headline":"≤40字","body":"80-160字"}，不要任何多余文本或代码围栏。`

type promptItem struct {
	Idx     int          `json:"idx"`
	Kind    string       `json:"类型"`
	Persona string       `json:"媒体人设"`
	Role    string       `json:"角色,omitempty"`
	Tilt    []promptTilt `json:"报道倾向,omitempty"`
}

type promptTilt struct {
	Subject   string `json:"对象"`
	Direction string `json:"方向"`
	Strength  string `json:"强度"`
}

type copyOut struct {
	Idx      int    `json:"idx"`
	Headline string `json:"headline"`
	Body     string `json:"body"`
}

// FillCopy generates copy for all events, chunked with cluster members kept
// together, bounded by cfg.Concurrency in-flight requests. Mutates evs.
func (g *Generator) FillCopy(ctx context.Context, sc *scenario.Scenario, evs []engine.NewsEvent) {
	displayName := map[string]string{}
	for _, f := range sc.Factors {
		displayName[f.ID] = f.Name
	}
	for _, inst := range sc.Instruments {
		displayName["IDIO:"+inst.ID] = inst.Alias
	}

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
			done <- g.fillChunk(ctx, displayName, evs, ch)
		}(ch)
	}
	for range chunks {
		filled += <-done
	}
	log.Printf("llm: filled %d/%d news items", filled, len(evs))
}

func (g *Generator) fillChunk(ctx context.Context, displayName map[string]string, evs []engine.NewsEvent, idxs []int) int {
	items := make([]promptItem, 0, len(idxs))
	for _, i := range idxs {
		ev := &evs[i]
		it := promptItem{Idx: i, Persona: mediaPersona[ev.MediaID]}
		switch ev.Track {
		case engine.TrackHistorical:
			it.Kind = "行情解读：当日市场出现剧烈波动，为其撰写现场报道"
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
	reqBody, err := json.Marshal(map[string]any{
		"model": g.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": string(userJSON)},
		},
		"temperature": 0.9,
	})
	if err != nil {
		return 0
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		g.cfg.BaseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	if g.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)
	}
	resp, err := g.httpc.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0
	}
	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil || len(cr.Choices) == 0 {
		return 0
	}
	content := strings.TrimSpace(cr.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var outs []copyOut
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &outs); err != nil {
		return 0
	}
	valid := map[int]bool{}
	for _, i := range idxs {
		valid[i] = true
	}
	n := 0
	for _, o := range outs {
		h := strings.TrimSpace(o.Headline)
		b := strings.TrimSpace(o.Body)
		if !valid[o.Idx] || h == "" || b == "" || len([]rune(h)) > 60 || len([]rune(b)) > 400 {
			continue
		}
		evs[o.Idx].Headline = h
		evs[o.Idx].Body = b
		n++
	}
	return n
}
