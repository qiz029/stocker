import React, { useEffect, useState } from "react";
import { ScrollView, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useRouter } from "expo-router";
import {
  ApiError, DebunkVerdict, NewsItem, RoomState, api, fetchNewsItem, postDebunk,
} from "@core/api";
import { fmtCents, prettifyHeadline } from "@core/format";
import { mediaName, pickL } from "@core/i18n";
import { useSession } from "../../../../src/session";
import { loadDebunkVerdicts, saveDebunkVerdict } from "../../../../src/debunkVerdicts";
import { colors } from "../../../../src/theme";

const DEBUNK_FEE_CENTS = 200_000;

export default function NewsDetailScreen() {
  const { id: roomId, newsId } = useLocalSearchParams<{ id: string; newsId: string }>();
  const router = useRouter();
  const { t, lang, user } = useSession();
  const [story, setStory] = useState<NewsItem | null>(null);
  const [room, setRoom] = useState<RoomState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [verdict, setVerdict] = useState<DebunkVerdict | null>(null);

  useEffect(() => {
    let active = true;
    if (user) void loadDebunkVerdicts(user.id, roomId!).then(v => {
      if (active) setVerdict(v[Number(newsId)] ?? null);
    });
    Promise.all([fetchNewsItem(roomId!, newsId!), api.get<RoomState>(`/api/rooms/${roomId}`)])
      .then(([s, r]) => { if (active) { setStory(s); setRoom(r); } })
      .catch((e: unknown) => {
        if (active) setError(e instanceof ApiError ? e.message : t("news.loadFailed"));
      });
    return () => { active = false; };
  }, [newsId, roomId, user, t]);

  async function investigate() {
    if (!story || !user) return;
    setError(null);
    try {
      const res = await postDebunk(roomId!, story.id);
      setVerdict(res.verdict);
      void saveDebunkVerdict(user.id, roomId!, story.id, res.verdict);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("news.investigateFailed"));
    }
  }

  const aliasOf = (id: string) => room?.instruments.find(i => i.id === id)?.alias ?? id;

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <View style={styles.topbar}>
        <TouchableOpacity onPress={() => router.back()} hitSlop={8}>
          <Text style={styles.back}>‹</Text>
        </TouchableOpacity>
        <Text style={styles.topTitle}>{t("news.readFull")}</Text>
        <View style={{ width: 40 }} />
      </View>
      <ScrollView contentContainerStyle={styles.body}>
        {error && <Text style={styles.error}>{error}</Text>}
        {story && room && (
          <View style={styles.card}>
            <View style={styles.metaRow}>
              <Text style={styles.kicker}>
                {mediaName(story.media_id, t)} · {t("common.day", { day: story.day })}
              </Text>
              {(story.disputed || verdict) && <Text style={styles.badgeDisputed}>{t("news.disputed")}</Text>}
              {story.exposed && <Text style={styles.badgeExposed}>{t("news.exposed")}</Text>}
            </View>
            <Text style={styles.h1}>
              {prettifyHeadline(pickL(lang, story.headline, story.headline_en), aliasOf)}
            </Text>
            <Text style={styles.bodyTxt}>
              {pickL(lang, story.body, story.body_en) || t("news.noBody")}
            </Text>
            <View style={styles.actions}>
              {room.room.is_member !== false && !story.disputed && !verdict && (
                <TouchableOpacity onPress={() => void investigate()} hitSlop={6}>
                  <Text style={styles.act}>{t("news.investigate", { fee: fmtCents(DEBUNK_FEE_CENTS) })}</Text>
                </TouchableOpacity>
              )}
              {verdict && (
                <Text style={styles.verdict}>
                  {t(`news.verdict.${verdict}`)} <Text style={styles.verdictSub}>{t("news.verdictPrivate")}</Text>
                </Text>
              )}
            </View>
          </View>
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
  topTitle: { color: colors.ink2, fontSize: 14 },
  body: { padding: 16, paddingBottom: 48 },
  error: { color: colors.down, fontSize: 13, marginBottom: 10 },
  card: {
    backgroundColor: colors.card, borderWidth: 1, borderColor: colors.line,
    borderRadius: 16, padding: 20,
  },
  metaRow: { flexDirection: "row", alignItems: "center", gap: 6, flexWrap: "wrap" },
  kicker: { color: colors.ink3, fontSize: 12, fontVariant: ["tabular-nums"] },
  badgeDisputed: { color: colors.ink2, backgroundColor: colors.card2, fontSize: 10, fontWeight: "700", paddingHorizontal: 6, paddingVertical: 1, borderRadius: 999, overflow: "hidden" },
  badgeExposed: { color: colors.down, backgroundColor: colors.downSoft, fontSize: 10, fontWeight: "700", paddingHorizontal: 6, paddingVertical: 1, borderRadius: 999, overflow: "hidden" },
  h1: { color: colors.ink, fontSize: 22, fontWeight: "700", lineHeight: 28, marginVertical: 14 },
  bodyTxt: { color: colors.ink2, fontSize: 15, lineHeight: 26 },
  actions: { flexDirection: "row", alignItems: "center", gap: 12, marginTop: 20 },
  act: { color: colors.up, fontSize: 13 },
  verdict: { color: colors.ink, fontSize: 13, fontWeight: "600" },
  verdictSub: { color: colors.ink3, fontWeight: "400", fontSize: 11 },
});
