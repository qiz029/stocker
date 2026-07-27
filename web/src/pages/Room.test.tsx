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
    // 郊狼网络 appears in both positions and watchlist
    expect(screen.getAllByText("郊狼网络").length).toBeGreaterThan(0);
    expect(screen.getAllByText("老树能源").length).toBeGreaterThan(0);
    expect(screen.getByText("现金")).toBeInTheDocument();
    // +10.00% appears on both the S1 position pill and the S1 watchlist pill
    expect(screen.getAllByText("+10.00%").length).toBeGreaterThan(0);
    // day counter is split across nodes; assert the pill's combined text
    expect(document.querySelector(".day-pill")?.textContent).toContain("第 2 / 300");
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
    expect(await screen.findByText(/等待房主启动/)).toBeInTheDocument();
    expect(screen.queryByText(/启动时间轴/)).not.toBeInTheDocument();
  });
});
