import React from "react";
import { Polyline, Svg } from "react-native-svg";
import { colors } from "../theme";

/** Map a close series to svg polyline points within w×h (padding kept). */
export function linePoints(series: number[], w: number, h: number, pad = 2): string {
  if (series.length === 0) return "";
  if (series.length === 1) return `${pad},${h / 2} ${w - pad},${h / 2}`;
  const min = Math.min(...series);
  const max = Math.max(...series);
  const span = max - min || 1;
  const dx = (w - pad * 2) / (series.length - 1);
  return series
    .map((v, i) => `${(pad + i * dx).toFixed(2)},${(pad + (1 - (v - min) / span) * (h - pad * 2)).toFixed(2)}`)
    .join(" ");
}

export function seriesUp(series: number[]): boolean {
  if (series.length < 2) return true;
  return series[series.length - 1]! >= series[0]!;
}

/** Small trend sparkline for list rows. */
export default function Sparkline({ series, width = 88, height = 30 }: {
  series: number[]; width?: number; height?: number;
}) {
  if (series.length === 0) return null;
  const stroke = seriesUp(series) ? colors.up : colors.down;
  return (
    <Svg width={width} height={height}>
      <Polyline points={linePoints(series, width, height)} fill="none" stroke={stroke} strokeWidth={1.5} />
    </Svg>
  );
}
