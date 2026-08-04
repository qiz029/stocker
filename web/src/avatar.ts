import type { AvatarID } from "./api";

export const avatarGlyphs: Record<AvatarID, string> = {
  bull: "🐂", bear: "🐻", fox: "🦊", owl: "🦉",
  shark: "🦈", tiger: "🐯", rocket: "🚀", diamond: "◆",
};

export const avatarIDs = Object.keys(avatarGlyphs) as AvatarID[];

export function avatarGlyph(id: AvatarID | undefined, fallback: string): string {
  return id ? avatarGlyphs[id] : fallback.slice(0, 2).toUpperCase();
}
