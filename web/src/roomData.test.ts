import { describe, expect, it } from "vitest";
import { buildOhlcMap, buildSeriesMap } from "./roomData";

describe("roomData helpers", () => {
  it("builds a close-series map from price responses", () => {
    const map = buildSeriesMap([
      ["S1", { days: [{ open: 1, high: 2, low: 0.5, close: 1.5 }, { open: 1.5, high: 2, low: 1, close: 1.8 }] }],
      ["S2", { days: [{ open: 9, high: 9, low: 9, close: 9 }] }],
    ]);
    expect(map.S1).toEqual([1.5, 1.8]);
    expect(map.S2).toEqual([9]);
  });

  it("builds an OHLC map from the same price responses", () => {
    const days = [{ open: 1, high: 2, low: 0.5, close: 1.5 }, { open: 1.5, high: 2, low: 1, close: 1.8 }];
    const map = buildOhlcMap([["S1", { days }]]);
    expect(map.S1).toEqual(days);
  });
});
