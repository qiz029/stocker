import { describe, expect, it } from "vitest";
import { fmtCents, fmtPct, prettifyHeadline, windowed } from "./format";

describe("format helpers", () => {
  it("formats cents as dollars", () => {
    expect(fmtCents(1234567)).toBe("$12,345.67");
    expect(fmtCents(0)).toBe("$0.00");
    expect(fmtCents(10_000_000)).toBe("$100,000.00");
  });
  it("formats signed percentages", () => {
    expect(fmtPct(0.0123)).toBe("+1.23%");
    expect(fmtPct(-0.5)).toBe("-50.00%");
  });
  it("windows a series", () => {
    const s = [1, 2, 3, 4, 5];
    expect(windowed(s, 2)).toEqual([[4, 5], 3]);
    expect(windowed(s, Infinity)).toEqual([[1, 2, 3, 4, 5], 0]);
    expect(windowed(s, 99)).toEqual([[1, 2, 3, 4, 5], 0]);
  });
  it("prettifies engine factor tokens in headlines", () => {
    const aliasOf = (id: string) => ({ S1: "郊狼网络", S8: "环宇工业" }[id] ?? id);
    expect(prettifyHeadline("消息面变化，S8板块获得提振，市场解读不一", aliasOf))
      .toBe("消息面变化，环宇工业板块获得提振，市场解读不一");
    expect(prettifyHeadline("消息面变化，market板块承压，市场解读不一", aliasOf))
      .toBe("消息面变化，大盘承压，市场解读不一");
    expect(prettifyHeadline("tech sector板块波动 IDIO:S1", aliasOf))
      .toBe("科技板块波动 郊狼网络");
  });
});
