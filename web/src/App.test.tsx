import { fireEvent, render, screen } from "@testing-library/react";
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
    fireEvent.click(screen.getByRole("button", { name: "＋ Create new room" }));
    expect(screen.getByLabelText("Room name")).toHaveValue("市场猫头鹰's Room");
    fireEvent.click(screen.getByRole("button", { name: /^Create$/ }));
    fireEvent.change(screen.getByPlaceholderText("Enter invite code"), { target: { value: "MOCK88" } });
    fireEvent.click(screen.getByRole("button", { name: /^Join$/ }));
    const waitingRoom = screen.getByText("互联网泡沫最后一舞").closest(".hall-room")!;
    fireEvent.click(waitingRoom.querySelector<HTMLButtonElement>(".hall-room-action")!);
    fireEvent.click(screen.getByRole("button", { name: /^Profile$/ }));
    fireEvent.change(screen.getByLabelText("Display name"), { target: { value: "Mock Owl" } });
    fireEvent.click(screen.getByRole("button", { name: /^Save changes$/ }));
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
