import React, { useState } from "react";
import { StyleSheet, Text, TextInput, TouchableOpacity, View } from "react-native";
import { ApiError, LoanAction, Portfolio, postLoan } from "@core/api";
import { fmtCents } from "@core/format";
import { useSession } from "../session";
import { colors } from "../theme";

const fmtRate = (bp: number) => `${(bp / 100).toFixed(2)}%`;

/** Credit-line panel: borrow/repay against the room loan. Port of web LoanPanel. */
export default function LoanPanel({ roomId, portfolio, onChanged }: {
  roomId: string; portfolio: Portfolio | null; onChanged: () => void;
}) {
  const { t } = useSession();
  const [action, setAction] = useState<LoanAction>("borrow");
  const [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<{ ok: boolean; text: string } | null>(null);

  const cash = portfolio?.cash_cents ?? 0;
  const debt = portfolio?.debt_cents ?? 0;
  const maxDebt = portfolio?.max_debt_cents ?? 0;
  const rateBp = portfolio?.interest_rate_annual_bp ?? 0;
  const bankrupt = portfolio?.bankrupt ?? false;

  // borrow: % of remaining cap headroom; repay: % of min(cash, debt)
  const headroom = Math.max(0, maxDebt - debt);
  const repayable = Math.min(cash, debt);
  const maxCents = action === "borrow" ? headroom : repayable;

  const value = parseFloat(raw) || 0;
  const overLimit = value * 100 > maxCents + 1e-9;
  const capPct = maxDebt > 0 ? debt / maxDebt : 0;

  async function submit() {
    setBusy(true);
    setNotice(null);
    try {
      await postLoan(roomId, action, Math.round(value * 100));
      setNotice({ ok: true, text: t("loan.done") });
      setRaw("");
      onChanged();
    } catch (e) {
      setNotice({ ok: false, text: e instanceof ApiError ? e.message : t("loan.failed") });
    } finally {
      setBusy(false);
    }
  }

  return (
    <View style={styles.card}>
      <Text style={styles.title}>{t("loan.title")}</Text>
      <View style={styles.estRow}>
        <Text style={styles.estK}>{t("loan.debt")}</Text>
        <Text style={styles.estV}>{fmtCents(debt)}</Text>
      </View>
      <View style={styles.estRow}>
        <Text style={styles.estK}>{t("loan.rate")}</Text>
        <Text style={styles.estV}>{fmtRate(rateBp)}</Text>
      </View>
      <View style={styles.estRow}>
        <Text style={styles.estK}>{t("loan.capUsage", { used: fmtCents(debt), cap: fmtCents(maxDebt) })}</Text>
        <Text style={styles.estV}>{(capPct * 100).toFixed(0)}%</Text>
      </View>
      <View style={styles.capBar}>
        <View style={[styles.capFill, { flex: Math.min(1, capPct) }]} />
        <View style={{ flex: Math.max(0, 1 - capPct) }} />
      </View>
      {capPct >= 0.8 && !bankrupt && <Text style={styles.warn}>{t("loan.capWarning")}</Text>}

      <View style={styles.tabs}>
        {(["borrow", "repay"] as const).map(a => (
          <TouchableOpacity key={a} style={[styles.tab, action === a && (a === "borrow" ? styles.tabBuy : styles.tabSell)]}
            onPress={() => { setAction(a); setRaw(""); setNotice(null); }}>
            <Text style={[styles.tabTxt, action === a && (a === "borrow" ? styles.up : styles.down)]}>
              {a === "borrow" ? t("loan.borrow") : t("loan.repay")}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      <Text style={styles.fieldLabel}>{t("loan.amount")}</Text>
      <View style={styles.amt}>
        <Text style={styles.amtSign}>$</Text>
        <TextInput style={styles.amtInput} placeholder="0" placeholderTextColor={colors.ink3}
          keyboardType="decimal-pad" value={raw} editable={!bankrupt} onChangeText={setRaw} />
      </View>
      <View style={styles.chips}>
        {([["25%", 0.25], ["50%", 0.5], ["75%", 0.75], [t("trade.all"), 1]] as [string, number][]).map(([label, f]) => (
          <TouchableOpacity key={label} style={styles.chip} disabled={bankrupt}
            onPress={() => setRaw(String(Math.floor((maxCents / 100) * f)))}>
            <Text style={styles.chipTxt}>{label}</Text>
          </TouchableOpacity>
        ))}
      </View>
      {notice && <Text style={[styles.note, { color: notice.ok ? colors.up : colors.down }]}>{notice.text}</Text>}
      <TouchableOpacity
        style={[styles.submit, action === "repay" && styles.submitSell,
          (busy || bankrupt || value <= 0 || overLimit) && styles.disabled]}
        disabled={busy || bankrupt || value <= 0 || overLimit}
        onPress={submit}>
        <Text style={styles.submitTxt}>{action === "borrow" ? t("loan.placeBorrow") : t("loan.placeRepay")}</Text>
      </TouchableOpacity>
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
    textTransform: "uppercase", marginBottom: 8,
  },
  estRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 3 },
  estK: { color: colors.ink2, fontSize: 13 },
  estV: { color: colors.ink, fontSize: 13, fontWeight: "500", fontVariant: ["tabular-nums"] },
  capBar: { flexDirection: "row", height: 4, backgroundColor: colors.card2, borderRadius: 4, marginVertical: 8, overflow: "hidden" },
  capFill: { backgroundColor: colors.down, borderRadius: 4 },
  warn: { color: colors.down, fontSize: 12, marginBottom: 6 },
  tabs: { flexDirection: "row", borderBottomWidth: 1, borderBottomColor: colors.line, marginVertical: 12 },
  tab: { flex: 1, paddingVertical: 10, alignItems: "center", borderBottomWidth: 2, borderBottomColor: "transparent" },
  tabBuy: { borderBottomColor: colors.up },
  tabSell: { borderBottomColor: colors.down },
  tabTxt: { color: colors.ink2, fontSize: 14, fontWeight: "600" },
  up: { color: colors.up },
  down: { color: colors.down },
  fieldLabel: { color: colors.ink2, fontSize: 12, marginBottom: 6 },
  amt: {
    flexDirection: "row", alignItems: "center", gap: 6,
    backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 10, paddingHorizontal: 14, paddingVertical: 10,
  },
  amtSign: { color: colors.ink, fontSize: 17, fontWeight: "600" },
  amtInput: { flex: 1, color: colors.ink, fontSize: 17, fontWeight: "600", padding: 0 },
  chips: { flexDirection: "row", gap: 6, marginVertical: 10 },
  chip: {
    flex: 1, backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 999, paddingVertical: 5, alignItems: "center",
  },
  chipTxt: { color: colors.ink2, fontSize: 12 },
  note: { fontSize: 12, marginTop: 4 },
  submit: {
    backgroundColor: colors.up, borderRadius: 999, paddingVertical: 12,
    alignItems: "center", marginTop: 10,
  },
  submitSell: { backgroundColor: colors.down },
  submitTxt: { color: "#04140a", fontWeight: "700", fontSize: 14 },
  disabled: { opacity: 0.35 },
});
