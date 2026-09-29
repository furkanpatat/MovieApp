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
