-- NPC forum posts, pre-generated per room like room_news. Player-visible
-- in full (id/day/npc_name/body only); there is nothing server-private
-- on this table.
CREATE TABLE room_forum_posts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    day INT NOT NULL,
    npc_name TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX room_forum_posts_room ON room_forum_posts (room_id, day, id);
