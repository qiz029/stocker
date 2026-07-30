import { render, screen, waitFor, fireEvent } from "@testing-library/react";
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
  it("lists my rooms with status tags", async () => {
    mockRoutes((url) => url === "/api/scenarios" ? { items: [
      { id: "dotcom-2000", name: "2000 互联网泡沫", days: 750 },
      { id: "synthetic-v1", name: "合成测试剧本", days: 300 },
    ] } : { rooms });
    render(
      <MemoryRouter>
        <UserCtxForTest.Provider value={{ id: 1, username: "alice" }}>
          <Lobby />
        </UserCtxForTest.Provider>
      </MemoryRouter>
    );
    // day counter is split across nodes (第 <b>150</b> / 300) — assert the bold number
    await waitFor(() => expect(screen.getByText("150")).toBeInTheDocument());
    expect(screen.getByText("Ended")).toBeInTheDocument();
    expect(screen.getByText("Waiting to start")).toBeInTheDocument();
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
        <UserCtxForTest.Provider value={{ id: 1, username: "alice" }}>
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
    render(<MemoryRouter><UserCtxForTest.Provider value={{ id: 1, username: "me" }}><Lobby /></UserCtxForTest.Provider></MemoryRouter>);
    fireEvent.click(await screen.findByRole("button", { name: "＋ Create new room" }));
    // scenario defaults to the first entry; pick the 2-week duration
    const selects = screen.getAllByRole("combobox");
    fireEvent.change(selects[1], { target: { value: String(Math.round(2 * 604800 / 750)) } });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(bodies).toEqual([
      { scenario_id: "dotcom-2000", day_duration_secs: Math.round(2 * 604800 / 750) }]));
  });
});
