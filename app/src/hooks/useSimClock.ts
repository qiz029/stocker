import { useEffect, useState } from "react";
import { Room } from "@core/api";
import { SimClockState, simClock } from "@core/simClock";

/** Sim market clock ticking every second while the room runs; null when
    the room hasn't started or has ended (mirrors web/src/roomData.ts). */
export function useSimClock(room: Room | undefined | null): SimClockState | null {
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

export const mmss = (s: number): string =>
  `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
