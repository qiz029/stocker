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

// RawSeries loads a committed raw market-data download (fetched via
// cmd/pipeline fetch) by short name.
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
//
// The name refers to the CSV column format, not the data source: when the
// fetch source was switched to the Yahoo Finance chart API (see
// cmd/pipeline), its JSON response was translated into this same
// Date,Open,High,Low,Close,Volume layout so this parser needed no changes.
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
