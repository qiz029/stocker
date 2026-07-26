# stocker server

Go backend for the time-travel stock game. Single binary + Postgres; no
schedulers, no external services at runtime.

## Local development

```bash
# One-time setup
createdb stocker
export DATABASE_URL=postgres://localhost:5432/stocker?sslmode=disable

# Load the built-in synthetic scenario (real scenarios come from the
# data pipeline, plan 4)
go run ./cmd/seedscenario

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
