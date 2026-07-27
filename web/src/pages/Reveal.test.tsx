import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import Reveal from "./Reveal";

afterEach(() => vi.restoreAllMocks());

const reveal = {
  instruments: [
    { id: "S1", alias: "郊狼网络", real_name: "Cisco Systems" },
    { id: "S6", alias: "老树能源", real_name: "" },
  ],
  trades: [
    { username: "amy", instrument_id: "S1", side: "buy", day: 20, price: 180.5, shares: 221.6, amount_cents: 4_000_000 },
  ],
  leaderboard: [
    { username: "amy", total_cents: 13_420_000, return_pct: 0.342, late_join: false },
    { username: "me", total_cents: 9_100_000, return_pct: -0.09, late_join: false },
  ],
  real_period: "1999-01 ~ 2001-12",
};

function renderAt(handler: () => Response) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async () => handler());
  render(
    <MemoryRouter initialEntries={["/rooms/1/reveal"]}>
      <Routes><Route path="/rooms/:roomId/reveal" element={<Reveal />} /></Routes>
    </MemoryRouter>,
  );
}

describe("Reveal page", () => {
  it("shows identities, trades and final leaderboard", async () => {
    renderAt(() => new Response(JSON.stringify(reveal), { status: 200 }));
    await waitFor(() => expect(screen.getByText("Cisco Systems")).toBeInTheDocument());
    expect(screen.getAllByText("郊狼网络").length).toBeGreaterThan(0);
    expect(screen.getByText("+34.20%")).toBeInTheDocument();
    expect(screen.getAllByText("amy")[0].closest(".lb-row")).toBeTruthy();
    expect(screen.getByText("$40,000.00")).toBeInTheDocument();
    expect(screen.getByText(/1999-01 ~ 2001-12/)).toBeInTheDocument();
  });

  it("shows a waiting message before the game ends", async () => {
    renderAt(() => new Response(JSON.stringify({ error: "game not finished" }), { status: 409 }));
    expect(await screen.findByText(/尚未揭晓/)).toBeInTheDocument();
  });
});
