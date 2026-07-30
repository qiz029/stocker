import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import OptionsChain from "./OptionsChain";
import type { OptionContract, Portfolio } from "../api";

const portfolio: Portfolio = {
  cash_cents: 1_000_000,   // $10,000
  total_cents: 1_000_000,
  debt_cents: 0,
  max_debt_cents: 0,
  interest_rate_annual_bp: 0,
  bankrupt: false,
  positions: [],
  pending: [],
  options: [],
};

const chain: OptionContract[] = [
  { option_id: 1, kind: "call", strike: 100, expiry_day: 5, price: 2.5 },
  { option_id: 2, kind: "put", strike: 100, expiry_day: 5, price: 1.5 },
  { option_id: 3, kind: "call", strike: 120, expiry_day: 5, price: 0.8 },
  { option_id: 4, kind: "put", strike: 120, expiry_day: 5, price: 3.1 },
  { option_id: 5, kind: "call", strike: 100, expiry_day: 10, price: 4.2 },
];

afterEach(() => vi.restoreAllMocks());

function mockApi(posted?: unknown[]) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_url, init) => {
    if (init?.method === "POST") {
      posted?.push(JSON.parse(String(init.body)));
      return new Response(JSON.stringify(
        { action: "buy", contracts: 2, price: 2.5, amount_cents: 500, cash_cents: 999_500 }),
        { status: 200 });
    }
    return new Response(JSON.stringify(chain), { status: 200 });
  });
}

function renderChain(extra?: Partial<Parameters<typeof OptionsChain>[0]>) {
  return render(
    <OptionsChain roomId="1" instrumentId="S1" alias="Ridgeline" lastClose={110}
      currentDay={2} portfolio={portfolio} onChanged={vi.fn()} {...extra} />,
  );
}

describe("OptionsChain", () => {
  it("renders expiry pills and one row per strike with call/put prices", async () => {
    mockApi();
    const { container } = renderChain();
    await waitFor(() => expect(screen.getByRole("button", { name: "$2.50" })).toBeInTheDocument());

    expect(screen.getByRole("button", { name: "Day 5 · 3d left" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Day 10 · 8d left" })).toBeInTheDocument();
    // first expiry selected: two strike rows, day-10 contract hidden
    expect(screen.getByRole("button", { name: "$0.80" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "$4.20" })).not.toBeInTheDocument();
    expect(container.querySelectorAll(".chain-table tbody tr")).toHaveLength(2);

    // ITM tint: call 100 < close 110 is ITM; put 120 > 110 is ITM
    expect(screen.getByRole("button", { name: "$2.50" }).className).toContain("itm");
    expect(screen.getByRole("button", { name: "$3.10" }).className).toContain("itm");
    expect(screen.getByRole("button", { name: "$0.80" }).className).not.toContain("itm");
  });

  it("switches expiries via the pills", async () => {
    mockApi();
    renderChain();
    await waitFor(() => expect(screen.getByRole("button", { name: "$2.50" })).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Day 10 · 8d left" }));
    expect(screen.getByRole("button", { name: "$4.20" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "$2.50" })).not.toBeInTheDocument();
  });

  it("buy flow: pick a call, size by chips, submit posts the order", async () => {
    const posted: unknown[] = [];
    mockApi(posted);
    const onChanged = vi.fn();
    renderChain({ onChanged });
    await waitFor(() => expect(screen.getByRole("button", { name: "$2.50" })).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "$2.50" }));
    // inline form with the contract description
    expect(screen.getByText("Ridgeline Call $100.00 · expires Day 5 (3d)")).toBeInTheDocument();

    // ALL sizes by cash: $10,000 / $2.50 = 4000 contracts
    fireEvent.click(screen.getByRole("button", { name: "All" }));
    expect((screen.getByPlaceholderText("0") as HTMLInputElement).value).toBe("4000");
    expect(screen.getByText("≈ $10,000.00")).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("0"), { target: { value: "2" } });
    expect(screen.getByText("≈ $5.00")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Buy to open" }));
    await waitFor(() => expect(posted).toEqual([{ option_id: 1, action: "buy", contracts: 2 }]));
    expect(onChanged).toHaveBeenCalled();
  });

  it("disables the chain and the form when bankrupt", async () => {
    mockApi();
    renderChain({ disabled: true, note: "Trading disabled — you are bankrupt." });
    await waitFor(() => expect(screen.getByRole("button", { name: "$2.50" })).toBeInTheDocument());

    expect(screen.getByText(/Trading disabled/)).toBeInTheDocument();
    for (const btn of document.querySelectorAll<HTMLButtonElement>(".chain-px")) {
      expect(btn).toBeDisabled();
    }
    fireEvent.click(screen.getByRole("button", { name: "$2.50" }));
    expect(screen.queryByRole("button", { name: "Buy to open" })).not.toBeInTheDocument();
  });
});
