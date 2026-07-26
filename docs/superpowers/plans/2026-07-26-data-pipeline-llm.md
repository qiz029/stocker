# Data Pipeline + LLM News Copy (Plan 4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the synthetic demo scenario with the real 2000 dot-com scenario (22 blind-box instruments: 16 real Stooq series + 4 anchor-reconstructed dead companies + 2 indices, 1999-01 → 2001-12), add per-stock perturbation calibration and news clusters to the engine, generate news copy with an OpenAI-compatible LLM at room creation (DeepSeek-ready, template fallback), and close the deferred API/frontend follow-ups (scenario picker, is_host, reveal real period, Monte Carlo property tests).

**Architecture:** The pipeline is dev-time only: raw Stooq CSVs are fetched once and committed as `go:embed` data inside `internal/pipeline`, so builds, tests, and CI never touch the network. `cmd/pipeline import` builds the scenario in-process (align → normalize → β regression → idio calibration → dossiers) and writes it to Postgres through the existing store. The LLM is the game's single external dependency, invoked exactly once per room creation with bounded concurrency and a hard timeout; any failure leaves the engine's template copy in place (spec §5.1).

**Tech Stack:** Go stdlib only for the pipeline and LLM client (NO new Go modules). Frontend: no new npm deps.

## Global Constraints

- Backend deps stay exactly chi / pgx / x-crypto; frontend runtime deps stay exactly react / react-dom / react-router-dom. The LLM client is stdlib `net/http` + `encoding/json` against an OpenAI-compatible `/chat/completions` endpoint.
- Zero runtime external dependencies except the one-shot LLM call at room creation (spec §5.1); `LLM_BASE_URL` unset → template copy, no network, all tests offline.
- Engine spec-locked shock constants (`pMarketDaily=0.15, pSectorDaily=0.25, pIdioDaily=0.05, shockMean=0.022, pPositive=0.45, bigShock=0.02, pFlipInWin=0.8, lamFast=0.7, lamSlow=0.99, fracFast=0.65, clampX=0.30, epsSigma=0.004`) are NOT modified. Per-stock calibration happens only through the new `Instrument.IdioScale` multiplier (spec §4.2 sanctions this: "真实基线数据进来后逐股校准").
- The golden test hash changes ONCE, in Task 2 (news clusters add events to the world). The task regenerates `testdata/world-42.sha256` and documents why in the commit. Task 1 must NOT change the hash (IdioScale defaults to a no-op ×1.0).
- Blind box: real names, real dates, and the scenario's real period reach the client ONLY via the reveal endpoint. LLM prompts receive aliases and factor display names — never real company names or real dates (era flavor only), so generated copy cannot leak identity.
- Reconstructed instruments (WCOM/LU/NT/GBLX) are approximations built from documented anchor points; the code marks them `Reconstructed: true` and the dossier data keeps the anchor tables reviewable. Pets.com needs mid-scenario listing support — explicitly deferred to phase 2.
- Money/display conventions unchanged. All new randomness is engine-seeded (`engine.Stream`) or pipeline-seeded (fixed PCG constants); `crypto/rand` only where plan 2 already used it.
- Backend commits: `go vet` clean, `gofmt -l` empty, full `STOCKER_TEST_DB=... go test ./... -count=1` green. Frontend commits: `npx vitest run` green, `npx tsc --noEmit` clean, `npm run build` succeeds. Slow calibration tests honor `go test -short` skipping.

## File Structure

```
server/
  internal/scenario/types.go        # + Instrument.IdioScale, Instrument.Reconstructed
  internal/scenario/synthetic.go    # instruments set IdioScale: 1 explicitly
  internal/engine/shocks.go         # idio shocks × IdioScale; ExpandClusters; NewsEvent.ClusterID
  internal/engine/news.go           # cluster fallback copy (【传闻】/【追踪】prefixes)
  internal/engine/world.go          # GenerateWorld calls ExpandClusters
  internal/engine/testdata/world-42.sha256   # regenerated once (Task 2)
  internal/pipeline/
    csv.go                          # Stooq CSV parsing + Bar type
    rawdata/*.csv                   # committed one-shot Stooq downloads (go:embed)
    reconstruct.go                  # anchor interpolation with bridge noise
    universe.go                     # the 2000-scenario data file: tickers, factors,
                                    # aliases, dossiers, real names, anchors, windows
    build.go                        # align/normalize/β-regression/calibration → *scenario.Scenario
    build_test.go  csv_test.go  reconstruct_test.go
    fidelity_test.go                # multi-seed VerifyFidelity on the built scenario
    calibration_test.go             # Monte Carlo stats vs spec §4.2 targets (-short skips)
  internal/store/migrations/0003_dotcom.sql  # scenarios.name/.real_period, instruments.idio_scale,
                                             # room_news.cluster_id
  internal/store/scenarios.go       # IdioScale round-trip; SetScenarioMeta; InstrumentDisplay.RealName
  internal/store/rooms.go           # CreateRoom(..., filler NewsCopyFiller); news CopyFrom + body/cluster_id
  internal/llm/llm.go               # OpenAI-compatible batch copy generator (+ llm_test.go)
  internal/httpapi/rooms.go         # GET /api/scenarios; roomJSON is_host; CreateRoom filler
  internal/httpapi/reveal.go        # + real_period
  cmd/pipeline/main.go              # subcommands: fetch (network, dev-only), import (build → Postgres)
  cmd/server/main.go                # wire llm.FromEnv() into httpapi.Server
web/
  src/api.ts                        # ScenarioInfo, Room.is_host, RevealData.real_period
  src/pages/Lobby.tsx               # scenario picker + durations computed from days
  src/pages/Room.tsx                # start button gated on is_host
  src/pages/Reveal.tsx              # real period display
```

Engine interfaces consumed (plan 1, unchanged unless listed): `GenerateWorld`, `GenerateShockTimeline`, `EvolveFactorStates`, `SynthesizePrices`, `VerifyFidelity`, `Stream`, `NewsEvent{Day, Track, MediaID, TrueShock, ReportShock, Headline}` (+ new `Body`, `ClusterID` fields added by Tasks 2/8-9 — see tasks), `MediaTable` (rho: wire .9, paper .75, tv .6, tabloid .4, forum .25).

---

### Task 1: Engine — per-instrument idio calibration (IdioScale)

**Files:**
- Modify: `server/internal/scenario/types.go`, `server/internal/scenario/synthetic.go`
- Modify: `server/internal/engine/shocks.go`
- Test: `server/internal/engine/shocks_test.go` (append), `server/internal/engine/world_test.go` (golden must NOT change)

**Interfaces:**
- Consumes: existing engine/scenario.
- Produces:
  - `scenario.Instrument` gains `IdioScale float64` and `Reconstructed bool`
  - `func (i *Instrument) IdioScaleOrDefault() float64` — returns 1 when `IdioScale <= 0`
  - `GenerateShockTimeline` multiplies idio shock magnitudes by the instrument's scale (market/sector shocks untouched)
  - Golden hash unchanged (synthetic sets `IdioScale: 1`; ×1.0 is float-exact)

- [ ] **Step 1: Write the failing test**

Append to `server/internal/engine/shocks_test.go`:

```go
func TestIdioScaleScalesOnlyIdioShocks(t *testing.T) {
	sc := scenario.Synthetic()
	base := GenerateShockTimeline(sc, 7)

	scaled := scenario.Synthetic()
	for i := range scaled.Instruments {
		scaled.Instruments[i].IdioScale = 2.0
	}
	got := GenerateShockTimeline(scaled, 7)

	if len(got) != len(base) {
		t.Fatalf("event count changed: %d vs %d", len(got), len(base))
	}
	var checkedIdio, checkedShared int
	for i := range base {
		for f, v := range base[i].TrueShock {
			gv := got[i].TrueShock[f]
			if strings.HasPrefix(f, "IDIO:") {
				checkedIdio++
				if math.Abs(gv-2*v) > 1e-12 {
					t.Fatalf("idio shock not doubled: %v vs %v", gv, v)
				}
			} else {
				checkedShared++
				if gv != v {
					t.Fatalf("market/sector shock changed: %v vs %v", gv, v)
				}
			}
		}
	}
	if checkedIdio == 0 || checkedShared == 0 {
		t.Fatal("test exercised nothing")
	}
}

func TestIdioScaleDefaultsToOne(t *testing.T) {
	inst := scenario.Instrument{}
	if inst.IdioScaleOrDefault() != 1 {
		t.Fatalf("zero value must default to 1, got %v", inst.IdioScaleOrDefault())
	}
	inst.IdioScale = 0.5
	if inst.IdioScaleOrDefault() != 0.5 {
		t.Fatalf("explicit scale ignored")
	}
}
```

(`strings` may need adding to the test file imports.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/engine/ -run TestIdioScale -v`
Expected: FAIL (field undefined).

- [ ] **Step 3: Implement**

`server/internal/scenario/types.go` — extend Instrument:

```go
type Instrument struct {
	ID, Alias, Desc string
	Beta            map[string]float64
	// IdioScale multiplies this instrument's idiosyncratic shock
	// magnitudes (per-stock calibration, spec §4.2). Zero means 1.0 so
	// pre-existing data keeps its behavior.
	IdioScale float64
	// Reconstructed marks series rebuilt from documented anchor points
	// rather than exchange data (dead dot-com companies).
	Reconstructed bool
}

// IdioScaleOrDefault treats the zero value as the neutral 1.0 multiplier.
func (i *Instrument) IdioScaleOrDefault() float64 {
	if i.IdioScale <= 0 {
		return 1
	}
	return i.IdioScale
}
```

`server/internal/scenario/synthetic.go` — in the instrument literal, add `IdioScale: 1`:

```go
		sc.Instruments = append(sc.Instruments, Instrument{
			ID: id, Alias: "Syn " + id, Desc: "synthetic", Beta: beta,
			IdioScale: 1,
		})
```

`server/internal/engine/shocks.go` — in `GenerateShockTimeline`, the idio emit loop passes the scale. Replace the instrument loop body:

```go
		for i := range sc.Instruments {
			inst := &sc.Instruments[i]
			if fid, ok := idioOf[inst.ID]; ok {
				if rng.Float64() < pIdioDaily {
					emit(d, map[string]float64{
						fid: signedShock(rng, sc, d) * inst.IdioScaleOrDefault(),
					})
				}
			}
		}
```

(Keep RNG call order identical to before — one `signedShock` per emitted idio event — so seeded streams are unchanged and the golden hash stays put.)

- [ ] **Step 4: Run the engine suite including golden**

Run: `cd server && go test ./internal/engine/ ./internal/scenario/ -count=1`
Expected: PASS, including `TestGoldenWorld` (hash `448577ba…` unchanged — ×1.0 is exact). If the golden fails, the RNG call order drifted: fix the code, never the hash, in this task.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/scenario/ internal/engine/
git commit -m "feat(engine): per-instrument IdioScale for real-data calibration"
```

---

### Task 2: Engine — news clusters (传闻 → 主事件 → 追踪)

**Files:**
- Modify: `server/internal/engine/shocks.go` (NewsEvent.ClusterID + Body fields, `ExpandClusters`)
- Modify: `server/internal/engine/world.go` (call ExpandClusters)
- Modify: `server/internal/engine/news.go` (cluster fallback copy)
- Modify: `server/internal/engine/testdata/world-42.sha256` (regenerate — documented)
- Test: `server/internal/engine/shocks_test.go` (append), `server/internal/engine/news_test.go` (append)

**Interfaces:**
- Consumes: Task 1.
- Produces:
  - `NewsEvent` gains `Body string` (empty until LLM fills it; template copy stays headline-only) and `ClusterID int` (0 = standalone)
  - `func ExpandClusters(sc *scenario.Scenario, seed uint64, evs []NewsEvent) []NewsEvent` — for each market/sector impact event whose single true shock magnitude ≥ `bigShock`, with probability `pCluster = 0.6`: prepend a rumor (day−1, media ∈ {tabloid, forum}, `TrueShock nil`, `ReportShock` = event's report × factor `0.5+rng.Float64()` — deliberate 幅度错配) and append a follow-up (day+1, media ∈ {wire, paper}, `TrueShock nil`, `ReportShock` = true shock × 0.9). All three share a sequential ClusterID starting at 1. Rumor skipped when day==0, follow-up skipped when day+1 ≥ sc.Days. Idio events never cluster (spec §4.4 clusters are 大事件).
  - Zero-impact companions (`TrueShock nil`) mean prices and fidelity are UNCHANGED — only the news list grows, which is why the golden hash must be regenerated.
  - `FillFallbackCopy`: rumor headlines prefixed `【传闻】`, follow-ups `【追踪】`, both using the impact phrasing off `ReportShock`.
  - `GenerateWorld` pipeline becomes: shocks → **ExpandClusters** → states (companions are no-ops there) → prices → fidelity → news assembly.

- [ ] **Step 1: Write the failing tests**

Append to `server/internal/engine/shocks_test.go`:

```go
func TestExpandClustersShape(t *testing.T) {
	sc := scenario.Synthetic()
	base := GenerateShockTimeline(sc, 42)
	out := ExpandClusters(sc, 42, base)

	if len(out) < len(base) {
		t.Fatalf("clusters removed events: %d < %d", len(out), len(base))
	}
	byCluster := map[int][]NewsEvent{}
	for _, ev := range out {
		if ev.ClusterID != 0 {
			byCluster[ev.ClusterID] = append(byCluster[ev.ClusterID], ev)
		}
	}
	if len(byCluster) == 0 {
		t.Fatal("no clusters formed over 300 days at pCluster=0.6 — implausible")
	}
	lowRho := map[string]bool{"tabloid": true, "forum": true}
	highRho := map[string]bool{"wire": true, "paper": true}
	for id, evs := range byCluster {
		var main, rumor, follow int
		var mainDay int
		for _, ev := range evs {
			if ev.TrueShock != nil {
				main++
				mainDay = ev.Day
				// only market/sector events cluster
				for f := range ev.TrueShock {
					if strings.HasPrefix(f, "IDIO:") {
						t.Fatalf("cluster %d built around idio event", id)
					}
					if math.Abs(ev.TrueShock[f]) < bigShock {
						t.Fatalf("cluster %d around small shock %v", id, ev.TrueShock[f])
					}
				}
			}
		}
		if main != 1 {
			t.Fatalf("cluster %d has %d main events", id, main)
		}
		for _, ev := range evs {
			if ev.TrueShock != nil {
				continue
			}
			if ev.ReportShock == nil {
				t.Fatalf("cluster %d companion missing report shock", id)
			}
			switch {
			case ev.Day == mainDay-1:
				rumor++
				if !lowRho[ev.MediaID] {
					t.Fatalf("rumor from %s", ev.MediaID)
				}
			case ev.Day == mainDay+1:
				follow++
				if !highRho[ev.MediaID] {
					t.Fatalf("follow-up from %s", ev.MediaID)
				}
			default:
				t.Fatalf("companion on unexpected day %d (main %d)", ev.Day, mainDay)
			}
		}
		if rumor > 1 || follow > 1 {
			t.Fatalf("cluster %d duplicated companions", id)
		}
	}
	// Determinism.
	again := ExpandClusters(sc, 42, GenerateShockTimeline(sc, 42))
	if len(again) != len(out) {
		t.Fatal("ExpandClusters not deterministic")
	}
}

func TestClusterCompanionsDoNotMovePrices(t *testing.T) {
	sc := scenario.Synthetic()
	shocks := GenerateShockTimeline(sc, 42)
	statesPlain, err := EvolveFactorStates(sc, shocks)
	if err != nil {
		t.Fatal(err)
	}
	statesClustered, err := EvolveFactorStates(sc, ExpandClusters(sc, 42, shocks))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(statesPlain, statesClustered) {
		t.Fatal("zero-impact companions changed factor states")
	}
}
```

Append to `server/internal/engine/news_test.go`:

```go
func TestClusterFallbackCopyPrefixes(t *testing.T) {
	sc := scenario.Synthetic()
	evs := ExpandClusters(sc, 42, GenerateShockTimeline(sc, 42))
	FillFallbackCopy(sc, evs, 42)
	var sawRumor, sawFollow bool
	for _, ev := range evs {
		if ev.ClusterID == 0 || ev.TrueShock != nil {
			continue
		}
		if strings.HasPrefix(ev.Headline, "【传闻】") {
			sawRumor = true
		}
		if strings.HasPrefix(ev.Headline, "【追踪】") {
			sawFollow = true
		}
	}
	if !sawRumor || !sawFollow {
		t.Fatalf("cluster copy prefixes missing: rumor=%v follow=%v", sawRumor, sawFollow)
	}
}
```

(Add `reflect`/`strings` imports where missing.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/engine/ -run 'TestExpandClusters|TestCluster' -v`
Expected: FAIL (undefined: ExpandClusters / ClusterID).

- [ ] **Step 3: Implement**

`server/internal/engine/shocks.go` — extend the type and add the pass:

```go
type NewsEvent struct {
	Day         int
	Track       Track
	MediaID     string
	TrueShock   map[string]float64
	ReportShock map[string]float64
	Headline    string
	Body        string // LLM copy (plan 4); empty on template fallback
	ClusterID   int    // 0 = standalone; shared by 传闻/主事件/追踪 triplets
}

const pCluster = 0.6

// ExpandClusters turns big market/sector impact events into 传闻→主事件→追踪
// narratives (spec §4.4). Companions carry only ReportShock (TrueShock nil),
// so factor states and prices are untouched — clusters are pure narrative.
func ExpandClusters(sc *scenario.Scenario, seed uint64, evs []NewsEvent) []NewsEvent {
	rng := Stream(seed, "clusters")
	lowRho := []string{"tabloid", "forum"}
	highRho := []string{"wire", "paper"}
	out := make([]NewsEvent, 0, len(evs)+len(evs)/2)
	nextCluster := 1
	for _, ev := range evs {
		big := false
		if ev.Track == TrackImpact && len(ev.TrueShock) == 1 {
			for f, v := range ev.TrueShock {
				if !strings.HasPrefix(f, "IDIO:") && math.Abs(v) >= bigShock {
					big = true
				}
			}
		}
		if !big || rng.Float64() >= pCluster {
			out = append(out, ev)
			continue
		}
		id := nextCluster
		nextCluster++
		ev.ClusterID = id
		if ev.Day > 0 {
			rumorReport := make(map[string]float64, 1)
			mismatch := 0.5 + rng.Float64() // 幅度错配: ×[0.5, 1.5)
			for f, v := range ev.ReportShock {
				rumorReport[f] = v * mismatch
			}
			out = append(out, NewsEvent{
				Day: ev.Day - 1, Track: TrackImpact,
				MediaID: lowRho[rng.IntN(len(lowRho))],
				ReportShock: rumorReport, ClusterID: id,
			})
		}
		out = append(out, ev)
		if ev.Day+1 < sc.Days {
			followReport := make(map[string]float64, 1)
			for f, v := range ev.TrueShock {
				followReport[f] = v * 0.9
			}
			out = append(out, NewsEvent{
				Day: ev.Day + 1, Track: TrackImpact,
				MediaID: highRho[rng.IntN(len(highRho))],
				ReportShock: followReport, ClusterID: id,
			})
		}
	}
	return out
}
```

(`strings` joins the imports.)

`server/internal/engine/world.go` — thread the pass through:

```go
	shocks := ExpandClusters(sc, seed, GenerateShockTimeline(sc, seed))
```

(`EvolveFactorStates` already no-ops on nil TrueShock maps — its loop iterates `ev.TrueShock` entries.)

`server/internal/engine/news.go` — in `FillFallbackCopy`'s `TrackImpact` case, prefix cluster companions. Replace the impact case body:

```go
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
			headline := fmt.Sprintf("消息面变化，%s板块%s，市场解读不一", name[top], tone)
			if evs[i].ClusterID != 0 && evs[i].TrueShock == nil {
				prefix := "【追踪】"
				if isRumor(evs, i) {
					prefix = "【传闻】"
				}
				headline = prefix + headline
			}
			evs[i].Headline = headline
```

with helper (same file):

```go
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
```

- [ ] **Step 4: Regenerate the golden hash — deliberately**

The world's news list changed (companions added), so `TestGoldenWorld` fails by design. Regenerate:

```bash
cd server && go test ./internal/engine/ -run TestGoldenWorld -v 2>&1 | head -20
```

Read the failing test's output for the new hash, update `internal/engine/testdata/world-42.sha256` with it (the golden test is self-recording — check `world_test.go` for the exact regeneration mechanism: if it writes the file when absent, delete the file and re-run instead). Then re-run the FULL suite:

```bash
STOCKER_TEST_DB=postgres://localhost:5432/stocker_test?sslmode=disable go test ./... -count=1
```

Expected: all green, including store/httpapi (news rows gain nothing yet — ClusterID/Body aren't persisted until Task 7/9).

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/engine/
git commit -m "feat(engine): news clusters (rumor/main/follow-up) — regenerates golden hash: cluster companions extend the news list without touching prices"
```

---
### Task 3: Pipeline scaffold — CSV parsing, Stooq fetch, committed raw data

**Files:**
- Create: `server/internal/pipeline/csv.go`, `server/internal/pipeline/universe.go` (ticker list portion), `server/cmd/pipeline/main.go` (fetch subcommand)
- Create (by running fetch once): `server/internal/pipeline/rawdata/*.csv`
- Test: `server/internal/pipeline/csv_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `type Bar struct { Date time.Time; Open, High, Low, Close float64 }`
  - `func ParseStooqCSV(r io.Reader) ([]Bar, error)` — Stooq format `Date,Open,High,Low,Close,Volume`, ascending dates enforced, zero/negative prices rejected, volume ignored
  - `//go:embed rawdata/*.csv` + `func RawSeries(name string) ([]Bar, error)` — reads an embedded file by short name (e.g. `"msft"`)
  - `var FetchList = []FetchSpec{...}` in universe.go: short name → Stooq symbol for the 16 real stocks, 2 indices, 3 macro proxies
  - `cmd/pipeline fetch` — downloads each FetchSpec from `https://stooq.com/q/d/l/?s=<sym>&i=d` into `internal/pipeline/rawdata/<name>.csv` (dev-time only; polite 500 ms delay between requests; reports failures without aborting the rest)

- [ ] **Step 1: Write the failing test**

`server/internal/pipeline/csv_test.go`:

```go
package pipeline

import (
	"strings"
	"testing"
	"time"
)

const sampleCSV = `Date,Open,High,Low,Close,Volume
1999-01-04,34.5,35.1,34.0,35.0,1000
1999-01-05,35.0,36.0,34.8,35.9,1200
1999-01-06,35.9,36.2,35.5,36.0,900
`

func TestParseStooqCSV(t *testing.T) {
	bars, err := ParseStooqCSV(strings.NewReader(sampleCSV))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bars) != 3 {
		t.Fatalf("bars: %d", len(bars))
	}
	if !bars[0].Date.Equal(time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date: %v", bars[0].Date)
	}
	if bars[1].Close != 35.9 || bars[2].High != 36.2 {
		t.Fatalf("values: %+v", bars)
	}
}

func TestParseStooqCSVRejectsBadData(t *testing.T) {
	cases := []string{
		"Date,Open,High,Low,Close,Volume\n1999-01-05,1,1,1,1,0\n1999-01-04,1,1,1,1,0\n", // descending
		"Date,Open,High,Low,Close,Volume\n1999-01-04,1,1,1,0,0\n",                        // zero close
		"Date,Open,High,Low,Close,Volume\nnot-a-date,1,1,1,1,0\n",
	}
	for i, c := range cases {
		if _, err := ParseStooqCSV(strings.NewReader(c)); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestEmbeddedRawSeriesLoad(t *testing.T) {
	// Every fetch-list entry must have committed data that parses and
	// covers the scenario window with real history on both ends.
	for _, spec := range FetchList {
		bars, err := RawSeries(spec.Name)
		if err != nil {
			t.Fatalf("%s: %v", spec.Name, err)
		}
		if len(bars) < 500 {
			t.Errorf("%s: only %d bars", spec.Name, len(bars))
		}
		if bars[0].Date.After(time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("%s: starts too late (%v)", spec.Name, bars[0].Date)
		}
		if bars[len(bars)-1].Date.Before(time.Date(2001, 12, 28, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("%s: ends too early (%v)", spec.Name, bars[len(bars)-1].Date)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/pipeline/ -v`
Expected: FAIL (package missing).

- [ ] **Step 3: Implement**

`server/internal/pipeline/csv.go`:

```go
// Package pipeline builds the real 2000-era scenario from committed raw
// market data. Everything here is deterministic and offline: the raw CSVs
// are embedded at compile time; only `cmd/pipeline fetch` (dev-time)
// touches the network.
package pipeline

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type Bar struct {
	Date                   time.Time
	Open, High, Low, Close float64
}

//go:embed rawdata/*.csv
var rawFS embed.FS

// RawSeries loads a committed Stooq download by short name.
func RawSeries(name string) ([]Bar, error) {
	f, err := rawFS.Open("rawdata/" + name + ".csv")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseStooqCSV(f)
}

// ParseStooqCSV parses Stooq's daily CSV (Date,Open,High,Low,Close,Volume).
// Dates must be strictly ascending; prices must be positive.
func ParseStooqCSV(r io.Reader) ([]Bar, error) {
	sc := bufio.NewScanner(r)
	var bars []Bar
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if strings.HasPrefix(strings.ToLower(line), "date,") {
				continue
			}
		}
		parts := strings.Split(line, ",")
		if len(parts) < 5 {
			return nil, fmt.Errorf("bad row %q", line)
		}
		date, err := time.Parse("2006-01-02", parts[0])
		if err != nil {
			return nil, fmt.Errorf("bad date %q: %w", parts[0], err)
		}
		var b Bar
		b.Date = date
		for i, dst := range []*float64{&b.Open, &b.High, &b.Low, &b.Close} {
			v, err := strconv.ParseFloat(parts[i+1], 64)
			if err != nil {
				return nil, fmt.Errorf("bad number in %q: %w", line, err)
			}
			*dst = v
		}
		if b.Close <= 0 || b.Open <= 0 || b.High <= 0 || b.Low <= 0 {
			return nil, fmt.Errorf("non-positive price on %s", parts[0])
		}
		if len(bars) > 0 && !bars[len(bars)-1].Date.Before(b.Date) {
			return nil, fmt.Errorf("dates not ascending at %s", parts[0])
		}
		bars = append(bars, b)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return bars, nil
}
```

`server/internal/pipeline/universe.go` (this task: the fetch list only; Task 5 appends the full instrument/factor/dossier data):

```go
package pipeline

// FetchSpec maps a short local name to a Stooq symbol.
type FetchSpec struct {
	Name   string // rawdata/<Name>.csv
	Symbol string // stooq query symbol
}

// FetchList: 16 surviving stocks, 2 indices, 3 macro proxies.
// The four dead companies (wcom/lu/nt/gblx) are anchor-reconstructed in
// reconstruct.go — free sources carry no delisted daily data.
var FetchList = []FetchSpec{
	{"msft", "msft.us"}, {"csco", "csco.us"}, {"intc", "intc.us"},
	{"orcl", "orcl.us"}, {"ibm", "ibm.us"}, {"aapl", "aapl.us"},
	{"amzn", "amzn.us"}, {"ebay", "ebay.us"}, {"amd", "amd.us"},
	{"qcom", "qcom.us"}, {"txn", "txn.us"}, {"adbe", "adbe.us"},
	{"hpq", "hpq.us"}, {"ge", "ge.us"}, {"xom", "xom.us"}, {"wmt", "wmt.us"},
	{"ndx", "^ndx"}, {"spx", "^spx"},
	{"gold", "xauusd"}, {"oil", "cl.f"}, {"us10y", "10usy.b"},
}
```

`server/cmd/pipeline/main.go`:

```go
// Command pipeline builds and imports the real 2000 dot-com scenario.
//
//	go run ./cmd/pipeline fetch    # dev-time: download raw CSVs from stooq
//	go run ./cmd/pipeline import   # build scenario from embedded data → Postgres (Task 7)
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/toddzheng/stocker/server/internal/pipeline"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: pipeline <fetch|import>")
	}
	switch os.Args[1] {
	case "fetch":
		fetch()
	default:
		log.Fatalf("unknown subcommand %q", os.Args[1])
	}
}

func fetch() {
	client := &http.Client{Timeout: 30 * time.Second}
	failed := 0
	for _, spec := range pipeline.FetchList {
		url := fmt.Sprintf("https://stooq.com/q/d/l/?s=%s&i=d", spec.Symbol)
		resp, err := client.Get(url)
		if err != nil {
			log.Printf("FAIL %s (%s): %v", spec.Name, spec.Symbol, err)
			failed++
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 || len(body) < 100 {
			log.Printf("FAIL %s (%s): status=%d bytes=%d err=%v",
				spec.Name, spec.Symbol, resp.StatusCode, len(body), err)
			failed++
			continue
		}
		path := "internal/pipeline/rawdata/" + spec.Name + ".csv"
		if err := os.WriteFile(path, body, 0o644); err != nil {
			log.Fatalf("write %s: %v", path, err)
		}
		log.Printf("ok   %s ← %s (%d bytes)", path, spec.Symbol, len(body))
		time.Sleep(500 * time.Millisecond) // be polite to stooq
	}
	if failed > 0 {
		log.Printf("%d symbols failed — investigate before building", failed)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Fetch the data once and commit it**

```bash
mkdir -p /Users/toddzheng/Workspace/react/stocker/server/internal/pipeline/rawdata
cd /Users/toddzheng/Workspace/react/stocker/server && go run ./cmd/pipeline fetch
```

Then trim each file to the needed era to keep the repo lean (full Stooq history can be decades; we only need 1998-06-01 onward — keep a pre-window margin for β estimation):

```bash
cd internal/pipeline/rawdata && for f in *.csv; do
  { head -1 "$f"; awk -F, 'NR>1 && $1 >= "1998-06-01" && $1 <= "2002-03-31"' "$f"; } > "$f.tmp" && mv "$f.tmp" "$f"
done && du -sh . && cd /Users/toddzheng/Workspace/react/stocker/server
```

**If any symbol fails to download** (Stooq coverage varies, especially `cl.f`/`10usy.b`): report DONE_WITH_CONCERNS naming the failures. Contingency, pre-authorized: a failed MACRO proxy (gold/oil/us10y) may be dropped from FetchList — the corresponding factor keeps β=0 curated values in Task 5. A failed STOCK or INDEX is a blocker: stop and escalate rather than substituting.

Also verify: Stooq prices are split-adjusted (their standard for `.us` symbols). Sanity-check one series: `msft.csv` around 1999 should show ~$35–45 split-adjusted-to-2003 style values, NOT $90–120 unadjusted. Record the observed ranges for msft/intc/csco in the report; if values look unadjusted, flag DONE_WITH_CONCERNS (β regression tolerates it; the fidelity test in Task 5 is the real gate).

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd server && go test ./internal/pipeline/ -count=1`
Expected: PASS including `TestEmbeddedRawSeriesLoad` over the real committed files.

- [ ] **Step 6: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/pipeline/ cmd/pipeline/
git commit -m "feat(pipeline): stooq fetch tooling and committed 1998-2002 raw market data"
```

---

### Task 4: Pipeline — anchor reconstruction for dead companies

**Files:**
- Create: `server/internal/pipeline/reconstruct.go`
- Test: `server/internal/pipeline/reconstruct_test.go`

**Interfaces:**
- Consumes: `Bar` (Task 3).
- Produces:
  - `type Anchor struct { Date string; Price float64 }` (`"2006-01-02"` format)
  - `func Reconstruct(anchors []Anchor, calendar []time.Time, seed uint64) ([]Bar, error)` — for each calendar day: geometric (log-space) interpolation between surrounding anchors + a Brownian-bridge noise term that is exactly zero AT anchor days (anchors are hit exactly), with daily log-noise σ = 0.025. OHLC synthesized from consecutive closes: `Open = prevClose`, `High = max(Open, Close) × (1+|n|)`, `Low = min(Open, Close) × (1−|n|)` with `n ~ N(0, 0.008)`. Deterministic per (anchors, calendar, seed) via `rand.NewPCG(seed, 0x9E3779B97F4A7C15)`.
  - Errors when: fewer than 2 anchors, anchors outside the calendar range, non-positive prices, unordered anchors.
  - **Calibration lesson from plan 1 (apply, do not rediscover):** interpolated series MUST carry realistic volatility (σ=0.025 ≈ real 2000-era single-stock vol) or the fidelity gate's correlation floor becomes mathematically unreachable (corr ceiling = σ_b/√(σ_b²+σ_p²)); and each anchor that is a global extreme must beat its neighbors by more than the ±10-day perturbation swing — the anchor tables in Task 5 are chosen with that margin.

- [ ] **Step 1: Write the failing test**

`server/internal/pipeline/reconstruct_test.go`:

```go
package pipeline

import (
	"math"
	"testing"
	"time"
)

func tradingCalendar(start time.Time, days int) []time.Time {
	cal := make([]time.Time, 0, days)
	d := start
	for len(cal) < days {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			cal = append(cal, d)
		}
		d = d.AddDate(0, 0, 1)
	}
	return cal
}

func TestReconstructHitsAnchorsExactly(t *testing.T) {
	cal := tradingCalendar(time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC), 400)
	anchors := []Anchor{
		{cal[0].Format("2006-01-02"), 60},
		{cal[150].Format("2006-01-02"), 90},
		{cal[399].Format("2006-01-02"), 12},
	}
	bars, err := Reconstruct(anchors, cal, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 400 {
		t.Fatalf("bars: %d", len(bars))
	}
	for _, tc := range []struct {
		idx  int
		want float64
	}{{0, 60}, {150, 90}, {399, 12}} {
		if math.Abs(bars[tc.idx].Close-tc.want) > 1e-9 {
			t.Fatalf("anchor at %d: close %v want %v", tc.idx, bars[tc.idx].Close, tc.want)
		}
	}
}

func TestReconstructHasRealisticVolAndValidOHLC(t *testing.T) {
	cal := tradingCalendar(time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC), 400)
	anchors := []Anchor{
		{cal[0].Format("2006-01-02"), 60},
		{cal[399].Format("2006-01-02"), 20},
	}
	bars, err := Reconstruct(anchors, cal, 7)
	if err != nil {
		t.Fatal(err)
	}
	var sum, sum2 float64
	for i := 1; i < len(bars); i++ {
		r := math.Log(bars[i].Close / bars[i-1].Close)
		sum += r
		sum2 += r * r
	}
	n := float64(len(bars) - 1)
	vol := math.Sqrt(sum2/n - (sum/n)*(sum/n))
	if vol < 0.015 || vol > 0.045 {
		t.Fatalf("daily vol %.4f outside [0.015, 0.045]", vol)
	}
	for i, b := range bars {
		if b.Low <= 0 || b.High < b.Low || b.Close > b.High || b.Close < b.Low ||
			b.Open > b.High || b.Open < b.Low {
			t.Fatalf("invalid OHLC at %d: %+v", i, b)
		}
	}
	// Determinism.
	again, _ := Reconstruct(anchors, cal, 7)
	if again[123].Close != bars[123].Close {
		t.Fatal("not deterministic")
	}
	// Different seed differs.
	other, _ := Reconstruct(anchors, cal, 8)
	if other[123].Close == bars[123].Close {
		t.Fatal("seed has no effect")
	}
}

func TestReconstructValidation(t *testing.T) {
	cal := tradingCalendar(time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC), 10)
	bad := [][]Anchor{
		{{cal[0].Format("2006-01-02"), 60}},                                                  // one anchor
		{{cal[3].Format("2006-01-02"), 60}, {cal[1].Format("2006-01-02"), 50}},               // unordered
		{{cal[0].Format("2006-01-02"), -1}, {cal[9].Format("2006-01-02"), 50}},               // bad price
		{{"1990-01-01", 60}, {cal[9].Format("2006-01-02"), 50}},                              // outside calendar
	}
	for i, a := range bad {
		if _, err := Reconstruct(a, cal, 1); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/pipeline/ -run TestReconstruct -v`
Expected: FAIL (undefined: Reconstruct).

- [ ] **Step 3: Implement**

`server/internal/pipeline/reconstruct.go`:

```go
package pipeline

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// Anchor pins a reconstructed series to a documented price on a date.
type Anchor struct {
	Date  string // "2006-01-02"
	Price float64
}

const (
	reconNoiseSigma = 0.025 // daily log noise — must be representative of
	// 2000-era single-stock vol, or the fidelity gate's correlation floor
	// becomes unreachable (plan-1 lesson: corr ceiling = σb/√(σb²+σp²)).
	reconWickSigma = 0.008
)

// Reconstruct builds a daily OHLC series through the anchors: log-space
// linear interpolation plus Brownian-bridge noise that vanishes exactly at
// anchor days, so every anchor price is hit to the digit. Deterministic in
// (anchors, calendar, seed).
func Reconstruct(anchors []Anchor, calendar []time.Time, seed uint64) ([]Bar, error) {
	if len(anchors) < 2 {
		return nil, fmt.Errorf("need at least 2 anchors, got %d", len(anchors))
	}
	idxOf := make(map[string]int, len(calendar))
	for i, d := range calendar {
		idxOf[d.Format("2006-01-02")] = i
	}
	type pin struct {
		idx  int
		logP float64
	}
	pins := make([]pin, 0, len(anchors))
	for _, a := range anchors {
		if a.Price <= 0 {
			return nil, fmt.Errorf("anchor %s has non-positive price %v", a.Date, a.Price)
		}
		idx, ok := idxOf[a.Date]
		if !ok {
			return nil, fmt.Errorf("anchor %s not a calendar trading day", a.Date)
		}
		if len(pins) > 0 && idx <= pins[len(pins)-1].idx {
			return nil, fmt.Errorf("anchors not strictly ascending at %s", a.Date)
		}
		pins = append(pins, pin{idx, math.Log(a.Price)})
	}
	if pins[0].idx != 0 || pins[len(pins)-1].idx != len(calendar)-1 {
		return nil, fmt.Errorf("anchors must cover the full calendar (first day and last day)")
	}

	rng := rand.New(rand.NewPCG(seed, 0x9E3779B97F4A7C15))
	logC := make([]float64, len(calendar))
	for seg := 0; seg+1 < len(pins); seg++ {
		a, b := pins[seg], pins[seg+1]
		n := b.idx - a.idx
		// Random walk over the segment, then subtract the linear drift of
		// its own endpoint so the bridge is exactly zero at both anchors.
		walk := make([]float64, n+1)
		for i := 1; i <= n; i++ {
			walk[i] = walk[i-1] + rng.NormFloat64()*reconNoiseSigma
		}
		for i := 0; i <= n; i++ {
			frac := float64(i) / float64(n)
			trend := a.logP + (b.logP-a.logP)*frac
			bridge := walk[i] - walk[n]*frac
			logC[a.idx+i] = trend + bridge
		}
	}

	bars := make([]Bar, len(calendar))
	for i := range calendar {
		c := math.Exp(logC[i])
		o := c
		if i > 0 {
			o = bars[i-1].Close
		}
		wick := math.Abs(rng.NormFloat64() * reconWickSigma)
		hi := math.Max(o, c) * (1 + wick)
		lo := math.Min(o, c) * (1 - wick)
		bars[i] = Bar{Date: calendar[i], Open: o, High: hi, Low: lo, Close: c}
	}
	return bars, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/pipeline/ -count=1`
Expected: PASS.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/pipeline/reconstruct.go internal/pipeline/reconstruct_test.go
git commit -m "feat(pipeline): anchor-based reconstruction for delisted dot-com stocks"
```

---
### Task 5: Pipeline — build the dotcom-2000 scenario (universe, β regression, calibration, fidelity)

**Files:**
- Modify: `server/internal/pipeline/universe.go` (append the full universe data)
- Create: `server/internal/pipeline/build.go`
- Modify: `server/internal/engine/fidelity.go` (near-flat direction exemption — see below)
- Test: `server/internal/pipeline/build_test.go`, `server/internal/pipeline/fidelity_test.go`, `server/internal/engine/fidelity_test.go` (append one test)

**Interfaces:**
- Consumes: Tasks 1, 3, 4; `scenario.Scenario`, `engine.GenerateWorld`, `engine.VerifyFidelity`.
- Produces:
  - `type Dossier struct { Alias, Desc, RealName, Business, Bull, Bear string }`
  - `type InstrumentSpec struct { ID, Raw, Sector string; Anchors []Anchor; Dossier Dossier; ExtraBeta map[string]float64 }` (`Raw == ""` → reconstructed from Anchors)
  - `var Universe = struct { ScenarioID, Name, RealPeriod, WindowStart, WindowEnd string; Sectors []SectorSpec; Instruments []InstrumentSpec; KeyWindows []DateWindow }{...}` — the complete 2000-scenario data (22 instruments, 7 sectors, 3 macro factors, dossiers, anchors, key windows)
  - `func BuildScenario() (*scenario.Scenario, error)` — deterministic, offline (embedded raw data): calendar from SPX bars in window → align/forward-fill real series (error if >15% missing or first day missing) → reconstruct martyrs → normalize to 100 → sector proxies (equal-weight member log-returns) → β_MKT via OLS vs SPX, β_sector via OLS of residuals vs sector-proxy residuals (clamped [−0.5, 3]) → curated SENT/macro βs from ExtraBeta → IdioScale from `target = 0.40·σ_i` (clamped [0.4, 3]) → key windows to day indexes → assembled `*scenario.Scenario` (ID `dotcom-2000`)
  - `func BuildMeta() Meta` with `Meta{Name, RealPeriod string; Dossiers map[string]Dossier}` for Task 7's import
  - Engine amendment: `VerifyFidelity` skips the cumulative-direction check when the baseline's |net log return| < `minNetLogForDirection = 0.10` — a near-flat series has no 大势 to preserve, and sign-flipping a ±2% net move is noise, not infidelity (spec §4.6's intent). Correlation and extremum checks still apply to such instruments. Synthetic nets are all ≥ 0.5 log, so the golden is unaffected.

- [ ] **Step 1: Append the universe data to `universe.go`**

```go
type Dossier struct {
	Alias, Desc, RealName, Business, Bull, Bear string
}

type SectorSpec struct {
	ID, Name string
}

type InstrumentSpec struct {
	ID        string   // blind-box id (X01..X22); IDIO factor = "IDIO:"+ID
	Raw       string   // rawdata short name; "" => reconstructed
	Sector    string   // sector factor id; "" for indices
	Anchors   []Anchor // reconstructed instruments only
	Dossier   Dossier
	ExtraBeta map[string]float64 // curated SENT/macro betas
}

type DateWindow struct {
	Start, End string // "2006-01-02"
	Direction  int
}

var Universe = struct {
	ScenarioID, Name, RealPeriod string
	WindowStart, WindowEnd       string
	Sectors                      []SectorSpec
	Macros                       []SectorSpec // reuse shape: id+中文名
	Instruments                  []InstrumentSpec
	KeyWindows                   []DateWindow
}{
	ScenarioID: "dotcom-2000",
	Name:       "2000 互联网泡沫",
	RealPeriod: "1999-01 ~ 2001-12",
	WindowStart: "1999-01-04",
	WindowEnd:   "2001-12-31",
	Sectors: []SectorSpec{
		{"NET", "网络设备"}, {"TELCO", "电信运营"}, {"ECOM", "电商门户"},
		{"CHIP", "半导体"}, {"SOFT", "软件服务"}, {"HW", "硬件整机"}, {"OLD", "传统经济"},
	},
	Macros: []SectorSpec{{"GOLD", "黄金"}, {"OIL", "原油"}, {"RATE", "利率"}},
	KeyWindows: []DateWindow{
		{"2000-03-10", "2000-05-26", -1}, // 见顶崩盘期: 火上浇油可以，泼冷水不行
		{"2000-11-08", "2001-01-05", -1}, // 二次下跌
		{"2001-09-17", "2001-09-28", -1}, // 重开市恐慌
	},
	Instruments: []InstrumentSpec{
		{ID: "X01", Raw: "msft", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.3},
			Dossier: Dossier{Alias: "巨硬软件", Desc: "桌面软件的垄断者", RealName: "Microsoft",
				Business: "操作系统与办公套件的授权费像税收一样稳定，正把触角伸向服务器与互联网。",
				Bull:     "每台新电脑都要向它交税，现金堆成山，垄断者的定价权在任何时代都值钱。",
				Bear:     "反垄断阴云笼罩，拆分传闻不断；互联网时代它更像追赶者而非引领者。"}},
		{ID: "X02", Raw: "csco", Sector: "NET",
			ExtraBeta: map[string]float64{"SENT": 0.8, "RATE": -0.4},
			Dossier: Dossier{Alias: "郊狼网络", Desc: "网络设备巨头，泡沫叙事的旗手", RealName: "Cisco Systems",
				Business: "路由器与交换机的绝对霸主，客户是整个新经济：运营商、门户、企业机房。",
				Bull:     "淘金潮里卖铲子的人——不管哪家网站赢，铲子都得从它这买。",
				Bear:     "客户都在烧风投的钱；融资一断，下游资本开支先于一切崩塌，而估值已透支十年。"}},
		{ID: "X03", Raw: "intc", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.3},
			Dossier: Dossier{Alias: "芯际半导", Desc: "处理器行业的王者", RealName: "Intel",
				Business: "个人电脑与服务器处理器的双料霸主，制程领先一代就是护城河。",
				Bull:     "上网热潮就是换机热潮，每一台新电脑的心脏都印着它的标。",
				Bear:     "半导体是周期行业，库存一旦掉头，'供不应求'三个月内变'砍单'。"}},
		{ID: "X04", Raw: "orcl", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.3},
			Dossier: Dossier{Alias: "神谕数据", Desc: "企业数据库军火商", RealName: "Oracle",
				Business: "关系数据库的行业标准，电商潮里每个网站背后都要一套它的授权。",
				Bull:     "'触网'是所有 CEO 的年度关键词，而所有网站的地基都是数据库。",
				Bear:     "授权收入一次性确认，增长靠不断找新客户；该上网的都上完了怎么办。"}},
		{ID: "X05", Raw: "ibm", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.2, "RATE": -0.2},
			Dossier: Dossier{Alias: "蓝色巨人", Desc: "百年计算公司", RealName: "IBM",
				Business: "大型机、服务与咨询三驾马车，新经济里做旧经济的生意。",
				Bull:     "企业级信任无可替代，泡沫破了大家还得找它做系统集成。",
				Bear:     "增长平庸，故事乏味，在狂热年代里资金看不上稳健。"}},
		{ID: "X06", Raw: "aapl", Sector: "HW",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.2},
			Dossier: Dossier{Alias: "果核电脑", Desc: "特立独行的电脑厂", RealName: "Apple",
				Business: "设计驱动的个人电脑，创始人回归后靠一体机打了场翻身仗。",
				Bull:     "品牌信徒式的忠诚度，硬件卖出了奢侈品毛利。",
				Bear:     "市场份额个位数，一款产品失手就是一个财年的灾难。"}},
		{ID: "X07", Raw: "amzn", Sector: "ECOM",
			ExtraBeta: map[string]float64{"SENT": 1.0, "RATE": -0.5},
			Dossier: Dossier{Alias: "雨林书店", Desc: "烧钱换增长的万货商店", RealName: "Amazon",
				Business: "从图书扩到全品类的线上零售，自建仓储物流，每单都亏，每季单量都新高。",
				Bull:     "零售的未来在线上，先烧钱圈地者赢者通吃；今天的亏损是明天垄断的门票。",
				Bear:     "现金消耗率惊人，命脉握在资本市场手里；融资窗口一关，增长故事一个季度变清算故事。"}},
		{ID: "X08", Raw: "ebay", Sector: "ECOM",
			ExtraBeta: map[string]float64{"SENT": 0.9, "RATE": -0.4},
			Dossier: Dossier{Alias: "万人集市", Desc: "全民线上拍卖行", RealName: "eBay",
				Business: "买卖双方自己定价的拍卖平台，轻资产抽佣，罕见地在互联网公司里赚钱。",
				Bull:     "网络效应教科书：买家越多卖家越多，飞轮一旦转起来没人追得上。",
				Bear:     "假货与欺诈是平台的原罪，信任一旦崩塌飞轮就倒转。"}},
		{ID: "X09", Raw: "amd", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.3},
			Dossier: Dossier{Alias: "二号芯厂", Desc: "万年老二的芯片挑战者", RealName: "AMD",
				Business: "处理器市场的挑战者，靠性价比和新架构从霸主嘴里抢份额。",
				Bull:     "老二逆袭的故事最性感，新品每赢一次评测股价就跳一级。",
				Bear:     "价格战里毛利被霸主按在地上摩擦，一代产品失手就要卖厂求生。"}},
		{ID: "X10", Raw: "qcom", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.9, "RATE": -0.4},
			Dossier: Dossier{Alias: "通联芯片", Desc: "无线时代的专利收租人", RealName: "Qualcomm",
				Business: "手机通信标准的核心专利持有者，卖芯片更收授权费，躺着分成。",
				Bull:     "无线互联网是下一波浪潮，每卖一部手机它都抽成。",
				Bear:     "标准之争悬而未决，押错路线专利池就成废纸；估值已按赢者通吃计价。"}},
		{ID: "X11", Raw: "txn", Sector: "CHIP",
			ExtraBeta: map[string]float64{"SENT": 0.5, "RATE": -0.3},
			Dossier: Dossier{Alias: "仪芯半导", Desc: "模拟芯片的隐形冠军", RealName: "Texas Instruments",
				Business: "从计算器到手机基带的模拟与数字信号芯片，客户遍布所有电子产品。",
				Bear:     "下游消费电子景气一凉，订单立刻跟着凉。",
				Bull:     "不押注单一终端，什么电子设备火它都有份。"}},
		{ID: "X12", Raw: "adbe", Sector: "SOFT",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.3},
			Dossier: Dossier{Alias: "创意软件", Desc: "设计师的标配工具箱", RealName: "Adobe",
				Business: "图像处理与排版软件的事实标准，网页时代设计需求爆发的直接受益者。",
				Bull:     "每个新网站都需要设计师，每个设计师都得买它的全家桶。",
				Bear:     "工具软件盗版猖獗，增长天花板取决于正版化速度。"}},
		{ID: "X13", Raw: "hpq", Sector: "HW",
			ExtraBeta: map[string]float64{"SENT": 0.3, "RATE": -0.2},
			Dossier: Dossier{Alias: "车库仪器", Desc: "硅谷车库神话的老牌厂", RealName: "Hewlett-Packard",
				Business: "打印机、个人电脑与服务器的全线硬件厂，打印耗材是隐藏的现金牛。",
				Bull:     "墨盒是比咖啡更暴利的消耗品，装机量就是年金。",
				Bear:     "电脑业务毛利薄如纸，大公司病拖慢每一次转身。"}},
		{ID: "X14", Raw: "ge", Sector: "OLD",
			ExtraBeta: map[string]float64{"SENT": 0.1, "RATE": -0.4, "OIL": 0.1},
			Dossier: Dossier{Alias: "万象电气", Desc: "什么都造的工业帝国", RealName: "General Electric",
				Business: "发电机、飞机引擎、医疗设备加一个庞大的金融部门，业务横跨所有周期。",
				Bull:     "传奇 CEO 治下二十年利润从不失手，机构的压舱石首选。",
				Bear:     "金融部门的杠杆是报表深处的暗礁，'从不失手'本身就值得怀疑。"}},
		{ID: "X15", Raw: "xom", Sector: "OLD",
			ExtraBeta: map[string]float64{"SENT": -0.1, "OIL": 0.8, "GOLD": 0.1},
			Dossier: Dossier{Alias: "磐石石油", Desc: "全球油气巨轮", RealName: "ExxonMobil",
				Business: "从油井到加油站的全产业链油气巨头，世纪合并后规模冠绝全球。",
				Bull:     "无论线上线下，人总要开车取暖；恐慌时现金流是最硬的叙事。",
				Bear:     "油价下行周期里巨轮也得随波逐流，狂热年代长期跑输大盘。"}},
		{ID: "X16", Raw: "wmt", Sector: "OLD",
			ExtraBeta: map[string]float64{"SENT": 0.0, "RATE": -0.2},
			Dossier: Dossier{Alias: "平价百货", Desc: "乡镇起家的零售之王", RealName: "Walmart",
				Business: "天天低价的连锁超市帝国，供应链效率碾压一切同行。",
				Bull:     "九成五的零售仍在线下，它便宜、赚钱、还在扩张。",
				Bear:     "它是电商故事里被指名道姓的'被颠覆者'。"}},
		{ID: "X17", Raw: "", Sector: "TELCO",
			ExtraBeta: map[string]float64{"SENT": 0.4, "RATE": -0.5},
			Anchors: []Anchor{
				{"1999-01-04", 60}, {"1999-06-21", 64}, {"2000-01-03", 44},
				{"2000-07-03", 40}, {"2000-11-01", 18}, {"2001-03-01", 16},
				{"2001-07-02", 15}, {"2001-10-01", 14}, {"2001-12-31", 11.5},
			},
			Dossier: Dossier{Alias: "环声通讯", Desc: "并购成瘾的长途电话帝国", RealName: "WorldCom（重建）",
				Business: "靠连环并购堆出来的长途与数据通信巨头，报表增速全行业最快。",
				Bull:     "互联网流量每百天翻一倍——它自己说的，卖的就是流量的管道。",
				Bear:     "并购停下来的那天，增长从哪来？报表越漂亮的公司越要多问一句。"}},
		{ID: "X18", Raw: "", Sector: "NET",
			ExtraBeta: map[string]float64{"SENT": 0.6, "RATE": -0.4},
			Anchors: []Anchor{
				{"1999-01-04", 55}, {"1999-11-01", 65}, {"1999-12-09", 80},
				{"2000-01-14", 72}, {"2000-07-03", 58}, {"2000-10-02", 30},
				{"2000-12-01", 17}, {"2001-06-01", 7}, {"2001-12-31", 6.3},
			},
			Dossier: Dossier{Alias: "贝铃设备", Desc: "百年实验室拆出的设备商", RealName: "Lucent（重建）",
				Business: "从传奇实验室分拆的电信设备商，光网络与交换机订单排到明年。",
				Bull:     "运营商军备竞赛的最大军火商，手握全行业最深的技术储备。",
				Bear:     "为了冲营收向客户放贷卖货——客户还不上钱时，营收和坏账一起爆。"}},
		{ID: "X19", Raw: "", Sector: "NET",
			ExtraBeta: map[string]float64{"SENT": 0.7, "RATE": -0.4},
			Anchors: []Anchor{
				{"1999-01-04", 12}, {"1999-12-01", 25}, {"2000-07-17", 87},
				{"2000-10-02", 60}, {"2001-02-01", 32}, {"2001-06-01", 11},
				{"2001-09-17", 5.5}, {"2001-12-31", 7.5},
			},
			Dossier: Dossier{Alias: "北极星网络", Desc: "北方来的光网络之王", RealName: "Nortel（重建）",
				Business: "光传输设备的领跑者，骨干网扩容潮里订单接到手软。",
				Bull:     "带宽需求没有尽头，光纤铺到哪它卖到哪。",
				Bear:     "客户全是举债铺网的运营商；产能按泡沫顶点规划，退潮时最先搁浅。"}},
		{ID: "X20", Raw: "", Sector: "TELCO",
			ExtraBeta: map[string]float64{"SENT": 0.8, "RATE": -0.6},
			Anchors: []Anchor{
				{"1999-01-04", 22}, {"1999-05-03", 60}, {"2000-01-03", 50},
				{"2000-09-01", 30}, {"2001-01-02", 14}, {"2001-06-01", 8},
				{"2001-10-01", 1.9}, {"2001-12-31", 0.85},
			},
			Dossier: Dossier{Alias: "环洋光缆", Desc: "海底光缆狂想家", RealName: "Global Crossing（重建）",
				Business: "举债在大洋底铺光缆的全球网络运营商，资产是几万公里的海底玻璃。",
				Bull:     "把五大洲连起来的人收全世界的过路费。",
				Bear:     "光缆铺得越多带宽越不值钱；债务是刚性的，带宽价格不是。"}},
		{ID: "X21", Raw: "ndx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.6},
			Dossier: Dossier{Alias: "新经济一篮子", Desc: "科技百强指数基金", RealName: "NASDAQ-100 指数",
				Business: "一键买入整个新经济：一篮子科技龙头的指数化组合。",
				Bull:     "选不出赢家就全买，时代的贝塔好过个股的阿尔法。",
				Bear:     "篮子里装的全是同一个故事，泡沫破时没有分散可言。"}},
		{ID: "X22", Raw: "spx", Sector: "",
			ExtraBeta: map[string]float64{"SENT": 0.3},
			Dossier: Dossier{Alias: "大盘五百", Desc: "全市场指数基金", RealName: "S&P 500 指数",
				Business: "五百家大公司的市值加权组合，美国经济本身。",
				Bull:     "不赌行业不赌个股，赌国运。",
				Bear:     "科技权重已被泡沫吹到历史高位，'分散'没有看上去那么分散。"}},
	},
}
```

(Note: X11 lists `Bear` before `Bull` — field names are explicit, order is irrelevant; keep as-is or reorder freely.)

- [ ] **Step 2: Write the failing tests**

`server/internal/pipeline/build_test.go`:

```go
package pipeline

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestBuildScenarioShape(t *testing.T) {
	sc, err := BuildScenario()
	if err != nil {
		t.Fatalf("BuildScenario: %v", err)
	}
	if sc.ID != "dotcom-2000" {
		t.Fatalf("id: %s", sc.ID)
	}
	if sc.Days < 730 || sc.Days > 770 {
		t.Fatalf("days: %d (window 1999-01-04..2001-12-31 ≈ 750 trading days)", sc.Days)
	}
	if len(sc.Instruments) != 22 {
		t.Fatalf("instruments: %d", len(sc.Instruments))
	}
	// factors: MKT + SENT + 7 sectors + 3 macros + 22 idio = 34
	if len(sc.Factors) != 34 {
		t.Fatalf("factors: %d", len(sc.Factors))
	}
	for _, inst := range sc.Instruments {
		base := sc.Baseline[inst.ID]
		if len(base) != sc.Days {
			t.Fatalf("%s: baseline length %d", inst.ID, len(base))
		}
		if math.Abs(base[0].Close-100) > 1e-9 {
			t.Fatalf("%s: not normalized, day0 close %v", inst.ID, base[0].Close)
		}
		if inst.Beta["IDIO:"+inst.ID] != 1 {
			t.Fatalf("%s: idio beta missing", inst.ID)
		}
		if inst.IdioScale < 0.4 || inst.IdioScale > 3.0 {
			t.Fatalf("%s: idio scale %v outside clamp", inst.ID, inst.IdioScale)
		}
		if b := inst.Beta["MKT"]; b < -0.5 || b > 3 {
			t.Fatalf("%s: market beta %v outside clamp", inst.ID, b)
		}
	}
	// Reconstructed flags.
	recon := 0
	for _, inst := range sc.Instruments {
		if inst.Reconstructed {
			recon++
		}
	}
	if recon != 4 {
		t.Fatalf("reconstructed count: %d", recon)
	}
	// The market proxy instrument tracks the market factor strongly.
	for i := range sc.Instruments {
		if sc.Instruments[i].ID == "X22" {
			if b := sc.Instruments[i].Beta["MKT"]; b < 0.7 || b > 1.3 {
				t.Fatalf("SPX-tracking instrument MKT beta %v", b)
			}
		}
	}
	// Key windows resolved to sane day indexes.
	if len(sc.KeyWindows) != 3 {
		t.Fatalf("key windows: %d", len(sc.KeyWindows))
	}
	for _, w := range sc.KeyWindows {
		if w.StartDay <= 0 || w.EndDay >= sc.Days || w.StartDay >= w.EndDay || w.Direction != -1 {
			t.Fatalf("bad key window: %+v", w)
		}
	}
	// Determinism.
	sc2, err := BuildScenario()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sc, sc2) {
		t.Fatal("BuildScenario not deterministic")
	}
	// Blind box: no real names anywhere in the scenario object.
	for _, inst := range sc.Instruments {
		if strings.Contains(inst.Alias, "Microsoft") || strings.Contains(inst.Desc, "Cisco") {
			t.Fatalf("real name leaked into scenario: %+v", inst)
		}
	}
}

func TestBuildMeta(t *testing.T) {
	meta := BuildMeta()
	if meta.Name != "2000 互联网泡沫" || meta.RealPeriod != "1999-01 ~ 2001-12" {
		t.Fatalf("meta: %+v", meta)
	}
	if len(meta.Dossiers) != 22 {
		t.Fatalf("dossiers: %d", len(meta.Dossiers))
	}
	d, ok := meta.Dossiers["X02"]
	if !ok || d.RealName == "" || d.Alias == "" || d.Bull == "" || d.Bear == "" || d.Business == "" {
		t.Fatalf("X02 dossier incomplete: %+v", d)
	}
}
```

`server/internal/pipeline/fidelity_test.go`:

```go
package pipeline

import (
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
)

// The plan-1 fidelity gate must hold for the real scenario across seeds —
// this is the release gate for the whole pipeline (spec §4.6).
func TestDotcomFidelityAcrossSeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-seed world generation is slow")
	}
	sc, err := BuildScenario()
	if err != nil {
		t.Fatal(err)
	}
	for seed := uint64(1); seed <= 12; seed++ {
		if _, err := engine.GenerateWorld(sc, seed); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}
```

Append to `server/internal/engine/fidelity_test.go`:

```go
func TestFidelityDirectionExemptForNearFlat(t *testing.T) {
	// A baseline with ~zero net move must not fail on direction sign alone.
	sc := &scenario.Scenario{
		ID: "flat", Days: 260,
		Factors:     []scenario.Factor{{ID: "MKT", Kind: scenario.KindMarket}},
		Instruments: []scenario.Instrument{{ID: "F1", Beta: map[string]float64{"MKT": 1}, IdioScale: 1}},
		Baseline:    map[string][]scenario.OHLC{},
	}
	base := make([]scenario.OHLC, sc.Days)
	prices := make([]scenario.OHLC, sc.Days)
	for d := 0; d < sc.Days; d++ {
		// gentle oscillation, net ≈ +1% (|net log| « 0.10)
		b := 100 + 3*float64(d%40)/40
		base[d] = scenario.OHLC{Open: b, High: b + 1, Low: b - 1, Close: b}
		p := b * 1.01 // display drifted slightly the "wrong" way is irrelevant here
		prices[d] = scenario.OHLC{Open: p, High: p + 1, Low: p - 1, Close: p}
	}
	// Flip the display's net direction: end 1% below start while tracking base.
	for d := range prices {
		f := 1.0 - 0.02*float64(d)/float64(sc.Days-1)
		prices[d].Close = base[d].Close * f
		prices[d].Open, prices[d].High, prices[d].Low = prices[d].Close, prices[d].Close+1, prices[d].Close-1
	}
	sc.Baseline["F1"] = base
	if err := VerifyFidelity(sc, map[string][]scenario.OHLC{"F1": prices}); err != nil {
		t.Fatalf("near-flat direction flip must be exempt, got: %v", err)
	}
}
```

(If this fixture trips the correlation or extremum checks instead, adjust the fixture — the test's point is solely that DIRECTION does not fail for |net log| < 0.10. Constructing prices as `base × smooth-declining-factor` keeps correlation ≈ 1; the extremum check follows base's own shape.)

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd server && go test ./internal/pipeline/ ./internal/engine/ -run 'TestBuild|TestDotcom|TestFidelityDirection' -v`
Expected: FAIL (undefined: BuildScenario / direction exemption missing).

- [ ] **Step 4: Implement `build.go` and the fidelity exemption**

`server/internal/engine/fidelity.go` — in the cumulative-direction check, add before the sign comparison:

```go
	// 近似横盘的标的没有"大势"可保真：净变动小于 minNetLogForDirection 时
	// 跳过方向检查（相关性与极值检查仍然生效）。spec §4.6 的方向条款保护
	// 的是历史大势，不是 ±2% 的噪声符号。
	const minNetLogForDirection = 0.10
	if math.Abs(baseNet) < minNetLogForDirection {
		// skip direction comparison for this instrument
	} else if ... existing sign check ...
```

(Adapt to the file's actual structure: locate where `baseNet`/display net logs are compared, guard that comparison with the threshold, keep the error message unchanged for the non-exempt path.)

`server/internal/pipeline/build.go`:

```go
package pipeline

import (
	"fmt"
	"math"
	"time"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

type Meta struct {
	Name, RealPeriod string
	Dossiers         map[string]Dossier
}

func BuildMeta() Meta {
	m := Meta{Name: Universe.Name, RealPeriod: Universe.RealPeriod,
		Dossiers: map[string]Dossier{}}
	for _, spec := range Universe.Instruments {
		m.Dossiers[spec.ID] = spec.Dossier
	}
	return m
}

const (
	maxMissingFrac = 0.15
	// Perturbation vol decomposition under the spec-locked engine constants
	// (measured by the Monte Carlo calibration, spec §4.2): shocks shared
	// through MKT/sector channels contribute ≈1.3%/day; the idio channel at
	// scale 1 contributes ≈0.7%/day. IdioScale solves
	//   sharedVol² + (scale·idioBaseVol)² ≈ (targetFrac·σ_i)²
	sharedVol     = 0.013
	idioBaseVol   = 0.007
	targetFrac    = 0.40
	idioScaleMin  = 0.4
	idioScaleMax  = 3.0
	betaClampLo   = -0.5
	betaClampHi   = 3.0
)

func parseDay(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic("universe date " + s + ": " + err.Error()) // static data; fail loud
	}
	return t
}

// BuildScenario assembles the dotcom-2000 scenario from embedded raw data.
// Deterministic and offline.
func BuildScenario() (*scenario.Scenario, error) {
	start, end := parseDay(Universe.WindowStart), parseDay(Universe.WindowEnd)

	// 1. Trading calendar = SPX trading days inside the window.
	spxBars, err := RawSeries("spx")
	if err != nil {
		return nil, fmt.Errorf("spx: %w", err)
	}
	var calendar []time.Time
	for _, b := range spxBars {
		if !b.Date.Before(start) && !b.Date.After(end) {
			calendar = append(calendar, b.Date)
		}
	}
	days := len(calendar)
	if days < 700 {
		return nil, fmt.Errorf("calendar too short: %d days", days)
	}
	dayIndex := make(map[string]int, days)
	for i, d := range calendar {
		dayIndex[d.Format("2006-01-02")] = i
	}

	// 2. Per-instrument aligned, normalized series.
	closes := map[string][]float64{}   // instrument id -> normalized closes
	baseline := map[string][]scenario.OHLC{}
	for _, spec := range Universe.Instruments {
		var bars []Bar
		if spec.Raw != "" {
			raw, err := RawSeries(spec.Raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", spec.ID, err)
			}
			bars, err = alignToCalendar(raw, calendar)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", spec.ID, err)
			}
		} else {
			seed := uint64(0xD07C0)
			for _, c := range spec.ID {
				seed = seed*131 + uint64(c)
			}
			bars, err = Reconstruct(spec.Anchors, calendar, seed)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", spec.ID, err)
			}
		}
		norm := 100 / bars[0].Close
		series := make([]scenario.OHLC, days)
		cl := make([]float64, days)
		for i, b := range bars {
			series[i] = scenario.OHLC{
				Open: b.Open * norm, High: b.High * norm,
				Low: b.Low * norm, Close: b.Close * norm,
			}
			cl[i] = series[i].Close
		}
		baseline[spec.ID] = series
		closes[spec.ID] = cl
	}

	// 3. Log-return series and factor proxies.
	rets := map[string][]float64{}
	for id, cl := range closes {
		rets[id] = logReturns(cl)
	}
	mkt := rets["X22"] // SPX proxy
	sectorRet := map[string][]float64{}
	for _, sec := range Universe.Sectors {
		var members [][]float64
		for _, spec := range Universe.Instruments {
			if spec.Sector == sec.ID {
				members = append(members, rets[spec.ID])
			}
		}
		if len(members) == 0 {
			return nil, fmt.Errorf("sector %s has no members", sec.ID)
		}
		sectorRet[sec.ID] = meanSeries(members)
	}

	// 4. Betas: market OLS, then sector OLS on residuals.
	sectorResid := map[string][]float64{}
	for id, sr := range sectorRet {
		b := olsBeta(sr, mkt)
		sectorResid[id] = residual(sr, mkt, b)
	}
	instruments := make([]scenario.Instrument, 0, len(Universe.Instruments))
	factorIDs := map[string]bool{}
	for _, spec := range Universe.Instruments {
		r := rets[spec.ID]
		bMkt := clampF(olsBeta(r, mkt), betaClampLo, betaClampHi)
		beta := map[string]float64{"MKT": bMkt, "IDIO:" + spec.ID: 1}
		if spec.Sector != "" {
			res := residual(r, mkt, bMkt)
			bSec := clampF(olsBeta(res, sectorResid[spec.Sector]), betaClampLo, betaClampHi)
			beta[spec.Sector] = bSec
		}
		for f, v := range spec.ExtraBeta {
			beta[f] = v
		}
		sigma := stddev(r)
		target := targetFrac * sigma
		var scale float64
		if target*target > sharedVol*sharedVol {
			scale = math.Sqrt(target*target-sharedVol*sharedVol) / idioBaseVol
		} else {
			scale = idioScaleMin
		}
		scale = clampF(scale, idioScaleMin, idioScaleMax)
		instruments = append(instruments, scenario.Instrument{
			ID: spec.ID, Alias: spec.Dossier.Alias, Desc: spec.Dossier.Desc,
			Beta: beta, IdioScale: scale, Reconstructed: spec.Raw == "",
		})
		for f := range beta {
			factorIDs[f] = true
		}
	}

	// 5. Factor declarations in stable order.
	factors := []scenario.Factor{
		{ID: "MKT", Name: "大盘", Kind: scenario.KindMarket},
		{ID: "SENT", Name: "风险情绪", Kind: scenario.KindSentiment},
	}
	for _, sec := range Universe.Sectors {
		factors = append(factors, scenario.Factor{ID: sec.ID, Name: sec.Name, Kind: scenario.KindSector})
	}
	for _, mac := range Universe.Macros {
		factors = append(factors, scenario.Factor{ID: mac.ID, Name: mac.Name, Kind: scenario.KindMacro})
	}
	for _, spec := range Universe.Instruments {
		factors = append(factors, scenario.Factor{
			ID: "IDIO:" + spec.ID, Name: spec.Dossier.Alias, Kind: scenario.KindIdio,
		})
	}

	// 6. Key windows → day indexes.
	var windows []scenario.KeyWindow
	for _, w := range Universe.KeyWindows {
		si, ok1 := dayIndex[w.Start]
		ei, ok2 := dayIndex[w.End]
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("key window %s..%s not on trading calendar", w.Start, w.End)
		}
		windows = append(windows, scenario.KeyWindow{StartDay: si, EndDay: ei, Direction: w.Direction})
	}

	// 7. Every beta key must reference a declared factor (the engine
	// rejects unknown factor ids at world-generation time — catch it here
	// with a clearer message).
	declared := map[string]bool{}
	for _, f := range factors {
		declared[f.ID] = true
	}
	for f := range factorIDs {
		if !declared[f] {
			return nil, fmt.Errorf("beta references undeclared factor %q", f)
		}
	}

	return &scenario.Scenario{
		ID: Universe.ScenarioID, Days: days,
		Factors: factors, Instruments: instruments,
		KeyWindows: windows, Baseline: baseline,
	}, nil
}

func alignToCalendar(bars []Bar, calendar []time.Time) ([]Bar, error) {
	byDate := make(map[string]Bar, len(bars))
	for _, b := range bars {
		byDate[b.Date.Format("2006-01-02")] = b
	}
	out := make([]Bar, len(calendar))
	missing := 0
	var last Bar
	for i, d := range calendar {
		if b, ok := byDate[d.Format("2006-01-02")]; ok {
			out[i] = b
			last = b
		} else {
			if i == 0 {
				return nil, fmt.Errorf("no data on window start %s", d.Format("2006-01-02"))
			}
			missing++
			ff := last
			ff.Date = d
			ff.Open, ff.High, ff.Low = last.Close, last.Close, last.Close
			out[i] = ff
		}
	}
	if frac := float64(missing) / float64(len(calendar)); frac > maxMissingFrac {
		return nil, fmt.Errorf("%.0f%% of days missing", frac*100)
	}
	return out, nil
}

func logReturns(cl []float64) []float64 {
	out := make([]float64, len(cl)-1)
	for i := 1; i < len(cl); i++ {
		out[i-1] = math.Log(cl[i] / cl[i-1])
	}
	return out
}

func meanSeries(series [][]float64) []float64 {
	out := make([]float64, len(series[0]))
	for i := range out {
		var s float64
		for _, sr := range series {
			s += sr[i]
		}
		out[i] = s / float64(len(series))
	}
	return out
}

func olsBeta(y, x []float64) float64 {
	var sx, sy, sxx, sxy float64
	n := float64(len(x))
	for i := range x {
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
	}
	den := sxx - sx*sx/n
	if den == 0 {
		return 0
	}
	return (sxy - sx*sy/n) / den
}

func residual(y, x []float64, beta float64) []float64 {
	out := make([]float64, len(y))
	for i := range y {
		out[i] = y[i] - beta*x[i]
	}
	return out
}

func stddev(x []float64) float64 {
	var s, s2 float64
	n := float64(len(x))
	for _, v := range x {
		s += v
		s2 += v * v
	}
	return math.Sqrt(s2/n - (s/n)*(s/n))
}

func clampF(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}
```

- [ ] **Step 5: Run the suite — and if fidelity fails, follow the playbook, don't grid-search**

Run: `cd server && go test ./internal/pipeline/ ./internal/engine/ -count=1`

**Fidelity failure playbook (plan-1 lesson: diagnose structurally, stop after 2 tuning rounds):**
1. Correlation floor failure on a REAL instrument → its baseline vol is genuinely low; lower `targetFrac` effect via its computed IdioScale (the formula already shrinks perturbation for low-σ stocks; if a specific stock still fails, its IdioScale is at the clamp floor — reduce `idioScaleMin` to 0.3 for all, re-run).
2. Extremum drift on a RECONSTRUCTED instrument → tighten/deepen its tail anchors: the global extreme must beat every rival day by ≥0.15 log with ≤25 trading days per adjacent anchor segment (the Brownian-bridge swing over n days is ≈0.025·√n). Adjust the anchor table in universe.go, not the noise σ.
3. Extremum drift on a REAL instrument → its true extreme is a plateau; verify with a quick print of the top-5 closes. If the runner-up is within the perturbation band and >10 days away, this is a legitimate spec conflict: STOP and report BLOCKED with the instrument + numbers rather than tuning past round 2.
4. Direction failure → should be impossible for |net|≥0.10 instruments; if a near-flat one fails, the Task-5 exemption isn't wired correctly.

Expected: PASS (including `TestDotcomFidelityAcrossSeeds`, non-short mode).

- [ ] **Step 6: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/pipeline/ internal/engine/fidelity.go internal/engine/fidelity_test.go
git commit -m "feat(pipeline): dotcom-2000 scenario build with beta regression and per-stock calibration"
```

---

### Task 6: Monte Carlo calibration property test (spec §4.2 / §9)

**Files:**
- Create: `server/internal/pipeline/calibration_test.go`

**Interfaces:**
- Consumes: Task 5's `BuildScenario`, engine.
- Produces: a property test pinning the perturbation statistics on the REAL scenario across seeds, replacing the plan-1 Python calibration script as the living record of spec §4.2's numbers. `-short` skips it.

- [ ] **Step 1: Write the test (it should PASS immediately if Task 5 calibrated correctly — a failure here is a real calibration bug)**

`server/internal/pipeline/calibration_test.go`:

```go
package pipeline

import (
	"math"
	"sort"
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
)

// Monte Carlo calibration gate (spec §4.2, ported from the plan-1 Python
// study): across seeds and instruments, the perturbation layer must stay
// a *seasoning* on real history — bounded deviation, zero clamp hits.
func TestPerturbationCalibrationStats(t *testing.T) {
	if testing.Short() {
		t.Skip("Monte Carlo calibration is slow")
	}
	sc, err := BuildScenario()
	if err != nil {
		t.Fatal(err)
	}
	var devs []float64      // |log(display/baseline)| samples
	var extraRets []float64 // daily display-vs-baseline excess log returns
	clampHits := 0
	for seed := uint64(100); seed < 110; seed++ {
		world, err := engine.GenerateWorld(sc, seed)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		for _, inst := range sc.Instruments {
			base := sc.Baseline[inst.ID]
			disp := world.Prices[inst.ID]
			prevDev := 0.0
			for d := 0; d < sc.Days; d++ {
				dev := math.Log(disp[d].Close / base[d].Close)
				devs = append(devs, math.Abs(dev))
				if d > 0 {
					extraRets = append(extraRets, dev-prevDev)
				}
				prevDev = dev
				if math.Abs(dev) > 0.30+1e-9 {
					t.Fatalf("%s day %d: deviation %.4f exceeds clamp", inst.ID, d, dev)
				}
				if math.Abs(dev) > 0.299 {
					clampHits++
				}
			}
		}
	}
	sort.Float64s(devs)
	median := devs[len(devs)/2]
	p95 := devs[int(float64(len(devs))*0.95)]
	var s2 float64
	for _, r := range extraRets {
		s2 += r * r
	}
	extraVol := math.Sqrt(s2 / float64(len(extraRets)))

	// Spec §4.2 targets with tolerance for the per-stock-calibrated real
	// scenario: 额外日波动约 1.6%（区间放宽）; 偏离中位约 3%; p95 约 9%;
	// clamp 零触发（纯保险丝）.
	if extraVol < 0.006 || extraVol > 0.030 {
		t.Errorf("extra daily vol %.4f outside [0.006, 0.030]", extraVol)
	}
	if median < 0.008 || median > 0.08 {
		t.Errorf("median |deviation| %.4f outside [0.008, 0.08]", median)
	}
	if p95 > 0.15 {
		t.Errorf("p95 |deviation| %.4f > 0.15", p95)
	}
	if frac := float64(clampHits) / float64(len(devs)); frac > 0.001 {
		t.Errorf("clamp near-hits fraction %.5f > 0.1%% — clamp is not a fuse anymore", frac)
	}
	t.Logf("calibration: extraVol=%.4f median=%.4f p95=%.4f clampNearHits=%d/%d",
		extraVol, median, p95, clampHits, len(devs))
}
```

- [ ] **Step 2: Run it**

Run: `cd server && go test ./internal/pipeline/ -run TestPerturbationCalibration -count=1 -v`
Expected: PASS with a logged stats line. If a bound fails, the fix belongs in Task 5's calibration constants (`sharedVol`/`idioBaseVol`/`targetFrac`) — adjust there with the measured numbers, never widen the spec-derived bounds beyond the stated tolerances, and re-run Task 5's fidelity suite afterwards.

Also confirm the short path: `go test ./internal/pipeline/ -short -count=1` skips it.

- [ ] **Step 3: Commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/pipeline/calibration_test.go
git commit -m "test(pipeline): monte carlo calibration gate for the real scenario"
```

---
### Task 7: Persistence + API — migration 0003, scenario meta, import command, is_host, scenarios list

**Files:**
- Create: `server/internal/store/migrations/0003_dotcom.sql`
- Modify: `server/internal/store/scenarios.go` (IdioScale/Reconstructed round-trip; `SetScenarioMeta`; `ScenarioInfos`; `InstrumentDisplay.RealName`)
- Modify: `server/internal/store/db_test.go` (expect 3 migrations)
- Modify: `server/cmd/pipeline/main.go` (import subcommand), `server/cmd/seedscenario/main.go` (set synthetic meta name)
- Modify: `server/internal/httpapi/server.go` (route), `server/internal/httpapi/rooms.go` (`handleScenarios`, `roomJSON` is_host), `server/internal/httpapi/reveal.go` (real_period)
- Test: `server/internal/store/scenarios_test.go` (append), `server/internal/httpapi/rooms_test.go` (append), `server/internal/httpapi/reveal_test.go` (modify)

**Interfaces:**
- Consumes: Tasks 1, 5.
- Produces:
  - Migration 0003: `scenarios.name TEXT NOT NULL DEFAULT ''`, `scenarios.real_period TEXT NOT NULL DEFAULT ''`, `instruments.idio_scale DOUBLE PRECISION NOT NULL DEFAULT 1`, `instruments.reconstructed BOOLEAN NOT NULL DEFAULT FALSE`, `room_news.cluster_id BIGINT NOT NULL DEFAULT 0`
  - `SaveScenario`/`LoadScenario` round-trip `IdioScale` + `Reconstructed` (existing `TestScenarioRoundTrip`'s DeepEqual keeps passing because synthetic now sets `IdioScale: 1`)
  - `func SetScenarioMeta(ctx context.Context, q Querier, id, name, realPeriod string) error` — `ErrNotFound` on missing id
  - `type ScenarioInfo struct { ID, Name string; Days int }`; `func ScenarioInfos(ctx context.Context, q Querier) ([]ScenarioInfo, error)` — ordered by id
  - `InstrumentDisplay` gains `RealName string`; `SetInstrumentDisplay` writes the `real_name` column with it
  - `cmd/pipeline import` — `DATABASE_URL` required; `BuildScenario` + `SaveScenario` + `SetScenarioMeta` + `SetInstrumentDisplay` (dossiers incl. real names)
  - `cmd/seedscenario` additionally calls `SetScenarioMeta(..., "synthetic-v1", "合成测试剧本", "")`
  - `GET /api/scenarios` → `{"items":[{id,name,days}]}` (name falls back to id when empty)
  - `roomJSON` gains `"is_host": room.HostUserID == <caller>` — signature becomes `roomJSON(room *store.Room, curDay int, ended, started bool, userID int64)`; all five call sites updated
  - Reveal payload gains `"real_period"` (from the room's scenario; empty for synthetic)

- [ ] **Step 1: Write the failing tests**

Append to `server/internal/store/scenarios_test.go`:

```go
func TestScenarioMetaAndInfos(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	sc := scenario.Synthetic()
	if err := SaveScenario(ctx, pool, sc); err != nil {
		t.Fatal(err)
	}
	if err := SetScenarioMeta(ctx, pool, sc.ID, "合成测试剧本", ""); err != nil {
		t.Fatalf("SetScenarioMeta: %v", err)
	}
	if err := SetScenarioMeta(ctx, pool, "nope", "x", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing scenario: %v", err)
	}
	infos, err := ScenarioInfos(ctx, pool)
	if err != nil || len(infos) != 1 {
		t.Fatalf("infos: %+v err=%v", infos, err)
	}
	if infos[0].ID != sc.ID || infos[0].Name != "合成测试剧本" || infos[0].Days != sc.Days {
		t.Fatalf("info: %+v", infos[0])
	}
}

func TestIdioScaleAndRealNameRoundTrip(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	sc := scenario.Synthetic()
	sc.Instruments[0].IdioScale = 1.7
	sc.Instruments[0].Reconstructed = true
	if err := SaveScenario(ctx, pool, sc); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadScenario(ctx, pool, sc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Instruments[0].IdioScale != 1.7 || !loaded.Instruments[0].Reconstructed {
		t.Fatalf("round-trip lost calibration fields: %+v", loaded.Instruments[0])
	}
	if err := SetInstrumentDisplay(ctx, pool, sc.ID, map[string]InstrumentDisplay{
		"S1": {Alias: "郊狼网络", Desc: "d", RealName: "Cisco Systems", Business: "b", Bull: "u", Bear: "r"},
	}); err != nil {
		t.Fatal(err)
	}
	var realName string
	if err := pool.QueryRow(ctx,
		`SELECT real_name FROM instruments WHERE scenario_id=$1 AND id='S1'`, sc.ID).Scan(&realName); err != nil {
		t.Fatal(err)
	}
	if realName != "Cisco Systems" {
		t.Fatalf("real_name: %q", realName)
	}
}
```

Append to `server/internal/httpapi/rooms_test.go`:

```go
func TestScenarioListAndIsHost(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	if err := store.SetScenarioMeta(context.Background(), s.DB, "synthetic-v1", "合成测试剧本", ""); err != nil {
		t.Fatal(err)
	}
	host := registerClient(t, s, "host")
	guest := registerClient(t, s, "guest")

	scen := host.mustJSON("GET", "/api/scenarios", nil, http.StatusOK)
	items := scen["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("scenarios: %v", items)
	}
	first := items[0].(map[string]any)
	if first["id"] != "synthetic-v1" || first["name"] != "合成测试剧本" || first["days"].(float64) != 300 {
		t.Fatalf("scenario info: %v", first)
	}

	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	if created["is_host"] != true {
		t.Fatalf("creator not host: %v", created)
	}
	roomID := int64(created["id"].(float64))
	joined := guest.mustJSON("POST", "/api/rooms/join",
		map[string]any{"invite_code": created["invite_code"]}, http.StatusOK)
	if joined["is_host"] != false {
		t.Fatalf("guest is host: %v", joined)
	}
	state := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil, http.StatusOK)
	if state["room"].(map[string]any)["is_host"] != false {
		t.Fatalf("state is_host wrong for guest")
	}
}
```

(`context` and `store` imports may need adding.)

In `server/internal/httpapi/reveal_test.go`, extend `TestRevealOnlyAfterGameEnds`: before creating the room add

```go
	if err := store.SetScenarioMeta(context.Background(), s.DB, "synthetic-v1", "合成测试剧本", "1999-01 ~ 2001-12"); err != nil {
		t.Fatal(err)
	}
```

and after the reveal succeeds add

```go
	if got["real_period"] != "1999-01 ~ 2001-12" {
		t.Fatalf("real_period: %v", got["real_period"])
	}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ ./internal/httpapi/ -run 'TestScenarioMeta|TestIdioScaleAndRealName|TestScenarioListAndIsHost|TestReveal' -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

`server/internal/store/migrations/0003_dotcom.sql`:

```sql
ALTER TABLE scenarios ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE scenarios ADD COLUMN real_period TEXT NOT NULL DEFAULT '';
ALTER TABLE instruments ADD COLUMN idio_scale DOUBLE PRECISION NOT NULL DEFAULT 1;
ALTER TABLE instruments ADD COLUMN reconstructed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE room_news ADD COLUMN cluster_id BIGINT NOT NULL DEFAULT 0;
```

`server/internal/store/db_test.go`: change the applied-migrations assertion from 2 to 3.

`server/internal/store/scenarios.go`:
- Instrument insert in `SaveScenario` gains the two columns:

```go
			if _, err := tx.Exec(ctx, `
				INSERT INTO instruments (scenario_id, id, ord, alias, descr, beta, idio_scale, reconstructed)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				sc.ID, inst.ID, ord, inst.Alias, inst.Desc, string(beta),
				inst.IdioScale, inst.Reconstructed); err != nil {
				return err
			}
```

- `LoadScenario` instrument query/scan gains them:

```go
	instRows, err := q.Query(ctx, `
		SELECT id, alias, descr, beta, idio_scale, reconstructed FROM instruments
		WHERE scenario_id = $1 ORDER BY ord`, id)
```

```go
		if err := instRows.Scan(&inst.ID, &inst.Alias, &inst.Desc, &beta,
			&inst.IdioScale, &inst.Reconstructed); err != nil {
```

- Append:

```go
func SetScenarioMeta(ctx context.Context, q Querier, id, name, realPeriod string) error {
	tag, err := q.Exec(ctx,
		`UPDATE scenarios SET name = $2, real_period = $3 WHERE id = $1`,
		id, name, realPeriod)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type ScenarioInfo struct {
	ID, Name string
	Days     int
}

func ScenarioInfos(ctx context.Context, q Querier) ([]ScenarioInfo, error) {
	rows, err := q.Query(ctx, `SELECT id, name, days FROM scenarios ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScenarioInfo
	for rows.Next() {
		var s ScenarioInfo
		if err := rows.Scan(&s.ID, &s.Name, &s.Days); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
```

- `InstrumentDisplay` gains `RealName string \`json:"-"\`` and `SetInstrumentDisplay`'s UPDATE becomes:

```go
			tag, err := tx.Exec(ctx, `
				UPDATE instruments SET alias = $3, descr = $4, profile = $5, real_name = $6
				WHERE scenario_id = $1 AND id = $2`,
				scenarioID, id, d.Alias, d.Desc, string(profile), d.RealName)
```

`server/cmd/pipeline/main.go` — add the import subcommand:

```go
	case "import":
		importScenario()
```

```go
func importScenario() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := store.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	sc, err := pipeline.BuildScenario()
	if err != nil {
		log.Fatalf("build: %v", err)
	}
	meta := pipeline.BuildMeta()
	if err := store.SaveScenario(ctx, pool, sc); err != nil {
		log.Fatalf("save: %v", err)
	}
	if err := store.SetScenarioMeta(ctx, pool, sc.ID, meta.Name, meta.RealPeriod); err != nil {
		log.Fatalf("meta: %v", err)
	}
	display := map[string]store.InstrumentDisplay{}
	for id, d := range meta.Dossiers {
		display[id] = store.InstrumentDisplay{
			Alias: d.Alias, Desc: d.Desc, RealName: d.RealName,
			Business: d.Business, Bull: d.Bull, Bear: d.Bear,
		}
	}
	if err := store.SetInstrumentDisplay(ctx, pool, sc.ID, display); err != nil {
		log.Fatalf("display: %v", err)
	}
	log.Printf("imported %q: %d instruments, %d days", sc.ID, len(sc.Instruments), sc.Days)
}
```

(add `context` and the store import.)

`server/cmd/seedscenario/main.go` — after `SetInstrumentDisplay` succeeds:

```go
	if err := store.SetScenarioMeta(ctx, pool, sc.ID, "合成测试剧本", ""); err != nil {
		log.Fatalf("set scenario meta: %v", err)
	}
```

`server/internal/httpapi/rooms.go`:
- `roomJSON` signature and body:

```go
func roomJSON(room *store.Room, curDay int, ended, started bool, userID int64) map[string]any {
	m := map[string]any{
		"id":                room.ID,
		"invite_code":       room.InviteCode,
		"scenario_id":       room.ScenarioID,
		"days":              room.Days,
		"status":            room.Status,
		"day_duration_secs": room.DayDurationSecs,
		"is_host":           room.HostUserID == userID,
	}
	// (started_at / current_day / ended handling unchanged)
```

Update all call sites to pass `userFrom(r).ID`.
- New handler + route (`r.Get("/api/scenarios", s.handleScenarios)` in the authed group):

```go
func (s *Server) handleScenarios(w http.ResponseWriter, r *http.Request) {
	infos, err := store.ScenarioInfos(r.Context(), s.DB)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, info := range infos {
		name := info.Name
		if name == "" {
			name = info.ID
		}
		items = append(items, map[string]any{"id": info.ID, "name": name, "days": info.Days})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
```

`server/internal/httpapi/reveal.go` — after the leaderboard block:

```go
	var realPeriod string
	if err := s.DB.QueryRow(r.Context(), `
		SELECT s.real_period FROM scenarios s JOIN rooms rm ON rm.scenario_id = s.id
		WHERE rm.id = $1`, room.ID).Scan(&realPeriod); err != nil {
		s.storeErr(w, err)
		return
	}
```

and add `"real_period": realPeriod` to the response map.

- [ ] **Step 4: Run the full backend suite + build the import command**

Run: `cd server && STOCKER_TEST_DB=... go test ./... -count=1 -short && go build ./cmd/pipeline ./cmd/seedscenario && rm -f pipeline seedscenario`
Expected: PASS (use `-short` here to skip the slow calibration test; the final sweep runs everything).

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/store/ internal/httpapi/ cmd/
git commit -m "feat(store,api): scenario meta, calibration round-trip, import command, scenarios list, is_host, reveal period"
```

---

### Task 8: LLM batch copy generator (OpenAI-compatible, DeepSeek-ready)

**Files:**
- Create: `server/internal/llm/llm.go`
- Test: `server/internal/llm/llm_test.go`

**Interfaces:**
- Consumes: `engine.NewsEvent` (with Body/ClusterID from Task 2), `scenario.Scenario`, `engine.MediaTable`.
- Produces:
  - `type Config struct { BaseURL, APIKey, Model string; Concurrency int; Timeout time.Duration }`
  - `func FromEnv() *Config` — nil unless `LLM_BASE_URL` AND `LLM_MODEL` set; `LLM_API_KEY` optional (some proxies), `LLM_CONCURRENCY` default 4, `LLM_TIMEOUT_SECS` default 90
  - `func New(cfg Config) *Generator`
  - `func (g *Generator) FillCopy(ctx context.Context, sc *scenario.Scenario, evs []engine.NewsEvent)` — mutates `Headline`/`Body` in place for successfully generated items; on ANY error (HTTP, JSON, validation, timeout) the affected items keep their template copy; never returns an error (logs counts). Cluster members are grouped into the same request chunk so the narrative stays coherent.
  - Prompt rules (spec §4.4, enforced in the system prompt): 使用化名与板块中文名，禁止真实公司/人名/年份/具体日期；媒体人设决定文风（通讯社克制/大报分析/电视口播/小报耸动/论坛口语）；含糊、多信源、对冲措辞；禁止"应声大涨"式看图说话；传闻条目留悬念、追踪条目回收；输出严格 JSON 数组。
  - Shock exposure to the LLM: only qualitative — `ReportShock` mapped to `{因子中文名, 利好|利空, 弱|中|强}` (|v|<0.01 弱, <0.025 中, else 强). Raw numbers, TrueShock, and real identities never enter the prompt.

- [ ] **Step 1: Write the failing tests**

`server/internal/llm/llm_test.go`:

```go
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

func testScenario() *scenario.Scenario {
	sc := scenario.Synthetic()
	sc.Instruments[0].Alias = "郊狼网络"
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
	var gotAuth, gotModel atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		var req struct {
			Model    string           `json:"model"`
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotModel.Store(req.Model)
		// Echo back copy for every idx mentioned in the user message.
		user := req.Messages[len(req.Messages)-1]["content"].(string)
		var out []map[string]any
		for idx := 0; idx < 100; idx++ {
			if strings.Contains(user, fmt.Sprintf(`"idx":%d`, idx)) {
				out = append(out, map[string]any{
					"idx": idx, "headline": fmt.Sprintf("生成标题%d", idx),
					"body": fmt.Sprintf("生成正文%d。", idx),
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
	evs := []engine.NewsEvent{
		{Day: 3, Track: engine.TrackImpact, MediaID: "wire",
			ReportShock: map[string]float64{"MKT": -0.03}, Headline: "模板标题"},
		{Day: 5, Track: engine.TrackNoise, MediaID: "forum", Headline: "模板花边"},
	}
	g.FillCopy(context.Background(), sc, evs)

	if evs[0].Headline != "生成标题0" || evs[0].Body != "生成正文0。" {
		t.Fatalf("item 0 not filled: %+v", evs[0])
	}
	if evs[1].Headline != "生成标题1" {
		t.Fatalf("item 1 not filled: %+v", evs[1])
	}
	if gotAuth.Load() != "Bearer sk-test" {
		t.Fatalf("auth header: %v", gotAuth.Load())
	}
	if gotModel.Load() != "deepseek-chat" {
		t.Fatalf("model: %v", gotModel.Load())
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/llm/ -v`
Expected: FAIL (package missing).

- [ ] **Step 3: Implement**

`server/internal/llm/llm.go`:

```go
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

	type chunk []int
	var chunks []chunk
	for start := 0; start < len(order); start += chunkSize {
		end := start + chunkSize
		if end > len(order) {
			end = len(order)
		}
		chunks = append(chunks, chunk(order[start:end]))
	}

	sem := make(chan struct{}, g.cfg.Concurrency)
	done := make(chan int, len(chunks))
	filled := 0
	for _, ch := range chunks {
		sem <- struct{}{}
		go func(ch chunk) {
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
		if !valid[o.Idx] || h == "" || len([]rune(h)) > 60 || len([]rune(b)) > 400 {
			continue
		}
		evs[o.Idx].Headline = h
		evs[o.Idx].Body = b
		n++
	}
	return n
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/llm/ -count=1`
Expected: PASS.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/llm/
git commit -m "feat(llm): openai-compatible batch news copy generator with template fallback"
```

---

### Task 9: Wire LLM into room creation; persist body and cluster_id

**Files:**
- Modify: `server/internal/store/rooms.go` (`NewsCopyFiller` interface; `CreateRoom` signature + news CopyFrom columns)
- Modify: `server/internal/httpapi/server.go` (`Server.CopyFiller`), `server/internal/httpapi/rooms.go` (pass it)
- Modify: `server/cmd/server/main.go` (configure from env)
- Modify: all `CreateRoom` call sites in tests (add `nil`)
- Test: `server/internal/store/rooms_test.go` (append)

**Interfaces:**
- Consumes: Tasks 2, 7, 8.
- Produces:
  - `type NewsCopyFiller interface { FillCopy(ctx context.Context, sc *scenario.Scenario, evs []engine.NewsEvent) }` (in store; `llm.Generator` satisfies it)
  - `CreateRoom(ctx, db, sc, hostID, dayDurationSecs int, filler NewsCopyFiller)` — after world generation and BEFORE persisting: `if filler != nil { fctx, cancel := context.WithTimeout(ctx, 120*time.Second); filler.FillCopy(fctx, sc, world.News); cancel() }`
  - `room_news` CopyFrom gains `body` and `cluster_id` columns
  - `cmd/server`: `if cfg := llm.FromEnv(); cfg != nil { api.CopyFiller = llm.New(*cfg); log.Printf("llm copy enabled: %s", cfg.Model) }`

- [ ] **Step 1: Write the failing test**

Append to `server/internal/store/rooms_test.go`:

```go
type fakeFiller struct{ calls int }

func (f *fakeFiller) FillCopy(_ context.Context, _ *scenario.Scenario, evs []engine.NewsEvent) {
	f.calls++
	for i := range evs {
		evs[i].Headline = "AI标题"
		evs[i].Body = "AI正文。"
	}
}

func TestCreateRoomAppliesCopyFiller(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)

	filler := &fakeFiller{}
	room, err := CreateRoom(ctx, pool, sc, host.ID, 3600, filler)
	if err != nil {
		t.Fatal(err)
	}
	if filler.calls != 1 {
		t.Fatalf("filler calls: %d", filler.calls)
	}
	var headline, body string
	if err := pool.QueryRow(ctx, `
		SELECT headline, body FROM room_news WHERE room_id = $1 ORDER BY id LIMIT 1`,
		room.ID).Scan(&headline, &body); err != nil {
		t.Fatal(err)
	}
	if headline != "AI标题" || body != "AI正文。" {
		t.Fatalf("copy not persisted: %q %q", headline, body)
	}
	// Clusters persisted (synthetic worlds form clusters — engine Task 2).
	var clustered int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM room_news WHERE room_id = $1 AND cluster_id > 0`,
		room.ID).Scan(&clustered); err != nil {
		t.Fatal(err)
	}
	if clustered == 0 {
		t.Fatal("no clustered news persisted")
	}
}

func TestCreateRoomNilFillerKeepsTemplates(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 3600, nil)
	if err != nil {
		t.Fatal(err)
	}
	var headline, body string
	if err := pool.QueryRow(ctx, `
		SELECT headline, body FROM room_news WHERE room_id = $1 ORDER BY id LIMIT 1`,
		room.ID).Scan(&headline, &body); err != nil {
		t.Fatal(err)
	}
	if headline == "" || body != "" {
		t.Fatalf("template state wrong: %q %q", headline, body)
	}
}
```

(`engine` import may need adding to rooms_test.go.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -run TestCreateRoom -v`
Expected: FAIL (signature).

- [ ] **Step 3: Implement**

`server/internal/store/rooms.go`:

```go
// NewsCopyFiller rewrites news headlines/bodies before a room's world is
// persisted (the LLM generator in internal/llm; nil keeps template copy).
type NewsCopyFiller interface {
	FillCopy(ctx context.Context, sc *scenario.Scenario, evs []engine.NewsEvent)
}
```

`CreateRoom` gains the final parameter; after the world is generated and the invite code made, before the transaction:

```go
	if filler != nil {
		fctx, cancel := context.WithTimeout(ctx, 120*time.Second)
		filler.FillCopy(fctx, sc, world.News)
		cancel()
	}
```

News CopyFrom columns and row values gain `body` + `cluster_id`:

```go
			news = append(news, []any{room.ID, ev.Day, ev.MediaID, ev.Headline,
				string(ev.Track), shockJSON(ev.TrueShock), shockJSON(ev.ReportShock),
				ev.Body, ev.ClusterID})
```

```go
			[]string{"room_id", "day", "media_id", "headline", "track", "true_shock", "report_shock", "body", "cluster_id"},
```

Update ALL call sites (`grep -rn "CreateRoom(" server/` — httpapi/rooms.go passes `s.CopyFiller`; every test passes `nil`).

`server/internal/httpapi/server.go`:

```go
type Server struct {
	DB         *pgxpool.Pool
	Now        func() time.Time
	CopyFiller store.NewsCopyFiller // nil → template news copy
}
```

`server/cmd/server/main.go` after constructing the api server:

```go
	api := httpapi.NewServer(pool)
	if cfg := llm.FromEnv(); cfg != nil {
		api.CopyFiller = llm.New(*cfg)
		log.Printf("llm news copy enabled: model=%s concurrency=%d", cfg.Model, cfg.Concurrency)
	} else {
		log.Printf("llm news copy disabled (LLM_BASE_URL unset) — template copy")
	}
	srv := &http.Server{Addr: addr, Handler: api.Router()}
```

- [ ] **Step 4: Run the full backend suite**

Run: `cd server && STOCKER_TEST_DB=... go test ./... -count=1 -short`
Expected: PASS (plan-2/3 blind-box tests still pass — `body` was already public, `cluster_id` is not exposed by the news handler).

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/store/ internal/httpapi/ cmd/server/
git commit -m "feat(store,api): LLM copy filler hook in room creation; persist news body and cluster id"
```

---

### Task 10: Frontend — scenario picker, computed durations, is_host gating, reveal period

**Files:**
- Modify: `web/src/api.ts`, `web/src/pages/Lobby.tsx`, `web/src/pages/Room.tsx`, `web/src/pages/Reveal.tsx`
- Test: `web/src/pages/Lobby.test.tsx` (modify), `web/src/pages/Room.test.tsx` (append case), `web/src/pages/Reveal.test.tsx` (modify)

**Interfaces:**
- Consumes: Task 7's API additions.
- Produces:
  - `api.ts`: `export type ScenarioInfo = { id: string; name: string; days: number };`; `Room` gains `is_host?: boolean`; `RevealData` gains `real_period?: string`
  - Lobby: fetches `/api/scenarios` on mount; the create form has a scenario `<select>` (defaults to the first entry) and a duration `<select>` whose options derive from the chosen scenario's days: `durationOptions(days)` = for weeks 1/2/4 → `Math.max(60, Math.round(weeks * 604800 / days))` seconds labeled `1周局/2周局/4周局（约 N 分钟/交易日）`, plus `测试局（1 分钟/交易日）` = 60
  - Room lobby card: start button rendered only when `room.is_host`; non-hosts see `等待房主启动时间轴…`
  - Reveal 身份揭晓 card: when `real_period` non-empty, headline line `真实时期：{real_period}`

- [ ] **Step 1: Update the tests (they encode the new behavior and fail first)**

`web/src/pages/Lobby.test.tsx` — the `mockRoutes` handler must also answer `/api/scenarios`:

```ts
      if (url === "/api/scenarios") return { items: [
        { id: "dotcom-2000", name: "2000 互联网泡沫", days: 750 },
        { id: "synthetic-v1", name: "合成测试剧本", days: 300 },
      ] };
```

Replace the "creates a room" case with:

```tsx
  it("creates a room with the selected scenario and computed duration", async () => {
    const bodies: unknown[] = [];
    mockRoutes((url, init) => {
      if (url === "/api/scenarios") return { items: [
        { id: "dotcom-2000", name: "2000 互联网泡沫", days: 750 },
        { id: "synthetic-v1", name: "合成测试剧本", days: 300 },
      ] };
      if (url === "/api/rooms" && init?.method === "POST") {
        bodies.push(JSON.parse(String(init.body)));
        return rooms[2];
      }
      return { rooms: [] };
    });
    render(<MemoryRouter><UserCtxForTest.Provider value={{ id: 1, username: "me" }}><Lobby /></UserCtxForTest.Provider></MemoryRouter>);
    fireEvent.click(await screen.findByRole("button", { name: "＋ 创建新房间" }));
    // scenario defaults to the first entry; pick the 2-week duration
    const selects = screen.getAllByRole("combobox");
    fireEvent.change(selects[1], { target: { value: String(Math.round(2 * 604800 / 750)) } });
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    await waitFor(() => expect(bodies).toEqual([
      { scenario_id: "dotcom-2000", day_duration_secs: Math.round(2 * 604800 / 750) }]));
  });
```

(If existing Lobby tests already wrap in a user provider, keep their shape; only the fetch mock and this case change.)

Append to `web/src/pages/Room.test.tsx`:

```tsx
  it("shows the start button only to the host", async () => {
    const lobbyState = {
      ...state,
      room: { ...state.room, status: "lobby", started_at: undefined, current_day: undefined, is_host: false },
      quotes: [], leaderboard: [],
    };
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = lobbyState;
      else if (u.endsWith("/portfolio")) body = { cash_cents: 10_000_000, total_cents: 10_000_000, positions: [], pending: [] };
      else if (u.endsWith("/trades")) body = { items: [] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(
      <MemoryRouter initialEntries={["/rooms/1"]}>
        <UserCtxForTest.Provider value={{ id: 1, username: "me" }}>
          <Routes><Route path="/rooms/:roomId" element={<Room />} /></Routes>
        </UserCtxForTest.Provider>
      </MemoryRouter>,
    );
    expect(await screen.findByText(/等待房主启动/)).toBeInTheDocument();
    expect(screen.queryByText(/启动时间轴/)).not.toBeInTheDocument();
  });
```

`web/src/pages/Reveal.test.tsx` — add `real_period: "1999-01 ~ 2001-12"` to the `reveal` fixture and assert:

```tsx
    expect(screen.getByText(/1999-01 ~ 2001-12/)).toBeInTheDocument();
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/pages/`
Expected: FAIL.

- [ ] **Step 3: Implement**

`web/src/api.ts` additions:

```ts
export type ScenarioInfo = { id: string; name: string; days: number };
```

`Room` type: add `is_host?: boolean;` — `RevealData`: add `real_period?: string;`

`web/src/pages/Lobby.tsx` — replace the fixed `DURATIONS` with computed options and add the scenario select:

```tsx
function durationOptions(days: number): [string, number][] {
  const opts: [string, number][] = [1, 2, 4].map(weeks => {
    const secs = Math.max(60, Math.round((weeks * 604800) / days));
    return [`${weeks} 周局（约 ${Math.max(1, Math.round(secs / 60))} 分钟/交易日）`, secs];
  });
  opts.push(["测试局（1 分钟/交易日）", 60]);
  return opts;
}
```

State: `const [scenarios, setScenarios] = useState<ScenarioInfo[]>([]);`, `const [scenarioID, setScenarioID] = useState("");`, load once:

```tsx
  useEffect(() => {
    api.get<{ items: ScenarioInfo[] }>("/api/scenarios")
      .then(res => {
        const items = res.items ?? []; // defensive: older mocks/tests may omit it
        setScenarios(items);
        if (items.length && !scenarioID) setScenarioID(items[0]!.id);
      })
      .catch(() => setScenarios([]));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
```

Create form (replaces the old single-select block; `duration` state now re-derives when scenario changes):

```tsx
      {showCreate ? (
        <div className="lobby-form">
          <select value={scenarioID} onChange={e => { setScenarioID(e.target.value); setDuration(0); }}>
            {scenarios.map(sc => <option key={sc.id} value={sc.id}>{sc.name}（{sc.days} 交易日）</option>)}
          </select>
          <select value={duration || durationOptions(currentScenario?.days ?? 300)[1]![1]}
            onChange={e => setDuration(Number(e.target.value))}>
            {durationOptions(currentScenario?.days ?? 300).map(([label, secs]) => (
              <option key={secs} value={secs}>{label}</option>
            ))}
          </select>
          <button className="submit" style={{ width: "auto", padding: "10px 22px" }}
            onClick={create} disabled={busy || !scenarioID}>{busy ? "生成平行世界…" : "创建"}</button>
        </div>
      ) : (
        <button className="ghost-btn" onClick={() => setShowCreate(true)}>＋ 创建新房间</button>
      )}
```

with `const currentScenario = scenarios.find(sc => sc.id === scenarioID);` and `create()` posting `{ scenario_id: scenarioID, day_duration_secs: duration || durationOptions(currentScenario?.days ?? 300)[1]![1] }`. (`duration` state starts at 0 = "use the 2-week default".)

`web/src/pages/Room.tsx` — lobby card start section becomes:

```tsx
            {room.is_host
              ? <button className="submit" style={{ marginTop: 14 }} onClick={startRoom}>启动时间轴（房主）</button>
              : <p className="rc-meta" style={{ marginTop: 14 }}>等待房主启动时间轴…</p>}
```

`web/src/pages/Reveal.tsx` — inside the 身份揭晓 card, above the table:

```tsx
        {data.real_period && (
          <p className="rc-meta">真实时期：<b className="num">{data.real_period}</b></p>
        )}
```

- [ ] **Step 4: Run the frontend suite**

Run: `cd web && npx vitest run && npx tsc --noEmit && npm run build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
git add web/src/
git commit -m "feat(web): scenario picker with computed durations, host-gated start, reveal period"
```

---

### Task 11: Final sweep — dotcom end-to-end smoke, docs

**Files:**
- Modify: `README.md`, `server/README.md`, `web/README.md` (pipeline + LLM sections)
- Test: full suites + live smoke

- [ ] **Step 1: Docs**

`server/README.md` — replace the seedscenario line in Local development with both options and add an LLM section:

```markdown
# Load scenarios
go run ./cmd/seedscenario        # synthetic test scenario
go run ./cmd/pipeline import     # real 2000 dot-com scenario (from committed data)

## LLM news copy (optional)

Set these to generate news copy at room creation via any OpenAI-compatible
endpoint (e.g. DeepSeek). Unset = built-in template copy; generation
failures always fall back to templates.

```bash
export LLM_BASE_URL=https://api.deepseek.com   # /chat/completions appended
export LLM_API_KEY=sk-...
export LLM_MODEL=deepseek-chat
# optional: LLM_CONCURRENCY (default 4), LLM_TIMEOUT_SECS (default 90)
```

## Data pipeline

`internal/pipeline` builds the dotcom-2000 scenario offline from raw CSVs
committed under `internal/pipeline/rawdata/` (fetched once from stooq.com
via `go run ./cmd/pipeline fetch`). Four dead companies (WorldCom, Lucent,
Nortel, Global Crossing) are reconstructed from documented price anchors —
marked `reconstructed` in code and data.
```

Root `README.md` quick start: swap `go run ./cmd/seedscenario` for `go run ./cmd/pipeline import` (and mention seedscenario as the test alternative). `web/README.md`: mention the scenario picker.

- [ ] **Step 2: Full verification (including slow tests)**

```bash
cd /Users/toddzheng/Workspace/react/stocker/server && go vet ./... && gofmt -l . && STOCKER_TEST_DB=postgres://localhost:5432/stocker_test?sslmode=disable go test ./... -count=1
cd /Users/toddzheng/Workspace/react/stocker/web && npx vitest run && npx tsc --noEmit && npm run build
```

Expected: everything green including the multi-seed fidelity and Monte Carlo tests.

- [ ] **Step 3: Live API smoke on the real scenario (no LLM)**

```bash
createdb stocker_smoke 2>/dev/null || true
cd /Users/toddzheng/Workspace/react/stocker/server
DATABASE_URL=postgres://localhost:5432/stocker_smoke?sslmode=disable go run ./cmd/pipeline import
DATABASE_URL=postgres://localhost:5432/stocker_smoke?sslmode=disable ADDR=:8099 go run ./cmd/server &
```

Then with curl: register (cookie jar) → `GET /api/scenarios` (expect dotcom-2000 listed with ~750 days) → create a room `{"scenario_id":"dotcom-2000","day_duration_secs":60}` (expect `is_host: true`; creation includes full world generation for 22×750 — allow ~30 s) → room state (expect 22 instruments with Chinese aliases, no real names in the response body) → news feed (expect template headlines incl. some 【传闻】/【追踪】) → place a buy on `X02` → wait 65 s → portfolio shows the position → reveal returns 409. Kill the server, `dropdb stocker_smoke`. Record all outputs in the report. Browser-level check is left to the human.

If an LLM key is available in the environment (`LLM_BASE_URL` set), optionally repeat room creation once with it and eyeball two generated headlines in the report; do NOT fail the task on LLM issues — that path is fallback-protected by design.

- [ ] **Step 4: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
git add README.md server/README.md web/README.md
git commit -m "docs: dotcom scenario pipeline and LLM configuration"
```
