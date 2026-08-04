ALTER TABLE users
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN avatar_id TEXT NOT NULL DEFAULT '';

ALTER TABLE rooms
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'private'
        CHECK (visibility IN ('public', 'private'));

CREATE INDEX rooms_public_recent
    ON rooms (created_at DESC) WHERE visibility = 'public';
