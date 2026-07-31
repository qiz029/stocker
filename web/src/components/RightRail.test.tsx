import { render, screen, fireEvent, within, act } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserProviderForTest } from "./testutil";
import RightRail from "./RightRail";
import type { RoomState } from "../api";

const state: RoomState = {
  room: { id: 1, invite_code: "A", scenario_id: "s", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-26T12:00:00Z", current_day: 5, ended: false },
  instruments: [{ id: "S1", alias: "Ridgeline Networks", desc: "", profile: null }],
  quotes: [],
  leaderboard: [
    { username: "me", total_cents: 12_000_000, return_pct: 0.2, late_join: false,
      bankrupt: false, curve: [10_000_000, 11_000_000, 12_000_000] },
    { username: "amy", total_cents: 9_000_000, return_pct: -0.1, late_join: true,
      bankrupt: false, curve: [10_000_000, 9_500_000, 9_000_000] },
    { username: "bob", total_cents: 0, return_pct: -1, late_join: false,
      bankrupt: true, curve: [10_000_000, 5_000_000, 0] },
  ],
};

afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });

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
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    // leaderboard: own row highlighted, late-join marked
    expect(await screen.findByText("me")).toBeInTheDocument();
    expect(screen.getByText("me").closest(".lb-row")).toHaveClass("me");
    expect(screen.getByText("Late join")).toBeInTheDocument();
    expect(screen.getByText("+20.00%")).toBeInTheDocument();

    // per-player asset curve sparklines on every row
    expect(document.querySelectorAll(".lb-row canvas").length).toBe(3);

    // bankrupt player: dimmed row + badge
    expect(screen.getByText("Bankrupt")).toBeInTheDocument();
    expect(screen.getByText("bob").closest(".lb-row")).toHaveClass("bankrupt");

    // whale
    expect(await screen.findByText(/large buy of Ridgeline Networks/)).toBeInTheDocument();

    // news headline prettified + body expands on click (jsdom loads no CSS,
    // so assert the state class rather than computed visibility)
    const headline = await screen.findByText(/Ridgeline Networks板块承压/);
    const item = headline.closest(".feed-item")!;
    expect(item).not.toHaveClass("open");
    fireEvent.click(headline);
    expect(item).toHaveClass("open");
    expect(screen.getByText("正文内容。")).toBeInTheDocument();
  });
});

describe("RightRail news enrichment", () => {
  it("groups clustered news into one story chain with role labels and accuracy badges", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u.includes("/news")) body = {
        items: [
          { id: 1, day: 3, media_id: "tabloid", headline: "传闻：S1将停牌", body: "传闻正文。", cluster_id: 7 },
          { id: 2, day: 4, media_id: "wire", headline: "S1宣布重大资产重组", body: "主事件正文。", cluster_id: 7 },
          { id: 3, day: 5, media_id: "paper", headline: "S1重组追踪：监管问询", body: "", cluster_id: 7 },
          { id: 4, day: 5, media_id: "tv", headline: "S2股价创新高", body: "", cluster_id: null },
        ],
        media_accuracy: {
          tabloid: { reports: 2, hits: 2 },   // < 3 reports → insufficient data
          wire: { reports: 5, hits: 4 },      // 80%
          paper: { reports: 10, hits: 6 },    // 60%
        },
      };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    // the three clustered items render inside one .news-chain, in day order
    const chain = (await screen.findByText("Rumor")).closest(".news-chain")!;
    expect(chain).not.toBeNull();
    expect(within(chain as HTMLElement).getByText("Report")).toBeInTheDocument();
    expect(within(chain as HTMLElement).getByText("Follow-up")).toBeInTheDocument();
    const titles = within(chain as HTMLElement).getAllByText(/Ridgeline Networks/).map(el => el.textContent);
    expect(titles).toEqual([
      "传闻：Ridgeline Networks将停牌",
      "Ridgeline Networks宣布重大资产重组",
      "Ridgeline Networks重组追踪：监管问询",
    ]);

    // body expand still works inside a chain
    const rumorHeadline = within(chain as HTMLElement).getByText("传闻：Ridgeline Networks将停牌");
    const rumorItem = rumorHeadline.closest(".feed-item")!;
    expect(rumorItem).not.toHaveClass("open");
    fireEvent.click(rumorHeadline);
    expect(rumorItem).toHaveClass("open");

    // accuracy: rounded pct for outlets with enough reports, hint otherwise
    expect(screen.getAllByText(/80% accurate/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/60% accurate/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/not enough data/).length).toBeGreaterThan(0);

    // standalone item (cluster_id: null) renders outside any chain, no badge
    const solo = screen.getByText("Ridgeline Networks股价创新高").closest(".feed-item")!;
    expect(solo.closest(".news-chain")).toBeNull();
    expect(solo.querySelector(".acc")).toBeNull();   // tv has no accuracy stats
  });

  it("forum tab renders NPC posts and polls incrementally with the cursor", async () => {
    vi.useFakeTimers();
    const forumCalls: string[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u.includes("/forum")) {
        forumCalls.push(u);
        const after = new URL(u, "http://x").searchParams.get("after");
        body = after === "0"
          ? { items: [{ id: 1, day: 4, npc_name: "老韭菜", body: "这票我看着要涨" }] }
          : { items: [{ id: 2, day: 5, npc_name: "量化小王", body: "模型提示风险" }] };
      }
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);
    await act(async () => { /* flush mount-time fetches */ });

    // default tab is news; switch to the forum
    fireEvent.click(screen.getByRole("button", { name: "Forum" }));
    expect(screen.getByText("老韭菜")).toBeInTheDocument();
    expect(screen.getByText("这票我看着要涨")).toBeInTheDocument();

    // next 30s tick asks for items after the last-seen id and appends
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    expect(screen.getByText("量化小王")).toBeInTheDocument();
    expect(screen.getByText("模型提示风险")).toBeInTheDocument();
    expect(forumCalls[0]).toContain("after=0");
    expect(forumCalls.some(u => u.includes("after=1"))).toBe(true);
  });

  it("shows English news/forum fields in en mode, falling back to zh when missing", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u.includes("/news")) body = { items: [
        { id: 1, day: 5, media_id: "wire", headline: "S1板块承压", body: "中文正文。",
          headline_en: "S1 sector under pressure", body_en: "English body." },
        { id: 2, day: 5, media_id: "paper", headline: "S1再创新高", body: "" }] };
      if (u.includes("/forum")) body = { items: [
        { id: 1, day: 5, npc_name: "老韭菜", body: "这票要涨",
          npc_name_en: "OldChive", body_en: "This one's going up" },
        { id: 2, day: 5, npc_name: "量化小王", body: "模型提示风险", npc_name_en: null }] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    // news: English headline (S1 prettified to the alias) + body in en mode
    expect(await screen.findByText("Ridgeline Networks sector under pressure")).toBeInTheDocument();
    expect(screen.getByText("English body.")).toBeInTheDocument();
    // missing English fields fall back to the Chinese original
    expect(screen.getByText(/Ridgeline Networks再创新高/)).toBeInTheDocument();

    // forum: English npc name/body, null falls back to zh
    fireEvent.click(screen.getByRole("button", { name: "Forum" }));
    expect(screen.getByText("OldChive")).toBeInTheDocument();
    expect(screen.getByText("This one's going up")).toBeInTheDocument();
    expect(screen.getByText("量化小王")).toBeInTheDocument();
    expect(screen.getByText("模型提示风险")).toBeInTheDocument();
  });

  it("shows the forum empty state when there are no posts", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
      new Response(JSON.stringify({ items: [] }), { status: 200 }));
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);
    fireEvent.click(screen.getByRole("button", { name: "Forum" }));
    expect(await screen.findByText("No forum posts yet")).toBeInTheDocument();
  });
});

describe("RightRail player actions", () => {
  it("investigates a news item and shows the verdict only to the actor", async () => {
    const posted: { url: string; body: unknown }[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const u = String(url);
      if (init?.method === "POST") {
        posted.push({ url: u, body: JSON.parse(String(init.body)) });
        return new Response(JSON.stringify(
          { verdict: "likely_false", fee_cents: 200_000, cash_cents: 9_800_000 }), { status: 200 });
      }
      let body: unknown = { items: [] };
      if (u.includes("/news")) body = { items: [
        { id: 1, day: 5, media_id: "tabloid", headline: "传闻：S1将停牌", body: "正文。" }] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    fireEvent.click(await screen.findByRole("button", { name: "Investigate ($2,000.00)" }));
    expect(await screen.findByText(/Verdict: likely FALSE/)).toBeInTheDocument();
    expect(screen.getByText("only you can see this")).toBeInTheDocument();
    // the item is now publicly disputed and the action is spent
    expect(screen.getByText("Disputed")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Investigate/ })).not.toBeInTheDocument();
    expect(posted).toEqual([{ url: "/api/rooms/1/actions/debunk", body: { news_id: 1 } }]);
  });

  it("toasts the server's 400 when the item was already disputed", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const u = String(url);
      if (init?.method === "POST") {
        return new Response(JSON.stringify({ error: "already disputed" }), { status: 400 });
      }
      let body: unknown = { items: [] };
      if (u.includes("/news")) body = { items: [
        { id: 1, day: 5, media_id: "wire", headline: "S1宣布重组", body: "" }] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    fireEvent.click(await screen.findByRole("button", { name: "Investigate ($2,000.00)" }));
    expect(await screen.findByText("already disputed")).toBeInTheDocument();
    expect(screen.queryByText(/Verdict:/)).not.toBeInTheDocument();
  });

  it("renders disputed/exposed badges and hides the action on disputed items", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u.includes("/news")) body = { items: [
        { id: 1, day: 4, media_id: "wire", headline: "S1业绩大增", body: "", disputed: true },
        { id: 2, day: 5, media_id: "tabloid", headline: "传闻：S1利好", body: "", exposed: true },
      ] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    expect(await screen.findByText("Manipulation confirmed")).toBeInTheDocument();
    expect(screen.getByText("Disputed")).toBeInTheDocument();
    const disputedItem = screen.getByText(/Ridgeline Networks业绩大增/).closest(".feed-item")!;
    expect(within(disputedItem as HTMLElement).queryByRole("button", { name: /Investigate/ })).toBeNull();
    // the merely-exposed item can still be investigated
    const exposedItem = screen.getByText(/传闻：Ridgeline Networks利好/).closest(".feed-item")!;
    expect(within(exposedItem as HTMLElement).getByRole("button", { name: /Investigate/ })).toBeInTheDocument();
  });

  it("renders manipulation_bust events with the fined username", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u.includes("/events")) body = { items: [
        { id: 1, day: 4, kind: "manipulation_bust",
          payload: { username: "amy", fine_cents: 2_000_000, instrument_id: "S1", day: 4 } },
        { id: 2, day: 5, kind: "whale", payload: { instrument_id: "S1", side: "sell" } },
      ] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    const bust = await screen.findByText(/amy was fined \$20,000\.00 for manipulating Ridgeline Networks/);
    expect(bust.closest(".feed-item")).toHaveClass("bust");
    // whale events still render through the old branch
    expect(screen.getByText(/large sell of Ridgeline Networks/)).toBeInTheDocument();
  });

  it("renders bankrupt events instead of falling through to the whale branch", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u.includes("/events")) body = { items: [
        { id: 1, day: 6, kind: "bankrupt", payload: { username: "bob", day: 6 } },
      ] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    expect(await screen.findByText(/bob went bankrupt/)).toBeInTheDocument();
  });
});
