import AsyncStorage from "@react-native-async-storage/async-storage";
import { DebunkVerdict } from "@core/api";

const isVerdict = (value: unknown): value is DebunkVerdict =>
  value === "likely_true" || value === "likely_false" || value === "no_substance";

const storageKey = (userID: number, roomID: string) =>
  `stocker:debunk-verdicts:${userID}:${roomID}`;

/** Paid, private investigation results, persisted on-device (RN port of
    web/src/debunkVerdicts.ts, backed by AsyncStorage instead of sessionStorage). */
export async function loadDebunkVerdicts(userID: number, roomID: string): Promise<Record<number, DebunkVerdict>> {
  try {
    const parsed = JSON.parse((await AsyncStorage.getItem(storageKey(userID, roomID))) ?? "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    return Object.fromEntries(
      Object.entries(parsed).filter(([id, verdict]) => /^\d+$/.test(id) && isVerdict(verdict)),
    ) as Record<number, DebunkVerdict>;
  } catch {
    return {};
  }
}

export async function saveDebunkVerdict(userID: number, roomID: string, newsID: number, verdict: DebunkVerdict) {
  try {
    const verdicts = await loadDebunkVerdicts(userID, roomID);
    await AsyncStorage.setItem(storageKey(userID, roomID), JSON.stringify({ ...verdicts, [newsID]: verdict }));
  } catch {
    // Storage may be unavailable; the caller still keeps the verdict in memory.
  }
}
