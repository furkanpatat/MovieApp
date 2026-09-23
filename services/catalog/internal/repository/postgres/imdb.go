package postgres

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// omdbColumns are the OMDbDetails columns, in scanDetails order.
const omdbColumns = `rated, rotten_tomatoes, metascore, awards, director, writer, box_office, country, language`

// detailsDest returns scan targets for omdbColumns, and a func that copies
// the scanned (nullable) values into d.
func detailsDest(d *domain.OMDbDetails) ([]any, func()) {
	var rated, rt, awards, director, writer, box, country, lang *string
	var meta *int32
	return []any{&rated, &rt, &meta, &awards, &director, &writer, &box, &country, &lang}, func() {
		d.Rated, d.RottenTomatoes, d.Awards = deref(rated), deref(rt), deref(awards)
		d.Director, d.Writer, d.BoxOffice = deref(director), deref(writer), deref(box)
		d.Country, d.Language = deref(country), deref(lang)
		if meta != nil {
			d.Metascore = int(*meta)
		}
	}
}

func (r *Repository) GetIMDbRating(ctx context.Context, imdbID string) (domain.IMDbRating, error) {
	var rating *float32
	var votes *int32
	var out domain.IMDbRating
	details, fill := detailsDest(&out.OMDbDetails)
	err := r.pool.QueryRow(ctx, `SELECT rating, votes, fetched_at, `+omdbColumns+` FROM imdb_ratings WHERE imdb_id = $1`, imdbID).
		Scan(append([]any{&rating, &votes, &out.FetchedAt}, details...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.IMDbRating{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.IMDbRating{}, err
	}
	if rating != nil {
		out.Rating = roundRating(*rating)
	}
	if votes != nil {
		out.Votes = int(*votes)
	}
	fill()
	return out, nil
}

// SaveIMDbRating stores a freshly fetched rating (fetched_at = now).
// A zero Rating is stored as NULL: IMDb has no rating for the title yet.
func (r *Repository) SaveIMDbRating(ctx context.Context, imdbID string, rt domain.IMDbRating) error {
	var rating *float64
	if rt.Rating > 0 {
		rating = &rt.Rating
	}
	var metascore *int
	if rt.Metascore > 0 {
		metascore = &rt.Metascore
	}
	d := rt.OMDbDetails
	_, err := r.pool.Exec(ctx, `
		INSERT INTO imdb_ratings (imdb_id, rating, votes, `+omdbColumns+`, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		ON CONFLICT (imdb_id) DO UPDATE SET
			rating = EXCLUDED.rating, votes = EXCLUDED.votes,
			rated = EXCLUDED.rated, rotten_tomatoes = EXCLUDED.rotten_tomatoes, metascore = EXCLUDED.metascore,
			awards = EXCLUDED.awards, director = EXCLUDED.director, writer = EXCLUDED.writer,
			box_office = EXCLUDED.box_office, country = EXCLUDED.country, language = EXCLUDED.language,
			fetched_at = NOW()`,
		imdbID, rating, rt.Votes, nullIfEmpty(d.Rated), nullIfEmpty(d.RottenTomatoes), metascore,
		nullIfEmpty(d.Awards), nullIfEmpty(d.Director), nullIfEmpty(d.Writer),
		nullIfEmpty(d.BoxOffice), nullIfEmpty(d.Country), nullIfEmpty(d.Language))
	return err
}

func (r *Repository) IMDbRatingsForMovies(ctx context.Context, movieIDs []int) (map[int]domain.IMDbRating, error) {
	out := make(map[int]domain.IMDbRating, len(movieIDs))
	if len(movieIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, r.rating, r.votes, r.fetched_at
		FROM movies m JOIN imdb_ratings r ON r.imdb_id = m.imdb_id
		WHERE m.id = ANY($1) AND r.rating IS NOT NULL`, movieIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var rating float32
		var votes *int32
		var rt domain.IMDbRating
		if err := rows.Scan(&id, &rating, &votes, &rt.FetchedAt); err != nil {
			return nil, err
		}
		rt.Rating = roundRating(rating)
		if votes != nil {
			rt.Votes = int(*votes)
		}
		out[id] = rt
	}
	return out, rows.Err()
}

// roundRating undoes REAL's float32 noise (8.1 -> 8.100000381).
func roundRating(f float32) float64 { return math.Round(float64(f)*10) / 10 }
