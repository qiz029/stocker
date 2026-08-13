import React, { useState } from "react";
import {
  ScrollView, StyleSheet, Text, TextInput, TouchableOpacity, View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useRouter } from "expo-router";
import { api, ApiError, OHLC, Portfolio, RoomState } from "@core/api";
import { fmt$, fmtCents, fmtPct, fmtSignedCents } from "@core/format";
import { pickL } from "@core/i18n";
import { usePoll } from "@core/usePoll";
import { useSession } from "../../../src/session";
import CandleChart from "../../../src/components/CandleChart";
import OptionsChain from "../../../src/components/OptionsChain";
import OptionPositions from "../../../src/components/OptionPositions";
import ActionPanel from "../../../src/components/ActionPanel";
import { useSimClock } from "../../../src/hooks/useSimClock";
import { colors } from "../../../src/theme";

type PriceResponse = { days: OHLC[] };

export default function InstrumentScreen() {
  const { id: roomId, instrumentId } = useLocalSearchParams<{ id: string; instrumentId: string }>();
  const router = useRouter();
  const { t, lang } = useSession();

  const { data: state, error } = usePoll(
    () => api.get<RoomState>(`/api/rooms/${roomId}`), 5_000, [roomId]);
  const isMember = state ? state.room.is_member !== false : false;
  const { data: portfolio, reload: reloadPortfolio } = usePoll(
    () => (isMember ? api.get<Portfolio>(`/api/rooms/${roomId}/portfolio`) : Promise.resolve(null)),
    5_000, [roomId, isMember]);
  const { data: prices } = usePoll(
    () => api.get<PriceResponse>(`/api/rooms/${roomId}/prices/${instrumentId}`), 5_000,
    [roomId, instrumentId]);

  const clock = useSimClock(state?.room);
  const [chartWidth, setChartWidth] = useState(0);

  const [side, setSide] = useState<"buy" | "sell">("buy");
  const [raw, setRaw] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<{ ok: boolean; text: string } | null>(null);

  if (error && !state) {
    return (
      <SafeAreaView style={styles.safe}>
        <Text style={[styles.note, { padding: 20, color: colors.down }]}>{error}</Text>
      </SafeAreaView>
    );
  }
  if (!state) return <SafeAreaView style={styles.safe} />;

  const inst = state.instruments.find(i => i.id === instrumentId);
  if (!inst) {
    return (
      <SafeAreaView style={styles.safe}>
        <Text style={[styles.note, { padding: 20, color: colors.down }]}>{t("stock.unknown")}</Text>
      </SafeAreaView>
    );
  }

  const spectator = state.room.is_member === false;
  const closes = (prices?.days ?? []).map(d => d.close);
  const last = closes[closes.length - 1] ?? 0;
  const prev = closes[closes.length - 2] ?? last;
  const position = portfolio?.positions.find(p => p.instrument_id === instrumentId);
  const heldShares = position?.shares ?? 0;
  const pending = (portfolio?.pending ?? []).filter(o => o.instrument_id === instrumentId);
  const cash = portfolio?.cash_cents ?? 0;
  const tradeLocked = (portfolio?.bankrupt ?? false) || (state.room.ended ?? false);
  const afterHours = clock?.phase === "closed" || clock?.phase === "weekend";
  const optionPositions = (portfolio?.options ?? []).filter(o => o.instrument_id === instrumentId);
  const optionsNote = portfolio?.bankrupt
    ? t("trade.bankruptNote")
    : state.room.ended ? t("option.endedNote") : undefined;
  const actionsNote = portfolio?.bankrupt
    ? t("actions.bankruptNote")
    : state.room.ended ? t("actions.endedNote") : undefined;
  const aliasOf = (id: string) => state.instruments.find(i => i.id === id)?.alias ?? id;

  // Buy orders are dollar amounts; sell orders are share counts (server contract).
  const value = parseFloat(raw) || 0;
  const maxValue = side === "buy" ? cash / 100 : heldShares;
  const overLimit = value > maxValue + 1e-9;

  async function submit() {
    setBusy(true);
    setNotice(null);
    try {
      const body = side === "buy"
        ? { instrument_id: instrumentId, side, amount_cents: Math.round(value * 100) }
        : { instrument_id: instrumentId, side, shares: value };
      await api.post(`/api/rooms/${roomId}/orders`, body);
      setNotice({ ok: true, text: t("trade.ordered") });
      setRaw("");
      void reloadPortfolio();
    } catch (e) {
      setNotice({ ok: false, text: e instanceof ApiError ? e.message : t("trade.orderFailed") });
    } finally {
      setBusy(false);
    }
  }

  async function cancel(orderID: number) {
    setNotice(null);
    try {
      await api.del(`/api/rooms/${roomId}/orders/${orderID}`);
      setNotice({ ok: true, text: t("trade.cancelled") });
      void reloadPortfolio();
    } catch (e) {
      setNotice({ ok: false, text: e instanceof ApiError ? e.message : t("trade.cancelFailed") });
    }
  }

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <View style={styles.topbar}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={8}>
          <Text style={styles.back}>‹</Text>
        </TouchableOpacity>
        <Text style={styles.title} numberOfLines={1}>{inst.alias}</Text>
        <View style={{ width: 40 }} />
      </View>

      <ScrollView contentContainerStyle={styles.body}
        onLayout={e => setChartWidth(e.nativeEvent.layout.width - 32)}>
        <Text style={styles.sub} numberOfLines={1}>
          {pickL(lang, inst.desc, inst.desc_en)} · {inst.id}
        </Text>

        <CandleChart days={prices?.days ?? []} width={Math.max(0, chartWidth)}
          seed={Number(roomId) * 31 + [...inst.id].reduce((s, c) => s + c.charCodeAt(0), 0)} />

        <View style={styles.statStrip}>
          <Stat k={t("stock.todayClose")} v={last ? fmt$(last) : "—"} />
          <Stat k={t("stock.prevClose")} v={prev ? fmt$(prev) : "—"} />
          <Stat k={t("stock.sinceStart")} v={closes[0] ? fmtPct(last / closes[0]! - 1) : "—"} />
          {!spectator && (
            <Stat k={t("stock.myHolding")}
              v={heldShares > 0 ? t("unit.shares", { n: heldShares.toFixed(1) }) : "—"} />
          )}
        </View>

        {!spectator && position && (
          <View style={styles.card}>
            <Text style={styles.cardTitle}>{t("stock.myHolding")}</Text>
            <Text style={styles.meta}>
              {t("room.positionSub", { shares: position.shares.toFixed(1), value: fmtCents(position.value_cents) })}
            </Text>
            <Text style={[styles.meta, position.pnl_cents >= 0 ? styles.up : styles.down]}>
              {t("room.positionPnl", {
                avg: fmt$(position.avg_cost),
                amount: fmtSignedCents(position.pnl_cents),
                pct: fmtPct(position.pnl_pct),
              })}
            </Text>
          </View>
        )}

        {!spectator && optionPositions.length > 0 && (
          <View style={styles.card}>
            <Text style={styles.cardTitle}>{t("option.myPositions")}</Text>
            <OptionPositions roomId={roomId!} positions={optionPositions}
              currentDay={state.room.current_day ?? 0} aliasOf={aliasOf}
              onChanged={() => void reloadPortfolio()} disabled={tradeLocked} />
          </View>
        )}

        {!spectator && (
          <OptionsChain roomId={roomId!} instrumentId={instrumentId!} alias={inst.alias}
            lastClose={last} currentDay={state.room.current_day ?? 0}
            portfolio={portfolio} onChanged={() => void reloadPortfolio()}
            disabled={tradeLocked} note={optionsNote} />
        )}

        {!spectator && (
          <ActionPanel roomId={roomId!} instrumentId={instrumentId!} alias={inst.alias}
            portfolio={portfolio} onChanged={() => void reloadPortfolio()}
            disabled={tradeLocked} note={actionsNote} />
        )}

        {!spectator && (
          <View style={styles.card}>
            <View style={styles.tabs}>
              {(["buy", "sell"] as const).map(s => (
                <TouchableOpacity key={s} style={[styles.tab, side === s && (s === "buy" ? styles.tabBuy : styles.tabSell)]}
                  onPress={() => { setSide(s); setRaw(""); setNotice(null); }}>
                  <Text style={[styles.tabTxt, side === s && (s === "buy" ? styles.up : styles.down)]}>
                    {s === "buy" ? t("side.Buy") : t("side.Sell")}
                  </Text>
                </TouchableOpacity>
              ))}
            </View>

            <Text style={styles.fieldLabel}>{side === "buy" ? t("trade.buyAmount") : t("trade.sellShares")}</Text>
            <View style={styles.amt}>
              {side === "buy" && <Text style={styles.amtSign}>$</Text>}
              <TextInput style={styles.amtInput} placeholder="0" placeholderTextColor={colors.ink3}
                keyboardType="decimal-pad" value={raw} editable={!tradeLocked} onChangeText={setRaw} />
            </View>

            <View style={styles.chips}>
              {([["25%", 0.25], ["50%", 0.5], ["75%", 0.75], [t("trade.all"), 1]] as [string, number][]).map(([label, f]) => (
                <TouchableOpacity key={label} style={styles.chip} onPress={() => {
                  if (side === "buy") setRaw(String(Math.floor((cash / 100) * f)));
                  else setRaw((Math.min(heldShares, Math.floor(heldShares * f * 10) / 10)).toFixed(1));
                }}>
                  <Text style={styles.chipTxt}>{label}</Text>
                </TouchableOpacity>
              ))}
            </View>

            <View style={styles.estRow}>
              <Text style={styles.estK}>{t("trade.available")}</Text>
              <Text style={styles.estV}>
                {side === "buy" ? fmtCents(cash) : t("unit.shares", { n: heldShares.toFixed(1) })}
              </Text>
            </View>
            <View style={styles.estRow}>
              <Text style={styles.estK}>{side === "buy" ? t("trade.estShares") : t("trade.estAmount")}</Text>
              <Text style={styles.estV}>
                {value > 0 && last > 0
                  ? side === "buy"
                    ? `≈ ${t("unit.shares", { n: (value / last).toFixed(1) })}`
                    : `≈ ${fmt$(value * last)}`
                  : "—"}
              </Text>
            </View>

            <Text style={styles.note}>{t("trade.noteA")}{t("trade.noteB")}{t("trade.noteC")}</Text>
            {afterHours && <Text style={styles.note}>{t("trade.afterHours")}</Text>}
            {portfolio?.bankrupt && <Text style={styles.note}>{t("trade.bankruptNote")}</Text>}
            {notice && (
              <Text style={[styles.note, { color: notice.ok ? colors.up : colors.down }]}>{notice.text}</Text>
            )}

            <TouchableOpacity
              style={[styles.submit, side === "sell" && styles.submitSell,
                (busy || tradeLocked || value <= 0 || overLimit) && styles.disabled]}
              disabled={busy || tradeLocked || value <= 0 || overLimit}
              onPress={submit}>
              <Text style={styles.submitTxt}>{side === "buy" ? t("trade.placeBuy") : t("trade.placeSell")}</Text>
            </TouchableOpacity>

            {pending.length > 0 && (
              <View style={styles.pendingList}>
                <Text style={styles.fieldLabel}>{t("trade.pending")}</Text>
                {pending.map(o => (
                  <View key={o.id} style={styles.pendingItem}>
                    <Text style={styles.pendingTxt}>
                      {o.side === "buy"
                        ? t("trade.pendingBuy", { amount: fmtCents(o.amount_cents) })
                        : t("trade.pendingSell", { shares: o.shares.toFixed(1) })}
                      {" · "}{t("trade.pendingExec", { day: o.exec_day })}
                    </Text>
                    <TouchableOpacity onPress={() => void cancel(o.id)} hitSlop={6}>
                      <Text style={styles.cancelTxt}>{t("trade.cancel")}</Text>
                    </TouchableOpacity>
                  </View>
                ))}
              </View>
            )}
          </View>
        )}
      </ScrollView>
    </SafeAreaView>
  );
}

function Stat({ k, v }: { k: string; v: string }) {
  return (
    <View style={styles.stat}>
      <Text style={styles.statK}>{k}</Text>
      <Text style={styles.statV}>{v}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.bg },
  topbar: {
    flexDirection: "row", alignItems: "center", justifyContent: "space-between",
    paddingHorizontal: 16, paddingVertical: 10,
    borderBottomWidth: 1, borderBottomColor: colors.line,
  },
  back: { color: colors.up, fontSize: 30, fontWeight: "300", width: 40 },
  title: { color: colors.ink, fontSize: 16, fontWeight: "700", flex: 1, textAlign: "center" },
  body: { padding: 16, paddingBottom: 48 },
  sub: { color: colors.ink2, fontSize: 13 },
  big: { color: colors.ink, fontSize: 34, fontWeight: "600", marginTop: 8, fontVariant: ["tabular-nums"] },
  delta: { fontSize: 15, fontWeight: "500", marginTop: 2, marginBottom: 10, fontVariant: ["tabular-nums"] },
  statStrip: {
    flexDirection: "row", flexWrap: "wrap", gap: 14,
    borderTopWidth: 1, borderTopColor: colors.line, paddingTop: 12, marginTop: 10,
  },
  stat: { minWidth: "30%" },
  statK: { color: colors.ink3, fontSize: 11 },
  statV: { color: colors.ink, fontSize: 14, marginTop: 1, fontVariant: ["tabular-nums"] },
  card: {
    backgroundColor: colors.card, borderWidth: 1, borderColor: colors.line,
    borderRadius: 14, padding: 16, marginTop: 18,
  },
  cardTitle: {
    color: colors.ink2, fontSize: 12, fontWeight: "700", letterSpacing: 1.2,
    textTransform: "uppercase", marginBottom: 6,
  },
  meta: { color: colors.ink2, fontSize: 13, marginTop: 2, fontVariant: ["tabular-nums"] },
  up: { color: colors.up },
  down: { color: colors.down },
  tabs: { flexDirection: "row", borderBottomWidth: 1, borderBottomColor: colors.line, marginBottom: 14 },
  tab: { flex: 1, paddingVertical: 10, alignItems: "center", borderBottomWidth: 2, borderBottomColor: "transparent" },
  tabBuy: { borderBottomColor: colors.up },
  tabSell: { borderBottomColor: colors.down },
  tabTxt: { color: colors.ink2, fontSize: 15, fontWeight: "600" },
  fieldLabel: { color: colors.ink2, fontSize: 12, marginBottom: 6 },
  amt: {
    flexDirection: "row", alignItems: "center", gap: 6,
    backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 10, paddingHorizontal: 14, paddingVertical: 10,
  },
  amtSign: { color: colors.ink, fontSize: 19, fontWeight: "600" },
  amtInput: { flex: 1, color: colors.ink, fontSize: 19, fontWeight: "600", padding: 0 },
  chips: { flexDirection: "row", gap: 6, marginVertical: 10 },
  chip: {
    flex: 1, backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 999, paddingVertical: 5, alignItems: "center",
  },
  chipTxt: { color: colors.ink2, fontSize: 12 },
  estRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 3 },
  estK: { color: colors.ink2, fontSize: 13 },
  estV: { color: colors.ink, fontSize: 13, fontWeight: "500", fontVariant: ["tabular-nums"] },
  note: { color: colors.ink3, fontSize: 12, marginTop: 10, lineHeight: 18 },
  submit: {
    backgroundColor: colors.up, borderRadius: 999, paddingVertical: 13,
    alignItems: "center", marginTop: 14,
  },
  submitSell: { backgroundColor: colors.down },
  submitTxt: { color: "#04140a", fontWeight: "700", fontSize: 15 },
  disabled: { opacity: 0.35 },
  pendingList: { marginTop: 16, borderTopWidth: 1, borderTopColor: colors.line, paddingTop: 10 },
  pendingItem: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", paddingVertical: 6 },
  pendingTxt: { color: colors.ink2, fontSize: 13, fontVariant: ["tabular-nums"], flex: 1, marginRight: 10 },
  cancelTxt: { color: colors.ink3, fontSize: 13 },
});
