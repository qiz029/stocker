-- Public player aliases are case-insensitively unique. Repair legacy rows
-- before adding the index so existing installations can migrate safely.
DO $$
DECLARE
    rec RECORD;
    base TEXT;
    suffix TEXT;
    candidate TEXT;
    attempt INT;
BEGIN
    FOR rec IN
        WITH ranked AS (
            SELECT u.id, u.display_name,
                row_number() OVER (PARTITION BY lower(u.display_name) ORDER BY u.id) alias_rank,
                EXISTS (
                    SELECT 1 FROM users a
                    WHERE a.is_agent AND (
                        lower(a.agent_name) = lower(u.display_name)
                        OR lower(COALESCE(a.agent_name_en, '')) = lower(u.display_name)
                    )
                ) agent_conflict
            FROM users u
            WHERE NOT u.is_agent
        )
        SELECT id, display_name
        FROM ranked
        WHERE display_name = '' OR alias_rank > 1 OR agent_conflict
        ORDER BY id
    LOOP
        base := COALESCE(NULLIF(rec.display_name, ''), 'Trader');
        attempt := 0;
        LOOP
            suffix := '~' || rec.id::text
                || CASE WHEN attempt = 0 THEN '' ELSE '-' || attempt::text END;
            candidate := left(base, GREATEST(0, 24 - char_length(suffix))) || suffix;
            EXIT WHEN NOT EXISTS (
                SELECT 1 FROM users other
                WHERE other.id <> rec.id AND (
                    (NOT other.is_agent AND other.display_name <> ''
                        AND lower(other.display_name) = lower(candidate))
                    OR (other.is_agent AND (
                        lower(other.agent_name) = lower(candidate)
                        OR lower(COALESCE(other.agent_name_en, '')) = lower(candidate)
                    ))
                )
            );
            attempt := attempt + 1;
        END LOOP;
        UPDATE users SET display_name = candidate WHERE id = rec.id;
    END LOOP;
END $$;

CREATE UNIQUE INDEX users_display_name_unique
    ON users (lower(display_name)) WHERE NOT is_agent AND display_name <> '';

-- Historical human events stored the login username in their display payload.
-- Replace it with the repaired public alias so old rooms no longer expose it.
UPDATE room_events e
SET payload = jsonb_set(e.payload, '{username}', to_jsonb(u.display_name), true)
FROM users u
WHERE NOT u.is_agent
  AND e.kind IN ('bankrupt', 'manipulation_bust')
  AND e.payload->>'username' = u.username
  AND u.display_name <> '';
