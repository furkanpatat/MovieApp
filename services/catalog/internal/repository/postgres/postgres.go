package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// UpsertMovie inserts or updates a movie in the database.
func (r *Repository) UpsertMovie(ctx context.Context, m domain.Movie) error {
	query := `
		INSERT INTO movies (
			id, title, overview, poster_path, backdrop_path,
			trailer_key, release_date, vote_average, vote_count, cast_json,
			tagline, runtime, genres, imdb_id, fetched_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW()
		) ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			overview = EXCLUDED.overview,
			poster_path = EXCLUDED.poster_path,
			backdrop_path = EXCLUDED.backdrop_path,
			trailer_key = EXCLUDED.trailer_key,
			release_date = EXCLUDED.release_date,
			vote_average = EXCLUDED.vote_average,
			vote_count = EXCLUDED.vote_count,
			cast_json = EXCLUDED.cast_json,
			tagline = EXCLUDED.tagline,
			runtime = EXCLUDED.runtime,
			genres = EXCLUDED.genres,
			imdb_id = EXCLUDED.imdb_id,
			fetched_at = NOW()
	`

	// Always store an array (never SQL NULL) so a hydrated row is
	// distinguishable from one written before genres were persisted.
	genres := m.Genres
	if genres == nil {
		genres = []domain.Genre{}
	}
	genresJSON, err := json.Marshal(genres)
	if err != nil {
		return fmt.Errorf("encode genres: %w", err)
	}

	_, err = r.pool.Exec(ctx, query,
		m.ID, m.Title, m.Overview, m.PosterPath, m.BackdropPath,
		m.TrailerKey, m.ReleaseDate, m.VoteAverage, m.VoteCount, nullIfEmpty(m.CastJSON),
		m.Tagline, m.Runtime, string(genresJSON), nullIfEmpty(m.IMDbID),
	)
	return err
}

// GetMovie returns a movie and its fetched_at timestamp from the database,
// with its IMDb rating when one is stored.
func (r *Repository) GetMovie(ctx context.Context, id int) (domain.Movie, time.Time, error) {
	query := `
		SELECT
			m.id, m.title, m.overview, m.poster_path, m.backdrop_path,
			m.trailer_key, m.release_date, m.vote_average, m.vote_count, m.cast_json,
			m.tagline, m.runtime, m.genres, m.imdb_id, r.rating, r.votes, m.fetched_at,
			r.rated, r.rotten_tomatoes, r.metascore, r.awards, r.director, r.writer,
			r.box_office, r.country, r.language
		FROM movies m LEFT JOIN imdb_ratings r ON r.imdb_id = m.imdb_id
		WHERE m.id = $1
	`

	var m domain.Movie
	var fetchedAt time.Time
	var castJSON, trailerKey, posterPath, backdropPath, releaseDate, tagline, imdbID *string
	var runtime, imdbVotes *int32
	var imdbRating *float32
	var genresJSON []byte

	details, fillDetails := detailsDest(&m.OMDbDetails)
	err := r.pool.QueryRow(ctx, query, id).Scan(append([]any{
		&m.ID, &m.Title, &m.Overview, &posterPath, &backdropPath,
		&trailerKey, &releaseDate, &m.VoteAverage, &m.VoteCount, &castJSON,
		&tagline, &runtime, &genresJSON, &imdbID, &imdbRating, &imdbVotes, &fetchedAt,
	}, details...)...)

	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.Movie{}, time.Time{}, domain.ErrNotFound
		}
		return domain.Movie{}, time.Time{}, err
	}

	if posterPath != nil {
		m.PosterPath = *posterPath
	}
	if backdropPath != nil {
		m.BackdropPath = *backdropPath
	}
	if trailerKey != nil {
		m.TrailerKey = *trailerKey
	}
	if releaseDate != nil {
		m.ReleaseDate = *releaseDate
	}
	if castJSON != nil {
		m.CastJSON = *castJSON
	}
	if tagline != nil {
		m.Tagline = *tagline
	}
	if runtime != nil {
		m.Runtime = int(*runtime)
	}
	m.IMDbID = deref(imdbID)
	fillDetails()
	if imdbRating != nil {
		m.IMDbRating = roundRating(*imdbRating)
	}
	if imdbVotes != nil {
		m.IMDbVotes = int(*imdbVotes)
	}
	if len(genresJSON) > 0 {
		if err := json.Unmarshal(genresJSON, &m.Genres); err != nil {
			return domain.Movie{}, time.Time{}, fmt.Errorf("decode genres for movie %d: %w", id, err)
		}
		if len(m.Genres) == 0 {
			m.Genres = nil // keep omitempty behaviour identical to a TMDB response
		}
	}

	return m, fetchedAt, nil
}

// nullIfEmpty maps "" to SQL NULL; an empty string is not valid JSONB.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
