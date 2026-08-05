import React from "react";
import { StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { useRouter } from "expo-router";
import { useSession } from "../session";
import { colors } from "../theme";

export type RoomTab = "market" | "news" | "feed" | "chat";

/** In-room navigation: market / news / activity / chat. Tabs replace (not
    push) so the back button always returns to the hall. */
export default function RoomTabs({ roomId, active }: { roomId: string; active: RoomTab }) {
  const { t } = useSession();
  const router = useRouter();
  const tabs: [RoomTab, string, string][] = [
    ["market", t("tab.market"), `/room/${roomId}`],
    ["news", t("rail.tabNews"), `/room/${roomId}/news`],
    ["feed", t("rail.events"), `/room/${roomId}/feed`],
    ["chat", t("chat.title"), `/room/${roomId}/chat`],
  ];
  return (
    <View style={styles.bar}>
      {tabs.map(([id, label, href]) => (
        <TouchableOpacity key={id} style={styles.tab} onPress={() => {
          if (id !== active) router.replace(href as never);
        }}>
          <Text style={[styles.txt, id === active && styles.on]}>{label}</Text>
        </TouchableOpacity>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    flexDirection: "row", gap: 4, paddingHorizontal: 12, paddingVertical: 6,
    borderBottomWidth: 1, borderBottomColor: colors.line,
  },
  tab: { flex: 1, alignItems: "center", paddingVertical: 7, borderRadius: 8 },
  txt: { color: colors.ink3, fontSize: 13, fontWeight: "600" },
  on: { color: colors.up },
});
