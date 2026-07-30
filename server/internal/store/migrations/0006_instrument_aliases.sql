-- Per-instrument candidate aliases for the per-room blind-box name pick.
-- NULL means "no candidates recorded": readers fall back to the alias column.
ALTER TABLE instruments ADD COLUMN aliases JSONB;
