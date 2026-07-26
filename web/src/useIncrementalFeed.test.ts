import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useIncrementalFeed } from "./useIncrementalFeed";

type Item = { id: number };

afterEach(() => vi.restoreAllMocks());

function afterParam(url: unknown): string | null {
  return new URL(String(url), "http://x").searchParams.get("after");
}

describe("useIncrementalFeed", () => {
  it("fetches the initial batch on mount", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
      new Response(JSON.stringify({ items: [{ id: 1 }, { id: 2 }] }), { status: 200 }));

    const { result } = renderHook(() => useIncrementalFeed<Item>(after => `/api/x?after=${after}`, 30_000, "room-1"));

    await waitFor(() => expect(result.current.items).toEqual([{ id: 1 }, { id: 2 }]));
  });

  it("advances the cursor and appends only newer items on the next tick", async () => {
    const afterParams: (string | null)[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async url => {
      const after = afterParam(url);
      afterParams.push(after);
      if (after === "0") {
        return new Response(JSON.stringify({ items: [{ id: 1 }, { id: 2 }] }), { status: 200 });
      }
      // subsequent ticks must query with the advanced cursor (id 2)
      return new Response(JSON.stringify({ items: [{ id: 3 }] }), { status: 200 });
    });

    // Short interval so the second tick fires within the test's wait window.
    const { result } = renderHook(() => useIncrementalFeed<Item>(after => `/api/x?after=${after}`, 10, "room-1"));

    await waitFor(() => expect(result.current.items).toEqual([{ id: 1 }, { id: 2 }, { id: 3 }]));
    // First call used the initial cursor (0); a later call used the advanced one (2).
    expect(afterParams[0]).toBe("0");
    expect(afterParams).toContain("2");
  });

  it("does not duplicate items when the same batch is fetched twice", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
      new Response(JSON.stringify({ items: [{ id: 1 }, { id: 2 }] }), { status: 200 }));

    // Even though a well-behaved server wouldn't re-return already-seen ids,
    // the hook must still de-dup defensively (same guard as Chat.tsx),
    // so repeated ticks returning the same fixture must not grow the list.
    const { result } = renderHook(() => useIncrementalFeed<Item>(after => `/api/x?after=${after}`, 10, "room-2"));

    await waitFor(() => expect(result.current.items).toEqual([{ id: 1 }, { id: 2 }]));
    await new Promise(r => setTimeout(r, 50));
    expect(result.current.items).toEqual([{ id: 1 }, { id: 2 }]);
  });
});
