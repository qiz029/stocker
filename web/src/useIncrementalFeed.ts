import { useEffect, useRef, useState } from "react";
import { api } from "./api";

const MAX_ROUNDS_PER_TICK = 10;
const PAGE_SIZE = 200;

/**
 * Polls a cursor-paginated feed endpoint (news/events/etc.) incrementally,
 * mirroring Chat.tsx's fetchNew pattern: track the last-seen id, ask the
 * server for everything after it, and de-dup defensively when appending.
 *
 * Unlike a single after=0 poll (which only ever sees the server's oldest
 * page once it fills up), this keeps re-fetching within a tick whenever a
 * full page comes back, so a long backlog drains instead of stalling.
 */
export function useIncrementalFeed<T extends { id: number }>(
  url: (after: number) => string,
  intervalMs: number,
  resetKey: string,
) {
  const [items, setItems] = useState<T[]>([]);
  const lastID = useRef(0);

  async function fetchNew() {
    for (let round = 0; round < MAX_ROUNDS_PER_TICK; round++) {
      let batch: T[];
      try {
        const res = await api.get<{ items: T[] }>(url(lastID.current));
        batch = res.items;
      } catch {
        /* transient poll errors are silent; next tick retries */
        return;
      }
      const fresh = batch.filter(x => x.id > lastID.current);
      if (fresh.length) {
        lastID.current = fresh[fresh.length - 1]!.id;
        setItems(prev => {
          const seen = new Set(prev.map(x => x.id));
          return [...prev, ...fresh.filter(x => !seen.has(x.id))];
        });
      }
      if (batch.length !== PAGE_SIZE) break;
    }
  }

  useEffect(() => {
    lastID.current = 0;
    setItems([]);
    void fetchNew();
    const t = setInterval(() => void fetchNew(), intervalMs);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  return { items };
}
