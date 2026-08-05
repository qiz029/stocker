# Stocker iOS app (Expo)

React Native (Expo) client for the stocker game, at feature parity with
the web app: hall (create/join/public rooms, era leaderboard), room
watch screen with live sim clock, news/forum/events/chat, candle charts,
trading, options, loans, player actions, and the end-of-game reveal. Shares pure-TS logic (`api`, `format`, `i18n`, `simClock`,
`usePoll`) with the web app via the repo-root `core/` directory
(imported as `@core/*`; Metro is pinned in `metro.config.js` so those
files resolve `react` from `app/node_modules`, not from a parent
directory).

## Run

```bash
cd app
npm install
npx expo start
```

Then press `i` for the iOS simulator (requires Xcode), or scan the QR
code with Expo Go / a dev build on a physical device.

## Backend URL

The API base URL comes from `extra.apiBase` in `app.json`
(default `http://localhost:8080`, read in `src/config.ts`).

- iOS simulator: `http://localhost:8080` works as-is.
- Physical device: replace `localhost` with your Mac's LAN IP
  (e.g. `http://192.168.1.20:8080`) — the phone cannot reach the Mac's
  localhost.

The Go server must be running (`cd server && go run ./cmd/server`,
see the repo-root README).

## Structure

- `app/` — expo-router screens: `_layout.tsx` (session + profile gate,
  stack), `login.tsx`, `profile.tsx` (forced display-name/avatar setup),
  `index.tsx` (hall: my rooms, create room, public rooms, era
  leaderboard), `room/[id]/index.tsx` (market tab: watch screen + loan
  panel), `room/[id]/news.tsx` + `news/[newsId].tsx` (news chains,
  media accuracy, debunk), `room/[id]/feed.tsx` (events + forum),
  `room/[id]/chat.tsx`, `room/[id]/trades.tsx` (my fills),
  `room/[id]/reveal.tsx` (end-of-game reveal),
  `room/[id]/[instrumentId].tsx` (candle chart + ranges, trading,
  options chain, hype/intel actions)
- `src/session.tsx` — SessionProvider: boot `GET /api/me`, user,
  logout, zh/EN language (device locale via `expo-localization`,
  persisted in AsyncStorage), `t` from `@core/i18n`
- `src/config.ts` — API base URL + `setApiBase`
- `src/components/` — `Sparkline`, `CandleChart` (react-native-svg,
  range tabs + 1D intraday bridge), `Avatar`, `LangToggle`, `RoomTabs`
  (market/news/activity/chat), `LoanPanel`, `OptionsChain`,
  `OptionPositions`, `ActionPanel`
- `src/hooks/` — `useSimClock` (1s ticker over `@core/simClock`),
  `useIncrementalFeed` (cursor-paginated news/events/forum/chat)
- `src/avatar.ts`, `src/era.ts`, `src/debunkVerdicts.ts` — ports of the
  matching web helpers (AsyncStorage-backed for verdicts)
- `src/notifications.ts` — Expo push skeleton: permission + token
  registration after login, unregister on logout; fully best-effort

Auth uses the `stocker_session` HttpOnly cookie; RN `fetch`
stores/sends it automatically — no token handling.

## Assets

`assets/icon.png` and `assets/splash-icon.png` are **placeholder icons**
(dark background + green "S", generated with Pillow). Replace them with
real artwork before App Store submission.

## Distribution (EAS Build / TestFlight)

Prerequisites: an Apple Developer account, an Expo account, and the EAS
CLI logged in on this machine:

```bash
npm install -g eas-cli
eas login
eas build:configure   # only if eas.json is missing/regenerated
```

Build profiles (see `eas.json`):

```bash
# Simulator dev client (fast local iteration on a real build):
eas build --profile development --platform ios

# Internal distribution (ad-hoc / TestFlight-style device install):
eas build --profile preview --platform ios

# App Store / TestFlight:
eas build --profile production --platform ios
eas submit --platform ios   # uploads to App Store Connect → TestFlight
```

Note: `extra.apiBase` in `app.json` must point at a publicly reachable
backend for device builds — `localhost`/LAN IPs won't work once the app
leaves your machine. The push skeleton (`src/notifications.ts`) uses the
Expo push service, so no APNs key is needed until you switch to direct
APNs; device builds need an EAS project id (`eas init` fills
`extra.eas.projectId`).

## Roadmap (not built)

- **Live Activity / widgets** — require an Apple Developer account, a
  native Widget Extension + ActivityKit module (Expo config plugin or
  bare/prebuild native code), and a physical device to verify. No code
  exists for these yet; start from `expo prebuild` and a WidgetKit
  target once the account is available.
- **Android** — out of scope for now.
