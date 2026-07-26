import { useEffect, useMemo, useRef, useState } from "react";
import { api, Portfolio, RoomState, Trade } from "./api";
import { assetCurve } from "./assetCurve";
import { usePoll } from "./usePoll";

export type PriceResponse = { days: { open: number; high: number; low: number; close: number }[] };

export function buildSeriesMap(entries: [string, PriceResponse][]): Record<string, number[]> {
  const out: Record<string, number[]> = {};
  for (const [id, res] of entries) out[id] = res.days.map(d => d.close);
  return out;
}

/** Seconds until the next historical trading day begins. */
export function dayCountdown(startedAt: string, dayDurationSecs: number): number {
  const elapsed = (Date.now() - new Date(startedAt).getTime()) / 1000;
  const into = elapsed % dayDurationSecs;
  return Math.max(0, Math.round(dayDurationSecs - into));
}

export function useRoomData(roomId: string) {
  const { data: state, error, reload: reloadState } = usePoll(
    () => api.get<RoomState>(`/api/rooms/${roomId}`), 30_000, [roomId]);
  const { data: portfolio, reload: reloadPortfolio } = usePoll(
    () => api.get<Portfolio>(`/api/rooms/${roomId}/portfolio`), 30_000, [roomId]);
  const { data: tradesRes, reload: reloadTrades } = usePoll(
    () => api.get<{ items: Trade[] }>(`/api/rooms/${roomId}/trades`), 30_000, [roomId]);

  const [series, setSeries] = useState<Record<string, number[]>>({});
  const fetchedDay = useRef(-1);
  const curDay = state?.room.current_day ?? -1;
  const instrumentIds = useMemo(
    () => (state?.instruments ?? []).map(i => i.id).join(","), [state]);

  useEffect(() => {
    if (curDay < 0 || !instrumentIds || fetchedDay.current === curDay) return;
    fetchedDay.current = curDay;
    void Promise.all(
      instrumentIds.split(",").map(async id =>
        [id, await api.get<PriceResponse>(`/api/rooms/${roomId}/prices/${id}`)] as [string, PriceResponse]),
    ).then(entries => setSeries(buildSeriesMap(entries)));
  }, [roomId, curDay, instrumentIds]);

  const trades = tradesRes?.items ?? [];
  const curve = useMemo(
    () => (curDay >= 0 && Object.keys(series).length ? assetCurve(trades, series, curDay) : []),
    [trades, series, curDay]);

  return {
    state, portfolio, trades, series, curve, error,
    reload: () => { void reloadState(); void reloadPortfolio(); void reloadTrades(); },
  };
}
