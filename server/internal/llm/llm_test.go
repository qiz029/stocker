package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

func testScenario() *scenario.Scenario {
	sc := scenario.Synthetic()
	sc.Instruments[0].Alias = "Ridgeline Networks"
	return sc
}

func chatResponse(items string) string {
	body := map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": items}}},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func TestFillCopyHappyPath(t *testing.T) {
	var gotAuth, gotModel, gotBodies, gotSystem atomic.Value
	gotBodies.Store([]string{})
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		var req struct {
			Model    string           `json:"model"`
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel.Store(req.Model)
		gotSystem.Store(req.Messages[0]["content"].(string))
		// Echo back copy for every idx mentioned in the user message.
		user := req.Messages[len(req.Messages)-1]["content"].(string)
		mu.Lock()
		bodies := gotBodies.Load().([]string)
		gotBodies.Store(append(bodies, user))
		mu.Unlock()
		var out []map[string]any
		for idx := 0; idx < 100; idx++ {
			if strings.Contains(user, fmt.Sprintf(`"idx":%d`, idx)) {
				out = append(out, map[string]any{
					"idx": idx, "headline": fmt.Sprintf("生成标题%d", idx),
					"body":        fmt.Sprintf("生成正文%d。", idx),
					"headline_en": fmt.Sprintf("Generated headline %d", idx),
					"body_en":     fmt.Sprintf("Generated body %d.", idx),
				})
			}
		}
		b, _ := json.Marshal(out)
		fmt.Fprint(w, chatResponse(string(b)))
	}))
	defer srv.Close()

	g := New(Config{BaseURL: srv.URL, APIKey: "sk-test", Model: "deepseek-chat",
		Concurrency: 2, Timeout: 10 * time.Second})
	sc := testScenario()
	sc.EraHint = "类似某个狂热"
	evs := []engine.NewsEvent{
		{Day: 3, Track: engine.TrackImpact, MediaID: "wire",
			ReportShock: map[string]float64{"MKT": -0.03}, Headline: "模板标题"},
		{Day: 5, Track: engine.TrackNoise, MediaID: "forum", Headline: "模板花边"},
		// Day 150 is the synthetic scenario's scripted crash-start gap-down
		// (drift -0.15 across S1-S5), so the baseline's mean day-150 log
		// return is unambiguously negative — exercises the historical-track
		// direction wording without depending on RNG noise.
		{Day: 150, Track: engine.TrackHistorical, MediaID: "wire", Headline: "模板历史"},
	}
	g.FillCopy(context.Background(), sc, evs)

	if evs[0].Headline != "生成标题0" || evs[0].Body != "生成正文0。" {
		t.Fatalf("item 0 not filled: %+v", evs[0])
	}
	if evs[0].HeadlineEn != "Generated headline 0" || evs[0].BodyEn != "Generated body 0." {
		t.Fatalf("item 0 English copy not filled: %+v", evs[0])
	}
	if evs[1].Headline != "生成标题1" || evs[1].HeadlineEn != "Generated headline 1" {
		t.Fatalf("item 1 not filled: %+v", evs[1])
	}
	if gotAuth.Load() != "Bearer sk-test" {
		t.Fatalf("auth header: %v", gotAuth.Load())
	}
	if gotModel.Load() != "deepseek-chat" {
		t.Fatalf("model: %v", gotModel.Load())
	}
	var sawDirection bool
	for _, body := range gotBodies.Load().([]string) {
		if strings.Contains(body, "上涨") || strings.Contains(body, "下跌") {
			sawDirection = true
		}
	}
	if !sawDirection {
		t.Fatal("historical item's marshaled Kind missing day direction (上涨/下跌)")
	}
	if sys := gotSystem.Load().(string); !strings.Contains(sys, "类似某个狂热") {
		t.Fatalf("system prompt missing era hint: %s", sys)
	}
}

func TestFillCopyPartialBilingualDegradation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, chatResponse(`[
			{"idx":0,"headline":"中文标题","body":"中文正文"},
			{"idx":1,"headline_en":"English headline","body_en":"English body"},
			{"idx":2}
		]`))
	}))
	defer srv.Close()
	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 1, Timeout: 5 * time.Second})
	evs := []engine.NewsEvent{
		{Day: 1, Track: engine.TrackNoise, MediaID: "forum", Headline: "模板0"},
		{Day: 1, Track: engine.TrackNoise, MediaID: "forum", Headline: "模板1"},
		{Day: 1, Track: engine.TrackNoise, MediaID: "forum", Headline: "模板2"},
	}
	g.FillCopy(context.Background(), testScenario(), evs)

	// zh only: zh written back, en left empty for the engine fallback.
	if evs[0].Headline != "中文标题" || evs[0].Body != "中文正文" {
		t.Fatalf("item 0 zh not filled: %+v", evs[0])
	}
	if evs[0].HeadlineEn != "" || evs[0].BodyEn != "" {
		t.Fatalf("item 0 en must stay empty: %+v", evs[0])
	}
	// en only: en written back, zh template retained.
	if evs[1].HeadlineEn != "English headline" || evs[1].BodyEn != "English body" {
		t.Fatalf("item 1 en not filled: %+v", evs[1])
	}
	if evs[1].Headline != "模板1" {
		t.Fatalf("item 1 zh template must be retained: %+v", evs[1])
	}
	// neither: template retained on both sides.
	if evs[2].Headline != "模板2" || evs[2].HeadlineEn != "" || evs[2].BodyEn != "" {
		t.Fatalf("item 2 must be fully untouched: %+v", evs[2])
	}
}

func TestFillCopyNeverLeaksNumbersOrTruth(t *testing.T) {
	var captured atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		b, _ := json.Marshal(req)
		captured.Store(string(b))
		fmt.Fprint(w, chatResponse("[]"))
	}))
	defer srv.Close()
	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 1, Timeout: 5 * time.Second})
	evs := []engine.NewsEvent{{
		Day: 3, Track: engine.TrackImpact, MediaID: "tabloid",
		TrueShock:   map[string]float64{"MKT": -0.031415},
		ReportShock: map[string]float64{"MKT": -0.027182},
		Headline:    "t",
	}}
	g.FillCopy(context.Background(), testScenario(), evs)
	req := captured.Load().(string)
	for _, leak := range []string{"0.031415", "0.027182", "TrueShock", "true_shock"} {
		if strings.Contains(req, leak) {
			t.Fatalf("prompt leaks %q", leak)
		}
	}
	if !strings.Contains(req, "利空") {
		t.Fatal("qualitative direction missing from prompt")
	}
}

func TestFillCopySurvivesBadResponsesAndTimeouts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, chatResponse("这不是JSON"))
	}))
	defer srv.Close()
	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 1, Timeout: 5 * time.Second})
	evs := []engine.NewsEvent{{Day: 1, Track: engine.TrackNoise, MediaID: "forum", Headline: "模板"}}
	g.FillCopy(context.Background(), testScenario(), evs)
	if evs[0].Headline != "模板" || evs[0].Body != "" {
		t.Fatalf("bad response must leave template intact: %+v", evs[0])
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer slow.Close()
	g2 := New(Config{BaseURL: slow.URL, Model: "m", Concurrency: 1, Timeout: 200 * time.Millisecond})
	start := time.Now()
	g2.FillCopy(context.Background(), testScenario(), evs)
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout not honored")
	}
	if evs[0].Headline != "模板" {
		t.Fatal("timeout must leave template intact")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "")
	if FromEnv() != nil {
		t.Fatal("unset base url must yield nil")
	}
	t.Setenv("LLM_BASE_URL", "https://api.deepseek.com")
	t.Setenv("LLM_MODEL", "deepseek-chat")
	t.Setenv("LLM_API_KEY", "sk-x")
	cfg := FromEnv()
	if cfg == nil || cfg.Model != "deepseek-chat" || cfg.Concurrency != 4 || cfg.Timeout != 90*time.Second {
		t.Fatalf("cfg: %+v", cfg)
	}
}

func TestClusterAwareChunking(t *testing.T) {
	// Capture each request's user message to verify cluster members are in the same request.
	var requestBodies atomic.Value
	requestBodies.Store([]string{})
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string           `json:"model"`
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		userMsg := req.Messages[len(req.Messages)-1]["content"].(string)
		mu.Lock()
		bodies := requestBodies.Load().([]string)
		bodies = append(bodies, userMsg)
		requestBodies.Store(bodies)
		mu.Unlock()

		// Echo back copy for every idx mentioned.
		var out []map[string]any
		for idx := 0; idx < 100; idx++ {
			if strings.Contains(userMsg, fmt.Sprintf(`"idx":%d`, idx)) {
				out = append(out, map[string]any{
					"idx": idx, "headline": fmt.Sprintf("标题%d", idx),
					"body": fmt.Sprintf("正文%d", idx),
				})
			}
		}
		b, _ := json.Marshal(out)
		fmt.Fprint(w, chatResponse(string(b)))
	}))
	defer srv.Close()

	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 2, Timeout: 10 * time.Second})
	sc := testScenario()

	// Create 11 standalone events + 3-member cluster to trigger split at chunkSize boundary.
	evs := make([]engine.NewsEvent, 14)
	for i := 0; i < 11; i++ {
		evs[i] = engine.NewsEvent{
			Day: i, Track: engine.TrackImpact, MediaID: "wire",
			ReportShock: map[string]float64{"MKT": 0.01},
			Headline:    "模板",
		}
	}
	// Add 3-member cluster with IDs 11, 12, 13.
	for i := 11; i < 14; i++ {
		evs[i] = engine.NewsEvent{
			Day: i, Track: engine.TrackImpact, MediaID: "wire",
			ClusterID:   1,
			ReportShock: map[string]float64{"MKT": 0.01},
			Headline:    "模板",
		}
		if i == 11 {
			evs[i].TrueShock = map[string]float64{"MKT": 0.01}
		}
	}

	g.FillCopy(context.Background(), sc, evs)

	// Verify all 3 cluster members appear in the same request.
	clusterIdxsFound := false
	bodies := requestBodies.Load().([]string)
	for _, body := range bodies {
		hasIdx11 := strings.Contains(body, `"idx":11`)
		hasIdx12 := strings.Contains(body, `"idx":12`)
		hasIdx13 := strings.Contains(body, `"idx":13`)
		if hasIdx11 && hasIdx12 && hasIdx13 {
			clusterIdxsFound = true
			break
		}
	}
	if !clusterIdxsFound {
		t.Fatalf("cluster members not in same request; requests: %v", bodies)
	}
}

func TestDisableThinkingFieldInRequest(t *testing.T) {
	var captured atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		b, _ := json.Marshal(req)
		captured.Store(string(b))
		fmt.Fprint(w, chatResponse("[]"))
	}))
	defer srv.Close()

	evs := []engine.NewsEvent{{Day: 1, Track: engine.TrackNoise, MediaID: "forum", Headline: "模板"}}

	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 1,
		Timeout: 5 * time.Second, DisableThinking: true})
	g.FillCopy(context.Background(), testScenario(), evs)
	if !strings.Contains(captured.Load().(string), `"thinking":{"type":"disabled"}`) {
		t.Fatal("thinking-disabled field missing when DisableThinking is set")
	}

	g2 := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 1, Timeout: 5 * time.Second})
	g2.FillCopy(context.Background(), testScenario(), evs)
	if strings.Contains(captured.Load().(string), `"thinking"`) {
		t.Fatal("thinking field must be absent by default")
	}
}

func TestFromEnvDisableThinking(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "https://api.example.com")
	t.Setenv("LLM_MODEL", "m")
	t.Setenv("LLM_DISABLE_THINKING", "1")
	cfg := FromEnv()
	if cfg == nil || !cfg.DisableThinking {
		t.Fatalf("LLM_DISABLE_THINKING=1 not honored: %+v", cfg)
	}
	t.Setenv("LLM_DISABLE_THINKING", "")
	if cfg := FromEnv(); cfg == nil || cfg.DisableThinking {
		t.Fatalf("unset must default off: %+v", cfg)
	}
}

func TestFillForumCopyHappyPath(t *testing.T) {
	var gotSystem atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotSystem.Store(req.Messages[0]["content"].(string))
		user := req.Messages[len(req.Messages)-1]["content"].(string)
		var out []map[string]any
		for idx := 0; idx < 100; idx++ {
			if strings.Contains(user, fmt.Sprintf(`"idx":%d`, idx)) {
				out = append(out, map[string]any{
					"idx": idx, "body": fmt.Sprintf("改写帖子%d", idx),
					"body_en": fmt.Sprintf("Rewritten post %d", idx),
				})
			}
		}
		b, _ := json.Marshal(out)
		fmt.Fprint(w, chatResponse(string(b)))
	}))
	defer srv.Close()

	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 2, Timeout: 5 * time.Second})
	sc := testScenario()
	sc.EraHint = "类似某个狂热"
	posts := []engine.ForumPost{
		{Day: 1, NPCName: "满仓踏空王", Persona: "多头", Body: "看到阿尔法的消息我直接加仓"},
		{Day: 1, NPCName: "空仓观望中", Persona: "阴阳师", Body: "今天又是看戏的一天"},
	}
	g.FillForumCopy(context.Background(), sc, posts)
	if posts[0].Body != "改写帖子0" || posts[1].Body != "改写帖子1" {
		t.Fatalf("forum posts not polished: %+v", posts)
	}
	if posts[0].BodyEn != "Rewritten post 0" || posts[1].BodyEn != "Rewritten post 1" {
		t.Fatalf("forum English bodies not polished: %+v", posts)
	}
	sys := gotSystem.Load().(string)
	if !strings.Contains(sys, "股民论坛") || !strings.Contains(sys, "类似某个狂热") {
		t.Fatalf("forum system prompt wrong: %s", sys)
	}
	if !strings.Contains(sys, "不得虚构自己买卖、持仓、收益或掌握内幕") {
		t.Fatalf("forum system prompt allows fake trade narration: %s", sys)
	}
}

func TestFillForumCopyNeverLeaksNumbersOrTruth(t *testing.T) {
	var captured atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		b, _ := json.Marshal(req)
		captured.Store(string(b))
		fmt.Fprint(w, chatResponse("[]"))
	}))
	defer srv.Close()
	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 1, Timeout: 5 * time.Second})
	posts := []engine.ForumPost{
		{Day: 1, NPCName: "满仓踏空王", Persona: "多头", Body: "听说阿尔法那边有情况，懂的都懂"},
	}
	g.FillForumCopy(context.Background(), testScenario(), posts)
	req := captured.Load().(string)
	for _, leak := range []string{"0.03", "TrueShock", "true_shock", "ReportShock", "report_shock"} {
		if strings.Contains(req, leak) {
			t.Fatalf("forum prompt leaks %q", leak)
		}
	}
	// The draft facts and persona reach the model; the no-numbers rule is stated.
	if !strings.Contains(req, "阿尔法") || !strings.Contains(req, "多头") {
		t.Fatal("forum prompt missing draft or persona")
	}
	if !strings.Contains(req, "禁止真实公司名、真实人名与任何数字") {
		t.Fatal("forum system prompt missing the no-numbers rule")
	}
}

func TestFillForumCopyPartialBilingualDegradation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, chatResponse(`[
			{"idx":0,"body":"中文回帖"},
			{"idx":1,"body_en":"English reply"},
			{"idx":2}
		]`))
	}))
	defer srv.Close()
	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 1, Timeout: 5 * time.Second})
	posts := []engine.ForumPost{
		{Day: 1, NPCName: "n", Persona: "p", Body: "模板帖子0"},
		{Day: 1, NPCName: "n", Persona: "p", Body: "模板帖子1"},
		{Day: 1, NPCName: "n", Persona: "p", Body: "模板帖子2"},
	}
	g.FillForumCopy(context.Background(), testScenario(), posts)

	// zh only: zh written back, en left empty.
	if posts[0].Body != "中文回帖" || posts[0].BodyEn != "" {
		t.Fatalf("post 0: %+v", posts[0])
	}
	// en only: en written back, zh template retained.
	if posts[1].BodyEn != "English reply" || posts[1].Body != "模板帖子1" {
		t.Fatalf("post 1: %+v", posts[1])
	}
	// neither: template retained on both sides.
	if posts[2].Body != "模板帖子2" || posts[2].BodyEn != "" {
		t.Fatalf("post 2 must be fully untouched: %+v", posts[2])
	}
}

func TestFillForumCopyDegradesToTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, chatResponse("这不是JSON"))
	}))
	defer srv.Close()
	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 1, Timeout: 5 * time.Second})
	posts := []engine.ForumPost{{Day: 1, NPCName: "n", Persona: "p", Body: "模板帖子"}}
	g.FillForumCopy(context.Background(), testScenario(), posts)
	if posts[0].Body != "模板帖子" {
		t.Fatalf("bad response must leave template intact: %+v", posts[0])
	}
}

func TestRecapPromptBranchIsObjectiveAndLeakFree(t *testing.T) {
	var captured atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		b, _ := json.Marshal(req)
		captured.Store(string(b))
		fmt.Fprint(w, chatResponse("[]"))
	}))
	defer srv.Close()
	g := New(Config{BaseURL: srv.URL, Model: "m", Concurrency: 1, Timeout: 5 * time.Second})
	evs := []engine.NewsEvent{{
		Day: 2, Track: engine.TrackHistorical, MediaID: "wire", Recap: true,
		Headline: "昨日复盘：阿尔法领涨，贝塔垫底，明星板块最强",
	}}
	g.FillCopy(context.Background(), testScenario(), evs)
	req := captured.Load().(string)
	if !strings.Contains(req, "每日复盘") || !strings.Contains(req, "市场复盘专栏") {
		t.Fatal("recap branch missing objective persona/kind")
	}
	if !strings.Contains(req, "阿尔法") || !strings.Contains(req, "明星板块") {
		t.Fatal("recap prompt missing the template facts")
	}
	for _, leak := range []string{"0.03", "TrueShock", "true_shock", "ReportShock", "report_shock"} {
		if strings.Contains(req, leak) {
			t.Fatalf("recap prompt leaks %q", leak)
		}
	}
}
