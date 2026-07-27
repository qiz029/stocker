// Command pipeline builds and imports the real 2000 dot-com scenario.
//
//	go run ./cmd/pipeline fetch    # dev-time: download raw CSVs from Yahoo Finance
//	go run ./cmd/pipeline import   # build scenario from embedded data → Postgres (Task 7)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/toddzheng/stocker/server/internal/pipeline"
	"github.com/toddzheng/stocker/server/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: pipeline <fetch|import>")
	}
	switch os.Args[1] {
	case "fetch":
		fetch()
	case "import":
		importScenario()
	default:
		log.Fatalf("unknown subcommand %q", os.Args[1])
	}
}

// Era window for the fetch, as Unix seconds: 1998-06-01 .. 2002-03-31 UTC.
// Chosen to cover the scenario window (1999-01-04 .. 2001-12-28) with a
// pre-window margin for β estimation.
const (
	period1 = 896659200
	period2 = 1017532800
)

// yahooChartResponse is the subset of the Yahoo Finance chart API response
// (https://query1.finance.yahoo.com/v8/finance/chart/<symbol>) that we need.
// Quote fields use *float64 so a JSON `null` (missing bar) is distinguishable
// from a real zero.
type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open  []*float64 `json:"open"`
					High  []*float64 `json:"high"`
					Low   []*float64 `json:"low"`
					Close []*float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func fetch() {
	client := &http.Client{Timeout: 30 * time.Second}
	failed := 0
	for i, spec := range pipeline.FetchList {
		if i > 0 {
			time.Sleep(500 * time.Millisecond) // be polite to the source, on every iteration
		}
		escaped := strings.ReplaceAll(spec.Symbol, "^", "%5E")
		url := fmt.Sprintf(
			"https://query1.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d",
			escaped, period1, period2,
		)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			log.Printf("FAIL %s (%s): %v", spec.Name, spec.Symbol, err)
			failed++
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("FAIL %s (%s): %v", spec.Name, spec.Symbol, err)
			failed++
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			log.Printf("FAIL %s (%s): status=%d bytes=%d err=%v",
				spec.Name, spec.Symbol, resp.StatusCode, len(body), err)
			failed++
			continue
		}

		var parsed yahooChartResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			log.Printf("FAIL %s (%s): bad json: %v", spec.Name, spec.Symbol, err)
			failed++
			continue
		}
		if parsed.Chart.Error != nil {
			log.Printf("FAIL %s (%s): yahoo error: %s %s",
				spec.Name, spec.Symbol, parsed.Chart.Error.Code, parsed.Chart.Error.Description)
			failed++
			continue
		}
		if len(parsed.Chart.Result) == 0 || len(parsed.Chart.Result[0].Indicators.Quote) == 0 {
			log.Printf("FAIL %s (%s): empty result", spec.Name, spec.Symbol)
			failed++
			continue
		}

		result := parsed.Chart.Result[0]
		q := result.Indicators.Quote[0]

		var sb strings.Builder
		sb.WriteString("Date,Open,High,Low,Close,Volume\n")
		rows := 0
		for i, ts := range result.Timestamp {
			if i >= len(q.Open) || i >= len(q.High) || i >= len(q.Low) || i >= len(q.Close) {
				continue
			}
			o, h, l, c := q.Open[i], q.High[i], q.Low[i], q.Close[i]
			if o == nil || h == nil || l == nil || c == nil {
				continue // missing bar
			}
			if *o == 0 || *h == 0 || *l == 0 || *c == 0 {
				continue // missing bar encoded as zero
			}
			date := time.Unix(ts, 0).UTC().Format("2006-01-02")
			sb.WriteString(date)
			sb.WriteByte(',')
			// Fixed 4-decimal precision: plenty for β regression, and it
			// discards Yahoo's floating-point noise (e.g.
			// 0.23660700023174286) that would otherwise bloat the
			// committed CSVs well past what the raw price data needs.
			sb.WriteString(strconv.FormatFloat(*o, 'f', 4, 64))
			sb.WriteByte(',')
			sb.WriteString(strconv.FormatFloat(*h, 'f', 4, 64))
			sb.WriteByte(',')
			sb.WriteString(strconv.FormatFloat(*l, 'f', 4, 64))
			sb.WriteByte(',')
			sb.WriteString(strconv.FormatFloat(*c, 'f', 4, 64))
			sb.WriteString(",0\n")
			rows++
		}
		if rows < 100 {
			log.Printf("FAIL %s (%s): only %d usable bars", spec.Name, spec.Symbol, rows)
			failed++
			continue
		}

		path := "internal/pipeline/rawdata/" + spec.Name + ".csv"
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			log.Fatalf("write %s: %v", path, err)
		}
		log.Printf("ok   %s ← %s (%d bars)", path, spec.Symbol, rows)
	}
	if failed > 0 {
		log.Printf("%d symbols failed — investigate before building", failed)
		os.Exit(1)
	}
}

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
