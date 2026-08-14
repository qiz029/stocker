export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "ApiError";
  }
}

/* Base URL prefix for API requests. Web serves /api same-origin (default "");
   the RN app calls setApiBase() at boot with the server origin. */
let apiBase = "";

export function setApiBase(base: string) {
  apiBase = base.replace(/\/$/, "");
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(apiBase + path, {
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
  put: <T>(path: string, body?: unknown) => req<T>("PUT", path, body),
  del: <T>(path: string) => req<T>("DELETE", path),
};

/* ---------- API types (mirror the Go handlers exactly) ---------- */
export type AvatarID = "bull" | "bear" | "fox" | "owl" | "shark" | "tiger" | "rocket" | "diamond";
export type SocialLinkKey = "website" | "x" | "github" | "linkedin";
export type User = {
  id: number; username: string; display_name?: string; avatar_id?: AvatarID;
  email?: string; description?: string; social_links?: Partial<Record<SocialLinkKey, string>>;
  profile_complete?: boolean;
};
export type ScenarioInfo = { id: string; name: string; days: number; name_en?: string };
export type Room = {
  id: number; name?: string; invite_code?: string; share_token?: string; scenario_id: string; days: number;
  status: "lobby" | "running"; day_duration_secs: number;
  started_at?: string; current_day?: number; ended?: boolean; is_host?: boolean;
  visibility?: "public" | "private"; is_member?: boolean;
};
export type PublicRoom = Room & {
  human_players: number; max_human_players: number; agent_players: number;
  leader_name?: string; leader_avatar?: AvatarID; leader_return?: number;
};
export type EraLeader = { scenario_id: string; username: string; avatar_id?: AvatarID; return_pct: number; wins: number };
export type InstrumentProfile = { business: string; bull: string; bear: string };
export type Instrument = {
  id: string; alias: string; desc: string; profile: InstrumentProfile | null;
  desc_en?: string; profile_en?: InstrumentProfile | null;   // English mirrors of the zh fields
};
export type Quote = { instrument_id: string; close: number; prev_close: number };
export type LeaderboardRow = {
  username: string; username_en?: string; total_cents: number; return_pct: number; late_join: boolean;
  bankrupt: boolean; curve: number[]; is_agent?: boolean; avatar_id?: AvatarID;   // curve: total_cents per sim day
};
export type RoomState = { room: Room; instruments: Instrument[]; quotes: Quote[]; leaderboard: LeaderboardRow[] };
export type OHLC = { open: number; high: number; low: number; close: number };
export type NewsItem = {
  id: number; day: number; media_id: string; headline: string; body: string; cluster_id?: number | null;
  headline_en?: string; body_en?: string;   // English mirrors of the zh fields
  disputed?: boolean;   // public: at least one player investigated this item
  exposed?: boolean;    // public: a manipulation bust was tied to this item
};
export type MediaAccuracyStat = { reports: number; hits: number };
export type MediaAccuracy = Record<string, MediaAccuracyStat>;
export type NewsResponse = { items: NewsItem[]; media_accuracy?: MediaAccuracy };
export type ForumItem = {
  id: number; day: number; npc_name: string; body: string;
  npc_name_en?: string; body_en?: string; is_agent?: boolean;   // English mirrors of the zh fields
};

/** Incremental news page (cursor = last seen id); carries per-outlet accuracy stats. */
export function fetchNews(roomID: string, after: number): Promise<NewsResponse> {
  return api.get<NewsResponse>(`/api/rooms/${roomID}/news?after=${after}`);
}

/** Fetch one published story directly; future stories remain server-hidden. */
export function fetchNewsItem(roomID: string, newsID: string): Promise<NewsItem> {
  return api.get<NewsItem>(`/api/rooms/${roomID}/news/${newsID}`);
}

/** Incremental forum page (cursor = last seen id). */
export function fetchForum(roomID: string, after: number): Promise<{ items: ForumItem[] }> {
  return api.get<{ items: ForumItem[] }>(`/api/rooms/${roomID}/forum?after=${after}`);
}
/** Event payloads vary by kind: whale carries instrument_id+side,
    manipulation_bust carries username+fine_cents+instrument_id. */
export type EventItem = {
  id: number; day: number; kind: string;
  payload: { instrument_id?: string; side?: string; username?: string; username_en?: string; fine_cents?: number; is_agent?: boolean; order_id?: number };
};
export type ChatMessage = {
  id: number; username: string; username_en?: string; is_agent: boolean; avatar_id?: AvatarID; is_me?: boolean; day: number; text: string; text_en?: string;
};
export type Position = {
  instrument_id: string; shares: number; close: number; value_cents: number;
  avg_cost: number; pnl_cents: number; pnl_pct: number;
};
export type PendingOrder = { id: number; instrument_id: string; side: string; amount_cents: number; shares: number; exec_day: number };
export type OptionKind = "call" | "put";
export type OptionContract = {
  option_id: number; kind: OptionKind; strike: number; expiry_day: number;
  price: number;   // current-day Black-Scholes price, per share
};
export type OptionPosition = {
  option_id: number; instrument_id: string; kind: OptionKind; strike: number; expiry_day: number;
  contracts: number; price: number;          // price: current BS per share
  value_cents: number; avg_cost: number; pnl_cents: number; pnl_pct: number;
};
export type OptionAction = "buy" | "sell";
export type OptionOrderResponse = {
  action: OptionAction; contracts: number; price: number; amount_cents: number; cash_cents: number;
};
export type Portfolio = {
  cash_cents: number; total_cents: number;   // total_cents is NET of debt
  debt_cents: number; max_debt_cents: number;
  interest_rate_annual_bp: number;           // e.g. 300 = 3.00%
  bankrupt: boolean;
  positions: Position[]; pending: PendingOrder[];
  options?: OptionPosition[];
};

/** List live option contracts for one instrument (expiry > current day). */
export function fetchOptions(roomID: string, instrumentID: string): Promise<OptionContract[]> {
  return api.get<OptionContract[]>(`/api/rooms/${roomID}/options?instrument_id=${instrumentID}`);
}

/** Buy to open / sell to close option contracts. Throws ApiError(400) with the server message. */
export function postOptionOrder(roomID: string, optionID: number, action: OptionAction, contracts: number): Promise<OptionOrderResponse> {
  return api.post<OptionOrderResponse>(`/api/rooms/${roomID}/options/orders`, { option_id: optionID, action, contracts });
}
export type LoanAction = "borrow" | "repay";
export type LoanResponse = {
  action: LoanAction; amount_cents: number;
  cash_cents: number; debt_cents: number; max_debt_cents: number;
};

/** Borrow against / repay the room credit line. Throws ApiError(400) on cap/cash violations. */
export function postLoan(roomID: string, action: LoanAction, amountCents: number): Promise<LoanResponse> {
  return api.post<LoanResponse>(`/api/rooms/${roomID}/loans`, { action, amount_cents: amountCents });
}

/* ---------- player actions (hype / debunk / intel) ---------- */
export type HypeDirection = "up" | "down";
export type HypeTier = 1 | 2 | 3;
export type HypeResponse = { fee_cents: number; caught: boolean; fine_cents: number; cash_cents: number };

/** Paid manipulation planting a next-day price shock. One hype per player per day.
    Throws ApiError(400) on insufficient cash / daily limit. */
export function postHype(roomID: string, instrumentID: string, direction: HypeDirection, tier: HypeTier): Promise<HypeResponse> {
  return api.post<HypeResponse>(`/api/rooms/${roomID}/actions/hype`,
    { instrument_id: instrumentID, direction, tier });
}

export type DebunkVerdict = "likely_true" | "likely_false" | "no_substance";
export type DebunkResponse = { verdict: DebunkVerdict; fee_cents: number; cash_cents: number };

/** Paid investigation of one news item; verdict is private to the caller.
    Throws ApiError(400) when the item was already disputed. */
export function postDebunk(roomID: string, newsID: number): Promise<DebunkResponse> {
  return api.post<DebunkResponse>(`/api/rooms/${roomID}/actions/debunk`, { news_id: newsID });
}

export type IntelOutlook = "up" | "down" | "quiet";
export type IntelStrength = "strong" | "medium" | "weak";
export type IntelResponse = {
  outlook: IntelOutlook; strength: IntelStrength | null;
  fee_cents: number; cash_cents: number;
};

/** Rumor-grade peek at tomorrow's move on one instrument (1 per instrument per day).
    Noisy by design — never present the result as certain. */
export function postIntel(roomID: string, instrumentID: string): Promise<IntelResponse> {
  return api.post<IntelResponse>(`/api/rooms/${roomID}/actions/intel`, { instrument_id: instrumentID });
}
export type Trade = { instrument_id: string; side: string; day: number; price: number; shares: number; amount_cents: number };
export type RevealInstrument = { id: string; alias: string; real_name: string };
export type RevealTrade = Trade & { username: string; username_en?: string; is_agent?: boolean };
export type RevealData = { instruments: RevealInstrument[]; trades: RevealTrade[]; leaderboard: LeaderboardRow[]; real_period?: string };

export const INITIAL_CASH_CENTS = 10_000_000;
/* Media outlet display names are translated via i18n keys `media.<id>`
   (see mediaName() in i18n.tsx). */
