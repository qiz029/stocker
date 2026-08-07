-- The server periodically reclaims rooms that were never started. This
-- partial index keeps that maintenance delete scoped to waiting rooms.
CREATE INDEX rooms_lobby_created_at
    ON rooms (created_at)
    WHERE status = 'lobby';
