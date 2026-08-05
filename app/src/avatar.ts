import type { AvatarID } from "@core/api";

/* Glyph + color per avatar id. Mirrors web/src/avatar.ts glyphs; colors are
   app-local (web uses avif art, RN uses a colored disc). */
export const avatarGlyphs: Record<AvatarID, string> = {
  bull: "🐂", bear: "🐻", fox: "🦊", owl: "🦉",
  shark: "🦈", tiger: "🐯", rocket: "🚀", diamond: "◆",
};

export const avatarColors: Record<AvatarID, string> = {
  bull: "#00c805", bear: "#8a5a2b", fox: "#ff5000", owl: "#9d6cff",
  shark: "#52c7d9", tiger: "#efb84a", rocket: "#ff463d", diamond: "#6ee7ff",
};

export const avatarIDs = Object.keys(avatarGlyphs) as AvatarID[];

export function avatarGlyph(id: AvatarID | undefined, fallback: string): string {
  return id ? avatarGlyphs[id] : fallback.slice(0, 2).toUpperCase();
}
