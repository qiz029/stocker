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
export type ScenarioInfo = { id: string; name: string; days: number };
export type Room = {
  id: number; invite_code: string; scenario_id: string; days: number;
  status: "lobby" | "running"; day_duration_secs: number;
  started_at?: string; current_day?: number; ended?: boolean; is_host?: boolean;
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
export type RevealData = { instruments: RevealInstrument[]; trades: RevealTrade[]; leaderboard: LeaderboardRow[]; real_period?: string };

export const INITIAL_CASH_CENTS = 10_000_000;
export const MEDIA_NAMES: Record<string, string> = {
  wire: "通讯社", paper: "财经日报", tv: "财经频道", tabloid: "市场小报", forum: "股友论坛",
};
