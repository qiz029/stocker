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
