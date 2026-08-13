import { useEffect, useRef, useState } from "react";

export type FlashDir = 1 | -1 | 0;

/**
 * Flags a short-lived visual "flash" whenever the watched value changes.
 * `dir` is the sign of the change for numbers (1 up / -1 down) and 0 for
 * string changes (neutral highlight). `nonce` increments on every change
 * so callers can retrigger a CSS animation by keying the element on it.
 * The first render never flashes, and a change of value type (e.g.
 * placeholder → data) is ignored.
 */
export function useChangeFlash(value: number | string): { dir: FlashDir; nonce: number } {
  const prev = useRef(value);
  const [flash, setFlash] = useState<{ dir: FlashDir; nonce: number }>({ dir: 0, nonce: 0 });

  useEffect(() => {
    const before = prev.current;
    prev.current = value;
    if (before === value || typeof before !== typeof value) return;
    const dir: FlashDir =
      typeof before === "number" && typeof value === "number" ? (value > before ? 1 : -1) : 0;
    setFlash(f => ({ dir, nonce: f.nonce + 1 }));
  }, [value]);

  return flash;
}
