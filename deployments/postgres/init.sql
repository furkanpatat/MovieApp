-- Idempotent: safe to re-run against an existing database (see `make db-init`).
-- Docker only auto-runs this on a fresh data volume.
CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS interaction;

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
