import React, { useState } from "react";
import {
  KeyboardAvoidingView, Platform, StyleSheet, Text, TextInput, TouchableOpacity, View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { api, ApiError, User } from "@core/api";
import { useSession } from "../src/session";
import LangToggle from "../src/components/LangToggle";
import { colors } from "../src/theme";

export default function LoginScreen() {
  const { t, setUser } = useSession();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      const u = await api.post<User>(`/api/${mode}`, { username: username.trim(), password });
      setUser(u);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("auth.networkError"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <SafeAreaView style={styles.safe}>
      <View style={styles.tools}><LangToggle /></View>
      <KeyboardAvoidingView behavior={Platform.OS === "ios" ? "padding" : undefined} style={styles.center}>
        <View style={styles.card}>
          <Text style={styles.brand}><Text style={{ color: colors.up }}>●</Text> Stocker</Text>
          <Text style={styles.sub}>{t("auth.sub")}</Text>
          <TextInput style={styles.input} placeholder={t("auth.username")} placeholderTextColor={colors.ink3}
            autoCapitalize="none" autoCorrect={false} value={username} onChangeText={setUsername} />
          <TextInput style={styles.input} placeholder={t("auth.password")} placeholderTextColor={colors.ink3}
            secureTextEntry value={password} onChangeText={setPassword} onSubmitEditing={submit} />
          {error && <Text style={styles.error}>{error}</Text>}
          <TouchableOpacity style={[styles.submit, (busy || !username || !password) && styles.disabled]}
            disabled={busy || !username || !password} onPress={submit}>
            <Text style={styles.submitTxt}>{mode === "login" ? t("auth.login") : t("auth.register")}</Text>
          </TouchableOpacity>
          <TouchableOpacity onPress={() => { setMode(mode === "login" ? "register" : "login"); setError(null); }}>
            <Text style={styles.link}>{mode === "login" ? t("auth.toRegister") : t("auth.toLogin")}</Text>
          </TouchableOpacity>
        </View>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.bg },
  tools: { position: "absolute", top: 60, right: 20, zIndex: 1 },
  center: { flex: 1, justifyContent: "center", padding: 20 },
  card: {
    backgroundColor: colors.card, borderWidth: 1, borderColor: colors.line,
    borderRadius: 16, padding: 28, gap: 10,
  },
  brand: { color: colors.ink, fontSize: 22, fontWeight: "700" },
  sub: { color: colors.ink2, fontSize: 14, marginBottom: 12 },
  input: {
    backgroundColor: colors.card2, borderWidth: 1, borderColor: colors.line,
    borderRadius: 10, paddingHorizontal: 14, paddingVertical: 12, color: colors.ink, fontSize: 15,
  },
  error: { color: colors.down, fontSize: 13 },
  submit: {
    backgroundColor: colors.up, borderRadius: 999, paddingVertical: 13,
    alignItems: "center", marginTop: 4,
  },
  disabled: { opacity: 0.35 },
  submitTxt: { color: "#04140a", fontWeight: "700", fontSize: 15 },
  link: { color: colors.ink2, fontSize: 13, textAlign: "center", marginTop: 12 },
});
