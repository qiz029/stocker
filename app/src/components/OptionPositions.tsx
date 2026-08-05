import React, { useState } from "react";
import { StyleSheet, Text, TextInput, TouchableOpacity, View } from "react-native";
import { ApiError, OptionPosition, postOptionOrder } from "@core/api";
import { fmt$, fmtCents, fmtPct, fmtSignedCents } from "@core/format";
import { useSession } from "../session";
import { colors } from "../theme";

/** Held option contracts with a sell-to-close action. Port of web OptionPositions. */
export default function OptionPositions({ roomId, positions, currentDay, aliasOf, onChanged, disabled }: {
  roomId: string; positions: OptionPosition[]; currentDay: number;
  aliasOf: (instrumentID: string) => string; onChanged: () => void; disabled?: boolean;
}) {
  const { t } = useSession();
  const [raw, setRaw] = useState<Record<number, string>>({});
  const [busy, setBusy] = useState<number | null>(null);
  const [notice, setNotice] = useState<{ ok: boolean; text: string } | null>(null);

  async function sell(p: OptionPosition) {
    const n = Math.min(p.contracts, parseInt(raw[p.option_id] ?? "", 10) || p.contracts);
    if (n <= 0) return;
    setBusy(p.option_id);
    setNotice(null);
    try {
      await postOptionOrder(roomId, p.option_id, "sell", n);
      setNotice({ ok: true, text: t("option.sold") });
      onChanged();
    } catch (e) {
      setNotice({ ok: false, text: e instanceof ApiError ? e.message : t("option.orderFailed") });
    } finally {
      setBusy(null);
    }
  }

  return (
    <View>
      {positions.map(p => (
        <View key={p.option_id} style={styles.row}>
          <Text style={styles.name}>
            {t("option.positionDesc", {
              alias: aliasOf(p.instrument_id),
              kind: t(p.kind === "call" ? "option.kind.call" : "option.kind.put"),
              strike: fmt$(p.strike), day: p.expiry_day, left: p.expiry_day - currentDay,
            })}
          </Text>
          <Text style={styles.sub}>
            {t("option.positionSub", { n: p.contracts, value: fmtCents(p.value_cents) })}
          </Text>
          <Text style={[styles.sub, p.pnl_cents >= 0 ? styles.up : styles.down]}>
            {t("option.positionPnl", {
              avg: fmt$(p.avg_cost), amount: fmtSignedCents(p.pnl_cents), pct: fmtPct(p.pnl_pct),
            })}
          </Text>
          <View style={styles.sellRow}>
            <TextInput style={styles.sellInput} placeholder={String(p.contracts)}
              placeholderTextColor={colors.ink3} keyboardType="number-pad"
              value={raw[p.option_id] ?? ""} editable={!disabled}
              onChangeText={v => setRaw({ ...raw, [p.option_id]: v })} />
            <TouchableOpacity style={[styles.sellBtn, (disabled || busy === p.option_id) && styles.disabled]}
              disabled={disabled || busy === p.option_id} onPress={() => void sell(p)}>
              <Text style={styles.sellTxt}>{t("option.sellClose")}</Text>
            </TouchableOpacity>
          </View>
        </View>
      ))}
      {notice && (
        <Text style={[styles.note, { color: notice.ok ? colors.up : colors.down }]}>{notice.text}</Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { paddingVertical: 11, borderBottomWidth: 1, borderBottomColor: colors.line },
  name: { color: colors.ink, fontSize: 14, fontWeight: "600" },
  sub: { color: colors.ink3, fontSize: 12, marginTop: 2, fontVariant: ["tabular-nums"] },
  up: { color: colors.up },
  down: { color: colors.down },
  sellRow: { flexDirection: "row", gap: 8, marginTop: 8 },
  sellInput: {
    width: 76, backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 8, color: colors.ink, paddingHorizontal: 8, paddingVertical: 5, fontSize: 13,
  },
  sellBtn: {
    backgroundColor: colors.downSoft, borderRadius: 999,
    paddingHorizontal: 14, paddingVertical: 5, justifyContent: "center",
  },
  sellTxt: { color: colors.down, fontSize: 12, fontWeight: "600" },
  disabled: { opacity: 0.4 },
  note: { fontSize: 12, marginTop: 8 },
});
