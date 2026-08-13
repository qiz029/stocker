export const FASTEST_TARGET_SECONDS = 30 * 60;
export const FASTEST_MIN_DAY_SECONDS = 2;

/** Integer seconds per trading day for an approximately thirty-minute room. */
export function fastestDayDuration(totalDays: number): number {
  if (totalDays <= 0) return 60;
  return Math.max(FASTEST_MIN_DAY_SECONDS, Math.round(FASTEST_TARGET_SECONDS / totalDays));
}
