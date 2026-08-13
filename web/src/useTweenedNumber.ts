import { useEffect, useRef, useState } from "react";

const DURATION_MS = 550;

const easeOut = (t: number) => 1 - Math.pow(1 - t, 3);

/** requestAnimationFrame with a setTimeout fallback (jsdom has no rAF). */
export const raf = (cb: FrameRequestCallback): number =>
  typeof requestAnimationFrame === "function"
    ? requestAnimationFrame(cb)
    : (setTimeout(() => cb(performance.now()), 16) as unknown as number);

export const caf = (id: number): void => {
  if (typeof cancelAnimationFrame === "function") cancelAnimationFrame(id);
  else clearTimeout(id);
};

export const prefersReducedMotion = (): boolean =>
  typeof window !== "undefined" &&
  typeof window.matchMedia === "function" &&
  window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/**
 * Tweens a numeric display value toward its latest target with a short
 * ease-out, so polled updates read as motion instead of jumps. The first
 * value renders immediately (no count-up from zero on mount). While
 * `disabled` (e.g. scrubbing a chart) the target passes through untouched.
 */
export function useTweenedNumber(target: number, disabled = false): number {
  const [value, setValue] = useState(target);
  const shown = useRef(target);
  const id = useRef(0);

  useEffect(() => {
    const from = shown.current;
    if (disabled || prefersReducedMotion() || !Number.isFinite(from) || !Number.isFinite(target)) {
      shown.current = target;
      setValue(target);
      return;
    }
    if (from === target) return;
    const start = performance.now();
    const step = (now: number) => {
      const t = Math.min(1, (now - start) / DURATION_MS);
      const v = from + (target - from) * easeOut(t);
      shown.current = v;
      setValue(v);
      if (t < 1) id.current = raf(step);
    };
    id.current = raf(step);
    return () => caf(id.current);
  }, [target, disabled]);

  return value;
}
