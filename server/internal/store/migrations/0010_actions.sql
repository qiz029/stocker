-- Player actions (hype / debunk / intel).
--
-- room_news gains:
--   driven_by_user_id — which user planted this item (hype). Server-private:
--     it must NEVER appear in API responses (blind box).
--   disputed — public flag set by the debunk action (辟谣/调查).
--   exposed  — public flag set when a planted item is busted by regulators.
ALTER TABLE room_news
    ADD COLUMN driven_by_user_id BIGINT,
    ADD COLUMN disputed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN exposed BOOLEAN NOT NULL DEFAULT FALSE;

-- One row per paid player action. payload is server-private (it carries the
-- planted shock, verdicts, etc.); only the acting player ever sees a verdict,
-- in the action's own response.
CREATE TABLE player_actions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day INT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('hype', 'debunk', 'intel')),
    payload JSONB,
    fee_cents BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX player_actions_room ON player_actions (room_id, user_id, day, kind);
