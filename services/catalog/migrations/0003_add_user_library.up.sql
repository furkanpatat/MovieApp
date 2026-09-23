-- Per-user library: the watchlist ("My List") and personal ratings.
-- Lives next to the movies table it references, in its own schema.
--
-- movie_id references the catalog's movies table (the service stores the
-- movie before inserting), so a list can always be rendered from Postgres.
-- Deleting an account deletes its library; deleting a movie that is still in
-- someone's library is refused (movies is a cache, the library is user data).
CREATE SCHEMA IF NOT EXISTS library;

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
