import React, { useState } from "react";
import { FlatList, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useRouter } from "expo-router";
import { EventItem, ForumItem, RoomState, api, fetchForum } from "@core/api";
import { fmtCents } from "@core/format";
import { pickL } from "@core/i18n";
import { usePoll } from "@core/usePoll";
import { useSession } from "../../../src/session";
import { useIncrementalFeed } from "../../../src/hooks/useIncrementalFeed";
import RoomTabs from "../../../src/components/RoomTabs";
import { colors } from "../../../src/theme";

type FeedRow =
  | { kind: "event"; ev: EventItem }
  | { kind: "forum"; post: ForumItem };

/** Room activity: whale/bust/bankrupt events + NPC forum, one list with a
    segmented toggle (mirrors web RightRail events + forum panes). */
export default function FeedScreen() {
  const { id: roomId } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { t, lang } = useSession();
  const [tab, setTab] = useState<"events" | "forum">("events");
  const { data: state } = usePoll(() => api.get<RoomState>(`/api/rooms/${roomId}`), 5_000, [roomId]);
  const feedPollMs = state?.room.day_duration_secs && state.room.day_duration_secs < 60 ? 5_000 : 30_000;
  const { items: events } = useIncrementalFeed<EventItem, { items: EventItem[] }>(
    after => api.get<{ items: EventItem[] }>(`/api/rooms/${roomId}/events?after=${after}`), feedPollMs, roomId!);
  const { items: forum } = useIncrementalFeed<ForumItem, { items: ForumItem[] }>(
    after => fetchForum(roomId!, after), feedPollMs, roomId!);

  const aliasOf = (id: string) => state?.instruments.find(i => i.id === id)?.alias ?? id;

  const rows: FeedRow[] = tab === "events"
    ? [...events].sort((a, b) => b.id - a.id).map(ev => ({ kind: "event", ev }))
    : [...forum].sort((a, b) => b.id - a.id).map(post => ({ kind: "forum", post }));

  function eventText(ev: EventItem): { txt: string; tone: "whale" | "bust" | "agent" } {
    const p = ev.payload;
    if (ev.kind === "agent_order") {
      return {
        tone: "agent",
        txt: t(p.side === "buy" ? "rail.orderBuy" : "rail.orderSell", { alias: aliasOf(p.instrument_id ?? "") }),
      };
    }
    if (ev.kind === "manipulation_bust") {
      return {
        tone: "bust",
        txt: t("rail.bust", {
          username: p.username ?? "?", amount: fmtCents(p.fine_cents ?? 0),
          alias: aliasOf(p.instrument_id ?? ""),
        }),
      };
    }
    if (ev.kind === "bankrupt") {
      return { tone: "bust", txt: t("rail.bankruptEvent", { username: p.username ?? "?" }) };
    }
    return {
      tone: "whale",
      txt: t("rail.whale", {
        side: t(p.side === "buy" ? "side.buy" : "side.sell"),
        alias: aliasOf(p.instrument_id ?? ""),
      }),
    };
  }

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <View style={styles.topbar}>
        <TouchableOpacity onPress={() => router.replace("/")} hitSlop={8}>
          <Text style={styles.back}>‹</Text>
        </TouchableOpacity>
        <Text style={styles.title}>{t("rail.events")}</Text>
        <View style={{ width: 40 }} />
      </View>
      <RoomTabs roomId={roomId!} active="feed" />

      <View style={styles.seg}>
        {(["events", "forum"] as const).map(k => (
          <TouchableOpacity key={k} style={[styles.segBtn, tab === k && styles.segOn]} onPress={() => setTab(k)}>
            <Text style={[styles.segTxt, tab === k && styles.segTxtOn]}>
              {k === "events" ? t("rail.events") : t("rail.tabForum")}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      <FlatList
        data={rows}
        keyExtractor={r => (r.kind === "event" ? `e${r.ev.id}` : `f${r.post.id}`)}
        contentContainerStyle={styles.list}
        ListEmptyComponent={
          <Text style={styles.empty}>{tab === "events" ? t("rail.noEvents") : t("rail.noForum")}</Text>
        }
        renderItem={({ item }) => {
          if (item.kind === "forum") {
            const p = item.post;
            return (
              <View style={styles.item}>
                <View style={styles.metaRow}>
                  <Text style={styles.npc}>{pickL(lang, p.npc_name, p.npc_name_en)}</Text>
                  {p.is_agent && <Text style={styles.agentBadge}>{t("common.agent")}</Text>}
                  <Text style={styles.meta}> · {t("common.day", { day: p.day })}</Text>
                </View>
                <Text style={styles.bodyTxt}>{pickL(lang, p.body, p.body_en)}</Text>
              </View>
            );
          }
          const { txt, tone } = eventText(item.ev);
          const p = item.ev.payload;
          return (
            <View style={[styles.item, tone === "bust" && styles.itemBust,
              tone === "whale" && styles.itemWhale, tone === "agent" && styles.itemAgent]}>
              <View style={styles.metaRow}>
                {tone === "agent" && (
                  <>
                    <Text style={styles.npc}>{p.is_agent ? pickL(lang, p.username ?? "?", p.username_en) : p.username ?? "?"}</Text>
                    {p.is_agent && <Text style={styles.agentBadge}>{t("common.agent")}</Text>}
                    <Text style={styles.meta}> · </Text>
                  </>
                )}
                <Text style={styles.meta}>{t("rail.tradingDay", { day: item.ev.day })}</Text>
              </View>
              <Text style={styles.bodyStrong}>{txt}</Text>
            </View>
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
  },
  back: { color: colors.up, fontSize: 30, fontWeight: "300", width: 40 },
  title: { color: colors.ink, fontSize: 16, fontWeight: "700" },
  seg: {
    flexDirection: "row", gap: 4, backgroundColor: colors.card2, borderRadius: 8,
    padding: 3, marginHorizontal: 16, marginTop: 10,
  },
  segBtn: { flex: 1, paddingVertical: 5, borderRadius: 6, alignItems: "center" },
  segOn: { backgroundColor: colors.card },
  segTxt: { color: colors.ink2, fontSize: 13 },
  segTxtOn: { color: colors.ink, fontWeight: "600" },
  list: { padding: 16, paddingBottom: 40 },
  empty: { color: colors.ink3, fontSize: 13, paddingVertical: 8 },
  item: { paddingVertical: 9, borderBottomWidth: 1, borderBottomColor: colors.line },
  itemWhale: { borderLeftWidth: 3, borderLeftColor: colors.up, paddingLeft: 10 },
  itemBust: { borderLeftWidth: 3, borderLeftColor: colors.down, paddingLeft: 10 },
  itemAgent: { borderLeftWidth: 3, borderLeftColor: "#8b72e8", paddingLeft: 10 },
  metaRow: { flexDirection: "row", alignItems: "center", flexWrap: "wrap", marginBottom: 2 },
  meta: { color: colors.ink3, fontSize: 11, fontVariant: ["tabular-nums"] },
  npc: { color: colors.up, fontSize: 12, fontWeight: "600" },
  agentBadge: {
    color: colors.up, fontSize: 9, fontWeight: "700", textTransform: "uppercase",
    marginLeft: 5, paddingHorizontal: 5, paddingVertical: 1, borderRadius: 999,
    borderWidth: 1, borderColor: colors.line, overflow: "hidden",
  },
  bodyTxt: { color: colors.ink, fontSize: 13, lineHeight: 19 },
  bodyStrong: { color: colors.ink, fontSize: 13, fontWeight: "600" },
});
