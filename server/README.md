# stocker server

Go backend for the time-travel stock game. Single binary + Postgres; no
schedulers, no external services at runtime.

## Local development

```bash
# One-time setup
createdb stocker
export DATABASE_URL=postgres://localhost:5432/stocker?sslmode=disable

# Load scenarios
go run ./cmd/seedscenario        # synthetic test scenario
go run ./cmd/pipeline import     # real 2000 dot-com scenario (from committed data)

# Run the API on :8080
go run ./cmd/server
```

Smoke test:

```bash
curl -c /tmp/jar -X POST localhost:8080/api/register \
  -d '{"username":"alice","password":"password123"}'
curl -b /tmp/jar -X POST localhost:8080/api/rooms \
  -d '{"scenario_id":"synthetic-v1","day_duration_secs":60}'
```

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
committed under `internal/pipeline/rawdata/` (fetched once from the Yahoo
Finance chart API via `go run ./cmd/pipeline fetch`). Four dead companies
(WorldCom, Lucent, Nortel, Global Crossing) are reconstructed from
documented price anchors — marked `reconstructed` in code and data.

## Tests

Unit tests for the engine run anywhere: `go test ./...`.
Store/API tests need a scratch database:

```bash
createdb stocker_test
STOCKER_TEST_DB=postgres://localhost:5432/stocker_test?sslmode=disable go test ./... -count=1
```

Without `STOCKER_TEST_DB` those tests skip.

## Layout

- `internal/engine` — deterministic world generation (plan 1)
- `internal/scenario` — scenario types + built-in synthetic scenario
- `internal/store` — Postgres layer: rooms, orders, lazy settlement
- `internal/httpapi` — REST handlers (blind-box filtering lives here)
- `cmd/server`, `cmd/seedscenario` — entrypoints
