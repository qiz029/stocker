import { render, screen } from "@testing-library/react";
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
});
