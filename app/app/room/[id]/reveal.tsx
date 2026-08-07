import React, { useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useRouter } from "expo-router";
import { ApiError, RevealData, api } from "@core/api";
import { fmt$, fmtCents, fmtPct } from "@core/format";
import { pickL } from "@core/i18n";
import { useSession } from "../../../src/session";
import Avatar from "../../../src/components/Avatar";
import { colors } from "../../../src/theme";

/** End-of-game reveal: true identities, final standings, all trades replay. */
export default function RevealScreen() {
  const { id: roomId } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { t, lang } = useSession();
  const [data, setData] = useState<RevealData | null>(null);
  const [notReady, setNotReady] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.get<RevealData>(`/api/rooms/${roomId}/reveal`)
      .then(setData)
      .catch(e => {
        if (e instanceof ApiError && e.status === 409) setNotReady(true);
        else setError(e instanceof Error ? e.message : String(e));
      });
  }, [roomId]);

  const aliasOf = (id: string) => data?.instruments.find(i => i.id === id)?.alias ?? id;
  const hasRealNames = data?.instruments.some(i => i.real_name !== "") ?? false;

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <View style={styles.topbar}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={8}>
          <Text style={styles.back}>‹</Text>
        </TouchableOpacity>
        <Text style={styles.title}>{t("reveal.title")}</Text>
        <View style={{ width: 40 }} />
      </View>
      <ScrollView contentContainerStyle={styles.body}>
        {error && <Text style={styles.error}>{error}</Text>}
        {notReady && (
          <View>
            <Text style={styles.h1}>{t("reveal.notReadyTitle")}</Text>
            <Text style={styles.sub}>{t("reveal.notReadySub")}</Text>
          </View>
        )}
        {data && (
          <>
            <Text style={styles.sub}>{t("reveal.sub")}</Text>

            <Text style={styles.sectionTitle}>{t("reveal.finalBoard")}</Text>
            <View style={styles.card}>
              {data.leaderboard.map((row, i) => (
                <View key={row.username} style={styles.lbRow}>
                  <Text style={styles.rank}>{i === 0 ? "🏆" : i + 1}</Text>
                  {!row.is_agent && <Avatar id={row.avatar_id} username={row.username} size={24} />}
                  <View style={{ flex: 1, flexDirection: "row", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
                    <Text style={styles.lbName}>{row.is_agent ? pickL(lang, row.username, row.username_en) : row.username}</Text>
                    {row.is_agent && <Text style={styles.agentBadge}>{t("common.agent")}</Text>}
                    {row.late_join && <Text style={styles.meta}>{t("reveal.lateJoin")}</Text>}
                  </View>
                  <View style={{ alignItems: "flex-end" }}>
                    <Text style={styles.lbVal}>{fmtCents(row.total_cents)}</Text>
                    <Text style={[styles.lbRet, row.return_pct >= 0 ? styles.up : styles.down]}>
                      {fmtPct(row.return_pct)}
                    </Text>
                  </View>
                </View>
              ))}
            </View>

            <Text style={styles.sectionTitle}>{t("reveal.identities")}</Text>
            <View style={styles.card}>
              {data.real_period && (
                <Text style={styles.meta}>
                  {t("reveal.realPeriod")}<Text style={styles.ink}>{data.real_period}</Text>
                </Text>
              )}
              {!hasRealNames && <Text style={styles.meta}>{t("reveal.syntheticNote")}</Text>}
              <View style={styles.tableHead}>
                <Text style={[styles.th, { flex: 1 }]}>{t("reveal.alias")}</Text>
                <Text style={[styles.th, { flex: 1 }]}>{t("reveal.realName")}</Text>
              </View>
              {data.instruments.map(inst => (
                <View key={inst.id} style={styles.tableRow}>
                  <Text style={[styles.td, { flex: 1 }]}>
                    {inst.alias} <Text style={styles.meta}>{inst.id}</Text>
                  </Text>
                  <Text style={[styles.td, { flex: 1 }]}>{inst.real_name || "——"}</Text>
                </View>
              ))}
            </View>

            <Text style={styles.sectionTitle}>{t("reveal.tradesReplay")}</Text>
            <View style={styles.card}>
              <View style={styles.tableHead}>
                <Text style={[styles.th, { width: 30 }]}>{t("reveal.thDay")}</Text>
                <Text style={[styles.th, { flex: 1.2 }]}>{t("reveal.thPlayer")}</Text>
                <Text style={[styles.th, { width: 40 }]}>{t("reveal.thSide")}</Text>
                <Text style={[styles.th, { flex: 1 }]}>{t("reveal.thTicker")}</Text>
                <Text style={[styles.th, styles.num, { flex: 1 }]}>{t("reveal.thAmount")}</Text>
              </View>
              {data.trades.map((tr, i) => (
                <View key={i} style={styles.tableRow}>
                  <Text style={[styles.td, styles.num, { width: 30 }]}>{tr.day}</Text>
                  <Text style={[styles.td, { flex: 1.2 }]} numberOfLines={1}>
                    {tr.is_agent ? pickL(lang, tr.username, tr.username_en) : tr.username}{tr.is_agent ? ` (${t("common.agent")})` : ""}
                  </Text>
                  <Text style={[styles.td, { width: 40 }, tr.side === "buy" ? styles.up : styles.down]}>
                    {tr.side === "buy" ? t("side.Buy") : t("side.Sell")}
                  </Text>
                  <Text style={[styles.td, { flex: 1 }]} numberOfLines={1}>{aliasOf(tr.instrument_id)}</Text>
                  <Text style={[styles.td, styles.num, { flex: 1 }]}>
                    {fmtCents(tr.amount_cents)}
                    <Text style={styles.meta}>{`\n${fmt$(tr.price)} × ${tr.shares.toFixed(1)}`}</Text>
                  </Text>
                </View>
              ))}
            </View>
          </>
        )}
      </ScrollView>
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
  title: { color: colors.ink, fontSize: 16, fontWeight: "700" },
  body: { padding: 16, paddingBottom: 48 },
  h1: { color: colors.ink, fontSize: 22, fontWeight: "700", marginTop: 8 },
  sub: { color: colors.ink2, fontSize: 13, marginTop: 6, marginBottom: 10 },
  error: { color: colors.down, fontSize: 13, marginBottom: 10 },
  sectionTitle: {
    color: colors.ink2, fontSize: 12, fontWeight: "700", letterSpacing: 1.2,
    textTransform: "uppercase", marginTop: 16, marginBottom: 8,
  },
  card: {
    backgroundColor: colors.card, borderWidth: 1, borderColor: colors.line,
    borderRadius: 14, padding: 14,
  },
  lbRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingVertical: 7 },
  rank: { color: colors.ink3, fontSize: 12, width: 22, fontVariant: ["tabular-nums"] },
  lbName: { color: colors.ink, fontSize: 14, fontWeight: "600" },
  lbVal: { color: colors.ink, fontSize: 13, fontWeight: "600", fontVariant: ["tabular-nums"] },
  lbRet: { fontSize: 11, fontVariant: ["tabular-nums"] },
  agentBadge: {
    color: colors.up, fontSize: 9, fontWeight: "700", textTransform: "uppercase",
    paddingHorizontal: 5, paddingVertical: 1, borderRadius: 999,
    borderWidth: 1, borderColor: colors.line, overflow: "hidden",
  },
  meta: { color: colors.ink3, fontSize: 11, fontVariant: ["tabular-nums"] },
  ink: { color: colors.ink },
  up: { color: colors.up },
  down: { color: colors.down },
  num: { textAlign: "right", fontVariant: ["tabular-nums"] },
  tableHead: {
    flexDirection: "row", gap: 8, borderBottomWidth: 1, borderBottomColor: colors.line,
    paddingBottom: 6, marginTop: 8, marginBottom: 2,
  },
  th: {
    color: colors.ink2, fontSize: 10, textTransform: "uppercase", letterSpacing: 0.8,
  },
  tableRow: {
    flexDirection: "row", gap: 8, alignItems: "center",
    borderBottomWidth: 1, borderBottomColor: colors.line, paddingVertical: 7,
  },
  td: { color: colors.ink, fontSize: 12 },
});
