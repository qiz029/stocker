import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import Login from "./Login";

afterEach(() => vi.restoreAllMocks());

describe("Login page", () => {
  it("logs in and calls onAuthed", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ id: 1, username: "alice" }), { status: 200 }),
    );
    const onAuthed = vi.fn();
    render(<MemoryRouter><Login onAuthed={onAuthed} /></MemoryRouter>);

    fireEvent.change(screen.getByPlaceholderText("Username"), { target: { value: "alice" } });
    fireEvent.change(screen.getByPlaceholderText("Password"), { target: { value: "password123" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));

    await waitFor(() => expect(onAuthed).toHaveBeenCalledWith({ id: 1, username: "alice" }));
    expect(fetchSpy.mock.calls[0]![0]).toBe("/api/login");
  });

  it("shows the server error on failure", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "invalid username or password" }), { status: 401 }),
    );
    render(<MemoryRouter><Login onAuthed={vi.fn()} /></MemoryRouter>);
    fireEvent.change(screen.getByPlaceholderText("Username"), { target: { value: "alice" } });
    fireEvent.change(screen.getByPlaceholderText("Password"), { target: { value: "wrong-pass" } });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByText("invalid username or password")).toBeInTheDocument();
  });

  it("switches to register mode", async () => {
    render(<MemoryRouter><Login onAuthed={vi.fn()} /></MemoryRouter>);
    fireEvent.click(screen.getByRole("button", { name: "Create an account" }));
    expect(screen.getByRole("button", { name: "Sign up" })).toBeInTheDocument();
  });
});
