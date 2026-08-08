import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UserCtxForTest } from "../App";
import Profile from "./Profile";

afterEach(() => vi.restoreAllMocks());

describe("Profile page", () => {
  it("edits the extended public profile and changes the password", async () => {
    const calls: { url: string; method: string; body?: unknown }[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const url = String(input);
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ url, method: init?.method ?? "GET", body });
      if (url === "/api/me/profile") return new Response(JSON.stringify({
        id: 1, username: "alice", display_name: body.display_name, avatar_id: body.avatar_id,
        email: body.email, description: body.description, social_links: body.social_links, profile_complete: true,
      }), { status: 200 });
      return new Response(JSON.stringify({ status: "ok" }), { status: 200 });
    });

    render(<MemoryRouter><UserCtxForTest.Provider value={{
      id: 1, username: "alice", display_name: "Market Owl", avatar_id: "owl", profile_complete: true,
      email: "old@example.com", description: "Patient investor", social_links: { github: "https://github.com/old" },
    }}><Profile /></UserCtxForTest.Provider></MemoryRouter>);

    expect(screen.getByRole("heading", { name: "Your profile" })).toBeInTheDocument();
    expect(screen.getByLabelText("Alias")).toHaveValue("Market Owl");
    fireEvent.change(screen.getByLabelText("Alias"), { target: { value: "Macro Owl" } });
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "owl@example.com" } });
    fireEvent.change(screen.getByLabelText("Description"), { target: { value: "Macro investor and patient skeptic." } });
    fireEvent.change(screen.getByLabelText("GitHub"), { target: { value: "https://github.com/market-owl" } });
    fireEvent.click(screen.getByRole("button", { name: "Save profile" }));

    await waitFor(() => expect(calls).toContainEqual({
      url: "/api/me/profile", method: "PUT", body: {
        display_name: "Macro Owl", avatar_id: "owl", email: "owl@example.com",
        description: "Macro investor and patient skeptic.",
        social_links: { github: "https://github.com/market-owl" },
      },
    }));

    fireEvent.change(screen.getByLabelText("Current password"), { target: { value: "password123" } });
    fireEvent.change(screen.getByLabelText("New password"), { target: { value: "new-password-456" } });
    fireEvent.change(screen.getByLabelText("Confirm new password"), { target: { value: "new-password-456" } });
    fireEvent.click(screen.getByRole("button", { name: "Update password" }));

    await waitFor(() => expect(calls).toContainEqual({
      url: "/api/me/password", method: "PUT", body: {
        current_password: "password123", new_password: "new-password-456",
      },
    }));
  });

  it("rejects mismatched password confirmation before making a request", () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    render(<MemoryRouter><UserCtxForTest.Provider value={{ id: 1, username: "alice", display_name: "Alice", avatar_id: "bull" }}><Profile /></UserCtxForTest.Provider></MemoryRouter>);

    fireEvent.change(screen.getByLabelText("Current password"), { target: { value: "password123" } });
    fireEvent.change(screen.getByLabelText("New password"), { target: { value: "new-password-456" } });
    fireEvent.change(screen.getByLabelText("Confirm new password"), { target: { value: "different-password" } });
    fireEvent.click(screen.getByRole("button", { name: "Update password" }));

    expect(screen.getByText("New passwords do not match.")).toBeInTheDocument();
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
