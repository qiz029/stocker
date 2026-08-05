import React, { useState } from "react";
import { StyleSheet, Text, TouchableOpacity, View } from "react-native";
import {
  ApiError, HypeDirection, HypeTier, IntelResponse, Portfolio, postHype, postIntel,
} from "@core/api";
import { fmtCents } from "@core/format";
import { useSession } from "../session";
import { colors } from "../theme";

/* Mirrors the server-side tiers: fee / shock size / regulator catch risk. */
const TIERS: { tier: HypeTier; feeCents: number; shockPct: number; riskPct: number }[] = [
  { tier: 1, feeCents: 500_000, shockPct: 1.5, riskPct: 10 },
  { tier: 2, feeCents: 1_500_000, shockPct: 3, riskPct: 20 },
  { tier: 3, feeCents: 4_000_000, shockPct: 5, riskPct: 30 },
];

const INTEL_FEE_CENTS = 300_000;

/** Hype (manipulation) + intel (rumor tip) actions. Port of web ActionPanel. */
export default function ActionPanel({ roomId, instrumentId, alias, portfolio, onChanged, disabled, note }: {
  roomId: string; instrumentId: string; alias: string; portfolio: Portfolio | null;
  onChanged: () => void; disabled?: boolean; note?: string;
}) {
  const { t } = useSession();
  const [direction, setDirection] = useState<HypeDirection>("up");
  const [tier, setTier] = useState<HypeTier>(1);
  const [busy, setBusy] = useState(false);
  const [caughtFine, setCaughtFine] = useState<number | null>(null);
  const [tip, setTip] = useState<IntelResponse | null>(null);
  const [notice, setNotice] = useState<{ ok: boolean; text: string } | null>(null);

  const sel = TIERS.find(x => x.tier === tier)!;

  async function hype() {
    setBusy(true);
    setNotice(null);
    try {
      const res = await postHype(roomId, instrumentId, direction, tier);
      if (res.caught) {
        // Alarming, persistent banner: fined AND publicly exposed.
        setCaughtFine(res.fine_cents);
      } else {
        setCaughtFine(null);
        setNotice({ ok: true, text: t("actions.hypeDone") });
      }
      onChanged();
    } catch (e) {
      setNotice({ ok: false, text: e instanceof ApiError ? e.message : t("actions.failed") });
    } finally {
      setBusy(false);
    }
  }

  async function intel() {
    setBusy(true);
    setNotice(null);
    try {
      const res = await postIntel(roomId, instrumentId);
      setTip(res);
      onChanged();
    } catch (e) {
      setNotice({ ok: false, text: e instanceof ApiError ? e.message : t("actions.failed") });
    } finally {
      setBusy(false);
    }
  }

  return (
    <View style={styles.card}>
      <Text style={styles.title}>{t("actions.title")}</Text>

      <Text style={styles.fieldLabel}>{t("actions.hypeTitle")}</Text>
      <View style={styles.seg}>
        {(["up", "down"] as const).map(d => (
          <TouchableOpacity key={d} style={[styles.segBtn, direction === d && styles.segOn]}
            disabled={disabled} onPress={() => setDirection(d)}>
            <Text style={[styles.segTxt, direction === d && styles.segTxtOn]}>
              {d === "up" ? t("actions.direction.up") : t("actions.direction.down")}
            </Text>
          </TouchableOpacity>
        ))}
      </View>
      <View style={styles.tierRow}>
        {TIERS.map(x => (
          <TouchableOpacity key={x.tier} style={[styles.tier, x.tier === tier && styles.tierOn]}
            disabled={disabled} onPress={() => setTier(x.tier)}>
            <Text style={[styles.tierName, x.tier === tier && styles.up]}>{t("actions.tier", { n: x.tier })}</Text>
            <Text style={styles.tierMeta}>
              {t("actions.tierMeta", { fee: fmtCents(x.feeCents), shock: x.shockPct, risk: x.riskPct })}
            </Text>
          </TouchableOpacity>
        ))}
      </View>
      <Text style={styles.note}>{t("actions.hypeNote")}</Text>
      {note && <Text style={styles.note}>{note}</Text>}
      <View style={styles.estRow}>
        <Text style={styles.estK}>{t("trade.available")}</Text>
        <Text style={styles.estV}>{fmtCents(portfolio?.cash_cents ?? 0)}</Text>
      </View>
      {caughtFine !== null && (
        <View style={styles.caught}>
          <Text style={styles.caughtTitle}>{t("actions.caughtTitle")}</Text>
          <Text style={styles.caughtBody}>
            {t("actions.caughtBody", { amount: fmtCents(caughtFine), alias })}
          </Text>
        </View>
      )}
      {notice && (
        <Text style={[styles.note, { color: notice.ok ? colors.up : colors.down }]}>{notice.text}</Text>
      )}
      <TouchableOpacity style={[styles.submit, direction === "down" && styles.submitSell,
        (busy || disabled) && styles.disabled]}
        disabled={busy || disabled} onPress={hype}>
        <Text style={styles.submitTxt}>{t("actions.hypeSubmit", { fee: fmtCents(sel.feeCents) })}</Text>
      </TouchableOpacity>

      <View style={styles.intelBlock}>
        <Text style={styles.fieldLabel}>{t("actions.intelTitle")}</Text>
        <Text style={styles.note}>{t("actions.intelNote")}</Text>
        {tip && (
          <View style={styles.tipPanel}>
            <Text style={styles.fieldLabel}>{t("actions.tipHeading")}</Text>
            <Text style={[styles.tipOutlook,
              tip.outlook === "up" ? styles.up : tip.outlook === "down" ? styles.down : null]}>
              {t(`actions.tip.${tip.outlook}`)}
              {tip.strength && (
                <Text style={styles.tipStrength}> · {t(`actions.strength.${tip.strength}`)}</Text>
              )}
            </Text>
            <Text style={styles.note}>{t("actions.tipCaveat")}</Text>
          </View>
        )}
        <TouchableOpacity style={[styles.submit, (busy || disabled) && styles.disabled]}
          disabled={busy || disabled} onPress={intel}>
          <Text style={styles.submitTxt}>{t("actions.intelBuy", { fee: fmtCents(INTEL_FEE_CENTS) })}</Text>
        </TouchableOpacity>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: colors.card, borderWidth: 1, borderColor: colors.line,
    borderRadius: 14, padding: 16, marginTop: 18,
  },
  title: {
    color: colors.ink2, fontSize: 12, fontWeight: "700", letterSpacing: 1.2,
    textTransform: "uppercase", marginBottom: 10,
  },
  fieldLabel: { color: colors.ink2, fontSize: 12, marginBottom: 6 },
  seg: { flexDirection: "row", gap: 4, backgroundColor: colors.card2, borderRadius: 8, padding: 3, marginBottom: 10 },
  segBtn: { flex: 1, paddingVertical: 5, borderRadius: 6, alignItems: "center" },
  segOn: { backgroundColor: colors.card },
  segTxt: { color: colors.ink2, fontSize: 13 },
  segTxtOn: { color: colors.ink, fontWeight: "600" },
  tierRow: { flexDirection: "row", gap: 6, marginBottom: 4 },
  tier: {
    flex: 1, backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 10, paddingHorizontal: 6, paddingVertical: 8,
  },
  tierOn: { borderColor: colors.up },
  tierName: { color: colors.ink2, fontSize: 13, fontWeight: "600" },
  tierMeta: { color: colors.ink3, fontSize: 10, lineHeight: 15, marginTop: 2 },
  up: { color: colors.up },
  down: { color: colors.down },
  note: { color: colors.ink3, fontSize: 12, marginTop: 8, lineHeight: 17 },
  estRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 3, marginTop: 6 },
  estK: { color: colors.ink2, fontSize: 13 },
  estV: { color: colors.ink, fontSize: 13, fontWeight: "500", fontVariant: ["tabular-nums"] },
  caught: {
    borderWidth: 1, borderColor: colors.down, borderRadius: 12,
    backgroundColor: colors.downSoft, paddingHorizontal: 14, paddingVertical: 10, marginTop: 10,
  },
  caughtTitle: { color: colors.ink, fontSize: 13, fontWeight: "600" },
  caughtBody: { color: colors.ink2, fontSize: 12, marginTop: 2 },
  submit: {
    backgroundColor: colors.up, borderRadius: 999, paddingVertical: 12,
    alignItems: "center", marginTop: 12,
  },
  submitSell: { backgroundColor: colors.down },
  submitTxt: { color: "#04140a", fontWeight: "700", fontSize: 14 },
  disabled: { opacity: 0.35 },
  intelBlock: { marginTop: 18, borderTopWidth: 1, borderTopColor: colors.line, paddingTop: 14 },
  tipPanel: { backgroundColor: colors.card2, borderRadius: 10, paddingHorizontal: 14, paddingVertical: 10, marginVertical: 8 },
  tipOutlook: { color: colors.ink, fontSize: 14, fontWeight: "600" },
  tipStrength: { color: colors.ink3, fontWeight: "400", fontSize: 12 },
});
