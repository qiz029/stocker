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
- Scenario picker — available in the Lobby to select which historical market era to play
