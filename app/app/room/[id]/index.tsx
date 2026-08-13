import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  AppState, FlatList, RefreshControl, StyleSheet, Text, TouchableOpacity, View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useRouter } from "expo-router";
import { api, ApiError, INITIAL_CASH_CENTS, OHLC, Portfolio, RoomState } from "@core/api";
import { fmt$, fmtCents, fmtPct } from "@core/format";
import { pickL } from "@core/i18n";
import { dayLabel } from "@core/simClock";
import { usePoll } from "@core/usePoll";
import { useSession } from "../../../src/session";
import Sparkline from "../../../src/components/Sparkline";
import LangToggle from "../../../src/components/LangToggle";
import RoomTabs from "../../../src/components/RoomTabs";
import LoanPanel from "../../../src/components/LoanPanel";
import { mmss, useSimClock } from "../../../src/hooks/useSimClock";
import { colors } from "../../../src/theme";

type PriceResponse = { days: OHLC[] };

export default function RoomScreen() {
  const { id: roomId } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { t, lang, user } = useSession();

  const { data: state, error, reload: reloadState } = usePoll(
    () => api.get<RoomState>(`/api/rooms/${roomId}`), 5_000, [roomId]);
  const isMember = state ? state.room.is_member !== false : false;
  const { data: portfolio, reload: reloadPortfolio } = usePoll(
    () => (isMember ? api.get<Portfolio>(`/api/rooms/${roomId}/portfolio`) : Promise.resolve(null)),
    30_000, [roomId, isMember]);

  const reload = React.useCallback(() => {
    void reloadState();
    void reloadPortfolio();
  }, [reloadState, reloadPortfolio]);

  // Reload immediately when returning to the foreground.
  useEffect(() => {
    const sub = AppState.addEventListener("change", s => { if (s === "active") reload(); });
    return () => sub.remove();
  }, [reload]);

  // Daily close series per instrument, refetched when the sim day advances.
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
        [id, await api.get<PriceResponse>(`/api/rooms/${roomId}/prices/${id}`)] as const),
    ).then(entries => {
      const out: Record<string, number[]> = {};
      for (const [id, res] of entries) out[id] = res.days.map(d => d.close);
      setSeries(out);
    }).catch(() => { fetchedDay.current = -1; });
  }, [roomId, curDay, instrumentIds]);

  const room = state?.room;
  const clock = useSimClock(room);
  const [refreshing, setRefreshing] = useState(false);
  const [joining, setJoining] = useState(false);
  const [joinError, setJoinError] = useState<string | null>(null);

  async function onRefresh() {
    setRefreshing(true);
    await reload();
    setRefreshing(false);
  }

  async function joinRoom() {
    setJoining(true);
    setJoinError(null);
    try {
      await api.post(`/api/rooms/${roomId}/join`);
      reload();
    } catch (e) {
      setJoinError(e instanceof ApiError ? e.message : t("lobby.joinFailed"));
    } finally {
      setJoining(false);
    }
  }

  if (error && !state) {
    return (
      <SafeAreaView style={styles.safe}>
        <Text style={[styles.errorText, { padding: 20 }]}>{error}</Text>
      </SafeAreaView>
    );
  }
  if (!state || !room) return <SafeAreaView style={styles.safe} />;

  const spectator = room.is_member === false;
  const myRow = user ? state.leaderboard.find(r => r.username === user.display_name?.trim()) : undefined;
  const myRank = myRow ? state.leaderboard.indexOf(myRow) + 1 : null;
  const returnPct = myRow?.return_pct
    ?? (portfolio ? portfolio.total_cents / INITIAL_CASH_CENTS - 1 : 0);
  const dateLabel = clock ? dayLabel(clock.day, lang) : "";

  const header = (
    <View>
      {/* status / clock line */}
      <View style={styles.statusRow}>
        <View style={styles.dayPill}>
          <Text style={styles.dayPillTxt}>
            {room.status === "lobby"
              ? `${room.name || t("era.name")} · ${t("status.waiting")}`
              : room.ended
                ? `${room.name || t("era.name")} · ${t("room.endedPill")}`
                : `${room.name ? `${room.name} · ` : ""}${t("era.name")} · ${t("lobby.dayA")} ${room.current_day ?? 0} ${t("lobby.dayB", { days: room.days })}`}
          </Text>
        </View>
        {clock && clock.phase !== "ended" && (
          <Text style={styles.countdown}>
            {clock.phase === "open"
              ? `${dateLabel} ${clock.time ?? ""}`
              : `${clock.phase === "weekend" ? t("room.weekend") : `${dateLabel} · ${t("room.closed")}`}` +
                (clock.nextOpenSecs !== null ? ` · ${t("room.nextOpen")} ${mmss(clock.nextOpenSecs)}` : "")}
          </Text>
        )}
      </View>

      {spectator && (
        <View style={styles.spectatorBanner}>
          <Text style={styles.spectatorTxt}>
            {lang === "zh"
              ? "围观模式 — 你可以查看行情与榜单，但不能交易。"
              : "Spectator mode — you can watch the market, but cannot trade."}
          </Text>
          <TouchableOpacity style={styles.joinBtn} disabled={joining} onPress={joinRoom}>
            <Text style={styles.joinBtnTxt}>{lang === "zh" ? "加入对局" : "Join game"}</Text>
          </TouchableOpacity>
        </View>
      )}
      {joinError && <Text style={styles.errorText}>{joinError}</Text>}

      {room.status === "lobby" ? (
        <View style={styles.card}>
          <Text style={styles.cardTitle}>{t("room.notStarted")}</Text>
          {!spectator && room.invite_code && (
            <Text style={styles.meta}>
              {t("room.shareA")} {room.invite_code} {t("room.shareB")}
              {room.is_host ? t("room.shareHost") : t("room.shareEnd")}
            </Text>
          )}
          {spectator
            ? null
            : !room.is_host && <Text style={[styles.meta, { marginTop: 10 }]}>{t("room.waitingHost")}</Text>}
        </View>
      ) : (
        <>
          {portfolio && !spectator && (
            <View style={styles.hero}>
              <Text style={styles.heroLabel}>{t("room.totalAssets")}</Text>
              <Text style={styles.heroBig}>{fmtCents(portfolio.total_cents)}</Text>
              <Text style={[styles.heroDelta, returnPct >= 0 ? styles.up : styles.down]}>
                {fmtPct(returnPct)}{myRank !== null ? `   ·   #${myRank}` : ""}
              </Text>
              {portfolio.debt_cents > 0 && (
                <Text style={styles.meta}>
                  {t("loan.capUsage", { used: fmtCents(portfolio.debt_cents), cap: fmtCents(portfolio.max_debt_cents) })}
                </Text>
              )}
              {portfolio.bankrupt && (
                <Text style={styles.bankrupt}>{t("room.bankruptBanner")}</Text>
              )}
              <TouchableOpacity onPress={() => router.push(`/room/${roomId}/trades`)} hitSlop={6}>
                <Text style={styles.tradesLink}>{t("trades.title")} ›</Text>
              </TouchableOpacity>
            </View>
          )}
          {room.ended && !spectator && (
            <TouchableOpacity style={styles.revealBtn} onPress={() => router.push(`/room/${roomId}/reveal`)}>
              <Text style={styles.revealBtnTxt}>{t("room.reveal")}</Text>
            </TouchableOpacity>
          )}
          {!spectator && !room.ended && (
            <LoanPanel roomId={roomId!} portfolio={portfolio} onChanged={reload} />
          )}
          <Text style={styles.sectionTitle}>{t("room.market")}</Text>
        </>
      )}
    </View>
  );

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <View style={styles.topbar}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={8}>
          <Text style={styles.back}>‹</Text>
        </TouchableOpacity>
        <Text style={styles.brand}><Text style={{ color: colors.up }}>●</Text> Stocker</Text>
        <LangToggle />
      </View>
      <RoomTabs roomId={roomId!} active="market" />

      <FlatList
        data={room.status === "lobby" ? [] : state.instruments}
        keyExtractor={i => i.id}
        contentContainerStyle={styles.list}
        ListHeaderComponent={header}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={colors.ink2} />}
        renderItem={({ item }) => {
          const q = state.quotes.find(x => x.instrument_id === item.id);
          const s = series[item.id] ?? [];
          const change = q ? q.close / q.prev_close - 1 : 0;
          const up = q ? q.close >= q.prev_close : true;
          return (
            <TouchableOpacity style={styles.row}
              onPress={() => router.push(`/room/${roomId}/${item.id}`)}>
              <View style={styles.rowLeft}>
                <Text style={styles.rowName} numberOfLines={1}>{item.alias}</Text>
                <Text style={styles.rowDesc} numberOfLines={1}>{pickL(lang, item.desc, item.desc_en)}</Text>
              </View>
              <Sparkline series={s.slice(Math.max(0, s.length - 30))} width={74} height={30} />
              <View style={styles.rowRight}>
                <Text style={styles.rowPrice}>{q ? fmt$(q.close) : "—"}</Text>
                {q && (
                  <View style={[styles.pill, up ? styles.pillUp : styles.pillDown]}>
                    <Text style={[styles.pillTxt, up ? styles.up : styles.down]}>{fmtPct(change)}</Text>
                  </View>
                )}
              </View>
            </TouchableOpacity>
          );
        }}
      />
    </SafeAreaView>
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
  brand: { color: colors.ink, fontSize: 16, fontWeight: "700" },
  list: { padding: 16, paddingBottom: 40 },
  statusRow: { flexDirection: "row", alignItems: "center", gap: 10, flexWrap: "wrap", marginBottom: 12 },
  dayPill: {
    borderWidth: 1, borderColor: colors.line, borderRadius: 999,
    paddingHorizontal: 12, paddingVertical: 3,
  },
  dayPillTxt: { color: colors.ink2, fontSize: 12, fontVariant: ["tabular-nums"] },
  countdown: { color: colors.ink3, fontSize: 12, fontVariant: ["tabular-nums"] },
  spectatorBanner: {
    backgroundColor: colors.card, borderWidth: 1, borderColor: colors.line, borderRadius: 12,
    padding: 14, marginBottom: 12, gap: 10,
  },
  spectatorTxt: { color: colors.ink2, fontSize: 13 },
  joinBtn: { backgroundColor: colors.up, borderRadius: 999, paddingVertical: 10, alignItems: "center" },
  joinBtnTxt: { color: "#04140a", fontWeight: "700", fontSize: 14 },
  card: {
    backgroundColor: colors.card, borderWidth: 1, borderColor: colors.line,
    borderRadius: 14, padding: 18, marginBottom: 12,
  },
  cardTitle: { color: colors.ink, fontSize: 16, fontWeight: "600" },
  meta: { color: colors.ink2, fontSize: 13, marginTop: 6 },
  hero: { marginBottom: 18 },
  heroLabel: { color: colors.ink2, fontSize: 13 },
  heroBig: { color: colors.ink, fontSize: 34, fontWeight: "600", marginTop: 2, fontVariant: ["tabular-nums"] },
  heroDelta: { fontSize: 15, fontWeight: "500", marginTop: 2, fontVariant: ["tabular-nums"] },
  bankrupt: {
    color: colors.down, backgroundColor: colors.downSoft, borderRadius: 10,
    padding: 10, marginTop: 10, fontSize: 13, overflow: "hidden",
  },
  sectionTitle: {
    color: colors.ink2, fontSize: 12, fontWeight: "700", letterSpacing: 1.2,
    textTransform: "uppercase", marginBottom: 4,
  },
  row: {
    flexDirection: "row", alignItems: "center", gap: 10,
    paddingVertical: 11, borderBottomWidth: 1, borderBottomColor: colors.line,
  },
  rowLeft: { flex: 1, minWidth: 0 },
  rowName: { color: colors.ink, fontSize: 15, fontWeight: "600" },
  rowDesc: { color: colors.ink3, fontSize: 12, marginTop: 1 },
  rowRight: { alignItems: "flex-end", minWidth: 86 },
  rowPrice: { color: colors.ink, fontSize: 15, fontWeight: "600", fontVariant: ["tabular-nums"] },
  pill: { borderRadius: 6, paddingHorizontal: 8, paddingVertical: 2, marginTop: 3, minWidth: 74, alignItems: "center" },
  pillUp: { backgroundColor: colors.upSoft },
  pillDown: { backgroundColor: colors.downSoft },
  pillTxt: { fontSize: 12, fontWeight: "600", fontVariant: ["tabular-nums"] },
  up: { color: colors.up },
  down: { color: colors.down },
  tradesLink: { color: colors.up, fontSize: 13, marginTop: 10 },
  revealBtn: {
    backgroundColor: colors.up, borderRadius: 999, paddingVertical: 12,
    alignItems: "center", marginBottom: 14,
  },
  revealBtnTxt: { color: "#04140a", fontWeight: "700", fontSize: 14 },
  errorText: { color: colors.down, fontSize: 13, marginBottom: 10 },
});
