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

-- ---------------------------------------------------------------------------
-- Interaction service: write-side source of truth (CQRS "command" store).
-- The Redis read model is derived from these tables and can be rebuilt.
-- ---------------------------------------------------------------------------

-- One rating per (movie, user); re-rating replaces the score.
CREATE TABLE IF NOT EXISTS interaction.ratings (
    movie_id    INTEGER     NOT NULL,
    user_id     TEXT        NOT NULL,
    score       SMALLINT    NOT NULL CHECK (score BETWEEN 1 AND 10),
    event_id    UUID        NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,           -- when the user acted (event time)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (movie_id, user_id)
);

-- Running aggregate, updated in the same transaction as the rating so it is
-- O(1) to read. `version` increments on every change and lets the Redis read
-- model reject out-of-order writes.
CREATE TABLE IF NOT EXISTS interaction.movie_rating_stats (
    movie_id    INTEGER     PRIMARY KEY,
    total_score BIGINT      NOT NULL DEFAULT 0,
    vote_count  BIGINT      NOT NULL DEFAULT 0,
    version     BIGINT      NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- event_id is the primary key: redelivered messages are ignored (idempotent).
CREATE TABLE IF NOT EXISTS interaction.comments (
    event_id    UUID        PRIMARY KEY,
    movie_id    INTEGER     NOT NULL,
    user_id     TEXT        NOT NULL,
    body        TEXT        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS comments_movie_recent_idx
    ON interaction.comments (movie_id, occurred_at DESC);

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
