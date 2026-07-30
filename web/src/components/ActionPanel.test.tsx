import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import ActionPanel from "./ActionPanel";
import type { Portfolio } from "../api";

const portfolio: Portfolio = {
  cash_cents: 10_000_000,
  total_cents: 10_000_000,
  debt_cents: 0,
  max_debt_cents: 20_000_000,
  interest_rate_annual_bp: 300,
  bankrupt: false,
  positions: [],
  pending: [],
};

const ok = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

afterEach(() => vi.restoreAllMocks());

function renderPanel(over: Partial<Parameters<typeof ActionPanel>[0]> = {}) {
  return render(
    <ActionPanel roomId="1" instrumentId="S1" alias="Ridgeline Networks"
      portfolio={portfolio} onChanged={vi.fn()} {...over} />,
  );
}

describe("ActionPanel", () => {
  it("selects direction + tier and posts the hype; subtle toast when not caught", async () => {
    const posted: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (_url, init) => {
      if (init?.method === "POST") posted.push(JSON.parse(String(init.body)));
      return ok({ fee_cents: 4_000_000, caught: false, fine_cents: 0, cash_cents: 6_000_000 });
    });
    const onChanged = vi.fn();
    renderPanel({ onChanged });

    // tier rows show fee / shock / catch risk; default tier 1 fee on the submit
    expect(screen.getByText(/Fee \$5,000\.00 · shock ±1\.5% · 10% catch risk/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Launch campaign ($5,000.00)" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Smear" }));
    fireEvent.click(screen.getByRole("button", { name: /Tier 3/ }));
    expect(screen.getByRole("button", { name: "Launch campaign ($40,000.00)" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Launch campaign ($40,000.00)" }));
    await waitFor(() => expect(posted).toEqual([{ instrument_id: "S1", direction: "down", tier: 3 }]));
    expect(onChanged).toHaveBeenCalled();
    expect(await screen.findByText(/Campaign planted/)).toBeInTheDocument();
    expect(screen.queryByText(/Caught by regulators/)).not.toBeInTheDocument();
  });

  it("renders an alarming banner with the fine when caught", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      ok({ fee_cents: 500_000, caught: true, fine_cents: 1_000_000, cash_cents: 8_500_000 }));
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: "Launch campaign ($5,000.00)" }));
    expect(await screen.findByText(/Caught by regulators!/)).toBeInTheDocument();
    expect(screen.getByText(
      "Fined $10,000.00 — your manipulation of Ridgeline Networks is now public.")).toBeInTheDocument();
  });

  it("surfaces the server's 400 message as a toast", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "one hype per day" }), { status: 400 }));
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: "Launch campaign ($5,000.00)" }));
    expect(await screen.findByText("one hype per day")).toBeInTheDocument();
  });

  it("buys an intel tip and shows it inline as unverified rumor", async () => {
    const posted: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (_url, init) => {
      if (init?.method === "POST") posted.push(JSON.parse(String(init.body)));
      return ok({ outlook: "up", strength: "strong", fee_cents: 300_000, cash_cents: 9_700_000 });
    });
    const onChanged = vi.fn();
    renderPanel({ onChanged });

    fireEvent.click(screen.getByRole("button", { name: "Buy tip ($3,000.00)" }));
    expect(await screen.findByText("Your tip — unverified rumor")).toBeInTheDocument();
    expect(screen.getByText(/Tomorrow: leaning up ▲/)).toBeInTheDocument();
    expect(screen.getByText(/strong signal/)).toBeInTheDocument();
    expect(screen.getByText(/treat it as noise, not fact/)).toBeInTheDocument();
    expect(posted).toEqual([{ instrument_id: "S1" }]);
    expect(onChanged).toHaveBeenCalled();
  });

  it("shows a quiet tip without a strength label", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      ok({ outlook: "quiet", strength: null, fee_cents: 300_000, cash_cents: 9_700_000 }));
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: "Buy tip ($3,000.00)" }));
    expect(await screen.findByText(/Tomorrow: probably quiet/)).toBeInTheDocument();
    expect(screen.queryByText(/signal/)).not.toBeInTheDocument();
  });

  it("locks all actions when disabled (bankrupt / room ended)", () => {
    renderPanel({ disabled: true, note: "Actions disabled — you are bankrupt." });
    expect(screen.getByText("Actions disabled — you are bankrupt.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Launch campaign ($5,000.00)" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Buy tip ($3,000.00)" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Boost" })).toBeDisabled();
    expect(screen.getByRole("button", { name: /Tier 2/ })).toBeDisabled();
  });
});
