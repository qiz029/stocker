package pipeline

import "sort"

// FetchSpec maps a short local name to a source symbol.
type FetchSpec struct {
	Name   string // rawdata/<Name>.csv
	Symbol string // Yahoo Finance chart-API symbol
	// From/To bound the fetch window (YYYY-MM-DD; empty = the dotcom-era
	// default). Pre-1970 dates resolve to negative Unix seconds, which the
	// Yahoo API accepts.
	From, To string
}

type Dossier struct {
	Alias, Desc, RealName, Business, Bull, Bear string
	// Aliases is the full set of candidate display names for the per-room
	// blind-box pick (Alias itself must be one of the entries). Room
	// creation resolves one entry deterministically from the room seed.
	Aliases []string
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
