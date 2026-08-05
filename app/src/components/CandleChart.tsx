import React, { useMemo, useState } from "react";
import { StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { Circle, Line, Polyline, Rect, Svg } from "react-native-svg";
import type { OHLC } from "@core/api";
import { RANGE_TABS, fmt$, fmtPct, windowed } from "@core/format";
import { buildIntradayCurve } from "@core/intradayCurve";
import { useSession } from "../session";
import { colors } from "../theme";

const PAD = { l: 6, r: 6, t: 12, b: 12 };

/** Daily-candle chart with range tabs (RN port of web CandleChart).
    The 1D tab shows a densified intraday bridge (core/intradayCurve) instead
    of two candles. Crosshair omitted by design. */
export default function CandleChart({ days, width, height = 200, seed = 1 }: {
  days: OHLC[]; width: number; height?: number; seed?: number;
}) {
  const { t } = useSession();
  const [rangeDays, setRangeDays] = useState<number>(Infinity);
  const [win] = useMemo(() => windowed(days, rangeDays), [days, rangeDays]);

  const isIntraday = rangeDays === 2 && win.length >= 1;
  const intraday = useMemo(() => {
    if (!isIntraday) return [];
    const last = win[win.length - 1]!;
    const start = win.length >= 2 ? win[win.length - 2]!.close : last.open;
    return buildIntradayCurve(start, last.close, seed + win.length);
  }, [isIntraday, win, seed]);

  if (width <= 0 || win.length === 0) return null;

  const ref = win[0]?.close ?? 0;
  const shown = win[win.length - 1]!;
  const diff = shown.close - ref;
  const pct = ref ? diff / ref : 0;
  const up = diff >= 0;

  let body: React.ReactNode = null;
  if (isIntraday && intraday.length > 1) {
    const min = Math.min(...intraday);
    const max = Math.max(...intraday);
    const span = max - min || 1;
    const dx = (width - PAD.l - PAD.r) / (intraday.length - 1);
    const pts = intraday.map((v, i) =>
      `${(PAD.l + i * dx).toFixed(2)},${(PAD.t + (1 - (v - min) / span) * (height - PAD.t - PAD.b)).toFixed(2)}`
    ).join(" ");
    body = (
      <Svg width={width} height={height}>
        <Polyline points={pts} fill="none" stroke={up ? colors.up : colors.down} strokeWidth={2} />
      </Svg>
    );
  } else if (win.length >= 2) {
    let lo = Math.min(...win.map(d => d.low));
    let hi = Math.max(...win.map(d => d.high));
    if (hi === lo) hi = lo + 1;
    const slot = (width - PAD.l - PAD.r) / win.length;
    const cx = (i: number) => PAD.l + slot * (i + 0.5);
    const y = (v: number) => height - PAD.b - ((height - PAD.b - PAD.t) * (v - lo)) / (hi - lo);
    const bodyW = Math.max(2, Math.min(24, slot * 0.6));
    const baseY = y(ref);
    body = (
      <Svg width={width} height={height}>
        <Line x1={PAD.l} y1={baseY} x2={width - PAD.r} y2={baseY}
          stroke="rgba(255,255,255,0.18)" strokeWidth={1} strokeDasharray="2 7" />
        {win.map((d, i) => {
          const c = d.close >= d.open ? colors.up : colors.down;
          const top = y(Math.max(d.open, d.close));
          const bot = y(Math.min(d.open, d.close));
          return (
            <React.Fragment key={i}>
              <Line x1={cx(i)} y1={y(d.high)} x2={cx(i)} y2={y(d.low)} stroke={c} strokeWidth={1.5} />
              <Rect x={cx(i) - bodyW / 2} y={top} width={bodyW} height={Math.max(2, bot - top)} fill={c} />
            </React.Fragment>
          );
        })}
        <Circle cx={cx(win.length - 1)} cy={y(shown.close)} r={3} fill={up ? colors.up : colors.down} />
      </Svg>
    );
  }

  return (
    <View>
      <View style={styles.hero}>
        <Text style={styles.big}>{fmt$(shown.close)}</Text>
        <Text style={[styles.delta, up ? styles.up : styles.down]}>
          {up ? "▲" : "▼"} {fmt$(Math.abs(diff))} ({fmtPct(pct)}) {t("hero.range")}
        </Text>
        {!isIntraday && (
          <Text style={styles.ohlc}>
            O {fmt$(shown.open)} · H {fmt$(shown.high)} · L {fmt$(shown.low)} · C {fmt$(shown.close)}
          </Text>
        )}
      </View>
      {body}
      <View style={styles.ranges}>
        {RANGE_TABS.map(([tabKey, d]) => (
          <TouchableOpacity key={tabKey} style={[styles.rangeBtn, rangeDays === d && styles.rangeOn]}
            onPress={() => setRangeDays(d)}>
            <Text style={[styles.rangeTxt, rangeDays === d && styles.rangeTxtOn]}>{t(`range.${tabKey}`)}</Text>
          </TouchableOpacity>
        ))}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  hero: { marginBottom: 6 },
  big: { color: colors.ink, fontSize: 30, fontWeight: "600", fontVariant: ["tabular-nums"] },
  delta: { fontSize: 13, fontWeight: "500", marginTop: 2, fontVariant: ["tabular-nums"] },
  ohlc: { color: colors.ink2, fontSize: 12, marginTop: 2, fontVariant: ["tabular-nums"] },
  up: { color: colors.up },
  down: { color: colors.down },
  ranges: {
    flexDirection: "row", gap: 4, borderBottomWidth: 1, borderBottomColor: colors.line,
    paddingBottom: 10, marginTop: 8,
  },
  rangeBtn: { paddingHorizontal: 12, paddingVertical: 4, borderRadius: 999 },
  rangeOn: { backgroundColor: colors.up },
  rangeTxt: { color: colors.ink2, fontSize: 12, fontWeight: "600" },
  rangeTxtOn: { color: "#04140a" },
});
