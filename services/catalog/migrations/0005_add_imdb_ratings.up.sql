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
