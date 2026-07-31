import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserCtxForTest } from "../App";
import Stock from "./Stock";

const state = {
  room: { id: 1, invite_code: "A", scenario_id: "s", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-26T12:00:00Z", current_day: 2, ended: false },
  instruments: [{ id: "S1", alias: "Ridgeline Networks", desc: "网络设备巨头",
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
      else if (u.endsWith("/portfolio")) body = { cash_cents: 10_000_000, total_cents: 10_000_000,
        debt_cents: 0, max_debt_cents: 20_000_000, interest_rate_annual_bp: 300, bankrupt: false,
        positions: [], pending: [] };
      else if (u.endsWith("/trades")) body = { items: [] };
      else if (u.includes("/options")) body = [];
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
    await waitFor(() => expect(screen.getByText(/Ridgeline Networks/)).toBeInTheDocument());
    // $110.00 appears in the hero AND the 今日收盘 stat; +10.00% in the hero
    // delta AND 开局至今 — use getAllByText for both.
    expect(screen.getAllByText("$110.00").length).toBeGreaterThan(0);
    expect(screen.getByText("Prev close")).toBeInTheDocument();
    expect(screen.getAllByText(/\+10\.00%/).length).toBeGreaterThan(0);
    expect(screen.getByText("卖铲人逻辑")).toBeInTheDocument();   // profile bull
    expect(screen.getByText("Place buy")).toBeInTheDocument();     // trade panel
  });

  it("shows English instrument/news fields in en mode, falling back to zh when missing", async () => {
    const enState = {
      ...state,
      instruments: [{ id: "S1", alias: "Ridgeline Networks", desc: "网络设备巨头",
        desc_en: "Networking gear giant",
        profile: { business: "路由器业务", bull: "卖铲人逻辑", bear: "客户都在烧钱" },
        profile_en: { business: "Router business", bull: "Picks-and-shovels play", bear: "" } }],
    };
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = enState;
      else if (u.endsWith("/portfolio")) body = { cash_cents: 10_000_000, total_cents: 10_000_000,
        debt_cents: 0, max_debt_cents: 20_000_000, interest_rate_annual_bp: 300, bankrupt: false,
        positions: [], pending: [] };
      else if (u.endsWith("/trades")) body = { items: [] };
      else if (u.includes("/options")) body = [];
      else if (u.includes("/news")) body = { items: [
        { id: 1, day: 2, media_id: "wire", headline: "S1板块承压", body: "中文正文。",
          headline_en: "S1 sector under pressure", body_en: "English body." },
        { id: 2, day: 2, media_id: "paper", headline: "S1再创新高", body: "" }] };
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
    // en mode: English desc + profile fields (empty bear falls back to zh)
    await waitFor(() => expect(screen.getByText("Networking gear giant")).toBeInTheDocument());
    expect(screen.getByText("Router business")).toBeInTheDocument();
    expect(screen.getByText("Picks-and-shovels play")).toBeInTheDocument();
    expect(screen.getByText("客户都在烧钱")).toBeInTheDocument();
    // related news: English headline (S1 prettified to the alias) + body
    expect(screen.getByText("Ridgeline Networks sector under pressure")).toBeInTheDocument();
    expect(screen.getByText("English body.")).toBeInTheDocument();
    // item without English fields shows the Chinese original
    expect(screen.getByText(/Ridgeline Networks再创新高/)).toBeInTheDocument();
  });

  it("renders option positions with P&L and sells to close", async () => {
    const posted: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const u = String(url);
      if (init?.method === "POST") {
        posted.push(JSON.parse(String(init.body)));
        return new Response(JSON.stringify(
          { action: "sell", contracts: 3, price: 1.2, amount_cents: 360, cash_cents: 10_000_360 }),
          { status: 200 });
      }
      let body: unknown = { items: [] };
      if (u === "/api/rooms/1") body = state;
      else if (u.endsWith("/portfolio")) body = { cash_cents: 10_000_000, total_cents: 10_000_360,
        debt_cents: 0, max_debt_cents: 20_000_000, interest_rate_annual_bp: 300, bankrupt: false,
        positions: [], pending: [],
        options: [{ option_id: 7, instrument_id: "S1", kind: "call", strike: 120, expiry_day: 10,
          contracts: 3, price: 1.2, value_cents: 360, avg_cost: 1.0, pnl_cents: 60, pnl_pct: 0.2 }] };
      else if (u.endsWith("/trades")) body = { items: [] };
      else if (u.includes("/options")) body = [];
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
    await waitFor(() =>
      expect(screen.getByText("Ridgeline Networks Call $120.00 · expires Day 10 (8d)")).toBeInTheDocument());
    expect(screen.getByText("My options")).toBeInTheDocument();
    expect(screen.getByText("3 contracts · value $3.60")).toBeInTheDocument();
    expect(screen.getByText("Avg $1.00 · P&L +$0.60 (+20.00%)")).toBeInTheDocument();
    expect(screen.getByText("Options chain")).toBeInTheDocument();

    // empty input defaults to selling all held contracts
    fireEvent.click(screen.getByRole("button", { name: "Sell to close" }));
    await waitFor(() => expect(posted).toEqual([{ option_id: 7, action: "sell", contracts: 3 }]));
  });
});
