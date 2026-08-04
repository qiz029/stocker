import { beforeEach, describe, expect, it } from "vitest";
import { loadDebunkVerdicts, saveDebunkVerdict } from "./debunkVerdicts";

describe("debunk verdict session storage", () => {
  beforeEach(() => sessionStorage.clear());

  it("restores a private verdict after navigating away without leaking it to another user", () => {
    saveDebunkVerdict(7, "room-1", 42, "likely_false");

    expect(loadDebunkVerdicts(7, "room-1")).toEqual({ 42: "likely_false" });
    expect(loadDebunkVerdicts(8, "room-1")).toEqual({});
  });

  it("ignores malformed or unsupported stored values", () => {
    sessionStorage.setItem("stocker:debunk-verdicts:7:room-1", '{"42":"invented","bad":"likely_true"}');
    expect(loadDebunkVerdicts(7, "room-1")).toEqual({});
  });
});
