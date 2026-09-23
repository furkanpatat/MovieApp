-- More of what OMDb returns alongside the IMDb rating, stored with it (same
-- refresh cycle) so movie pages serve it from Postgres, never from OMDb.
-- Text as OMDb formats it ("PG-13", "85%", "$389,813,101"); NULL when N/A.
ALTER TABLE imdb_ratings
    ADD COLUMN IF NOT EXISTS rated           TEXT,
    ADD COLUMN IF NOT EXISTS rotten_tomatoes TEXT,
    ADD COLUMN IF NOT EXISTS metascore       INT,
    ADD COLUMN IF NOT EXISTS awards          TEXT,
    ADD COLUMN IF NOT EXISTS director        TEXT,
    ADD COLUMN IF NOT EXISTS writer          TEXT,
    ADD COLUMN IF NOT EXISTS box_office      TEXT,
    ADD COLUMN IF NOT EXISTS country         TEXT,
    ADD COLUMN IF NOT EXISTS language        TEXT;

-- Rows fetched before this migration lack the details: refetch them.
UPDATE imdb_ratings SET fetched_at = 'epoch' WHERE rated IS NULL AND awards IS NULL;
