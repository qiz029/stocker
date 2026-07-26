import { useCallback, useEffect, useState } from "react";

export function usePoll<T>(fn: () => Promise<T>, ms: number, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const tick = useCallback(async () => {
    try {
      setData(await fn());
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, deps);
  useEffect(() => {
    void tick();
    const t = setInterval(() => void tick(), ms);
    return () => clearInterval(t);
  }, [tick, ms]);
  return { data, error, reload: tick };
}
