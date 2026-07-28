import { describe, expect, it } from "vitest";
import { dayLabel, sessionTimes, simClock } from "./simClock";

describe("sessionTimes", () => {
  it("starts 9:30, ends 16:00, gaps are 2-4h on 5-min steps", () => {
    for (let day = 0; day < 200; day++) {
      const t = sessionTimes(7, day);
      expect(t[0]).toBe(570);
      expect(t[t.length - 1]).toBe(960);
      for (let i = 1; i < t.length; i++) {
        const gap = t[i]! - t[i - 1]!;
        expect(gap).toBeGreaterThanOrEqual(120);
        expect(gap).toBeLessThanOrEqual(240);
        expect(gap % 5).toBe(0);
      }
    }
  });

  it("is deterministic per (roomId, day) and varies across days", () => {
    expect(sessionTimes(7, 3)).toEqual(sessionTimes(7, 3));
    const distinct = new Set(
      Array.from({ length: 50 }, (_, d) => sessionTimes(7, d).join(",")));
    expect(distinct.size).toBeGreaterThan(1);
  });
});

describe("dayLabel", () => {
  it("maps a day index to the fictional calendar", () => {
    expect(dayLabel(0)).toBe("第1周 · 周一");
    expect(dayLabel(4)).toBe("第1周 · 周五");
    expect(dayLabel(5)).toBe("第2周 · 周一");
    expect(dayLabel(17)).toBe("第4周 · 周三");
  });
});

describe("simClock", () => {
  // D = 100 s/day → 交易时段 78 s,收盘 22 s;100_000 ms 一天
  const t0 = Date.parse("2026-07-26T12:00:00Z");
  const started = new Date(t0).toISOString();
  const D = 100;

  it("opens at 9:30 and clamps clock skew to day 0", () => {
    expect(simClock(started, D, 1, 10, t0).time).toBe("9:30");
    expect(simClock(started, D, 1, 10, t0 - 5000).phase).toBe("open");
  });

  it("clock is non-decreasing within a session and hits 16:00 last", () => {
    let prev = 0;
    for (let ms = 0; ms < 78_000; ms += 500) {
      const c = simClock(started, D, 1, 10, t0 + ms);
      expect(c.phase).toBe("open");
      const [h, m] = c.time!.split(":").map(Number);
      const mins = h! * 60 + m!;
      expect(mins).toBeGreaterThanOrEqual(prev);
      prev = mins;
    }
    expect(prev).toBe(960); // 16:00
  });

  it("closes after 78% of the wall day with a next-open countdown", () => {
    const c = simClock(started, D, 1, 10, t0 + 78_000);
    expect(c.phase).toBe("closed"); // day 0 = 周一
    expect(c.time).toBeNull();
    expect(c.nextOpenSecs).toBe(22);
    expect(c.dateLabel).toBe("第1周 · 周一");
  });

  it("friday close is a weekend; next day is monday of week 2", () => {
    const fri = simClock(started, D, 1, 10, t0 + 4 * 100_000 + 90_000);
    expect(fri.phase).toBe("weekend");
    const mon = simClock(started, D, 1, 10, t0 + 5 * 100_000);
    expect(mon.phase).toBe("open");
    expect(mon.dateLabel).toBe("第2周 · 周一");
  });

  it("is ended once elapsed passes totalDays", () => {
    const c = simClock(started, D, 1, 10, t0 + 10 * 100_000);
    expect(c.phase).toBe("ended");
    expect(c.time).toBeNull();
    expect(c.nextOpenSecs).toBeNull();
  });

  it("has no phantom next-open countdown on the final day (weekend)", () => {
    // totalDays 10 → last day index 9; 9 % 5 === 4 → weekend
    const c = simClock(started, D, 1, 10, t0 + 9 * 100_000 + 90_000);
    expect(c.phase).toBe("weekend");
    expect(c.nextOpenSecs).toBeNull();
  });

  it("has no phantom next-open countdown on the final day (non-friday close)", () => {
    // totalDays 9 → last day index 8; 8 % 5 === 3 → closed, not weekend
    const c = simClock(started, D, 1, 9, t0 + 8 * 100_000 + 90_000);
    expect(c.phase).toBe("closed");
    expect(c.nextOpenSecs).toBeNull();
  });

  it("still counts down to next open on a mid-game close", () => {
    const c = simClock(started, D, 1, 10, t0 + 78_000);
    expect(c.phase).toBe("closed");
    expect(c.nextOpenSecs).toBe(22);
  });

  it("clamps the next-open countdown above zero at a day boundary", () => {
    const c = simClock(started, D, 1, 10, t0 + 99_999);
    expect(c.phase).toBe("closed");
    expect(c.nextOpenSecs).toBeGreaterThanOrEqual(1);
  });
});
