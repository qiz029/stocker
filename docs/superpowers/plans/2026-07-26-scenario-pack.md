# Three-Scenario Pack: 1987 / 1972 / 2008 (Plan 5) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the single-scenario pipeline into a scenario registry and ship three new US-market eras — crash-1987 (Black Monday), nifty-1972 (Nifty Fifty collapse + oil crisis), gfc-2008 (financial crisis) — each passing the same fidelity and calibration gates as dotcom-2000, each with blind-box dossiers, era-appropriate LLM copy hints, and reconstructed dead companies where free data ends.

**Architecture:** One refactor task converts the pipeline's single `Universe` global into a registry (`map[id]*ScenarioUniverse`) and parametrizes the fidelity/shape/calibration tests over it — after that, each scenario is a pure data task: instrument list + factors + key windows + anchors (I provide) plus dossier prose (implementer authors per template, reviewer gates). `EraHint` travels inside `scenario.Scenario` (new column, migration 0005) so the LLM's system prompt gets per-era flavor without the llm package knowing about the pipeline. Reconstruction gains per-segment noise sigma so post-bankruptcy penny-stock tails carry realistic 15%+ daily volatility (this is what lets Lehman's tail pass the correlation floor).

**Tech Stack:** unchanged — Go stdlib, existing deps only. No frontend changes needed (the scenario picker is already data-driven).

## Global Constraints

- Dependency footprint unchanged (backend chi/pgx/x-crypto; llm stdlib). No frontend code changes expected; if a frontend test needs a fixture tweak, that is the only permissible web/ touch.
- Spec-locked engine shock constants untouched. The fidelity gate's post-plan-4 semantics (corr ≥ 0.85, extremumSlack ±10 OR twin-equivalence tol 0.18, direction exemption |net| < 0.13, near-flat handling) are FROZEN — new scenarios must pass the gate as-is; failures are fixed in scenario data (anchors, windows, universe composition) per the Task-4/5/6 playbooks, never by touching `engine/fidelity.go` thresholds. If a scenario structurally cannot pass, BLOCKED with numbers.
- Golden test hash unchanged by every task in this plan (no engine behavior changes; `EraHint` is data the engine ignores).
- Blind box: aliases/desc/dossiers/EraHint must contain no real company names, no real person names, no years/dates. Real identities only in `RealName` → reveal. EraHint feeds LLM prompts — same rules apply to it.
- Reconstruction honesty: every reconstructed series marked `Reconstructed: true`, RealName carries （重建） suffix, and BSC's post-acquisition tail additionally carries （对价模拟） because it models the takeover consideration value rather than a traded price.
- All game randomness engine-seeded; pipeline randomness fixed-PCG as before.
- Backend commits: vet clean, gofmt clean, full `STOCKER_TEST_DB=... go test ./... -count=1 -short` green per commit; the slow gates (`-count=1` non-short) run in Task 3 onward whenever a universe changes and in the final sweep.
- Network only in Task 3 (one-time Yahoo fetch), same politeness/failure policy as plan 4's fetch.

## File Structure

```
server/internal/pipeline/
  universe.go            # shared types + registry: ScenarioUniverse, Register, Universes, ByID
  universe_dotcom.go     # existing dotcom data moved here, registered
  universe_1987.go       # Task 4
  universe_1972.go       # Task 5
  universe_2008.go       # Task 6
  build.go               # BuildScenario(id)/BuildMeta(id); EraHint copied into scenario
  reconstruct.go         # Anchor.Sigma per-segment noise
  registry_test.go       # parametrized shape/fidelity over the registry (Task 1)
  rawdata/*.csv          # + ~20 new symbol files (Task 3)
server/internal/scenario/types.go    # Scenario.EraHint
server/internal/store/migrations/0005_era_hint.sql
server/internal/store/scenarios.go   # EraHint round-trip
server/internal/llm/llm.go           # system prompt uses sc.EraHint
server/cmd/pipeline/main.go          # import -scenario flag; fetch unions all universes
```

---

### Task 1: Scenario registry, EraHint plumbing, parametrized gates

**Files:**
- Modify: `server/internal/pipeline/universe.go` (registry types), create `server/internal/pipeline/universe_dotcom.go` (move data)
- Modify: `server/internal/pipeline/build.go` (`BuildScenario(id)`, `BuildMeta(id)`)
- Modify: `server/internal/scenario/types.go` (+`EraHint string` on Scenario)
- Create: `server/internal/store/migrations/0005_era_hint.sql`; modify `server/internal/store/scenarios.go` (round-trip), `db_test.go` (5 migrations)
- Modify: `server/internal/llm/llm.go` (era hint into system prompt)
- Modify: `server/cmd/pipeline/main.go` (`import -scenario <id|all>`; fetch = union of all universes)
- Create: `server/internal/pipeline/registry_test.go`; modify existing pipeline tests to call `BuildScenario("dotcom-2000")`
- Test: adjust `server/internal/llm/llm_test.go` (era hint assertion), `server/internal/store/scenarios_test.go` (EraHint round-trip)

**Interfaces:**
- Consumes: everything from plan 4.
- Produces:
  - `type ScenarioUniverse struct { ScenarioID, Name, RealPeriod, EraHint, WindowStart, WindowEnd string; MarketProxy string; Sectors, Macros []SectorSpec; Instruments []InstrumentSpec; KeyWindows []DateWindow; FidelitySeeds int }` — `MarketProxy` names the instrument whose returns define MKT (was hardcoded `"X22"`); `FidelitySeeds` defaults 12
  - `func Register(u *ScenarioUniverse)` (panics on duplicate id — static data), `func Universes() []string` (sorted ids), `func ByID(id string) (*ScenarioUniverse, bool)`
  - `func BuildScenario(id string) (*scenario.Scenario, error)`, `func BuildMeta(id string) (Meta, error)` — unknown id errors; dotcom behavior byte-identical to plan 4 (existing dotcom tests keep passing with the id argument)
  - `scenario.Scenario.EraHint string` — persisted via `scenarios.era_hint` (migration 0005), round-tripped by Save/Load, engine ignores it
  - `llm` system prompt: the hardcoded era line `游戏背景是一个类似上世纪末科技泡沫的虚构平行世界` becomes `游戏背景是一个%s的虚构平行世界` filled from `sc.EraHint`, falling back to `架空年代` when empty; dotcom's registered `EraHint = "类似世纪之交科技股狂热"`
  - `registry_test.go`: `TestRegistryLists` (dotcom present, sorted, ByID roundtrip, unknown missing); `TestAllScenariosShape` — for every registered id: builds, `Days > 200`, every instrument has alias ≠ "" and `Beta["IDIO:"+ID] == 1` and IdioScale within [0.1, 3], baselines normalized to 100 and full-length, all beta keys declared as factors, key windows in range, deterministic rebuild (DeepEqual), no real names in alias/desc (spot list: `Microsoft|Cisco|Intel|IBM|Apple|Amazon|Lehman|Goldman|Kodak|Xerox` must not appear in alias/desc/EraHint); `TestAllScenariosFidelity` (non-short) — for every id: `FidelitySeeds` seeds through `engine.GenerateWorld`; `TestAllScenariosCalibration` (non-short) — per id, 4 seeds (dotcom keeps its own 10-seed test too), same stat bands as plan 4's calibration test
  - `cmd/pipeline import -scenario dotcom-2000` (or `all`, the default) imports the chosen universes; `fetch` downloads the union of every registered universe's FetchSpecs (deduped by name), skipping files that already exist unless `-force`

- [ ] **Step 1: Write the failing tests** — registry_test.go with the four tests above (full code per the interfaces; the shape test loops `Universes()`); store EraHint round-trip appended to scenarios_test.go (`sc.EraHint = "测试年代"` before Save, assert after Load); llm test extended: register the fake scenario with `EraHint: "类似某个狂热"` and assert the marshaled system message contains it.
- [ ] **Step 2: Run to verify failures** (`go test ./internal/pipeline/ ./internal/store/ ./internal/llm/ -short`).
- [ ] **Step 3: Implement** — mechanical refactor: move dotcom's literal into `universe_dotcom.go` with `func init() { Register(&dotcomUniverse) }`; `build.go` looks up by id, threads `MarketProxy` (dotcom: "X22") and copies `EraHint` onto the scenario; migration 0005 single ALTER; Save/Load add the column; llm formats the system prompt once per FillCopy call (`fmt.Sprintf` on a `const systemPromptTmpl`); cmd flag via `flag.NewFlagSet("import", ...)`.
- [ ] **Step 4: Full suite** — `-short` green everywhere, then non-short pipeline once (`go test ./internal/pipeline/ -count=1`) to confirm dotcom's 12-seed gate still passes through the registry path. Golden unchanged.
- [ ] **Step 5: Vet, gofmt, commit** — `feat(pipeline): scenario registry with parametrized gates and era hints`

---

### Task 2: Reconstruction per-segment noise sigma

**Files:**
- Modify: `server/internal/pipeline/reconstruct.go`
- Test: `server/internal/pipeline/reconstruct_test.go` (append)

**Interfaces:**
- Consumes: Task-4-of-plan-4 `Reconstruct`.
- Produces: `Anchor` gains `Sigma float64` — the daily log-noise sigma for the segment ENDING at this anchor; `0` means the default `reconNoiseSigma = 0.025`. The first anchor's Sigma is ignored (no segment ends there). Wick sigma stays global. This exists so post-bankruptcy penny tails can carry realistic 12-20% daily volatility — which is what keeps a dead stock's 15-month tail from diluting its return correlation below the 0.85 floor (penny stocks really do swing like that; it is historical honesty, not a trick).

- [ ] **Step 1: Failing test** — append `TestReconstructSegmentSigma`: two-segment series (day 0→200 default sigma, day 200→400 anchor with `Sigma: 0.15`); assert measured tail-segment daily vol ∈ [0.10, 0.22] and head-segment vol ∈ [0.015, 0.045]; anchors still hit exactly; determinism preserved.
- [ ] **Step 2: Verify fail.**
- [ ] **Step 3: Implement** — thread per-segment sigma into the bridge walk (`rng.NormFloat64()*segSigma`); validation: negative Sigma errors.
- [ ] **Step 4: Pipeline suite green** (dotcom anchors have Sigma 0 → default → byte-identical output; assert by running `TestBuildScenarioShape`'s determinism check unchanged).
- [ ] **Step 5: Vet, commit** — `feat(pipeline): per-segment reconstruction noise for penny-stock tails`

---

### Task 3: Fetch the three eras' raw data (network, one-time)

**Files:**
- Modify: `server/internal/pipeline/universe.go` or per-universe files: FetchSpecs now live on each `ScenarioUniverse` (`FetchList` global becomes the dotcom universe's list; `fetch` unions all)
- Create (by running fetch): ~20 new `rawdata/*.csv`
- Test: extend `csv_test.go`'s embedded-load test to iterate all universes' specs with per-spec window bounds

**New symbols and their trim windows** (Yahoo chart API, same client as plan 4; pre-1970 dates need negative epoch seconds — compute `period1` per spec):

| name | Yahoo | needed window | for |
|---|---|---|---|
| ko | KO | 1970-06-01..1989-06-30 | 1972+1987 |
| mcd | MCD | 1970-06-01..1989-06-30 | 1972+1987 |
| dis | DIS | 1970-06-01..1989-06-30 | 1972+1987 |
| jnj | JNJ | 1970-06-01..1989-06-30 | 1972+1987 |
| pg | PG | 1970-06-01..1989-06-30 | 1972+1987 |
| mmm | MMM | 1970-06-01..1989-06-30 | 1972+1987 |
| cat | CAT | 1970-06-01..1989-06-30 | 1972+1987 |
| mrk | MRK | 1985-06-01..1989-06-30 | 1987 |
| ba | BA | 1985-06-01..1989-06-30 | 1987 |
| axp | AXP | 1985-06-01..1989-06-30 | 1987 |
| dji | ^DJI | 1970-06-01..2010-06-30 | all three |
| xrx | XRX | 1970-06-01..1976-06-30 | 1972 (fallback: reconstruct) |
| avp | AVP | 1970-06-01..1976-06-30 | 1972 (fallback: reconstruct) |
| ek | EK | 1970-06-01..1976-06-30 | 1972 (fallback: reconstruct) |
| gs | GS | 2006-04-01..2010-06-30 | 2008 |
| ms | MS | 2006-04-01..2010-06-30 | 2008 |
| jpm | JPM | 2006-04-01..2010-06-30 | 2008 |
| bac | BAC | 2006-04-01..2010-06-30 | 2008 |
| c | C | 2006-04-01..2010-06-30 | 2008 |
| wfc | WFC | 2006-04-01..2010-06-30 | 2008 |
| aig | AIG | 2006-04-01..2010-06-30 | 2008 |
| len | LEN | 2006-04-01..2010-06-30 | 2008 |
| gld | GLD | 2006-04-01..2010-06-30 | 2008 |
| cvx | CVX | 2006-04-01..2010-06-30 | 2008 |
| aapl08/ge08/xom08/wmt08/mcd08/spx-extensions | — | — | REUSE: widen existing files instead (see below) |

Existing dotcom-era files (aapl/ge/xom/wmt/mcd already exist only for 1998-2002; spx/ibm/hpq likewise): symbols needed by multiple eras get ONE file covering the union of their windows — re-fetch `spx (^GSPC) 1970-06-01..2010-06-30`, `ibm 1970-06-01..1989-06-30 ∪ 1998-06..2002-03` (fetch 1970-06..2002-03 whole), `hpq 1985-06..2002-03`, `aapl/ge/xom/wmt 1985-06..2010-06-30` (covers 1987+dotcom+2008 as applicable), `mcd08 → mcd file covers 1970..1989 plus 2006..2010`: for symbols with disjoint needs, fetch the FULL span between earliest and latest need (daily rows are ~35 bytes; a 40-year file ≈ 350KB — acceptable for ibm/spx/dji; document sizes in the report). Alignment slices per scenario window at build time, so oversized files cost repo bytes only.

**Availability contingency (pre-authorized):** XRX/AVP/EK may lack pre-1977 rows on Yahoo (coverage varies). Policy: if a symbol's data does not start by the scenario window's first day, DROP the file and mark the instrument `Raw: ""` — Task 5's anchor tables cover all three as reconstruction fallbacks. Everything else (all 2008-era symbols, all 1985+ symbols, indices) failing = BLOCKED.

**Verification:** per new file record first/last dates + row count; sanity spot-checks in the report: ^DJI 1987-10-19 close ≈ 1738 (−22.6%), C 2009-03 low < $1.5, AIG 2008-09 collapse, KO early-1970s split-adjusted under $2. Embedded-load test asserts each spec's window coverage.

Commit: `feat(pipeline): raw market data for the 1987, 1972 and 2008 eras`

---
## Dossier authoring policy (applies to Tasks 4–6)

Dossier prose (`Business`/`Bull`/`Bear`) is authored content, not code. The plan deliberately delegates the writing to the implementer under this spec — the reviewer gates it like any requirement:

- 1–2 sentences per field, in the register of the dotcom dossiers (read `universe_dotcom.go` first; two fresh examples are given per scenario below).
- Era-accurate: business models, technologies, and market narratives must fit the era; no anachronisms (no internet references in 1972, no smartphones in 1987, etc.).
- Blind-box: no real company/person names, no years, no numbers precise enough to identify the company.
- `Bull` reads like the era's bull case (what believers said THEN); `Bear` like the era's skeptic case — ideally the one history vindicated. The pair should be a playable clue, not a spoiler.
- Reviewer checklist: every field non-empty; era accuracy; blind-box; style match.

### Task 4: crash-1987 universe (Black Monday)

**Files:** create `server/internal/pipeline/universe_1987.go`; test additions ride on the Task-1 parametrized suite (add one scenario-specific test: the crash-day sanity below).

**Hard data (transcribe exactly; author the dossier prose per the policy):**

```
ScenarioID "crash-1987"  Name "1987 黑色星期一"  RealPeriod "1986-01 ~ 1988-12"
EraHint "类似程序化交易与并购热潮推起大牛市的年代"
WindowStart 1986-01-02  WindowEnd 1988-12-30   MarketProxy "Y17" (SPX)
Sectors: IND 工业制造 / CONS 消费品牌 / PHRM 医药 / TECH 计算机 / FIN 金融 / ENGY 能源 / RETL 零售
Macros:  GOLD 黄金 / OIL 原油 / RATE 利率   (curated betas only — same as dotcom)
KeyWindows: 1987-01-02..1987-08-25 direction +1 (疯牛期，压制逆势冷水)
            1987-10-14..1987-10-30 direction -1 (崩盘周)
Instruments (id / raw / sector / alias / desc / RealName):
 Y01 ibm  TECH Atlas Computing Corp.   百年计算公司                     IBM
 Y02 hpq  TECH Camberwell Instruments   老牌硅谷仪器厂                   Hewlett-Packard
 Y03 ko   CONS Harborline Beverages   全球装瓶的饮料霸主               Coca-Cola
 Y04 pg   CONS Willowbrook Consumer Goods   百年日化帝国                     Procter & Gamble
 Y05 mcd  CONS Roadside Restaurant Group 全球连锁快餐之王                 McDonald's
 Y06 dis  CONS Silverglade Entertainment   动画起家的娱乐帝国               Disney
 Y07 mrk  PHRM Vireo Therapeutics   研发驱动的大药厂                 Merck
 Y08 jnj  PHRM Wellmark Health Group   从婴儿爽身粉到手术缝线           Johnson & Johnson
 Y09 mmm  IND  Stratford Materials   什么都能粘的材料公司             3M
 Y10 ba   IND  Continental Aerospace   民航客机双雄之一                 Boeing
 Y11 cat  IND  Ironfield Heavy Industries   推土机与矿山机械之王             Caterpillar
 Y12 ge   IND  Concord Industrial Group   什么都造的工业帝国               General Electric
 Y13 axp  FIN  Marquis Financial Services   高端签账卡与旅行支票             American Express
 Y14 xom  ENGY Bastion Petroleum   全球油气巨轮                     Exxon
 Y15 wmt  RETL Plainfield Stores   乡镇起家的零售之王               Walmart
 Y16 dji  ""   Legacy Bluechip Index   三十家工业巨头的价格加权指数     道琼斯工业指数 (sector "", index)
 Y17 spx  ""   Premier 500 Index   五百家大公司的市值加权指数       S&P 500 指数
ExtraBeta guidance: SENT 0.2-0.5 (higher for CONS growth names & indices), RATE -0.2..-0.5
  (1987's crash trigger was rate fear — give FIN/IND the stronger negative RATE), OIL 0.6 for Y14, GOLD 0.1 for Y14.
```

Two dossier examples to match (transcribe as Y03 and Y13's dossiers, author the rest):
- Y03 Harborline Beverages — Business: "一瓶糖水卖遍全球的品牌机器，装瓶网络深入每个国家的毛细血管。" Bull: "品牌就是复利：只要人还口渴，提价权与全球化就是双引擎，牛市里机构最安心的核心持仓。" Bear: "所有人都安心的持仓，估值早已不便宜；当组合保险开始机械抛售时，最拥挤的地方跌得最快。"
- Y13 Marquis Financial Services — Business: "高端客群的签账卡与旅行支票生意，赚的是商户回佣与浮存金。" Bull: "消费升级的年代，绿色卡片是身份的通行证，会员费提了又提照样排队。" Bear: "利率一抬头，浮存金优势缩水；金融股在恐慌里从没当过避风港。"

**Scenario-specific test** (append to registry_test.go): `TestCrash1987SingleDayDrop` — build crash-1987, find the baseline day where the SPX-proxy instrument (Y17) has its worst single-day log return; assert that return ≤ −0.18 (the −20%+ crash day survives alignment/normalization) and that day falls inside the second key window.

**Fidelity playbook for this task:** all-real data, no anchors to tune. If the gate fails: (a) correlation → the instrument's vol budget handles it automatically, investigate only if a specific stock fails all seeds; (b) extremum — 1987's peak (Aug 25) and crash trough are sharp, twin-equivalence should absorb the rest; (c) direction — every net here is large except possibly Y16/Y17 over 3 years (indices ended ABOVE 1986 start despite the crash — net ≈ +0.15..+0.25; if an index sits in (0.13, 0.20) and flips on some seed, report the numbers as BLOCKED rather than tuning — that's a controller decision).

Commit: `feat(pipeline): crash-1987 scenario (black monday)`

### Task 5: nifty-1972 universe (漂亮50崩塌 + 石油危机)

**Files:** create `server/internal/pipeline/universe_1972.go`.

**Hard data:**

```
ScenarioID "nifty-1972"  Name "1972 漂亮50与石油危机"  RealPeriod "1972-01 ~ 1975-06"
EraHint "类似蓝筹成长股信仰与石油危机交织的年代"
WindowStart 1972-01-03  WindowEnd 1975-06-30   MarketProxy "N15" (SPX)
Sectors: NIFT 一流成长 / OFFC 办公科技 / IND 工业 / ENGY 能源
Macros:  GOLD 黄金 / OIL 原油 / RATE 利率
KeyWindows: 1973-11-01..1974-01-04 direction -1 (禁运恐慌)
            1974-08-01..1974-10-04 direction -1 (投降底)
Instruments:
 N01 ko   NIFT Suncrest Beverages   全球装瓶的饮料霸主           Coca-Cola
 N02 mcd  NIFT Crossroads Restaurants 高速展店的快餐新贵           McDonald's
 N03 dis  NIFT Carousel Entertainment   刚开完新乐园的娱乐帝国       Disney
 N04 jnj  NIFT Cloverleaf Health   永远增长的健康帝国           Johnson & Johnson
 N05 pg   NIFT Kirkwood Household Brands   百年日化帝国                 Procter & Gamble
 N06 ibm  OFFC Sterling Computing   大型计算机的代名词           IBM
 N07 xrx  OFFC Paperflow Systems     办公室复印机的代名词         Xerox        (fetch; fallback anchors below)
 N08 ek   NIFT Silvergrain Photographics     感光胶卷与相机霸主           Eastman Kodak (fetch; fallback anchors below)
 N09 prd  NIFT Lumina Optics   即时成像相机的发明者         Polaroid（重建）(always reconstructed)
 N10 avp  NIFT Homeway Cosmetics   上门直销的化妆品帝国         Avon         (fetch; fallback anchors below)
 N11 mmm  IND  Versatek Materials   什么都能粘的材料公司         3M
 N12 cat  IND  Quarryline Heavy Industries   推土机与矿山机械之王         Caterpillar
 N13 xom  ENGY Ironshore Petroleum   全球油气巨轮                 Exxon
 N14 dji  ""   Founders Bluechip Index   三十家工业巨头指数           道琼斯工业指数
 N15 spx  ""   Bluechip 500 Index   五百家大公司指数             S&P 500 指数
ExtraBeta guidance: SENT 0.5-0.8 for NIFT (信仰股情绪), OIL +0.7 for N13 and −0.2 for NIFT
  (油危机反噬消费), RATE −0.4 for NIFT (贴现率杀估值), GOLD 0.15 for N13.
Anchors (used when Yahoo lacks pre-window data; prices are split-adjusted-era approximations,
documented as such; Sigma 0 = default):
 prd: 1972-01-03: 90 / 1972-06-01: 110 / 1973-01-02: 140 / 1973-06-01: 120 /
      1973-12-03: 70 / 1974-06-03: 40 / 1974-10-01: 15 / 1975-06-30: 30
 ek(fallback):  1972-01-03: 90 / 1973-01-02: 145 / 1973-12-03: 110 / 1974-10-01: 60 / 1975-06-30: 95
 xrx(fallback): 1972-01-03: 140 / 1972-12-01: 165 / 1973-12-03: 110 / 1974-10-01: 50 / 1975-06-30: 65
 avp(fallback): 1972-01-03: 100 / 1973-03-01: 135 / 1974-01-02: 60 / 1974-10-01: 19 / 1975-06-30: 35
```

Dossier examples (transcribe as N09 and N07, author the rest):
- N09 Lumina Optics — Business: "按下快门一分钟后照片就在手里——即时成像的独家专利帝国，毛利像奢侈品。" Bull: "一次性决策股：这种公司买了就永远不用卖，付五十倍市盈率是为未来三十年付的。" Bear: "为'永远'付的价格，只要增长慢一个季度就会塌方；专利到期与新技术是永远悬着的剑。"
- N07 Paperflow Systems — Business: "每台复印机都是印钞机——设备租赁加按张计费，办公室离不开它。" Bear: "当核心专利保护伞收起、模仿者涌入时，按张计费的暴利模式首当其冲。" Bull: "无纸化还是科幻小说，纸张洪流只增不减，装机量就是年金。"

**Scenario-specific test:** `TestNifty1972BearMarketDepth` — build; assert the SPX proxy's max drawdown from its window peak exceeds 40% in the baseline (log peak-to-trough ≤ −0.5), and at least 4 NIFT-sector instruments have baseline drawdowns ≥ 60% — the "最优质的公司也跌八成" texture is the scenario's whole point; if the real data disagrees, the window/universe needs a second look (BLOCKED with numbers, not silent acceptance).

**Playbook:** the three fetch-or-fallback instruments are the risk. If a fetched series covers the window but starts mid-window, use the DROP policy (Task 3) and its anchors. Anchor extremes here are deep (−80% troughs) with wide margins; if extremum still drifts for a reconstructed name, tighten tail anchors per the plan-4 rule (≥0.15 log margin, ≤25-day segments near the extreme).

Commit: `feat(pipeline): nifty-1972 scenario (nifty fifty collapse and oil crisis)`

### Task 6: gfc-2008 universe (金融危机)

**Files:** create `server/internal/pipeline/universe_2008.go`.

**Hard data:**

```
ScenarioID "gfc-2008"  Name "2008 金融危机"  RealPeriod "2006-10 ~ 2009-12"
EraHint "类似信贷与地产狂欢滑向系统性金融危机的年代"
WindowStart 2006-10-02  WindowEnd 2009-12-31   MarketProxy "G17" (SPX)
Sectors: IBNK 投资银行 / BANK 商业银行 / INSR 保险 / HOME 地产建商 / DEFC 防御消费 / TECH 科技 / ENGY 能源
Macros:  GOLD 黄金 / OIL 原油 / RATE 利率
KeyWindows: 2008-03-10..2008-03-20 direction -1 (贝尔斯登周)
            2008-09-08..2008-10-10 direction -1 (雷曼连锁崩塌)
            2009-03-02..2009-03-31 direction +1 (绝望底反转)
Instruments:
 G01 gs   IBNK Sovereign Capital Partners   投行之王，自营交易机器           Goldman Sachs
 G02 ms   IBNK Whitfield Securities   老牌白鞋投行                     Morgan Stanley
 G03 ""   IBNK Hartwell Investment Bank 债券起家的百年投行               Lehman Brothers（重建）
 G04 ""   IBNK Stonebridge Securities   按揭证券化的先锋                 Bear Stearns（重建·后段为对价模拟）
 G05 jpm  BANK Metropolitan Bank Corp   大而稳的全能银行                 JPMorgan Chase
 G06 bac  BANK Heartland National Bank   零售网点最多的巨无霸             Bank of America
 G07 c    BANK Panorama Financial Group   全球扩张最激进的银行             Citigroup
 G08 wfc  BANK Homestead Bank & Trust   西部起家的房贷大行               Wells Fargo
 G09 aig  INSR Worldspan Insurance Group   保险帝国与它神秘的衍生品部门     AIG
 G10 len  HOME Sunhaven Homes   全国性住宅建商                   Lennar
 G11 wmt  DEFC Ledgemont Retail Group   乡镇起家的零售之王               Walmart
 G12 mcd  DEFC Quickbite Restaurants 全球连锁快餐之王                 McDonald's
 G13 aapl TECH Copperleaf Technologies   刚发布革命性手机的电脑厂         Apple
 G14 xom  ENGY Vanguard Petroleum   全球油气巨轮                     ExxonMobil
 G15 cvx  ENGY Blueridge Energy   全产业链油气巨头                 Chevron
 G16 gld  ""   HardAsset Trust   黄金现货支持的基金               SPDR Gold（sector ""）
 G17 spx  ""   National 500 Index   五百家大公司指数                 S&P 500 指数
 G18 dji  ""   Heritage Bluechip Index   三十家巨头指数                   道琼斯工业指数
ExtraBeta: SENT 0.6-0.9 for IBNK/BANK/INSR/HOME, 0.0-0.2 for DEFC; RATE −0.5 for financials;
 GOLD: G16 gets GOLD 0.9 (it IS the gold instrument), others 0-0.1; OIL 0.7 for ENGY.
Anchors (approximate, documented; Sigma marked where non-default):
 G03 leh: 2006-10-02: 74 / 2007-02-01: 84 / 2007-06-01: 76 / 2008-01-02: 62 /
   2008-03-17: 32 / 2008-06-02: 33 / 2008-09-02: 16 / 2008-09-09: 7.8 /
   2008-09-12: 3.65 / 2008-09-15: 0.21 {Sigma 0.25} / 2008-10-01: 0.10 {Sigma 0.18} /
   2009-03-02: 0.05 {Sigma 0.15} / 2009-12-31: 0.032 {Sigma 0.15}
   (破产后 OTC 仙股阶段真实存在且波动巨大——高 Sigma 是历史诚实)
 G04 bsc: 2006-10-02: 138 / 2007-01-16: 168 / 2007-08-01: 115 / 2008-01-02: 85 /
   2008-03-13: 57 / 2008-03-17: 4.8 / 2008-03-24: 10.9 / 2008-05-30: 9.9 /
   2008-11-20: 5.2 {Sigma 0.05} / 2009-03-09: 3.9 {Sigma 0.05} / 2009-06-01: 7.4 {Sigma 0.04} /
   2009-12-31: 9.0 {Sigma 0.04}
   (2008-06 之后按换股对价价值模拟——RealName 已标注（对价模拟），揭晓页会如实呈现)
```

Dossier examples (transcribe as G03 and G09, author the rest):
- G03 Hartwell Investment Bank — Business: "债券承销起家的百年投行，这些年最赚钱的部门是把千千万万笔房贷打包成证券再卖出去。" Bull: "熬过一个半世纪所有危机的公司还会怕这一次？杠杆是利润的放大器，管理层说流动性充足。" Bear: "三十倍杠杆意味着资产跌百分之三就资不抵债；当抵押品是别人不想再要的东西，'充足'两个字撑不过一个周末。"
- G09 Worldspan Insurance Group — Business: "全球最大的保险帝国，但利润引擎藏在一个几百人的衍生品部门——他们为天量的债券违约风险卖了保险。" Bull: "百年一遇的违约潮才能伤到它，而它的精算师说那是不可能事件。" Bear: "卖的是'不可能事件'的保险，收的是确定的小钱，赌上的是整个集团；不可能事件只需要发生一次。"

**Scenario-specific test:** `TestGfc2008CrisisShape` — build; assert G03's baseline net log return ≤ −5 (雷曼归零), G09 drawdown ≥ 90%, the DEFC instruments' 2008 drawdowns < 40% (防御股确实防御), and the +1 key window's start day sits within 10 days of the SPX proxy's global baseline trough (2009-03-09 must be the bottom).

**Playbook:** G03's penny tail is the fidelity stress case this task exists for — if its correlation fails, tail Sigmas may go UP (more historically honest, more baseline variance), never down; if BSC's low-Sigma tail fails correlation, raise its tail Sigmas toward 0.06 (deal-spread volatility was real). Extremum/direction should be trivially safe (enormous nets, sharp extremes). Two tuning rounds max, then BLOCKED with numbers.

Commit: `feat(pipeline): gfc-2008 scenario (financial crisis)`

### Task 7: Final sweep — import all, per-scenario smoke, docs

**Files:** modify `server/README.md`, root `README.md` (scenario table); test = full gates + live smoke.

- [ ] Full non-short suite: `STOCKER_TEST_DB=... go test ./... -count=1` — all four scenarios through fidelity + calibration gates; golden unchanged; frontend suite untouched but run once (`npx vitest run`) to confirm nothing drifted.
- [ ] Live smoke: throwaway db; `go run ./cmd/pipeline import` (all four); `GET /api/scenarios` lists 4 with correct day counts; for EACH new scenario create a room (60s/day; world gen for ~750-day universes takes ~30s each), check instruments have Chinese aliases and no real names, news carries era-appropriate template copy, place+settle one order in the 1987 room across the crash day if timing allows (order placed day-of-crash settles at the gapped-down open — record the fill as a gameplay sanity note). Reveal 409 mid-game. Kill server, dropdb, record everything.
- [ ] README scenario table:

```markdown
| 剧本 | 时期 | 交易日 | 标的 | 剧情 |
|---|---|---|---|---|
| 2000 互联网泡沫 | 1999-01 ~ 2001-12 | ~752 | 22 | 泡沫、见顶、漫长阴跌 |
| 1987 黑色星期一 | 1986-01 ~ 1988-12 | ~756 | 17 | 疯牛、单日 −22%、默默收复 |
| 1972 漂亮50 | 1972-01 ~ 1975-06 | ~875 | 15 | 信仰蓝筹的慢刀子 + 石油危机 |
| 2008 金融危机 | 2006-10 ~ 2009-12 | ~815 | 18 | 系统性崩塌与绝望底反转 |
```

- [ ] Commit: `docs: four-scenario lineup`
