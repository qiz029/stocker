import React from "react";
import { StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { Lang } from "@core/i18n";
import { colors } from "../theme";
import { useSession } from "../session";

/** zh/EN pill toggle, persisted via the session provider. */
export default function LangToggle() {
  const { lang, setLang } = useSession();
  const opt = (l: Lang, label: string) => (
    <TouchableOpacity key={l} onPress={() => setLang(l)}
      style={[styles.btn, lang === l && styles.on]} hitSlop={6}>
      <Text style={[styles.txt, lang === l && styles.txtOn]}>{label}</Text>
    </TouchableOpacity>
  );
  return <View style={styles.wrap}>{opt("zh", "中文")}{opt("en", "EN")}</View>;
}

const styles = StyleSheet.create({
  wrap: {
    flexDirection: "row", gap: 2, padding: 2,
    borderWidth: 1, borderColor: colors.line, borderRadius: 999,
  },
  btn: { paddingHorizontal: 10, paddingVertical: 3, borderRadius: 999 },
  on: { backgroundColor: colors.upSoft },
  txt: { color: colors.ink3, fontSize: 12 },
  txtOn: { color: colors.up },
});
