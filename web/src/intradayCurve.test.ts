import { describe, expect, it } from "vitest";
import { buildIntradayCurve, intradayTimeLabel } from "./intradayCurve";

describe("intraday curve", () => {
  it("creates a stable organic bridge without changing settled endpoints", () => {
    const first = buildIntradayCurve(10_000_000, 10_120_000, 42);
    const second = buildIntradayCurve(10_000_000, 10_120_000, 42);

    expect(first).toEqual(second);
    expect(first).toHaveLength(65);
    expect(first[0]).toBe(10_000_000);
    expect(first.at(-1)).toBe(10_120_000);
    expect(new Set(first.slice(1, -1)).size).toBeGreaterThan(20);
    expect(first.some((value, i) => i > 0 && i < first.length - 1
      && value !== Math.round(10_000_000 + 120_000 * (i / 64)))).toBe(true);
  });

  it("maps the one-day scrubber from market open to close", () => {
    expect(intradayTimeLabel(0, 65)).toBe("09:30");
    expect(intradayTimeLabel(64, 65)).toBe("16:00");
  });
});
