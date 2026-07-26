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
