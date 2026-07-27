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
