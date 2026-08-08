import React, { useState } from "react";
import {
  FlatList, KeyboardAvoidingView, Platform, StyleSheet, Text, TextInput, TouchableOpacity, View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useLocalSearchParams, useRouter } from "expo-router";
import { ApiError, ChatMessage, RoomState, api } from "@core/api";
import { pickL } from "@core/i18n";
import { usePoll } from "@core/usePoll";
import { useSession } from "../../../src/session";
import { useIncrementalFeed } from "../../../src/hooks/useIncrementalFeed";
import RoomTabs from "../../../src/components/RoomTabs";
import Avatar from "../../../src/components/Avatar";
import { colors } from "../../../src/theme";

export default function ChatScreen() {
  const { id: roomId } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { t, lang, user } = useSession();
  const { data: state } = usePoll(() => api.get<RoomState>(`/api/rooms/${roomId}`), 30_000, [roomId]);
  const { items, reload } = useIncrementalFeed<ChatMessage, { items: ChatMessage[] }>(
    after => api.get<{ items: ChatMessage[] }>(`/api/rooms/${roomId}/chat?after=${after}`), 30_000, roomId!);
  const [text, setText] = useState("");
  const [err, setErr] = useState<string | null>(null);

  const readOnly = state ? state.room.is_member === false : true;

  async function send() {
    const body = text.trim();
    if (!body) return;
    setErr(null);
    try {
      await api.post(`/api/rooms/${roomId}/chat`, { text: body });
      setText("");
      await reload();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : t("chat.sendFailed"));
    }
  }

  return (
    <SafeAreaView style={styles.safe} edges={["top"]}>
      <View style={styles.topbar}>
        <TouchableOpacity onPress={() => router.replace("/")} hitSlop={8}>
          <Text style={styles.back}>‹</Text>
        </TouchableOpacity>
        <Text style={styles.title}>{t("chat.title")}</Text>
        <View style={{ width: 40 }} />
      </View>
      <RoomTabs roomId={roomId!} active="chat" />

      <KeyboardAvoidingView behavior={Platform.OS === "ios" ? "padding" : undefined} style={{ flex: 1 }}>
        <FlatList
          data={[...items].sort((a, b) => b.id - a.id)}
          inverted
          keyExtractor={m => String(m.id)}
          contentContainerStyle={styles.list}
          renderItem={({ item: m }) => {
            const mine = !m.is_agent && (m.is_me ?? m.username === user?.display_name?.trim());
            return (
              <View style={[styles.msg, mine && styles.msgMe]}>
                <View style={styles.metaRow}>
                  {!m.is_agent && <Avatar id={m.avatar_id} username={m.username} size={18} />}
                  <Text style={[styles.who, mine && { color: colors.up }]}>
                    {m.is_agent ? pickL(lang, m.username, m.username_en) : m.username}
                  </Text>
                  {m.is_agent && <Text style={styles.agentBadge}>{t("common.agent")}</Text>}
                  <Text style={styles.meta}> · {t("common.day", { day: m.day })}</Text>
                </View>
                <View style={[styles.bubble, mine && styles.bubbleMe]}>
                  <Text style={styles.bubbleTxt}>{m.is_agent ? pickL(lang, m.text, m.text_en) : m.text}</Text>
                </View>
              </View>
            );
          }}
        />
        {err && <Text style={styles.error}>{err}</Text>}
        {!readOnly && (
          <View style={styles.inputRow}>
            <TextInput style={styles.input} placeholder={t("chat.placeholder")}
              placeholderTextColor={colors.ink3} value={text} maxLength={500}
              onChangeText={setText} onSubmitEditing={send} returnKeyType="send" />
            <TouchableOpacity style={styles.sendBtn} onPress={send}>
              <Text style={styles.sendTxt}>{t("chat.send")}</Text>
            </TouchableOpacity>
          </View>
        )}
      </KeyboardAvoidingView>
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
  list: { padding: 16, gap: 10 },
  msg: { alignItems: "flex-start" },
  msgMe: { alignItems: "flex-end" },
  metaRow: { flexDirection: "row", alignItems: "center", gap: 4, marginBottom: 2 },
  who: { color: colors.ink2, fontSize: 11, fontWeight: "600" },
  meta: { color: colors.ink3, fontSize: 10, fontVariant: ["tabular-nums"] },
  agentBadge: {
    color: colors.up, fontSize: 9, fontWeight: "700", textTransform: "uppercase",
    paddingHorizontal: 5, paddingVertical: 1, borderRadius: 999,
    borderWidth: 1, borderColor: colors.line, overflow: "hidden",
  },
  bubble: {
    backgroundColor: colors.card2, borderRadius: 12, borderTopLeftRadius: 4,
    paddingHorizontal: 12, paddingVertical: 7, maxWidth: "92%",
  },
  bubbleMe: { backgroundColor: "rgba(0, 200, 5, 0.14)", borderTopLeftRadius: 12, borderTopRightRadius: 4 },
  bubbleTxt: { color: colors.ink, fontSize: 13, lineHeight: 19 },
  error: { color: colors.down, fontSize: 12, paddingHorizontal: 16, paddingBottom: 4 },
  inputRow: {
    flexDirection: "row", gap: 8, padding: 12,
    borderTopWidth: 1, borderTopColor: colors.line,
  },
  input: {
    flex: 1, backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 999, paddingHorizontal: 14, paddingVertical: 8, color: colors.ink, fontSize: 13,
  },
  sendBtn: {
    backgroundColor: colors.up, borderRadius: 999, paddingHorizontal: 16, justifyContent: "center",
  },
  sendTxt: { color: "#04140a", fontSize: 13, fontWeight: "700" },
});
