-- Idempotent: safe to re-run against an existing database (see `make db-init`).
-- Docker only auto-runs this on a fresh data volume.
CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS interaction;
CREATE SCHEMA IF NOT EXISTS library;

-- ---------------------------------------------------------------------------
-- Auth service: user accounts. id (UUID) is the JWT `sub`, so it stays stable
-- even if a user later changes username or email.
-- Username and email are unique case-insensitively (Alice == alice).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS auth.users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT        NOT NULL CHECK (char_length(username) BETWEEN 3 AND 32),
    email         TEXT        NOT NULL CHECK (char_length(email) <= 254),
    password_hash TEXT        NOT NULL,             -- bcrypt; never the password itself
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON auth.users (lower(username));
CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_idx    ON auth.users (lower(email));

-- Refresh tokens (long sessions for API clients such as the mobile app).
-- Only a SHA-256 of each token is stored. A family is one sign-in: each
-- refresh consumes its token (used_at) and adds the next; a consumed token
-- presented again means a stolen copy, and revokes the whole family.
CREATE TABLE IF NOT EXISTS auth.refresh_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES auth.users (id) ON DELETE CASCADE,
    family_id   UUID        NOT NULL,
    token_hash  BYTEA       NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    used_at     TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS refresh_tokens_family_idx ON auth.refresh_tokens (family_id);
CREATE INDEX IF NOT EXISTS refresh_tokens_user_idx   ON auth.refresh_tokens (user_id);

-- ---------------------------------------------------------------------------
-- Interaction service: write-side source of truth (CQRS "command" store).
-- The Redis read model is derived from these tables and can be rebuilt.
-- ---------------------------------------------------------------------------

-- One rating per (title, user); re-rating replaces the score. A title is a
-- movie or a TV series: media_type says which, since TMDB numbers them
-- separately (movie 1399 and tv 1399 are different titles). movie_id is the
-- TMDB id of either (the name predates series).
CREATE TABLE IF NOT EXISTS interaction.ratings (
    media_type  TEXT        NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv')),
    movie_id    INTEGER     NOT NULL,
    user_id     TEXT        NOT NULL,
    score       SMALLINT    NOT NULL CHECK (score BETWEEN 1 AND 10),
    event_id    UUID        NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,           -- when the user acted (event time)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (media_type, movie_id, user_id)
);

-- Running aggregate, updated in the same transaction as the rating so it is
-- O(1) to read. `version` increments on every change and lets the Redis read
-- model reject out-of-order writes.
CREATE TABLE IF NOT EXISTS interaction.movie_rating_stats (
    media_type  TEXT        NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv')),
    movie_id    INTEGER     NOT NULL,
    total_score BIGINT      NOT NULL DEFAULT 0,
    vote_count  BIGINT      NOT NULL DEFAULT 0,
    version     BIGINT      NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (media_type, movie_id)
);

-- event_id is the primary key: redelivered messages are ignored (idempotent).
CREATE TABLE IF NOT EXISTS interaction.comments (
    event_id    UUID        PRIMARY KEY,
    media_type  TEXT        NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv')),
    movie_id    INTEGER     NOT NULL,
    user_id     TEXT        NOT NULL,
    body        TEXT        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Databases created before series: add media_type (existing rows are movies)
-- and widen the keys to (media_type, movie_id, ...). Idempotent.
ALTER TABLE interaction.ratings
    ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv'));
ALTER TABLE interaction.movie_rating_stats
    ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv'));
ALTER TABLE interaction.comments
    ADD COLUMN IF NOT EXISTS media_type TEXT NOT NULL DEFAULT 'movie' CHECK (media_type IN ('movie', 'tv'));
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.key_column_usage
                   WHERE table_schema = 'interaction' AND table_name = 'ratings'
                     AND constraint_name = 'ratings_pkey' AND column_name = 'media_type') THEN
        ALTER TABLE interaction.ratings DROP CONSTRAINT ratings_pkey,
            ADD PRIMARY KEY (media_type, movie_id, user_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.key_column_usage
                   WHERE table_schema = 'interaction' AND table_name = 'movie_rating_stats'
                     AND constraint_name = 'movie_rating_stats_pkey' AND column_name = 'media_type') THEN
        ALTER TABLE interaction.movie_rating_stats DROP CONSTRAINT movie_rating_stats_pkey,
            ADD PRIMARY KEY (media_type, movie_id);
    END IF;
END $$;

DROP INDEX IF EXISTS interaction.comments_movie_recent_idx;
CREATE INDEX IF NOT EXISTS comments_title_recent_idx
    ON interaction.comments (media_type, movie_id, occurred_at DESC);

-- ---------------------------------------------------------------------------
-- Transactional outbox. The HTTP write path only INSERTs here; the relay
-- worker publishes pending rows to RabbitMQ and marks them published.
-- Delivery is at-least-once: consumers are idempotent (see comments/ratings).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS interaction.outbox_events (
    id           UUID        PRIMARY KEY,          -- the event_id inside the payload
    event_type   TEXT        NOT NULL,             -- RatingSubmitted | CommentAdded
    payload      JSONB       NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'published')),
    attempts     INTEGER     NOT NULL DEFAULT 0,   -- failed publish attempts (observability)
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

-- The relay only ever scans pending rows, oldest first.
CREATE INDEX IF NOT EXISTS outbox_pending_idx
    ON interaction.outbox_events (created_at, id) WHERE status = 'pending';
-- Retention cleanup of published rows.
CREATE INDEX IF NOT EXISTS outbox_published_idx
    ON interaction.outbox_events (published_at) WHERE status = 'published';

-- ---------------------------------------------------------------------------
-- Catalog service: persistent (L2) cache of TMDB movies (migrations 0001-0002).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS movies (
    id INT PRIMARY KEY,
    title TEXT NOT NULL,
    overview TEXT NOT NULL,
    poster_path TEXT,
    backdrop_path TEXT,
    trailer_key TEXT,
    release_date TEXT,
    vote_average FLOAT,
    vote_count INT,
    cast_json JSONB,
    tagline TEXT,
    runtime INT,
    genres JSONB,
    fetched_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- Catalog service: TV series (migration 0007).
-- ---------------------------------------------------------------------------
-- L2 cache of TMDB TV series, the counterpart of movies (media_type "tv").
-- A table of its own rather than a media_type column on movies: TMDB numbers
-- movies and series separately (movie 1399 and tv 1399 are different titles),
-- and the user library's foreign keys point at movies only.
-- Dates are TMDB's (first_air_date is what lists call the release date);
-- genres, networks and creators are JSON arrays.
CREATE TABLE IF NOT EXISTS tv_shows (
    id                 INT PRIMARY KEY,
    name               TEXT NOT NULL,
    overview           TEXT NOT NULL,
    poster_path        TEXT,
    backdrop_path      TEXT,
    trailer_key        TEXT,
    first_air_date     TEXT,
    last_air_date      TEXT,
    vote_average       FLOAT,
    vote_count         INT,
    cast_json          JSONB,
    tagline            TEXT,
    episode_runtime    INT,
    genres             JSONB NOT NULL DEFAULT '[]',
    imdb_id            TEXT,
    status             TEXT,
    number_of_seasons  INT,
    number_of_episodes INT,
    networks           JSONB NOT NULL DEFAULT '[]',
    creators           JSONB NOT NULL DEFAULT '[]',
    fetched_at         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- Catalog service: people and IMDb ratings (migrations 0004-0005).
-- ---------------------------------------------------------------------------
-- L2 cache of TMDB people (actors, directors...), the same pattern as movies:
-- Redis in front, TMDB behind, rows re-fetched once older than a week.
-- credits holds the person's movie credits (JSON array), popularity order.
CREATE TABLE IF NOT EXISTS people (
    id                   INT PRIMARY KEY,
    name                 TEXT NOT NULL,
    biography            TEXT NOT NULL DEFAULT '',
    profile_path         TEXT,
    birthday             TEXT,
    deathday             TEXT,
    place_of_birth       TEXT,
    known_for_department TEXT,
    credits              JSONB NOT NULL DEFAULT '[]',
    fetched_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- IMDb ratings (from OMDb), kept apart from TMDB's own vote_average.
-- movies.imdb_id links a TMDB movie to IMDb (TMDB details include it);
-- imdb_ratings is keyed by IMDb id and re-fetched only every few days, to stay
-- far inside OMDb's daily request limit. rating is NULL when IMDb has none.
ALTER TABLE movies ADD COLUMN IF NOT EXISTS imdb_id TEXT;

CREATE TABLE IF NOT EXISTS imdb_ratings (
    imdb_id    TEXT PRIMARY KEY,
    rating     REAL,
    votes      INT,
    fetched_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

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


-- ---------------------------------------------------------------------------
-- Catalog service: each user's library, the watchlist ("My List") and
-- personal ratings (services/catalog/migrations 0003).
-- movie_id references movies (the service stores the movie before inserting).
-- Deleting an account deletes its library; deleting a movie still in someone's
-- library is refused (movies is a cache, the library is user data).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS library.watchlists (
    user_id    UUID        NOT NULL,
    movie_id   INTEGER     NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, movie_id),
    CONSTRAINT watchlists_user_fk  FOREIGN KEY (user_id)  REFERENCES auth.users (id) ON DELETE CASCADE,
    CONSTRAINT watchlists_movie_fk FOREIGN KEY (movie_id) REFERENCES public.movies (id)
);
CREATE INDEX IF NOT EXISTS watchlists_movie_idx ON library.watchlists (movie_id);

-- One rating per (user, movie); re-rating updates it in place.
CREATE TABLE IF NOT EXISTS library.user_ratings (
    user_id    UUID        NOT NULL,
    movie_id   INTEGER     NOT NULL,
    rating     INTEGER     NOT NULL CHECK (rating BETWEEN 1 AND 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, movie_id),
    CONSTRAINT user_ratings_user_fk  FOREIGN KEY (user_id)  REFERENCES auth.users (id) ON DELETE CASCADE,
    CONSTRAINT user_ratings_movie_fk FOREIGN KEY (movie_id) REFERENCES public.movies (id)
);
CREATE INDEX IF NOT EXISTS user_ratings_movie_idx ON library.user_ratings (movie_id);

-- Series in the library (migration 0008).
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

-- Watched titles (migration 0009).
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


