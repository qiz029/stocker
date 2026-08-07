import { render, screen, fireEvent, within, act } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserProviderForTest } from "./testutil";
import RightRail from "./RightRail";
import type { RoomState } from "../api";
import { saveDebunkVerdict } from "../debunkVerdicts";

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
    { username: "Nova", is_agent: true, total_cents: 10_000_000, return_pct: 0, late_join: false,
      bankrupt: false, curve: [10_000_000, 10_000_000, 10_000_000] },
  ],
};

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  sessionStorage.clear();
});

describe("RightRail", () => {
  it("renders leaderboard, whale event and expandable news", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u.includes("/events")) body = { items: [
        { id: 1, day: 4, kind: "whale", payload: { instrument_id: "S1", side: "buy" } },
        { id: 2, day: 5, kind: "agent_order", payload: {
          username: "Nova", is_agent: true, instrument_id: "S1", side: "sell" } }] };
      if (u.includes("/news")) body = { items: [
        { id: 1, day: 5, media_id: "wire", headline: "消息面变化，S1板块承压，市场解读不一", body: "正文内容。",
          headline_en: "S1 comes under pressure", body_en: "Full story." }] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    // leaderboard: own row highlighted, late-join marked
    expect(await screen.findByText("me")).toBeInTheDocument();
    expect(screen.getByText("me").closest(".lb-row")).toHaveClass("me");
    expect(screen.getByText("Late join")).toBeInTheDocument();
    expect(screen.getByText("+20.00%")).toBeInTheDocument();

    // per-player asset curve sparklines on every row
    expect(document.querySelectorAll(".lb-row canvas").length).toBe(4);

    // Agent identity and actions are explicit rather than name-based guesses.
    const leaderboardAgent = screen.getAllByText("Nova").find(node => node.closest(".lb-row"))!;
    expect(leaderboardAgent.closest(".lb-row")).toHaveTextContent("Agent");
    const agentTrade = (await screen.findByText("Sold Ridgeline Networks")).closest(".feed-item")!;
    expect(agentTrade).toHaveTextContent("Nova");
    expect(agentTrade).toHaveTextContent("Agent");

    // bankrupt player: dimmed row + badge
    expect(screen.getByText("Bankrupt")).toBeInTheDocument();
    expect(screen.getByText("bob").closest(".lb-row")).toHaveClass("bankrupt");

    // whale
    expect(await screen.findByText(/large buy of Ridgeline Networks/)).toBeInTheDocument();

    // news headline prettified + body expands on click (jsdom loads no CSS,
    // so assert the state class rather than computed visibility)
    const headline = await screen.findByText(/Ridgeline Networks comes under pressure/);
    const item = headline.closest(".feed-item")!;
    expect(item).not.toHaveClass("open");
    fireEvent.click(headline);
    expect(item).toHaveClass("open");
    expect(screen.getByText("Full story.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Read full article" })).toHaveAttribute("href", "/rooms/1/news/1");
  });
});

describe("RightRail news enrichment", () => {
  it("groups clustered news into one story chain with role labels and accuracy badges", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      let body: unknown = { items: [] };
      if (u.includes("/news")) body = {
        items: [
          { id: 1, day: 3, media_id: "tabloid", headline: "传闻：S1将停牌", body: "传闻正文。", headline_en: "Rumor: S1 may halt", body_en: "Rumor copy.", cluster_id: 7 },
          { id: 2, day: 4, media_id: "wire", headline: "S1宣布重大资产重组", body: "主事件正文。", headline_en: "S1 announces a major restructuring", body_en: "Report copy.", cluster_id: 7 },
          { id: 3, day: 5, media_id: "paper", headline: "S1重组追踪：监管问询", body: "", headline_en: "S1 restructuring follow-up", body_en: "", cluster_id: 7 },
          { id: 4, day: 5, media_id: "tv", headline: "S2股价创新高", body: "", headline_en: "S2 hits a new high", body_en: "", cluster_id: null },
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
      "Rumor: Ridgeline Networks may halt",
      "Ridgeline Networks announces a major restructuring",
      "Ridgeline Networks restructuring follow-up",
    ]);

    // body expand still works inside a chain
    const rumorHeadline = within(chain as HTMLElement).getByText("Rumor: Ridgeline Networks may halt");
    const rumorItem = rumorHeadline.closest(".feed-item")!;
    expect(rumorItem).not.toHaveClass("open");
    fireEvent.click(rumorHeadline);
    expect(rumorItem).toHaveClass("open");

    // accuracy: rounded pct for outlets with enough reports, hint otherwise
    expect(screen.getAllByText(/80% accurate/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/60% accurate/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/not enough data/).length).toBeGreaterThan(0);

    // standalone item (cluster_id: null) renders outside any chain, no badge
    const solo = screen.getByText("Ridgeline Networks hits a new high").closest(".feed-item")!;
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
          ? { items: [{ id: 1, day: 4, npc_name: "老韭菜", body: "这票我看着要涨", npc_name_en: "Old Chive", body_en: "This one looks ready to rise" }] }
          : { items: [{ id: 2, day: 5, npc_name: "量化小王", body: "模型提示风险", npc_name_en: "Quant Kid", body_en: "The model flags risk", is_agent: true }] };
      }
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);
    await act(async () => { /* flush mount-time fetches */ });

    // default tab is news; switch to the forum
    fireEvent.click(screen.getByRole("button", { name: "Forum" }));
    expect(screen.getByText("Old Chive")).toBeInTheDocument();
    expect(screen.getByText("This one looks ready to rise")).toBeInTheDocument();

    // next 30s tick asks for items after the last-seen id and appends
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    expect(screen.getByText("Quant Kid")).toBeInTheDocument();
    expect(screen.getByText("The model flags risk")).toBeInTheDocument();
    expect(screen.getByText("Quant Kid").closest(".feed-item")).toHaveTextContent("Agent");
    expect(forumCalls[0]).toContain("after=0");
    expect(forumCalls.some(u => u.includes("after=1"))).toBe(true);
  });

  it("shows English news/forum fields without leaking zh when English is missing", async () => {
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
    // missing English fields do not expose the Chinese original
    expect(screen.queryByText(/再创新高/)).not.toBeInTheDocument();

    // forum: English npc name/body; untranslated Chinese stays out.
    fireEvent.click(screen.getByRole("button", { name: "Forum" }));
    expect(screen.getByText("OldChive")).toBeInTheDocument();
    expect(screen.getByText("This one's going up")).toBeInTheDocument();
    expect(screen.queryByText("量化小王")).not.toBeInTheDocument();
    expect(screen.queryByText("模型提示风险")).not.toBeInTheDocument();
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
  it("reloads private verdicts when the room changes", async () => {
    saveDebunkVerdict(1, "1", 1, "likely_false");
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const u = String(url);
      const body = u.includes("/news")
        ? { items: [{ id: 1, day: 5, media_id: "wire", headline: "S1重组", body: "正文。", headline_en: "S1 restructuring", body_en: "Story." }] }
        : { items: [] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    const view = render(
      <UserProviderForTest username="me">
        <RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} />
      </UserProviderForTest>,
    );
    expect(await screen.findByText(/Verdict: likely FALSE/)).toBeInTheDocument();

    view.rerender(
      <UserProviderForTest username="me">
        <RightRail roomId="2" state={state} aliasOf={() => "Ridgeline Networks"} />
      </UserProviderForTest>,
    );
    expect(await screen.findByRole("button", { name: "Investigate ($2,000.00)" })).toBeInTheDocument();
    expect(screen.queryByText(/Verdict:/)).not.toBeInTheDocument();
  });

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
        { id: 1, day: 5, media_id: "tabloid", headline: "传闻：S1将停牌", body: "正文。", headline_en: "Rumor: S1 may halt", body_en: "Story." }] };
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
        { id: 1, day: 5, media_id: "wire", headline: "S1宣布重组", body: "", headline_en: "S1 announces restructuring", body_en: "" }] };
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
        { id: 1, day: 4, media_id: "wire", headline: "S1业绩大增", body: "", headline_en: "S1 earnings surge", body_en: "", disputed: true },
        { id: 2, day: 5, media_id: "tabloid", headline: "传闻：S1利好", body: "", headline_en: "Rumor: S1 has a catalyst", body_en: "", exposed: true },
      ] };
      return new Response(JSON.stringify(body), { status: 200 });
    });
    render(<UserProviderForTest username="me"><RightRail roomId="1" state={state} aliasOf={() => "Ridgeline Networks"} /></UserProviderForTest>);

    expect(await screen.findByText("Manipulation confirmed")).toBeInTheDocument();
    expect(screen.getByText("Disputed")).toBeInTheDocument();
    const disputedItem = screen.getByText(/Ridgeline Networks earnings surge/).closest(".feed-item")!;
    expect(within(disputedItem as HTMLElement).queryByRole("button", { name: /Investigate/ })).toBeNull();
    // the merely-exposed item can still be investigated
    const exposedItem = screen.getByText(/Rumor: Ridgeline Networks has a catalyst/).closest(".feed-item")!;
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
