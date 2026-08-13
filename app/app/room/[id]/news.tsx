import React, { useEffect, useState } from "react";
import { FlatList, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useRouter } from "expo-router";
import {
  ApiError, DebunkVerdict, MediaAccuracy, NewsItem, NewsResponse, RoomState,
  api, fetchNews, postDebunk,
} from "@core/api";
import { fmtCents, prettifyHeadline } from "@core/format";
import { MsgKey, TFunc, mediaName, pickL } from "@core/i18n";
import { usePoll } from "@core/usePoll";
import { useSession } from "../../../src/session";
import { useIncrementalFeed } from "../../../src/hooks/useIncrementalFeed";
import { loadDebunkVerdicts, saveDebunkVerdict } from "../../../src/debunkVerdicts";
import RoomTabs from "../../../src/components/RoomTabs";
import { colors } from "../../../src/theme";

const DEBUNK_FEE_CENTS = 200_000;

type NewsGroup = { key: string; items: NewsItem[] };

/** Group the feed into story chains (shared cluster_id), positioned by the
    latest item; standalone items stay singletons. Port of web RightRail. */
function groupNews(items: NewsItem[]): NewsGroup[] {
  const groups: NewsGroup[] = [];
  const chains = new Map<number, NewsGroup>();
  for (const n of [...items].sort((a, b) => b.id - a.id)) {
    if (n.cluster_id == null) {
      groups.push({ key: `n${n.id}`, items: [n] });
      continue;
    }
    let g = chains.get(n.cluster_id);
    if (!g) {
      g = { key: `c${n.cluster_id}`, items: [] };
      chains.set(n.cluster_id, g);
      groups.push(g);
    }
    g.items.push(n);
  }
  for (const g of groups) g.items.sort((a, b) => a.day - b.day || a.id - b.id);
  return groups;
}

function chainRole(index: number, length: number): MsgKey {
  if (index === 0) return "news.chain.rumor";
  if (index === length - 1 && length >= 3) return "news.chain.followup";
  return "news.chain.report";
}

function accuracyText(acc: MediaAccuracy | undefined, mediaID: string, t: TFunc): string | null {
  const s = acc?.[mediaID];
  if (!s || s.reports <= 0) return null;
  if (s.reports < 3) return t("news.accuracyNA");
  return t("news.accuracy", { pct: Math.round((s.hits / s.reports) * 100) });
}

export default function NewsScreen() {
  const { id: roomId } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { t, lang, user } = useSession();
  const { data: state } = usePoll(() => api.get<RoomState>(`/api/rooms/${roomId}`), 5_000, [roomId]);
  const feedPollMs = state?.room.day_duration_secs && state.room.day_duration_secs < 60 ? 5_000 : 30_000;
  const { items, extra } = useIncrementalFeed<NewsItem, NewsResponse>(
    after => fetchNews(roomId!, after), feedPollMs, roomId!);
  const [verdicts, setVerdicts] = useState<Record<number, DebunkVerdict>>({});
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    if (user) void loadDebunkVerdicts(user.id, roomId!).then(setVerdicts);
  }, [roomId, user]);

  const readOnly = state ? state.room.is_member === false : true;
  const accuracy = extra?.media_accuracy;
  const groups = groupNews(items);
  const aliasOf = (id: string) => state?.instruments.find(i => i.id === id)?.alias ?? id;

  async function investigate(newsID: number) {
    if (!user) return;
    setNotice(null);
    try {
      const res = await postDebunk(roomId!, newsID);
      setVerdicts(v => ({ ...v, [newsID]: res.verdict }));
      void saveDebunkVerdict(user.id, roomId!, newsID, res.verdict);
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : t("news.investigateFailed"));
    }
  }

  const renderItem = (n: NewsItem, role?: MsgKey) => {
    const acc = accuracyText(accuracy, n.media_id, t);
    const verdict = verdicts[n.id];
    const headline = prettifyHeadline(pickL(lang, n.headline, n.headline_en), aliasOf);
    return (
      <TouchableOpacity key={n.id} style={[styles.item, role != null && styles.chainItem]}
        onPress={() => router.push(`/room/${roomId}/news/${n.id}`)}>
        <View style={styles.metaRow}>
          {role && (
            <Text style={[styles.chainRole,
              role === "news.chain.rumor" ? styles.roleRumor
                : role === "news.chain.followup" ? styles.roleFollowup : styles.roleReport]}>
              {t(role)}
            </Text>
          )}
          <Text style={styles.meta}>
            {mediaName(n.media_id, t)}{acc ? ` · ${acc}` : ""} · {t("common.day", { day: n.day })}
          </Text>
          {(n.disputed || verdict) && <Text style={styles.badgeDisputed}>{t("news.disputed")}</Text>}
          {n.exposed && <Text style={styles.badgeExposed}>{t("news.exposed")}</Text>}
        </View>
        <Text style={styles.headline}>{headline}</Text>
        <View style={styles.actions}>
          {!readOnly && !n.disputed && !verdict && (
            <TouchableOpacity onPress={() => void investigate(n.id)} hitSlop={6}>
              <Text style={styles.act}>{t("news.investigate", { fee: fmtCents(DEBUNK_FEE_CENTS) })}</Text>
            </TouchableOpacity>
          )}
          {verdict && (
            <Text style={styles.verdict}>
              {t(`news.verdict.${verdict}`)} <Text style={styles.verdictSub}>{t("news.verdictPrivate")}</Text>
            </Text>
          )}
        </View>
      </TouchableOpacity>
    );
  };

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <View style={styles.topbar}>
        <TouchableOpacity onPress={() => router.replace("/")} hitSlop={8}>
          <Text style={styles.back}>‹</Text>
        </TouchableOpacity>
        <Text style={styles.title}>{t("rail.tabNews")}</Text>
        <View style={{ width: 40 }} />
      </View>
      <RoomTabs roomId={roomId!} active="news" />
      <FlatList
        data={groups}
        keyExtractor={g => g.key}
        contentContainerStyle={styles.list}
        ListEmptyComponent={<Text style={styles.empty}>{t("rail.noNews")}</Text>}
        ListHeaderComponent={notice ? <Text style={styles.error}>{notice}</Text> : null}
        renderItem={({ item: g }) => (
          <View style={g.items.length > 1 || g.items[0]!.cluster_id != null ? styles.chain : undefined}>
            {g.items.map((n, i) => renderItem(n, g.items.length > 1 ? chainRole(i, g.items.length) : undefined))}
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
  },
  back: { color: colors.up, fontSize: 30, fontWeight: "300", width: 40 },
  title: { color: colors.ink, fontSize: 16, fontWeight: "700" },
  list: { padding: 16, paddingBottom: 40 },
  empty: { color: colors.ink3, fontSize: 13, paddingVertical: 8 },
  error: { color: colors.down, fontSize: 13, marginBottom: 10 },
  chain: {
    borderBottomWidth: 1, borderBottomColor: colors.line, paddingVertical: 3,
  },
  item: { paddingVertical: 9, borderBottomWidth: 1, borderBottomColor: colors.line },
  chainItem: { borderBottomWidth: 0, borderLeftWidth: 2, borderLeftColor: colors.line, paddingLeft: 12 },
  metaRow: { flexDirection: "row", alignItems: "center", flexWrap: "wrap", gap: 6, marginBottom: 2 },
  meta: { color: colors.ink3, fontSize: 11, fontVariant: ["tabular-nums"] },
  chainRole: { fontSize: 10, fontWeight: "700", paddingHorizontal: 6, paddingVertical: 1, borderRadius: 999, overflow: "hidden" },
  roleRumor: { color: colors.down, backgroundColor: colors.downSoft },
  roleReport: { color: colors.up, backgroundColor: colors.upSoft },
  roleFollowup: { color: colors.ink2, backgroundColor: colors.card2 },
  badgeDisputed: { color: colors.ink2, backgroundColor: colors.card2, fontSize: 10, fontWeight: "700", paddingHorizontal: 6, paddingVertical: 1, borderRadius: 999, overflow: "hidden" },
  badgeExposed: { color: colors.down, backgroundColor: colors.downSoft, fontSize: 10, fontWeight: "700", paddingHorizontal: 6, paddingVertical: 1, borderRadius: 999, overflow: "hidden" },
  headline: { color: colors.ink, fontSize: 14, lineHeight: 20 },
  actions: { flexDirection: "row", alignItems: "center", gap: 12, marginTop: 6, minHeight: 4 },
  act: { color: colors.up, fontSize: 12 },
  verdict: { color: colors.ink, fontSize: 12, fontWeight: "600" },
  verdictSub: { color: colors.ink3, fontWeight: "400", fontSize: 11 },
});
