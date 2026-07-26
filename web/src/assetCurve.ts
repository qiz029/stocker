import type { Trade } from "./api";
import { INITIAL_CASH_CENTS } from "./api";

/**
 * Rebuild the player's per-day total (cents) from settled trades + close
 * series. Matches the server's assetsCents convention: frozen buy cash is
 * still counted here as plain cash (the server adds it back explicitly),
 * and frozen sell shares are still counted as position value — so the
 * curve's last point equals the portfolio endpoint's total_cents.
 */
export function assetCurve(
  trades: Trade[],
  series: Record<string, number[]>,
  curDay: number,
): number[] {
  const byDay = new Map<number, Trade[]>();
  for (const t of trades) {
    const list = byDay.get(t.day);
    if (list) list.push(t);
    else byDay.set(t.day, [t]);
  }
  const out: number[] = [];
  let cash = INITIAL_CASH_CENTS;
  const shares: Record<string, number> = {};
  for (let d = 0; d <= curDay; d++) {
    for (const t of byDay.get(d) ?? []) {
      if (t.side === "buy") {
        cash -= t.amount_cents;
        shares[t.instrument_id] = (shares[t.instrument_id] ?? 0) + t.shares;
      } else {
        cash += t.amount_cents;
        shares[t.instrument_id] = (shares[t.instrument_id] ?? 0) - t.shares;
      }
    }
    let posVal = 0;
    for (const [inst, sh] of Object.entries(shares)) {
      posVal += sh * (series[inst]?.[d] ?? 0);
    }
    out.push(cash + Math.round(posVal * 100));
  }
  return out;
}
