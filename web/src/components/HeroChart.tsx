import { useEffect, useMemo, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { RANGE_TABS, fmtPct, windowed } from "../format";
import { useT } from "../i18n";
import { dayLabel } from "../simClock";

const cssVar = (n: string) => getComputedStyle(document.documentElement).getPropertyValue(n).trim();
const PAD = { l: 6, r: 6, t: 22, b: 26 };

type Props = {
  label: string;
  series: number[];   // full history, day 0 .. current day
  startDay: number;   // day index of series[0]
  formatValue: (v: number) => string;
};

export default function HeroChart({ label, series, startDay, formatValue }: Props) {
  const { lang, t } = useT();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [rangeDays, setRangeDays] = useState<number>(Infinity);
  const [hover, setHover] = useState<number | null>(null);

  const [win, winStart] = useMemo(() => windowed(series, rangeDays), [series, rangeDays]);
  const shown = hover !== null && win[hover] !== undefined ? win[hover]! : win[win.length - 1] ?? 0;
  const ref = win[0] ?? 0;
  const up = shown >= ref;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || win.length === 0) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const W = canvas.width, H = canvas.height, n = win.length;
    ctx.clearRect(0, 0, W, H);
    let lo = Math.min(...win), hi = Math.max(...win);
    if (hi === lo) hi = lo + 1;
    const x = (i: number) => PAD.l + ((W - PAD.l - PAD.r) * i) / (n - 1);
    const y = (v: number) => H - PAD.b - ((H - PAD.b - PAD.t) * (v - lo)) / (hi - lo);
    const c = win[n - 1]! >= win[0]! ? cssVar("--up") : cssVar("--down");

    if (n === 1) {
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

    // line + gradient fill
    const grad = ctx.createLinearGradient(0, PAD.t, 0, H - PAD.b);
    grad.addColorStop(0, c + "26");
    grad.addColorStop(1, c + "00");
    ctx.beginPath();
    win.forEach((v, i) => (i === 0 ? ctx.moveTo(x(i), y(v)) : ctx.lineTo(x(i), y(v))));
    ctx.strokeStyle = c;
    ctx.lineWidth = 3.5;
    ctx.lineJoin = "round";
    ctx.stroke();
    ctx.lineTo(x(n - 1), H - PAD.b);
    ctx.lineTo(PAD.l, H - PAD.b);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();

    if (hover !== null && hover < n) {
      ctx.strokeStyle = "rgba(255,255,255,0.35)";
      ctx.lineWidth = 1.5;
      ctx.setLineDash([4, 5]);
      ctx.beginPath();
      ctx.moveTo(x(hover), PAD.t - 8);
      ctx.lineTo(x(hover), H - PAD.b + 8);
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.fillStyle = c;
      ctx.beginPath();
      ctx.arc(x(hover), y(win[hover]!), 8, 0, 7);
      ctx.fill();
      ctx.fillStyle = cssVar("--bg");
      ctx.beginPath();
      ctx.arc(x(hover), y(win[hover]!), 3.5, 0, 7);
      ctx.fill();
    } else {
      ctx.fillStyle = c;
      ctx.beginPath();
      ctx.arc(x(n - 1), y(win[n - 1]!), 7, 0, 7);
      ctx.fill();
    }
  }, [win, hover]);

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
          {hover !== null ? dayLabel(startDay + winStart + hover, lang) : " "}
        </div>
      </div>
      <div className="chart-box">
        <canvas
          className="chart" width={1560} height={440} ref={canvasRef}
          onPointerMove={onPointerMove} onPointerLeave={() => setHover(null)}
        />
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
