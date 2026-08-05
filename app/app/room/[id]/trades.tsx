import React from "react";
import { FlatList, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useRouter } from "expo-router";
import { RoomState, Trade, api } from "@core/api";
import { fmt$, fmtCents } from "@core/format";
import { usePoll } from "@core/usePoll";
import { useSession } from "../../../src/session";
import { colors } from "../../../src/theme";

/** My filled trades in this room, newest first. */
export default function TradesScreen() {
  const { id: roomId } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { t } = useSession();
  const { data: state } = usePoll(() => api.get<RoomState>(`/api/rooms/${roomId}`), 30_000, [roomId]);
  const { data: tradesRes } = usePoll(
    () => api.get<{ items: Trade[] }>(`/api/rooms/${roomId}/trades`), 30_000, [roomId]);

  const aliasOf = (id: string) => state?.instruments.find(i => i.id === id)?.alias ?? id;
  const trades = [...(tradesRes?.items ?? [])].sort((a, b) => b.day - a.day);

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <View style={styles.topbar}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={8}>
          <Text style={styles.back}>‹</Text>
        </TouchableOpacity>
        <Text style={styles.title}>{t("trades.title")}</Text>
        <View style={{ width: 40 }} />
      </View>
      <FlatList
        data={trades}
        keyExtractor={(tr, i) => `${tr.day}-${tr.instrument_id}-${tr.side}-${i}`}
        contentContainerStyle={styles.list}
        ListEmptyComponent={<Text style={styles.empty}>—</Text>}
        renderItem={({ item: tr }) => (
          <View style={styles.row}>
            <View style={{ flex: 1 }}>
              <Text style={styles.name}>{aliasOf(tr.instrument_id)}</Text>
              <Text style={styles.meta}>{t("common.day", { day: tr.day })}</Text>
            </View>
            <Text style={[styles.side, tr.side === "buy" ? styles.up : styles.down]}>
              {tr.side === "buy" ? t("side.Buy") : t("side.Sell")}
            </Text>
            <View style={styles.right}>
              <Text style={styles.val}>{fmtCents(tr.amount_cents)}</Text>
              <Text style={styles.meta}>{fmt$(tr.price)} × {tr.shares.toFixed(1)}</Text>
            </View>
          </View>
        )}
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
  title: { color: colors.ink, fontSize: 16, fontWeight: "700" },
  list: { padding: 16, paddingBottom: 40 },
  empty: { color: colors.ink3, fontSize: 13 },
  row: {
    flexDirection: "row", alignItems: "center", gap: 10,
    paddingVertical: 11, borderBottomWidth: 1, borderBottomColor: colors.line,
  },
  name: { color: colors.ink, fontSize: 14, fontWeight: "600" },
  meta: { color: colors.ink3, fontSize: 11, marginTop: 1, fontVariant: ["tabular-nums"] },
  side: { fontSize: 13, fontWeight: "600" },
  up: { color: colors.up },
  down: { color: colors.down },
  right: { alignItems: "flex-end" },
  val: { color: colors.ink, fontSize: 14, fontWeight: "600", fontVariant: ["tabular-nums"] },
});
