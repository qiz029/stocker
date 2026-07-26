# React Frontend + Chat (Plan 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** React SPA (Vite + TypeScript) implementing the approved Robinhood-style dark UI — auth, lobby, room watch screen with live asset curve, stock detail with trade panel, chat room, and reveal — plus the small backend increment it needs (room chat, news body column, instrument profiles, own-trades endpoint).

**Architecture:** The SPA talks to the existing plan-2 REST API through Vite's dev proxy (`/api` → `:8080`), cookie sessions carried automatically (same-origin). No WebSocket: room state polls every 30 s; news/events/chat use incremental `after=id` fetches on the same cadence. Charts are hand-rolled canvas (the hover-scrub interaction is the core experience — approved in the v3 mockup). The asset curve is computed client-side from the player's own settled trades + per-instrument price series.

**Tech Stack:** Backend unchanged stack (Go, chi, pgx). Frontend: Vite, React 18, TypeScript, react-router-dom. Dev/test: vitest, @testing-library/react, jsdom. NO chart library, NO UI/CSS framework, NO state-management library.

## Global Constraints

- Frontend runtime deps are exactly: `react`, `react-dom`, `react-router-dom`. Dev deps: `vite`, `@vitejs/plugin-react`, `typescript`, `@types/react`, `@types/react-dom`, `vitest`, `jsdom`, `@testing-library/react`, `@testing-library/jest-dom`. Nothing else (spec §5.1: 自足).
- Backend deps stay exactly chi/pgx/x-crypto; the backend increment adds NO new modules.
- No WebSocket, no SSE, no cron: polling only, 30 s cadence for room data (spec §5.1: "无 WebSocket，房间页 30–60 秒轮询动态").
- Blind box holds in the UI: the frontend never receives (and must never render) seed, news track, shock vectors, or real names outside the reveal page. Chat/news/events use only fields the API exposes.
- Visual identity is the approved v3 mockup: dark-only theme (deliberate choice), tokens `--bg:#0c0d10 --card:#15171c --card-2:#1b1e24 --line:#23262d --ink:#f6f7f9 --ink2:#8a919e --ink3:#565c66 --up:#00c805 --down:#ff5000`; numbers use `tabular-nums`; chart line green when period-up / red when period-down with dashed period-start baseline and hover scrub that live-updates the hero number.
- Money: API cents (int) ↔ UI dollars via the `fmtCents` helper only — no ad-hoc `/100` in components.
- Chat: messages ≤ 500 chars, day-stamped with the room's current day at post time; anonymous whale events stay separate from chat.
- Backend commits: `go vet` clean, `gofmt` clean, `go test ./... -count=1` green (STOCKER_TEST_DB set). Frontend commits: `npx tsc --noEmit` clean, `npx vitest run` green, `npm run build` succeeds.
- Frontend tests mock `fetch`; they never require a running backend or Postgres.

## File Structure

```
server/
  internal/store/migrations/0002_chat_profile_body.sql   # room_chat + instruments.profile + room_news.body
  internal/store/chat.go            # PostChat / ChatSince (+ chat_test.go)
  internal/store/scenarios.go       # + SetInstrumentDisplay (display-only upsert)
  internal/store/trades.go          # TradesForUser (+ trades_test.go)
  internal/httpapi/chat.go          # POST/GET /api/rooms/{roomID}/chat (+ chat_test.go)
  internal/httpapi/rooms.go         # room state: + instrument profile; news: + body
  internal/httpapi/trading.go       # + GET /api/rooms/{roomID}/trades
  cmd/seedscenario/main.go          # + Chinese aliases/descriptions/profiles for S1–S8
web/
  package.json  vite.config.ts  tsconfig.json  index.html
  src/main.tsx  src/App.tsx           # router + auth guard shell
  src/api.ts                          # typed fetch client + ApiError + all response types
  src/format.ts                       # fmtCents, fmtPct, deltaParts, windowed
  src/usePoll.ts                      # polling hook
  src/theme.css                       # full design-token stylesheet (from v3 mockup)
  src/components/HeroChart.tsx        # big number + delta + canvas + range tabs + hover scrub
  src/components/Sparkline.tsx
  src/components/InstrumentRow.tsx
  src/components/TradePanel.tsx
  src/components/Chat.tsx
  src/components/RightRail.tsx        # leaderboard + events + news (expandable body)
  src/pages/Login.tsx  Lobby.tsx  Room.tsx  Stock.tsx  Reveal.tsx
  src/[matching *.test.ts(x) files]
```

Existing API surface consumed (from plan 2, unchanged): `POST /api/register|login|logout`, `GET /api/me`, `POST /api/rooms`, `POST /api/rooms/join`, `GET /api/rooms`, `POST /api/rooms/{id}/start`, `GET /api/rooms/{id}` (`{room, instruments, quotes, leaderboard}`), `GET /api/rooms/{id}/prices/{inst}` (`{days:[{open,high,low,close}]}`), `GET /api/rooms/{id}/news?after=`, `GET /api/rooms/{id}/events?after=`, `POST /api/rooms/{id}/orders`, `DELETE /api/rooms/{id}/orders/{oid}`, `GET /api/rooms/{id}/portfolio`, `GET /api/rooms/{id}/reveal`.

---

### Task 1: Backend store — chat, display profiles, news body, own trades

**Files:**
- Create: `server/internal/store/migrations/0002_chat_profile_body.sql`
- Create: `server/internal/store/chat.go`
- Create: `server/internal/store/trades.go`
- Modify: `server/internal/store/scenarios.go` (append `SetInstrumentDisplay`)
- Modify: `server/internal/store/errors.go` (add `ErrBadChatMessage`)
- Test: `server/internal/store/chat_test.go`, `server/internal/store/trades_test.go`

**Interfaces:**
- Consumes: plan-2 store (`Querier`, `TestDB`, `Room`, helpers `mkUser`/`mkScenario`/`mkRunningRoom`, `PlaceOrder`, `SettleRoom`).
- Produces:
  - Migration 0002 (applied automatically by the existing `Migrate` on next boot/test)
  - `type ChatMessage struct { ID int64; Username string; Day int; Text string }`
  - `func PostChat(ctx context.Context, q Querier, room *Room, userID int64, day int, text string) (int64, error)` — trims text; empty or > 500 runes → `ErrBadChatMessage`
  - `func ChatSince(ctx context.Context, q Querier, roomID, afterID int64, limit int) ([]ChatMessage, error)` — ascending id, joined usernames
  - `type Trade struct { InstrumentID string; Side string; Day int; Price float64; Shares float64; AmountCents int64 }`
  - `func TradesForUser(ctx context.Context, q Querier, roomID, userID int64) ([]Trade, error)` — ordered day, id
  - `type InstrumentDisplay struct { Alias, Desc, Business, Bull, Bear string }`
  - `func SetInstrumentDisplay(ctx context.Context, db *pgxpool.Pool, scenarioID string, display map[string]InstrumentDisplay) error` — updates alias/descr and writes `{business,bull,bear}` into `instruments.profile`; unknown instrument id → error

- [ ] **Step 1: Write the failing tests**

`server/internal/store/migrations/0002_chat_profile_body.sql` is part of the implementation (Step 3), but the tests reference the new tables/columns, so write tests first and watch them fail on compile/SQL.

`server/internal/store/chat_test.go`:

```go
package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestChatPostAndSince(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, _ := mkRunningRoom(t, pool)

	id1, err := PostChat(ctx, pool, room, guest.ID, 0, "大家好")
	if err != nil {
		t.Fatalf("PostChat: %v", err)
	}
	id2, err := PostChat(ctx, pool, room, guest.ID, 3, "  科技股要起飞了  ")
	if err != nil || id2 <= id1 {
		t.Fatalf("second PostChat: id=%d err=%v", id2, err)
	}

	msgs, err := ChatSince(ctx, pool, room.ID, 0, 100)
	if err != nil {
		t.Fatalf("ChatSince: %v", err)
	}
	if len(msgs) != 2 || msgs[0].ID != id1 || msgs[1].ID != id2 {
		t.Fatalf("messages: %+v", msgs)
	}
	if msgs[1].Text != "科技股要起飞了" { // trimmed
		t.Fatalf("text not trimmed: %q", msgs[1].Text)
	}
	if msgs[0].Username != "guest" || msgs[0].Day != 0 || msgs[1].Day != 3 {
		t.Fatalf("metadata: %+v", msgs)
	}

	// Incremental fetch.
	tail, err := ChatSince(ctx, pool, room.ID, id1, 100)
	if err != nil || len(tail) != 1 || tail[0].ID != id2 {
		t.Fatalf("incremental: %+v err=%v", tail, err)
	}

	// Validation.
	if _, err := PostChat(ctx, pool, room, guest.ID, 0, "   "); !errors.Is(err, ErrBadChatMessage) {
		t.Fatalf("blank message: %v", err)
	}
	if _, err := PostChat(ctx, pool, room, guest.ID, 0, strings.Repeat("啊", 501)); !errors.Is(err, ErrBadChatMessage) {
		t.Fatalf("oversized message: %v", err)
	}
	if _, err := PostChat(ctx, pool, room, guest.ID, 0, strings.Repeat("a", 500)); err != nil {
		t.Fatalf("500 runes should be allowed: %v", err)
	}
}
```

`server/internal/store/trades_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"
)

func TestTradesForUser(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	if _, err := PlaceOrder(ctx, pool, room, guest.ID, t0, OrderReq{
		InstrumentID: "S1", Side: "buy", AmountCents: 3_000_000}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SettleRoom(ctx, pool, room, t0.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}

	trades, err := TradesForUser(ctx, pool, room.ID, guest.ID)
	if err != nil {
		t.Fatalf("TradesForUser: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("trades: %+v", trades)
	}
	tr := trades[0]
	if tr.InstrumentID != "S1" || tr.Side != "buy" || tr.Day != 1 ||
		tr.AmountCents != 3_000_000 || tr.Shares <= 0 || tr.Price <= 0 {
		t.Fatalf("trade: %+v", tr)
	}

	// Other players' trades are not visible.
	host, err := GetUserByUsername(ctx, pool, "host")
	if err != nil {
		t.Fatal(err)
	}
	hostTrades, err := TradesForUser(ctx, pool, room.ID, host.ID)
	if err != nil || len(hostTrades) != 0 {
		t.Fatalf("host trades: %+v err=%v", hostTrades, err)
	}
}

func TestSetInstrumentDisplay(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	sc := mkScenario(t, pool)

	err := SetInstrumentDisplay(ctx, pool, sc.ID, map[string]InstrumentDisplay{
		"S1": {Alias: "郊狼网络", Desc: "网络设备巨头", Business: "路由器", Bull: "卖铲人", Bear: "客户烧钱"},
	})
	if err != nil {
		t.Fatalf("SetInstrumentDisplay: %v", err)
	}
	var alias, descr string
	var profile []byte
	if err := pool.QueryRow(ctx, `
		SELECT alias, descr, profile FROM instruments WHERE scenario_id=$1 AND id='S1'`,
		sc.ID).Scan(&alias, &descr, &profile); err != nil {
		t.Fatal(err)
	}
	if alias != "郊狼网络" || descr != "网络设备巨头" || len(profile) == 0 {
		t.Fatalf("display not applied: %s %s %s", alias, descr, profile)
	}

	// Unknown instrument id errors instead of silently no-oping.
	if err := SetInstrumentDisplay(ctx, pool, sc.ID, map[string]InstrumentDisplay{
		"NOPE": {Alias: "x"}}); err == nil {
		t.Fatal("unknown instrument should error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && STOCKER_TEST_DB=postgres://localhost:5432/stocker_test?sslmode=disable go test ./internal/store/ -run 'TestChat|TestTrades|TestSetInstrument' -v`
Expected: FAIL (undefined symbols).

- [ ] **Step 3: Write the implementation**

`server/internal/store/migrations/0002_chat_profile_body.sql`:

```sql
CREATE TABLE room_chat (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    day INT NOT NULL,
    text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX room_chat_room ON room_chat (room_id, id);

-- Display-only rich profile {business, bull, bear}; filled by seedscenario
-- (synthetic) and by plan 4's data pipeline (real scenarios).
ALTER TABLE instruments ADD COLUMN profile JSONB;

-- News body copy; empty until plan 4's LLM batch generation fills it
-- (template fallback keeps headline-only news working).
ALTER TABLE room_news ADD COLUMN body TEXT NOT NULL DEFAULT '';
```

Append to `server/internal/store/errors.go` inside the `var (...)` block:

```go
	ErrBadChatMessage = errors.New("chat message must be 1-500 characters")
```

`server/internal/store/chat.go`:

```go
package store

import (
	"context"
	"strings"
	"unicode/utf8"
)

type ChatMessage struct {
	ID       int64
	Username string
	Day      int
	Text     string
}

const maxChatRunes = 500

// PostChat records a message stamped with the room's current day (the
// caller computes it — lobby rooms use day 0).
func PostChat(ctx context.Context, q Querier, room *Room, userID int64, day int, text string) (int64, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxChatRunes {
		return 0, ErrBadChatMessage
	}
	var id int64
	err := q.QueryRow(ctx, `
		INSERT INTO room_chat (room_id, user_id, day, text)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		room.ID, userID, day, text).Scan(&id)
	return id, err
}

// ChatSince returns messages with id > afterID in ascending id order.
func ChatSince(ctx context.Context, q Querier, roomID, afterID int64, limit int) ([]ChatMessage, error) {
	rows, err := q.Query(ctx, `
		SELECT c.id, u.username, c.day, c.text
		FROM room_chat c JOIN users u ON u.id = c.user_id
		WHERE c.room_id = $1 AND c.id > $2
		ORDER BY c.id LIMIT $3`, roomID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.Username, &m.Day, &m.Text); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
```

`server/internal/store/trades.go`:

```go
package store

import "context"

type Trade struct {
	InstrumentID string
	Side         string
	Day          int
	Price        float64
	Shares       float64
	AmountCents  int64
}

// TradesForUser returns the caller's own settled trades — the frontend
// rebuilds the personal asset curve from these. Other players' trades
// stay hidden until reveal (spec §2.2).
func TradesForUser(ctx context.Context, q Querier, roomID, userID int64) ([]Trade, error) {
	rows, err := q.Query(ctx, `
		SELECT instrument_id, side, day, price, shares, amount_cents
		FROM trades WHERE room_id = $1 AND user_id = $2
		ORDER BY day, id`, roomID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Trade
	for rows.Next() {
		var t Trade
		if err := rows.Scan(&t.InstrumentID, &t.Side, &t.Day, &t.Price, &t.Shares, &t.AmountCents); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
```

Append to `server/internal/store/scenarios.go`:

```go
type InstrumentDisplay struct {
	Alias    string `json:"-"`
	Desc     string `json:"-"`
	Business string `json:"business"`
	Bull     string `json:"bull"`
	Bear     string `json:"bear"`
}

// SetInstrumentDisplay overwrites display-only columns (alias, descr,
// profile) for existing instruments. World generation never reads these,
// so applying them after SaveScenario cannot affect determinism.
func SetInstrumentDisplay(ctx context.Context, db *pgxpool.Pool, scenarioID string, display map[string]InstrumentDisplay) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		for id, d := range display {
			profile, err := json.Marshal(d)
			if err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `
				UPDATE instruments SET alias = $3, descr = $4, profile = $5
				WHERE scenario_id = $1 AND id = $2`,
				scenarioID, id, d.Alias, d.Desc, string(profile))
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return fmt.Errorf("set display: unknown instrument %q in scenario %q", id, scenarioID)
			}
		}
		return nil
	})
}
```

(Add `"fmt"` to the imports of `scenarios.go`; `encoding/json`, pgx, pgxpool are already imported.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/store/ -count=1`
Expected: PASS — including all plan-2 tests (migration 0002 applies cleanly on the fresh schemas).

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/store/
git commit -m "feat(store): room chat, own-trades query, instrument display profiles, news body column"
```

---

### Task 2: Backend HTTP — chat endpoints, trades endpoint, richer room payloads, seed display data

**Files:**
- Create: `server/internal/httpapi/chat.go`
- Modify: `server/internal/httpapi/server.go` (register chat + trades routes)
- Modify: `server/internal/httpapi/rooms.go` (room state instruments: + `profile`; news items: + `body`)
- Modify: `server/internal/httpapi/trading.go` (add `handleMyTrades`)
- Modify: `server/cmd/seedscenario/main.go` (apply Chinese display data for S1–S8)
- Test: `server/internal/httpapi/chat_test.go`; extend `server/internal/httpapi/trading_test.go`

**Interfaces:**
- Consumes: Task 1 store functions; existing httpapi helpers (`roomForMember`, `storeErr`, `writeJSON`, `readJSON`, test helpers).
- Produces (all under auth + membership):
  - `POST /api/rooms/{roomID}/chat` `{"text"}` → `{"id"}`; day = current day (0 in lobby); 400 on bad message
  - `GET /api/rooms/{roomID}/chat?after=N` → `{"items":[{id,username,day,text}]}` (limit 200)
  - `GET /api/rooms/{roomID}/trades` → `{"items":[{instrument_id,side,day,price,shares,amount_cents}]}` — settles first; own trades only
  - Room state `instruments[]` now each carry `profile: {business,bull,bear} | null`
  - News items now each carry `body` (empty string until plan 4)
  - `seedscenario` seeds the 8 approved display profiles (aliases 郊狼网络/门户之星/网购乐/芯速半导/码力软件/老树能源/稳健零售/环宇工业)

- [ ] **Step 1: Write the failing tests**

`server/internal/httpapi/chat_test.go`:

```go
package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestChatFlow(t *testing.T) {
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
	guest.mustJSON("POST", "/api/rooms/join",
		map[string]any{"invite_code": created["invite_code"]}, http.StatusOK)
	chatPath := fmt.Sprintf("/api/rooms/%d/chat", roomID)

	// Lobby chat is allowed, stamped day 0.
	host.mustJSON("POST", chatPath, map[string]any{"text": "开局前聊两句"}, http.StatusOK)

	// Start, advance to day 2, chat again.
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	advance(2*60*time.Second + time.Second)
	guest.mustJSON("POST", chatPath, map[string]any{"text": "科技股什么情况"}, http.StatusOK)

	got := guest.mustJSON("GET", chatPath+"?after=0", nil, http.StatusOK)
	items := got["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("chat items: %v", items)
	}
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	if first["username"] != "host" || first["day"].(float64) != 0 {
		t.Fatalf("first message: %v", first)
	}
	if second["username"] != "guest" || second["day"].(float64) != 2 {
		t.Fatalf("second message: %v", second)
	}

	// Incremental fetch.
	after := int64(second["id"].(float64))
	tail := guest.mustJSON("GET", fmt.Sprintf("%s?after=%d", chatPath, after), nil, http.StatusOK)
	if n := len(tail["items"].([]any)); n != 0 {
		t.Fatalf("incremental returned %d", n)
	}

	// Validation and membership.
	resp, _ := guest.do("POST", chatPath, map[string]any{"text": "   "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("blank chat: %d", resp.StatusCode)
	}
	resp, _ = guest.do("POST", chatPath, map[string]any{"text": strings.Repeat("啊", 501)})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized chat: %d", resp.StatusCode)
	}
	resp, _ = outsider.do("GET", chatPath+"?after=0", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider chat read: %d", resp.StatusCode)
	}
}

func TestRoomStateCarriesProfileAndNewsBody(t *testing.T) {
	s := newServer(t)
	sc := seedScenario(t, s)
	if err := storeSetDisplayForTest(s, sc.ID); err != nil {
		t.Fatalf("set display: %v", err)
	}
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	advance(61 * time.Second)

	state := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d", roomID), nil, http.StatusOK)
	instruments := state["instruments"].([]any)
	s1 := instruments[0].(map[string]any)
	if s1["alias"] != "郊狼网络" {
		t.Fatalf("alias not applied: %v", s1)
	}
	profile, ok := s1["profile"].(map[string]any)
	if !ok || profile["bull"] != "卖铲人" {
		t.Fatalf("profile missing: %v", s1)
	}
	// Instruments without display data have null profile, not a crash.
	s2 := instruments[1].(map[string]any)
	if _, hasKey := s2["profile"]; !hasKey {
		t.Fatalf("profile key absent on undisplayed instrument: %v", s2)
	}

	// News items carry a body field (empty until plan 4).
	news := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/news?after=0", roomID), nil, http.StatusOK)
	items := news["items"].([]any)
	if len(items) == 0 {
		t.Fatal("no news by day 1")
	}
	if _, hasBody := items[0].(map[string]any)["body"]; !hasBody {
		t.Fatalf("news item missing body: %v", items[0])
	}
}
```

Add this helper to `server/internal/httpapi/testutil_test.go`:

```go
func storeSetDisplayForTest(s *Server, scenarioID string) error {
	return store.SetInstrumentDisplay(context.Background(), s.DB, scenarioID, map[string]store.InstrumentDisplay{
		"S1": {Alias: "郊狼网络", Desc: "网络设备巨头", Business: "路由器", Bull: "卖铲人", Bear: "客户烧钱"},
	})
}
```

(`testutil_test.go` needs `context` added to its imports.)

Append to `server/internal/httpapi/trading_test.go`:

```go
func TestMyTradesEndpoint(t *testing.T) {
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

	guest.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/orders", roomID), map[string]any{
		"instrument_id": "S1", "side": "buy", "amount_cents": 1_000_000}, http.StatusOK)
	advance(61 * time.Second)

	// The trades endpoint settles lazily itself.
	got := guest.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/trades", roomID), nil, http.StatusOK)
	items := got["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("trades: %v", items)
	}
	tr := items[0].(map[string]any)
	if tr["instrument_id"] != "S1" || tr["day"].(float64) != 1 || tr["amount_cents"].(float64) != 1_000_000 {
		t.Fatalf("trade: %v", tr)
	}
	// Host sees only their own (none).
	got = host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/trades", roomID), nil, http.StatusOK)
	if n := len(got["items"].([]any)); n != 0 {
		t.Fatalf("host trades: %d", n)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && STOCKER_TEST_DB=... go test ./internal/httpapi/ -run 'TestChat|TestRoomStateCarries|TestMyTrades' -v`
Expected: FAIL (404 routes / missing fields).

- [ ] **Step 3: Write the implementation**

`server/internal/httpapi/chat.go`:

```go
package httpapi

import (
	"net/http"

	"github.com/toddzheng/stocker/server/internal/store"
)

func (s *Server) handlePostChat(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	day := 0
	if room.StartedAt != nil {
		if d, _, err := room.CurrentDay(s.Now()); err == nil {
			day = d
		}
	}
	id, err := store.PostChat(r.Context(), s.DB, room, userFrom(r).ID, day, req.Text)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) handleGetChat(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	msgs, err := store.ChatSince(r.Context(), s.DB, room.ID, afterParam(r), newsPageLimit)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, m := range msgs {
		items = append(items, map[string]any{
			"id": m.ID, "username": m.Username, "day": m.Day, "text": m.Text,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
```

Map `store.ErrBadChatMessage` to 400: add to the 400 case list in `helpers.go`'s `storeErr`:

```go
		errors.Is(err, store.ErrBadChatMessage),
```

In `server.go`'s authed group, after the reveal route add:

```go
		r.Post("/api/rooms/{roomID}/chat", s.handlePostChat)
		r.Get("/api/rooms/{roomID}/chat", s.handleGetChat)
		r.Get("/api/rooms/{roomID}/trades", s.handleMyTrades)
```

Append to `server/internal/httpapi/trading.go`:

```go
func (s *Server) handleMyTrades(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt != nil {
		if _, _, err := store.SettleRoom(r.Context(), s.DB, room, s.Now()); err != nil {
			s.storeErr(w, err)
			return
		}
	}
	trades, err := store.TradesForUser(r.Context(), s.DB, room.ID, userFrom(r).ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, t := range trades {
		items = append(items, map[string]any{
			"instrument_id": t.InstrumentID, "side": t.Side, "day": t.Day,
			"price": t.Price, "shares": t.Shares, "amount_cents": t.AmountCents,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
```

In `rooms.go` `handleRoomState`, change the instruments query and row scan to include profile:

```go
	instRows, err := s.DB.Query(r.Context(), `
		SELECT i.id, i.alias, i.descr, i.profile
		FROM instruments i JOIN rooms rm ON rm.scenario_id = i.scenario_id
		WHERE rm.id = $1 ORDER BY i.ord`, room.ID)
```

and in the loop:

```go
	for instRows.Next() {
		var id, alias, desc string
		var profile map[string]any // nil when column is NULL
		if err := instRows.Scan(&id, &alias, &desc, &profile); err != nil {
			s.storeErr(w, err)
			return
		}
		instruments = append(instruments, map[string]any{
			"id": id, "alias": alias, "desc": desc, "profile": profile,
		})
	}
```

In `rooms.go` `handleNews`, add body to the SELECT and payload:

```go
	rows, err := s.DB.Query(r.Context(), `
		SELECT id, day, media_id, headline, body FROM room_news
		WHERE room_id = $1 AND day <= $2 AND id > $3
		ORDER BY id LIMIT $4`, room.ID, curDay, afterParam(r), newsPageLimit)
```

```go
		var id int64
		var day int
		var mediaID, headline, body string
		if err := rows.Scan(&id, &day, &mediaID, &headline, &body); err != nil { ... }
		items = append(items, map[string]any{
			"id": id, "day": day, "media_id": mediaID, "headline": headline, "body": body,
		})
```

(Keep the existing error handling shape — only the fields change. The blind-box test asserting absence of `track`/`true_shock`/`report_shock` must still pass.)

In `cmd/seedscenario/main.go`, after `store.SaveScenario` succeeds, apply the approved display set:

```go
	display := map[string]store.InstrumentDisplay{
		"S1": {Alias: "郊狼网络", Desc: "网络设备巨头，泡沫叙事的旗手",
			Business: "路由器与交换机占营收七成，其余来自企业网络服务合约。客户遍布电信运营商、门户网站与新兴的宽带服务商——换句话说，它的客户就是整个新经济。",
			Bull:     "只要还有人往网上搬业务，它的订单就不会停。多头把它类比成淘金潮里卖铲子的人：不管哪家网站赢，铲子都得从它这买。",
			Bear:     "客户集中在烧钱的互联网公司——如果融资环境收紧，下游资本开支会先于一切崩塌。此外，估值已把未来十年的增长全部计入。"},
		"S2": {Alias: "门户之星", Desc: "流量入口，人人都从这里上网",
			Business: "门户页面广告位销售为主，附带邮箱、搜索与社区服务。用户时长是它对广告主报价的全部底气。",
			Bull:     "互联网人口每季度都在膨胀，而入口只有几个。眼球即货币，它是铸币厂。",
			Bear:     "广告主大多也是互联网创业公司——泡沫内循环。用户忠诚度存疑：换个主页只需要三秒钟。"},
		"S3": {Alias: "网购乐", Desc: "烧钱换增长的电商先驱",
			Business: "线上零售平台，从图书起家扩张到全品类。自建仓储物流，每单都亏钱，但每季度单量都创新高。",
			Bull:     "零售的未来在线上，先烧钱圈地者赢者通吃。今天的亏损是明天垄断的门票。",
			Bear:     "现金消耗率惊人，命脉握在资本市场手里。一旦融资窗口关闭，增长故事会在一个季度内变成清算故事。"},
		"S4": {Alias: "芯速半导", Desc: "为新经济供货的芯片厂",
			Business: "网络处理器与通信芯片设计制造。下游是服务器、路由器与个人电脑厂商。",
			Bull:     "半导体是数字时代的石油。产能供不应求，涨价函比财报先到。",
			Bear:     "半导体从来是周期行业——库存周期一旦掉头，'订单排到明年'会变成'砍单砍到明年'。"},
		"S5": {Alias: "码力软件", Desc: "企业上网潮的军火商",
			Business: "企业级数据库与电商中间件授权，配套实施顾问服务。签单模式：一次性授权费 + 年度维护费。",
			Bull:     "'触网'是所有 CEO 的年度关键词，预算无上限。它的销售漏斗就是整个财富五百强名单。",
			Bear:     "授权收入一次性确认，增长依赖不断找到新客户。当'该上网的都上完了'，增长引擎会突然熄火。"},
		"S6": {Alias: "老树能源", Desc: "现金流稳健的传统油气",
			Business: "上游油气开采与管道运输，长期供销合约锁定大部分产量。资本开支保守，分红率常年行业前列。",
			Bull:     "无论线上线下，人总要开车取暖。市场恐慌时，现金流就是最硬的叙事。",
			Bear:     "增长天花板肉眼可见，油价下行周期里分红也难保。在狂热年代，它的股价可能长期跑输大盘。"},
		"S7": {Alias: "稳健零售", Desc: "全国连锁的百货集团",
			Business: "全国数百家门店的连锁百货，自有品牌占比逐年提升，会员体系贡献一半复购。",
			Bull:     "电商吵得再凶，九成五的零售额仍发生在线下。它便宜、赚钱、还在回购股票。",
			Bear:     "同店增速逐年放缓，年轻客群流失。它是电商故事里被指名道姓的'被颠覆者'。"},
		"S8": {Alias: "环宇工业", Desc: "多元化经营的工业集团",
			Business: "发电设备、航空部件、医疗器械加一个不小的金融部门。业务横跨周期，东方不亮西方亮。",
			Bull:     "分散即防御。当单一赛道的故事破灭时，资金会回流到这种什么都做一点的巨轮上。",
			Bear:     "多元化也意味着哪个业务都不性感，管理层被批'什么都做，什么都不精'。金融部门的杠杆是报表深处的暗礁。"},
	}
	if err := store.SetInstrumentDisplay(ctx, pool, sc.ID, display); err != nil {
		log.Fatalf("set instrument display: %v", err)
	}
	log.Printf("applied display profiles for %d instruments", len(display))
```

- [ ] **Step 4: Run the full backend suite**

Run: `cd server && STOCKER_TEST_DB=... go test ./... -count=1 && go build ./cmd/seedscenario`
Expected: PASS everywhere; the plan-2 blind-box tests still pass (news gained only `body`).

- [ ] **Step 5: Vet and commit**

```bash
cd server && go vet ./... && gofmt -l .
git add internal/httpapi/ cmd/seedscenario/
git commit -m "feat(api): chat endpoints, own-trades endpoint, instrument profiles and news body"
```

---
### Task 3: Frontend scaffold — Vite, router, API client, auth pages

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/index.html`, `web/vitest.setup.ts`
- Create: `web/src/main.tsx`, `web/src/App.tsx`, `web/src/api.ts`, `web/src/theme.css`
- Create: `web/src/pages/Login.tsx`
- Test: `web/src/api.test.ts`, `web/src/pages/Login.test.tsx`

**Interfaces:**
- Consumes: backend REST API (via Vite proxy `/api` → `http://localhost:8080`).
- Produces (used by every later task):
  - `api.get<T>(path)`, `api.post<T>(path, body?)`, `api.del<T>(path)`; `class ApiError extends Error { status: number }`
  - All response types: `User, Room, Instrument, InstrumentProfile, Quote, LeaderboardRow, RoomState, NewsItem, EventItem, ChatMessage, OHLC, Portfolio, PendingOrder, Position, Trade, RevealData`
  - `App.tsx` route table: `/login`, `/` (Lobby), `/rooms/:roomId` (Room), `/rooms/:roomId/i/:instrumentId` (Stock), `/rooms/:roomId/reveal` (Reveal) — all but `/login` wrapped in `<RequireAuth>`; `useUser()` hook exposes the logged-in user
  - `web/src/theme.css` — the complete design-token stylesheet; later tasks add NO new css files, only these classes
  - Placeholder page components for Lobby/Room/Stock/Reveal (`<div>…</div>` stubs replaced by Tasks 5–9)

- [ ] **Step 1: Scaffold the project**

```bash
mkdir -p /Users/toddzheng/Workspace/react/stocker/web/src/pages /Users/toddzheng/Workspace/react/stocker/web/src/components
cd /Users/toddzheng/Workspace/react/stocker/web
```

`web/package.json`:

```json
{
  "name": "stocker-web",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc --noEmit && vite build",
    "preview": "vite preview",
    "test": "vitest run",
    "typecheck": "tsc --noEmit"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^6.26.0"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.4.8",
    "@testing-library/react": "^16.0.0",
    "@types/react": "^18.3.3",
    "@types/react-dom": "^18.3.0",
    "@vitejs/plugin-react": "^4.3.1",
    "jsdom": "^24.1.1",
    "typescript": "^5.5.4",
    "vite": "^5.4.0",
    "vitest": "^2.0.5"
  }
}
```

Run `npm install` (creates `package-lock.json` — commit it).

`web/vite.config.ts`:

```ts
/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: { "/api": "http://localhost:8080" },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    globals: true,
  },
});
```

`web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noEmit": true,
    "skipLibCheck": true,
    "types": ["vitest/globals", "@testing-library/jest-dom"]
  },
  "include": ["src", "vitest.setup.ts", "vite.config.ts"]
}
```

`web/vitest.setup.ts`:

```ts
import "@testing-library/jest-dom/vitest";

// jsdom has no canvas; stub the 2D context so chart components can mount.
const noop = () => undefined;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
(HTMLCanvasElement.prototype as any).getContext = function () {
  return new Proxy(
    { canvas: this },
    { get: (t, p) => (p in t ? (t as never)[p] : noop) },
  );
};
```

`web/index.html`:

```html
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <meta name="color-scheme" content="dark" />
    <title>Stocker · 盲盒行情台</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 2: Write the failing tests**

`web/src/api.test.ts`:

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "./api";

const mockFetch = (status: number, body: unknown) =>
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );

afterEach(() => vi.restoreAllMocks());

describe("api client", () => {
  it("returns parsed JSON on success", async () => {
    mockFetch(200, { id: 1, username: "alice" });
    const me = await api.get<{ id: number; username: string }>("/api/me");
    expect(me.username).toBe("alice");
  });

  it("throws ApiError with server message on failure", async () => {
    mockFetch(409, { error: "username taken" });
    await expect(api.post("/api/register", { username: "a", password: "b" }))
      .rejects.toMatchObject({ status: 409, message: "username taken" });
  });

  it("sends JSON bodies with the right content type", async () => {
    const f = mockFetch(200, {});
    await api.post("/api/login", { username: "u", password: "p" });
    const [, init] = f.mock.calls[0]!;
    expect((init!.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
    expect(init!.body).toBe(JSON.stringify({ username: "u", password: "p" }));
  });

  it("throws ApiError even when the error body is not JSON", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("boom", { status: 500 }));
    await expect(api.get("/api/me")).rejects.toBeInstanceOf(ApiError);
  });
});
```

`web/src/pages/Login.test.tsx`:

```tsx
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import Login from "./Login";

afterEach(() => vi.restoreAllMocks());

describe("Login page", () => {
  it("logs in and calls onAuthed", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ id: 1, username: "alice" }), { status: 200 }),
    );
    const onAuthed = vi.fn();
    render(<MemoryRouter><Login onAuthed={onAuthed} /></MemoryRouter>);

    fireEvent.change(screen.getByPlaceholderText("用户名"), { target: { value: "alice" } });
    fireEvent.change(screen.getByPlaceholderText("密码"), { target: { value: "password123" } });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));

    await waitFor(() => expect(onAuthed).toHaveBeenCalledWith({ id: 1, username: "alice" }));
    expect(fetchSpy.mock.calls[0]![0]).toBe("/api/login");
  });

  it("shows the server error on failure", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "invalid username or password" }), { status: 401 }),
    );
    render(<MemoryRouter><Login onAuthed={vi.fn()} /></MemoryRouter>);
    fireEvent.change(screen.getByPlaceholderText("用户名"), { target: { value: "alice" } });
    fireEvent.change(screen.getByPlaceholderText("密码"), { target: { value: "wrong-pass" } });
    fireEvent.click(screen.getByRole("button", { name: "登录" }));
    expect(await screen.findByText("invalid username or password")).toBeInTheDocument();
  });

  it("switches to register mode", async () => {
    render(<MemoryRouter><Login onAuthed={vi.fn()} /></MemoryRouter>);
    fireEvent.click(screen.getByRole("button", { name: "注册新账号" }));
    expect(screen.getByRole("button", { name: "注册" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd web && npx vitest run`
Expected: FAIL (modules not found).

- [ ] **Step 4: Write the implementation**

`web/src/api.ts`:

```ts
export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "ApiError";
  }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  let data: unknown = {};
  try {
    data = await res.json();
  } catch {
    /* non-JSON or empty body */
  }
  if (!res.ok) {
    throw new ApiError(res.status, (data as { error?: string }).error ?? `HTTP ${res.status}`);
  }
  return data as T;
}

export const api = {
  get: <T>(path: string) => req<T>("GET", path),
  post: <T>(path: string, body?: unknown) => req<T>("POST", path, body),
  del: <T>(path: string) => req<T>("DELETE", path),
};

/* ---------- API types (mirror the Go handlers exactly) ---------- */
export type User = { id: number; username: string };
export type Room = {
  id: number; invite_code: string; scenario_id: string; days: number;
  status: "lobby" | "running"; day_duration_secs: number;
  started_at?: string; current_day?: number; ended?: boolean;
};
export type InstrumentProfile = { business: string; bull: string; bear: string };
export type Instrument = { id: string; alias: string; desc: string; profile: InstrumentProfile | null };
export type Quote = { instrument_id: string; close: number; prev_close: number };
export type LeaderboardRow = { username: string; total_cents: number; return_pct: number; late_join: boolean };
export type RoomState = { room: Room; instruments: Instrument[]; quotes: Quote[]; leaderboard: LeaderboardRow[] };
export type OHLC = { open: number; high: number; low: number; close: number };
export type NewsItem = { id: number; day: number; media_id: string; headline: string; body: string };
export type EventItem = { id: number; day: number; kind: string; payload: { instrument_id: string; side: string } };
export type ChatMessage = { id: number; username: string; day: number; text: string };
export type Position = { instrument_id: string; shares: number; close: number; value_cents: number };
export type PendingOrder = { id: number; instrument_id: string; side: string; amount_cents: number; shares: number; exec_day: number };
export type Portfolio = { cash_cents: number; total_cents: number; positions: Position[]; pending: PendingOrder[] };
export type Trade = { instrument_id: string; side: string; day: number; price: number; shares: number; amount_cents: number };
export type RevealInstrument = { id: string; alias: string; real_name: string };
export type RevealTrade = Trade & { username: string };
export type RevealData = { instruments: RevealInstrument[]; trades: RevealTrade[]; leaderboard: LeaderboardRow[] };

export const INITIAL_CASH_CENTS = 10_000_000;
export const MEDIA_NAMES: Record<string, string> = {
  wire: "通讯社", paper: "财经日报", tv: "财经频道", tabloid: "市场小报", forum: "股友论坛",
};
```

`web/src/pages/Login.tsx`:

```tsx
import { FormEvent, useState } from "react";
import { api, ApiError, User } from "../api";

export default function Login({ onAuthed }: { onAuthed: (u: User) => void }) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const u = await api.post<User>(`/api/${mode}`, { username, password });
      onAuthed(u);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "网络错误，请重试");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-wrap">
      <div className="auth-card">
        <div className="brand"><em>●</em> Stocker</div>
        <p className="auth-sub">回到过去，和朋友重新炒一次那段历史。</p>
        <form onSubmit={submit}>
          <input placeholder="用户名" value={username} autoComplete="username"
            onChange={e => setUsername(e.target.value)} />
          <input placeholder="密码" type="password" autoComplete="current-password"
            value={password} onChange={e => setPassword(e.target.value)} />
          {error && <p className="form-error">{error}</p>}
          <button className="submit" disabled={busy || !username || !password}>
            {mode === "login" ? "登录" : "注册"}
          </button>
        </form>
        <button className="link-btn" onClick={() => { setMode(mode === "login" ? "register" : "login"); setError(null); }}>
          {mode === "login" ? "注册新账号" : "已有账号，去登录"}
        </button>
      </div>
    </div>
  );
}
```

`web/src/App.tsx`:

```tsx
import { createContext, useContext, useEffect, useState } from "react";
import { BrowserRouter, Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { api, ApiError, User } from "./api";
import Login from "./pages/Login";
import Lobby from "./pages/Lobby";
import Room from "./pages/Room";
import Stock from "./pages/Stock";
import Reveal from "./pages/Reveal";

// Exported so page/component tests can provide a fake user.
export const UserCtxForTest = createContext<User | null>(null);
export const useUser = () => useContext(UserCtxForTest)!;

function Shell() {
  const [user, setUser] = useState<User | null>(null);
  const [checked, setChecked] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    api.get<User>("/api/me")
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setChecked(true));
  }, []);

  if (!checked) return null;
  if (!user) {
    return <Login onAuthed={u => { setUser(u); navigate("/"); }} />;
  }
  return (
    <UserCtxForTest.Provider value={user}>
      <Routes>
        <Route path="/" element={<Lobby />} />
        <Route path="/rooms/:roomId" element={<Room />} />
        <Route path="/rooms/:roomId/i/:instrumentId" element={<Stock />} />
        <Route path="/rooms/:roomId/reveal" element={<Reveal />} />
        <Route path="*" element={<Navigate to="/" />} />
      </Routes>
    </UserCtxForTest.Provider>
  );
}

/** Redirect to login when any child API call hits a 401 (session expiry). */
export function isAuthError(e: unknown): boolean {
  return e instanceof ApiError && e.status === 401;
}

export default function App() {
  return (
    <BrowserRouter>
      <Shell />
    </BrowserRouter>
  );
}
```

`web/src/main.tsx`:

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./theme.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

Placeholder pages so the router compiles (each replaced by its own task):

```tsx
// web/src/pages/Lobby.tsx   (replaced in Task 5)
export default function Lobby() { return <div className="wrap">Lobby — Task 5</div>; }
// web/src/pages/Room.tsx    (replaced in Task 6)
export default function Room() { return <div className="wrap">Room — Task 6</div>; }
// web/src/pages/Stock.tsx   (replaced in Task 8)
export default function Stock() { return <div className="wrap">Stock — Task 8</div>; }
// web/src/pages/Reveal.tsx  (replaced in Task 9)
export default function Reveal() { return <div className="wrap">Reveal — Task 9</div>; }
```

`web/src/theme.css` — the complete approved design system. This is the v3 mockup stylesheet adapted to class names used across Tasks 5–9; transcribe verbatim:

```css
:root {
  --bg: #0c0d10;
  --card: #15171c;
  --card-2: #1b1e24;
  --line: #23262d;
  --ink: #f6f7f9;
  --ink2: #8a919e;
  --ink3: #565c66;
  --up: #00c805;
  --down: #ff5000;
  --up-soft: rgba(0, 200, 5, 0.12);
  --down-soft: rgba(255, 80, 0, 0.12);
}
* { box-sizing: border-box; margin: 0; }
html { color-scheme: dark; }
body {
  background: var(--bg);
  color: var(--ink);
  font-family: -apple-system, "SF Pro Text", "PingFang SC", "Hiragino Sans GB", "Noto Sans SC", sans-serif;
  line-height: 1.5;
}
.num { font-variant-numeric: tabular-nums; }
button { font-family: inherit; cursor: pointer; }

/* top bar */
.topbar {
  position: sticky; top: 0; z-index: 20;
  display: flex; align-items: center; gap: 14px;
  padding: 12px 20px;
  background: rgba(12, 13, 16, 0.86);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--line);
}
.brand { font-weight: 700; letter-spacing: 0.02em; cursor: pointer; }
.brand em { font-style: normal; color: var(--up); }
.day-pill {
  font-size: 0.78rem; color: var(--ink2);
  border: 1px solid var(--line); border-radius: 999px;
  padding: 3px 12px; white-space: nowrap;
}
.day-pill b { color: var(--ink); font-weight: 600; }
.countdown { font-size: 0.78rem; color: var(--ink3); white-space: nowrap; }
.countdown b { color: var(--ink2); font-weight: 500; }
.topbar .spacer { flex: 1; }
.invite {
  font-size: 0.78rem; color: var(--up);
  background: var(--up-soft); border: none; border-radius: 999px;
  padding: 5px 14px;
}
.avatar {
  width: 30px; height: 30px; border-radius: 50%;
  background: linear-gradient(135deg, #00c805, #007a10);
  display: grid; place-items: center;
  font-size: 0.75rem; font-weight: 700; color: #04140a;
}

.wrap { max-width: 1120px; margin: 0 auto; padding: 22px 20px; }

/* auth */
.auth-wrap { min-height: 100vh; display: grid; place-items: center; padding: 20px; }
.auth-card {
  width: 100%; max-width: 380px;
  background: var(--card); border: 1px solid var(--line); border-radius: 16px;
  padding: 32px 28px;
}
.auth-card .brand { font-size: 1.3rem; }
.auth-sub { color: var(--ink2); font-size: 0.86rem; margin: 6px 0 22px; }
.auth-card form { display: grid; gap: 10px; }
.auth-card input {
  background: var(--card-2); border: 1px solid var(--line); border-radius: 10px;
  padding: 11px 14px; color: var(--ink); font: inherit; outline: none;
}
.auth-card input:focus { border-color: #3a3f49; }
.form-error { color: var(--down); font-size: 0.8rem; }
.submit {
  width: 100%; border: none; border-radius: 999px; padding: 13px;
  font-size: 0.95rem; font-weight: 700; color: #04140a; background: var(--up);
}
.submit.sell { background: var(--down); color: #1a0800; }
.submit:disabled { opacity: 0.35; cursor: default; }
.link-btn {
  width: 100%; margin-top: 14px; background: none; border: none;
  color: var(--ink2); font-size: 0.82rem;
}
.link-btn:hover { color: var(--up); }

/* lobby */
.lobby h1 { font-size: 1.5rem; font-weight: 700; margin: 8px 0 2px; }
.lobby .sub { color: var(--ink2); font-size: 0.9rem; margin-bottom: 22px; }
.room-card {
  background: var(--card); border: 1px solid var(--line); border-radius: 14px;
  padding: 20px; margin-bottom: 14px; cursor: pointer;
  transition: border-color 0.15s;
}
.room-card:hover { border-color: #3a3f49; }
.room-card .rc-top { display: flex; justify-content: space-between; align-items: baseline; gap: 10px; }
.room-card h3 { font-size: 1.05rem; font-weight: 650; }
.tag { font-size: 0.72rem; padding: 2px 10px; border-radius: 999px; }
.tag.live { color: var(--up); background: var(--up-soft); }
.tag.done { color: var(--ink2); background: var(--card-2); }
.room-card .rc-meta { color: var(--ink2); font-size: 0.82rem; margin-top: 4px; }
.progress { height: 4px; background: var(--card-2); border-radius: 4px; margin-top: 14px; overflow: hidden; }
.progress i { display: block; height: 100%; background: var(--up); border-radius: 4px; }
.ghost-btn {
  width: 100%; padding: 14px; margin-top: 6px;
  background: none; border: 1px dashed var(--line); border-radius: 14px;
  color: var(--ink2); font-size: 0.9rem;
}
.ghost-btn:hover { color: var(--up); border-color: var(--up); }
.lobby-form { display: flex; gap: 8px; margin-top: 10px; }
.lobby-form input, .lobby-form select {
  flex: 1; background: var(--card-2); border: 1px solid var(--line); border-radius: 10px;
  padding: 10px 14px; color: var(--ink); font: inherit; outline: none;
}

/* room grid */
.grid { display: grid; grid-template-columns: minmax(0, 1fr) 320px; gap: 22px; }
@media (max-width: 900px) { .grid { grid-template-columns: 1fr; } }

.hero .label { font-size: 0.8rem; color: var(--ink2); }
.hero .big { font-size: 2.4rem; font-weight: 650; letter-spacing: -0.01em; line-height: 1.15; }
.hero .delta { font-size: 0.92rem; font-weight: 500; margin-top: 2px; }
.delta.up { color: var(--up); }
.delta.down { color: var(--down); }
.hero .scrub-date { color: var(--ink3); font-size: 0.8rem; height: 1.2em; margin-top: 2px; }

.chart-box { position: relative; margin: 10px 0 4px; }
canvas.chart { width: 100%; height: auto; display: block; touch-action: none; }

.ranges { display: flex; gap: 4px; border-bottom: 1px solid var(--line); padding-bottom: 12px; }
.ranges button {
  background: none; border: none; color: var(--ink2);
  font-size: 0.8rem; font-weight: 600; padding: 4px 12px; border-radius: 999px;
}
.ranges button.on { color: #04140a; background: var(--up); }

.section { margin-top: 26px; }
.section h2, .card h2 {
  font-size: 0.78rem; font-weight: 700; letter-spacing: 0.1em;
  text-transform: uppercase; color: var(--ink2); margin-bottom: 6px;
}
.row {
  display: grid; grid-template-columns: minmax(0, 1.4fr) 88px minmax(96px, 0.7fr);
  align-items: center; gap: 14px;
  padding: 11px 2px; border-bottom: 1px solid var(--line);
  cursor: pointer;
}
.row:hover { background: rgba(255,255,255,0.02); }
.row .name { font-weight: 600; font-size: 0.94rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row .desc { color: var(--ink3); font-size: 0.76rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row canvas { width: 88px; height: 30px; }
.row .px { text-align: right; }
.row .px .p { font-weight: 600; font-size: 0.94rem; }
.pill {
  display: inline-block; min-width: 74px; text-align: center;
  font-size: 0.8rem; font-weight: 600;
  border-radius: 6px; padding: 2px 8px; margin-top: 2px;
}
.pill.up { color: var(--up); background: var(--up-soft); }
.pill.down { color: var(--down); background: var(--down-soft); }

/* right rail */
.card {
  background: var(--card); border: 1px solid var(--line); border-radius: 14px;
  padding: 16px 18px; margin-bottom: 18px;
}
.lb-row { display: flex; align-items: center; gap: 10px; padding: 7px 0; }
.lb-row .rank { width: 18px; color: var(--ink3); font-size: 0.8rem; }
.lb-row .who { flex: 1; font-size: 0.9rem; font-weight: 550; }
.lb-row .who small { color: var(--ink3); font-weight: 400; margin-left: 6px; }
.lb-row .val { text-align: right; font-size: 0.86rem; font-weight: 600; }
.lb-row .ret { display: block; font-size: 0.74rem; font-weight: 500; }
.lb-row.me .who { color: var(--up); }

.feed-item { padding: 9px 0; border-bottom: 1px solid var(--line); font-size: 0.84rem; }
.feed-item:last-child { border-bottom: none; }
.feed-item .fi-meta { color: var(--ink3); font-size: 0.72rem; margin-bottom: 1px; }
.feed-item.whale { border-left: 3px solid var(--up); padding-left: 10px; margin-left: -10px; }
.feed-item.whale.sell { border-left-color: var(--down); }
.feed-item .whale-txt { font-weight: 600; }
.feed-item.news { cursor: pointer; }
.feed-item.news .fi-title { position: relative; padding-right: 18px; }
.feed-item.news .fi-title::after {
  content: "›"; position: absolute; right: 2px; top: 0;
  color: var(--ink3); transition: transform 0.15s; transform: rotate(90deg) scaleY(1.2);
}
.feed-item.news.open .fi-title::after { transform: rotate(-90deg) scaleY(1.2); }
.fi-body {
  display: none; color: var(--ink2); font-size: 0.8rem; line-height: 1.6;
  margin-top: 7px; padding: 9px 12px;
  background: var(--card-2); border-radius: 10px;
}
.feed-item.open .fi-body { display: block; }
.feed-more {
  width: 100%; background: none; border: none; color: var(--ink2);
  font-size: 0.8rem; padding: 8px 0 0;
}
.feed-more:hover { color: var(--up); }

/* chat */
.chat-list { max-height: 280px; overflow-y: auto; display: flex; flex-direction: column; gap: 10px; padding-bottom: 4px; }
.chat-msg .cm-meta { font-size: 0.7rem; color: var(--ink3); margin-bottom: 2px; }
.chat-msg .cm-meta b { color: var(--ink2); font-weight: 600; }
.chat-msg.me .cm-meta b { color: var(--up); }
.chat-msg .cm-bubble {
  display: inline-block; font-size: 0.84rem; line-height: 1.45;
  background: var(--card-2); border-radius: 4px 12px 12px 12px;
  padding: 7px 12px; max-width: 92%;
}
.chat-msg.me { text-align: right; }
.chat-msg.me .cm-bubble {
  background: rgba(0, 200, 5, 0.14); border-radius: 12px 4px 12px 12px; text-align: left;
}
.chat-input { display: flex; gap: 8px; margin-top: 12px; }
.chat-input input {
  flex: 1; min-width: 0; background: var(--card-2); border: 1px solid var(--line);
  border-radius: 999px; padding: 8px 14px; color: var(--ink); font: inherit; font-size: 0.84rem;
  outline: none;
}
.chat-input input:focus { border-color: #3a3f49; }
.chat-input button {
  border: none; border-radius: 999px; padding: 8px 16px;
  background: var(--up); color: #04140a; font-size: 0.82rem; font-weight: 700;
}

/* stock view */
.back-btn { background: none; border: none; color: var(--ink2); font-size: 0.86rem; padding: 0 0 14px; }
.back-btn:hover { color: var(--ink); }
.stock-grid { display: grid; grid-template-columns: minmax(0, 1fr) 320px; gap: 22px; align-items: start; }
@media (max-width: 900px) { .stock-grid { grid-template-columns: 1fr; } }
.stat-strip { display: flex; flex-wrap: wrap; gap: 8px 26px; padding: 14px 0; border-top: 1px solid var(--line); margin-top: 6px; }
.stat-strip div { font-size: 0.84rem; }
.stat-strip .k { color: var(--ink3); font-size: 0.72rem; display: block; }
.profile-grid { display: grid; gap: 14px; max-width: 68ch; }
.profile-item .pk {
  font-size: 0.72rem; font-weight: 700; letter-spacing: 0.08em;
  text-transform: uppercase; color: var(--ink3); margin-bottom: 2px;
}
.profile-item .pk.bull { color: var(--up); }
.profile-item .pk.bear { color: var(--down); }
.profile-item p { color: var(--ink2); font-size: 0.88rem; }

/* trade panel */
.trade { position: sticky; top: 74px; }
.trade .tabs { display: flex; border-bottom: 1px solid var(--line); margin-bottom: 16px; }
.trade .tabs button {
  flex: 1; background: none; border: none; padding: 10px;
  color: var(--ink2); font-size: 0.9rem; font-weight: 650;
  border-bottom: 2px solid transparent;
}
.trade .tabs button.on.buy-tab { color: var(--up); border-bottom-color: var(--up); }
.trade .tabs button.on.sell-tab { color: var(--down); border-bottom-color: var(--down); }
.field-label { font-size: 0.78rem; color: var(--ink2); margin-bottom: 6px; }
.amt {
  display: flex; align-items: center; gap: 6px;
  background: var(--card-2); border: 1px solid var(--line); border-radius: 10px;
  padding: 10px 14px; font-size: 1.2rem; font-weight: 600;
}
.amt input {
  flex: 1; min-width: 0; background: none; border: none; outline: none;
  color: var(--ink); font: inherit; font-variant-numeric: tabular-nums;
}
.chips { display: flex; gap: 6px; margin: 10px 0 14px; }
.chips button {
  flex: 1; background: var(--card-2); border: 1px solid var(--line);
  border-radius: 999px; color: var(--ink2); font-size: 0.78rem; padding: 5px 0;
}
.chips button:hover { color: var(--ink); }
.est { font-size: 0.8rem; color: var(--ink2); display: flex; justify-content: space-between; padding: 3px 0; }
.est b { color: var(--ink); font-weight: 550; }
.note { font-size: 0.74rem; color: var(--ink3); margin: 10px 0 14px; line-height: 1.5; }
.pending-list { margin-top: 16px; border-top: 1px solid var(--line); padding-top: 10px; }
.pending-item { display: flex; justify-content: space-between; font-size: 0.8rem; color: var(--ink2); padding: 5px 0; }
.pending-item .cancel { background: none; border: none; color: var(--ink3); font-size: 0.76rem; }
.pending-item .cancel:hover { color: var(--down); }

/* reveal */
.reveal-table { width: 100%; border-collapse: collapse; font-size: 0.86rem; }
.reveal-table th, .reveal-table td { text-align: left; padding: 8px 10px; border-bottom: 1px solid var(--line); }
.reveal-table th { color: var(--ink2); font-size: 0.72rem; text-transform: uppercase; letter-spacing: 0.08em; }
.reveal-table .num { text-align: right; }

/* toast */
.toast {
  position: fixed; left: 50%; bottom: 32px; transform: translateX(-50%) translateY(20px);
  background: var(--card-2); border: 1px solid var(--line); color: var(--ink);
  padding: 10px 20px; border-radius: 999px; font-size: 0.85rem;
  opacity: 0; pointer-events: none; transition: opacity 0.25s, transform 0.25s;
  z-index: 40; white-space: nowrap;
}
.toast.show { opacity: 1; transform: translateX(-50%) translateY(0); }
.err-banner { color: var(--down); font-size: 0.82rem; padding: 8px 0; }
@media (prefers-reduced-motion: reduce) {
  .toast, .feed-item.news .fi-title::after { transition: none; }
}
```

- [ ] **Step 5: Run tests, typecheck, build**

Run: `cd web && npx vitest run && npx tsc --noEmit && npm run build`
Expected: all tests PASS, no type errors, build succeeds.

- [ ] **Step 6: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
printf 'node_modules/\ndist/\n' > web/.gitignore
git add web/
git commit -m "feat(web): vite scaffold, api client, auth pages, design-token stylesheet"
```

---

### Task 4: Chart components, format helpers, polling hook, asset-curve math

**Files:**
- Create: `web/src/format.ts`, `web/src/usePoll.ts`, `web/src/assetCurve.ts`
- Create: `web/src/components/HeroChart.tsx`, `web/src/components/Sparkline.tsx`
- Test: `web/src/format.test.ts`, `web/src/assetCurve.test.ts`, `web/src/components/HeroChart.test.tsx`

**Interfaces:**
- Consumes: types from `api.ts`; theme classes from Task 3.
- Produces:
  - `fmtCents(c: number): string` (`1234567` → `"$12,345.67"`), `fmt$(v: number): string`, `fmtPct(v: number): string` (`0.0123` → `"+1.23%"`)
  - `windowed<T>(series: T[], days: number): [T[], number]` — last N items + start offset; `Infinity` = whole series
  - `RANGE_TABS: [string, number][]` = `[["7日",7],["1月",21],["3月",63],["全部",Infinity]]`
  - `prettifyHeadline(h: string, aliasOf: (id: string) => string): string` — maps engine factor tokens (`IDIO:Sx`, bare `Sx`, `market板块`/`MKT板块` → 大盘, `tech sector板块` → 科技板块, `old economy板块` → 传统板块) to display names
  - `usePoll<T>(fn: () => Promise<T>, ms: number, deps: unknown[]): { data: T | null; error: string | null; reload: () => void }`
  - `assetCurve(trades: Trade[], series: Record<string, number[]>, curDay: number): number[]` — per-day total in CENTS from settled trades + close series
  - `<HeroChart label series startDay formatValue>` — big number + colored delta + canvas with hover scrub + range tabs (internal state); green when period-up, red when period-down, dashed period-start baseline
  - `<Sparkline series />` — 88×30 mini line, green/red by direction

- [ ] **Step 1: Write the failing tests**

`web/src/format.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { fmtCents, fmtPct, prettifyHeadline, windowed } from "./format";

describe("format helpers", () => {
  it("formats cents as dollars", () => {
    expect(fmtCents(1234567)).toBe("$12,345.67");
    expect(fmtCents(0)).toBe("$0.00");
    expect(fmtCents(10_000_000)).toBe("$100,000.00");
  });
  it("formats signed percentages", () => {
    expect(fmtPct(0.0123)).toBe("+1.23%");
    expect(fmtPct(-0.5)).toBe("-50.00%");
  });
  it("windows a series", () => {
    const s = [1, 2, 3, 4, 5];
    expect(windowed(s, 2)).toEqual([[4, 5], 3]);
    expect(windowed(s, Infinity)).toEqual([[1, 2, 3, 4, 5], 0]);
    expect(windowed(s, 99)).toEqual([[1, 2, 3, 4, 5], 0]);
  });
  it("prettifies engine factor tokens in headlines", () => {
    const aliasOf = (id: string) => ({ S1: "郊狼网络", S8: "环宇工业" }[id] ?? id);
    expect(prettifyHeadline("消息面变化，S8板块获得提振，市场解读不一", aliasOf))
      .toBe("消息面变化，环宇工业板块获得提振，市场解读不一");
    expect(prettifyHeadline("消息面变化，market板块承压，市场解读不一", aliasOf))
      .toBe("消息面变化，大盘承压，市场解读不一");
    expect(prettifyHeadline("tech sector板块波动 IDIO:S1", aliasOf))
      .toBe("科技板块波动 郊狼网络");
  });
});
```

`web/src/assetCurve.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { assetCurve } from "./assetCurve";
import type { Trade } from "./api";

const series = {
  S1: [100, 110, 120, 90],
  S6: [100, 100, 101, 102],
};

describe("assetCurve", () => {
  it("stays flat with no trades", () => {
    expect(assetCurve([], series, 3)).toEqual([10_000_000, 10_000_000, 10_000_000, 10_000_000]);
  });

  it("tracks a buy through price moves", () => {
    // Buy $40,000 at day-1 price 110 → 363.636… shares.
    const trades: Trade[] = [
      { instrument_id: "S1", side: "buy", day: 1, price: 110, shares: 4_000_000 / 100 / 110, amount_cents: 4_000_000 },
    ];
    const curve = assetCurve(trades, series, 3);
    expect(curve[0]).toBe(10_000_000); // before the trade
    expect(curve[1]).toBe(10_000_000); // buy at market value: no instant P&L
    // Day 2: shares worth ×(120/110)
    const shares = 4_000_000 / 100 / 110;
    expect(curve[2]).toBe(6_000_000 + Math.round(shares * 120 * 100));
    expect(curve[3]).toBe(6_000_000 + Math.round(shares * 90 * 100));
  });

  it("credits sells back to cash", () => {
    const shares = 4_000_000 / 100 / 110;
    const trades: Trade[] = [
      { instrument_id: "S1", side: "buy", day: 1, price: 110, shares, amount_cents: 4_000_000 },
      { instrument_id: "S1", side: "sell", day: 2, price: 120, shares, amount_cents: Math.round(shares * 120 * 100) },
    ];
    const curve = assetCurve(trades, series, 3);
    const afterSell = 6_000_000 + Math.round(shares * 120 * 100);
    expect(curve[2]).toBe(afterSell);
    expect(curve[3]).toBe(afterSell); // all cash again — day-3 crash doesn't touch us
  });
});
```

`web/src/components/HeroChart.test.tsx`:

```tsx
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import HeroChart from "./HeroChart";

describe("HeroChart", () => {
  const series = [100, 105, 103, 108];
  it("shows the latest value and an up delta", () => {
    render(<HeroChart label="总资产" series={series} startDay={0} formatValue={v => `$${v.toFixed(2)}`} />);
    expect(screen.getByText("$108.00")).toBeInTheDocument();
    expect(screen.getByText(/\+8\.00%/)).toBeInTheDocument();
  });
  it("switches range tabs", () => {
    render(<HeroChart label="x" series={series} startDay={0} formatValue={v => String(v)} />);
    fireEvent.click(screen.getByRole("button", { name: "7日" }));
    expect(screen.getByRole("button", { name: "7日" })).toHaveClass("on");
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run`
Expected: FAIL (modules not found).

- [ ] **Step 3: Write the implementation**

`web/src/format.ts`:

```ts
export const fmtCents = (c: number): string =>
  "$" + (c / 100).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export const fmt$ = (v: number): string =>
  "$" + v.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export const fmtPct = (v: number): string =>
  (v >= 0 ? "+" : "-") + Math.abs(v * 100).toFixed(2) + "%";

export const RANGE_TABS: [string, number][] = [["7日", 7], ["1月", 21], ["3月", 63], ["全部", Infinity]];

export function windowed<T>(series: T[], days: number): [T[], number] {
  const start = days === Infinity ? 0 : Math.max(0, series.length - days);
  return [series.slice(start), start];
}

/** Map engine factor tokens in fallback headlines to display names. */
export function prettifyHeadline(h: string, aliasOf: (id: string) => string): string {
  return h
    .replace(/IDIO:(S\d+)/g, (_, id: string) => aliasOf(id))
    .replace(/\b(S\d+)\b/g, (_, id: string) => aliasOf(id))
    .replace(/(market|MKT)板块/g, "大盘")
    .replace(/(tech sector|TECH)板块/g, "科技板块")
    .replace(/(old economy|OLD)板块/g, "传统板块");
}
```

`web/src/usePoll.ts`:

```ts
import { useCallback, useEffect, useState } from "react";

export function usePoll<T>(fn: () => Promise<T>, ms: number, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const tick = useCallback(async () => {
    try {
      setData(await fn());
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, deps);
  useEffect(() => {
    void tick();
    const t = setInterval(() => void tick(), ms);
    return () => clearInterval(t);
  }, [tick, ms]);
  return { data, error, reload: tick };
}
```

`web/src/assetCurve.ts`:

```ts
import type { Trade } from "./api";
import { INITIAL_CASH_CENTS } from "./api";

/**
 * Rebuild the player's per-day total (cents) from settled trades + close
 * series. Matches the server's assetsCents convention: frozen buy cash is
 * still counted here as plain cash (the server adds it back explicitly),
 * and frozen sell shares are still counted as position value — so the
 * curve's last point equals the portfolio endpoint's total_cents.
 */
export function assetCurve(
  trades: Trade[],
  series: Record<string, number[]>,
  curDay: number,
): number[] {
  const byDay = new Map<number, Trade[]>();
  for (const t of trades) {
    const list = byDay.get(t.day);
    if (list) list.push(t);
    else byDay.set(t.day, [t]);
  }
  const out: number[] = [];
  let cash = INITIAL_CASH_CENTS;
  const shares: Record<string, number> = {};
  for (let d = 0; d <= curDay; d++) {
    for (const t of byDay.get(d) ?? []) {
      if (t.side === "buy") {
        cash -= t.amount_cents;
        shares[t.instrument_id] = (shares[t.instrument_id] ?? 0) + t.shares;
      } else {
        cash += t.amount_cents;
        shares[t.instrument_id] = (shares[t.instrument_id] ?? 0) - t.shares;
      }
    }
    let posVal = 0;
    for (const [inst, sh] of Object.entries(shares)) {
      posVal += sh * (series[inst]?.[d] ?? 0);
    }
    out.push(cash + Math.round(posVal * 100));
  }
  return out;
}
```

`web/src/components/Sparkline.tsx`:

```tsx
import { useEffect, useRef } from "react";

const cssVar = (n: string) => getComputedStyle(document.documentElement).getPropertyValue(n).trim();

export default function Sparkline({ series }: { series: number[] }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = ref.current;
    if (!canvas || series.length < 2) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const W = (canvas.width = 176), H = (canvas.height = 60);
    ctx.clearRect(0, 0, W, H);
    let lo = Math.min(...series), hi = Math.max(...series);
    if (hi === lo) hi = lo + 1;
    ctx.strokeStyle = series[series.length - 1]! >= series[0]! ? cssVar("--up") : cssVar("--down");
    ctx.lineWidth = 3;
    ctx.lineJoin = "round";
    ctx.beginPath();
    series.forEach((v, i) => {
      const x = 3 + ((W - 6) * i) / (series.length - 1);
      const y = H - 5 - ((H - 10) * (v - lo)) / (hi - lo);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
  }, [series]);
  return <canvas ref={ref} />;
}
```

`web/src/components/HeroChart.tsx`:

```tsx
import { useEffect, useMemo, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { RANGE_TABS, fmtPct, windowed } from "../format";

const cssVar = (n: string) => getComputedStyle(document.documentElement).getPropertyValue(n).trim();
const PAD = { l: 6, r: 6, t: 22, b: 26 };

type Props = {
  label: string;
  series: number[];   // full history, day 0 .. current day
  startDay: number;   // day index of series[0]
  formatValue: (v: number) => string;
};

export default function HeroChart({ label, series, startDay, formatValue }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [rangeDays, setRangeDays] = useState<number>(Infinity);
  const [hover, setHover] = useState<number | null>(null);

  const [win, winStart] = useMemo(() => windowed(series, rangeDays), [series, rangeDays]);
  const shown = hover !== null && win[hover] !== undefined ? win[hover]! : win[win.length - 1] ?? 0;
  const ref = win[0] ?? 0;
  const up = shown >= ref;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || win.length < 2) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const W = canvas.width, H = canvas.height, n = win.length;
    ctx.clearRect(0, 0, W, H);
    let lo = Math.min(...win), hi = Math.max(...win);
    if (hi === lo) hi = lo + 1;
    const x = (i: number) => PAD.l + ((W - PAD.l - PAD.r) * i) / (n - 1);
    const y = (v: number) => H - PAD.b - ((H - PAD.b - PAD.t) * (v - lo)) / (hi - lo);
    const c = win[n - 1]! >= win[0]! ? cssVar("--up") : cssVar("--down");

    // dashed period-start baseline
    ctx.strokeStyle = "rgba(255,255,255,0.18)";
    ctx.setLineDash([2, 7]);
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(PAD.l, y(win[0]!));
    ctx.lineTo(W - PAD.r, y(win[0]!));
    ctx.stroke();
    ctx.setLineDash([]);

    // line + gradient fill
    const grad = ctx.createLinearGradient(0, PAD.t, 0, H - PAD.b);
    grad.addColorStop(0, c + "26");
    grad.addColorStop(1, c + "00");
    ctx.beginPath();
    win.forEach((v, i) => (i === 0 ? ctx.moveTo(x(i), y(v)) : ctx.lineTo(x(i), y(v))));
    ctx.strokeStyle = c;
    ctx.lineWidth = 3.5;
    ctx.lineJoin = "round";
    ctx.stroke();
    ctx.lineTo(x(n - 1), H - PAD.b);
    ctx.lineTo(PAD.l, H - PAD.b);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();

    if (hover !== null && hover < n) {
      ctx.strokeStyle = "rgba(255,255,255,0.35)";
      ctx.lineWidth = 1.5;
      ctx.setLineDash([4, 5]);
      ctx.beginPath();
      ctx.moveTo(x(hover), PAD.t - 8);
      ctx.lineTo(x(hover), H - PAD.b + 8);
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.fillStyle = c;
      ctx.beginPath();
      ctx.arc(x(hover), y(win[hover]!), 8, 0, 7);
      ctx.fill();
      ctx.fillStyle = cssVar("--bg");
      ctx.beginPath();
      ctx.arc(x(hover), y(win[hover]!), 3.5, 0, 7);
      ctx.fill();
    } else {
      ctx.fillStyle = c;
      ctx.beginPath();
      ctx.arc(x(n - 1), y(win[n - 1]!), 7, 0, 7);
      ctx.fill();
    }
  }, [win, hover]);

  function onPointerMove(e: ReactPointerEvent<HTMLCanvasElement>) {
    const canvas = canvasRef.current!;
    const r = canvas.getBoundingClientRect();
    const i = Math.round(((e.clientX - r.left) / r.width) * (win.length - 1));
    setHover(Math.max(0, Math.min(i, win.length - 1)));
  }

  const diff = shown - ref;
  const pct = ref ? diff / ref : 0;

  return (
    <div>
      <div className="hero">
        <div className="label">{label}</div>
        <div className="big num">{formatValue(shown)}</div>
        <div className={`delta num ${up ? "up" : "down"}`}>
          {up ? "▲" : "▼"} {formatValue(Math.abs(diff)).replace("-", "")} ({fmtPct(pct)}) 区间
        </div>
        <div className="scrub-date num">
          {hover !== null ? `第 ${startDay + winStart + hover} 个交易日` : " "}
        </div>
      </div>
      <div className="chart-box">
        <canvas
          className="chart" width={1560} height={440} ref={canvasRef}
          onPointerMove={onPointerMove} onPointerLeave={() => setHover(null)}
        />
      </div>
      <div className="ranges">
        {RANGE_TABS.map(([tabLabel, days]) => (
          <button key={tabLabel} className={rangeDays === days ? "on" : ""}
            onClick={() => { setRangeDays(days); setHover(null); }}>
            {tabLabel}
          </button>
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run && npx tsc --noEmit`
Expected: PASS, no type errors.

- [ ] **Step 5: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
git add web/src/
git commit -m "feat(web): chart components, format helpers, polling hook, asset-curve math"
```

---
### Task 5: Lobby page — my rooms, create, join

**Files:**
- Create: `web/src/pages/Lobby.tsx` (replace stub)
- Create: `web/src/Toast.tsx`
- Test: `web/src/pages/Lobby.test.tsx`

**Interfaces:**
- Consumes: `api`, `Room` type, `useUser`, theme classes (`lobby`, `room-card`, `tag`, `progress`, `ghost-btn`, `lobby-form`).
- Produces:
  - Lobby route content: list of my rooms (progress bar for running rooms, 已结束 tag when ended, click → `/rooms/:id`), a join-by-invite form, and a create form (day duration select: 1周=2016s/day for 300 days ≈ `2016`, 2周=`4032`, 4周=`8064`, plus 测试=60)
  - `<Toast>` component + `useToast()` hook — `{ toast, node }`; every later page reuses it

- [ ] **Step 1: Write the failing test**

`web/src/pages/Lobby.test.tsx`:

```tsx
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import Lobby from "./Lobby";

const rooms = [
  { id: 1, invite_code: "ABC", scenario_id: "synthetic-v1", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-26T12:00:00Z", current_day: 150, ended: false },
  { id: 2, invite_code: "DEF", scenario_id: "synthetic-v1", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-01T12:00:00Z", current_day: 299, ended: true },
  { id: 3, invite_code: "GHI", scenario_id: "synthetic-v1", days: 300, status: "lobby",
    day_duration_secs: 4032 },
];

function mockRoutes(handler: (url: string, init?: RequestInit) => unknown) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) =>
    new Response(JSON.stringify(handler(String(url), init as RequestInit)), { status: 200 }));
}

afterEach(() => vi.restoreAllMocks());

describe("Lobby", () => {
  it("lists my rooms with status tags", async () => {
    mockRoutes(() => ({ rooms }));
    render(<MemoryRouter><Lobby /></MemoryRouter>);
    // day counter is split across nodes (第 <b>150</b> / 300) — assert the bold number
    await waitFor(() => expect(screen.getByText("150")).toBeInTheDocument());
    expect(screen.getByText("已结束")).toBeInTheDocument();
    expect(screen.getByText("等待开局")).toBeInTheDocument();
  });

  it("joins a room by invite code", async () => {
    const calls: string[] = [];
    mockRoutes((url, init) => {
      calls.push(`${init?.method ?? "GET"} ${url}`);
      if (url === "/api/rooms/join") return rooms[0];
      return { rooms: [] };
    });
    render(<MemoryRouter><Lobby /></MemoryRouter>);
    fireEvent.change(await screen.findByPlaceholderText("输入邀请码"), { target: { value: "ABC" } });
    fireEvent.click(screen.getByRole("button", { name: "加入" }));
    await waitFor(() => expect(calls).toContain("POST /api/rooms/join"));
  });

  it("creates a room", async () => {
    const bodies: unknown[] = [];
    mockRoutes((url, init) => {
      if (url === "/api/rooms" && init?.method === "POST") {
        bodies.push(JSON.parse(String(init.body)));
        return rooms[2];
      }
      return { rooms: [] };
    });
    render(<MemoryRouter><Lobby /></MemoryRouter>);
    fireEvent.click(await screen.findByRole("button", { name: "＋ 创建新房间" }));
    fireEvent.click(screen.getByRole("button", { name: "创建" }));
    await waitFor(() => expect(bodies).toEqual([{ scenario_id: "synthetic-v1", day_duration_secs: 4032 }]));
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/pages/Lobby.test.tsx`
Expected: FAIL (stub has none of this).

- [ ] **Step 3: Write the implementation**

`web/src/Toast.tsx`:

```tsx
import { useCallback, useRef, useState } from "react";

export function useToast() {
  const [msg, setMsg] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const toast = useCallback((m: string) => {
    setMsg(m);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setMsg(null), 2400);
  }, []);
  const node = <div className={`toast ${msg ? "show" : ""}`}>{msg}</div>;
  return { toast, node };
}
```

`web/src/pages/Lobby.tsx`:

```tsx
import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError, Room } from "../api";
import { usePoll } from "../usePoll";
import { useToast } from "../Toast";
import { useUser } from "../App";

const DURATIONS: [string, number][] = [
  ["1 周局（约 34 分钟/交易日）", 2016],
  ["2 周局（约 67 分钟/交易日）", 4032],
  ["4 周局（约 134 分钟/交易日）", 8064],
  ["测试局（1 分钟/交易日）", 60],
];

export default function Lobby() {
  const user = useUser();
  const navigate = useNavigate();
  const { toast, node } = useToast();
  const { data, reload } = usePoll(() => api.get<{ rooms: Room[] }>("/api/rooms"), 30_000, []);
  const [invite, setInvite] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [duration, setDuration] = useState(4032);
  const [busy, setBusy] = useState(false);

  async function join(e: FormEvent) {
    e.preventDefault();
    try {
      const room = await api.post<Room>("/api/rooms/join", { invite_code: invite.trim() });
      navigate(`/rooms/${room.id}`);
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "加入失败");
    }
  }

  async function create() {
    setBusy(true);
    try {
      const room = await api.post<Room>("/api/rooms", {
        scenario_id: "synthetic-v1", day_duration_secs: duration,
      });
      toast("平行世界生成完毕");
      void reload();
      navigate(`/rooms/${room.id}`);
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "创建失败");
    } finally {
      setBusy(false);
    }
  }

  function roomStatus(r: Room): { tag: string; cls: string } {
    if (r.status === "lobby") return { tag: "等待开局", cls: "done" };
    if (r.ended) return { tag: "已结束", cls: "done" };
    return { tag: "进行中", cls: "live" };
  }

  return (
    <div className="wrap lobby">
      <div className="topbar" style={{ margin: "-22px -20px 22px" }}>
        <div className="brand"><em>●</em> Stocker</div>
        <div className="spacer" />
        <div className="avatar">{user.username.slice(0, 2)}</div>
      </div>
      <h1>我的房间</h1>
      <p className="sub">和朋友回到过去，重新炒一次那段历史。</p>

      {(data?.rooms ?? []).map(r => {
        const st = roomStatus(r);
        return (
          <div key={r.id} className="room-card" onClick={() => navigate(`/rooms/${r.id}`)}>
            <div className="rc-top">
              <h3>神秘年代 #{r.id}</h3>
              <span className={`tag ${st.cls}`}>{st.tag}</span>
            </div>
            <div className="rc-meta">
              {r.status === "running" && r.current_day !== undefined
                ? <>第 <b className="num">{r.current_day}</b> / {r.days} 个交易日</>
                : <>邀请码 <b className="num">{r.invite_code}</b> · 每交易日 {Math.round(r.day_duration_secs / 60)} 分钟</>}
            </div>
            {r.status === "running" && r.current_day !== undefined && (
              <div className="progress"><i style={{ width: `${(r.current_day / r.days) * 100}%` }} /></div>
            )}
          </div>
        );
      })}

      <form className="lobby-form" onSubmit={join}>
        <input placeholder="输入邀请码" value={invite} onChange={e => setInvite(e.target.value)} />
        <button className="submit" style={{ width: "auto", padding: "10px 22px" }} disabled={!invite.trim()}>加入</button>
      </form>

      {showCreate ? (
        <div className="lobby-form">
          <select value={duration} onChange={e => setDuration(Number(e.target.value))}>
            {DURATIONS.map(([label, secs]) => <option key={secs} value={secs}>{label}</option>)}
          </select>
          <button className="submit" style={{ width: "auto", padding: "10px 22px" }}
            onClick={create} disabled={busy}>{busy ? "生成平行世界…" : "创建"}</button>
        </div>
      ) : (
        <button className="ghost-btn" onClick={() => setShowCreate(true)}>＋ 创建新房间</button>
      )}
      {node}
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
git add web/src/
git commit -m "feat(web): lobby page with room list, create and join flows"
```

---

### Task 6: Room page — top bar, asset hero chart, positions, watchlist

**Files:**
- Create: `web/src/pages/Room.tsx` (replace stub)
- Create: `web/src/components/InstrumentRow.tsx`
- Create: `web/src/roomData.ts`
- Test: `web/src/roomData.test.ts`, `web/src/pages/Room.test.tsx`

**Interfaces:**
- Consumes: Tasks 3–5 (`api`, `usePoll`, `HeroChart`, `Sparkline`, `assetCurve`, `useToast`, theme classes). RightRail arrives in Task 7 — Room renders a `<div id="rail-slot" />` placeholder column this task, replaced next task.
- Produces:
  - `useRoomData(roomId: string)` in `roomData.ts` — one hook that polls (30 s) room state + portfolio + own trades, fetches all price series when `current_day` changes, and derives `{ state, portfolio, trades, series, curve }`; exposed for Room and Stock pages
  - `<InstrumentRow instrument sub right onClick sparkSeries?>` — the shared list row (name+desc | sparkline | price+pill)
  - Room page layout per the approved mockup: top bar (brand → lobby, day pill, invite-code copy, avatar), hero `总资产` chart from `curve`, 持仓 section (positions + cash row + pending note), 行情 section (8 instruments, 30-day sparkline, day-over-day pill), start button when lobby + host
  - Row click navigates to `/rooms/:roomId/i/:instrumentId`

- [ ] **Step 1: Write the failing tests**

`web/src/roomData.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { buildSeriesMap, dayCountdown } from "./roomData";

describe("roomData helpers", () => {
  it("builds a close-series map from price responses", () => {
    const map = buildSeriesMap([
      ["S1", { days: [{ open: 1, high: 2, low: 0.5, close: 1.5 }, { open: 1.5, high: 2, low: 1, close: 1.8 }] }],
      ["S2", { days: [{ open: 9, high: 9, low: 9, close: 9 }] }],
    ]);
    expect(map.S1).toEqual([1.5, 1.8]);
    expect(map.S2).toEqual([9]);
  });

  it("computes seconds until next trading day", () => {
    // started 90 s ago at 60 s/day → 30 s left in day 1
    const started = new Date(Date.now() - 90_000).toISOString();
    const secs = dayCountdown(started, 60);
    expect(secs).toBeGreaterThan(25);
    expect(secs).toBeLessThanOrEqual(30);
  });
});
```

`web/src/pages/Room.test.tsx`:

```tsx
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserCtxForTest } from "../App";
import Room from "./Room";

const state = {
  room: { id: 1, invite_code: "ABC", scenario_id: "synthetic-v1", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-26T12:00:00Z", current_day: 2, ended: false },
  instruments: [
    { id: "S1", alias: "郊狼网络", desc: "网络设备巨头", profile: null },
    { id: "S6", alias: "老树能源", desc: "传统油气", profile: null },
  ],
  quotes: [
    { instrument_id: "S1", close: 110, prev_close: 100 },
    { instrument_id: "S6", close: 99, prev_close: 100 },
  ],
  leaderboard: [{ username: "host", total_cents: 10_000_000, return_pct: 0, late_join: false }],
};
const portfolio = {
  cash_cents: 6_000_000, total_cents: 10_400_000,
  positions: [{ instrument_id: "S1", shares: 400, close: 110, value_cents: 4_400_000 }],
  pending: [],
};
const priceDays = { days: [{ open: 100, high: 111, low: 99, close: 100 }, { open: 100, high: 111, low: 99, close: 105 }, { open: 105, high: 112, low: 100, close: 110 }] };

afterEach(() => vi.restoreAllMocks());

function mockApi() {
  vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
    const u = String(url);
    let body: unknown = {};
    if (u === "/api/rooms/1") body = state;
    else if (u.endsWith("/portfolio")) body = portfolio;
    else if (u.endsWith("/trades")) body = { items: [{ instrument_id: "S1", side: "buy", day: 1, price: 100, shares: 400, amount_cents: 4_000_000 }] };
    else if (u.includes("/prices/")) body = priceDays;
    else if (u.includes("/news") || u.includes("/events") || u.includes("/chat")) body = { items: [] };
    return new Response(JSON.stringify(body), { status: 200 });
  });
}

describe("Room page", () => {
  it("renders hero total, positions and watchlist", async () => {
    mockApi();
    render(
      <MemoryRouter initialEntries={["/rooms/1"]}>
        <UserCtxForTest.Provider value={{ id: 1, username: "me" }}>
          <Routes><Route path="/rooms/:roomId" element={<Room />} /></Routes>
        </UserCtxForTest.Provider>
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText("$104,000.00")).toBeInTheDocument());
    expect(screen.getByText("郊狼网络")).toBeInTheDocument();
    expect(screen.getByText("老树能源")).toBeInTheDocument();
    expect(screen.getByText("现金")).toBeInTheDocument();
    // +10.00% appears on both the S1 position pill and the S1 watchlist pill
    expect(screen.getAllByText("+10.00%").length).toBeGreaterThan(0);
    // day counter is split across nodes; assert the pill's combined text
    expect(document.querySelector(".day-pill")?.textContent).toContain("第 2 / 300");
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/roomData.test.ts src/pages/Room.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Write the implementation**

`web/src/roomData.ts`:

```ts
import { useEffect, useMemo, useRef, useState } from "react";
import { api, Portfolio, RoomState, Trade } from "./api";
import { assetCurve } from "./assetCurve";
import { usePoll } from "./usePoll";

export type PriceResponse = { days: { open: number; high: number; low: number; close: number }[] };

export function buildSeriesMap(entries: [string, PriceResponse][]): Record<string, number[]> {
  const out: Record<string, number[]> = {};
  for (const [id, res] of entries) out[id] = res.days.map(d => d.close);
  return out;
}

/** Seconds until the next historical trading day begins. */
export function dayCountdown(startedAt: string, dayDurationSecs: number): number {
  const elapsed = (Date.now() - new Date(startedAt).getTime()) / 1000;
  const into = elapsed % dayDurationSecs;
  return Math.max(0, Math.round(dayDurationSecs - into));
}

export function useRoomData(roomId: string) {
  const { data: state, error, reload: reloadState } = usePoll(
    () => api.get<RoomState>(`/api/rooms/${roomId}`), 30_000, [roomId]);
  const { data: portfolio, reload: reloadPortfolio } = usePoll(
    () => api.get<Portfolio>(`/api/rooms/${roomId}/portfolio`), 30_000, [roomId]);
  const { data: tradesRes, reload: reloadTrades } = usePoll(
    () => api.get<{ items: Trade[] }>(`/api/rooms/${roomId}/trades`), 30_000, [roomId]);

  const [series, setSeries] = useState<Record<string, number[]>>({});
  const fetchedDay = useRef(-1);
  const curDay = state?.room.current_day ?? -1;
  const instrumentIds = useMemo(
    () => (state?.instruments ?? []).map(i => i.id).join(","), [state]);

  useEffect(() => {
    if (curDay < 0 || !instrumentIds || fetchedDay.current === curDay) return;
    fetchedDay.current = curDay;
    void Promise.all(
      instrumentIds.split(",").map(async id =>
        [id, await api.get<PriceResponse>(`/api/rooms/${roomId}/prices/${id}`)] as [string, PriceResponse]),
    ).then(entries => setSeries(buildSeriesMap(entries)));
  }, [roomId, curDay, instrumentIds]);

  const trades = tradesRes?.items ?? [];
  const curve = useMemo(
    () => (curDay >= 0 && Object.keys(series).length ? assetCurve(trades, series, curDay) : []),
    [trades, series, curDay]);

  return {
    state, portfolio, trades, series, curve, error,
    reload: () => { void reloadState(); void reloadPortfolio(); void reloadTrades(); },
  };
}
```

`web/src/components/InstrumentRow.tsx`:

```tsx
import Sparkline from "./Sparkline";

type Props = {
  name: string;
  sub: string;
  price: string;
  pill: string;
  pillUp: boolean;
  sparkSeries?: number[];
  onClick?: () => void;
};

export default function InstrumentRow({ name, sub, price, pill, pillUp, sparkSeries, onClick }: Props) {
  return (
    <div className="row" onClick={onClick} style={onClick ? undefined : { cursor: "default" }}>
      <div>
        <div className="name">{name}</div>
        <div className="desc">{sub}</div>
      </div>
      <div>{sparkSeries && sparkSeries.length > 1 ? <Sparkline series={sparkSeries} /> : null}</div>
      <div className="px">
        <div className="p num">{price}</div>
        {pill && <span className={`pill num ${pillUp ? "up" : "down"}`}>{pill}</span>}
      </div>
    </div>
  );
}
```

`web/src/pages/Room.tsx`:

```tsx
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api, ApiError } from "../api";
import { fmtCents, fmt$, fmtPct } from "../format";
import { dayCountdown, useRoomData } from "../roomData";
import { useToast } from "../Toast";
import { useUser } from "../App";
import HeroChart from "../components/HeroChart";
import InstrumentRow from "../components/InstrumentRow";
import RightRail from "../components/RightRail";

export default function Room() {
  const { roomId } = useParams<{ roomId: string }>();
  const navigate = useNavigate();
  const user = useUser();
  const { toast, node } = useToast();
  const { state, portfolio, series, curve, error, reload } = useRoomData(roomId!);
  const [countdown, setCountdown] = useState<number | null>(null);

  const room = state?.room;
  useEffect(() => {
    if (!room?.started_at || room.ended) { setCountdown(null); return; }
    const update = () => setCountdown(dayCountdown(room.started_at!, room.day_duration_secs));
    update();
    const t = setInterval(update, 1000);
    return () => clearInterval(t);
  }, [room?.started_at, room?.day_duration_secs, room?.ended]);

  if (error) return <div className="wrap err-banner">{error}</div>;
  if (!state || !room) return null;

  const curDay = room.current_day ?? 0;
  const aliasOf = (id: string) => state.instruments.find(i => i.id === id)?.alias ?? id;

  async function startRoom() {
    try {
      await api.post(`/api/rooms/${roomId}/start`);
      toast("时间轴已启动");
      reload();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "启动失败");
    }
  }

  function copyInvite() {
    void navigator.clipboard?.writeText(room!.invite_code);
    toast(`邀请码 ${room!.invite_code} 已复制`);
  }

  const mmss = countdown !== null
    ? `${String(Math.floor(countdown / 60)).padStart(2, "0")}:${String(countdown % 60).padStart(2, "0")}`
    : null;

  return (
    <div>
      <div className="topbar">
        <div className="brand" onClick={() => navigate("/")}><em>●</em> Stocker</div>
        <div className="day-pill">
          {room.status === "lobby"
            ? "等待开局"
            : room.ended
              ? "已结束 · 等待揭晓"
              : <>神秘年代 · 第 <b className="num">{curDay}</b> / {room.days} 个交易日</>}
        </div>
        {mmss && <div className="countdown">距下一交易日 <b className="num">{mmss}</b></div>}
        <div className="spacer" />
        {room.ended && (
          <button className="invite" onClick={() => navigate(`/rooms/${roomId}/reveal`)}>查看揭晓</button>
        )}
        <button className="invite" onClick={copyInvite}>邀请好友</button>
        <div className="avatar">{user.username.slice(0, 2)}</div>
      </div>

      <div className="wrap">
        {room.status === "lobby" ? (
          <div className="room-card" style={{ cursor: "default" }}>
            <h3>房间尚未开局</h3>
            <p className="rc-meta">把邀请码 <b className="num">{room.invite_code}</b> 发给朋友；人齐后由房主启动时间轴。</p>
            <button className="submit" style={{ marginTop: 14 }} onClick={startRoom}>启动时间轴（房主）</button>
          </div>
        ) : (
          <div className="grid">
            <div>
              {curve.length > 1 && (
                <HeroChart label="总资产" series={curve} startDay={0} formatValue={fmtCents} />
              )}

              <div className="section">
                <h2>持仓</h2>
                {(portfolio?.positions ?? []).map(p => (
                  <InstrumentRow key={p.instrument_id}
                    name={aliasOf(p.instrument_id)}
                    sub={`${p.shares.toFixed(1)} 股 · 市值 ${fmtCents(p.value_cents)}`}
                    price={fmt$(p.close)}
                    pill={fmtPct(p.close / (series[p.instrument_id]?.[0] ?? p.close) - 1)}
                    pillUp={p.close >= (series[p.instrument_id]?.[0] ?? p.close)}
                    onClick={() => navigate(`/rooms/${roomId}/i/${p.instrument_id}`)}
                  />
                ))}
                <InstrumentRow name="现金" sub="可随时下单"
                  price={portfolio ? fmtCents(portfolio.cash_cents) : "—"} pill="" pillUp />
                {(portfolio?.pending?.length ?? 0) > 0 && (
                  <p className="rc-meta">另有 {portfolio!.pending.length} 笔挂单冻结中，开盘成交。</p>
                )}
              </div>

              <div className="section">
                <h2>行情 · 盲盒标的</h2>
                {state.instruments.map(inst => {
                  const q = state.quotes.find(x => x.instrument_id === inst.id);
                  const s = series[inst.id] ?? [];
                  return (
                    <InstrumentRow key={inst.id}
                      name={inst.alias} sub={inst.desc}
                      price={q ? fmt$(q.close) : "—"}
                      pill={q ? fmtPct(q.close / q.prev_close - 1) : ""}
                      pillUp={q ? q.close >= q.prev_close : true}
                      sparkSeries={s.slice(Math.max(0, s.length - 30))}
                      onClick={() => navigate(`/rooms/${roomId}/i/${inst.id}`)}
                    />
                  );
                })}
              </div>
            </div>

            <RightRail roomId={roomId!} state={state} aliasOf={aliasOf} />
          </div>
        )}
      </div>
      {node}
    </div>
  );
}
```

For this task only, create a minimal `web/src/components/RightRail.tsx` stub so Room compiles (Task 7 replaces it):

```tsx
import { RoomState } from "../api";

export default function RightRail(_props: { roomId: string; state: RoomState; aliasOf: (id: string) => string }) {
  return <div />;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
git add web/src/
git commit -m "feat(web): room page with asset hero chart, positions and watchlist"
```

---

### Task 7: Right rail — leaderboard, whale feed, news with bodies, chat

**Files:**
- Modify: `web/src/components/RightRail.tsx` (replace stub with the real component)
- Create: `web/src/components/Chat.tsx`
- Test: `web/src/components/RightRail.test.tsx`, `web/src/components/Chat.test.tsx`

**Interfaces:**
- Consumes: `usePoll`, `api` (`NewsItem`, `EventItem`, `ChatMessage`, `LeaderboardRow`), `prettifyHeadline`, `useUser`, theme classes (`card`, `lb-row`, `feed-item`, `chat-*`).
- Produces:
  - `<RightRail roomId state aliasOf />` — three cards: 排行榜 (rank, name+晚入场, total+return, own row highlighted), 聊天室 (`<Chat>`), 房间动态 (whale feed: 🐳 匿名买入/卖出 with alias, green/red left border), 今日新闻 (headline via `prettifyHeadline`, click-to-expand body when `body !== ""`, headline-only otherwise, "更早的新闻" pagination by slice)
  - `<Chat roomId />` — polls `/chat?after=<lastId>` every 30 s appending increments, posts on submit (Enter or button), own messages right-aligned green, error toast on 400

- [ ] **Step 1: Write the failing tests**

`web/src/components/Chat.test.tsx`:

```tsx
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserProviderForTest } from "./testutil";
import Chat from "./Chat";

afterEach(() => vi.restoreAllMocks());

describe("Chat", () => {
  it("renders messages and sends a new one", async () => {
    const posted: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const u = String(url);
      if (init?.method === "POST") {
        posted.push(JSON.parse(String(init.body)));
        return new Response(JSON.stringify({ id: 3 }), { status: 200 });
      }
      return new Response(JSON.stringify({ items: [
        { id: 1, username: "host", day: 0, text: "开局前聊两句" },
        { id: 2, username: "me", day: 2, text: "冲了" },
      ] }), { status: 200 });
    });
    render(<UserProviderForTest username="me"><Chat roomId="1" /></UserProviderForTest>);
    expect(await screen.findByText("开局前聊两句")).toBeInTheDocument();
    // own message gets the .me class
    expect(screen.getByText("冲了").closest(".chat-msg")).toHaveClass("me");

    fireEvent.change(screen.getByPlaceholderText("说点什么…"), { target: { value: "科技股什么情况" } });
    fireEvent.click(screen.getByRole("button", { name: "发送" }));
    await waitFor(() => expect(posted).toEqual([{ text: "科技股什么情况" }]));
  });
});
```

`web/src/components/RightRail.test.tsx`:

```tsx
import { render, screen, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserProviderForTest } from "./testutil";
import RightRail from "./RightRail";
import type { RoomState } from "../api";

const state: RoomState = {
  room: { id: 1, invite_code: "A", scenario_id: "s", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-26T12:00:00Z", current_day: 5, ended: false },
  instruments: [{ id: "S1", alias: "郊狼网络", desc: "", profile: null }],
  quotes: [],
  leaderboard: [
    { username: "me", total_cents: 12_000_000, return_pct: 0.2, late_join: false },
    { username: "amy", total_cents: 9_000_000, return_pct: -0.1, late_join: true },
  ],
};

afterEach(() => vi.restoreAllMocks());

describe("RightRail", () => {
  it("renders leaderboard, whale event and expandable news", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u.includes("/events")) body = { items: [
        { id: 1, day: 4, kind: "whale", payload: { instrument_id: "S1", side: "buy" } }] };
      if (u.includes("/news")) body = { items: [
        { id: 1, day: 5, media_id: "wire", headline: "消息面变化，S1板块承压，市场解读不一", body: "正文内容。" }] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "郊狼网络"} /></UserProviderForTest>);

    // leaderboard: own row highlighted, late-join marked
    expect(await screen.findByText("me")).toBeInTheDocument();
    expect(screen.getByText("me").closest(".lb-row")).toHaveClass("me");
    expect(screen.getByText("晚入场")).toBeInTheDocument();
    expect(screen.getByText("+20.00%")).toBeInTheDocument();

    // whale
    expect(await screen.findByText(/大额买入 郊狼网络/)).toBeInTheDocument();

    // news headline prettified + body expands on click (jsdom loads no CSS,
    // so assert the state class rather than computed visibility)
    const headline = await screen.findByText(/郊狼网络板块承压/);
    const item = headline.closest(".feed-item")!;
    expect(item).not.toHaveClass("open");
    fireEvent.click(headline);
    expect(item).toHaveClass("open");
    expect(screen.getByText("正文内容。")).toBeInTheDocument();
  });
});
```

`web/src/components/testutil.tsx` (test-only helper, imported by both tests):

```tsx
import { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { UserCtxForTest } from "../App";

export function UserProviderForTest({ username, children }: { username: string; children: ReactNode }) {
  return (
    <MemoryRouter>
      <UserCtxForTest.Provider value={{ id: 1, username }}>{children}</UserCtxForTest.Provider>
    </MemoryRouter>
  );
}
```

(`UserCtxForTest` is already exported from `App.tsx` since Task 3.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/components/`
Expected: FAIL.

- [ ] **Step 3: Write the implementation**

`web/src/components/Chat.tsx`:

```tsx
import { FormEvent, useEffect, useRef, useState } from "react";
import { api, ApiError, ChatMessage } from "../api";
import { useUser } from "../App";

export default function Chat({ roomId }: { roomId: string }) {
  const user = useUser();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [text, setText] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const lastID = useRef(0);
  const listRef = useRef<HTMLDivElement>(null);

  async function fetchNew() {
    try {
      const res = await api.get<{ items: ChatMessage[] }>(`/api/rooms/${roomId}/chat?after=${lastID.current}`);
      if (res.items.length) {
        lastID.current = res.items[res.items.length - 1]!.id;
        setMessages(m => [...m, ...res.items]);
      }
    } catch {
      /* transient poll errors are silent; next tick retries */
    }
  }

  useEffect(() => {
    lastID.current = 0;
    setMessages([]);
    void fetchNew();
    const t = setInterval(() => void fetchNew(), 30_000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roomId]);

  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages]);

  async function send(e: FormEvent) {
    e.preventDefault();
    const t = text.trim();
    if (!t) return;
    setErr(null);
    try {
      await api.post(`/api/rooms/${roomId}/chat`, { text: t });
      setText("");
      await fetchNew();
    } catch (error) {
      setErr(error instanceof ApiError ? error.message : "发送失败");
    }
  }

  return (
    <div className="card">
      <h2>聊天室</h2>
      <div className="chat-list" ref={listRef}>
        {messages.map(m => (
          <div key={m.id} className={`chat-msg ${m.username === user.username ? "me" : ""}`}>
            <div className="cm-meta"><b>{m.username}</b> · <span className="num">第 {m.day} 日</span></div>
            <span className="cm-bubble">{m.text}</span>
          </div>
        ))}
      </div>
      {err && <p className="form-error">{err}</p>}
      <form className="chat-input" onSubmit={send}>
        <input placeholder="说点什么…" value={text} maxLength={500}
          onChange={e => setText(e.target.value)} />
        <button type="submit">发送</button>
      </form>
    </div>
  );
}
```

`web/src/components/RightRail.tsx` (replace the stub):

```tsx
import { useState } from "react";
import { api, EventItem, MEDIA_NAMES, NewsItem, RoomState } from "../api";
import { fmtCents, fmtPct, prettifyHeadline } from "../format";
import { usePoll } from "../usePoll";
import { useUser } from "../App";
import Chat from "./Chat";

type Props = { roomId: string; state: RoomState; aliasOf: (id: string) => string };

export default function RightRail({ roomId, state, aliasOf }: Props) {
  const user = useUser();
  const [newsShown, setNewsShown] = useState(8);
  const [openNews, setOpenNews] = useState<number | null>(null);
  const { data: newsRes } = usePoll(
    () => api.get<{ items: NewsItem[] }>(`/api/rooms/${roomId}/news?after=0`), 30_000, [roomId]);
  const { data: eventsRes } = usePoll(
    () => api.get<{ items: EventItem[] }>(`/api/rooms/${roomId}/events?after=0`), 30_000, [roomId]);

  const news = [...(newsRes?.items ?? [])].sort((a, b) => b.id - a.id).slice(0, newsShown);
  const events = [...(eventsRes?.items ?? [])].sort((a, b) => b.id - a.id).slice(0, 6);

  return (
    <div>
      <div className="card">
        <h2>排行榜</h2>
        {state.leaderboard.map((row, i) => (
          <div key={row.username} className={`lb-row ${row.username === user.username ? "me" : ""}`}>
            <span className="rank num">{i + 1}</span>
            <span className="who">{row.username}{row.late_join && <small>晚入场</small>}</span>
            <span className="val num">
              {fmtCents(row.total_cents)}
              <span className={`ret delta ${row.return_pct >= 0 ? "up" : "down"}`}>{fmtPct(row.return_pct)}</span>
            </span>
          </div>
        ))}
      </div>

      <Chat roomId={roomId} />

      <div className="card">
        <h2>房间动态</h2>
        {events.length === 0 && <div className="feed-item">暂无动态</div>}
        {events.map(ev => (
          <div key={ev.id} className={`feed-item whale ${ev.payload.side === "sell" ? "sell" : ""}`}>
            <div className="fi-meta num">第 {ev.day} 个交易日</div>
            <span className="whale-txt">
              🐳 有玩家大额{ev.payload.side === "buy" ? "买入" : "卖出"} {aliasOf(ev.payload.instrument_id)}
            </span>
          </div>
        ))}
      </div>

      <div className="card">
        <h2>今日新闻</h2>
        {news.length === 0 && <div className="feed-item">暂无新闻</div>}
        {news.map(n => (
          <div key={n.id}
            className={`feed-item ${n.body ? "news" : ""} ${openNews === n.id ? "open" : ""}`}
            onClick={n.body ? () => setOpenNews(openNews === n.id ? null : n.id) : undefined}>
            <div className="fi-meta">{MEDIA_NAMES[n.media_id] ?? n.media_id} · <span className="num">第 {n.day} 日</span></div>
            <div className={n.body ? "fi-title" : ""}>{prettifyHeadline(n.headline, aliasOf)}</div>
            {n.body && <div className="fi-body">{n.body}</div>}
          </div>
        ))}
        {(newsRes?.items.length ?? 0) > newsShown && (
          <button className="feed-more" onClick={() => setNewsShown(n => n + 8)}>更早的新闻 ↓</button>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run && npx tsc --noEmit`
Expected: PASS (including Task 6's Room test, now exercising the real RightRail — its fetch mock already answers news/events/chat with empty items).

- [ ] **Step 5: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
git add web/src/
git commit -m "feat(web): right rail with leaderboard, whale feed, expandable news and chat"
```

---
### Task 8: Stock page — chart, stats, profile, per-stock news, trade panel

**Files:**
- Create: `web/src/pages/Stock.tsx` (replace stub)
- Create: `web/src/components/TradePanel.tsx`
- Test: `web/src/components/TradePanel.test.tsx`, `web/src/pages/Stock.test.tsx`

**Interfaces:**
- Consumes: `useRoomData`, `HeroChart`, `usePoll`, `prettifyHeadline`, `useToast`, theme classes (`stock-grid`, `stat-strip`, `profile-grid`, `trade`, `amt`, `chips`, `est`, `note`, `pending-*`).
- Produces:
  - Stock page: back button → room; hero chart of the instrument's close series (`formatValue: fmt$`); stat strip (今日收盘 / 昨收 / 3月最高 / 3月最低 / 开局至今 pct / 我的持仓股数); 档案 section rendering `profile.business/bull/bear` when present (falls back to `desc` only); 相关新闻 (client-side filter: headline contains instrument id or alias, up to 5, expandable bodies); `<TradePanel>` in the right column
  - `<TradePanel roomId instrumentId lastClose portfolio onChanged>` — 买入/卖出 tabs; buy input = dollars (converted to cents on submit), sell input = shares; 25/50/75/全部 chips (buy base = cash, sell base = held shares of this instrument); estimates vs `lastClose` labeled 参考; submit disabled when 0 or over available; POST order → toast `已下单，开盘成交（已冻结）` + `onChanged()`; pending orders for this instrument listed with 撤单 (DELETE) buttons; API errors shown via toast

- [ ] **Step 1: Write the failing tests**

`web/src/components/TradePanel.test.tsx`:

```tsx
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import TradePanel from "./TradePanel";
import type { Portfolio } from "../api";

const portfolio: Portfolio = {
  cash_cents: 6_000_000,
  total_cents: 10_000_000,
  positions: [{ instrument_id: "S1", shares: 400, close: 110, value_cents: 4_400_000 }],
  pending: [{ id: 9, instrument_id: "S1", side: "buy", amount_cents: 100_000, shares: 0, exec_day: 3 }],
};

afterEach(() => vi.restoreAllMocks());

describe("TradePanel", () => {
  it("estimates shares, submits a buy in cents, disables on overspend", async () => {
    const posted: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      if (init?.method === "POST") posted.push(JSON.parse(String(init.body)));
      return new Response(JSON.stringify({ id: 1 }), { status: 200 });
    });
    const onChanged = vi.fn();
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={110} portfolio={portfolio} onChanged={onChanged} />);

    const input = screen.getByPlaceholderText("0");
    fireEvent.change(input, { target: { value: "11000" } });
    expect(screen.getByText(/≈ 100.0 股/)).toBeInTheDocument();

    // over available cash ($60,000) → disabled
    fireEvent.change(input, { target: { value: "60001" } });
    expect(screen.getByRole("button", { name: "下单买入" })).toBeDisabled();

    fireEvent.change(input, { target: { value: "50000" } });
    fireEvent.click(screen.getByRole("button", { name: "下单买入" }));
    await waitFor(() => expect(posted).toEqual([
      { instrument_id: "S1", side: "buy", amount_cents: 5_000_000 }]));
    expect(onChanged).toHaveBeenCalled();
  });

  it("sell chips fill from held shares and submit sends shares", async () => {
    const posted: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      if (init?.method === "POST") posted.push(JSON.parse(String(init.body)));
      return new Response(JSON.stringify({ id: 2 }), { status: 200 });
    });
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={110} portfolio={portfolio} onChanged={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "卖出" }));
    fireEvent.click(screen.getByRole("button", { name: "50%" }));
    expect((screen.getByPlaceholderText("0") as HTMLInputElement).value).toBe("200.0");
    fireEvent.click(screen.getByRole("button", { name: "下单卖出" }));
    await waitFor(() => expect(posted).toEqual([
      { instrument_id: "S1", side: "sell", shares: 200 }]));
  });

  it("lists and cancels pending orders", async () => {
    const deleted: string[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      if (init?.method === "DELETE") deleted.push(String(url));
      return new Response(JSON.stringify({ status: "cancelled" }), { status: 200 });
    });
    const onChanged = vi.fn();
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={110} portfolio={portfolio} onChanged={onChanged} />);
    expect(screen.getByText(/买入 \$1,000\.00/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "撤单" }));
    await waitFor(() => expect(deleted).toEqual(["/api/rooms/1/orders/9"]));
    expect(onChanged).toHaveBeenCalled();
  });
});
```

`web/src/pages/Stock.test.tsx`:

```tsx
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserCtxForTest } from "../App";
import Stock from "./Stock";

const state = {
  room: { id: 1, invite_code: "A", scenario_id: "s", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-26T12:00:00Z", current_day: 2, ended: false },
  instruments: [{ id: "S1", alias: "郊狼网络", desc: "网络设备巨头",
    profile: { business: "路由器业务", bull: "卖铲人逻辑", bear: "客户都在烧钱" } }],
  quotes: [{ instrument_id: "S1", close: 110, prev_close: 105 }],
  leaderboard: [],
};

afterEach(() => vi.restoreAllMocks());

describe("Stock page", () => {
  it("renders chart hero, stats, profile and trade panel", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = state;
      else if (u.endsWith("/portfolio")) body = { cash_cents: 10_000_000, total_cents: 10_000_000, positions: [], pending: [] };
      else if (u.endsWith("/trades")) body = { items: [] };
      else if (u.includes("/prices/")) body = { days: [
        { open: 100, high: 101, low: 99, close: 100 },
        { open: 100, high: 106, low: 99, close: 105 },
        { open: 105, high: 111, low: 104, close: 110 }] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(
      <MemoryRouter initialEntries={["/rooms/1/i/S1"]}>
        <UserCtxForTest.Provider value={{ id: 1, username: "me" }}>
          <Routes><Route path="/rooms/:roomId/i/:instrumentId" element={<Stock />} /></Routes>
        </UserCtxForTest.Provider>
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText(/郊狼网络/)).toBeInTheDocument());
    // $110.00 appears in the hero AND the 今日收盘 stat; +10.00% in the hero
    // delta AND 开局至今 — use getAllByText for both.
    expect(screen.getAllByText("$110.00").length).toBeGreaterThan(0);
    expect(screen.getByText("昨收")).toBeInTheDocument();
    expect(screen.getAllByText(/\+10\.00%/).length).toBeGreaterThan(0);
    expect(screen.getByText("卖铲人逻辑")).toBeInTheDocument();   // profile bull
    expect(screen.getByText("下单买入")).toBeInTheDocument();     // trade panel
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/components/TradePanel.test.tsx src/pages/Stock.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Write the implementation**

`web/src/components/TradePanel.tsx`:

```tsx
import { useMemo, useState } from "react";
import { api, ApiError, Portfolio } from "../api";
import { fmtCents, fmt$ } from "../format";
import { useToast } from "../Toast";

type Props = {
  roomId: string;
  instrumentId: string;
  lastClose: number;
  portfolio: Portfolio | null;
  onChanged: () => void;
};

export default function TradePanel({ roomId, instrumentId, lastClose, portfolio, onChanged }: Props) {
  const { toast, node } = useToast();
  const [side, setSide] = useState<"buy" | "sell">("buy");
  const [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false);

  const cash = portfolio?.cash_cents ?? 0;
  const heldShares = useMemo(
    () => portfolio?.positions.find(p => p.instrument_id === instrumentId)?.shares ?? 0,
    [portfolio, instrumentId]);
  const pending = (portfolio?.pending ?? []).filter(o => o.instrument_id === instrumentId);

  const value = parseFloat(raw) || 0;
  const maxValue = side === "buy" ? cash / 100 : heldShares;
  const overLimit = value > maxValue + 1e-9;

  function pickFraction(f: number) {
    if (side === "buy") setRaw(String(Math.floor((cash / 100) * f)));
    else setRaw((heldShares * f).toFixed(1));
  }

  async function submit() {
    setBusy(true);
    try {
      const body = side === "buy"
        ? { instrument_id: instrumentId, side, amount_cents: Math.round(value * 100) }
        : { instrument_id: instrumentId, side, shares: value };
      await api.post(`/api/rooms/${roomId}/orders`, body);
      toast("已下单，开盘成交（已冻结）");
      setRaw("");
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "下单失败");
    } finally {
      setBusy(false);
    }
  }

  async function cancel(orderID: number) {
    try {
      await api.del(`/api/rooms/${roomId}/orders/${orderID}`);
      toast("已撤单，资金解冻");
      onChanged();
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "撤单失败");
    }
  }

  return (
    <div className="card trade">
      <div className="tabs">
        <button className={`buy-tab ${side === "buy" ? "on" : ""}`}
          onClick={() => { setSide("buy"); setRaw(""); }}>买入</button>
        <button className={`sell-tab ${side === "sell" ? "on" : ""}`}
          onClick={() => { setSide("sell"); setRaw(""); }}>卖出</button>
      </div>
      <div className="field-label">{side === "buy" ? "买入金额" : "卖出股数"}</div>
      <div className="amt">
        {side === "buy" && <span>$</span>}
        <input inputMode="decimal" placeholder="0" value={raw}
          onChange={e => setRaw(e.target.value)} />
      </div>
      <div className="chips">
        {[["25%", 0.25], ["50%", 0.5], ["75%", 0.75], ["全部", 1]].map(([label, f]) => (
          <button key={label as string} onClick={() => pickFraction(f as number)}>{label as string}</button>
        ))}
      </div>
      <div className="est"><span>可用</span>
        <b className="num">{side === "buy" ? fmtCents(cash) : `${heldShares.toFixed(1)} 股`}</b></div>
      <div className="est">
        <span>{side === "buy" ? "预估股数（按今日收盘参考）" : "预估金额（按今日收盘参考）"}</span>
        <b className="num">
          {value > 0 && lastClose > 0
            ? side === "buy" ? `≈ ${(value / lastClose).toFixed(1)} 股` : `≈ ${fmt$(value * lastClose)}`
            : "—"}
        </b>
      </div>
      <p className="note">订单将在<b>下一个历史交易日的开盘价</b>成交，成交价此刻未知。下单即冻结，开盘前可撤单。</p>
      <button className={`submit ${side === "sell" ? "sell" : ""}`}
        disabled={busy || value <= 0 || overLimit} onClick={submit}>
        {side === "buy" ? "下单买入" : "下单卖出"}
      </button>
      {pending.length > 0 && (
        <div className="pending-list">
          <div className="field-label">待成交</div>
          {pending.map(o => (
            <div key={o.id} className="pending-item">
              <span className="num">
                {o.side === "buy" ? `买入 ${fmtCents(o.amount_cents)}` : `卖出 ${o.shares} 股`} · 第 {o.exec_day} 日开盘
              </span>
              <button className="cancel" onClick={() => cancel(o.id)}>撤单</button>
            </div>
          ))}
        </div>
      )}
      {node}
    </div>
  );
}
```

`web/src/pages/Stock.tsx`:

```tsx
import { useNavigate, useParams } from "react-router-dom";
import { MEDIA_NAMES, NewsItem, api } from "../api";
import { fmt$, fmtPct, prettifyHeadline } from "../format";
import { useRoomData } from "../roomData";
import { usePoll } from "../usePoll";
import { useState } from "react";
import HeroChart from "../components/HeroChart";
import TradePanel from "../components/TradePanel";

export default function Stock() {
  const { roomId, instrumentId } = useParams<{ roomId: string; instrumentId: string }>();
  const navigate = useNavigate();
  const { state, portfolio, series, reload } = useRoomData(roomId!);
  const { data: newsRes } = usePoll(
    () => api.get<{ items: NewsItem[] }>(`/api/rooms/${roomId}/news?after=0`), 30_000, [roomId]);
  const [openNews, setOpenNews] = useState<number | null>(null);

  if (!state) return null;
  const inst = state.instruments.find(i => i.id === instrumentId);
  const closes = series[instrumentId!] ?? [];
  if (!inst) return <div className="wrap err-banner">未知标的</div>;

  const aliasOf = (id: string) => state.instruments.find(i => i.id === id)?.alias ?? id;
  const last = closes[closes.length - 1] ?? 0;
  const prev = closes[closes.length - 2] ?? last;
  const q3m = closes.slice(-63);
  const held = portfolio?.positions.find(p => p.instrument_id === instrumentId)?.shares ?? 0;
  const relatedNews = (newsRes?.items ?? [])
    .filter(n => n.headline.includes(instrumentId!) || n.headline.includes(inst.alias))
    .sort((a, b) => b.id - a.id)
    .slice(0, 5);

  return (
    <div className="wrap">
      <button className="back-btn" onClick={() => navigate(`/rooms/${roomId}`)}>← 返回房间</button>
      <div className="stock-grid">
        <div>
          {closes.length > 1 && (
            <HeroChart
              label={`${inst.alias} · ${inst.desc} · ${inst.id}`}
              series={closes} startDay={0} formatValue={fmt$}
            />
          )}
          <div className="stat-strip num">
            <div><span className="k">今日收盘</span>{fmt$(last)}</div>
            <div><span className="k">昨收</span>{fmt$(prev)}</div>
            <div><span className="k">3月最高</span>{q3m.length ? fmt$(Math.max(...q3m)) : "—"}</div>
            <div><span className="k">3月最低</span>{q3m.length ? fmt$(Math.min(...q3m)) : "—"}</div>
            <div><span className="k">开局至今</span>
              <span className={`delta ${last >= (closes[0] ?? last) ? "up" : "down"}`}>
                {closes[0] ? fmtPct(last / closes[0] - 1) : "—"}
              </span>
            </div>
            <div><span className="k">我的持仓</span>{held > 0 ? `${held.toFixed(1)} 股` : "—"}</div>
          </div>

          <div className="section">
            <h2>档案</h2>
            <div className="profile-grid">
              <div className="profile-item"><div className="pk">简介</div><p>{inst.desc || "——"}</p></div>
              {inst.profile && (
                <>
                  <div className="profile-item"><div className="pk">主营业务</div><p>{inst.profile.business}</p></div>
                  <div className="profile-item"><div className="pk bull">多头故事</div><p>{inst.profile.bull}</p></div>
                  <div className="profile-item"><div className="pk bear">风险提示</div><p>{inst.profile.bear}</p></div>
                </>
              )}
            </div>
          </div>

          <div className="section">
            <h2>相关新闻</h2>
            {relatedNews.length === 0 && <div className="feed-item">暂无相关新闻</div>}
            {relatedNews.map(n => (
              <div key={n.id}
                className={`feed-item ${n.body ? "news" : ""} ${openNews === n.id ? "open" : ""}`}
                onClick={n.body ? () => setOpenNews(openNews === n.id ? null : n.id) : undefined}>
                <div className="fi-meta">{MEDIA_NAMES[n.media_id] ?? n.media_id} · <span className="num">第 {n.day} 日</span></div>
                <div className={n.body ? "fi-title" : ""}>{prettifyHeadline(n.headline, aliasOf)}</div>
                {n.body && <div className="fi-body">{n.body}</div>}
              </div>
            ))}
          </div>
        </div>

        <TradePanel roomId={roomId!} instrumentId={instrumentId!} lastClose={last}
          portfolio={portfolio} onChanged={reload} />
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
git add web/src/
git commit -m "feat(web): stock page with chart, profile, related news and trade panel"
```

---

### Task 9: Reveal page

**Files:**
- Create: `web/src/pages/Reveal.tsx` (replace stub)
- Test: `web/src/pages/Reveal.test.tsx`

**Interfaces:**
- Consumes: `api` (`RevealData`), `fmtCents`, `fmtPct`, `fmt$`, theme classes (`reveal-table`, `card`, `lb-row`).
- Produces: Reveal route — fetches `/api/rooms/:id/reveal` once; on 409 shows 尚未揭晓 with a back link; on success: 最终排行 (podium-ordered leaderboard), 身份揭晓 table (化名 → 真实身份, "——" when `real_name` empty with a note that synthetic scenarios carry no real identity), 全场成交回放 table (day, username, side colored, alias, price, shares, amount).

- [ ] **Step 1: Write the failing test**

`web/src/pages/Reveal.test.tsx`:

```tsx
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import Reveal from "./Reveal";

afterEach(() => vi.restoreAllMocks());

const reveal = {
  instruments: [
    { id: "S1", alias: "郊狼网络", real_name: "Cisco Systems" },
    { id: "S6", alias: "老树能源", real_name: "" },
  ],
  trades: [
    { username: "amy", instrument_id: "S1", side: "buy", day: 20, price: 180.5, shares: 221.6, amount_cents: 4_000_000 },
  ],
  leaderboard: [
    { username: "amy", total_cents: 13_420_000, return_pct: 0.342, late_join: false },
    { username: "me", total_cents: 9_100_000, return_pct: -0.09, late_join: false },
  ],
};

function renderAt(handler: () => Response) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async () => handler());
  render(
    <MemoryRouter initialEntries={["/rooms/1/reveal"]}>
      <Routes><Route path="/rooms/:roomId/reveal" element={<Reveal />} /></Routes>
    </MemoryRouter>,
  );
}

describe("Reveal page", () => {
  it("shows identities, trades and final leaderboard", async () => {
    renderAt(() => new Response(JSON.stringify(reveal), { status: 200 }));
    await waitFor(() => expect(screen.getByText("Cisco Systems")).toBeInTheDocument());
    expect(screen.getAllByText("郊狼网络").length).toBeGreaterThan(0);
    expect(screen.getByText("+34.20%")).toBeInTheDocument();
    expect(screen.getByText("amy").closest(".lb-row")).toBeTruthy();
    expect(screen.getByText("$40,000.00")).toBeInTheDocument();
  });

  it("shows a waiting message before the game ends", async () => {
    renderAt(() => new Response(JSON.stringify({ error: "game not finished" }), { status: 409 }));
    expect(await screen.findByText(/尚未揭晓/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/pages/Reveal.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Write the implementation**

`web/src/pages/Reveal.tsx`:

```tsx
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, ApiError, RevealData } from "../api";
import { fmt$, fmtCents, fmtPct } from "../format";

export default function Reveal() {
  const { roomId } = useParams<{ roomId: string }>();
  const [data, setData] = useState<RevealData | null>(null);
  const [notReady, setNotReady] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.get<RevealData>(`/api/rooms/${roomId}/reveal`)
      .then(setData)
      .catch(e => {
        if (e instanceof ApiError && e.status === 409) setNotReady(true);
        else setError(e instanceof Error ? e.message : String(e));
      });
  }, [roomId]);

  if (error) return <div className="wrap err-banner">{error}</div>;
  if (notReady) {
    return (
      <div className="wrap lobby">
        <h1>尚未揭晓</h1>
        <p className="sub">时间轴还没走完——回到房间继续操作，终点见分晓。</p>
        <Link className="link-btn" to={`/rooms/${roomId}`}>← 返回房间</Link>
      </div>
    );
  }
  if (!data) return null;

  const aliasOf = (id: string) => data.instruments.find(i => i.id === id)?.alias ?? id;
  const hasRealNames = data.instruments.some(i => i.real_name !== "");

  return (
    <div className="wrap lobby">
      <h1>揭晓时刻</h1>
      <p className="sub">盲盒打开：这段历史的真身，和每个人的全部操作。</p>

      <div className="card">
        <h2>最终排行</h2>
        {data.leaderboard.map((row, i) => (
          <div key={row.username} className="lb-row">
            <span className="rank num">{i === 0 ? "🏆" : i + 1}</span>
            <span className="who">{row.username}{row.late_join && <small>晚入场</small>}</span>
            <span className="val num">
              {fmtCents(row.total_cents)}
              <span className={`ret delta ${row.return_pct >= 0 ? "up" : "down"}`}>{fmtPct(row.return_pct)}</span>
            </span>
          </div>
        ))}
      </div>

      <div className="card">
        <h2>身份揭晓</h2>
        {!hasRealNames && (
          <p className="rc-meta">本局使用合成剧本，标的没有真实历史身份；真实剧本（如 2000 年互联网泡沫）会在这里揭晓每只股票的真名与真实日期区间。</p>
        )}
        <table className="reveal-table">
          <thead><tr><th>化名</th><th>真实身份</th></tr></thead>
          <tbody>
            {data.instruments.map(inst => (
              <tr key={inst.id}>
                <td>{inst.alias} <span className="num" style={{ color: "var(--ink3)" }}>{inst.id}</span></td>
                <td>{inst.real_name || "——"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="card">
        <h2>全场成交回放</h2>
        <table className="reveal-table">
          <thead>
            <tr><th>日</th><th>玩家</th><th>方向</th><th>标的</th>
              <th className="num">成交价</th><th className="num">股数</th><th className="num">金额</th></tr>
          </thead>
          <tbody>
            {data.trades.map((t, i) => (
              <tr key={i}>
                <td className="num">{t.day}</td>
                <td>{t.username}</td>
                <td className={`delta ${t.side === "buy" ? "up" : "down"}`}>{t.side === "buy" ? "买入" : "卖出"}</td>
                <td>{aliasOf(t.instrument_id)}</td>
                <td className="num">{fmt$(t.price)}</td>
                <td className="num">{t.shares.toFixed(1)}</td>
                <td className="num">{fmtCents(t.amount_cents)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
git add web/src/
git commit -m "feat(web): reveal page with identities, trade replay and final standings"
```

---

### Task 10: Final sweep — build, docs, dev workflow

**Files:**
- Create: `web/README.md`
- Create: `README.md` (repo root)
- Test: full suites both sides

**Interfaces:**
- Consumes: everything.
- Produces: documented two-terminal dev workflow; both builds green.

- [ ] **Step 1: Write the docs**

`web/README.md`:

```markdown
# stocker web

React SPA for the time-travel stock game. Vite + TypeScript; no chart or
UI libraries — the canvas chart with hover-scrub IS the product.

## Development

Backend first (see ../server/README.md):

```bash
createdb stocker
export DATABASE_URL=postgres://localhost:5432/stocker?sslmode=disable
go run ./cmd/seedscenario   # loads synthetic-v1 + display profiles
go run ./cmd/server         # API on :8080
```

Then:

```bash
npm install
npm run dev        # http://localhost:5173, /api proxied to :8080
```

## Tests

```bash
npm test           # vitest (jsdom, fetch mocked — no backend needed)
npm run typecheck
npm run build
```

## Structure

- `src/api.ts` — typed fetch client; every server payload type lives here
- `src/theme.css` — the entire design system (dark, Robinhood-style tokens)
- `src/components/HeroChart.tsx` — big-number hero + hover-scrub canvas chart
- `src/roomData.ts` — one hook per room: state + portfolio + trades + price series + asset curve
- `src/pages/` — Login / Lobby / Room / Stock / Reveal
```

Root `README.md`:

```markdown
# stocker

Web-based multiplayer stock game: friends travel back to a historical
market era (dot-com bubble first), trade a blind-box version of real
history, and compete. Deterministic parallel worlds, no schedulers.

- `server/` — Go backend: deterministic world engine + Postgres + REST API
- `web/` — React SPA (Vite + TS), Robinhood-style watch screen
- `docs/superpowers/` — design spec and implementation plans

## Quick start

```bash
# 1. Backend
createdb stocker
export DATABASE_URL=postgres://localhost:5432/stocker?sslmode=disable
cd server && go run ./cmd/seedscenario && go run ./cmd/server

# 2. Frontend (second terminal)
cd web && npm install && npm run dev
```

Open http://localhost:5173, register two accounts (two browsers),
create a room with the 测试局 duration, share the invite code, start
the clock, and trade.
```

- [ ] **Step 2: Full verification**

```bash
cd /Users/toddzheng/Workspace/react/stocker/server && go vet ./... && gofmt -l . && STOCKER_TEST_DB=postgres://localhost:5432/stocker_test?sslmode=disable go test ./... -count=1
cd /Users/toddzheng/Workspace/react/stocker/web && npx vitest run && npm run build
```

Expected: everything green; `web/dist/` builds.

- [ ] **Step 3: Manual smoke (documented, not automated)**

With backend + `npm run dev` running: register → create 测试局 room → start → buy S1 → wait 1 minute → portfolio shows position, asset curve moves, chat works. Record the result in the task report; do not commit any state.

- [ ] **Step 4: Commit**

```bash
cd /Users/toddzheng/Workspace/react/stocker
git add web/README.md README.md
git commit -m "docs: dev workflow for server + web"
```
