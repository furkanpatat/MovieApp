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
