import React from "react";
import { StyleSheet, Text, View } from "react-native";
import type { AvatarID } from "@core/api";
import { avatarColors, avatarGlyph } from "../avatar";

/** Colored disc with the avatar glyph (or username initials as fallback). */
export default function Avatar({ id, username, size = 28 }: {
  id?: AvatarID; username: string; size?: number;
}) {
  return (
    <View style={[styles.disc, {
      width: size, height: size, borderRadius: size / 2,
      backgroundColor: id ? avatarColors[id] : "#252b35",
    }]}>
      <Text style={{ fontSize: size * 0.5 }}>{avatarGlyph(id, username)}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  disc: { alignItems: "center", justifyContent: "center" },
});
