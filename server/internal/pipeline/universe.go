package pipeline

import "sort"

// FetchSpec maps a short local name to a source symbol.
type FetchSpec struct {
	Name   string // rawdata/<Name>.csv
	Symbol string // Yahoo Finance chart-API symbol
	// From/To bound the fetch window as "2006-01-02" dates (UTC). Empty
	// defaults to the plan-4 dotcom fetch window (cmd/pipeline's
	// period1/period2 constants, 1998-06-01..2002-03-31) — the window most
	// of the original dotcom-2000 symbols still use. Dates before 1970
	// (e.g. "1970-06-01" boundaries land fine, but some series need to
	// start earlier) resolve to negative Unix seconds, which time.Unix
	// handles natively.
	From, To string
}

type Dossier struct {
	Alias, Desc, RealName, Business, Bull, Bear string
}

type SectorSpec struct {
	ID, Name string
}

type InstrumentSpec struct {
	ID        string   // blind-box id; IDIO factor = "IDIO:"+ID
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

// ScenarioUniverse is the static, hand-curated data and calibration knobs
// for one scenario era (dotcom-2000, crash-1987, ...). Every field mirrors
// the shape of the former single `Universe` global; scenarios are added by
// calling Register from an init() in their own universe_<era>.go file.
type ScenarioUniverse struct {
	ScenarioID, Name, RealPeriod string
	// EraHint feeds the LLM's system prompt (see internal/llm); it must obey
	// the same blind-box rules as instrument aliases/descriptions.
	EraHint                string
	WindowStart, WindowEnd string
	// MarketProxy names the instrument (by ID) whose returns define the MKT
	// factor and whose raw series defines the trading calendar.
	MarketProxy string
	Sectors     []SectorSpec
	Macros      []SectorSpec // reuse shape: id+中文名
	Instruments []InstrumentSpec
	KeyWindows  []DateWindow
	// FidelitySeeds is how many seeds TestAllScenariosFidelity runs through
	// engine.GenerateWorld for this scenario. Zero means the default (12),
	// applied by Register.
	FidelitySeeds int
	// FetchSpecs is this universe's contribution to `pipeline fetch`'s
	// download list; the union across all registered universes (deduped by
	// Name) is what actually gets fetched.
	FetchSpecs []FetchSpec
}

const defaultFidelitySeeds = 12

var registry = map[string]*ScenarioUniverse{}

// Register adds a universe to the registry. Panics on duplicate id — all
// registrations happen at init() time from static data, so a collision is a
// programming error, not a runtime condition to handle gracefully.
func Register(u *ScenarioUniverse) {
	if _, exists := registry[u.ScenarioID]; exists {
		panic("pipeline: duplicate scenario id " + u.ScenarioID)
	}
	if u.FidelitySeeds <= 0 {
		u.FidelitySeeds = defaultFidelitySeeds
	}
	registry[u.ScenarioID] = u
}

// Universes returns every registered scenario id, sorted.
func Universes() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ByID looks up a registered universe.
func ByID(id string) (*ScenarioUniverse, bool) {
	u, ok := registry[id]
	return u, ok
}

// pendingFetchSpecs holds FetchSpecs for symbols that plan-5 Task 3 fetches
// ahead of the universes that will consume them. gfc-2008 (Task 6) doesn't
// exist yet when this list is populated, so its raw data can't live on a
// ScenarioUniverse.FetchSpecs field the way dotcom-2000's does. crash-1987
// (Task 4) and nifty-1972 (Task 5) are now registered and have each moved
// their own symbols off this slice onto universe_1987.go's
// crash1987FetchSpecs and universe_1972.go's nifty1972FetchSpecs. Task 6's
// universe_2008.go should do the same when that universe is created; once
// every era is registered this slice should be empty and can be deleted
// along with PendingFetchSpecs.
//
// Until then, `pipeline fetch` (cmd/pipeline/main.go) and the
// embedded-load test (csv_test.go) both union this list with every
// registered universe's FetchSpecs, so the symbols are fetched and
// verified now even though no universe references them yet.
var pendingFetchSpecs = []FetchSpec{
	// dji: requested from 1970-06-01, but Yahoo's chart API has no daily
	// ^DJI history before 1992-01-02 no matter how early period1 is set
	// (verified live against several period1 values spanning 1970-1985 —
	// every one comes back with the same first bar); From reflects the
	// verified floor rather than the brief's aspirational 1970 start, so a
	// future -force refetch doesn't silently re-request an unfulfillable
	// window. This means dji.csv cannot serve crash-1987 or nifty-1972
	// (both windows end before 1992) — both eras use spx (^GSPC, real
	// coverage since 1970-06-01) as their market proxy/index context
	// instead, and nifty-1972 drops its would-be N14 (道琼斯工业指数) entirely
	// per controller ruling (see universe_1972.go). dji remains valid for
	// gfc-2008, which falls entirely inside 1992-2010.
	{Name: "dji", Symbol: "^DJI", From: "1992-01-01", To: "2010-06-30"},
	// gfc-2008 financials + macro proxies.
	{Name: "gs", Symbol: "GS", From: "2006-04-01", To: "2010-06-30"},
	{Name: "ms", Symbol: "MS", From: "2006-04-01", To: "2010-06-30"},
	{Name: "jpm", Symbol: "JPM", From: "2006-04-01", To: "2010-06-30"},
	{Name: "bac", Symbol: "BAC", From: "2006-04-01", To: "2010-06-30"},
	{Name: "c", Symbol: "C", From: "2006-04-01", To: "2010-06-30"},
	{Name: "wfc", Symbol: "WFC", From: "2006-04-01", To: "2010-06-30"},
	{Name: "aig", Symbol: "AIG", From: "2006-04-01", To: "2010-06-30"},
	{Name: "len", Symbol: "LEN", From: "2006-04-01", To: "2010-06-30"},
	{Name: "gld", Symbol: "GLD", From: "2006-04-01", To: "2010-06-30"},
	{Name: "cvx", Symbol: "CVX", From: "2006-04-01", To: "2010-06-30"},
}

// PendingFetchSpecs returns pendingFetchSpecs. Exported so cmd/pipeline's
// fetch command (a different package) can union it into its download list;
// see the doc comment on pendingFetchSpecs for why this list exists.
func PendingFetchSpecs() []FetchSpec {
	return pendingFetchSpecs
}
