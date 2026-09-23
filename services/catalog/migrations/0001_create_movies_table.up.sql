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
    fetched_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
