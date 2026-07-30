import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import TradePanel from "./TradePanel";
import type { Portfolio } from "../api";

const portfolio: Portfolio = {
  cash_cents: 6_000_000,
  total_cents: 10_000_000,
  debt_cents: 0,
  max_debt_cents: 20_000_000,
  interest_rate_annual_bp: 300,
  bankrupt: false,
  positions: [{ instrument_id: "S1", shares: 400, close: 110, value_cents: 4_400_000,
    avg_cost: 100, pnl_cents: 400_000, pnl_pct: 0.1 }],
  pending: [{ id: 9, instrument_id: "S1", side: "buy", amount_cents: 100_000, shares: 0, exec_day: 3 }],
};

afterEach(() => vi.restoreAllMocks());

describe("TradePanel", () => {
  it("estimates shares, submits a buy in cents, disables on overspend", async () => {
    const posted: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (_url, init) => {
      if (init?.method === "POST") posted.push(JSON.parse(String(init.body)));
      return new Response(JSON.stringify({ id: 1 }), { status: 200 });
    });
    const onChanged = vi.fn();
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={110} portfolio={portfolio} onChanged={onChanged} />);

    const input = screen.getByPlaceholderText("0");
    fireEvent.change(input, { target: { value: "11000" } });
    expect(screen.getByText(/≈ 100.0 shares/)).toBeInTheDocument();

    // over available cash ($60,000) → disabled
    fireEvent.change(input, { target: { value: "60001" } });
    expect(screen.getByRole("button", { name: "Place buy" })).toBeDisabled();

    fireEvent.change(input, { target: { value: "50000" } });
    fireEvent.click(screen.getByRole("button", { name: "Place buy" }));
    await waitFor(() => expect(posted).toEqual([
      { instrument_id: "S1", side: "buy", amount_cents: 5_000_000 }]));
    expect(onChanged).toHaveBeenCalled();
  });

  it("sell chips fill from held shares and submit sends shares", async () => {
    const posted: unknown[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (_url, init) => {
      if (init?.method === "POST") posted.push(JSON.parse(String(init.body)));
      return new Response(JSON.stringify({ id: 2 }), { status: 200 });
    });
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={110} portfolio={portfolio} onChanged={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Sell" }));
    fireEvent.click(screen.getByRole("button", { name: "50%" }));
    expect((screen.getByPlaceholderText("0") as HTMLInputElement).value).toBe("200.0");
    fireEvent.click(screen.getByRole("button", { name: "Place sell" }));
    await waitFor(() => expect(posted).toEqual([
      { instrument_id: "S1", side: "sell", shares: 200 }]));
  });

  it("lists and cancels pending orders", async () => {
    const deleted: string[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      if (init?.method === "DELETE") deleted.push(String(url));
      return new Response(JSON.stringify({ status: "cancelled" }), { status: 200 });
    });
    const onChanged = vi.fn();
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={110} portfolio={portfolio} onChanged={onChanged} />);
    expect(screen.getByText(/Buy \$1,000\.00/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(deleted).toEqual(["/api/rooms/1/orders/9"]));
    expect(onChanged).toHaveBeenCalled();
  });

  it("sell chips never overshoot fractional holdings", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ id: 3 }), { status: 200 }));
    const fractional: Portfolio = {
      ...portfolio,
      positions: [{ instrument_id: "S1", shares: 400.16, close: 110, value_cents: 4_401_760,
        avg_cost: 100, pnl_cents: 401_760, pnl_pct: 0.1 }],
    };
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={110} portfolio={fractional} onChanged={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Sell" }));
    fireEvent.click(screen.getByRole("button", { name: "All" }));
    expect((screen.getByPlaceholderText("0") as HTMLInputElement).value).toBe("400.1");
    expect(screen.getByRole("button", { name: "Place sell" })).not.toBeDisabled();
  });

  it("shows the after-hours note when the market is closed", () => {
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={100}
      portfolio={null} onChanged={() => {}} afterHours />);
    expect(screen.getByText(/Market is closed.*next open/)).toBeInTheDocument();
  });

  it("hides the after-hours note while the market is open", () => {
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={100}
      portfolio={null} onChanged={() => {}} />);
    expect(screen.queryByText(/after-hours/)).not.toBeInTheDocument();
  });

  it("locks input and submit when disabled (bankrupt)", () => {
    render(<TradePanel roomId="1" instrumentId="S1" lastClose={100}
      portfolio={{ ...portfolio, bankrupt: true }} onChanged={() => {}} disabled />);
    expect(screen.getByText(/Trading disabled/)).toBeInTheDocument();
    expect(screen.getByPlaceholderText("0")).toBeDisabled();
    fireEvent.change(screen.getByPlaceholderText("0"), { target: { value: "100" } });
    expect(screen.getByRole("button", { name: "Place buy" })).toBeDisabled();
  });
});
