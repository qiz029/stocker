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
		"Date,Open,High,Low,Close,Volume\n1999-01-04,1,1,1,0,0\n",                       // zero close
		"Date,Open,High,Low,Close,Volume\nnot-a-date,1,1,1,1,0\n",
	}
	for i, c := range cases {
		if _, err := ParseStooqCSV(strings.NewReader(c)); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

// windowMargin tolerates weekends/holidays landing a requested boundary a
// few calendar days away from the first/last committed bar.
const windowMargin = 7 * 24 * time.Hour

// checkCoverage fails the test unless name's committed raw series has
// enough bars and reaches from/to (within windowMargin on each end).
func checkCoverage(t *testing.T, name string, from, to time.Time) {
	t.Helper()
	bars, err := RawSeries(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if len(bars) < 100 {
		t.Errorf("%s: only %d bars", name, len(bars))
	}
	if bars[0].Date.After(from.Add(windowMargin)) {
		t.Errorf("%s: starts too late (%v, want by ~%v)", name, bars[0].Date, from)
	}
	if bars[len(bars)-1].Date.Before(to.Add(-windowMargin)) {
		t.Errorf("%s: ends too early (%v, want through ~%v)", name, bars[len(bars)-1].Date, to)
	}
}

func TestEmbeddedRawSeriesLoad(t *testing.T) {
	// Every fetch-list entry across every registered universe must have
	// committed data that parses and covers that universe's scenario
	// window (not necessarily the wider fetch request window — some
	// symbols legitimately IPO after the fetch's From, e.g. ebay in
	// 1998-09, which is still fine as long as it covers dotcom-2000's
	// actual 1999-01-04..2001-12-28 window).
	for _, id := range Universes() {
		id := id
		t.Run(id, func(t *testing.T) {
			u, ok := ByID(id)
			if !ok {
				t.Fatalf("ByID(%s) missing", id)
			}
			start, err := time.Parse("2006-01-02", u.WindowStart)
			if err != nil {
				t.Fatalf("%s: bad WindowStart: %v", id, err)
			}
			end, err := time.Parse("2006-01-02", u.WindowEnd)
			if err != nil {
				t.Fatalf("%s: bad WindowEnd: %v", id, err)
			}
			for _, spec := range u.FetchSpecs {
				checkCoverage(t, spec.Name, start, end)
			}
		})
	}
}
