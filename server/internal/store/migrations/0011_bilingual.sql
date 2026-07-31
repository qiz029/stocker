-- Bilingual content: English copies alongside the original Chinese text.
-- Empty en columns mean "no English version" (pre-migration rooms);
-- readers fall back to the Chinese field.
ALTER TABLE scenarios ADD COLUMN name_en TEXT NOT NULL DEFAULT '';

ALTER TABLE instruments ADD COLUMN descr_en TEXT NOT NULL DEFAULT '';
ALTER TABLE instruments ADD COLUMN profile_en JSONB; -- {business,bull,bear}

ALTER TABLE room_news ADD COLUMN headline_en TEXT NOT NULL DEFAULT '';
ALTER TABLE room_news ADD COLUMN body_en TEXT NOT NULL DEFAULT '';

ALTER TABLE room_forum_posts ADD COLUMN npc_name_en TEXT NOT NULL DEFAULT '';
ALTER TABLE room_forum_posts ADD COLUMN body_en TEXT NOT NULL DEFAULT '';
