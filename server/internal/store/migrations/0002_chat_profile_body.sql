CREATE TABLE room_chat (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    day INT NOT NULL,
    text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX room_chat_room ON room_chat (room_id, id);

-- Display-only rich profile {business, bull, bear}; filled by seedscenario
-- (synthetic) and by plan 4's data pipeline (real scenarios).
ALTER TABLE instruments ADD COLUMN profile JSONB;

-- News body copy; empty until plan 4's LLM batch generation fills it
-- (template fallback keeps headline-only news working).
ALTER TABLE room_news ADD COLUMN body TEXT NOT NULL DEFAULT '';
