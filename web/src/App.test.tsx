import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App";

afterEach(() => {
  window.history.replaceState({}, "", "/");
  localStorage.clear();
  vi.restoreAllMocks();
});

describe("App routes", () => {
  it("serves gameplay docs without requiring a session request", () => {
    window.history.replaceState({}, "", "/docs");
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    render(<App />);

    expect(screen.getByRole("heading", { name: "How to play Stocker" })).toBeInTheDocument();
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("serves a populated development hall without touching the API", async () => {
    window.history.replaceState({}, "", "/?mock=hall");
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    render(<App />);

    expect(await screen.findByText("黑色星期一抄底局")).toBeInTheDocument();
    expect(screen.getByText("互联网泡沫最后一舞")).toBeInTheDocument();
    expect(screen.getByText("Friday Night Traders")).toBeInTheDocument();
    expect(screen.getByText("market_owl")).toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("button", { name: "market_owl account menu" }), { key: "ArrowDown" });
    const accountMenu = screen.getByRole("menu", { name: "Account" });
    expect(accountMenu).toBeInTheDocument();
    const profileItem = within(accountMenu).getByRole("menuitem", { name: "Profile" });
    const logoutItem = within(accountMenu).getByRole("menuitem", { name: "Log out" });
    await waitFor(() => expect(profileItem).toHaveFocus());
    fireEvent.keyDown(profileItem, { key: "ArrowDown" });
    expect(logoutItem).toHaveFocus();
    fireEvent.keyDown(logoutItem, { key: "Home" });
    expect(profileItem).toHaveFocus();
    fireEvent.click(profileItem);
    expect(screen.getByRole("heading", { name: "Your profile" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Back to Market Hall" }));
    await screen.findByText("黑色星期一抄底局");
    fireEvent.click(screen.getByRole("button", { name: "＋ Create new room" }));
    expect(screen.getByLabelText("Room name")).toHaveValue("市场猫头鹰's Room");
    fireEvent.click(screen.getByRole("button", { name: /^Create$/ }));
    fireEvent.change(screen.getByPlaceholderText("Enter invite code"), { target: { value: "MOCK88" } });
    fireEvent.click(screen.getByRole("button", { name: /^Join$/ }));
    const waitingRoom = screen.getByText("互联网泡沫最后一舞").closest(".hall-room")!;
    fireEvent.click(waitingRoom.querySelector<HTMLButtonElement>(".hall-room-action")!);
    fireEvent.click(screen.getByRole("button", { name: "market_owl account menu" }));
    fireEvent.click(within(screen.getByRole("menu", { name: "Account" })).getByRole("menuitem", { name: "Log out" }));
    expect(await screen.findByPlaceholderText("Username")).toBeInTheDocument();
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("keeps the account active when server logout fails", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/api/me") return new Response(JSON.stringify({ id: 1, username: "alice", display_name: "Alice", avatar_id: "owl", profile_complete: true }), { status: 200 });
      if (url === "/api/logout") return new Response(JSON.stringify({ error: "Session store unavailable" }), { status: 500 });
      if (url === "/api/scenarios" || url === "/api/leaderboards/eras") return new Response(JSON.stringify({ items: [] }), { status: 200 });
      return new Response(JSON.stringify({ rooms: [] }), { status: 200 });
    });
    render(<App />);

    fireEvent.click(await screen.findByRole("button", { name: "alice account menu" }));
    fireEvent.click(within(screen.getByRole("menu", { name: "Account" })).getByRole("menuitem", { name: "Log out" }));

    expect(await screen.findByText("Session store unavailable")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "alice account menu" })).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Username")).not.toBeInTheDocument();
  });
});
