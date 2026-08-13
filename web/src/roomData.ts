import { useEffect, useMemo, useRef, useState } from "react";
import { api, OHLC, Portfolio, Room, RoomState, Trade } from "./api";
import { assetCurve } from "./assetCurve";
import { usePoll } from "./usePoll";
import { SimClockState, simClock } from "./simClock";

export type PriceResponse = { days: OHLC[] };

export function buildSeriesMap(entries: [string, PriceResponse][]): Record<string, number[]> {
  const out: Record<string, number[]> = {};
  for (const [id, res] of entries) out[id] = res.days.map(d => d.close);
  return out;
}

export function buildOhlcMap(entries: [string, PriceResponse][]): Record<string, OHLC[]> {
  const out: Record<string, OHLC[]> = {};
  for (const [id, res] of entries) out[id] = res.days;
  return out;
}

export function useRoomData(roomId: string) {
  const { data: state, error, reload: reloadState } = usePoll(
    () => api.get<RoomState>(`/api/rooms/${roomId}`), 30_000, [roomId]);
  const isMember = state ? state.room.is_member !== false : false;
  const { data: portfolio, reload: reloadPortfolio } = usePoll(
    () => isMember ? api.get<Portfolio>(`/api/rooms/${roomId}/portfolio`) : Promise.resolve(null), 30_000, [roomId, isMember]);
  const { data: tradesRes, reload: reloadTrades } = usePoll(
    () => isMember ? api.get<{ items: Trade[] }>(`/api/rooms/${roomId}/trades`) : Promise.resolve({ items: [] }), 30_000, [roomId, isMember]);

  useEffect(() => {
    if (!state || state.room.day_duration_secs >= 60 || state.room.ended) return;
    const timer = setInterval(() => {
      void reloadState();
      void reloadPortfolio();
      void reloadTrades();
    }, 3_000);
    return () => clearInterval(timer);
  }, [state?.room.day_duration_secs, state?.room.ended, reloadState, reloadPortfolio, reloadTrades]);

  const [series, setSeries] = useState<Record<string, number[]>>({});
  const [ohlc, setOhlc] = useState<Record<string, OHLC[]>>({});
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
    ).then(entries => {
      setSeries(buildSeriesMap(entries));
      setOhlc(buildOhlcMap(entries));
    });
  }, [roomId, curDay, instrumentIds]);

  const trades = tradesRes?.items ?? [];
  const curve = useMemo(
    () => (curDay >= 0 && Object.keys(series).length ? assetCurve(trades, series, curDay) : []),
    [trades, series, curDay]);

  return {
    state, portfolio, trades, series, ohlc, curve, error,
    reload: () => { void reloadState(); void reloadPortfolio(); void reloadTrades(); },
  };
}

/** 房间运行中每秒刷新的模拟市场时钟;未启动或已结束时为 null。 */
export function useSimClock(room: Room | undefined): SimClockState | null {
  const [clock, setClock] = useState<SimClockState | null>(null);
  const startedAt = room?.started_at;
  const ended = room?.ended ?? false;
  const durationSecs = room?.day_duration_secs;
  const roomId = room?.id;
  const days = room?.days;
  useEffect(() => {
    if (!startedAt || ended || !durationSecs || !roomId || !days) {
      setClock(null);
      return;
    }
    const update = () => setClock(simClock(startedAt, durationSecs, roomId, days));
    update();
    const t = setInterval(update, 1000);
    return () => clearInterval(t);
  }, [startedAt, ended, durationSecs, roomId, days]);
  return clock;
}
