import { useEffect, useMemo, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import type { OHLC } from "../api";
import { RANGE_TABS, fmtPct, windowed } from "../format";
import { useT } from "../i18n";
import { dayLabel } from "../simClock";

const cssVar = (n: string) => getComputedStyle(document.documentElement).getPropertyValue(n).trim();
const PAD = { l: 6, r: 6, t: 22, b: 26 };

type Props = {
  label: string;
  days: OHLC[];              // full history, day 0 .. current day
  formatValue: (v: number) => string;
};

export default function CandleChart({ label, days, formatValue }: Props) {
  const { lang, t } = useT();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [rangeDays, setRangeDays] = useState<number>(Infinity);
  const [hover, setHover] = useState<number | null>(null);

  const [win, winStart] = useMemo(() => windowed(days, rangeDays), [days, rangeDays]);
  const shown: OHLC = (hover !== null && win[hover]) || win[win.length - 1] || { open: 0, high: 0, low: 0, close: 0 };
  const ref = win[0]?.close ?? 0;
  const up = shown.close >= ref;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || win.length < 2) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const W = canvas.width, H = canvas.height, n = win.length;
    ctx.clearRect(0, 0, W, H);
    let lo = Math.min(...win.map(d => d.low)), hi = Math.max(...win.map(d => d.high));
    if (hi === lo) hi = lo + 1;
    const slot = (W - PAD.l - PAD.r) / n;
    const cx = (i: number) => PAD.l + slot * (i + 0.5);
    const y = (v: number) => H - PAD.b - ((H - PAD.b - PAD.t) * (v - lo)) / (hi - lo);
    const upC = cssVar("--up"), downC = cssVar("--down");
    const bodyW = Math.max(2, Math.min(28, slot * 0.6));

    // dashed period-start baseline (window's first close)
    ctx.strokeStyle = "rgba(255,255,255,0.18)";
    ctx.setLineDash([2, 7]);
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(PAD.l, y(win[0]!.close));
    ctx.lineTo(W - PAD.r, y(win[0]!.close));
    ctx.stroke();
    ctx.setLineDash([]);

    win.forEach((d, i) => {
      const c = d.close >= d.open ? upC : downC;
      const x = cx(i);
      // wick
      ctx.strokeStyle = c;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(x, y(d.high));
      ctx.lineTo(x, y(d.low));
      ctx.stroke();
      // body (min 2px so doji days stay visible)
      const top = y(Math.max(d.open, d.close)), bot = y(Math.min(d.open, d.close));
      ctx.fillStyle = c;
      ctx.fillRect(x - bodyW / 2, top, bodyW, Math.max(2, bot - top));
    });

    if (hover !== null && hover < n) {
      ctx.strokeStyle = "rgba(255,255,255,0.35)";
      ctx.lineWidth = 1.5;
      ctx.setLineDash([4, 5]);
      ctx.beginPath();
      ctx.moveTo(cx(hover), PAD.t - 8);
      ctx.lineTo(cx(hover), H - PAD.b + 8);
      ctx.stroke();
      ctx.setLineDash([]);
    }
  }, [win, hover]);

  function onPointerMove(e: ReactPointerEvent<HTMLCanvasElement>) {
    const canvas = canvasRef.current!;
    const r = canvas.getBoundingClientRect();
    const i = Math.floor(((e.clientX - r.left) / r.width) * win.length);
    setHover(Math.max(0, Math.min(i, win.length - 1)));
  }

  const diff = shown.close - ref;
  const pct = ref ? diff / ref : 0;

  return (
    <div>
      <div className="hero">
        <div className="label">{label}</div>
        <div className="big num">{formatValue(shown.close)}</div>
        <div className={`delta num ${up ? "up" : "down"}`}>
          {up ? "▲" : "▼"} {formatValue(Math.abs(diff)).replace("-", "")} ({fmtPct(pct)}) {t("hero.range")}
        </div>
        <div className="ohlc num">
          O {formatValue(shown.open)} · H {formatValue(shown.high)} · L {formatValue(shown.low)} · C {formatValue(shown.close)}
        </div>
        <div className="scrub-date num">
          {hover !== null ? dayLabel(winStart + hover, lang) : " "}
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
