-- Public share links: every room gets an unguessable read-only token used by
-- the /share/{token} battle-report page. Backfill existing rooms.
ALTER TABLE rooms ADD COLUMN share_token TEXT;
UPDATE rooms SET share_token = lower(substr(md5(random()::text || id::text || clock_timestamp()::text), 1, 13))
  WHERE share_token IS NULL;
ALTER TABLE rooms ALTER COLUMN share_token SET NOT NULL;
CREATE UNIQUE INDEX rooms_share_token_idx ON rooms (share_token);
