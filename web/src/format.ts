export const fmtCents = (c: number): string =>
  "$" + (c / 100).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export const fmt$ = (v: number): string =>
  "$" + v.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export const fmtPct = (v: number): string =>
  (v >= 0 ? "+" : "-") + Math.abs(v * 100).toFixed(2) + "%";

export const RANGE_TABS: [string, number][] = [["7日", 7], ["1月", 21], ["3月", 63], ["全部", Infinity]];

export function windowed<T>(series: T[], days: number): [T[], number] {
  const start = days === Infinity ? 0 : Math.max(0, series.length - days);
  return [series.slice(start), start];
}

/** Map engine factor tokens in fallback headlines to display names. */
export function prettifyHeadline(h: string, aliasOf: (id: string) => string): string {
  return h
    .replace(/IDIO:(S\d+)/g, (_, id: string) => aliasOf(id))
    .replace(/\b(S\d+)\b/g, (_, id: string) => aliasOf(id))
    .replace(/(market|MKT)板块/g, "大盘")
    .replace(/(tech sector|TECH)板块/g, "科技板块")
    .replace(/(old economy|OLD)板块/g, "传统板块");
}
