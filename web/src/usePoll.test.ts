import { describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { usePoll } from "./usePoll";

describe("usePoll", () => {
  it("ignores a stale slow response that resolves after a newer one", async () => {
    let call = 0;
    const fn = vi.fn(() => {
      call += 1;
      const mine = call;
      // First call resolves LAST (slow); later calls resolve fast.
      const delay = mine === 1 ? 50 : 0;
      return new Promise<number>(res => setTimeout(() => res(mine), delay));
    });
    const { result } = renderHook(() => usePoll(fn, 100_000, []));
    // Fire a manual reload while call 1 is still in flight.
    await result.current.reload();
    await waitFor(() => expect(result.current.data).toBe(2));
    // Give the slow call 1 time to resolve; data must stay 2.
    await new Promise(r => setTimeout(r, 80));
    expect(result.current.data).toBe(2);
  });
});
