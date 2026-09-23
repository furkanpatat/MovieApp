-- Persist the detail-only fields so an L2 hit returns the same Movie as TMDB.
ALTER TABLE movies
    ADD COLUMN IF NOT EXISTS tagline TEXT,
    ADD COLUMN IF NOT EXISTS runtime INT,
    ADD COLUMN IF NOT EXISTS genres  JSONB;

-- Rows written before this migration lack these fields; mark them stale so the
-- next read re-hydrates them from TMDB (they still serve as a fallback meanwhile).
UPDATE movies SET fetched_at = 'epoch' WHERE genres IS NULL;
