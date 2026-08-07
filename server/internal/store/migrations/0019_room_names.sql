ALTER TABLE rooms
    ADD COLUMN name TEXT NOT NULL DEFAULT '';

-- Preserve a useful label for rooms created before names were configurable.
UPDATE rooms SET name = 'Room #' || id WHERE name = '';
