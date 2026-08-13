import { describe, expect, it } from "vitest";
import { fastestDayDuration } from "../../core/tempo";

describe("fastestDayDuration", () => {
  it.each([[300, 6], [750, 2], [881, 2]])("maps %i days to %i seconds/day", (days, seconds) => {
    expect(fastestDayDuration(days)).toBe(seconds);
  });
});
