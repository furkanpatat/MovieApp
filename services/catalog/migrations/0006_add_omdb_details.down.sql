ALTER TABLE imdb_ratings
    DROP COLUMN IF EXISTS rated,
    DROP COLUMN IF EXISTS rotten_tomatoes,
    DROP COLUMN IF EXISTS metascore,
    DROP COLUMN IF EXISTS awards,
    DROP COLUMN IF EXISTS director,
    DROP COLUMN IF EXISTS writer,
    DROP COLUMN IF EXISTS box_office,
    DROP COLUMN IF EXISTS country,
    DROP COLUMN IF EXISTS language;
