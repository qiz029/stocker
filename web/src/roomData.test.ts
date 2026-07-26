import { describe, expect, it } from "vitest";
import { buildSeriesMap, dayCountdown } from "./roomData";

describe("roomData helpers", () => {
  it("builds a close-series map from price responses", () => {
    const map = buildSeriesMap([
      ["S1", { days: [{ open: 1, high: 2, low: 0.5, close: 1.5 }, { open: 1.5, high: 2, low: 1, close: 1.8 }] }],
      ["S2", { days: [{ open: 9, high: 9, low: 9, close: 9 }] }],
    ]);
    expect(map.S1).toEqual([1.5, 1.8]);
    expect(map.S2).toEqual([9]);
  });

  it("computes seconds until next trading day", () => {
    // started 90 s ago at 60 s/day → 30 s left in day 1
    const started = new Date(Date.now() - 90_000).toISOString();
    const secs = dayCountdown(started, 60);
    expect(secs).toBeGreaterThan(25);
    expect(secs).toBeLessThanOrEqual(30);
  });
});
