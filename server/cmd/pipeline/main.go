// Command pipeline builds and imports the registered scenario universes.
//
//	go run ./cmd/pipeline fetch                        # dev-time: download raw CSVs from Yahoo Finance
//	go run ./cmd/pipeline import                       # build every registered scenario → Postgres
//	go run ./cmd/pipeline import -scenario dotcom-2000 # build just one scenario → Postgres
package main

import (
	"context"
	"encoding/json"
	"flag"
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
		fetch(os.Args[2:])
	case "import":
		importScenarios(os.Args[2:])
	default:
		log.Fatalf("unknown subcommand %q", os.Args[1])
	}
}

// Era window for the fetch, as Unix seconds: 1998-06-01 .. 2002-03-31 UTC.
// Chosen to cover the dotcom scenario window (1999-01-04 .. 2001-12-28) with
// a pre-window margin for β estimation. Other eras widen individual symbol
// files as needed (see plan Task 3); this constant only bounds the initial
// dotcom-era fetch.
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

// unionFetchSpecs collects every registered universe's FetchSpecs, deduped
// by Name (symbols shared across eras, e.g. spx, are fetched once).
func unionFetchSpecs() []pipeline.FetchSpec {
	seen := map[string]bool{}
	var out []pipeline.FetchSpec
	add := func(specs []pipeline.FetchSpec) {
		for _, spec := range specs {
			if seen[spec.Name] {
				continue
			}
			seen[spec.Name] = true
			out = append(out, spec)
		}
	}
	for _, id := range pipeline.Universes() {
		u, ok := pipeline.ByID(id)
		if !ok {
			continue
		}
		add(u.FetchSpecs)
	}
	return out
}

// fetchWindow resolves a spec's period1/period2 (Unix seconds), falling
// back to the plan-4 dotcom window when From/To are empty. Dates before
// 1970 resolve to negative Unix seconds; time.Unix/time.Parse handle that
// natively, no special-casing needed.
func fetchWindow(spec pipeline.FetchSpec) (p1, p2 int64) {
	p1, p2 = period1, period2
	if spec.From != "" {
		t, err := time.Parse("2006-01-02", spec.From)
		if err != nil {
			log.Fatalf("%s: bad From %q: %v", spec.Name, spec.From, err)
		}
		p1 = t.Unix()
	}
	if spec.To != "" {
		t, err := time.Parse("2006-01-02", spec.To)
		if err != nil {
			log.Fatalf("%s: bad To %q: %v", spec.Name, spec.To, err)
		}
		p2 = t.Unix()
	}
	return p1, p2
}

func fetch(args []string) {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	force := fs.Bool("force", false, "re-download even if the file already exists")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}

	specs := unionFetchSpecs()
	client := &http.Client{Timeout: 30 * time.Second}
	failed := 0
	requested := 0
	for _, spec := range specs {
		path := "internal/pipeline/rawdata/" + spec.Name + ".csv"
		if !*force {
			if _, err := os.Stat(path); err == nil {
				log.Printf("skip %s (exists)", path)
				continue
			}
		}
		if requested > 0 {
			time.Sleep(500 * time.Millisecond) // be polite to the source, on every network call
		}
		requested++

		p1, p2 := fetchWindow(spec)
		escaped := strings.ReplaceAll(spec.Symbol, "^", "%5E")
		url := fmt.Sprintf(
			"https://query1.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d",
			escaped, p1, p2,
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

func importScenarios(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	scenarioFlag := fs.String("scenario", "all", `scenario id to import, or "all"`)
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}

	var ids []string
	if *scenarioFlag == "all" {
		ids = pipeline.Universes()
	} else {
		if _, ok := pipeline.ByID(*scenarioFlag); !ok {
			log.Fatalf("unknown scenario %q", *scenarioFlag)
		}
		ids = []string{*scenarioFlag}
	}

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

	for _, id := range ids {
		sc, err := pipeline.BuildScenario(id)
		if err != nil {
			log.Fatalf("build %s: %v", id, err)
		}
		meta, err := pipeline.BuildMeta(id)
		if err != nil {
			log.Fatalf("meta %s: %v", id, err)
		}
		if err := store.SaveScenario(ctx, pool, sc); err != nil {
			log.Fatalf("save %s: %v", id, err)
		}
		if err := store.SetScenarioMeta(ctx, pool, sc.ID, meta.Name, meta.NameEn, meta.RealPeriod); err != nil {
			log.Fatalf("meta %s: %v", id, err)
		}
		display := map[string]store.InstrumentDisplay{}
		for iid, d := range meta.Dossiers {
			display[iid] = store.InstrumentDisplay{
				Alias: d.Alias, Desc: d.Desc, RealName: d.RealName, Aliases: d.Aliases,
				Business: d.Business, Bull: d.Bull, Bear: d.Bear,
				DescEn: d.DescEn, BusinessEn: d.BusinessEn, BullEn: d.BullEn, BearEn: d.BearEn,
			}
		}
		if err := store.SetInstrumentDisplay(ctx, pool, sc.ID, display); err != nil {
			log.Fatalf("display %s: %v", id, err)
		}
		log.Printf("imported %q: %d instruments, %d days", sc.ID, len(sc.Instruments), sc.Days)
	}
}
