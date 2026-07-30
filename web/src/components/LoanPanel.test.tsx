import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import LoanPanel from "./LoanPanel";
import type { Portfolio } from "../api";

const portfolio: Portfolio = {
  cash_cents: 6_000_000,
  total_cents: 11_000_000,
  debt_cents: 1_000_000,
  max_debt_cents: 20_000_000,
  interest_rate_annual_bp: 300,
  bankrupt: false,
  positions: [],
  pending: [],
};

afterEach(() => vi.restoreAllMocks());

function mockPost(capture: unknown[], status = 200, body: unknown = { action: "borrow", amount_cents: 0 }) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_url, init) => {
    if (init?.method === "POST") capture.push({ url: String(_url), body: JSON.parse(String(init.body)) });
    return new Response(JSON.stringify(body), { status });
  });
}

describe("LoanPanel", () => {
  it("shows current debt, annual rate and cap usage", () => {
    render(<LoanPanel roomId="1" portfolio={portfolio} onChanged={() => {}} />);
    expect(screen.getByText("Current debt")).toBeInTheDocument();
    expect(screen.getByText("$10,000.00")).toBeInTheDocument();
    expect(screen.getByText("Annual rate")).toBeInTheDocument();
    expect(screen.getByText("3.00%")).toBeInTheDocument();
    expect(screen.getByText("$10,000.00 of $200,000.00 used")).toBeInTheDocument();
    // 5% of cap: no warning
    expect(screen.queryByText(/Near the credit cap/)).not.toBeInTheDocument();
  });

  it("warns when debt approaches the cap", () => {
    render(<LoanPanel roomId="1" portfolio={{ ...portfolio, debt_cents: 17_000_000 }} onChanged={() => {}} />);
    expect(screen.getByText(/Near the credit cap/)).toBeInTheDocument();
  });

  it("borrow chips fill % of remaining headroom and submit posts to /loans", async () => {
    const posted: { url: string; body: unknown }[] = [];
    mockPost(posted);
    const onChanged = vi.fn();
    render(<LoanPanel roomId="1" portfolio={portfolio} onChanged={onChanged} />);

    // headroom = $200,000 - $10,000 = $190,000; 50% chip → $95,000
    fireEvent.click(screen.getByRole("button", { name: "50%" }));
    expect((screen.getByPlaceholderText("0") as HTMLInputElement).value).toBe("95000");

    fireEvent.click(screen.getByRole("button", { name: "Place borrow" }));
    await waitFor(() => expect(posted).toEqual([
      { url: "/api/rooms/1/loans", body: { action: "borrow", amount_cents: 9_500_000 } }]));
    expect(onChanged).toHaveBeenCalled();
  });

  it("repay chips fill % of min(cash, debt)", async () => {
    const posted: { url: string; body: unknown }[] = [];
    mockPost(posted);
    render(<LoanPanel roomId="1" portfolio={portfolio} onChanged={() => {}} />);

    fireEvent.click(screen.getByRole("button", { name: "Repay" }));
    // min($60,000 cash, $10,000 debt) = $10,000; All chip → $10,000
    fireEvent.click(screen.getByRole("button", { name: "All" }));
    expect((screen.getByPlaceholderText("0") as HTMLInputElement).value).toBe("10000");

    fireEvent.click(screen.getByRole("button", { name: "Place repay" }));
    await waitFor(() => expect(posted).toEqual([
      { url: "/api/rooms/1/loans", body: { action: "repay", amount_cents: 1_000_000 } }]));
  });

  it("shows the server's error message on rejection", async () => {
    const posted: { url: string; body: unknown }[] = [];
    mockPost(posted, 400, { error: "debt cap exceeded" });
    render(<LoanPanel roomId="1" portfolio={portfolio} onChanged={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "All" }));
    fireEvent.click(screen.getByRole("button", { name: "Place borrow" }));
    expect(await screen.findByText("debt cap exceeded")).toBeInTheDocument();
  });

  it("locks inputs when bankrupt", () => {
    render(<LoanPanel roomId="1" portfolio={{ ...portfolio, bankrupt: true }} onChanged={() => {}} />);
    expect(screen.getByPlaceholderText("0")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Place borrow" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "All" })).toBeDisabled();
  });
});
