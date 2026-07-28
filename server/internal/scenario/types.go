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

type Scenario struct {
	ID   string
	Days int
	// EraHint feeds the LLM's system prompt with era-appropriate flavor
	// (see internal/llm); the engine itself ignores it entirely.
	EraHint     string
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
