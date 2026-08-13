import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useChangeFlash } from "./useChangeFlash";

describe("useChangeFlash", () => {
  it("stays idle on first render", () => {
    const { result } = renderHook(({ v }) => useChangeFlash(v), { initialProps: { v: 10 } });
    expect(result.current).toEqual({ dir: 0, nonce: 0 });
  });

  it("flags the direction of numeric changes and bumps the nonce", () => {
    const { result, rerender } = renderHook(({ v }) => useChangeFlash(v), { initialProps: { v: 10 } });
    rerender({ v: 12 });
    expect(result.current).toEqual({ dir: 1, nonce: 1 });
    rerender({ v: 11 });
    expect(result.current).toEqual({ dir: -1, nonce: 2 });
  });

  it("ignores unchanged values and treats string changes as neutral", () => {
    const { result, rerender } =
      renderHook(({ v }) => useChangeFlash(v), { initialProps: { v: "a" as string | number } });
    rerender({ v: "a" });
    expect(result.current.nonce).toBe(0);
    rerender({ v: "b" });
    expect(result.current).toEqual({ dir: 0, nonce: 1 });
  });
});
