-- Drops series from the library (their rows cannot be kept without media_type).
DELETE FROM library.watchlists WHERE media_type = 'tv';
DELETE FROM library.user_ratings WHERE media_type = 'tv';
ALTER TABLE library.watchlists DROP CONSTRAINT IF EXISTS watchlists_pkey, ADD PRIMARY KEY (user_id, movie_id);
ALTER TABLE library.user_ratings DROP CONSTRAINT IF EXISTS user_ratings_pkey, ADD PRIMARY KEY (user_id, movie_id);
ALTER TABLE library.watchlists DROP CONSTRAINT IF EXISTS watchlists_movie_fk, DROP CONSTRAINT IF EXISTS watchlists_tv_fk;
ALTER TABLE library.user_ratings DROP CONSTRAINT IF EXISTS user_ratings_movie_fk, DROP CONSTRAINT IF EXISTS user_ratings_tv_fk;
ALTER TABLE library.watchlists DROP COLUMN IF EXISTS movie_ref, DROP COLUMN IF EXISTS tv_ref, DROP COLUMN IF EXISTS media_type;
ALTER TABLE library.user_ratings DROP COLUMN IF EXISTS movie_ref, DROP COLUMN IF EXISTS tv_ref, DROP COLUMN IF EXISTS media_type;
ALTER TABLE library.watchlists ADD CONSTRAINT watchlists_movie_fk FOREIGN KEY (movie_id) REFERENCES public.movies (id);
ALTER TABLE library.user_ratings ADD CONSTRAINT user_ratings_movie_fk FOREIGN KEY (movie_id) REFERENCES public.movies (id);
