import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserCtxForTest } from "../App";
import Lobby from "./Lobby";

const rooms = [
  { id: 1, invite_code: "ABC", scenario_id: "synthetic-v1", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-26T12:00:00Z", current_day: 150, ended: false },
  { id: 2, invite_code: "DEF", scenario_id: "synthetic-v1", days: 300, status: "running",
    day_duration_secs: 60, started_at: "2026-07-01T12:00:00Z", current_day: 299, ended: true },
  { id: 3, invite_code: "GHI", scenario_id: "synthetic-v1", days: 300, status: "lobby",
    day_duration_secs: 4032 },
];

function mockRoutes(handler: (url: string, init?: RequestInit) => unknown) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) =>
    new Response(JSON.stringify(handler(String(url), init as RequestInit)), { status: 200 }));
}

afterEach(() => vi.restoreAllMocks());

describe("Lobby", () => {
  it("orders the production eras chronologically from left to right", async () => {
    mockRoutes((url) => url === "/api/scenarios" ? { items: [
      { id: "dotcom-2000", name: "2000 互联网泡沫", days: 750 },
      { id: "gfc-2008", name: "2008 金融危机", days: 815 },
      { id: "nifty-1972", name: "1972 漂亮50", days: 875 },
      { id: "crash-1987", name: "1987 黑色星期一", days: 756 },
    ] } : { rooms: [] });
    render(
      <MemoryRouter>
        <UserCtxForTest.Provider value={{ id: 1, username: "alice", profile_complete: true }}>
          <Lobby />
        </UserCtxForTest.Provider>
      </MemoryRouter>
    );

    fireEvent.click(await screen.findByRole("button", { name: "＋ Create new room" }));
    const timeline = await screen.findByRole("radiogroup", { name: "Choose an era" });
    const eras = within(timeline).getAllByRole("radio");

    expect(eras.map(era => era.getAttribute("data-era-year"))).toEqual(["1972", "1987", "2000", "2008"]);
    expect(eras[0]).toHaveAttribute("aria-checked", "true");

    eras[0]!.focus();
    fireEvent.keyDown(eras[0]!, { key: "ArrowRight" });
    expect(eras[1]).toHaveAttribute("aria-checked", "true");
    expect(eras[1]).toHaveFocus();
  });

  it.each([1, 3, 5])("adapts the timeline to %i available scenarios", async (count) => {
    const available = [
      { id: "future-2015", name: "2015 Future Market", days: 600 },
      { id: "dotcom-2000", name: "2000 Dot-com Bubble", days: 750 },
      { id: "gfc-2008", name: "2008 Financial Crisis", days: 815 },
      { id: "nifty-1972", name: "1972 Nifty Fifty", days: 875 },
      { id: "crash-1987", name: "1987 Black Monday", days: 756 },
    ].slice(0, count);
    mockRoutes((url) => url === "/api/scenarios" ? { items: available } : { rooms: [] });
    render(
      <MemoryRouter>
        <UserCtxForTest.Provider value={{ id: 1, username: "alice", profile_complete: true }}>
          <Lobby />
        </UserCtxForTest.Provider>
      </MemoryRouter>
    );

    fireEvent.click(await screen.findByRole("button", { name: "＋ Create new room" }));
    const timeline = await screen.findByRole("radiogroup", { name: "Choose an era" });

    expect(timeline).toHaveAttribute("data-era-count", String(count));
    expect(timeline.style.getPropertyValue("--era-count")).toBe(String(count));
    expect(timeline.querySelectorAll(".era-link")).toHaveLength(Math.max(0, count - 1));
  });

  it("lists my rooms with status tags", async () => {
    mockRoutes((url) => url === "/api/scenarios" ? { items: [
      { id: "dotcom-2000", name: "2000 互联网泡沫", days: 750 },
      { id: "synthetic-v1", name: "合成测试剧本", days: 300 },
    ] } : { rooms });
    render(
      <MemoryRouter>
        <UserCtxForTest.Provider value={{ id: 1, username: "alice", profile_complete: true }}>
          <Lobby />
        </UserCtxForTest.Provider>
      </MemoryRouter>
    );
    await waitFor(() => expect(screen.getAllByText("Day 150/300").length).toBeGreaterThan(0));
    expect(screen.getAllByText("ENDED").length).toBeGreaterThan(0);
    expect(screen.getAllByText("WAITING").length).toBeGreaterThan(0);
  });

  it("joins a room by invite code", async () => {
    const calls: string[] = [];
    mockRoutes((url, init) => {
      calls.push(`${init?.method ?? "GET"} ${url}`);
      if (url === "/api/scenarios") return { items: [
        { id: "dotcom-2000", name: "2000 互联网泡沫", days: 750 },
        { id: "synthetic-v1", name: "合成测试剧本", days: 300 },
      ] };
      if (url === "/api/rooms/join") return rooms[0];
      return { rooms: [] };
    });
    render(
      <MemoryRouter>
        <UserCtxForTest.Provider value={{ id: 1, username: "alice", profile_complete: true }}>
          <Lobby />
        </UserCtxForTest.Provider>
      </MemoryRouter>
    );
    fireEvent.change(await screen.findByPlaceholderText("Enter invite code"), { target: { value: "ABC" } });
    fireEvent.click(screen.getByRole("button", { name: "Join" }));
    await waitFor(() => expect(calls).toContain("POST /api/rooms/join"));
  });

  it("creates a room with the selected scenario and computed duration", async () => {
    const bodies: unknown[] = [];
    mockRoutes((url, init) => {
      if (url === "/api/scenarios") return { items: [
        { id: "dotcom-2000", name: "2000 互联网泡沫", days: 750 },
        { id: "synthetic-v1", name: "合成测试剧本", days: 300 },
      ] };
      if (url === "/api/rooms" && init?.method === "POST") {
        bodies.push(JSON.parse(String(init.body)));
        return rooms[2];
      }
      return { rooms: [] };
    });
    render(<MemoryRouter><UserCtxForTest.Provider value={{ id: 1, username: "me", profile_complete: true }}><Lobby /></UserCtxForTest.Provider></MemoryRouter>);
    fireEvent.click(await screen.findByRole("button", { name: "＋ Create new room" }));
    // scenario defaults to the earliest dated entry; pick the 2-week duration
    fireEvent.change(screen.getByRole("combobox", { name: "Game duration" }),
      { target: { value: String(Math.round(2 * 604800 / 750)) } });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(bodies).toEqual([
      { scenario_id: "dotcom-2000", day_duration_secs: Math.round(2 * 604800 / 750), visibility: "public" }]));
  });

  it("collects a player identity before opening the create-room dialog", async () => {
    const calls: string[] = [];
    mockRoutes((url, init) => {
      calls.push(`${init?.method ?? "GET"} ${url}`);
      if (url === "/api/scenarios") return { items: [{ id: "synthetic-v1", name: "Synthetic", days: 300 }] };
      if (url === "/api/me/profile") return { id: 1, username: "alice", display_name: "Market Owl", avatar_id: "owl", profile_complete: true };
      if (url === "/api/leaderboards/eras") return { items: [] };
      return { rooms: [] };
    });
    render(<MemoryRouter><UserCtxForTest.Provider value={{ id: 1, username: "alice", profile_complete: false }}><Lobby /></UserCtxForTest.Provider></MemoryRouter>);

    fireEvent.click(await screen.findByRole("button", { name: "＋ Create new room" }));
    expect(screen.getByRole("heading", { name: "Choose your player identity" })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Display name"), { target: { value: "Market Owl" } });
    fireEvent.click(screen.getByRole("button", { name: "owl" }));
    fireEvent.click(screen.getByRole("button", { name: "Save & continue" }));

    expect(await screen.findByRole("heading", { name: "Open a public timeline" })).toBeInTheDocument();
    expect(calls).toContain("PUT /api/me/profile");
  });

  it("requires a display name and avatar before joining a public waiting room", async () => {
    const calls: { url: string; method: string; body?: unknown }[] = [];
    const waiting = { ...rooms[2], visibility: "public", human_players: 1, max_human_players: 12, agent_players: 5 };
    mockRoutes((url, init) => {
      calls.push({ url, method: init?.method ?? "GET", body: init?.body ? JSON.parse(String(init.body)) : undefined });
      if (url === "/api/scenarios") return { items: [{ id: "synthetic-v1", name: "Synthetic", days: 300 }] };
      if (url === "/api/rooms/public") return { rooms: [waiting] };
      if (url === "/api/leaderboards/eras") return { items: [] };
      if (url === "/api/me/profile") return { id: 1, username: "alice", display_name: "Market Fox", avatar_id: "fox", profile_complete: true };
      if (url === "/api/rooms/3/join") return waiting;
      return { rooms: [] };
    });
    render(<MemoryRouter><UserCtxForTest.Provider value={{ id: 1, username: "alice", profile_complete: false }}><Lobby /></UserCtxForTest.Provider></MemoryRouter>);
    const room = await screen.findByText("Room #3");
    fireEvent.click(room.closest(".hall-room")!.querySelector(".hall-room-action")!);
    fireEvent.change(screen.getByLabelText("Display name"), { target: { value: "Market Fox" } });
    fireEvent.click(screen.getByRole("button", { name: "fox" }));
    fireEvent.click(screen.getByRole("button", { name: "Save & join" }));
    await waitFor(() => expect(calls.some(call => call.url === "/api/me/profile" && call.method === "PUT")).toBe(true));
    await waitFor(() => expect(calls.some(call => call.url === "/api/rooms/3/join" && call.method === "POST")).toBe(true));
  });
});
