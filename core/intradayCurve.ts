const OPEN_MINUTES = 9 * 60 + 30;
const SESSION_MINUTES = 6 * 60 + 30;
const DEFAULT_POINTS = 65;

function mulberry32(seed: number): () => number {
  let value = seed >>> 0;
  return () => {
    value = (value + 0x6d2b79f5) >>> 0;
    let mixed = value;
    mixed = Math.imul(mixed ^ (mixed >>> 15), mixed | 1);
    mixed ^= mixed + Math.imul(mixed ^ (mixed >>> 7), mixed | 61);
    return ((mixed ^ (mixed >>> 14)) >>> 0) / 4294967296;
  };
}

/**
 * Builds a deterministic, presentation-only intraday bridge. The first and
 * last values are exact settled portfolio totals; only the path between them
 * is simulated so a one-day chart has the density of a live market chart.
 */
export function buildIntradayCurve(
  start: number,
  end: number,
  seed: number,
  points = DEFAULT_POINTS,
): number[] {
  const count = Math.max(3, points);
  const rng = mulberry32(seed);
  const noise = new Array<number>(count).fill(0);
  let walk = 0;
  for (let i = 1; i < count - 1; i++) {
    walk = walk * 0.78 + (rng() - 0.5) * 0.9;
    noise[i] = walk;
  }

  const smoothed = noise.map((value, i) => {
    if (i === 0 || i === count - 1) return 0;
    return (noise[i - 1]! + value * 2 + noise[i + 1]!) / 4;
  });
  const maxNoise = Math.max(1e-9, ...smoothed.map(Math.abs));
  const level = Math.max(1, Math.abs(start), Math.abs(end));
  const amplitude = Math.min(
    level * 0.012,
    Math.max(level * 0.0025, Math.abs(end - start) * 0.28),
  );

  return smoothed.map((value, i) => {
    if (i === 0) return start;
    if (i === count - 1) return end;
    const progress = i / (count - 1);
    const trend = start + (end - start) * progress;
    const bridge = Math.sin(Math.PI * progress);
    return Math.round(trend + amplitude * bridge * (value / maxNoise));
  });
}

export function intradayTimeLabel(index: number, points: number): string {
  const progress = points <= 1 ? 0 : Math.max(0, Math.min(1, index / (points - 1)));
  const minutes = OPEN_MINUTES + Math.round(SESSION_MINUTES * progress);
  return `${String(Math.floor(minutes / 60)).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`;
}
