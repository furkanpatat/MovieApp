package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

var _ domain.TVStore = (*Repository)(nil)

// UpsertTV stores a TV series (details) in tv_shows.
func (r *Repository) UpsertTV(ctx context.Context, s domain.Movie) error {
	query := `
		INSERT INTO tv_shows (
			id, name, overview, poster_path, backdrop_path, trailer_key,
			first_air_date, last_air_date, vote_average, vote_count, cast_json, tagline,
			episode_runtime, genres, imdb_id, status, number_of_seasons, number_of_episodes,
			networks, creators, fetched_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, overview = EXCLUDED.overview,
			poster_path = EXCLUDED.poster_path, backdrop_path = EXCLUDED.backdrop_path,
			trailer_key = EXCLUDED.trailer_key, first_air_date = EXCLUDED.first_air_date,
			last_air_date = EXCLUDED.last_air_date, vote_average = EXCLUDED.vote_average,
			vote_count = EXCLUDED.vote_count, cast_json = EXCLUDED.cast_json, tagline = EXCLUDED.tagline,
			episode_runtime = EXCLUDED.episode_runtime, genres = EXCLUDED.genres, imdb_id = EXCLUDED.imdb_id,
			status = EXCLUDED.status, number_of_seasons = EXCLUDED.number_of_seasons,
			number_of_episodes = EXCLUDED.number_of_episodes, networks = EXCLUDED.networks,
			creators = EXCLUDED.creators, fetched_at = NOW()
	`
	genres, err := jsonArray(s.Genres)
	if err != nil {
		return fmt.Errorf("encode genres: %w", err)
	}
	networks, err := jsonArray(s.Networks)
	if err != nil {
		return fmt.Errorf("encode networks: %w", err)
	}
	creators, err := jsonArray(s.Creators)
	if err != nil {
		return fmt.Errorf("encode creators: %w", err)
	}
	_, err = r.pool.Exec(ctx, query,
		s.ID, s.Title, s.Overview, s.PosterPath, s.BackdropPath, s.TrailerKey,
		s.ReleaseDate, s.LastAirDate, s.VoteAverage, s.VoteCount, nullIfEmpty(s.CastJSON), s.Tagline,
		s.Runtime, genres, nullIfEmpty(s.IMDbID), s.Status, s.NumberOfSeasons, s.NumberOfEpisodes,
		networks, creators,
	)
	return err
}

// GetTV returns a TV series and when it was fetched, with its IMDb rating
// when one is stored.
func (r *Repository) GetTV(ctx context.Context, id int) (domain.Movie, time.Time, error) {
	query := `
		SELECT
			t.id, t.name, t.overview, t.poster_path, t.backdrop_path, t.trailer_key,
			t.first_air_date, t.last_air_date, t.vote_average, t.vote_count, t.cast_json, t.tagline,
			t.episode_runtime, t.genres, t.imdb_id, t.status, t.number_of_seasons, t.number_of_episodes,
			t.networks, t.creators, t.fetched_at, r.rating, r.votes,
			r.rated, r.rotten_tomatoes, r.metascore, r.awards, r.director, r.writer,
			r.box_office, r.country, r.language
		FROM tv_shows t LEFT JOIN imdb_ratings r ON r.imdb_id = t.imdb_id
		WHERE t.id = $1
	`
	s := domain.Movie{MediaType: domain.MediaTV}
	var fetchedAt time.Time
	var poster, backdrop, trailer, firstAir, lastAir, castJSON, tagline, imdbID, status *string
	var runtime, seasons, episodes, imdbVotes *int32
	var voteAverage *float64
	var voteCount *int32
	var imdbRating *float32
	var genres, networks, creators []byte

	details, fillDetails := detailsDest(&s.OMDbDetails)
	err := r.pool.QueryRow(ctx, query, id).Scan(append([]any{
		&s.ID, &s.Title, &s.Overview, &poster, &backdrop, &trailer,
		&firstAir, &lastAir, &voteAverage, &voteCount, &castJSON, &tagline,
		&runtime, &genres, &imdbID, &status, &seasons, &episodes,
		&networks, &creators, &fetchedAt, &imdbRating, &imdbVotes,
	}, details...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Movie{}, time.Time{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Movie{}, time.Time{}, err
	}

	s.PosterPath, s.BackdropPath, s.TrailerKey = deref(poster), deref(backdrop), deref(trailer)
	s.ReleaseDate, s.LastAirDate, s.CastJSON = deref(firstAir), deref(lastAir), deref(castJSON)
	s.Tagline, s.IMDbID, s.Status = deref(tagline), deref(imdbID), deref(status)
	s.Runtime, s.NumberOfSeasons, s.NumberOfEpisodes = derefInt(runtime), derefInt(seasons), derefInt(episodes)
	s.VoteCount, s.IMDbVotes = derefInt(voteCount), derefInt(imdbVotes)
	if voteAverage != nil {
		s.VoteAverage = *voteAverage
	}
	if imdbRating != nil {
		s.IMDbRating = roundRating(*imdbRating)
	}
	fillDetails()
	for _, f := range []struct {
		raw []byte
		dst any
	}{{genres, &s.Genres}, {networks, &s.Networks}, {creators, &s.Creators}} {
		if err := json.Unmarshal(f.raw, f.dst); err != nil {
			return domain.Movie{}, time.Time{}, fmt.Errorf("decode tv show %d: %w", id, err)
		}
	}
	// Empty arrays read back as nil, like a TMDB response (omitempty).
	if len(s.Genres) == 0 {
		s.Genres = nil
	}
	if len(s.Networks) == 0 {
		s.Networks = nil
	}
	if len(s.Creators) == 0 {
		s.Creators = nil
	}
	return s, fetchedAt, nil
}

// jsonArray encodes a slice as a JSON array ("[]" for nil, never null).
func jsonArray[T any](v []T) (string, error) {
	if v == nil {
		v = []T{}
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func derefInt(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}
