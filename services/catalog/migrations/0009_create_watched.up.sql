-- Titles a user has watched: shown on their profile and, by username, on
-- their public profile (/u/{username}). A movie or a series, like the rest
-- of the library (see 0008): one real foreign key per media type.
CREATE TABLE IF NOT EXISTS library.watched (
    user_id    UUID        NOT NULL,
    media_type TEXT        NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv')),
    movie_id   INTEGER     NOT NULL,
    movie_ref  INTEGER     GENERATED ALWAYS AS (CASE WHEN media_type = 'movie' THEN movie_id END) STORED,
    tv_ref     INTEGER     GENERATED ALWAYS AS (CASE WHEN media_type = 'tv' THEN movie_id END) STORED,
    watched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, media_type, movie_id),
    CONSTRAINT watched_user_fk  FOREIGN KEY (user_id)   REFERENCES auth.users (id) ON DELETE CASCADE,
    CONSTRAINT watched_movie_fk FOREIGN KEY (movie_ref) REFERENCES public.movies (id),
    CONSTRAINT watched_tv_fk    FOREIGN KEY (tv_ref)    REFERENCES public.tv_shows (id)
);
CREATE INDEX IF NOT EXISTS watched_user_recent_idx ON library.watched (user_id, watched_at DESC);
CREATE INDEX IF NOT EXISTS watched_movie_idx ON library.watched (movie_ref) WHERE movie_ref IS NOT NULL;
CREATE INDEX IF NOT EXISTS watched_tv_idx ON library.watched (tv_ref) WHERE tv_ref IS NOT NULL;
