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
