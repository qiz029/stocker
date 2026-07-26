# Persistence & API Layer (Plan 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Go HTTP service + Postgres persistence: auth, private rooms with world generation on creation, deterministic clock, spot-long trading with lazy settlement, whale alerts, leaderboard, blind-box-safe REST API, and end-of-game reveal.

**Architecture:** Single Go binary (`cmd/server`) over one Postgres database. Room creation calls `engine.GenerateWorld` once and bulk-writes the full parallel world (`room_prices`, `room_news`); afterwards the runtime only reads tables keyed by the deterministic current-day function. No schedulers, no daemons, no websockets: orders record `exec_day = current day + 1` at placement, and every read path first lazily settles due orders inside a transaction (idempotent, concurrency-safe via `FOR UPDATE`).

**Tech Stack:** Go 1.26, chi v5 (router), pgx v5 (Postgres driver + pool), golang.org/x/crypto (bcrypt). Nothing else.

## Global Constraints

- Backend deps are exactly: `github.com/go-chi/chi/v5`, `github.com/jackc/pgx/v5`, `golang.org/x/crypto`. No ORM, no migration tool, no other modules (spec §5.1 "自足无第三方依赖").
- Zero runtime scheduling: no cron, no tickers, no background goroutines that mutate game state (spec §5.2). LLM copy generation is plan 4; this plan stores template copy produced by the engine.
- All game randomness stays inside `internal/engine` seeded streams. The only new randomness is `crypto/rand` for session tokens, invite codes, and the room seed (which is then persisted, making the world reproducible).
- Blind box (spec §2.4): API responses must NEVER contain `Track`, `TrueShock`, `ReportShock`, real instrument identity (`real_name`), or real dates — the single exception is `GET /api/rooms/{id}/reveal`, which is only served after the game has ended.
- Money is integer cents (`BIGINT`, Go `int64`); share quantities are `DOUBLE PRECISION` (Go `float64`). Initial cash is `10_000_000` cents = $100,000 (spec §2.2).
- Orders: buy is by amount (cents), sell is by shares; placement freezes cash/shares immediately; execution at next day's open; cancellable while `status='pending'` and exec day not reached (spec §2.2).
- Whale alert threshold: trade value ≥ 20% of that player's total assets, anonymous (spec §2.3).
- `engine.CurrentDay` caller contract (documented in `engine/clock.go`): callers guarantee `now >= startedAt` and `dayDuration > 0`. `store.Room.CurrentDay` clamps clock skew to day 0; `store.CreateRoom` validates `60 <= day_duration_secs <= 86400`.
- DB tests require env `STOCKER_TEST_DB` (e.g. `postgres://localhost:5432/stocker_test?sslmode=disable`); when unset they `t.Skip`. Each test gets a freshly dropped+recreated Postgres schema, so tests never depend on leftover state.
- Every commit: `go vet ./...` clean, `gofmt` clean, `go test ./... -count=1` green from `server/`.

## File Structure

```
server/
  cmd/server/main.go             # HTTP entrypoint: env config, migrate, serve, graceful shutdown
  cmd/seedscenario/main.go       # dev CLI: writes scenario.Synthetic() into Postgres
  internal/store/
    db.go                        # Connect, Migrate (embedded SQL), Querier interface
    migrations/0001_init.sql     # full schema
    errors.go                    # sentinel errors shared by store + httpapi
    testutil.go                  # TestDB(t, schema): skip-or-fresh-schema pool for tests
    users.go                     # users + sessions CRUD
    scenarios.go                 # SaveScenario / LoadScenario (scenarios, instruments, scenario_prices)
    rooms.go                     # CreateRoom (world gen + COPY), Join/Start/Get/List, Room.CurrentDay
    orders.go                    # PlaceOrder / CancelOrder with freezing
    settle.go                    # SettleRoom / SettleTx lazy settlement + whale events
    portfolio.go                 # GetPortfolio, Leaderboard, assetsCents valuation
  internal/httpapi/
    server.go                    # Server struct, Router(), auth middleware, ctx helpers
    helpers.go                   # readJSON/writeJSON/writeErr/storeErr mapping
    auth.go                      # register/login/logout/me handlers
    rooms.go                     # room lifecycle + state/prices/news/events handlers
    trading.go                   # order place/cancel + portfolio handlers
    reveal.go                    # end-of-game reveal handler
```

Test files live next to their packages: `internal/store/*_test.go`, `internal/httpapi/*_test.go` (with `testutil_test.go` HTTP helpers).

Existing code consumed from plan 1 (do not modify):

```go
// internal/engine
func CurrentDay(startedAt time.Time, dayDuration time.Duration, totalDays int, now time.Time) (int, bool)
func GenerateWorld(sc *scenario.Scenario, seed uint64) (*engine.World, error)
type World struct { ScenarioID string; Seed uint64; Prices map[string][]scenario.OHLC; News []NewsEvent }
type NewsEvent struct { Day int; Track Track; MediaID string; TrueShock, ReportShock map[string]float64; Headline string }
// internal/scenario
func Synthetic() *scenario.Scenario   // 8 instruments ("S1".."S8") × 300 days
type Scenario struct { ID string; Days int; Factors []Factor; Instruments []Instrument; KeyWindows []KeyWindow; Baseline map[string][]OHLC }
type Instrument struct { ID, Alias, Desc string; Beta map[string]float64 }
type OHLC struct { Open, High, Low, Close float64 }
```

---

### Task 1: Postgres bootstrap — pool, embedded migrations, test harness

**Files:**
- Create: `server/internal/store/db.go`
- Create: `server/internal/store/migrations/0001_init.sql`
- Create: `server/internal/store/testutil.go`
- Create: `server/internal/store/errors.go`
- Test: `server/internal/store/db_test.go`
- Modify: `server/go.mod` (add pgx via `go get`)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `store.Connect(ctx context.Context, url string) (*pgxpool.Pool, error)`; `store.Migrate(ctx context.Context, db *pgxpool.Pool) error`; `store.Querier` interface (satisfied by both `*pgxpool.Pool` and `pgx.Tx`); `store.TestDB(t *testing.T, schema string) *pgxpool.Pool`; sentinel errors in `errors.go`. The full DB schema every later task relies on.

- [ ] **Step 1: Add the pgx dependency**

```bash
cd server && go get github.com/jackc/pgx/v5@latest
```

- [ ] **Step 2: Write the failing test**

`server/internal/store/db_test.go`:

```go
package store

import (
	"context"
	"testing"
)

// Requires STOCKER_TEST_DB (e.g. postgres://localhost:5432/stocker_test?sslmode=disable);
// skips otherwise. Create the db once with: createdb stocker_test
func TestMigrateCreatesSchemaAndIsIdempotent(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()

	// TestDB already ran Migrate once; running again must be a no-op.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	// Every table exists and is queryable.
	for _, table := range []string{
		"users", "sessions", "scenarios", "instruments", "scenario_prices",
		"rooms", "room_players", "room_prices", "room_news",
		"orders", "trades", "positions", "room_events",
	} {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Errorf("table %s: %v", table, err)
		}
	}

	// Exactly one migration recorded, exactly once.
	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied migrations = %d, want 1", applied)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=postgres://localhost:5432/stocker_test?sslmode=disable go test ./internal/store/ -run TestMigrate -v`
Expected: FAIL (compile error: `TestDB`, `Migrate` undefined). If the implementer has no local Postgres: `brew services start postgresql@17 && createdb stocker_test` (or document the exact blocker and stop).

- [ ] **Step 4: Write the implementation**

`server/internal/store/db.go`:

```go
// Package store is the Postgres persistence layer. All functions take a
// Querier (pool or transaction); functions that must be transactional take
// pgx.Tx explicitly or open their own via pgx.BeginFunc.
package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the subset of pgx shared by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies embedded migrations in filename order, once each.
// Tables are created unqualified so they land in the connection's
// search_path (tests point that at a throwaway schema).
func Migrate(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version := path.Base(name)
		var done bool
		if err := db.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`,
			version).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sqlText, err := migrationsFS.ReadFile(name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sqlText)); err != nil {
				return fmt.Errorf("migration %s: %w", version, err)
			}
			_, err := tx.Exec(ctx,
				`INSERT INTO schema_migrations (version) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}
```

`server/internal/store/migrations/0001_init.sql`:

```sql
CREATE TABLE users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE scenarios (
    id TEXT PRIMARY KEY,
    days INT NOT NULL,
    factors JSONB NOT NULL,
    key_windows JSONB NOT NULL
);

-- ord preserves declaration order; loading must not sort by id
-- ("S10" < "S2" lexicographically would silently reorder betas/state).
CREATE TABLE instruments (
    scenario_id TEXT NOT NULL REFERENCES scenarios(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    ord INT NOT NULL,
    alias TEXT NOT NULL,
    descr TEXT NOT NULL DEFAULT '',
    real_name TEXT NOT NULL DEFAULT '',
    beta JSONB NOT NULL,
    PRIMARY KEY (scenario_id, id)
);

CREATE TABLE scenario_prices (
    scenario_id TEXT NOT NULL,
    instrument_id TEXT NOT NULL,
    day INT NOT NULL,
    open DOUBLE PRECISION NOT NULL,
    high DOUBLE PRECISION NOT NULL,
    low DOUBLE PRECISION NOT NULL,
    close DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (scenario_id, instrument_id, day),
    FOREIGN KEY (scenario_id, instrument_id)
        REFERENCES instruments (scenario_id, id) ON DELETE CASCADE
);

CREATE TABLE rooms (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    invite_code TEXT NOT NULL UNIQUE,
    scenario_id TEXT NOT NULL REFERENCES scenarios(id),
    days INT NOT NULL,
    seed BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'lobby' CHECK (status IN ('lobby', 'running')),
    day_duration_secs INT NOT NULL,
    started_at TIMESTAMPTZ,
    host_user_id BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE room_players (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    cash_cents BIGINT NOT NULL,
    joined_day INT NOT NULL DEFAULT 0,
    PRIMARY KEY (room_id, user_id)
);

CREATE TABLE room_prices (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    instrument_id TEXT NOT NULL,
    day INT NOT NULL,
    open DOUBLE PRECISION NOT NULL,
    high DOUBLE PRECISION NOT NULL,
    low DOUBLE PRECISION NOT NULL,
    close DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (room_id, instrument_id, day)
);

-- track / true_shock / report_shock are server-side only (blind box).
CREATE TABLE room_news (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    day INT NOT NULL,
    media_id TEXT NOT NULL,
    headline TEXT NOT NULL,
    track TEXT NOT NULL,
    true_shock JSONB,
    report_shock JSONB
);
CREATE INDEX room_news_room_day ON room_news (room_id, day, id);

CREATE TABLE orders (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    instrument_id TEXT NOT NULL,
    side TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
    amount_cents BIGINT NOT NULL DEFAULT 0,
    shares DOUBLE PRECISION NOT NULL DEFAULT 0,
    exec_day INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'filled', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX orders_pending ON orders (room_id, exec_day) WHERE status = 'pending';

CREATE TABLE trades (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id),
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    instrument_id TEXT NOT NULL,
    side TEXT NOT NULL,
    day INT NOT NULL,
    price DOUBLE PRECISION NOT NULL,
    shares DOUBLE PRECISION NOT NULL,
    amount_cents BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX trades_room ON trades (room_id, day, id);

CREATE TABLE positions (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    instrument_id TEXT NOT NULL,
    shares DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (room_id, user_id, instrument_id)
);

CREATE TABLE room_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    day INT NOT NULL,
    kind TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX room_events_room ON room_events (room_id, id);
```

`server/internal/store/errors.go`:

```go
package store

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrUsernameTaken      = errors.New("username taken")
	ErrBadDayDuration     = errors.New("day duration must be between 60 and 86400 seconds")
	ErrRoomEnded          = errors.New("room has ended")
	ErrRoomNotRunning     = errors.New("room is not running")
	ErrAlreadyJoined      = errors.New("already joined this room")
	ErrCannotStart        = errors.New("room can only be started by its host while in lobby")
	ErrNotStarted         = errors.New("room not started")
	ErrBadOrder           = errors.New("invalid order")
	ErrUnknownInstrument  = errors.New("unknown instrument")
	ErrInsufficientCash   = errors.New("insufficient cash")
	ErrInsufficientShares = errors.New("insufficient shares")
	ErrNotCancellable     = errors.New("order cannot be cancelled")
)
```

`server/internal/store/testutil.go`:

```go
package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDB returns a pool whose search_path points at a freshly dropped and
// recreated schema, then runs migrations into it — every test starts from
// zero state. Skips when STOCKER_TEST_DB is unset. Use a distinct schema
// name per test package ("store", "httpapi") so `go test ./...` running
// packages in parallel cannot collide.
func TestDB(t *testing.T, schema string) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("STOCKER_TEST_DB")
	if url == "" {
		t.Skip("STOCKER_TEST_DB not set; skipping Postgres-backed test")
	}
	ctx := context.Background()
	ident := pgx.Identifier{schema}.Sanitize()

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse STOCKER_TEST_DB: %v", err)
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO "+ident)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+ident+" CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd server && STOCKER_TEST_DB=postgres://localhost:5432/stocker_test?sslmode=disable go test ./internal/store/ -count=1 -v`
Expected: PASS. Also run without the env var: `go test ./internal/store/ -count=1` — expected SKIP, not FAIL.

- [ ] **Step 6: Vet, tidy, commit**

```bash
cd server && go vet ./... && go mod tidy
git add go.mod go.sum internal/store/
git commit -m "feat(store): postgres bootstrap with embedded migrations and test harness"
```

---

### Task 2: Users and sessions store

**Files:**
- Create: `server/internal/store/users.go`
- Test: `server/internal/store/users_test.go`
- Modify: `server/go.mod` (add `golang.org/x/crypto` via `go get`)

**Interfaces:**
- Consumes: `Querier`, `TestDB`, `ErrNotFound`, `ErrUsernameTaken` (Task 1).
- Produces:
  - `type User struct { ID int64; Username string; PasswordHash string }`
  - `func CreateUser(ctx context.Context, q Querier, username, passwordHash string) (*User, error)` — `ErrUsernameTaken` on duplicate
  - `func GetUserByUsername(ctx context.Context, q Querier, username string) (*User, error)` — `ErrNotFound`
  - `func CreateSession(ctx context.Context, q Querier, userID int64, token string, expiresAt time.Time) error`
  - `func UserBySession(ctx context.Context, q Querier, token string, now time.Time) (*User, error)` — `ErrNotFound` if missing or expired
  - `func DeleteSession(ctx context.Context, q Querier, token string) error`
  - `func isUniqueViolation(err error) bool` (package-private helper reused by rooms)

- [ ] **Step 1: Add the bcrypt dependency** (used by httpapi in Task 3, added here with the user layer)

```bash
cd server && go get golang.org/x/crypto@latest
```

- [ ] **Step 2: Write the failing test**

`server/internal/store/users_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUsersAndSessions(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()

	u, err := CreateUser(ctx, pool, "alice", "hash1")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 || u.Username != "alice" {
		t.Fatalf("bad user: %+v", u)
	}

	if _, err := CreateUser(ctx, pool, "alice", "hash2"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate username: got %v, want ErrUsernameTaken", err)
	}

	got, err := GetUserByUsername(ctx, pool, "alice")
	if err != nil || got.ID != u.ID || got.PasswordHash != "hash1" {
		t.Fatalf("GetUserByUsername: %+v, %v", got, err)
	}
	if _, err := GetUserByUsername(ctx, pool, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user: got %v, want ErrNotFound", err)
	}

	now := time.Now()
	if err := CreateSession(ctx, pool, u.ID, "tok1", now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	su, err := UserBySession(ctx, pool, "tok1", now)
	if err != nil || su.ID != u.ID {
		t.Fatalf("UserBySession: %+v, %v", su, err)
	}
	// Expired session is invisible.
	if _, err := UserBySession(ctx, pool, "tok1", now.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session: got %v, want ErrNotFound", err)
	}
	// Deleted session is invisible.
	if err := DeleteSession(ctx, pool, "tok1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := UserBySession(ctx, pool, "tok1", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session: got %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -run TestUsersAndSessions -v`
Expected: FAIL (undefined: CreateUser …).

- [ ] **Step 4: Write the implementation**

`server/internal/store/users.go`:

```go
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func CreateUser(ctx context.Context, q Querier, username, passwordHash string) (*User, error) {
	u := &User{Username: username, PasswordHash: passwordHash}
	err := q.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id`,
		username, passwordHash).Scan(&u.ID)
	if isUniqueViolation(err) {
		return nil, ErrUsernameTaken
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByUsername(ctx context.Context, q Querier, username string) (*User, error) {
	u := &User{}
	err := q.QueryRow(ctx,
		`SELECT id, username, password_hash FROM users WHERE username = $1`,
		username).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func CreateSession(ctx context.Context, q Querier, userID int64, token string, expiresAt time.Time) error {
	_, err := q.Exec(ctx,
		`INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)`,
		token, userID, expiresAt)
	return err
}

func UserBySession(ctx context.Context, q Querier, token string, now time.Time) (*User, error) {
	u := &User{}
	err := q.QueryRow(ctx, `
		SELECT u.id, u.username, u.password_hash
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token = $1 AND s.expires_at > $2`,
		token, now).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func DeleteSession(ctx context.Context, q Querier, token string) error {
	_, err := q.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -count=1`
Expected: PASS.

- [ ] **Step 6: Vet, tidy, commit**

```bash
cd server && go vet ./... && go mod tidy
git add go.mod go.sum internal/store/users.go internal/store/users_test.go
git commit -m "feat(store): users and sessions"
```

---

### Task 3: HTTP scaffolding + auth endpoints + server entrypoint

**Files:**
- Create: `server/internal/httpapi/server.go`
- Create: `server/internal/httpapi/helpers.go`
- Create: `server/internal/httpapi/auth.go`
- Create: `server/cmd/server/main.go`
- Test: `server/internal/httpapi/testutil_test.go`, `server/internal/httpapi/auth_test.go`
- Modify: `server/go.mod` (add chi via `go get`)

**Interfaces:**
- Consumes: Task 2 user/session store functions; `store.TestDB`.
- Produces:
  - `type Server struct { DB *pgxpool.Pool; Now func() time.Time }`
  - `func NewServer(db *pgxpool.Pool) *Server` (Now defaults to `time.Now`; tests override it to fake the clock)
  - `func (s *Server) Router() http.Handler`
  - `func (s *Server) requireAuth(next http.Handler) http.Handler` middleware; `userFrom(r *http.Request) *store.User`
  - `writeJSON(w, status, v)`, `readJSON(r, dst) error`, `writeErr(w, status, msg)`, `(s *Server) storeErr(w, err)` — reused by every later handler task
  - Endpoints: `POST /api/register`, `POST /api/login`, `POST /api/logout`, `GET /api/me`
  - Session cookie: name `stocker_session`, HttpOnly, SameSite=Lax, Path=/, 30-day TTL
  - Test helpers: `newServer(t) *Server`, `type client struct`, `newClient(t, s) *client`, `(c *client) do(method, path string, body any) (*http.Response, []byte)`, `(c *client) mustJSON(method, path string, body any, wantStatus int) map[string]any`, `registerClient(t, s, username string) *client`

- [ ] **Step 1: Add the chi dependency**

```bash
cd server && go get github.com/go-chi/chi/v5@latest
```

- [ ] **Step 2: Write the failing tests**

`server/internal/httpapi/testutil_test.go`:

```go
package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/toddzheng/stocker/server/internal/store"
)

func newServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(store.TestDB(t, "httpapi"))
}

// client is a minimal cookie-carrying test client against Server.Router().
type client struct {
	t       *testing.T
	h       http.Handler
	cookies map[string]*http.Cookie
}

func newClient(t *testing.T, s *Server) *client {
	return &client{t: t, h: s.Router(), cookies: map[string]*http.Cookie{}}
}

func (c *client) do(method, path string, body any) (*http.Response, []byte) {
	c.t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	resp := rec.Result()
	for _, ck := range resp.Cookies() {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck
		}
	}
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func (c *client) mustJSON(method, path string, body any, wantStatus int) map[string]any {
	c.t.Helper()
	resp, data := c.do(method, path, body)
	if resp.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: status %d, want %d; body: %s", method, path, resp.StatusCode, wantStatus, data)
	}
	out := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &out); err != nil {
			c.t.Fatalf("%s %s: bad JSON %q: %v", method, path, data, err)
		}
	}
	return out
}

func registerClient(t *testing.T, s *Server, username string) *client {
	t.Helper()
	c := newClient(t, s)
	c.mustJSON("POST", "/api/register",
		map[string]any{"username": username, "password": "password123"}, http.StatusOK)
	return c
}
```

`server/internal/httpapi/auth_test.go`:

```go
package httpapi

import (
	"net/http"
	"testing"
)

func TestRegisterLoginMeLogout(t *testing.T) {
	s := newServer(t)
	c := newClient(t, s)

	// Unauthenticated /me is rejected.
	resp, _ := c.do("GET", "/api/me", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me before register: %d", resp.StatusCode)
	}

	got := c.mustJSON("POST", "/api/register",
		map[string]any{"username": "alice", "password": "password123"}, http.StatusOK)
	if got["username"] != "alice" {
		t.Fatalf("register response: %v", got)
	}

	// Register logs you in (cookie captured by client).
	got = c.mustJSON("GET", "/api/me", nil, http.StatusOK)
	if got["username"] != "alice" {
		t.Fatalf("me: %v", got)
	}

	// Duplicate username.
	c2 := newClient(t, s)
	resp, _ = c2.do("POST", "/api/register",
		map[string]any{"username": "alice", "password": "password456"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate register: %d", resp.StatusCode)
	}

	// Bad credentials.
	resp, _ = c2.do("POST", "/api/login",
		map[string]any{"username": "alice", "password": "wrong-password"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}

	// Good login on a fresh client.
	c2.mustJSON("POST", "/api/login",
		map[string]any{"username": "alice", "password": "password123"}, http.StatusOK)
	c2.mustJSON("GET", "/api/me", nil, http.StatusOK)

	// Logout invalidates the session server-side.
	c2.mustJSON("POST", "/api/logout", nil, http.StatusOK)
	resp, _ = c2.do("GET", "/api/me", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout: %d", resp.StatusCode)
	}
}

func TestRegisterValidation(t *testing.T) {
	s := newServer(t)
	c := newClient(t, s)
	for _, bad := range []map[string]any{
		{"username": "ab", "password": "password123"},        // too short
		{"username": "has space", "password": "password123"}, // bad charset
		{"username": "alice", "password": "short"},           // weak password
	} {
		resp, _ := c.do("POST", "/api/register", bad)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("register %v: status %d, want 400", bad, resp.StatusCode)
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/httpapi/ -v`
Expected: FAIL (package does not exist / undefined Server).

- [ ] **Step 4: Write the implementation**

`server/internal/httpapi/server.go`:

```go
// Package httpapi is the REST layer. Handlers stay thin: decode, call
// store, map errors. Blind-box rule: no response may carry news track,
// shock vectors, or real instrument identity except the reveal endpoint.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/store"
)

type Server struct {
	DB *pgxpool.Pool
	// Now is the wall clock; tests override it to steer the deterministic
	// timeline. Everything time-dependent must go through s.Now().
	Now func() time.Time
}

func NewServer(db *pgxpool.Pool) *Server {
	return &Server{DB: db, Now: time.Now}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Post("/api/register", s.handleRegister)
	r.Post("/api/login", s.handleLogin)
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Post("/api/logout", s.handleLogout)
		r.Get("/api/me", s.handleMe)
		// Room and trading routes are registered here by later tasks.
	})
	return r
}

type ctxKey int

const userKey ctxKey = 0

func userFrom(r *http.Request) *store.User {
	u, _ := r.Context().Value(userKey).(*store.User)
	return u
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie(sessionCookie)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not logged in")
			return
		}
		u, err := store.UserBySession(r.Context(), s.DB, ck.Value, s.Now())
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not logged in")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}
```

`server/internal/httpapi/helpers.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/toddzheng/stocker/server/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(dst)
}

// storeErr maps store sentinel errors onto HTTP statuses.
func (s *Server) storeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrUsernameTaken),
		errors.Is(err, store.ErrAlreadyJoined),
		errors.Is(err, store.ErrRoomEnded),
		errors.Is(err, store.ErrNotCancellable):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrCannotStart):
		writeErr(w, http.StatusForbidden, err.Error())
	case errors.Is(err, store.ErrBadDayDuration),
		errors.Is(err, store.ErrBadOrder),
		errors.Is(err, store.ErrUnknownInstrument),
		errors.Is(err, store.ErrInsufficientCash),
		errors.Is(err, store.ErrInsufficientShares),
		errors.Is(err, store.ErrRoomNotRunning),
		errors.Is(err, store.ErrNotStarted):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("internal error: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}
```

`server/internal/httpapi/auth.go`:

```go
package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/toddzheng/stocker/server/internal/store"
)

const (
	sessionCookie = "stocker_session"
	sessionTTL    = 30 * 24 * 3600 // seconds
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	expires := s.Now().Add(time.Duration(sessionTTL) * time.Second)
	if err := store.CreateSession(r.Context(), s.DB, u.ID, token, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: sessionTTL,
	})
	return nil
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !usernameRe.MatchString(req.Username) {
		writeErr(w, http.StatusBadRequest, "username must be 3-32 chars of letters, digits, underscore")
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	u, err := store.CreateUser(r.Context(), s.DB, req.Username, string(hash))
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": u.ID, "username": u.Username})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	u, err := store.GetUserByUsername(r.Context(), s.DB, req.Username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": u.ID, "username": u.Username})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(sessionCookie); err == nil {
		_ = store.DeleteSession(r.Context(), s.DB, ck.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeJSON(w, http.StatusOK, map[string]any{"id": u.ID, "username": u.Username})
}
```

`server/cmd/server/main.go`:

```go
// Command server runs the stocker HTTP API.
//
//	DATABASE_URL=postgres://localhost/stocker?sslmode=disable ADDR=:8080 go run ./cmd/server
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/toddzheng/stocker/server/internal/httpapi"
	"github.com/toddzheng/stocker/server/internal/store"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
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

	srv := &http.Server{Addr: addr, Handler: httpapi.NewServer(pool).Router()}
	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/httpapi/ -count=1 -v` and `go build ./cmd/server`
Expected: PASS; server binary compiles.

- [ ] **Step 6: Vet, tidy, commit**

```bash
cd server && go vet ./... && go mod tidy
git add go.mod go.sum internal/httpapi/ cmd/server/
git commit -m "feat(api): http scaffolding, auth endpoints, server entrypoint"
```

---
### Task 4: Scenario store + seed CLI

**Files:**
- Create: `server/internal/store/scenarios.go`
- Create: `server/cmd/seedscenario/main.go`
- Test: `server/internal/store/scenarios_test.go`

**Interfaces:**
- Consumes: `Querier`, `TestDB`, `ErrNotFound` (Task 1); `scenario.Scenario`, `scenario.Synthetic()`, `engine.GenerateWorld` (plan 1).
- Produces:
  - `func SaveScenario(ctx context.Context, db *pgxpool.Pool, sc *scenario.Scenario) error` — upsert (delete + reinsert in one tx); `real_name` column stays `''` (plan 4's pipeline fills it)
  - `func LoadScenario(ctx context.Context, q Querier, id string) (*scenario.Scenario, error)` — `ErrNotFound` if missing; MUST reproduce instrument declaration order via the `ord` column (never `ORDER BY id`: `"S10" < "S2"` lexicographically)

- [ ] **Step 1: Write the failing test**

`server/internal/store/scenarios_test.go`:

```go
package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestScenarioRoundTrip(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	orig := scenario.Synthetic()

	if err := SaveScenario(ctx, pool, orig); err != nil {
		t.Fatalf("SaveScenario: %v", err)
	}
	// Saving again must not error or duplicate (upsert).
	if err := SaveScenario(ctx, pool, orig); err != nil {
		t.Fatalf("second SaveScenario: %v", err)
	}

	loaded, err := LoadScenario(ctx, pool, orig.ID)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	if !reflect.DeepEqual(orig, loaded) {
		t.Fatalf("round-trip mismatch:\norig:   %+v\nloaded: %+v", orig, loaded)
	}

	if _, err := LoadScenario(ctx, pool, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing scenario: got %v, want ErrNotFound", err)
	}
}

// The determinism gate: a world generated from the DB-loaded scenario must
// be byte-identical (same prices, same news) to one generated from the
// in-memory scenario. float64 survives DOUBLE PRECISION and JSONB exactly
// with Go's encoders, so DeepEqual is the right check.
func TestLoadedScenarioGeneratesIdenticalWorld(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	orig := scenario.Synthetic()
	if err := SaveScenario(ctx, pool, orig); err != nil {
		t.Fatalf("SaveScenario: %v", err)
	}
	loaded, err := LoadScenario(ctx, pool, orig.ID)
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}

	w1, err := engine.GenerateWorld(orig, 42)
	if err != nil {
		t.Fatalf("GenerateWorld(orig): %v", err)
	}
	w2, err := engine.GenerateWorld(loaded, 42)
	if err != nil {
		t.Fatalf("GenerateWorld(loaded): %v", err)
	}
	if !reflect.DeepEqual(w1.Prices, w2.Prices) {
		t.Fatal("prices differ between original and DB-loaded scenario")
	}
	if !reflect.DeepEqual(w1.News, w2.News) {
		t.Fatal("news differ between original and DB-loaded scenario")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -run TestScenario -v`
Expected: FAIL (undefined: SaveScenario).

- [ ] **Step 3: Write the implementation**

`server/internal/store/scenarios.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// SaveScenario upserts a full scenario (metadata, instruments, baseline
// prices) in one transaction. Existing rows for the same id are replaced.
func SaveScenario(ctx context.Context, db *pgxpool.Pool, sc *scenario.Scenario) error {
	factors, err := json.Marshal(sc.Factors)
	if err != nil {
		return err
	}
	keyWindows, err := json.Marshal(sc.KeyWindows)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// ON DELETE CASCADE wipes instruments and scenario_prices too.
		if _, err := tx.Exec(ctx, `DELETE FROM scenarios WHERE id = $1`, sc.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO scenarios (id, days, factors, key_windows) VALUES ($1, $2, $3, $4)`,
			sc.ID, sc.Days, string(factors), string(keyWindows)); err != nil {
			return err
		}
		for ord, inst := range sc.Instruments {
			beta, err := json.Marshal(inst.Beta)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO instruments (scenario_id, id, ord, alias, descr, beta)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				sc.ID, inst.ID, ord, inst.Alias, inst.Desc, string(beta)); err != nil {
				return err
			}
		}
		rows := make([][]any, 0, len(sc.Instruments)*sc.Days)
		for _, inst := range sc.Instruments {
			for d, p := range sc.Baseline[inst.ID] {
				rows = append(rows, []any{sc.ID, inst.ID, d, p.Open, p.High, p.Low, p.Close})
			}
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"scenario_prices"},
			[]string{"scenario_id", "instrument_id", "day", "open", "high", "low", "close"},
			pgx.CopyFromRows(rows))
		return err
	})
}

func LoadScenario(ctx context.Context, q Querier, id string) (*scenario.Scenario, error) {
	sc := &scenario.Scenario{ID: id, Baseline: map[string][]scenario.OHLC{}}
	var factors, keyWindows []byte
	err := q.QueryRow(ctx,
		`SELECT days, factors, key_windows FROM scenarios WHERE id = $1`, id).
		Scan(&sc.Days, &factors, &keyWindows)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(factors, &sc.Factors); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(keyWindows, &sc.KeyWindows); err != nil {
		return nil, err
	}

	instRows, err := q.Query(ctx, `
		SELECT id, alias, descr, beta FROM instruments
		WHERE scenario_id = $1 ORDER BY ord`, id)
	if err != nil {
		return nil, err
	}
	defer instRows.Close()
	for instRows.Next() {
		var inst scenario.Instrument
		var beta []byte
		if err := instRows.Scan(&inst.ID, &inst.Alias, &inst.Desc, &beta); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(beta, &inst.Beta); err != nil {
			return nil, err
		}
		sc.Instruments = append(sc.Instruments, inst)
	}
	if err := instRows.Err(); err != nil {
		return nil, err
	}

	for i := range sc.Instruments {
		sc.Baseline[sc.Instruments[i].ID] = make([]scenario.OHLC, sc.Days)
	}
	priceRows, err := q.Query(ctx, `
		SELECT instrument_id, day, open, high, low, close
		FROM scenario_prices WHERE scenario_id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer priceRows.Close()
	for priceRows.Next() {
		var instID string
		var day int
		var p scenario.OHLC
		if err := priceRows.Scan(&instID, &day, &p.Open, &p.High, &p.Low, &p.Close); err != nil {
			return nil, err
		}
		sc.Baseline[instID][day] = p // assign by index: row order is irrelevant
	}
	return sc, priceRows.Err()
}
```

`server/cmd/seedscenario/main.go`:

```go
// Command seedscenario writes the built-in synthetic scenario into
// Postgres so rooms can be created against it during development.
// Plan 4's data pipeline replaces this with real historical scenarios.
//
//	DATABASE_URL=postgres://localhost/stocker?sslmode=disable go run ./cmd/seedscenario
package main

import (
	"context"
	"log"
	"os"

	"github.com/toddzheng/stocker/server/internal/scenario"
	"github.com/toddzheng/stocker/server/internal/store"
)

func main() {
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
	sc := scenario.Synthetic()
	if err := store.SaveScenario(ctx, pool, sc); err != nil {
		log.Fatalf("save scenario: %v", err)
	}
	log.Printf("seeded scenario %q (%d instruments, %d days)", sc.ID, len(sc.Instruments), sc.Days)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -count=1 && go build ./cmd/seedscenario`
Expected: PASS, CLI compiles.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./...
git add internal/store/scenarios.go internal/store/scenarios_test.go cmd/seedscenario/
git commit -m "feat(store): scenario persistence and seed CLI"
```

---

### Task 5: Rooms — create (world generation), join, start, deterministic clock

**Files:**
- Create: `server/internal/store/rooms.go`
- Test: `server/internal/store/rooms_test.go`

**Interfaces:**
- Consumes: Tasks 1–4; `engine.GenerateWorld`, `engine.CurrentDay`.
- Produces:
  - `const InitialCashCents int64 = 10_000_000`
  - `type Room struct { ID int64; InviteCode string; ScenarioID string; Days int; Seed uint64; Status string; DayDurationSecs int; StartedAt *time.Time; HostUserID int64 }`
  - `func (r *Room) CurrentDay(now time.Time) (day int, ended bool, err error)` — `ErrNotStarted` when `StartedAt == nil`; clamps `now < StartedAt` (clock skew) to day 0, satisfying engine.CurrentDay's caller contract
  - `func CreateRoom(ctx context.Context, db *pgxpool.Pool, sc *scenario.Scenario, hostID int64, dayDurationSecs int) (*Room, error)` — validates duration, generates the world (retrying derived seeds when the fidelity gate rejects one), bulk-inserts `room_prices` + `room_news`, adds host as player
  - `func GetRoom(ctx context.Context, q Querier, id int64) (*Room, error)`; `func GetRoomByInvite(ctx context.Context, q Querier, code string) (*Room, error)` — `ErrNotFound`
  - `func JoinRoom(ctx context.Context, db Querier, room *Room, userID int64, now time.Time) (joinedDay int, err error)` — `ErrAlreadyJoined`, `ErrRoomEnded`
  - `func StartRoom(ctx context.Context, q Querier, roomID, hostID int64, now time.Time) (*Room, error)` — `ErrCannotStart`
  - `func ListRoomsForUser(ctx context.Context, q Querier, userID int64) ([]Room, error)`
  - `func IsMember(ctx context.Context, q Querier, roomID, userID int64) (bool, error)`

- [ ] **Step 1: Write the failing test**

`server/internal/store/rooms_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

// mkUser / mkScenario are shared by later store tests too.
func mkUser(t *testing.T, pool *pgxpool.Pool, name string) *User {
	t.Helper()
	u, err := CreateUser(context.Background(), pool, name, "hash")
	if err != nil {
		t.Fatalf("mkUser(%s): %v", name, err)
	}
	return u
}

func mkScenario(t *testing.T, pool *pgxpool.Pool) *scenario.Scenario {
	t.Helper()
	sc := scenario.Synthetic()
	if err := SaveScenario(context.Background(), pool, sc); err != nil {
		t.Fatalf("mkScenario: %v", err)
	}
	return sc
}

func TestCreateRoomPersistsWorld(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)

	room, err := CreateRoom(ctx, pool, sc, host.ID, 3600)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.Status != "lobby" || room.Days != sc.Days || room.InviteCode == "" {
		t.Fatalf("bad room: %+v", room)
	}

	var nPrices int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM room_prices WHERE room_id = $1`, room.ID).Scan(&nPrices); err != nil {
		t.Fatal(err)
	}
	if want := len(sc.Instruments) * sc.Days; nPrices != want {
		t.Fatalf("room_prices rows = %d, want %d", nPrices, want)
	}

	// The stored world must equal a regeneration from the stored seed —
	// proves the seed persisted is the seed used.
	world, err := engine.GenerateWorld(sc, room.Seed)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	var nNews int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM room_news WHERE room_id = $1`, room.ID).Scan(&nNews); err != nil {
		t.Fatal(err)
	}
	if nNews != len(world.News) {
		t.Fatalf("room_news rows = %d, want %d", nNews, len(world.News))
	}
	var gotClose float64
	if err := pool.QueryRow(ctx, `
		SELECT close FROM room_prices
		WHERE room_id = $1 AND instrument_id = 'S1' AND day = 299`, room.ID).Scan(&gotClose); err != nil {
		t.Fatal(err)
	}
	if want := world.Prices["S1"][299].Close; gotClose != want {
		t.Fatalf("S1 day299 close = %v, want %v", gotClose, want)
	}

	// Host is seated with initial cash.
	var cash int64
	if err := pool.QueryRow(ctx,
		`SELECT cash_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		room.ID, host.ID).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	if cash != InitialCashCents {
		t.Fatalf("host cash = %d, want %d", cash, InitialCashCents)
	}
}

func TestCreateRoomValidatesDayDuration(t *testing.T) {
	pool := TestDB(t, "store")
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)
	for _, secs := range []int{0, 59, 86401, -5} {
		if _, err := CreateRoom(context.Background(), pool, sc, host.ID, secs); !errors.Is(err, ErrBadDayDuration) {
			t.Errorf("duration %d: got %v, want ErrBadDayDuration", secs, err)
		}
	}
}

func TestJoinStartAndClock(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	guest := mkUser(t, pool, "guest")
	late := mkUser(t, pool, "late")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Truncate(time.Second)

	// Clock before start.
	if _, _, err := room.CurrentDay(t0); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("CurrentDay before start: %v, want ErrNotStarted", err)
	}

	// Lobby join.
	day, err := JoinRoom(ctx, pool, room, guest.ID, t0)
	if err != nil || day != 0 {
		t.Fatalf("lobby join: day=%d err=%v", day, err)
	}
	if _, err := JoinRoom(ctx, pool, room, guest.ID, t0); !errors.Is(err, ErrAlreadyJoined) {
		t.Fatalf("double join: %v, want ErrAlreadyJoined", err)
	}

	// Only host can start.
	if _, err := StartRoom(ctx, pool, room.ID, guest.ID, t0); !errors.Is(err, ErrCannotStart) {
		t.Fatalf("guest start: %v, want ErrCannotStart", err)
	}
	room, err = StartRoom(ctx, pool, room.ID, host.ID, t0)
	if err != nil || room.Status != "running" || room.StartedAt == nil {
		t.Fatalf("host start: %+v, %v", room, err)
	}
	if _, err := StartRoom(ctx, pool, room.ID, host.ID, t0); !errors.Is(err, ErrCannotStart) {
		t.Fatalf("double start: %v, want ErrCannotStart", err)
	}

	// Deterministic clock (60s per day, 300 days).
	for _, tc := range []struct {
		at    time.Time
		day   int
		ended bool
	}{
		{t0.Add(-time.Hour), 0, false}, // clock skew clamps, never panics
		{t0, 0, false},
		{t0.Add(59 * time.Second), 0, false},
		{t0.Add(61 * time.Second), 1, false},
		{t0.Add(150 * 60 * time.Second), 150, false},
		{t0.Add(300 * 60 * time.Second), 299, true},
		{t0.Add(9999 * 60 * time.Second), 299, true},
	} {
		day, ended, err := room.CurrentDay(tc.at)
		if err != nil || day != tc.day || ended != tc.ended {
			t.Errorf("CurrentDay(%v) = (%d,%v,%v), want (%d,%v)", tc.at.Sub(t0), day, ended, err, tc.day, tc.ended)
		}
	}

	// Mid-game join stamps joined_day; post-game join refused.
	day, err = JoinRoom(ctx, pool, room, late.ID, t0.Add(5*60*time.Second))
	if err != nil || day != 5 {
		t.Fatalf("late join: day=%d err=%v", day, err)
	}
	extra := mkUser(t, pool, "extra")
	if _, err := JoinRoom(ctx, pool, room, extra.ID, t0.Add(400*60*time.Second)); !errors.Is(err, ErrRoomEnded) {
		t.Fatalf("post-game join: %v, want ErrRoomEnded", err)
	}

	// Lookup helpers.
	byInvite, err := GetRoomByInvite(ctx, pool, room.InviteCode)
	if err != nil || byInvite.ID != room.ID {
		t.Fatalf("GetRoomByInvite: %+v, %v", byInvite, err)
	}
	rooms, err := ListRoomsForUser(ctx, pool, guest.ID)
	if err != nil || len(rooms) != 1 || rooms[0].ID != room.ID {
		t.Fatalf("ListRoomsForUser: %+v, %v", rooms, err)
	}
	member, err := IsMember(ctx, pool, room.ID, guest.ID)
	if err != nil || !member {
		t.Fatalf("IsMember(guest): %v, %v", member, err)
	}
	member, err = IsMember(ctx, pool, room.ID, extra.ID)
	if err != nil || member {
		t.Fatalf("IsMember(extra): %v, %v", member, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -run 'TestCreateRoom|TestJoinStart' -v`
Expected: FAIL (undefined: CreateRoom …).

- [ ] **Step 3: Write the implementation**

`server/internal/store/rooms.go`:

```go
package store

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

const InitialCashCents int64 = 10_000_000 // $100,000 (spec §2.2)

type Room struct {
	ID              int64
	InviteCode      string
	ScenarioID      string
	Days            int
	Seed            uint64
	Status          string // "lobby" | "running"
	DayDurationSecs int
	StartedAt       *time.Time
	HostUserID      int64
}

const roomCols = `id, invite_code, scenario_id, days, seed, status, day_duration_secs, started_at, host_user_id`

func scanRoom(row pgx.Row) (*Room, error) {
	r := &Room{}
	var seed int64
	err := row.Scan(&r.ID, &r.InviteCode, &r.ScenarioID, &r.Days, &seed,
		&r.Status, &r.DayDurationSecs, &r.StartedAt, &r.HostUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.Seed = uint64(seed) // seeds are stored bit-cast into BIGINT
	return r, nil
}

// CurrentDay maps wall clock to the historical trading day.
// engine.CurrentDay's contract requires now >= startedAt and a positive
// duration; the duration is validated at creation and clock skew is
// clamped here, so the contract always holds.
func (r *Room) CurrentDay(now time.Time) (int, bool, error) {
	if r.StartedAt == nil {
		return 0, false, ErrNotStarted
	}
	if now.Before(*r.StartedAt) {
		now = *r.StartedAt
	}
	day, ended := engine.CurrentDay(*r.StartedAt,
		time.Duration(r.DayDurationSecs)*time.Second, r.Days, now)
	return day, ended, nil
}

func newInviteCode() (string, error) {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]), nil
}

func shockJSON(m map[string]float64) any {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return string(b)
}

// CreateRoom generates this room's parallel world and persists it whole.
// The fidelity gate (engine.VerifyFidelity) can reject a seed; we retry
// derived seeds a bounded number of times (spec §4.6: "不达标的参数组合拒绝").
func CreateRoom(ctx context.Context, db *pgxpool.Pool, sc *scenario.Scenario, hostID int64, dayDurationSecs int) (*Room, error) {
	if dayDurationSecs < 60 || dayDurationSecs > 86400 {
		return nil, ErrBadDayDuration
	}
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return nil, err
	}
	base := binary.LittleEndian.Uint64(b[:])
	var world *engine.World
	var seed uint64
	var lastErr error
	for attempt := uint64(0); attempt < 10; attempt++ {
		w, err := engine.GenerateWorld(sc, base+attempt)
		if err == nil {
			world, seed = w, base+attempt
			break
		}
		lastErr = err
	}
	if world == nil {
		return nil, fmt.Errorf("world generation failed after 10 seeds: %w", lastErr)
	}
	invite, err := newInviteCode()
	if err != nil {
		return nil, err
	}

	var room *Room
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		r, err := scanRoom(tx.QueryRow(ctx, `
			INSERT INTO rooms (invite_code, scenario_id, days, seed, day_duration_secs, host_user_id)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+roomCols,
			invite, sc.ID, sc.Days, int64(seed), dayDurationSecs, hostID))
		if err != nil {
			return err
		}
		room = r
		if _, err := tx.Exec(ctx, `
			INSERT INTO room_players (room_id, user_id, cash_cents, joined_day)
			VALUES ($1, $2, $3, 0)`, room.ID, hostID, InitialCashCents); err != nil {
			return err
		}

		prices := make([][]any, 0, len(sc.Instruments)*sc.Days)
		for _, inst := range sc.Instruments {
			for d, p := range world.Prices[inst.ID] {
				prices = append(prices, []any{room.ID, inst.ID, d, p.Open, p.High, p.Low, p.Close})
			}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"room_prices"},
			[]string{"room_id", "instrument_id", "day", "open", "high", "low", "close"},
			pgx.CopyFromRows(prices)); err != nil {
			return err
		}

		news := make([][]any, 0, len(world.News))
		for _, ev := range world.News {
			news = append(news, []any{room.ID, ev.Day, ev.MediaID, ev.Headline,
				string(ev.Track), shockJSON(ev.TrueShock), shockJSON(ev.ReportShock)})
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"room_news"},
			[]string{"room_id", "day", "media_id", "headline", "track", "true_shock", "report_shock"},
			pgx.CopyFromRows(news))
		return err
	})
	if err != nil {
		return nil, err
	}
	return room, nil
}

func GetRoom(ctx context.Context, q Querier, id int64) (*Room, error) {
	return scanRoom(q.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE id = $1`, id))
}

func GetRoomByInvite(ctx context.Context, q Querier, code string) (*Room, error) {
	return scanRoom(q.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE invite_code = $1`, code))
}

// JoinRoom seats a user. Mid-game joiners start on the current day with
// full initial cash and a "late join" marker (spec §2.1 中途加入).
func JoinRoom(ctx context.Context, db Querier, room *Room, userID int64, now time.Time) (int, error) {
	joinedDay := 0
	if room.Status == "running" {
		day, ended, err := room.CurrentDay(now)
		if err != nil {
			return 0, err
		}
		if ended {
			return 0, ErrRoomEnded
		}
		joinedDay = day
	}
	_, err := db.Exec(ctx, `
		INSERT INTO room_players (room_id, user_id, cash_cents, joined_day)
		VALUES ($1, $2, $3, $4)`, room.ID, userID, InitialCashCents, joinedDay)
	if isUniqueViolation(err) {
		return 0, ErrAlreadyJoined
	}
	if err != nil {
		return 0, err
	}
	return joinedDay, nil
}

func StartRoom(ctx context.Context, q Querier, roomID, hostID int64, now time.Time) (*Room, error) {
	r, err := scanRoom(q.QueryRow(ctx, `
		UPDATE rooms SET status = 'running', started_at = $3
		WHERE id = $1 AND host_user_id = $2 AND status = 'lobby'
		RETURNING `+roomCols, roomID, hostID, now))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrCannotStart
	}
	return r, err
}

func ListRoomsForUser(ctx context.Context, q Querier, userID int64) ([]Room, error) {
	rows, err := q.Query(ctx, `
		SELECT `+roomCols+` FROM rooms
		WHERE id IN (SELECT room_id FROM room_players WHERE user_id = $1)
		ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Room
	for rows.Next() {
		r, err := scanRoom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func IsMember(ctx context.Context, q Querier, roomID, userID int64) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM room_players WHERE room_id = $1 AND user_id = $2)`,
		roomID, userID).Scan(&ok)
	return ok, err
}
```

Note: `pgx.Row` is the interface `{ Scan(dest ...any) error }` and `pgx.Rows` embeds it, so `scanRoom` accepts both a `QueryRow` result and an iterating `rows`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -count=1`
Expected: PASS.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./...
git add internal/store/rooms.go internal/store/rooms_test.go
git commit -m "feat(store): rooms with world generation, invites, deterministic clock"
```

---

### Task 6: Order placement and cancellation with freezing

**Files:**
- Create: `server/internal/store/orders.go`
- Test: `server/internal/store/orders_test.go`

**Interfaces:**
- Consumes: Tasks 1–5 (`Room`, `Querier`, sentinel errors, `mkUser`/`mkScenario` test helpers).
- Produces:
  - `type Order struct { ID int64; RoomID int64; UserID int64; InstrumentID string; Side string; AmountCents int64; Shares float64; ExecDay int; Status string }`
  - `type OrderReq struct { InstrumentID string; Side string; AmountCents int64; Shares float64 }`
  - `func PlaceOrder(ctx context.Context, db *pgxpool.Pool, room *Room, userID int64, now time.Time, req OrderReq) (*Order, error)` — freezes cash (buy) or shares (sell) atomically; `exec_day = current day + 1`
  - `func CancelOrder(ctx context.Context, db *pgxpool.Pool, room *Room, userID, orderID int64, now time.Time) error` — refunds the frozen side; `ErrNotCancellable` if not pending/yours
  - (Task 7 will prepend `SettleTx` inside both transactions — the modification point is marked with a comment.)

- [ ] **Step 1: Write the failing test**

`server/internal/store/orders_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// mkRunningRoom creates a started room with a host and one guest and
// returns (room, guest, start time). 60s per historical day.
func mkRunningRoom(t *testing.T, pool *pgxpool.Pool) (*Room, *User, time.Time) {
	t.Helper()
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	guest := mkUser(t, pool, "guest")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := JoinRoom(ctx, pool, room, guest.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Truncate(time.Second)
	room, err = StartRoom(ctx, pool, room.ID, host.ID, t0)
	if err != nil {
		t.Fatal(err)
	}
	return room, guest, t0
}

func cashOf(t *testing.T, pool *pgxpool.Pool, roomID, userID int64) int64 {
	t.Helper()
	var cash int64
	if err := pool.QueryRow(context.Background(),
		`SELECT cash_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		roomID, userID).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	return cash
}

func TestPlaceBuyFreezesCash(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	o, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 4_000_000})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if o.ExecDay != 1 || o.Status != "pending" {
		t.Fatalf("order: %+v", o)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != 6_000_000 {
		t.Fatalf("cash after freeze = %d, want 6000000", cash)
	}

	// Cannot overspend what is left.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 7_000_000}); !errors.Is(err, ErrInsufficientCash) {
		t.Fatalf("overspend: %v, want ErrInsufficientCash", err)
	}

	// Cancel refunds in full.
	if err := CancelOrder(ctx, pool, room, guest.ID, o.ID, t0); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != InitialCashCents {
		t.Fatalf("cash after cancel = %d, want %d", cash, InitialCashCents)
	}
	// A cancelled order cannot be cancelled again.
	if err := CancelOrder(ctx, pool, room, guest.ID, o.ID, t0); !errors.Is(err, ErrNotCancellable) {
		t.Fatalf("double cancel: %v, want ErrNotCancellable", err)
	}
}

func TestPlaceSellRequiresShares(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// No position yet.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "sell", Shares: 1}); !errors.Is(err, ErrInsufficientShares) {
		t.Fatalf("sell without position: %v, want ErrInsufficientShares", err)
	}

	// Seed a position directly, then a sell freezes it.
	if _, err := pool.Exec(ctx, `
		INSERT INTO positions (room_id, user_id, instrument_id, shares)
		VALUES ($1, $2, 'S1', 100)`, room.ID, guest.ID); err != nil {
		t.Fatal(err)
	}
	o, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "sell", Shares: 60})
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	var left float64
	if err := pool.QueryRow(ctx, `
		SELECT shares FROM positions WHERE room_id=$1 AND user_id=$2 AND instrument_id='S1'`,
		room.ID, guest.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 40 {
		t.Fatalf("frozen position = %v, want 40", left)
	}
	// Cancel restores the shares.
	if err := CancelOrder(ctx, pool, room, guest.ID, o.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT shares FROM positions WHERE room_id=$1 AND user_id=$2 AND instrument_id='S1'`,
		room.ID, guest.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 100 {
		t.Fatalf("restored position = %v, want 100", left)
	}
}

func TestPlaceOrderValidation(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	cases := []struct {
		req  OrderReq
		want error
	}{
		{OrderReq{InstrumentID: "NOPE", Side: "buy", AmountCents: 100}, ErrUnknownInstrument},
		{OrderReq{InstrumentID: "S1", Side: "hold", AmountCents: 100}, ErrBadOrder},
		{OrderReq{InstrumentID: "S1", Side: "buy", AmountCents: 0}, ErrBadOrder},
		{OrderReq{InstrumentID: "S1", Side: "buy", AmountCents: -5}, ErrBadOrder},
		{OrderReq{InstrumentID: "S1", Side: "sell", Shares: 0}, ErrBadOrder},
	}
	for _, tc := range cases {
		if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, tc.req); !errors.Is(err, tc.want) {
			t.Errorf("req %+v: got %v, want %v", tc.req, err, tc.want)
		}
	}

	// Orders on ended rooms are refused.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0.Add(400*60*time.Second), OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 100}); !errors.Is(err, ErrRoomEnded) {
		t.Errorf("ended room: %v, want ErrRoomEnded", err)
	}

	// Orders on lobby rooms are refused.
	host2 := mkUser(t, pool, "host2")
	sc := scenarioMustLoad(t, pool)
	lobby, err := CreateRoom(ctx, pool, sc, host2.ID, 60)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PlaceOrder(ctx, pool, lobby, host2.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 100}); !errors.Is(err, ErrRoomNotRunning) {
		t.Errorf("lobby room: %v, want ErrRoomNotRunning", err)
	}
}
```

Add this small helper at the bottom of `rooms_test.go` (it reuses the already-saved scenario instead of re-saving):

```go
func scenarioMustLoad(t *testing.T, pool *pgxpool.Pool) *scenario.Scenario {
	t.Helper()
	sc, err := LoadScenario(context.Background(), pool, "synthetic-v1")
	if err != nil {
		t.Fatalf("scenarioMustLoad: %v", err)
	}
	return sc
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -run 'TestPlace' -v`
Expected: FAIL (undefined: PlaceOrder).

- [ ] **Step 3: Write the implementation**

`server/internal/store/orders.go`:

```go
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Order struct {
	ID           int64
	RoomID       int64
	UserID       int64
	InstrumentID string
	Side         string // "buy" | "sell"
	AmountCents  int64  // buy: frozen cash to spend at next open
	Shares       float64 // sell: frozen shares to liquidate at next open
	ExecDay      int
	Status       string // "pending" | "filled" | "cancelled"
}

type OrderReq struct {
	InstrumentID string
	Side         string
	AmountCents  int64
	Shares       float64
}

// PlaceOrder freezes the paying side immediately (spec §2.2 下单即冻结)
// and schedules execution at the next day's open. Placement is refused
// once the game has ended.
func PlaceOrder(ctx context.Context, db *pgxpool.Pool, room *Room, userID int64, now time.Time, req OrderReq) (*Order, error) {
	if room.Status != "running" {
		return nil, ErrRoomNotRunning
	}
	curDay, ended, err := room.CurrentDay(now)
	if err != nil {
		return nil, err
	}
	if ended {
		return nil, ErrRoomEnded
	}
	if req.Side != "buy" && req.Side != "sell" {
		return nil, ErrBadOrder
	}
	if req.Side == "buy" && req.AmountCents <= 0 {
		return nil, ErrBadOrder
	}
	if req.Side == "sell" && req.Shares <= 0 {
		return nil, ErrBadOrder
	}

	var out *Order
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// Task 7 prepends: if err := SettleTx(ctx, tx, room, curDay, false); err != nil { return err }
		var one int
		err := tx.QueryRow(ctx, `
			SELECT 1 FROM room_prices
			WHERE room_id = $1 AND instrument_id = $2 AND day = 0`,
			room.ID, req.InstrumentID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownInstrument
		}
		if err != nil {
			return err
		}

		switch req.Side {
		case "buy":
			tag, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents - $1
				WHERE room_id = $2 AND user_id = $3 AND cash_cents >= $1`,
				req.AmountCents, room.ID, userID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrInsufficientCash
			}
			req.Shares = 0
		case "sell":
			tag, err := tx.Exec(ctx, `
				UPDATE positions SET shares = shares - $1
				WHERE room_id = $2 AND user_id = $3 AND instrument_id = $4 AND shares >= $1`,
				req.Shares, room.ID, userID, req.InstrumentID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrInsufficientShares
			}
			req.AmountCents = 0
		}

		o := &Order{
			RoomID: room.ID, UserID: userID, InstrumentID: req.InstrumentID,
			Side: req.Side, AmountCents: req.AmountCents, Shares: req.Shares,
			ExecDay: curDay + 1, Status: "pending",
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO orders (room_id, user_id, instrument_id, side, amount_cents, shares, exec_day)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			o.RoomID, o.UserID, o.InstrumentID, o.Side, o.AmountCents, o.Shares, o.ExecDay).Scan(&o.ID); err != nil {
			return err
		}
		out = o
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CancelOrder cancels a still-pending order and returns the frozen side.
// Once the exec day has been settled the order is filled and no longer
// cancellable (spec §2.2 成交日到来前可撤单).
func CancelOrder(ctx context.Context, db *pgxpool.Pool, room *Room, userID, orderID int64, now time.Time) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// Task 7 prepends lazy settlement here so due orders fill before
		// cancellation is judged:
		// if room.Status == "running" {
		//     if curDay, ended, err := room.CurrentDay(now); err == nil {
		//         if err := SettleTx(ctx, tx, room, curDay, ended); err != nil { return err }
		//     }
		// }
		var side, instrumentID string
		var amountCents int64
		var shares float64
		err := tx.QueryRow(ctx, `
			UPDATE orders SET status = 'cancelled'
			WHERE id = $1 AND room_id = $2 AND user_id = $3 AND status = 'pending'
			RETURNING side, instrument_id, amount_cents, shares`,
			orderID, room.ID, userID).Scan(&side, &instrumentID, &amountCents, &shares)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotCancellable
		}
		if err != nil {
			return err
		}
		if side == "buy" {
			_, err = tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents + $1
				WHERE room_id = $2 AND user_id = $3`, amountCents, room.ID, userID)
		} else {
			_, err = tx.Exec(ctx, `
				INSERT INTO positions (room_id, user_id, instrument_id, shares)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (room_id, user_id, instrument_id)
				DO UPDATE SET shares = positions.shares + EXCLUDED.shares`,
				room.ID, userID, instrumentID, shares)
		}
		return err
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -count=1`
Expected: PASS.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./...
git add internal/store/orders.go internal/store/orders_test.go internal/store/rooms_test.go
git commit -m "feat(store): order placement and cancellation with cash/share freezing"
```

---

### Task 7: Lazy settlement + whale events

**Files:**
- Create: `server/internal/store/settle.go`
- Modify: `server/internal/store/orders.go` (activate the two `SettleTx` call sites marked by Task 6's comments)
- Test: `server/internal/store/settle_test.go`

**Interfaces:**
- Consumes: Tasks 1–6.
- Produces:
  - `func SettleRoom(ctx context.Context, db *pgxpool.Pool, room *Room, now time.Time) (day int, ended bool, err error)` — the entry point every read path calls; no-op for lobby rooms (returns 0, false, nil)
  - `func SettleTx(ctx context.Context, tx pgx.Tx, room *Room, curDay int, ended bool) error` — fills due orders at exec-day open, records trades, updates positions/cash, emits anonymous whale events (kind `whale`, payload `{"instrument_id":…,"side":…}`), and refunds unfillable orders once the game has ended
  - `func assetsCents(ctx context.Context, q Querier, roomID, userID int64, day int, priceCol string) (int64, error)` — total assets = cash + positions + frozen buys + frozen sells, valued at `open` or `close` of `day` (whitelisted column name); reused by Task 8

- [ ] **Step 1: Write the failing test**

`server/internal/store/settle_test.go`:

```go
package store

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestSettleBuyAndSell(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Day 0: buy $40,000 of S1, executes at day 1 open.
	o, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 4_000_000})
	if err != nil {
		t.Fatal(err)
	}

	// Advance to day 1 and settle via a read path.
	at := t0.Add(61 * time.Second)
	day, ended, err := SettleRoom(ctx, pool, room, at)
	if err != nil || day != 1 || ended {
		t.Fatalf("SettleRoom: day=%d ended=%v err=%v", day, ended, err)
	}

	var open float64
	if err := pool.QueryRow(ctx, `
		SELECT open FROM room_prices WHERE room_id=$1 AND instrument_id='S1' AND day=1`,
		room.ID).Scan(&open); err != nil {
		t.Fatal(err)
	}
	wantShares := 40_000.0 / open // $40,000 at day-1 open

	var gotShares float64
	if err := pool.QueryRow(ctx, `
		SELECT shares FROM positions WHERE room_id=$1 AND user_id=$2 AND instrument_id='S1'`,
		room.ID, guest.ID).Scan(&gotShares); err != nil {
		t.Fatal(err)
	}
	if math.Abs(gotShares-wantShares) > 1e-9 {
		t.Fatalf("shares = %v, want %v", gotShares, wantShares)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1`, o.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "filled" {
		t.Fatalf("order status = %s, want filled", status)
	}
	var nTrades int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM trades WHERE room_id=$1`, room.ID).Scan(&nTrades); err != nil {
		t.Fatal(err)
	}
	if nTrades != 1 {
		t.Fatalf("trades = %d, want 1", nTrades)
	}

	// Settling again changes nothing (idempotence).
	if _, _, err := SettleRoom(ctx, pool, room, at); err != nil {
		t.Fatal(err)
	}
	var nTrades2 int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM trades WHERE room_id=$1`, room.ID).Scan(&nTrades2)
	if nTrades2 != 1 {
		t.Fatalf("trades after resettle = %d, want 1", nTrades2)
	}

	// Day 1: sell everything, executes at day 2 open.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, at, OrderReq{
		InstrumentID: "S1", Side: "sell", Shares: gotShares}); err != nil {
		t.Fatal(err)
	}
	at2 := t0.Add(121 * time.Second)
	if _, _, err := SettleRoom(ctx, pool, room, at2); err != nil {
		t.Fatal(err)
	}
	var open2 float64
	if err := pool.QueryRow(ctx, `
		SELECT open FROM room_prices WHERE room_id=$1 AND instrument_id='S1' AND day=2`,
		room.ID).Scan(&open2); err != nil {
		t.Fatal(err)
	}
	wantCash := 6_000_000 + int64(math.Round(gotShares*open2*100))
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != wantCash {
		t.Fatalf("cash after sell = %d, want %d", cash, wantCash)
	}
}

func TestWhaleEventThreshold(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// 90% of assets -> whale. 5% of assets -> silent.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 9_000_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S2", Side: "buy", AmountCents: 500_000}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}

	rows, err := pool.Query(ctx, `
		SELECT payload->>'instrument_id', payload->>'side'
		FROM room_events WHERE room_id=$1 AND kind='whale'`, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var whales [][2]string
	for rows.Next() {
		var inst, side string
		if err := rows.Scan(&inst, &side); err != nil {
			t.Fatal(err)
		}
		whales = append(whales, [2]string{inst, side})
	}
	if len(whales) != 1 || whales[0] != [2]string{"S1", "buy"} {
		t.Fatalf("whale events = %v, want exactly [S1 buy]", whales)
	}
}

func TestEndOfGameRefundsPendingOrders(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Place on the last day: exec_day == 300 which never comes.
	lastDay := t0.Add(299 * 60 * time.Second)
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, lastDay, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 2_000_000}); err != nil {
		t.Fatal(err)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != 8_000_000 {
		t.Fatalf("frozen cash = %d", cash)
	}

	// Past the end: settlement refunds instead of filling.
	day, ended, err := SettleRoom(ctx, pool, room, t0.Add(400*60*time.Second))
	if err != nil || day != 299 || !ended {
		t.Fatalf("SettleRoom at end: day=%d ended=%v err=%v", day, ended, err)
	}
	if cash := cashOf(t, pool, room.ID, guest.ID); cash != InitialCashCents {
		t.Fatalf("cash after refund = %d, want %d", cash, InitialCashCents)
	}
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM orders WHERE room_id=$1 ORDER BY id DESC LIMIT 1`, room.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" {
		t.Fatalf("order status = %s, want cancelled", status)
	}
}

func TestPlaceOrderSettlesFirst(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Sell needs the shares a due-but-unsettled buy will deliver:
	// placement itself must settle first for this to succeed.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 4_000_000}); err != nil {
		t.Fatal(err)
	}
	at := t0.Add(61 * time.Second) // buy is now due but nothing has settled it yet
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, at, OrderReq{
		InstrumentID: "S1", Side: "sell", Shares: 1}); err != nil {
		t.Fatalf("sell after due buy: %v (PlaceOrder must run SettleTx first)", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -run 'TestSettle|TestWhale|TestEndOfGame|TestPlaceOrderSettles' -v`
Expected: FAIL (undefined: SettleRoom).

- [ ] **Step 3: Write the implementation**

`server/internal/store/settle.go`:

```go
package store

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SettleRoom lazily settles all due orders, then reports the current day.
// Every read path (room state, portfolio, leaderboard, events) calls this
// first, so whoever arrives first triggers settlement and everyone sees
// the same result (spec §5.4). Lobby rooms are a no-op.
func SettleRoom(ctx context.Context, db *pgxpool.Pool, room *Room, now time.Time) (int, bool, error) {
	if room.StartedAt == nil {
		return 0, false, nil
	}
	day, ended, err := room.CurrentDay(now)
	if err != nil {
		return 0, false, err
	}
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		return SettleTx(ctx, tx, room, day, ended)
	})
	return day, ended, err
}

// SettleTx fills every pending order whose exec day has arrived, at that
// day's open. Concurrency-safe: FOR UPDATE serializes settlers, and under
// READ COMMITTED a blocked competitor re-evaluates the WHERE clause after
// the lock is released, so orders another transaction just filled drop
// out of its result set — settlement is idempotent.
func SettleTx(ctx context.Context, tx pgx.Tx, room *Room, curDay int, ended bool) error {
	type due struct {
		id           int64
		userID       int64
		instrumentID string
		side         string
		amountCents  int64
		shares       float64
		execDay      int
	}
	collect := func(rows pgx.Rows) ([]due, error) {
		defer rows.Close()
		var out []due
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.id, &d.userID, &d.instrumentID, &d.side,
				&d.amountCents, &d.shares, &d.execDay); err != nil {
				return nil, err
			}
			out = append(out, d)
		}
		return out, rows.Err()
	}

	rows, err := tx.Query(ctx, `
		SELECT id, user_id, instrument_id, side, amount_cents, shares, exec_day
		FROM orders
		WHERE room_id = $1 AND status = 'pending' AND exec_day <= $2
		ORDER BY exec_day, id
		FOR UPDATE`, room.ID, curDay)
	if err != nil {
		return err
	}
	dueOrders, err := collect(rows)
	if err != nil {
		return err
	}

	for _, o := range dueOrders {
		var open float64
		if err := tx.QueryRow(ctx, `
			SELECT open FROM room_prices
			WHERE room_id = $1 AND instrument_id = $2 AND day = $3`,
			room.ID, o.instrumentID, o.execDay).Scan(&open); err != nil {
			return fmt.Errorf("settle order %d (day %d): %w", o.id, o.execDay, err)
		}
		var tradeShares float64
		var tradeCents int64
		if o.side == "buy" {
			tradeShares = float64(o.amountCents) / 100 / open
			tradeCents = o.amountCents
			if _, err := tx.Exec(ctx, `
				INSERT INTO positions (room_id, user_id, instrument_id, shares)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (room_id, user_id, instrument_id)
				DO UPDATE SET shares = positions.shares + EXCLUDED.shares`,
				room.ID, o.userID, o.instrumentID, tradeShares); err != nil {
				return err
			}
		} else {
			tradeShares = o.shares
			tradeCents = int64(math.Round(o.shares * open * 100))
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents + $1
				WHERE room_id = $2 AND user_id = $3`,
				tradeCents, room.ID, o.userID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO trades (order_id, room_id, user_id, instrument_id, side, day, price, shares, amount_cents)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			o.id, room.ID, o.userID, o.instrumentID, o.side, o.execDay,
			open, tradeShares, tradeCents); err != nil {
			return err
		}
		// Mark filled BEFORE the whale valuation so the order no longer
		// counts as frozen in assetsCents (no double counting).
		if _, err := tx.Exec(ctx, `UPDATE orders SET status = 'filled' WHERE id = $1`, o.id); err != nil {
			return err
		}
		assets, err := assetsCents(ctx, tx, room.ID, o.userID, o.execDay, "open")
		if err != nil {
			return err
		}
		// Whale alert: trade ≥ 20% of the player's total assets (spec §2.3).
		if assets > 0 && tradeCents*5 >= assets {
			if _, err := tx.Exec(ctx, `
				INSERT INTO room_events (room_id, day, kind, payload)
				VALUES ($1, $2, 'whale', jsonb_build_object('instrument_id', $3::text, 'side', $4::text))`,
				room.ID, o.execDay, o.instrumentID, o.side); err != nil {
				return err
			}
		}
	}

	if !ended {
		return nil
	}
	// Game over: whatever is still pending can never execute — refund it.
	rows, err = tx.Query(ctx, `
		SELECT id, user_id, instrument_id, side, amount_cents, shares, exec_day
		FROM orders
		WHERE room_id = $1 AND status = 'pending'
		ORDER BY id
		FOR UPDATE`, room.ID)
	if err != nil {
		return err
	}
	leftovers, err := collect(rows)
	if err != nil {
		return err
	}
	for _, o := range leftovers {
		if o.side == "buy" {
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET cash_cents = cash_cents + $1
				WHERE room_id = $2 AND user_id = $3`,
				o.amountCents, room.ID, o.userID); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `
				INSERT INTO positions (room_id, user_id, instrument_id, shares)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (room_id, user_id, instrument_id)
				DO UPDATE SET shares = positions.shares + EXCLUDED.shares`,
				room.ID, o.userID, o.instrumentID, o.shares); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET status = 'cancelled' WHERE id = $1`, o.id); err != nil {
			return err
		}
	}
	return nil
}

// priceCols whitelists the interpolated column name in assetsCents.
var priceCols = map[string]bool{"open": true, "close": true}

// assetsCents values a player's total assets at the given day: cash +
// positions + frozen buy cash + frozen sell shares, using the day's open
// or close. Frozen amounts count — freezing must not dent the leaderboard.
func assetsCents(ctx context.Context, q Querier, roomID, userID int64, day int, priceCol string) (int64, error) {
	if !priceCols[priceCol] {
		return 0, fmt.Errorf("assetsCents: bad price column %q", priceCol)
	}
	var cash int64
	if err := q.QueryRow(ctx,
		`SELECT cash_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		roomID, userID).Scan(&cash); err != nil {
		return 0, err
	}
	var posVal float64
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(SUM(p.shares * rp.%s), 0)
		FROM positions p
		JOIN room_prices rp ON rp.room_id = p.room_id
			AND rp.instrument_id = p.instrument_id AND rp.day = $3
		WHERE p.room_id = $1 AND p.user_id = $2`, priceCol),
		roomID, userID, day).Scan(&posVal); err != nil {
		return 0, err
	}
	var frozenBuy int64
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_cents), 0) FROM orders
		WHERE room_id = $1 AND user_id = $2 AND status = 'pending' AND side = 'buy'`,
		roomID, userID).Scan(&frozenBuy); err != nil {
		return 0, err
	}
	var frozenSellVal float64
	if err := q.QueryRow(ctx, fmt.Sprintf(`
		SELECT COALESCE(SUM(o.shares * rp.%s), 0)
		FROM orders o
		JOIN room_prices rp ON rp.room_id = o.room_id
			AND rp.instrument_id = o.instrument_id AND rp.day = $3
		WHERE o.room_id = $1 AND o.user_id = $2 AND o.status = 'pending' AND o.side = 'sell'`, priceCol),
		roomID, userID, day).Scan(&frozenSellVal); err != nil {
		return 0, err
	}
	return cash + frozenBuy + int64(math.Round((posVal+frozenSellVal)*100)), nil
}
```

Then activate the two marked call sites in `orders.go`:

In `PlaceOrder`, replace the comment line with:

```go
		if err := SettleTx(ctx, tx, room, curDay, false); err != nil {
			return err
		}
```

In `CancelOrder`, replace the comment block with:

```go
		if room.Status == "running" {
			curDay, ended, err := room.CurrentDay(now)
			if err != nil {
				return err
			}
			if err := SettleTx(ctx, tx, room, curDay, ended); err != nil {
				return err
			}
		}
```

- [ ] **Step 4: Run the full store suite**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -count=1`
Expected: PASS — including the Task 6 tests, which must still pass with settlement prepended.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./...
git add internal/store/settle.go internal/store/settle_test.go internal/store/orders.go
git commit -m "feat(store): lazy settlement with whale events and end-of-game refunds"
```

---
### Task 8: Portfolio and leaderboard valuation

**Files:**
- Create: `server/internal/store/portfolio.go`
- Test: `server/internal/store/portfolio_test.go`

**Interfaces:**
- Consumes: Tasks 1–7 (`assetsCents`, `mkRunningRoom`, `cashOf`).
- Produces:
  - `type PortfolioPosition struct { InstrumentID string; Shares float64; Close float64; ValueCents int64 }`
  - `type PendingOrder struct { ID int64; InstrumentID string; Side string; AmountCents int64; Shares float64; ExecDay int }`
  - `type Portfolio struct { CashCents int64; TotalCents int64; Positions []PortfolioPosition; Pending []PendingOrder }`
  - `func GetPortfolio(ctx context.Context, q Querier, room *Room, userID int64, curDay int) (*Portfolio, error)` — valued at `curDay` close; `TotalCents` includes frozen buy cash and frozen sell shares
  - `type LeaderboardRow struct { UserID int64; Username string; TotalCents int64; JoinedDay int }`
  - `func Leaderboard(ctx context.Context, q Querier, room *Room, curDay int) ([]LeaderboardRow, error)` — sorted by `TotalCents` desc, then `Username` asc

- [ ] **Step 1: Write the failing test**

`server/internal/store/portfolio_test.go`:

```go
package store

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestPortfolioValuation(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// Frozen-but-unsettled money still counts: total == initial exactly.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 4_000_000}); err != nil {
		t.Fatal(err)
	}
	p, err := GetPortfolio(ctx, pool, room, guest.ID, 0)
	if err != nil {
		t.Fatalf("GetPortfolio: %v", err)
	}
	if p.CashCents != 6_000_000 || p.TotalCents != InitialCashCents {
		t.Fatalf("frozen portfolio: cash=%d total=%d, want 6000000 / %d",
			p.CashCents, p.TotalCents, InitialCashCents)
	}
	if len(p.Pending) != 1 || p.Pending[0].Side != "buy" || p.Pending[0].ExecDay != 1 {
		t.Fatalf("pending: %+v", p.Pending)
	}

	// After settlement the position is valued at the current day's close.
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}
	p, err = GetPortfolio(ctx, pool, room, guest.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Positions) != 1 || p.Positions[0].InstrumentID != "S1" {
		t.Fatalf("positions: %+v", p.Positions)
	}
	var open1, close1 float64
	if err := pool.QueryRow(ctx, `
		SELECT open, close FROM room_prices
		WHERE room_id=$1 AND instrument_id='S1' AND day=1`, room.ID).Scan(&open1, &close1); err != nil {
		t.Fatal(err)
	}
	wantShares := 40_000.0 / open1
	if math.Abs(p.Positions[0].Shares-wantShares) > 1e-9 || p.Positions[0].Close != close1 {
		t.Fatalf("position detail: %+v, want shares %v close %v", p.Positions[0], wantShares, close1)
	}
	wantTotal := 6_000_000 + int64(math.Round(wantShares*close1*100))
	if p.TotalCents != wantTotal {
		t.Fatalf("total = %d, want %d", p.TotalCents, wantTotal)
	}
}

func TestLeaderboardOrdering(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	// guest goes all-in on S1 at day-1 open; host stays in cash.
	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: InitialCashCents}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}
	rows, err := Leaderboard(ctx, pool, room, 1)
	if err != nil {
		t.Fatalf("Leaderboard: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("leaderboard rows = %d, want 2", len(rows))
	}
	// S1 is the tech-bubble instrument: its day-1 close is above its
	// day-1 open in the synthetic scenario's boom phase, so the all-in
	// player leads; regardless, ordering must match the totals.
	if rows[0].TotalCents < rows[1].TotalCents {
		t.Fatalf("leaderboard not sorted desc: %+v", rows)
	}
	for _, r := range rows {
		if r.Username == "" {
			t.Fatalf("missing username: %+v", r)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -run 'TestPortfolio|TestLeaderboard' -v`
Expected: FAIL (undefined: GetPortfolio).

- [ ] **Step 3: Write the implementation**

`server/internal/store/portfolio.go`:

```go
package store

import (
	"context"
	"math"
	"sort"
)

type PortfolioPosition struct {
	InstrumentID string
	Shares       float64
	Close        float64
	ValueCents   int64
}

type PendingOrder struct {
	ID           int64
	InstrumentID string
	Side         string
	AmountCents  int64
	Shares       float64
	ExecDay      int
}

type Portfolio struct {
	CashCents  int64
	TotalCents int64
	Positions  []PortfolioPosition
	Pending    []PendingOrder
}

// GetPortfolio values a player's holdings at curDay's close. TotalCents
// counts frozen order money too, so placing an order never dents totals.
func GetPortfolio(ctx context.Context, q Querier, room *Room, userID int64, curDay int) (*Portfolio, error) {
	p := &Portfolio{}
	if err := q.QueryRow(ctx,
		`SELECT cash_cents FROM room_players WHERE room_id = $1 AND user_id = $2`,
		room.ID, userID).Scan(&p.CashCents); err != nil {
		return nil, err
	}

	posRows, err := q.Query(ctx, `
		SELECT p.instrument_id, p.shares, rp.close
		FROM positions p
		JOIN room_prices rp ON rp.room_id = p.room_id
			AND rp.instrument_id = p.instrument_id AND rp.day = $3
		WHERE p.room_id = $1 AND p.user_id = $2 AND p.shares > 0
		ORDER BY p.instrument_id`, room.ID, userID, curDay)
	if err != nil {
		return nil, err
	}
	defer posRows.Close()
	for posRows.Next() {
		var pos PortfolioPosition
		if err := posRows.Scan(&pos.InstrumentID, &pos.Shares, &pos.Close); err != nil {
			return nil, err
		}
		pos.ValueCents = int64(math.Round(pos.Shares * pos.Close * 100))
		p.Positions = append(p.Positions, pos)
	}
	if err := posRows.Err(); err != nil {
		return nil, err
	}

	ordRows, err := q.Query(ctx, `
		SELECT id, instrument_id, side, amount_cents, shares, exec_day
		FROM orders
		WHERE room_id = $1 AND user_id = $2 AND status = 'pending'
		ORDER BY id`, room.ID, userID)
	if err != nil {
		return nil, err
	}
	defer ordRows.Close()
	for ordRows.Next() {
		var o PendingOrder
		if err := ordRows.Scan(&o.ID, &o.InstrumentID, &o.Side, &o.AmountCents, &o.Shares, &o.ExecDay); err != nil {
			return nil, err
		}
		p.Pending = append(p.Pending, o)
	}
	if err := ordRows.Err(); err != nil {
		return nil, err
	}

	total, err := assetsCents(ctx, q, room.ID, userID, curDay, "close")
	if err != nil {
		return nil, err
	}
	p.TotalCents = total
	return p, nil
}

type LeaderboardRow struct {
	UserID     int64
	Username   string
	TotalCents int64
	JoinedDay  int
}

// Leaderboard values every player at curDay's close. Holdings stay
// hidden (spec §2.2): only totals are exposed.
func Leaderboard(ctx context.Context, q Querier, room *Room, curDay int) ([]LeaderboardRow, error) {
	rows, err := q.Query(ctx, `
		SELECT rp.user_id, u.username, rp.joined_day
		FROM room_players rp JOIN users u ON u.id = rp.user_id
		WHERE rp.room_id = $1`, room.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeaderboardRow
	for rows.Next() {
		var r LeaderboardRow
		if err := rows.Scan(&r.UserID, &r.Username, &r.JoinedDay); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		total, err := assetsCents(ctx, q, room.ID, out[i].UserID, curDay, "close")
		if err != nil {
			return nil, err
		}
		out[i].TotalCents = total
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalCents != out[j].TotalCents {
			return out[i].TotalCents > out[j].TotalCents
		}
		return out[i].Username < out[j].Username
	})
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -count=1`
Expected: PASS.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./...
git add internal/store/portfolio.go internal/store/portfolio_test.go
git commit -m "feat(store): portfolio and leaderboard valuation"
```

---

### Task 9: Room HTTP API (lifecycle, state, prices, news, events)

**Files:**
- Create: `server/internal/httpapi/rooms.go`
- Modify: `server/internal/httpapi/server.go` (register room routes inside the authed group)
- Test: `server/internal/httpapi/rooms_test.go`

**Interfaces:**
- Consumes: Tasks 3–8 store functions; test helpers from Task 3.
- Produces endpoints (all under `requireAuth`):
  - `POST /api/rooms` `{"scenario_id","day_duration_secs"}` → room JSON
  - `POST /api/rooms/join` `{"invite_code"}` → room JSON
  - `POST /api/rooms/{roomID}/start` → room JSON (host only)
  - `GET /api/rooms` → `{"rooms":[…]}` (my rooms)
  - `GET /api/rooms/{roomID}` → `{"room":…,"instruments":[…],"quotes":[…],"leaderboard":[…]}`
  - `GET /api/rooms/{roomID}/prices/{instrumentID}` → `{"days":[{open,high,low,close}…]}` truncated at current day
  - `GET /api/rooms/{roomID}/news?after=N` → `{"items":[{id,day,media_id,headline}…]}` (max 200; NEVER track/shocks — blind box)
  - `GET /api/rooms/{roomID}/events?after=N` → `{"items":[{id,day,kind,payload}…]}`
  - Helper `func (s *Server) roomForMember(w http.ResponseWriter, r *http.Request) (*store.Room, bool)` — 404 unknown room, 403 non-member; reused by Tasks 10–11
  - Helper `func roomJSON(room *store.Room, curDay int, ended, started bool) map[string]any`
  - Room JSON keys: `id, invite_code, scenario_id, days, status, day_duration_secs, started_at?, current_day?, ended?`
  - Leaderboard JSON keys: `username, total_cents, return_pct, late_join` (no user_id, no holdings — spec §2.2)

- [ ] **Step 1: Write the failing test**

`server/internal/httpapi/rooms_test.go`:

```go
package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/toddzheng/stocker/server/internal/scenario"
	"github.com/toddzheng/stocker/server/internal/store"
)

func seedScenario(t *testing.T, s *Server) *scenario.Scenario {
	t.Helper()
	sc := scenario.Synthetic()
	if err := store.SaveScenario(context.Background(), s.DB, sc); err != nil {
		t.Fatalf("seed scenario: %v", err)
	}
	return sc
}

// fakeClock pins the server's clock and returns a function to move it.
func fakeClock(s *Server, start time.Time) func(d time.Duration) {
	now := start
	s.Now = func() time.Time { return now }
	return func(d time.Duration) { now = now.Add(d) }
}

func TestRoomLifecycleAndState(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	guest := registerClient(t, s, "guest")
	outsider := registerClient(t, s, "outsider")

	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	invite := created["invite_code"].(string)
	if created["status"] != "lobby" || invite == "" {
		t.Fatalf("created room: %v", created)
	}

	// Bad scenario / bad duration.
	resp, _ := host.do("POST", "/api/rooms", map[string]any{"scenario_id": "nope", "day_duration_secs": 60})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown scenario: %d", resp.StatusCode)
	}
	resp, _ = host.do("POST", "/api/rooms", map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 5})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad duration: %d", resp.StatusCode)
	}

	// Join by invite; non-members are locked out of room reads.
	guest.mustJSON("POST", "/api/rooms/join", map[string]any{"invite_code": invite}, http.StatusOK)
	resp, _ = outsider.do("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider room read: %d", resp.StatusCode)
	}

	// Guest cannot start; host can.
	resp, _ = guest.do("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("guest start: %d", resp.StatusCode)
	}
	started := host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	if started["status"] != "running" {
		t.Fatalf("started room: %v", started)
	}

	// Advance two historical days; state reflects the deterministic clock.
	advance(2*60*time.Second + time.Second)
	state := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil, http.StatusOK)
	room := state["room"].(map[string]any)
	if room["current_day"].(float64) != 2 || room["ended"].(bool) {
		t.Fatalf("room state: %v", room)
	}
	instruments := state["instruments"].([]any)
	if len(instruments) != 8 {
		t.Fatalf("instruments: %d, want 8", len(instruments))
	}
	quotes := state["quotes"].([]any)
	if len(quotes) != 8 {
		t.Fatalf("quotes: %d, want 8", len(quotes))
	}
	lb := state["leaderboard"].([]any)
	if len(lb) != 2 {
		t.Fatalf("leaderboard: %v", lb)
	}
	row := lb[0].(map[string]any)
	if _, hasUID := row["user_id"]; hasUID {
		t.Fatalf("leaderboard leaks user_id: %v", row)
	}

	// Price series is truncated at the current day (no future peeking).
	prices := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/prices/S1", roomID), nil, http.StatusOK)
	if n := len(prices["days"].([]any)); n != 3 {
		t.Fatalf("price days = %d, want 3 (day 0..2)", n)
	}
	resp, _ = guest.do("GET", fmt.Sprintf("/api/rooms/%d/prices/NOPE", roomID), nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown instrument prices: %d", resp.StatusCode)
	}

	// My rooms.
	mine := guest.mustJSON("GET", "/api/rooms", nil, http.StatusOK)
	if n := len(mine["rooms"].([]any)); n != 1 {
		t.Fatalf("my rooms = %d, want 1", n)
	}
}

func TestNewsIsBlindBoxSafe(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	advance(10*60*time.Second + time.Second) // day 10

	resp, body := host.do("GET", fmt.Sprintf("/api/rooms/%d/news?after=0", roomID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("news: %d %s", resp.StatusCode, body)
	}
	raw := string(body)
	for _, leak := range []string{`"track"`, `"true_shock"`, `"report_shock"`, `"real_name"`} {
		if strings.Contains(raw, leak) {
			t.Fatalf("news response leaks %s: %s", leak, raw[:min(400, len(raw))])
		}
	}

	news := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/news?after=0", roomID), nil, http.StatusOK)
	items := news["items"].([]any)
	if len(items) == 0 {
		t.Fatal("no news items by day 10")
	}
	maxID := 0.0
	for _, it := range items {
		m := it.(map[string]any)
		if d := m["day"].(float64); d > 10 {
			t.Fatalf("future news leaked: day %v", d)
		}
		if m["headline"].(string) == "" || m["media_id"].(string) == "" {
			t.Fatalf("bad news item: %v", m)
		}
		maxID = max(maxID, m["id"].(float64))
	}

	// Incremental fetch returns nothing new.
	again := host.mustJSON("GET",
		fmt.Sprintf("/api/rooms/%d/news?after=%d", roomID, int(maxID)), nil, http.StatusOK)
	if n := len(again["items"].([]any)); n != 0 {
		t.Fatalf("incremental fetch returned %d items, want 0", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/httpapi/ -run TestRoom -v`
Expected: FAIL (404s — routes not registered).

- [ ] **Step 3: Write the implementation**

In `server.go`, replace the placeholder comment inside the authed group with:

```go
		r.Post("/api/rooms", s.handleCreateRoom)
		r.Post("/api/rooms/join", s.handleJoinRoom)
		r.Get("/api/rooms", s.handleMyRooms)
		r.Post("/api/rooms/{roomID}/start", s.handleStartRoom)
		r.Get("/api/rooms/{roomID}", s.handleRoomState)
		r.Get("/api/rooms/{roomID}/prices/{instrumentID}", s.handlePrices)
		r.Get("/api/rooms/{roomID}/news", s.handleNews)
		r.Get("/api/rooms/{roomID}/events", s.handleEvents)
		// Trading and reveal routes are registered here by later tasks.
```

`server/internal/httpapi/rooms.go`:

```go
package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/toddzheng/stocker/server/internal/store"
)

const newsPageLimit = 200

func roomJSON(room *store.Room, curDay int, ended, started bool) map[string]any {
	m := map[string]any{
		"id":                room.ID,
		"invite_code":       room.InviteCode,
		"scenario_id":       room.ScenarioID,
		"days":              room.Days,
		"status":            room.Status,
		"day_duration_secs": room.DayDurationSecs,
	}
	if room.StartedAt != nil {
		m["started_at"] = room.StartedAt.UTC().Format(time.RFC3339)
	}
	if started {
		m["current_day"] = curDay
		m["ended"] = ended
	}
	return m
}

// roomForMember loads the {roomID} route param and enforces membership.
func (s *Server) roomForMember(w http.ResponseWriter, r *http.Request) (*store.Room, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "roomID"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such room")
		return nil, false
	}
	room, err := store.GetRoom(r.Context(), s.DB, id)
	if err != nil {
		s.storeErr(w, err)
		return nil, false
	}
	member, err := store.IsMember(r.Context(), s.DB, room.ID, userFrom(r).ID)
	if err != nil {
		s.storeErr(w, err)
		return nil, false
	}
	if !member {
		writeErr(w, http.StatusForbidden, "not a member of this room")
		return nil, false
	}
	return room, true
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScenarioID      string `json:"scenario_id"`
		DayDurationSecs int    `json:"day_duration_secs"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sc, err := store.LoadScenario(r.Context(), s.DB, req.ScenarioID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	room, err := store.CreateRoom(r.Context(), s.DB, sc, userFrom(r).ID, req.DayDurationSecs)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roomJSON(room, 0, false, false))
}

func (s *Server) handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InviteCode string `json:"invite_code"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	room, err := store.GetRoomByInvite(r.Context(), s.DB, req.InviteCode)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if _, err := store.JoinRoom(r.Context(), s.DB, room, userFrom(r).ID, s.Now()); err != nil {
		s.storeErr(w, err)
		return
	}
	day, ended, started := s.roomProgress(room)
	writeJSON(w, http.StatusOK, roomJSON(room, day, ended, started))
}

// roomProgress computes clock state without settling (cheap display path).
func (s *Server) roomProgress(room *store.Room) (day int, ended, started bool) {
	if room.StartedAt == nil {
		return 0, false, false
	}
	day, ended, err := room.CurrentDay(s.Now())
	if err != nil {
		return 0, false, false
	}
	return day, ended, true
}

func (s *Server) handleStartRoom(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	room, err := store.StartRoom(r.Context(), s.DB, room.ID, userFrom(r).ID, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roomJSON(room, 0, false, true))
}

func (s *Server) handleMyRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := store.ListRoomsForUser(r.Context(), s.DB, userFrom(r).ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rooms))
	for i := range rooms {
		day, ended, started := s.roomProgress(&rooms[i])
		out = append(out, roomJSON(&rooms[i], day, ended, started))
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": out})
}

func (s *Server) handleRoomState(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	curDay, ended, err := store.SettleRoom(r.Context(), s.DB, room, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	started := room.StartedAt != nil

	// Blind-box instrument cards: alias + description only.
	instRows, err := s.DB.Query(r.Context(), `
		SELECT i.id, i.alias, i.descr
		FROM instruments i JOIN rooms rm ON rm.scenario_id = i.scenario_id
		WHERE rm.id = $1 ORDER BY i.ord`, room.ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer instRows.Close()
	instruments := []map[string]any{}
	for instRows.Next() {
		var id, alias, desc string
		if err := instRows.Scan(&id, &alias, &desc); err != nil {
			s.storeErr(w, err)
			return
		}
		instruments = append(instruments, map[string]any{"id": id, "alias": alias, "desc": desc})
	}
	if err := instRows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}

	quotes := []map[string]any{}
	if started {
		prevDay := curDay - 1
		if prevDay < 0 {
			prevDay = 0
		}
		quoteRows, err := s.DB.Query(r.Context(), `
			SELECT cur.instrument_id, cur.close, prev.close
			FROM room_prices cur
			JOIN room_prices prev ON prev.room_id = cur.room_id
				AND prev.instrument_id = cur.instrument_id AND prev.day = $3
			WHERE cur.room_id = $1 AND cur.day = $2
			ORDER BY cur.instrument_id`, room.ID, curDay, prevDay)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		defer quoteRows.Close()
		for quoteRows.Next() {
			var id string
			var cl, prev float64
			if err := quoteRows.Scan(&id, &cl, &prev); err != nil {
				s.storeErr(w, err)
				return
			}
			quotes = append(quotes, map[string]any{"instrument_id": id, "close": cl, "prev_close": prev})
		}
		if err := quoteRows.Err(); err != nil {
			s.storeErr(w, err)
			return
		}
	}

	leaderboard := []map[string]any{}
	if started {
		rows, err := store.Leaderboard(r.Context(), s.DB, room, curDay)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		for _, lr := range rows {
			leaderboard = append(leaderboard, map[string]any{
				"username":    lr.Username,
				"total_cents": lr.TotalCents,
				"return_pct":  float64(lr.TotalCents-store.InitialCashCents) / float64(store.InitialCashCents),
				"late_join":   lr.JoinedDay > 0,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"room":        roomJSON(room, curDay, ended, started),
		"instruments": instruments,
		"quotes":      quotes,
		"leaderboard": leaderboard,
	})
}

func (s *Server) handlePrices(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt == nil {
		writeErr(w, http.StatusBadRequest, store.ErrNotStarted.Error())
		return
	}
	curDay, _, err := room.CurrentDay(s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	instrumentID := chi.URLParam(r, "instrumentID")
	rows, err := s.DB.Query(r.Context(), `
		SELECT open, high, low, close FROM room_prices
		WHERE room_id = $1 AND instrument_id = $2 AND day <= $3
		ORDER BY day`, room.ID, instrumentID, curDay)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer rows.Close()
	days := []map[string]float64{}
	for rows.Next() {
		var o, h, l, c float64
		if err := rows.Scan(&o, &h, &l, &c); err != nil {
			s.storeErr(w, err)
			return
		}
		days = append(days, map[string]float64{"open": o, "high": h, "low": l, "close": c})
	}
	if err := rows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}
	if len(days) == 0 {
		writeErr(w, http.StatusNotFound, "no such instrument")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days})
}

func afterParam(r *http.Request) int64 {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	return after
}

// handleNews serves the player-visible feed. Blind box: id, day, media,
// headline — nothing else. Track and shock vectors never leave the server.
func (s *Server) handleNews(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt == nil {
		writeErr(w, http.StatusBadRequest, store.ErrNotStarted.Error())
		return
	}
	curDay, _, err := room.CurrentDay(s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	rows, err := s.DB.Query(r.Context(), `
		SELECT id, day, media_id, headline FROM room_news
		WHERE room_id = $1 AND day <= $2 AND id > $3
		ORDER BY id LIMIT $4`, room.ID, curDay, afterParam(r), newsPageLimit)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var day int
		var mediaID, headline string
		if err := rows.Scan(&id, &day, &mediaID, &headline); err != nil {
			s.storeErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"id": id, "day": day, "media_id": mediaID, "headline": headline,
		})
	}
	if err := rows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if _, _, err := store.SettleRoom(r.Context(), s.DB, room, s.Now()); err != nil {
		s.storeErr(w, err)
		return
	}
	rows, err := s.DB.Query(r.Context(), `
		SELECT id, day, kind, payload FROM room_events
		WHERE room_id = $1 AND id > $2
		ORDER BY id LIMIT $3`, room.ID, afterParam(r), newsPageLimit)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var day int
		var kind string
		var payload map[string]any
		if err := rows.Scan(&id, &day, &kind, &payload); err != nil {
			s.storeErr(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "day": day, "kind": kind, "payload": payload})
	}
	if err := rows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/httpapi/ -count=1`
Expected: PASS.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./...
git add internal/httpapi/rooms.go internal/httpapi/rooms_test.go internal/httpapi/server.go
git commit -m "feat(api): room lifecycle, state, prices, news and events endpoints"
```

---

### Task 10: Trading HTTP API (orders, portfolio)

**Files:**
- Create: `server/internal/httpapi/trading.go`
- Modify: `server/internal/httpapi/server.go` (register trading routes)
- Test: `server/internal/httpapi/trading_test.go`

**Interfaces:**
- Consumes: Tasks 6–9 (`store.PlaceOrder`, `store.CancelOrder`, `store.GetPortfolio`, `store.SettleRoom`, `roomForMember`, test helpers).
- Produces endpoints:
  - `POST /api/rooms/{roomID}/orders` `{"instrument_id","side","amount_cents"?,"shares"?}` → `{id, instrument_id, side, amount_cents, shares, exec_day, status}`
  - `DELETE /api/rooms/{roomID}/orders/{orderID}` → `{"status":"cancelled"}`
  - `GET /api/rooms/{roomID}/portfolio` → `{cash_cents, total_cents, positions:[{instrument_id,shares,close,value_cents}], pending:[{id,instrument_id,side,amount_cents,shares,exec_day}]}`

- [ ] **Step 1: Write the failing test**

`server/internal/httpapi/trading_test.go`:

```go
package httpapi

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// End-to-end: register → create/join/start → trade → settle-on-read →
// whale alert → leaderboard, all through HTTP with a fake clock.
func TestTradingFlow(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	guest := registerClient(t, s, "guest")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	guest.mustJSON("POST", "/api/rooms/join",
		map[string]any{"invite_code": created["invite_code"]}, http.StatusOK)
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)

	ordersPath := fmt.Sprintf("/api/rooms/%d/orders", roomID)
	portfolioPath := fmt.Sprintf("/api/rooms/%d/portfolio", roomID)

	// Day 0: guest goes 90% into S1 — big enough to trip the whale alert.
	order := guest.mustJSON("POST", ordersPath, map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 9_000_000}, http.StatusOK)
	if order["exec_day"].(float64) != 1 || order["status"] != "pending" {
		t.Fatalf("order: %v", order)
	}

	// Insufficient cash through the API maps to 400.
	resp, _ := guest.do("POST", ordersPath, map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 5_000_000})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("overspend via API: %d", resp.StatusCode)
	}

	// Frozen but unsettled: total still equals initial cash.
	p := guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	if p["total_cents"].(float64) != 10_000_000 {
		t.Fatalf("frozen total: %v", p["total_cents"])
	}

	// Day 1: reading the portfolio settles the order.
	advance(61 * time.Second)
	p = guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	positions := p["positions"].([]any)
	if len(positions) != 1 {
		t.Fatalf("positions after settle: %v", positions)
	}
	pos := positions[0].(map[string]any)
	if pos["instrument_id"] != "S1" || pos["shares"].(float64) <= 0 {
		t.Fatalf("position: %v", pos)
	}

	// The anonymous whale alert is in the feed; payload names no player.
	events := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/events?after=0", roomID), nil, http.StatusOK)
	items := events["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("events: %v", items)
	}
	ev := items[0].(map[string]any)
	payload := ev["payload"].(map[string]any)
	if ev["kind"] != "whale" || payload["instrument_id"] != "S1" || payload["side"] != "buy" {
		t.Fatalf("whale event: %v", ev)
	}
	if _, leaked := payload["user_id"]; leaked {
		t.Fatalf("whale event leaks user: %v", payload)
	}

	// Cancel flow: place then cancel restores cash.
	o2 := guest.mustJSON("POST", ordersPath, map[string]any{
		"instrument_id": "S2", "side": "buy", "amount_cents": 200_000}, http.StatusOK)
	guest.mustJSON("DELETE", fmt.Sprintf("%s/%d", ordersPath, int64(o2["id"].(float64))), nil, http.StatusOK)
	p = guest.mustJSON("GET", portfolioPath, nil, http.StatusOK)
	if n := len(p["pending"].([]any)); n != 0 {
		t.Fatalf("pending after cancel: %d", n)
	}
	// Cancelling someone else's (or a filled) order is a 409.
	resp, _ = host.do("DELETE", fmt.Sprintf("%s/%d", ordersPath, int64(o2["id"].(float64))), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("cancel foreign order: %d", resp.StatusCode)
	}

	// Orders after game end are refused with 409.
	advance(400 * 60 * time.Second)
	resp, _ = guest.do("POST", ordersPath, map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 100})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("order after end: %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/httpapi/ -run TestTradingFlow -v`
Expected: FAIL (405/404 — routes missing).

- [ ] **Step 3: Write the implementation**

In `server.go`, replace the Task 9 placeholder comment with:

```go
		r.Post("/api/rooms/{roomID}/orders", s.handlePlaceOrder)
		r.Delete("/api/rooms/{roomID}/orders/{orderID}", s.handleCancelOrder)
		r.Get("/api/rooms/{roomID}/portfolio", s.handlePortfolio)
		// Reveal route is registered here by Task 11.
```

`server/internal/httpapi/trading.go`:

```go
package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/toddzheng/stocker/server/internal/store"
)

func (s *Server) handlePlaceOrder(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	var req struct {
		InstrumentID string  `json:"instrument_id"`
		Side         string  `json:"side"`
		AmountCents  int64   `json:"amount_cents"`
		Shares       float64 `json:"shares"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	order, err := store.PlaceOrder(r.Context(), s.DB, room, userFrom(r).ID, s.Now(), store.OrderReq{
		InstrumentID: req.InstrumentID, Side: req.Side,
		AmountCents: req.AmountCents, Shares: req.Shares,
	})
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": order.ID, "instrument_id": order.InstrumentID, "side": order.Side,
		"amount_cents": order.AmountCents, "shares": order.Shares,
		"exec_day": order.ExecDay, "status": order.Status,
	})
}

func (s *Server) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	orderID, err := strconv.ParseInt(chi.URLParam(r, "orderID"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such order")
		return
	}
	if err := store.CancelOrder(r.Context(), s.DB, room, userFrom(r).ID, orderID, s.Now()); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) handlePortfolio(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	// Lobby: nothing to value yet — just the untouched cash.
	if room.StartedAt == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"cash_cents": store.InitialCashCents, "total_cents": store.InitialCashCents,
			"positions": []any{}, "pending": []any{},
		})
		return
	}
	curDay, _, err := store.SettleRoom(r.Context(), s.DB, room, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	p, err := store.GetPortfolio(r.Context(), s.DB, room, userFrom(r).ID, curDay)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	positions := []map[string]any{}
	for _, pos := range p.Positions {
		positions = append(positions, map[string]any{
			"instrument_id": pos.InstrumentID, "shares": pos.Shares,
			"close": pos.Close, "value_cents": pos.ValueCents,
		})
	}
	pending := []map[string]any{}
	for _, o := range p.Pending {
		pending = append(pending, map[string]any{
			"id": o.ID, "instrument_id": o.InstrumentID, "side": o.Side,
			"amount_cents": o.AmountCents, "shares": o.Shares, "exec_day": o.ExecDay,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cash_cents": p.CashCents, "total_cents": p.TotalCents,
		"positions": positions, "pending": pending,
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/httpapi/ -count=1`
Expected: PASS.

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./...
git add internal/httpapi/trading.go internal/httpapi/trading_test.go internal/httpapi/server.go
git commit -m "feat(api): order placement, cancellation and portfolio endpoints"
```

---

### Task 11: Reveal endpoint, README quickstart, final sweep

**Files:**
- Create: `server/internal/httpapi/reveal.go`
- Create: `server/README.md`
- Modify: `server/internal/httpapi/server.go` (register reveal route)
- Test: `server/internal/httpapi/reveal_test.go`

**Interfaces:**
- Consumes: Tasks 8–10.
- Produces:
  - `GET /api/rooms/{roomID}/reveal` — 409 until the game has ended; then `{instruments:[{id,alias,real_name}], trades:[{username,instrument_id,side,day,price,shares,amount_cents}], leaderboard:[{username,total_cents,return_pct,late_join}]}`. This is the ONLY endpoint allowed to expose `real_name`. (Real date ranges and historical-event annotations arrive with plan 4's real scenario data; for the synthetic scenario `real_name` is empty.)

- [ ] **Step 1: Write the failing test**

`server/internal/httpapi/reveal_test.go`:

```go
package httpapi

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestRevealOnlyAfterGameEnds(t *testing.T) {
	s := newServer(t)
	seedScenario(t, s)
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	revealPath := fmt.Sprintf("/api/rooms/%d/reveal", roomID)

	// Lobby: no reveal.
	resp, _ := host.do("GET", revealPath, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("reveal in lobby: %d", resp.StatusCode)
	}

	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/orders", roomID), map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 1_000_000}, http.StatusOK)

	// Mid-game: still no reveal.
	advance(10 * 60 * time.Second)
	resp, _ = host.do("GET", revealPath, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("reveal mid-game: %d", resp.StatusCode)
	}

	// Past the end: reveal opens up and carries real identity + trades.
	advance(400 * 60 * time.Second)
	got := host.mustJSON("GET", revealPath, nil, http.StatusOK)
	instruments := got["instruments"].([]any)
	if len(instruments) != 8 {
		t.Fatalf("reveal instruments: %d", len(instruments))
	}
	if _, hasKey := instruments[0].(map[string]any)["real_name"]; !hasKey {
		t.Fatalf("reveal missing real_name: %v", instruments[0])
	}
	trades := got["trades"].([]any)
	if len(trades) != 1 {
		t.Fatalf("reveal trades: %v", trades)
	}
	tr := trades[0].(map[string]any)
	if tr["username"] != "host" || tr["instrument_id"] != "S1" || tr["day"].(float64) != 1 {
		t.Fatalf("trade: %v", tr)
	}
	if len(got["leaderboard"].([]any)) != 1 {
		t.Fatalf("reveal leaderboard: %v", got["leaderboard"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/httpapi/ -run TestReveal -v`
Expected: FAIL (route missing).

- [ ] **Step 3: Write the implementation**

In `server.go`, replace the Task 10 placeholder comment with:

```go
		r.Get("/api/rooms/{roomID}/reveal", s.handleReveal)
```

`server/internal/httpapi/reveal.go`:

```go
package httpapi

import (
	"net/http"

	"github.com/toddzheng/stocker/server/internal/store"
)

// handleReveal is the blind box's opening ceremony (spec §2.4): only after
// the scenario's final day has passed may real identities and everyone's
// trades be shown. Until then it answers 409.
func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt == nil {
		writeErr(w, http.StatusConflict, "game not finished")
		return
	}
	curDay, ended, err := store.SettleRoom(r.Context(), s.DB, room, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if !ended {
		writeErr(w, http.StatusConflict, "game not finished")
		return
	}

	instRows, err := s.DB.Query(r.Context(), `
		SELECT i.id, i.alias, i.real_name
		FROM instruments i JOIN rooms rm ON rm.scenario_id = i.scenario_id
		WHERE rm.id = $1 ORDER BY i.ord`, room.ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer instRows.Close()
	instruments := []map[string]any{}
	for instRows.Next() {
		var id, alias, realName string
		if err := instRows.Scan(&id, &alias, &realName); err != nil {
			s.storeErr(w, err)
			return
		}
		instruments = append(instruments, map[string]any{
			"id": id, "alias": alias, "real_name": realName,
		})
	}
	if err := instRows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}

	tradeRows, err := s.DB.Query(r.Context(), `
		SELECT u.username, t.instrument_id, t.side, t.day, t.price, t.shares, t.amount_cents
		FROM trades t JOIN users u ON u.id = t.user_id
		WHERE t.room_id = $1 ORDER BY t.day, t.id`, room.ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer tradeRows.Close()
	trades := []map[string]any{}
	for tradeRows.Next() {
		var username, instrumentID, side string
		var day int
		var price, shares float64
		var amountCents int64
		if err := tradeRows.Scan(&username, &instrumentID, &side, &day, &price, &shares, &amountCents); err != nil {
			s.storeErr(w, err)
			return
		}
		trades = append(trades, map[string]any{
			"username": username, "instrument_id": instrumentID, "side": side,
			"day": day, "price": price, "shares": shares, "amount_cents": amountCents,
		})
	}
	if err := tradeRows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}

	lbRows, err := store.Leaderboard(r.Context(), s.DB, room, curDay)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	leaderboard := []map[string]any{}
	for _, lr := range lbRows {
		leaderboard = append(leaderboard, map[string]any{
			"username":    lr.Username,
			"total_cents": lr.TotalCents,
			"return_pct":  float64(lr.TotalCents-store.InitialCashCents) / float64(store.InitialCashCents),
			"late_join":   lr.JoinedDay > 0,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"instruments": instruments,
		"trades":      trades,
		"leaderboard": leaderboard,
	})
}
```

`server/README.md`:

```markdown
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
```

- [ ] **Step 4: Run the full suite**

Run: `cd server && go vet ./... && gofmt -l . && STOCKER_TEST_DB=... go test ./... -count=1`
Expected: vet clean, gofmt output empty, all packages PASS (engine + scenario + store + httpapi).

- [ ] **Step 5: Commit**

```bash
cd server
git add internal/httpapi/reveal.go internal/httpapi/reveal_test.go internal/httpapi/server.go README.md
git commit -m "feat(api): end-of-game reveal endpoint and server README"
```
