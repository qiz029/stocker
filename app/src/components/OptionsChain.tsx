import React, { useEffect, useMemo, useState } from "react";
import { StyleSheet, Text, TextInput, TouchableOpacity, View } from "react-native";
import { ApiError, OptionContract, Portfolio, fetchOptions, postOptionOrder } from "@core/api";
import { fmt$, fmtCents } from "@core/format";
import { useSession } from "../session";
import { colors } from "../theme";

/** Options chain for one instrument: expiry pills, call/strike/put rows,
    buy-to-open ticket. Port of web OptionsChain (RN styles, inline notice). */
export default function OptionsChain({ roomId, instrumentId, alias, lastClose, currentDay, portfolio, onChanged, disabled, note }: {
  roomId: string; instrumentId: string; alias: string; lastClose: number; currentDay: number;
  portfolio: Portfolio | null; onChanged: () => void;
  disabled?: boolean; note?: string;
}) {
  const { t } = useSession();
  const [chain, setChain] = useState<OptionContract[]>([]);
  const [expiry, setExpiry] = useState<number | null>(null);
  const [sel, setSel] = useState<OptionContract | null>(null);
  const [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    let live = true;
    setSel(null);
    setExpiry(null);
    setRaw("");
    fetchOptions(roomId, instrumentId)
      .then(cs => { if (live) setChain(cs); })
      .catch(() => { if (live) setChain([]); });
    return () => { live = false; };
  }, [roomId, instrumentId, currentDay]);

  const expiries = useMemo(
    () => [...new Set(chain.map(c => c.expiry_day))].sort((a, b) => a - b), [chain]);
  const activeExpiry = expiry ?? expiries[0] ?? null;
  const rows = useMemo(() => {
    const byStrike = new Map<number, { call?: OptionContract; put?: OptionContract }>();
    for (const c of chain) {
      if (c.expiry_day !== activeExpiry) continue;
      const r = byStrike.get(c.strike) ?? {};
      r[c.kind] = c;
      byStrike.set(c.strike, r);
    }
    return [...byStrike.entries()].sort((a, b) => a[0] - b[0]);
  }, [chain, activeExpiry]);

  const cash = portfolio?.cash_cents ?? 0;
  const contracts = parseInt(raw, 10) || 0;
  // Server prices the premium as round(price * contracts * 100) cents.
  const maxByCash = sel && sel.price > 0 ? Math.floor(cash / (sel.price * 100)) : 0;
  const overLimit = contracts > maxByCash;
  const premiumCents = sel ? Math.round(sel.price * contracts * 100) : 0;

  async function buy() {
    if (!sel) return;
    setBusy(true);
    setNotice(null);
    try {
      await postOptionOrder(roomId, sel.option_id, "buy", contracts);
      setNotice({ ok: true, text: t("option.bought", { n: contracts }) });
      setSel(null);
      setRaw("");
      onChanged();
    } catch (e) {
      setNotice({ ok: false, text: e instanceof ApiError ? e.message : t("option.orderFailed") });
    } finally {
      setBusy(false);
    }
  }

  const selDesc = sel
    ? t("option.positionDesc", {
        alias, kind: t(sel.kind === "call" ? "option.kind.call" : "option.kind.put"),
        strike: fmt$(sel.strike), day: sel.expiry_day, left: sel.expiry_day - currentDay,
      })
    : "";

  return (
    <View style={styles.card}>
      <Text style={styles.title}>{t("option.chainTitle")}</Text>
      {chain.length === 0 ? (
        <Text style={styles.empty}>{t("option.empty")}</Text>
      ) : (
        <>
          <View style={styles.pills}>
            {expiries.map(d => (
              <TouchableOpacity key={d} style={[styles.pill, d === activeExpiry && styles.pillOn]}
                onPress={() => { setExpiry(d); setSel(null); setRaw(""); }}>
                <Text style={[styles.pillTxt, d === activeExpiry && styles.pillTxtOn]}>
                  {t("option.expiryPill", { day: d, left: d - currentDay })}
                </Text>
              </TouchableOpacity>
            ))}
          </View>

          <View style={styles.headRow}>
            <Text style={[styles.headCell, { flex: 1 }]}>{t("option.thCalls")}</Text>
            <Text style={[styles.headCell, { width: 76 }]}>{t("option.thStrike")}</Text>
            <Text style={[styles.headCell, { flex: 1 }]}>{t("option.thPuts")}</Text>
          </View>
          {rows.map(([strike, r]) => (
            <View key={strike} style={styles.row}>
              <View style={{ flex: 1, alignItems: "center" }}>
                {r.call && (
                  <TouchableOpacity
                    style={[styles.px, strike < lastClose && styles.pxCallItm,
                      sel?.option_id === r.call.option_id && styles.pxSel]}
                    disabled={disabled}
                    onPress={() => { setSel(sel?.option_id === r.call!.option_id ? null : r.call!); setRaw(""); }}>
                    <Text style={[styles.pxTxt, strike < lastClose && styles.up]}>{fmt$(r.call.price)}</Text>
                  </TouchableOpacity>
                )}
              </View>
              <Text style={[styles.strike, { width: 76 }]}>{fmt$(strike)}</Text>
              <View style={{ flex: 1, alignItems: "center" }}>
                {r.put && (
                  <TouchableOpacity
                    style={[styles.px, strike > lastClose && styles.pxPutItm,
                      sel?.option_id === r.put.option_id && styles.pxSel]}
                    disabled={disabled}
                    onPress={() => { setSel(sel?.option_id === r.put!.option_id ? null : r.put!); setRaw(""); }}>
                    <Text style={[styles.pxTxt, strike > lastClose && styles.down]}>{fmt$(r.put.price)}</Text>
                  </TouchableOpacity>
                )}
              </View>
            </View>
          ))}
          {note && <Text style={styles.note}>{note}</Text>}

          {sel && (
            <View style={styles.ticket}>
              <Text style={styles.fieldLabel}>{selDesc}</Text>
              <Text style={styles.fieldLabel}>{t("option.contracts")}</Text>
              <View style={styles.amt}>
                <TextInput style={styles.amtInput} placeholder="0" placeholderTextColor={colors.ink3}
                  keyboardType="number-pad" value={raw} editable={!disabled} onChangeText={setRaw} />
              </View>
              <View style={styles.chips}>
                {[1, 5, 10].map(n => (
                  <TouchableOpacity key={n} style={styles.chip} onPress={() => setRaw(String(n))}>
                    <Text style={styles.chipTxt}>{n}</Text>
                  </TouchableOpacity>
                ))}
                <TouchableOpacity style={styles.chip} onPress={() => setRaw(String(maxByCash))}>
                  <Text style={styles.chipTxt}>{t("trade.all")}</Text>
                </TouchableOpacity>
              </View>
              <View style={styles.estRow}>
                <Text style={styles.estK}>{t("trade.available")}</Text>
                <Text style={styles.estV}>{fmtCents(cash)}</Text>
              </View>
              <View style={styles.estRow}>
                <Text style={styles.estK}>{t("option.estPremium")}</Text>
                <Text style={styles.estV}>{contracts > 0 ? `≈ ${fmtCents(premiumCents)}` : "—"}</Text>
              </View>
              {notice && (
                <Text style={[styles.note, { color: notice.ok ? colors.up : colors.down }]}>{notice.text}</Text>
              )}
              <TouchableOpacity
                style={[styles.submit, (busy || disabled || contracts <= 0 || overLimit) && styles.disabled]}
                disabled={busy || disabled || contracts <= 0 || overLimit}
                onPress={buy}>
                <Text style={styles.submitTxt}>{t("option.buyToOpen")}</Text>
              </TouchableOpacity>
            </View>
          )}
        </>
      )}
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
  empty: { color: colors.ink3, fontSize: 13 },
  pills: { flexDirection: "row", flexWrap: "wrap", gap: 6, marginBottom: 10 },
  pill: {
    backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 999, paddingHorizontal: 12, paddingVertical: 4,
  },
  pillOn: { backgroundColor: colors.up, borderColor: colors.up },
  pillTxt: { color: colors.ink2, fontSize: 12 },
  pillTxtOn: { color: "#04140a", fontWeight: "600" },
  headRow: {
    flexDirection: "row", borderBottomWidth: 1, borderBottomColor: colors.line,
    paddingBottom: 6, marginBottom: 2,
  },
  headCell: {
    color: colors.ink2, fontSize: 11, textTransform: "uppercase", letterSpacing: 0.8,
    textAlign: "center",
  },
  row: {
    flexDirection: "row", alignItems: "center",
    borderBottomWidth: 1, borderBottomColor: colors.line, paddingVertical: 5,
  },
  strike: { color: colors.ink2, fontSize: 13, fontWeight: "600", textAlign: "center", fontVariant: ["tabular-nums"] },
  px: {
    minWidth: 84, borderRadius: 8, borderWidth: 1, borderColor: colors.line,
    backgroundColor: colors.card2, paddingHorizontal: 10, paddingVertical: 5, alignItems: "center",
  },
  pxCallItm: { backgroundColor: colors.upSoft, borderColor: "transparent" },
  pxPutItm: { backgroundColor: colors.downSoft, borderColor: "transparent" },
  pxSel: { borderColor: colors.up, borderWidth: 2 },
  pxTxt: { color: colors.ink, fontSize: 13, fontWeight: "600", fontVariant: ["tabular-nums"] },
  up: { color: colors.up },
  down: { color: colors.down },
  note: { color: colors.ink3, fontSize: 12, marginTop: 10, lineHeight: 18 },
  ticket: { marginTop: 14, borderTopWidth: 1, borderTopColor: colors.line, paddingTop: 14 },
  fieldLabel: { color: colors.ink2, fontSize: 12, marginBottom: 6 },
  amt: {
    backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 10, paddingHorizontal: 14, paddingVertical: 8,
  },
  amtInput: { color: colors.ink, fontSize: 17, fontWeight: "600", padding: 0 },
  chips: { flexDirection: "row", gap: 6, marginVertical: 10 },
  chip: {
    flex: 1, backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 999, paddingVertical: 5, alignItems: "center",
  },
  chipTxt: { color: colors.ink2, fontSize: 12 },
  estRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 3 },
  estK: { color: colors.ink2, fontSize: 13 },
  estV: { color: colors.ink, fontSize: 13, fontWeight: "500", fontVariant: ["tabular-nums"] },
  submit: {
    backgroundColor: colors.up, borderRadius: 999, paddingVertical: 12,
    alignItems: "center", marginTop: 12,
  },
  submitTxt: { color: "#04140a", fontWeight: "700", fontSize: 14 },
  disabled: { opacity: 0.35 },
});
