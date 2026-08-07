-- LLM copy is generated in a rolling window instead of blocking room creation.
-- Each room owns one resumable job per simulation day. Workers claim jobs with
-- SKIP LOCKED, so multiple server replicas can share the queue safely.
CREATE TABLE room_copy_jobs (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    day INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'done', 'failed', 'skipped')),
    attempts INT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (room_id, day)
);
CREATE INDEX room_copy_jobs_ready
    ON room_copy_jobs (available_at, room_id, day)
    WHERE status IN ('pending', 'running');

-- Prompt-only metadata was previously discarded after world generation.
-- Keeping it server-side lets a day-sized background job retain the original
-- recap/persona/cluster semantics without seeing or rewriting future days.
ALTER TABLE room_news
    ADD COLUMN is_recap BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN copy_role TEXT NOT NULL DEFAULT '';
ALTER TABLE room_forum_posts
    ADD COLUMN persona TEXT NOT NULL DEFAULT '';

-- Existing rooms intentionally keep their already-published copy. Only rooms
-- created after this migration enter the rolling queue, avoiding historical
-- rewrites with incomplete prompt metadata.

-- Push banners are part of the localized app surface too. Re-registering a
-- device updates this preference when the user changes UI language.
ALTER TABLE push_tokens
    ADD COLUMN lang TEXT NOT NULL DEFAULT 'en'
        CHECK (lang IN ('en', 'zh'));
