import React, { useState } from "react";
import { StyleSheet, Text, TextInput, TouchableOpacity, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { api, ApiError, AvatarID, User } from "@core/api";
import { useSession } from "../src/session";
import Avatar from "../src/components/Avatar";
import LangToggle from "../src/components/LangToggle";
import { avatarIDs } from "../src/avatar";
import { colors } from "../src/theme";

/** Forced profile setup: users with profile_complete === false land here
    (the server rejects room create/join until a display name + avatar exist). */
export default function ProfileScreen() {
  const { t, user, setUser } = useSession();
  const [displayName, setDisplayName] = useState(user?.display_name ?? "");
  const [avatarID, setAvatarID] = useState<AvatarID>(user?.avatar_id ?? "bull");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      const updated = await api.put<User>("/api/me/profile", {
        display_name: displayName.trim(), avatar_id: avatarID,
      });
      setUser(updated);
    } catch (e) {
      setError(e instanceof ApiError && e.message === "alias already in use"
        ? t("profile.aliasTaken")
        : e instanceof ApiError ? e.message : t("auth.networkError"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.tools}><LangToggle /></View>
      <View style={styles.center}>
        <View style={styles.card}>
          <Text style={styles.title}>{t("profile.title")}</Text>
          <Text style={styles.sub}>{t("profile.sub")}</Text>

          <Text style={styles.label}>{t("profile.displayName")}</Text>
          <TextInput style={styles.input} value={displayName} onChangeText={setDisplayName}
            maxLength={24} autoFocus placeholderTextColor={colors.ink3} />

          <Text style={styles.label}>{t("profile.avatar")}</Text>
          <View style={styles.avatars}>
            {avatarIDs.map(id => (
              <TouchableOpacity key={id} onPress={() => setAvatarID(id)}
                style={[styles.avatarOpt, avatarID === id && styles.avatarOn]}>
                <Avatar id={id} username={displayName || "?"} size={36} />
              </TouchableOpacity>
            ))}
          </View>

          {error && <Text style={styles.error}>{error}</Text>}
          <TouchableOpacity style={[styles.submit, (busy || displayName.trim().length < 2) && styles.disabled]}
            disabled={busy || displayName.trim().length < 2} onPress={save}>
            <Text style={styles.submitTxt}>{busy ? "…" : t("profile.save")}</Text>
          </TouchableOpacity>
        </View>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.bg },
  tools: { position: "absolute", top: 60, right: 20, zIndex: 1 },
  center: { flex: 1, justifyContent: "center", padding: 20 },
  card: {
    backgroundColor: colors.card, borderWidth: 1, borderColor: colors.line,
    borderRadius: 16, padding: 24, gap: 8,
  },
  title: { color: colors.ink, fontSize: 20, fontWeight: "700" },
  sub: { color: colors.ink2, fontSize: 13, marginBottom: 10 },
  label: { color: colors.ink2, fontSize: 12, marginTop: 6 },
  input: {
    backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 10, paddingHorizontal: 14, paddingVertical: 11, color: colors.ink, fontSize: 15,
  },
  avatars: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 2 },
  avatarOpt: {
    padding: 6, borderRadius: 999, borderWidth: 2, borderColor: "transparent",
  },
  avatarOn: { borderColor: colors.up },
  error: { color: colors.down, fontSize: 13 },
  submit: {
    backgroundColor: colors.up, borderRadius: 999, paddingVertical: 13,
    alignItems: "center", marginTop: 10,
  },
  disabled: { opacity: 0.35 },
  submitTxt: { color: "#04140a", fontWeight: "700", fontSize: 15 },
});
