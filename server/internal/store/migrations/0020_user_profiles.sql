ALTER TABLE users
    ADD COLUMN email TEXT NOT NULL DEFAULT '',
    ADD COLUMN description TEXT NOT NULL DEFAULT '',
    ADD COLUMN social_links JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE UNIQUE INDEX users_email_unique
    ON users (lower(email)) WHERE email <> '';
