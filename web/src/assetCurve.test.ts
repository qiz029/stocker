import { describe, expect, it } from "vitest";
import { assetCurve } from "./assetCurve";
import type { Trade } from "./api";

const series = {
  S1: [100, 110, 120, 90],
  S6: [100, 100, 101, 102],
};

describe("assetCurve", () => {
  it("stays flat with no trades", () => {
    expect(assetCurve([], series, 3)).toEqual([10_000_000, 10_000_000, 10_000_000, 10_000_000]);
  });

  it("tracks a buy through price moves", () => {
    // Buy $40,000 at day-1 price 110 → 363.636… shares.
    const trades: Trade[] = [
      { instrument_id: "S1", side: "buy", day: 1, price: 110, shares: 4_000_000 / 100 / 110, amount_cents: 4_000_000 },
    ];
    const curve = assetCurve(trades, series, 3);
    expect(curve[0]).toBe(10_000_000); // before the trade
    expect(curve[1]).toBe(10_000_000); // buy at market value: no instant P&L
    // Day 2: shares worth ×(120/110)
    const shares = 4_000_000 / 100 / 110;
    expect(curve[2]).toBe(6_000_000 + Math.round(shares * 120 * 100));
    expect(curve[3]).toBe(6_000_000 + Math.round(shares * 90 * 100));
  });

  it("credits sells back to cash", () => {
    const shares = 4_000_000 / 100 / 110;
    const trades: Trade[] = [
      { instrument_id: "S1", side: "buy", day: 1, price: 110, shares, amount_cents: 4_000_000 },
      { instrument_id: "S1", side: "sell", day: 2, price: 120, shares, amount_cents: Math.round(shares * 120 * 100) },
    ];
    const curve = assetCurve(trades, series, 3);
    const afterSell = 6_000_000 + Math.round(shares * 120 * 100);
    expect(curve[2]).toBe(afterSell);
    expect(curve[3]).toBe(afterSell); // all cash again — day-3 crash doesn't touch us
  });

  it("falls back to the last known close when curDay outruns the price series", () => {
    // S1's series only has days 0-3 (length 4); ask for day 4 (one past the
    // end), simulating current_day bumping before the price refetch lands.
    const trades: Trade[] = [
      { instrument_id: "S1", side: "buy", day: 1, price: 110, shares: 4_000_000 / 100 / 110, amount_cents: 4_000_000 },
    ];
    const curve = assetCurve(trades, series, 4);
    const shares = 4_000_000 / 100 / 110;
    const lastClose = series.S1[series.S1.length - 1]!; // 90
    // Should use the last known close (90), not 0.
    expect(curve[4]).toBe(6_000_000 + Math.round(shares * lastClose * 100));
  });

  it("still values a truly missing instrument's position at 0", () => {
    const trades: Trade[] = [
      { instrument_id: "UNKNOWN", side: "buy", day: 1, price: 50, shares: 100, amount_cents: 5_000_000 },
    ];
    const curve = assetCurve(trades, series, 2);
    // No series entry at all for UNKNOWN → position values at 0, only cash remains.
    expect(curve[2]).toBe(10_000_000 - 5_000_000);
  });
});
