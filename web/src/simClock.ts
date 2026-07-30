/**
 * 模拟市场时钟:把墙钟在每个交易日区间内的进度映射为虚构日历
 * (周/星期索引,展示文案由 dayLabel + i18n 生成)和盘中 2-4 小时一跳的
 * 时钟(9:30→16:00),之后是收盘/周末时段。纯展示层,服务器的线性
 * day 映射不受影响。
 */

import type { Lang, MsgKey } from "./i18n";
import { tFor } from "./i18n";

export type ClockPhase = "open" | "closed" | "weekend" | "ended";

export interface SimClockState {
  day: number;
  week: number;                // 1 起始
  weekday: number;             // 0..4 → 周一..周五
  time: string | null;         // 盘中 "14:10",其余 null
  phase: ClockPhase;
  nextOpenSecs: number | null; // closed/weekend 时距下一开盘秒数;
                               // open/ended 阶段及最后一天收盘后均为 null(游戏已结束,不会再开盘)
}

export const SESSION_FRAC = 0.78;

const OPEN_MIN = 9 * 60 + 30;            // 570
const CLOSE_MIN = 16 * 60;               // 960
const TOTAL_MIN = CLOSE_MIN - OPEN_MIN;  // 390
const MIN_GAP = 120;
const MAX_GAP = 240;
// mulberry32:小型确定性 PRNG,返回 [0,1)
function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// 在 [lo, hi](均为 5 的倍数)中均匀取一个 5 的倍数
function pick5(rng: () => number, lo: number, hi: number): number {
  const count = (hi - lo) / 5 + 1;
  return lo + 5 * Math.min(count - 1, Math.floor(rng() * count));
}

/**
 * (roomId, day) 的盘中跳点,分钟数(自午夜)。首项 570(9:30),
 * 末项恰为 960(16:00),相邻间隔 120~240 分钟、5 分钟对齐。
 */
export function sessionTimes(roomId: number, day: number): number[] {
  const seed = (Math.imul(roomId, 2654435761) ^ Math.imul(day + 1, 40503)) >>> 0;
  const rng = mulberry32(seed);
  const n = rng() < 0.5 ? 2 : 3; // 跳数
  const times = [OPEN_MIN];
  let remaining = TOTAL_MIN;
  for (let k = n; k >= 1; k--) {
    // 留给后面 k-1 段的余量决定本段可行区间,保证末点恰为 16:00
    const gap = k === 1
      ? remaining
      : pick5(rng,
          Math.max(MIN_GAP, remaining - MAX_GAP * (k - 1)),
          Math.min(MAX_GAP, remaining - MIN_GAP * (k - 1)));
    times.push(times[times.length - 1]! + gap);
    remaining -= gap;
  }
  return times;
}

/** Fictional-calendar label for a day index, in the given UI language. */
export function dayLabel(day: number, lang: Lang = "en"): string {
  const t = tFor(lang);
  return t("cal.label", { week: Math.floor(day / 5) + 1, wd: t(`cal.wd.${day % 5}` as MsgKey) });
}

function fmtMin(mins: number): string {
  return `${Math.floor(mins / 60)}:${String(mins % 60).padStart(2, "0")}`;
}

export function simClock(
  startedAt: string,
  dayDurationSecs: number,
  roomId: number,
  totalDays: number,
  nowMs: number = Date.now(),
): SimClockState {
  const D = dayDurationSecs;
  const elapsed = Math.max(0, (nowMs - new Date(startedAt).getTime()) / 1000);
  let day = Math.floor(elapsed / D);
  const ended = day >= totalDays;
  if (ended) day = totalDays - 1;
  const weekday = day % 5;
  const base = {
    day, weekday,
    week: Math.floor(day / 5) + 1,
  };
  if (ended) return { ...base, time: null, phase: "ended", nextOpenSecs: null };

  const into = elapsed - day * D;
  const sessionSecs = D * SESSION_FRAC;
  if (into < sessionSecs) {
    const times = sessionTimes(roomId, day);
    const slice = Math.min(times.length - 1,
      Math.floor((into / sessionSecs) * times.length));
    return { ...base, time: fmtMin(times[slice]!), phase: "open", nextOpenSecs: null };
  }
  const isLastDay = day === totalDays - 1;
  return {
    ...base, time: null,
    phase: weekday === 4 ? "weekend" : "closed",
    nextOpenSecs: isLastDay ? null : Math.max(1, Math.round(D - into)),
  };
}
