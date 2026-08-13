import { useEffect, useMemo, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { RANGE_TABS, fmtPct, windowed } from "../format";
import { useT } from "../i18n";
import { buildIntradayCurve, intradayTimeLabel } from "../intradayCurve";
import { SESSION_FRAC, dayLabel } from "../simClock";
import { caf, prefersReducedMotion, raf } from "../useTweenedNumber";

const cssVar = (n: string) => getComputedStyle(document.documentElement).getPropertyValue(n).trim();
const PAD = { l: 6, r: 6, t: 22, b: 26 };
const W = 1560, H = 440;
const MORPH_MS = 420;

const easeOut = (t: number) => 1 - Math.pow(1 - t, 3);

/** Resample `src` onto `n` points (endpoints preserved) so two snapshots can interpolate. */
function resample(src: number[], n: number): number[] {
  if (src.length === n) return src.slice();
  if (n <= 1 || src.length <= 1) return src.slice(0, n);
  const out = new Array<number>(n);
  for (let i = 0; i < n; i++) out[i] = src[Math.round((i * (src.length - 1)) / (n - 1))]!;
  return out;
}

type Props = {
  label: string;
  series: number[];   // full history, day 0 .. current day
  startDay: number;   // day index of series[0]
  formatValue: (v: number) => string;
  intradaySeed?: number;
  liveDayStartMs?: number | null; // wall-clock start of the current sim day; enables the intraday crawl
  liveDaySecs?: number;           // sim day length in seconds
};

export default function HeroChart({ label, series, startDay, formatValue, intradaySeed = 0, liveDayStartMs = null, liveDaySecs = 0 }: Props) {
  const { lang, t } = useT();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [rangeDays, setRangeDays] = useState<number>(Infinity);
  const [hover, setHover] = useState<number | null>(null);

  const [dayWindow, winStart] = useMemo(() => windowed(series, rangeDays), [series, rangeDays]);
  const isIntraday = rangeDays === 2;
  const win = useMemo(() => {
    if (!isIntraday || dayWindow.length === 0) return dayWindow;
    const start = dayWindow.length > 1 ? dayWindow[0]! : dayWindow[dayWindow.length - 1]!;
    const end = dayWindow[dayWindow.length - 1]!;
    const day = startDay + series.length - 1;
    const seed = Math.imul(intradaySeed + 1, 2654435761) ^ Math.imul(day + 1, 40503) ^ Math.round(start);
    return buildIntradayCurve(start, end, seed);
  }, [dayWindow, intradaySeed, isIntraday, series.length, startDay]);

  // --- motion: morph between snapshots, and crawl the reveal along the live day
  const [frame, setFrame] = useState(0);
  const prevWin = useRef(win);
  const morphFrom = useRef(win);
  const morphStart = useRef(0);

  // Snapshot change detection during render (idempotent under StrictMode):
  // kick off a morph from the previous window, resampled onto the new length.
  if (prevWin.current !== win) {
    const prev = prevWin.current;
    prevWin.current = win;
    if (prev.length > 0 && win.length > 0 &&
        !(prev.length === win.length && prev.every((v, i) => v === win[i]))) {
      morphFrom.current = resample(prev, win.length);
      morphStart.current = prefersReducedMotion() ? 0 : performance.now();
    }
  }

  const morphT = morphStart.current ? Math.min(1, (performance.now() - morphStart.current) / MORPH_MS) : 1;
  const reveal = isIntraday && liveDayStartMs !== null && liveDaySecs > 0 && !prefersReducedMotion()
    ? Math.min(1, Math.max(2 / Math.max(2, win.length - 1),
        (Date.now() - liveDayStartMs) / (liveDaySecs * 1000 * SESSION_FRAC)))
    : 1;
  const animating = morphT < 1 || reveal < 1;

  useEffect(() => {
    if (!animating) return;
    const id = raf(() => setFrame(f => f + 1));
    return () => caf(id);
  }, [animating, frame]);

  const from = morphFrom.current;
  const morphed = morphT < 1 && from.length === win.length
    ? win.map((v, i) => from[i]! + (v - from[i]!) * easeOut(morphT))
    : win;
  const visibleCount = reveal < 1 ? Math.max(2, Math.ceil(win.length * reveal)) : win.length;
  const disp = visibleCount < win.length ? morphed.slice(0, visibleCount) : morphed;

  const hoverIdx = hover !== null ? Math.min(hover, disp.length - 1) : null;
  const shown = hoverIdx !== null && disp[hoverIdx] !== undefined ? disp[hoverIdx]! : disp[disp.length - 1] ?? 0;
  const ref = win[0] ?? 0;
  const up = shown >= ref;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || disp.length === 0) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const nFull = win.length, n = disp.length;
    ctx.clearRect(0, 0, W, H);
    // scale on the full window so the intraday reveal doesn't rescale mid-crawl
    let lo = Math.min(...win), hi = Math.max(...win);
    if (hi === lo) hi = lo + 1;
    const x = (i: number) => PAD.l + ((W - PAD.l - PAD.r) * i) / (nFull - 1);
    const y = (v: number) => H - PAD.b - ((H - PAD.b - PAD.t) * (v - lo)) / (hi - lo);
    const c = disp[n - 1]! >= disp[0]! ? cssVar("--up") : cssVar("--down");

    if (nFull === 1) {
      const valueY = y(win[0]!);
      ctx.strokeStyle = c;
      ctx.lineWidth = 3.5;
      ctx.beginPath();
      ctx.moveTo(PAD.l, valueY);
      ctx.lineTo(W - PAD.r, valueY);
      ctx.stroke();
      ctx.fillStyle = c;
      ctx.beginPath();
      ctx.arc(W - PAD.r, valueY, 7, 0, 7);
      ctx.fill();
      return;
    }

    // dashed period-start baseline
    ctx.strokeStyle = "rgba(255,255,255,0.18)";
    ctx.setLineDash([2, 7]);
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(PAD.l, y(win[0]!));
    ctx.lineTo(W - PAD.r, y(win[0]!));
    ctx.stroke();
    ctx.setLineDash([]);

    // line + gradient fill over the visible prefix, positioned by full-window index
    const grad = ctx.createLinearGradient(0, PAD.t, 0, H - PAD.b);
    grad.addColorStop(0, c + "26");
    grad.addColorStop(1, c + "00");
    ctx.beginPath();
    disp.forEach((v, i) => (i === 0 ? ctx.moveTo(x(i), y(v)) : ctx.lineTo(x(i), y(v))));
    ctx.strokeStyle = c;
    ctx.lineWidth = 3.5;
    ctx.lineJoin = "round";
    ctx.stroke();
    ctx.lineTo(x(n - 1), H - PAD.b);
    ctx.lineTo(PAD.l, H - PAD.b);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();

    if (hoverIdx !== null && hoverIdx < n) {
      ctx.strokeStyle = "rgba(255,255,255,0.35)";
      ctx.lineWidth = 1.5;
      ctx.setLineDash([4, 5]);
      ctx.beginPath();
      ctx.moveTo(x(hoverIdx), PAD.t - 8);
      ctx.lineTo(x(hoverIdx), H - PAD.b + 8);
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.fillStyle = c;
      ctx.beginPath();
      ctx.arc(x(hoverIdx), y(disp[hoverIdx]!), 8, 0, 7);
      ctx.fill();
      ctx.fillStyle = cssVar("--bg");
      ctx.beginPath();
      ctx.arc(x(hoverIdx), y(disp[hoverIdx]!), 3.5, 0, 7);
      ctx.fill();
    }
    // no canvas end dot: the CSS .live-dot overlay marks the live tip
  }, [win, disp, hoverIdx]);

  // pulsing dot over the live tip (percentages, so it scales with the canvas)
  let tip: { left: string; top: string; up: boolean } | null = null;
  if (hover === null && win.length > 1 && disp.length > 0) {
    let lo = Math.min(...win), hi = Math.max(...win);
    if (hi === lo) hi = lo + 1;
    const i = disp.length - 1;
    const px = PAD.l + ((W - PAD.l - PAD.r) * i) / (win.length - 1);
    const py = H - PAD.b - ((H - PAD.b - PAD.t) * (disp[i]! - lo)) / (hi - lo);
    tip = { left: `${(px / W) * 100}%`, top: `${(py / H) * 100}%`, up: disp[i]! >= disp[0]! };
  }

  function onPointerMove(e: ReactPointerEvent<HTMLCanvasElement>) {
    const canvas = canvasRef.current!;
    const r = canvas.getBoundingClientRect();
    const i = Math.round(((e.clientX - r.left) / r.width) * (win.length - 1));
    setHover(Math.max(0, Math.min(i, win.length - 1)));
  }

  const diff = shown - ref;
  const pct = ref ? diff / ref : 0;

  return (
    <div>
      <div className="hero">
        <div className="label">{label}</div>
        <div className="big num">{formatValue(shown)}</div>
        <div className={`delta num ${up ? "up" : "down"}`}>
          {up ? "▲" : "▼"} {formatValue(Math.abs(diff)).replace("-", "")} ({fmtPct(pct)}) {t("hero.range")}
        </div>
        <div className="scrub-date num">
          {hoverIdx !== null
            ? isIntraday ? intradayTimeLabel(hoverIdx, win.length) : dayLabel(startDay + winStart + hoverIdx, lang)
            : " "}
        </div>
      </div>
      <div className="chart-box">
        <canvas
          className="chart" width={W} height={H} ref={canvasRef}
          onPointerMove={onPointerMove} onPointerLeave={() => setHover(null)}
        />
        {tip && (
          <span
            className={`live-dot ${tip.up ? "up" : "down"}`}
            style={{ left: tip.left, top: tip.top }}
            aria-hidden="true"
          />
        )}
      </div>
      <div className="ranges">
        {RANGE_TABS.map(([tabKey, days]) => (
          <button key={tabKey} className={rangeDays === days ? "on" : ""}
            onClick={() => { setRangeDays(days); setHover(null); }}>
            {t(`range.${tabKey}`)}
          </button>
        ))}
      </div>
    </div>
  );
}
