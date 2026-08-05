import { useCallback, useEffect, useRef, useState } from "react";

const MAX_ROUNDS_PER_TICK = 10;
const PAGE_SIZE = 200;

/**
 * Polls a cursor-paginated feed endpoint (news/events/forum/chat/etc.)
 * incrementally: track the last-seen id, ask the server for everything after
 * it, and de-dup defensively when appending. Port of web/src/useIncrementalFeed.ts,
 * with an extra `reload` so callers can refresh right after a local mutation
 * (e.g. sending a chat message).
 *
 * `extra` exposes the last full response (e.g. news media_accuracy).
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
  const fetchPageRef = useRef(fetchPage);
  fetchPageRef.current = fetchPage;

  const reload = useCallback(async () => {
    for (let round = 0; round < MAX_ROUNDS_PER_TICK; round++) {
      let res: R;
      try {
        res = await fetchPageRef.current(lastID.current);
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
  }, []);

  useEffect(() => {
    lastID.current = 0;
    extraJSON.current = "";
    setItems([]);
    setExtra(undefined);
    void reload();
    const t = setInterval(() => void reload(), intervalMs);
    return () => clearInterval(t);
  }, [resetKey, intervalMs, reload]);

  return { items, extra, reload };
}
