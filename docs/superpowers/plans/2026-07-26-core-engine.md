# Stocker 核心引擎（世界生成器）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现从 (剧本, 种子) 确定性生成一局完整"平行世界"（扰动价格序列 + 三轨新闻时间线）的纯 Go 引擎，含大势保真校验，零外部依赖。

**Architecture:** 纯函数引擎包（无 DB、无网络、无全局状态）。所有随机性从房间种子派生的命名子流获得；时间是入参而非 `time.Now()`。剧本(scenario)包定义数据类型并提供合成测试剧本；engine 包实现时钟映射、冲击时间线、双时间尺度因子演化、价格合成、三轨新闻组装与保真校验；`cmd/worldgen` 提供 CLI 冒烟入口。

**Tech Stack:** Go 1.22+（仅标准库；随机数用 `math/rand/v2` 的 PCG）。

**计划系列:** 本计划是 4 个子计划中的第 1 个（后续：持久化与API → React前端 → 数据管线与LLM文案），spec 见 `docs/superpowers/specs/2026-07-25-stocker-design.md`。

## Global Constraints

- Go ≥ 1.22，本计划只用标准库；模块名 `github.com/toddzheng/stocker/server`。
- 引擎代码禁止调用 `time.Now()`、全局 `rand`；一切随机性必须经 `engine.Stream(seed, labels...)` 派生，同种子必须逐字节可复现。
- 扰动参数（spec §4.2 定版）：λ_fast=0.7、λ_slow=0.99、快慢分配 0.65/0.35、冲击幅度 Gamma(shape=2, mean=0.022)、正向概率 0.45、日噪声 σ=0.004、clamp=±0.30。
- 新闻发布于第 d 日收盘后，影响从第 d+1 日生效。
- 价格基线归一化为起始 100；显示价 = 基线 × exp(clamp(Σβ·X + ε))，OHLC 四个值同乘同一系数。
- 保真校验（spec §4.6）：日对数收益相关 ≥ 0.85；整局累计涨跌方向与基线一致；基线全局峰/谷日在扰动后 ±10 个交易日内仍是全局峰/谷。
- 代码标识符用英文；面向玩家的降级文案用中文。

## File Structure

```
server/
  go.mod
  internal/scenario/
    types.go            # Scenario/Factor/Instrument/OHLC/KeyWindow 类型
    synthetic.go        # 合成测试剧本（确定性生成，含泡沫-崩盘剧情）
    synthetic_test.go
  internal/engine/
    clock.go            # 确定性时间轴映射
    clock_test.go
    rng.go              # 种子派生的命名随机子流
    rng_test.go
    shocks.go           # 冲击新闻时间线生成（含关键期抑制）
    shocks_test.go
    factors.go          # 双时间尺度因子状态演化
    factors_test.go
    prices.go           # 价格合成
    prices_test.go
    news.go             # 历史解释性新闻 + 噪音新闻 + 降级文案
    news_test.go
    fidelity.go         # 大势保真校验
    fidelity_test.go
    world.go            # GenerateWorld 编排 + golden test
    world_test.go
  cmd/worldgen/main.go  # CLI：dump 一个世界为 JSON
```

---

### Task 1: 模块脚手架 + 确定性时钟

**Files:**
- Create: `server/go.mod`
- Create: `server/internal/engine/clock.go`
- Test: `server/internal/engine/clock_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `engine.CurrentDay(startedAt time.Time, dayDuration time.Duration, totalDays int, now time.Time) (day int, ended bool)`

- [ ] **Step 1: 初始化模块**

```bash
cd server && go mod init github.com/toddzheng/stocker/server
```

- [ ] **Step 2: 写失败的测试**

```go
// server/internal/engine/clock_test.go
package engine

import (
	"testing"
	"time"
)

func TestCurrentDay(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	day27 := 27 * time.Minute
	cases := []struct {
		name  string
		now   time.Time
		day   int
		ended bool
	}{
		{"at start", start, 0, false},
		{"just before day 1", start.Add(day27 - time.Second), 0, false},
		{"exactly day 1", start.Add(day27), 1, false},
		{"mid game", start.Add(100*day27 + time.Minute), 100, false},
		{"last day", start.Add(749 * day27), 749, false},
		{"past end clamps", start.Add(3000 * day27), 749, true},
	}
	for _, c := range cases {
		day, ended := CurrentDay(start, day27, 750, c.now)
		if day != c.day || ended != c.ended {
			t.Errorf("%s: got (%d,%v) want (%d,%v)", c.name, day, ended, c.day, c.ended)
		}
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `cd server && go test ./internal/engine/ -run TestCurrentDay -v`
Expected: FAIL（`CurrentDay` 未定义）

- [ ] **Step 4: 最小实现**

```go
// server/internal/engine/clock.go
package engine

import "time"

// CurrentDay maps wall-clock time to the historical trading-day index of a
// room. Callers guarantee now >= startedAt. When the scenario is exhausted
// the last day is returned with ended=true.
func CurrentDay(startedAt time.Time, dayDuration time.Duration, totalDays int, now time.Time) (int, bool) {
	day := int(now.Sub(startedAt) / dayDuration)
	if day >= totalDays {
		return totalDays - 1, true
	}
	return day, false
}
```

- [ ] **Step 5: 运行确认通过**

Run: `cd server && go test ./internal/engine/ -run TestCurrentDay -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add server/go.mod server/internal/engine/
git commit -m "feat(engine): module scaffold and deterministic clock"
```

---

### Task 2: 种子派生的命名随机子流

**Files:**
- Create: `server/internal/engine/rng.go`
- Test: `server/internal/engine/rng_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `engine.Stream(seed uint64, labels ...string) *rand.Rand`（`math/rand/v2`）——同 (seed, labels) 序列逐位相同；labels 不同则独立。

- [ ] **Step 1: 写失败的测试**

```go
// server/internal/engine/rng_test.go
package engine

import "testing"

func TestStreamDeterministic(t *testing.T) {
	a, b := Stream(42, "shocks"), Stream(42, "shocks")
	for i := 0; i < 100; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatalf("same seed+label diverged at %d", i)
		}
	}
}

func TestStreamIndependentLabels(t *testing.T) {
	a, b := Stream(42, "shocks"), Stream(42, "eps", "CSCO")
	same := 0
	for i := 0; i < 100; i++ {
		if a.Uint64() == b.Uint64() {
			same++
		}
	}
	if same > 2 {
		t.Fatalf("labels not independent: %d/100 collisions", same)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd server && go test ./internal/engine/ -run TestStream -v`
Expected: FAIL（`Stream` 未定义）

- [ ] **Step 3: 最小实现**

```go
// server/internal/engine/rng.go
package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
)

// Stream derives an independent, reproducible random stream from the room
// seed and a label path. All engine randomness must come from here.
func Stream(seed uint64, labels ...string) *rand.Rand {
	h := sha256.New()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], seed)
	h.Write(buf[:])
	for _, l := range labels {
		h.Write([]byte{0})
		h.Write([]byte(l))
	}
	sum := h.Sum(nil)
	return rand.New(rand.NewPCG(
		binary.LittleEndian.Uint64(sum[0:8]),
		binary.LittleEndian.Uint64(sum[8:16]),
	))
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd server && go test ./internal/engine/ -run TestStream -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/engine/rng.go server/internal/engine/rng_test.go
git commit -m "feat(engine): seed-derived named random streams"
```

---

### Task 3: 剧本类型 + 合成测试剧本

**Files:**
- Create: `server/internal/scenario/types.go`
- Create: `server/internal/scenario/synthetic.go`
- Test: `server/internal/scenario/synthetic_test.go`

**Interfaces:**
- Consumes: 无
- Produces:

```go
package scenario

type FactorKind string // "market" | "sentiment" | "sector" | "macro" | "idio"
type Factor struct{ ID, Name string; Kind FactorKind }
type OHLC struct{ Open, High, Low, Close float64 }
type KeyWindow struct{ StartDay, EndDay, Direction int } // Direction: +1 涨 / -1 跌
type Instrument struct {
	ID, Alias, Desc string
	Beta            map[string]float64 // factorID -> exposure（含自身特质因子，权重 1.0）
}
type Scenario struct {
	ID          string
	Days        int
	Factors     []Factor
	Instruments []Instrument
	KeyWindows  []KeyWindow
	Baseline    map[string][]OHLC // instrumentID -> Days 长度，起始 Open=100
}
func Synthetic() *Scenario // 8 只标的、300 天、内置"泡沫-崩盘"剧情、崩盘 KeyWindow
```

- [ ] **Step 1: 写失败的测试**

```go
// server/internal/scenario/synthetic_test.go
package scenario

import "testing"

func TestSyntheticShape(t *testing.T) {
	sc := Synthetic()
	if sc.Days != 300 || len(sc.Instruments) != 8 {
		t.Fatalf("shape: days=%d instruments=%d", sc.Days, len(sc.Instruments))
	}
	nIdio := 0
	for _, f := range sc.Factors {
		if f.Kind == "idio" {
			nIdio++
		}
	}
	if nIdio != len(sc.Instruments) {
		t.Fatalf("want one idio factor per instrument, got %d", nIdio)
	}
	for _, inst := range sc.Instruments {
		prices := sc.Baseline[inst.ID]
		if len(prices) != sc.Days {
			t.Fatalf("%s: %d days", inst.ID, len(prices))
		}
		if prices[0].Open != 100 {
			t.Fatalf("%s: start open %f, want 100", inst.ID, prices[0].Open)
		}
		for d, p := range prices {
			if p.Low > p.Open || p.Low > p.Close || p.High < p.Open || p.High < p.Close || p.Low <= 0 {
				t.Fatalf("%s day %d: invalid OHLC %+v", inst.ID, d, p)
			}
		}
	}
	if len(sc.KeyWindows) == 0 {
		t.Fatal("synthetic scenario must include a crash KeyWindow")
	}
}

func TestSyntheticDeterministic(t *testing.T) {
	a, b := Synthetic(), Synthetic()
	if a.Baseline["S1"][123] != b.Baseline["S1"][123] {
		t.Fatal("synthetic scenario must be deterministic")
	}
}

func TestSyntheticHasBoomAndCrash(t *testing.T) {
	sc := Synthetic()
	p := sc.Baseline["S1"]
	peak := p[150].Close
	if peak < 150 { // 前 150 天泡沫至少 +50%
		t.Fatalf("boom too weak: peak %f", peak)
	}
	if p[220].Close > peak*0.55 { // 崩盘至少 -45%
		t.Fatalf("crash too weak: %f vs peak %f", p[220].Close, peak)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd server && go test ./internal/scenario/ -v`
Expected: FAIL（包不存在）

- [ ] **Step 3: 实现类型与合成剧本**

```go
// server/internal/scenario/types.go
package scenario

type FactorKind string

const (
	KindMarket    FactorKind = "market"
	KindSentiment FactorKind = "sentiment"
	KindSector    FactorKind = "sector"
	KindMacro     FactorKind = "macro"
	KindIdio      FactorKind = "idio"
)

type Factor struct {
	ID   string
	Name string
	Kind FactorKind
}

type OHLC struct{ Open, High, Low, Close float64 }

type KeyWindow struct{ StartDay, EndDay, Direction int }

type Instrument struct {
	ID, Alias, Desc string
	Beta            map[string]float64
}

type Scenario struct {
	ID          string
	Days        int
	Factors     []Factor
	Instruments []Instrument
	KeyWindows  []KeyWindow
	Baseline    map[string][]OHLC
}

// FactorIDs returns factor ids in declaration order (canonical state ordering).
func (s *Scenario) FactorIDs() []string {
	ids := make([]string, len(s.Factors))
	for i, f := range s.Factors {
		ids[i] = f.ID
	}
	return ids
}
```

```go
// server/internal/scenario/synthetic.go
package scenario

import (
	"fmt"
	"math"
	"math/rand/v2"
)

// Synthetic builds a deterministic 8-instrument, 300-day test scenario with a
// scripted bubble (day 0-150), crash (150-220) and flat recovery, so engine
// tests can assert fidelity against known structure. Fixed internal seed —
// NOT derived from any room seed; this is test data, not gameplay data.
func Synthetic() *Scenario {
	const days = 300
	factors := []Factor{
		{ID: "MKT", Name: "market", Kind: KindMarket},
		{ID: "TECH", Name: "tech sector", Kind: KindSector},
		{ID: "OLD", Name: "old economy", Kind: KindSector},
	}
	sc := &Scenario{
		ID:         "synthetic-v1",
		Days:       days,
		KeyWindows: []KeyWindow{{StartDay: 150, EndDay: 220, Direction: -1}},
		Baseline:   map[string][]OHLC{},
	}
	rng := rand.New(rand.NewPCG(7, 7)) // fixed: determinism required by tests
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("S%d", i+1)
		idioID := "IDIO:" + id
		factors = append(factors, Factor{ID: idioID, Name: id, Kind: KindIdio})
		techy := i < 5 // S1-S5 科技股吃泡沫剧情, S6-S8 传统行业走平
		beta := map[string]float64{"MKT": 1.0, idioID: 1.0}
		if techy {
			beta["TECH"] = 1.2
		} else {
			beta["OLD"] = 0.8
		}
		sc.Instruments = append(sc.Instruments, Instrument{
			ID: id, Alias: "Syn " + id, Desc: "synthetic", Beta: beta,
		})
		prices := make([]OHLC, days)
		logp := math.Log(100)
		for d := 0; d < days; d++ {
			drift := 0.0005
			if techy {
				switch {
				case d < 150:
					drift = 0.006 // 泡沫
				case d < 220:
					drift = -0.013 // 崩盘
				default:
					drift = 0.0
				}
			}
			ret := drift + rng.NormFloat64()*0.015
			open := math.Exp(logp)
			logp += ret
			cls := math.Exp(logp)
			hi := math.Max(open, cls) * (1 + rng.Float64()*0.01)
			lo := math.Min(open, cls) * (1 - rng.Float64()*0.01)
			prices[d] = OHLC{Open: open, High: hi, Low: lo, Close: cls}
		}
		// 归一化：起始 Open 精确为 100
		k := 100 / prices[0].Open
		for d := range prices {
			prices[d].Open *= k
			prices[d].High *= k
			prices[d].Low *= k
			prices[d].Close *= k
		}
		sc.Baseline[id] = prices
	}
	sc.Factors = factors
	return sc
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd server && go test ./internal/scenario/ -v`
Expected: PASS（若 `TestSyntheticHasBoomAndCrash` 因固定种子的噪声偶然不达标，调大 boom/crash drift 使剧情结构稳定，不要改测试阈值）

- [ ] **Step 5: Commit**

```bash
git add server/internal/scenario/
git commit -m "feat(scenario): types and deterministic synthetic test scenario"
```

---

### Task 4: 冲击新闻时间线（含关键期抑制）

**Files:**
- Create: `server/internal/engine/shocks.go`
- Test: `server/internal/engine/shocks_test.go`

**Interfaces:**
- Consumes: `scenario.Scenario`、`engine.Stream`
- Produces:

```go
type Track string // TrackHistorical | TrackImpact | TrackNoise
type Media struct{ ID string; Rho float64 } // 媒体信誉度
var MediaTable []Media                      // 内置 4-5 家虚拟媒体
type NewsEvent struct {
	Day         int                // 发布日（收盘后），影响从 Day+1 生效
	Track       Track
	MediaID     string
	TrueShock   map[string]float64 // factorID -> log 冲击；historical/noise 轨为空
	ReportShock map[string]float64 // ρ·true + 噪声，喂给文案
	Headline    string             // 降级模板文案（Task 6 填充）
}
func GenerateShockTimeline(sc *scenario.Scenario, seed uint64) []NewsEvent
```

生成规则（数值即定版起点）：每日市场/宏观事件概率 0.15；每日行业事件概率 0.25（均匀抽一个 sector 因子）；每标的每日特质事件概率 0.05。幅度 = Gamma(2, mean 0.022)（用两指数和采样），正向概率 0.45。关键期抑制：事件日落在 KeyWindow 内且冲击方向与 `Direction` 相反且幅度 > 0.02 时，以 0.8 概率翻转为顺方向。`ReportShock = ρ·TrueShock + N(0, (1-ρ)·0.01)` 逐分量。

- [ ] **Step 1: 写失败的测试**

```go
// server/internal/engine/shocks_test.go
package engine

import (
	"math"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestShockTimelineDeterministic(t *testing.T) {
	sc := scenario.Synthetic()
	a := GenerateShockTimeline(sc, 42)
	b := GenerateShockTimeline(sc, 42)
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Day != b[i].Day || a[i].MediaID != b[i].MediaID {
			t.Fatalf("event %d differs", i)
		}
		for f, v := range a[i].TrueShock {
			if b[i].TrueShock[f] != v {
				t.Fatalf("event %d shock differs on %s", i, f)
			}
		}
	}
	c := GenerateShockTimeline(sc, 43)
	if len(c) == len(a) {
		same := true
		for i := range a {
			if a[i].Day != c[i].Day {
				same = false
				break
			}
		}
		if same {
			t.Fatal("different seeds produced identical timelines")
		}
	}
}

func TestShockTimelineRates(t *testing.T) {
	sc := scenario.Synthetic()
	n := 0
	for s := uint64(0); s < 20; s++ {
		n += len(GenerateShockTimeline(sc, s))
	}
	perDay := float64(n) / (20 * float64(sc.Days))
	// 期望 ≈ 0.15 + 0.25 + 8*0.05 = 0.80 事件/日
	if perDay < 0.5 || perDay > 1.2 {
		t.Fatalf("event rate %f/day out of range", perDay)
	}
}

func TestKeyWindowSuppression(t *testing.T) {
	sc := scenario.Synthetic()
	kw := sc.KeyWindows[0] // 崩盘窗口, Direction=-1
	inWinPos, inWinNeg := 0, 0
	for s := uint64(0); s < 50; s++ {
		for _, ev := range GenerateShockTimeline(sc, s) {
			if ev.Day < kw.StartDay || ev.Day > kw.EndDay {
				continue
			}
			for _, v := range ev.TrueShock {
				if math.Abs(v) <= 0.02 {
					continue
				}
				if v > 0 {
					inWinPos++
				} else {
					inWinNeg++
				}
			}
		}
	}
	if inWinPos*3 >= inWinNeg {
		t.Fatalf("suppression too weak in crash window: %d big-positive vs %d big-negative", inWinPos, inWinNeg)
	}
}

func TestReportShockDecoupled(t *testing.T) {
	sc := scenario.Synthetic()
	evs := GenerateShockTimeline(sc, 42)
	diff := false
	for _, ev := range evs {
		for f, v := range ev.TrueShock {
			if ev.ReportShock[f] != v {
				diff = true
			}
		}
	}
	if !diff {
		t.Fatal("ReportShock identical to TrueShock everywhere; rho/noise not applied")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd server && go test ./internal/engine/ -run 'TestShock|TestKeyWindow|TestReport' -v`
Expected: FAIL

- [ ] **Step 3: 实现**

```go
// server/internal/engine/shocks.go
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
		evs = append(evs, NewsEvent{
			Day: day, Track: TrackImpact, MediaID: m.ID,
			TrueShock: shock, ReportShock: report(rng, m.Rho, shock),
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
			if rng.Float64() < pIdioDaily {
				emit(d, map[string]float64{idioOf[inst.ID]: signedShock(rng, sc, d)})
			}
		}
	}
	return evs
}
```

注意：特质因子识别依赖 `IDIO:` 前缀约定（`scenario.Synthetic` 已遵守）；真实剧本数据管线（计划 4）同样遵守此约定。

- [ ] **Step 4: 运行确认通过**

Run: `cd server && go test ./internal/engine/ -run 'TestShock|TestKeyWindow|TestReport' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/engine/shocks.go server/internal/engine/shocks_test.go
git commit -m "feat(engine): impact news timeline with key-window suppression"
```

---

### Task 5: 因子状态演化 + 价格合成

**Files:**
- Create: `server/internal/engine/factors.go`
- Create: `server/internal/engine/prices.go`
- Test: `server/internal/engine/factors_test.go`
- Test: `server/internal/engine/prices_test.go`

**Interfaces:**
- Consumes: `NewsEvent`（Task 4）、`scenario.Scenario`、`Stream`
- Produces:

```go
// states[d][i] = 第 d 日第 i 个因子（按 sc.FactorIDs() 顺序）的 X = X_fast+X_slow
func EvolveFactorStates(sc *scenario.Scenario, events []NewsEvent) [][]float64
// 显示价：baseline × exp(clamp(Σβ·X + ε))；ε 从 Stream(seed,"eps",instrumentID) 取
func SynthesizePrices(sc *scenario.Scenario, states [][]float64, seed uint64) map[string][]scenario.OHLC
```

- [ ] **Step 1: 写失败的测试**

```go
// server/internal/engine/factors_test.go
package engine

import (
	"math"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestSingleShockDecay(t *testing.T) {
	sc := scenario.Synthetic()
	shock := 0.10
	evs := []NewsEvent{{Day: 9, Track: TrackImpact,
		TrueShock: map[string]float64{"MKT": shock}}}
	states := EvolveFactorStates(sc, evs)
	mkt := 0 // "MKT" 是 FactorIDs()[0]
	if states[9][mkt] != 0 {
		t.Fatal("shock must take effect on Day+1, not publish day")
	}
	x10 := states[10][mkt]
	want10 := shock // 生效首日：fast 0.65+slow 0.35 = 全额
	if math.Abs(x10-want10) > 1e-9 {
		t.Fatalf("day10 X=%f want %f", x10, want10)
	}
	x11 := states[11][mkt]
	want11 := shock*0.65*lamFast + shock*0.35*lamSlow
	if math.Abs(x11-want11) > 1e-9 {
		t.Fatalf("day11 X=%f want %f", x11, want11)
	}
	// 长期回归 0
	if math.Abs(states[299][mkt]) > 0.01 {
		t.Fatalf("X should decay toward 0, got %f", states[299][mkt])
	}
}

func TestNoEventsZeroStates(t *testing.T) {
	sc := scenario.Synthetic()
	states := EvolveFactorStates(sc, nil)
	for d := range states {
		for i, x := range states[d] {
			if x != 0 {
				t.Fatalf("day %d factor %d nonzero without events", d, i)
			}
		}
	}
}
```

```go
// server/internal/engine/prices_test.go
package engine

import (
	"math"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestPricesNoShocksTrackBaseline(t *testing.T) {
	sc := scenario.Synthetic()
	states := EvolveFactorStates(sc, nil)
	prices := SynthesizePrices(sc, states, 42)
	for _, inst := range sc.Instruments {
		for d := 0; d < sc.Days; d++ {
			ratio := prices[inst.ID][d].Close / sc.Baseline[inst.ID][d].Close
			// 仅剩 ε 噪声, |logratio| 应远小于 5σ=2%
			if math.Abs(math.Log(ratio)) > 0.02 {
				t.Fatalf("%s day %d deviates %f without shocks", inst.ID, d, ratio)
			}
		}
	}
}

func TestPricesClampBound(t *testing.T) {
	sc := scenario.Synthetic()
	// 构造巨量冲击, 验证 clamp
	evs := []NewsEvent{}
	for d := 0; d < 50; d++ {
		evs = append(evs, NewsEvent{Day: d, Track: TrackImpact,
			TrueShock: map[string]float64{"MKT": 0.5}})
	}
	states := EvolveFactorStates(sc, evs)
	prices := SynthesizePrices(sc, states, 42)
	for _, inst := range sc.Instruments {
		for d := 0; d < sc.Days; d++ {
			ratio := prices[inst.ID][d].Close / sc.Baseline[inst.ID][d].Close
			if ratio > math.Exp(clampX)+1e-9 {
				t.Fatalf("clamp violated: ratio %f", ratio)
			}
		}
	}
}

func TestPricesOHLCConsistent(t *testing.T) {
	sc := scenario.Synthetic()
	states := EvolveFactorStates(sc, GenerateShockTimeline(sc, 42))
	prices := SynthesizePrices(sc, states, 42)
	for _, inst := range sc.Instruments {
		for d, p := range prices[inst.ID] {
			b := sc.Baseline[inst.ID][d]
			// 四值同乘一个系数 → 比例关系保持
			r1, r2 := p.Open/b.Open, p.Close/b.Close
			if math.Abs(r1-r2) > 1e-9 {
				t.Fatalf("%s day %d: OHLC not uniformly scaled", inst.ID, d)
			}
			if p.Low > p.Open || p.High < p.Close || p.Low <= 0 {
				t.Fatalf("%s day %d: invalid OHLC", inst.ID, d)
			}
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd server && go test ./internal/engine/ -run 'TestSingleShock|TestNoEvents|TestPrices' -v`
Expected: FAIL

- [ ] **Step 3: 实现**

```go
// server/internal/engine/factors.go
package engine

import "github.com/toddzheng/stocker/server/internal/scenario"

// EvolveFactorStates advances the dual-timescale factor state day by day.
// An event published on day d first affects states on day d+1.
func EvolveFactorStates(sc *scenario.Scenario, events []NewsEvent) [][]float64 {
	ids := sc.FactorIDs()
	idx := make(map[string]int, len(ids))
	for i, id := range ids {
		idx[id] = i
	}
	byDay := map[int][]NewsEvent{}
	for _, ev := range events {
		if ev.Track == TrackImpact {
			byDay[ev.Day] = append(byDay[ev.Day], ev)
		}
	}
	fast := make([]float64, len(ids))
	slow := make([]float64, len(ids))
	states := make([][]float64, sc.Days)
	for d := 0; d < sc.Days; d++ {
		for i := range fast {
			fast[i] *= lamFast
			slow[i] *= lamSlow
		}
		for _, ev := range byDay[d-1] { // published after close of d-1
			for f, v := range ev.TrueShock {
				fast[idx[f]] += v * fracFast
				slow[idx[f]] += v * (1 - fracFast)
			}
		}
		row := make([]float64, len(ids))
		for i := range row {
			row[i] = fast[i] + slow[i]
		}
		states[d] = row
	}
	return states
}
```

```go
// server/internal/engine/prices.go
package engine

import (
	"math"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// SynthesizePrices produces the per-room display prices:
// display = baseline × exp(clamp(Σ β·X + ε)).
func SynthesizePrices(sc *scenario.Scenario, states [][]float64, seed uint64) map[string][]scenario.OHLC {
	ids := sc.FactorIDs()
	idx := make(map[string]int, len(ids))
	for i, id := range ids {
		idx[id] = i
	}
	out := make(map[string][]scenario.OHLC, len(sc.Instruments))
	for _, inst := range sc.Instruments {
		rng := Stream(seed, "eps", inst.ID)
		prices := make([]scenario.OHLC, sc.Days)
		for d := 0; d < sc.Days; d++ {
			x := rng.NormFloat64() * epsSigma
			for f, beta := range inst.Beta {
				x += beta * states[d][idx[f]]
			}
			x = math.Max(-clampX, math.Min(clampX, x))
			m := math.Exp(x)
			b := sc.Baseline[inst.ID][d]
			prices[d] = scenario.OHLC{
				Open: b.Open * m, High: b.High * m, Low: b.Low * m, Close: b.Close * m,
			}
		}
		out[inst.ID] = prices
	}
	return out
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd server && go test ./internal/engine/ -run 'TestSingleShock|TestNoEvents|TestPrices' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/engine/factors.go server/internal/engine/prices.go \
  server/internal/engine/factors_test.go server/internal/engine/prices_test.go
git commit -m "feat(engine): dual-timescale factor evolution and price synthesis"
```

---

### Task 6: 历史解释性新闻 + 噪音新闻 + 降级文案

**Files:**
- Create: `server/internal/engine/news.go`
- Test: `server/internal/engine/news_test.go`

**Interfaces:**
- Consumes: `scenario.Scenario`、`NewsEvent`、`Stream`
- Produces:

```go
// 扫描基线: 单日 |logret| > 0.06 的日子 → historical 轨事件（TrueShock 为空）
func HistoricalNews(sc *scenario.Scenario) []NewsEvent
// 每日概率 0.10 的零影响花边新闻
func NoiseNews(sc *scenario.Scenario, seed uint64) []NewsEvent
// 为无文案事件填充中文模板 Headline（LLM 文案在计划 4 替换）
func FillFallbackCopy(sc *scenario.Scenario, evs []NewsEvent, seed uint64)
```

- [ ] **Step 1: 写失败的测试**

```go
// server/internal/engine/news_test.go
package engine

import (
	"math"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestHistoricalNewsCoversBigMoves(t *testing.T) {
	sc := scenario.Synthetic()
	evs := HistoricalNews(sc)
	byDay := map[int]bool{}
	for _, ev := range evs {
		if ev.Track != TrackHistorical {
			t.Fatal("wrong track")
		}
		if len(ev.TrueShock) != 0 {
			t.Fatal("historical news must not inject shocks")
		}
		byDay[ev.Day] = true
	}
	// 找一个基线大波动日, 断言有报道
	found := false
	for _, inst := range sc.Instruments {
		p := sc.Baseline[inst.ID]
		for d := 1; d < sc.Days; d++ {
			if math.Abs(math.Log(p[d].Close/p[d-1].Close)) > 0.06 {
				found = true
				if !byDay[d] {
					t.Fatalf("big move day %d has no historical news", d)
				}
			}
		}
	}
	if !found {
		t.Skip("synthetic scenario produced no >6% day; loosen synthetic noise")
	}
}

func TestNoiseNewsZeroImpactAndDeterministic(t *testing.T) {
	sc := scenario.Synthetic()
	a := NoiseNews(sc, 42)
	b := NoiseNews(sc, 42)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("noise news not deterministic: %d vs %d", len(a), len(b))
	}
	for _, ev := range a {
		if ev.Track != TrackNoise || len(ev.TrueShock) != 0 {
			t.Fatal("noise news must have zero impact")
		}
	}
}

func TestFillFallbackCopy(t *testing.T) {
	sc := scenario.Synthetic()
	evs := GenerateShockTimeline(sc, 42)
	evs = append(evs, NoiseNews(sc, 42)...)
	FillFallbackCopy(sc, evs, 42)
	for i, ev := range evs {
		if ev.Headline == "" {
			t.Fatalf("event %d has empty headline", i)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd server && go test ./internal/engine/ -run 'TestHistorical|TestNoise|TestFillFallback' -v`
Expected: FAIL

- [ ] **Step 3: 实现**

```go
// server/internal/engine/news.go
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
			evs[i].Headline = fmt.Sprintf("消息面变化，%s板块%s，市场解读不一", name[top], tone)
		default:
			evs[i].Headline = noiseTemplates[rng.IntN(len(noiseTemplates))]
		}
	}
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd server && go test ./internal/engine/ -run 'TestHistorical|TestNoise|TestFillFallback' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/engine/news.go server/internal/engine/news_test.go
git commit -m "feat(engine): historical and noise news tracks with fallback copy"
```

---

### Task 7: 大势保真校验

**Files:**
- Create: `server/internal/engine/fidelity.go`
- Test: `server/internal/engine/fidelity_test.go`

**Interfaces:**
- Consumes: `scenario.Scenario`、显示价格（Task 5 输出格式）
- Produces: `VerifyFidelity(sc *scenario.Scenario, prices map[string][]scenario.OHLC) error` —— spec §4.6 三条款，任一不满足返回描述性 error。计划 4 的数据管线复用此函数做剧本准入校验。

- [ ] **Step 1: 写失败的测试**

```go
// server/internal/engine/fidelity_test.go
package engine

import (
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestFidelityHoldsAcrossSeeds(t *testing.T) {
	sc := scenario.Synthetic()
	for s := uint64(0); s < 30; s++ {
		evs := GenerateShockTimeline(sc, s)
		states := EvolveFactorStates(sc, evs)
		prices := SynthesizePrices(sc, states, s)
		if err := VerifyFidelity(sc, prices); err != nil {
			t.Fatalf("seed %d violates fidelity: %v", s, err)
		}
	}
}

func TestFidelityRejectsGarbage(t *testing.T) {
	sc := scenario.Synthetic()
	// 基线取反(镜像)必然违反相关性/方向条款
	bad := map[string][]scenario.OHLC{}
	for id, p := range sc.Baseline {
		rev := make([]scenario.OHLC, len(p))
		for i := range p {
			rev[i] = p[len(p)-1-i]
		}
		bad[id] = rev
	}
	if err := VerifyFidelity(sc, bad); err == nil {
		t.Fatal("reversed prices should fail fidelity")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd server && go test ./internal/engine/ -run TestFidelity -v`
Expected: FAIL

- [ ] **Step 3: 实现**

```go
// server/internal/engine/fidelity.go
package engine

import (
	"fmt"
	"math"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

const (
	minReturnCorr  = 0.85
	extremumSlack  = 10
)

func logReturns(p []scenario.OHLC) []float64 {
	r := make([]float64, len(p)-1)
	for i := 1; i < len(p); i++ {
		r[i-1] = math.Log(p[i].Close / p[i-1].Close)
	}
	return r
}

func pearson(a, b []float64) float64 {
	n := float64(len(a))
	var sa, sb, saa, sbb, sab float64
	for i := range a {
		sa += a[i]
		sb += b[i]
		saa += a[i] * a[i]
		sbb += b[i] * b[i]
		sab += a[i] * b[i]
	}
	cov := sab - sa*sb/n
	va := saa - sa*sa/n
	vb := sbb - sb*sb/n
	return cov / math.Sqrt(va*vb)
}

func argExtremum(p []scenario.OHLC, max bool) int {
	best := 0
	for i := range p {
		if (max && p[i].Close > p[best].Close) || (!max && p[i].Close < p[best].Close) {
			best = i
		}
	}
	return best
}

// VerifyFidelity enforces spec §4.6: perturbed prices must preserve the
// broad historical narrative. Also used by the data pipeline (plan 4) as a
// scenario acceptance gate.
func VerifyFidelity(sc *scenario.Scenario, prices map[string][]scenario.OHLC) error {
	for _, inst := range sc.Instruments {
		base, disp := sc.Baseline[inst.ID], prices[inst.ID]
		if corr := pearson(logReturns(base), logReturns(disp)); corr < minReturnCorr {
			return fmt.Errorf("%s: return correlation %.3f < %.2f", inst.ID, corr, minReturnCorr)
		}
		bDir := base[len(base)-1].Close >= base[0].Close
		dDir := disp[len(disp)-1].Close >= disp[0].Close
		if bDir != dDir {
			return fmt.Errorf("%s: cumulative direction flipped", inst.ID)
		}
		for _, max := range []bool{true, false} {
			bd, dd := argExtremum(base, max), argExtremum(disp, max)
			if abs(bd-dd) > extremumSlack {
				return fmt.Errorf("%s: extremum moved %d days (max=%v)", inst.ID, abs(bd-dd), max)
			}
		}
	}
	return nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd server && go test ./internal/engine/ -run TestFidelity -v`
Expected: PASS。若 `TestFidelityHoldsAcrossSeeds` 偶发失败：这是参数问题不是测试问题——优先收紧 `shocks.go` 的 `shockMean`（0.022 → 0.020）重跑，禁止放宽 spec 阈值。

- [ ] **Step 5: Commit**

```bash
git add server/internal/engine/fidelity.go server/internal/engine/fidelity_test.go
git commit -m "feat(engine): historical fidelity verification per spec 4.6"
```

---

### Task 8: GenerateWorld 编排 + golden test + CLI

**Files:**
- Create: `server/internal/engine/world.go`
- Create: `server/cmd/worldgen/main.go`
- Test: `server/internal/engine/world_test.go`

**Interfaces:**
- Consumes: Task 1–7 全部
- Produces:

```go
type World struct {
	ScenarioID string
	Seed       uint64
	Prices     map[string][]scenario.OHLC
	News       []NewsEvent // 按 Day 升序; 计划 2 据此入库 room_prices / room_news
}
// 编排: shocks → states → prices → historical+noise news → fallback copy → fidelity 校验
// fidelity 不过则返回 error（调用方换派生种子重试）
func GenerateWorld(sc *scenario.Scenario, seed uint64) (*World, error)
```

- [ ] **Step 1: 写失败的测试**

```go
// server/internal/engine/world_test.go
package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestGenerateWorldAssembles(t *testing.T) {
	sc := scenario.Synthetic()
	w, err := GenerateWorld(sc, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Prices) != len(sc.Instruments) {
		t.Fatalf("prices for %d instruments", len(w.Prices))
	}
	tracks := map[Track]int{}
	for i, ev := range w.News {
		if ev.Headline == "" {
			t.Fatalf("news %d missing headline", i)
		}
		tracks[ev.Track]++
	}
	for _, tr := range []Track{TrackHistorical, TrackImpact, TrackNoise} {
		if tracks[tr] == 0 {
			t.Fatalf("no %s-track news", tr)
		}
	}
	if !sort.SliceIsSorted(w.News, func(i, j int) bool { return w.News[i].Day < w.News[j].Day }) {
		t.Fatal("news not sorted by day")
	}
}

// Golden: 固定 (scenario, seed) 的世界哈希不随重构漂移。
// 首跑写入 testdata/world-42.sha256, 之后比对。
func TestGenerateWorldGolden(t *testing.T) {
	sc := scenario.Synthetic()
	w, err := GenerateWorld(sc, 42)
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(w)
	sum := sha256.Sum256(blob)
	got := hex.EncodeToString(sum[:])
	path := filepath.Join("testdata", "world-42.sha256")
	want, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(got), 0o644)
		t.Logf("golden recorded: %s", got)
		return
	}
	if got != string(want) {
		t.Fatalf("world changed for fixed seed:\n got %s\nwant %s", got, want)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd server && go test ./internal/engine/ -run TestGenerateWorld -v`
Expected: FAIL

- [ ] **Step 3: 实现 world.go 与 CLI**

```go
// server/internal/engine/world.go
package engine

import (
	"sort"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

type World struct {
	ScenarioID string
	Seed       uint64
	Prices     map[string][]scenario.OHLC
	News       []NewsEvent
}

// GenerateWorld builds one room's complete parallel world. Deterministic in
// (scenario, seed); returns an error if the fidelity gate fails, in which
// case the caller retries with a derived seed.
func GenerateWorld(sc *scenario.Scenario, seed uint64) (*World, error) {
	shocks := GenerateShockTimeline(sc, seed)
	states := EvolveFactorStates(sc, shocks)
	prices := SynthesizePrices(sc, states, seed)
	if err := VerifyFidelity(sc, prices); err != nil {
		return nil, err
	}
	news := append(append(shocks, HistoricalNews(sc)...), NoiseNews(sc, seed)...)
	FillFallbackCopy(sc, news, seed)
	sort.SliceStable(news, func(i, j int) bool { return news[i].Day < news[j].Day })
	return &World{ScenarioID: sc.ID, Seed: seed, Prices: prices, News: news}, nil
}
```

```go
// server/cmd/worldgen/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

func main() {
	seed := flag.Uint64("seed", 42, "room seed")
	flag.Parse()
	w, err := engine.GenerateWorld(scenario.Synthetic(), *seed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fidelity gate failed:", err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(w)
}
```

- [ ] **Step 4: 运行确认通过（跑两次，第二次验证 golden 比对生效）**

Run: `cd server && go test ./internal/engine/ -run TestGenerateWorld -v && go test ./internal/engine/ -run TestGenerateWorldGolden -v`
Expected: 两次均 PASS，第一次日志含 "golden recorded"

- [ ] **Step 5: CLI 冒烟**

Run: `cd server && go run ./cmd/worldgen -seed 7 | head -c 300`
Expected: 输出 JSON 前缀，含 `"ScenarioID":"synthetic-v1"`

- [ ] **Step 6: 全量测试 + Commit**

Run: `cd server && go vet ./... && go test ./...`
Expected: 全部 PASS

```bash
git add server/internal/engine/world.go server/internal/engine/world_test.go \
  server/internal/engine/testdata/ server/cmd/worldgen/
git commit -m "feat(engine): world generation orchestration, golden test, worldgen CLI"
```

---

## Self-Review 记录

- Spec 覆盖：本计划对应 spec §4（扰动引擎、三轨新闻、关键期抑制、保真四层中的 1/2/3/4 层——第 3 层在 Task 4、第 4 层在 Task 7）、§5.2（时钟）、§5.3（开局生成的纯计算部分）。§2/§3/§5.4/§6/§7/§8 属于计划 2–4。LLM 文案与真实数据剧本明确推迟到计划 4；`ReportShock`/ρ 已在本计划落地供其使用。
- 占位符扫描：无 TBD；所有测试与实现均给出完整代码。
- 类型一致性：`NewsEvent`/`Track`/`World`/`scenario.OHLC` 等签名在 Task 4/5/6/7/8 间核对一致；常量统一定义于 `shocks.go` 供 factors/prices 引用。
