import { DebunkVerdict } from "./api";

const isVerdict = (value: unknown): value is DebunkVerdict =>
  value === "likely_true" || value === "likely_false" || value === "no_substance";

const storageKey = (userID: number, roomID: string) =>
  `stocker:debunk-verdicts:${userID}:${roomID}`;

/** Keep paid, private investigation results available while this browser tab is open. */
export function loadDebunkVerdicts(userID: number, roomID: string): Record<number, DebunkVerdict> {
  try {
    const parsed = JSON.parse(sessionStorage.getItem(storageKey(userID, roomID)) ?? "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    return Object.fromEntries(
      Object.entries(parsed).filter(([id, verdict]) => /^\d+$/.test(id) && isVerdict(verdict)),
    ) as Record<number, DebunkVerdict>;
  } catch {
    return {};
  }
}

export function saveDebunkVerdict(userID: number, roomID: string, newsID: number, verdict: DebunkVerdict) {
  try {
    const verdicts = loadDebunkVerdicts(userID, roomID);
    sessionStorage.setItem(storageKey(userID, roomID), JSON.stringify({ ...verdicts, [newsID]: verdict }));
  } catch {
    // Storage may be unavailable; the caller still keeps the verdict in memory.
  }
}
