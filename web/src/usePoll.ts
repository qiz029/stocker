import { useCallback, useEffect, useRef, useState } from "react";

export function usePoll<T>(fn: () => Promise<T>, ms: number, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const seq = useRef(0);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const tick = useCallback(async () => {
    const my = ++seq.current;
    try {
      const result = await fn();
      if (my === seq.current) {
        setData(result);
        setError(null);
      }
    } catch (e) {
      if (my === seq.current) {
        setError(e instanceof Error ? e.message : String(e));
      }
    }
  }, deps);
  useEffect(() => {
    void tick();
    const t = setInterval(() => void tick(), ms);
    return () => clearInterval(t);
  }, [tick, ms]);
  return { data, error, reload: tick };
}
