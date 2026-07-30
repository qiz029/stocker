import { useEffect, useRef, useState } from "react";

const MAX_ROUNDS_PER_TICK = 10;
const PAGE_SIZE = 200;

/**
 * Polls a cursor-paginated feed endpoint (news/events/forum/etc.)
 * incrementally, mirroring Chat.tsx's fetchNew pattern: track the last-seen
 * id, ask the server for everything after it, and de-dup defensively when
 * appending.
 *
 * Unlike a single after=0 poll (which only ever sees the server's oldest
 * page once it fills up), this keeps re-fetching within a tick whenever a
 * full page comes back, so a long backlog drains instead of stalling.
 *
 * `extra` exposes the last full response (e.g. news media_accuracy); it only
 * changes identity when the response payload actually changes.
 */
export function useIncrementalFeed<T extends { id: number }, R extends { items: T[] }>(
  fetchPage: (after: number) => Promise<R>,
  intervalMs: number,
  resetKey: string,
) {
  const [items, setItems] = useState<T[]>([]);
  const [extra, setExtra] = useState<R | undefined>(undefined);
  const lastID = useRef(0);
  const extraJSON = useRef("");

  async function fetchNew() {
    for (let round = 0; round < MAX_ROUNDS_PER_TICK; round++) {
      let res: R;
      try {
        res = await fetchPage(lastID.current);
      } catch {
        /* transient poll errors are silent; next tick retries */
        return;
      }
      const batch = res.items;
      const j = JSON.stringify(res);
      if (j !== extraJSON.current) {
        extraJSON.current = j;
        setExtra(res);
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
    extraJSON.current = "";
    setItems([]);
    setExtra(undefined);
    void fetchNew();
    const t = setInterval(() => void fetchNew(), intervalMs);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);

  return { items, extra };
}
