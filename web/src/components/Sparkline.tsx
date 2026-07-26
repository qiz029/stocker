import { useEffect, useRef } from "react";

const cssVar = (n: string) => getComputedStyle(document.documentElement).getPropertyValue(n).trim();

export default function Sparkline({ series }: { series: number[] }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = ref.current;
    if (!canvas || series.length < 2) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const W = (canvas.width = 176), H = (canvas.height = 60);
    ctx.clearRect(0, 0, W, H);
    let lo = Math.min(...series), hi = Math.max(...series);
    if (hi === lo) hi = lo + 1;
    ctx.strokeStyle = series[series.length - 1]! >= series[0]! ? cssVar("--up") : cssVar("--down");
    ctx.lineWidth = 3;
    ctx.lineJoin = "round";
    ctx.beginPath();
    series.forEach((v, i) => {
      const x = 3 + ((W - 6) * i) / (series.length - 1);
      const y = H - 5 - ((H - 10) * (v - lo)) / (hi - lo);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
  }, [series]);
  return <canvas ref={ref} />;
}
