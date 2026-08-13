import type { ScenarioInfo } from "@core/api";
import type { Lang } from "@core/i18n";
import { fastestDayDuration } from "@core/tempo";

/* Era helpers ported from web/src/pages/Lobby.tsx (without the avif art). */

const eraYears: Record<string, number> = {
  "nifty-1972": 1972, "crash-1987": 1987, "dotcom-2000": 2000, "gfc-2008": 2008,
};

export function scenarioYear(sc: ScenarioInfo): number {
  const known = eraYears[sc.id];
  if (known) return known;
  const match = (sc.id + " " + sc.name).match(/(?:19|20)\d{2}/);
  return match ? Number(match[0]) : Number.MAX_SAFE_INTEGER;
}

export function chronologicalScenarios(items: ScenarioInfo[]): ScenarioInfo[] {
  return [...items].sort((a, b) => scenarioYear(a) - scenarioYear(b) || a.name.localeCompare(b.name));
}

export function yearLabel(sc: ScenarioInfo): string {
  const year = scenarioYear(sc);
  return year === Number.MAX_SAFE_INTEGER ? "ALT" : String(year);
}

export function durationOptions(days: number, lang: Lang): [string, number][] {
  const result: [string, number][] = [1, 2, 4].map(weeks => {
    const secs = Math.max(60, Math.round((weeks * 604800) / days));
    return [lang === "zh"
      ? `${weeks} 周（每交易日约 ${Math.round(secs / 60)} 分钟）`
      : `${weeks} weeks (~${Math.round(secs / 60)} min/day)`, secs];
  });
  result.push([lang === "zh" ? "测试局（每交易日 1 分钟）" : "Test game (1 min/day)", 60]);
  const fastest = fastestDayDuration(days);
  result.push([lang === "zh"
    ? `闪电局（约 ${Math.round(days * fastest / 60)} 分钟，精简动态）`
    : `Flash (~${Math.round(days * fastest / 60)} min, condensed activity)`, fastest]);
  return result;
}

export function fmtReturn(value?: number): string {
  if (value === undefined) return "—";
  return (value >= 0 ? "+" : "") + (value * 100).toFixed(1) + "%";
}
