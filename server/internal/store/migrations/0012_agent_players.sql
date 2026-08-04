-- Five built-in competitors participate in every room. Their login names
-- are deliberately reserved and their password hashes are invalid bcrypt,
-- so they cannot authenticate through the public API.
ALTER TABLE users
    ADD COLUMN is_agent BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN agent_slot INT,
    ADD COLUMN agent_name TEXT,
    ADD CONSTRAINT users_agent_slot_check CHECK (
        (is_agent AND agent_slot BETWEEN 1 AND 5 AND agent_name <> '')
        OR (NOT is_agent AND agent_slot IS NULL AND agent_name IS NULL)
    );

CREATE UNIQUE INDEX users_agent_slot_unique
    ON users (agent_slot) WHERE is_agent;

INSERT INTO users (username, password_hash, is_agent, agent_slot, agent_name) VALUES
    ('__stocker_agent_nova_019fca57', '!', TRUE, 1, 'Nova'),
    ('__stocker_agent_atlas_019fca57', '!', TRUE, 2, 'Atlas'),
    ('__stocker_agent_echo_019fca57', '!', TRUE, 3, 'Echo'),
    ('__stocker_agent_pixel_019fca57', '!', TRUE, 4, 'Pixel'),
    ('__stocker_agent_sage_019fca57', '!', TRUE, 5, 'Sage')
ON CONFLICT DO NOTHING;

-- Backfill rooms that predate this migration. New rooms insert the same
-- seats in CreateRoom.
INSERT INTO room_players (room_id, user_id, cash_cents, joined_day)
SELECT r.id, u.id, 10000000, 0
FROM rooms r CROSS JOIN users u
WHERE u.is_agent
ON CONFLICT (room_id, user_id) DO NOTHING;

-- The unique key is the idempotency boundary for the background decision
-- loop: an agent can make at most one trading decision per sim day.
CREATE TABLE agent_turns (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day INT NOT NULL,
    order_id BIGINT REFERENCES orders(id) ON DELETE SET NULL,
    instrument_id TEXT,
    side TEXT CHECK (side IN ('buy', 'sell')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, user_id, day)
);
