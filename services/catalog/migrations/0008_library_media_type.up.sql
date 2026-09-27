-- The library holds TV series too (media_type 'tv'). TMDB numbers movies and
-- series separately, so a library row is (user, media_type, movie_id), where
-- movie_id is the TMDB id of either (the name predates series).
-- Each media type keeps a real foreign key, on a generated column that is
-- NULL for the other type: movie_ref -> movies, tv_ref -> tv_shows. So a
-- list can always be rendered from Postgres, and a cached title that is on
-- someone's list cannot be deleted. Existing rows are movies. Idempotent.
ALTER TABLE library.watchlists
    ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv'));
ALTER TABLE library.user_ratings
    ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv'));
ALTER TABLE library.watchlists
    ADD COLUMN IF NOT EXISTS movie_ref INTEGER GENERATED ALWAYS AS (CASE WHEN media_type = 'movie' THEN movie_id END) STORED,
    ADD COLUMN IF NOT EXISTS tv_ref    INTEGER GENERATED ALWAYS AS (CASE WHEN media_type = 'tv' THEN movie_id END) STORED;
ALTER TABLE library.user_ratings
    ADD COLUMN IF NOT EXISTS movie_ref INTEGER GENERATED ALWAYS AS (CASE WHEN media_type = 'movie' THEN movie_id END) STORED,
    ADD COLUMN IF NOT EXISTS tv_ref    INTEGER GENERATED ALWAYS AS (CASE WHEN media_type = 'tv' THEN movie_id END) STORED;

DO $$
DECLARE
    tbl TEXT;
    pfx TEXT;
BEGIN
    FOREACH tbl IN ARRAY ARRAY['watchlists', 'user_ratings'] LOOP
        pfx := tbl;
        -- The movie foreign key moves from movie_id to movie_ref.
        IF EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conrelid = ('library.' || tbl)::regclass AND conname = pfx || '_movie_fk'
                     AND pg_get_constraintdef(oid) LIKE 'FOREIGN KEY (movie_id)%') THEN
            EXECUTE format('ALTER TABLE library.%I DROP CONSTRAINT %I', tbl, pfx || '_movie_fk');
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_constraint
                       WHERE conrelid = ('library.' || tbl)::regclass AND conname = pfx || '_movie_fk') THEN
            EXECUTE format('ALTER TABLE library.%I ADD CONSTRAINT %I FOREIGN KEY (movie_ref) REFERENCES public.movies (id)',
                           tbl, pfx || '_movie_fk');
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_constraint
                       WHERE conrelid = ('library.' || tbl)::regclass AND conname = pfx || '_tv_fk') THEN
            EXECUTE format('ALTER TABLE library.%I ADD CONSTRAINT %I FOREIGN KEY (tv_ref) REFERENCES public.tv_shows (id)',
                           tbl, pfx || '_tv_fk');
        END IF;
        -- The key includes the media type.
        IF NOT EXISTS (SELECT 1 FROM information_schema.key_column_usage
                       WHERE table_schema = 'library' AND table_name = tbl
                         AND constraint_name = tbl || '_pkey' AND column_name = 'media_type') THEN
            EXECUTE format('ALTER TABLE library.%I DROP CONSTRAINT %I, ADD PRIMARY KEY (user_id, media_type, movie_id)',
                           tbl, tbl || '_pkey');
        END IF;
    END LOOP;
END $$;

CREATE INDEX IF NOT EXISTS watchlists_tv_idx ON library.watchlists (tv_ref) WHERE tv_ref IS NOT NULL;
CREATE INDEX IF NOT EXISTS user_ratings_tv_idx ON library.user_ratings (tv_ref) WHERE tv_ref IS NOT NULL;
