import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserCtxForTest } from "../App";
import Room from "./Room";

const state = {
  room: { id: 1, invite_code: "ABC", scenario_id: "synthetic-v1", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-26T12:00:00Z", current_day: 2, ended: false },
  instruments: [
    { id: "S1", alias: "Ridgeline Networks", desc: "网络设备巨头", profile: null },
    { id: "S6", alias: "Oldfield Energy", desc: "传统油气", profile: null },
  ],
  quotes: [
    { instrument_id: "S1", close: 110, prev_close: 100 },
    { instrument_id: "S6", close: 99, prev_close: 100 },
  ],
  leaderboard: [{ username: "host", total_cents: 10_000_000, return_pct: 0, late_join: false,
    bankrupt: false, curve: [10_000_000, 10_000_000] }],
};
const portfolio = {
  cash_cents: 6_000_000, total_cents: 10_400_000,
  debt_cents: 1_000_000, max_debt_cents: 20_000_000,
  interest_rate_annual_bp: 300, bankrupt: false,
  positions: [{ instrument_id: "S1", shares: 400, close: 110, value_cents: 4_400_000,
    avg_cost: 100, pnl_cents: 400_000, pnl_pct: 0.1 }],
  pending: [],
  options: [{ option_id: 3, instrument_id: "S6", kind: "put", strike: 95, expiry_day: 12,
    contracts: 2, price: 2.0, value_cents: 400, avg_cost: 2.5, pnl_cents: -100, pnl_pct: -0.2 }],
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
    else if (u.includes("/news") || u.includes("/events") || u.includes("/chat") || u.includes("/forum")) body = { items: [] };
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
    await waitFor(() => expect(screen.getAllByText("$104,000.00").length).toBeGreaterThan(0));
    // Ridgeline Networks appears in both positions and watchlist
    expect(screen.getAllByText("Ridgeline Networks").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Oldfield Energy").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Cash").length).toBeGreaterThan(0);
    // +10.00% appears on both the S1 position pill and the S1 watchlist pill
    expect(screen.getAllByText("+10.00%").length).toBeGreaterThan(0);
    // day counter is split across nodes; assert the pill's combined text
    expect(document.querySelector(".day-pill")?.textContent).toContain("Day 2 / 300");
    expect(screen.getByRole("link", { name: "Docs" })).toHaveAttribute("href", "/docs");

    // held-position P&L line: avg cost + unrealized amount and %
    expect(screen.getByText("Avg $100.00 · P&L +$4,000.00 (+10.00%)")).toBeInTheDocument();

    // option holdings section: contract description, value and red P&L
    expect(screen.getByText("My options")).toBeInTheDocument();
    expect(screen.getByText("Oldfield Energy Put $95.00 · expires Day 12 (10d)")).toBeInTheDocument();
    expect(screen.getByText("2 contracts · value $4.00")).toBeInTheDocument();
    expect(screen.getByText("Avg $2.50 · P&L -$1.00 (-20.00%)")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sell to close" })).toBeInTheDocument();

    // asset breakdown card with debt: rate + headroom to the bankruptcy line
    expect(screen.getByText("Asset breakdown")).toBeInTheDocument();
    expect(screen.getByText("Net total")).toBeInTheDocument();
    expect(screen.getByText(/annual rate 3\.00% · \$190,000\.00 of headroom/)).toBeInTheDocument();

    // loan panel in the right rail
    expect(screen.getByText("Credit line")).toBeInTheDocument();
    expect(screen.getByText("Current debt")).toBeInTheDocument();
  });

  it("shows the bankruptcy banner when the player is bankrupt", async () => {
    mockApi();
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = state;
      else if (u.endsWith("/portfolio")) body = { ...portfolio, bankrupt: true, debt_cents: 20_000_000 };
      else if (u.endsWith("/trades")) body = { items: [] };
      else if (u.includes("/prices/")) body = priceDays;
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(
      <MemoryRouter initialEntries={["/rooms/1"]}>
        <UserCtxForTest.Provider value={{ id: 1, username: "me" }}>
          <Routes><Route path="/rooms/:roomId" element={<Room />} /></Routes>
        </UserCtxForTest.Provider>
      </MemoryRouter>,
    );
    expect(await screen.findByText(/Bankrupt — debt crossed the credit line/)).toBeInTheDocument();
  });

  it("shows the start button only to the host", async () => {
    const lobbyState = {
      ...state,
      room: { ...state.room, status: "lobby", started_at: undefined, current_day: undefined, is_host: false },
      quotes: [], leaderboard: [],
    };
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = lobbyState;
      else if (u.endsWith("/portfolio")) body = { cash_cents: 10_000_000, total_cents: 10_000_000, positions: [], pending: [] };
      else if (u.endsWith("/trades")) body = { items: [] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(
      <MemoryRouter initialEntries={["/rooms/1"]}>
        <UserCtxForTest.Provider value={{ id: 1, username: "me" }}>
          <Routes><Route path="/rooms/:roomId" element={<Room />} /></Routes>
        </UserCtxForTest.Provider>
      </MemoryRouter>,
    );
    expect(await screen.findByText(/Waiting for the host/)).toBeInTheDocument();
    expect(screen.queryByText(/Start timeline/)).not.toBeInTheDocument();
  });

  it("shows the sim market clock while a day is open", async () => {
    mockApi();
    const openState = {
      ...state,
      room: { ...state.room, started_at: new Date(Date.now() - 1000).toISOString(), current_day: 0 },
    };
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = openState;
      else if (u.endsWith("/portfolio")) body = portfolio;
      else if (u.endsWith("/trades")) body = { items: [] };
      else if (u.includes("/prices/")) body = priceDays;
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(
      <MemoryRouter initialEntries={["/rooms/1"]}>
        <UserCtxForTest.Provider value={{ id: 1, username: "me" }}>
          <Routes><Route path="/rooms/:roomId" element={<Room />} /></Routes>
        </UserCtxForTest.Provider>
      </MemoryRouter>,
    );
    // day 0 开盘:虚构日历 + 盘中时刻(9:30 起跳)
    await waitFor(() => {
      const el = document.querySelector(".countdown");
      expect(el?.textContent).toContain("Week 1 · Mon");
      expect(el?.textContent).toMatch(/\d{1,2}:\d{2}/);
    });
  });

  it("renders public spectators read-only without private portfolio requests", async () => {
    const calls: string[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url); calls.push(u);
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = { ...state, room: { ...state.room, visibility: "public", is_member: false, invite_code: undefined } };
      else if (u.includes("/prices/")) body = priceDays;
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(
      <MemoryRouter initialEntries={["/rooms/1"]}>
        <UserCtxForTest.Provider value={{ id: 9, username: "viewer", profile_complete: false }}>
          <Routes><Route path="/rooms/:roomId" element={<Room />} /></Routes>
        </UserCtxForTest.Provider>
      </MemoryRouter>,
    );
    expect(await screen.findByText("Spectator mode")).toBeInTheDocument();
    expect(screen.getByText(/cannot trade or post/)).toBeInTheDocument();
    expect(screen.queryByText("Asset breakdown")).not.toBeInTheDocument();
    expect(screen.queryByText("Credit line")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Invite friends" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Send" })).not.toBeInTheDocument();
    expect(calls.some(url => url.endsWith("/portfolio") || url.endsWith("/trades"))).toBe(false);
  });

  it("lets a waiting-room spectator complete a profile and join", async () => {
    const calls: { url: string; method: string }[] = [];
    const waitingState = { ...state, room: { ...state.room, status: "lobby", started_at: undefined, current_day: undefined, visibility: "public", is_member: false, invite_code: undefined }, quotes: [], leaderboard: [] };
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const u = String(url); calls.push({ url: u, method: init?.method ?? "GET" });
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = waitingState;
      else if (u === "/api/me/profile") body = { id: 9, username: "viewer", display_name: "Night Owl", avatar_id: "owl", profile_complete: true };
      else if (u === "/api/rooms/1/join") body = { ...waitingState.room, is_member: true };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(
      <MemoryRouter initialEntries={["/rooms/1"]}>
        <UserCtxForTest.Provider value={{ id: 9, username: "viewer", profile_complete: false }}>
          <Routes><Route path="/rooms/:roomId" element={<Room />} /></Routes>
        </UserCtxForTest.Provider>
      </MemoryRouter>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Join game" }));
    fireEvent.change(screen.getByLabelText("Display name"), { target: { value: "Night Owl" } });
    fireEvent.click(screen.getByRole("button", { name: "owl" }));
    fireEvent.click(screen.getByRole("button", { name: "Save & join" }));
    await waitFor(() => expect(calls.some(call => call.url === "/api/me/profile" && call.method === "PUT")).toBe(true));
    await waitFor(() => expect(calls.some(call => call.url === "/api/rooms/1/join" && call.method === "POST")).toBe(true));
  });
});
