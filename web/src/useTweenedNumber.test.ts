import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useTweenedNumber } from "./useTweenedNumber";

afterEach(() => vi.unstubAllGlobals());

/** Deterministic rAF: capture callbacks so tests can step time manually. */
function stubRaf() {
  const callbacks = new Map<number, FrameRequestCallback>();
  let nextId = 1;
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
    callbacks.set(nextId, cb);
    return nextId++;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => { callbacks.delete(id); });
  return {
    run(now: number) {
      const pending = [...callbacks.values()];
      callbacks.clear();
      pending.forEach(cb => cb(now));
    },
  };
}

describe("useTweenedNumber", () => {
  it("renders the initial target immediately (no count-up from zero)", () => {
    const { result } = renderHook(({ v }) => useTweenedNumber(v), { initialProps: { v: 108 } });
    expect(result.current).toBe(108);
  });

  it("passes the target through untouched while disabled", () => {
    const { result, rerender } =
      renderHook(({ v, d }) => useTweenedNumber(v, d), { initialProps: { v: 108, d: true } });
    rerender({ v: 200, d: true });
    expect(result.current).toBe(200);
  });

  it("eases toward a new target and lands exactly on it", () => {
    const raf = stubRaf();
    const { result, rerender } = renderHook(({ v }) => useTweenedNumber(v), { initialProps: { v: 0 } });
    rerender({ v: 100 });
    expect(result.current).toBe(0);

    const t0 = performance.now();
    act(() => raf.run(t0 + 200));
    expect(result.current).toBeGreaterThan(10);
    expect(result.current).toBeLessThan(100);

    act(() => raf.run(t0 + 5000));
    expect(result.current).toBe(100);
  });
});
