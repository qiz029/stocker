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
cd server && go run ./cmd/pipeline import && go run ./cmd/server
# (or use go run ./cmd/seedscenario for the built-in synthetic scenario)

# 2. Frontend (second terminal)
cd web && npm install && npm run dev
```

Open http://localhost:5173, register two accounts (two browsers),
create a room with the 测试局 duration, share the invite code, start
the clock, and trade.
