// Package domain holds the Catalog entities and the ports its adapters implement.
package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("movie not found")
	ErrInvalidInput = errors.New("invalid input")
	// ErrUnavailable means the upstream (TMDB) could not serve the request,
	// including when the circuit breaker is open.
	ErrUnavailable = errors.New("upstream unavailable")
)

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Media types. TMDB numbers movies and TV series separately (movie 1399 and
// tv 1399 are different titles), so a title is identified by both.
const (
	MediaMovie = "movie"
	MediaTV    = "tv"
)

// Movie is a title: a movie, or a TV series (MediaType "tv") mapped onto the
// same shape so lists and cards treat both alike (name -> Title,
// first_air_date -> ReleaseDate, episode length -> Runtime).
type Movie struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type,omitempty"` // "movie" (also when empty) or "tv"
	Title        string  `json:"title"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path,omitempty"`
	BackdropPath string  `json:"backdrop_path,omitempty"`
	ReleaseDate  string  `json:"release_date,omitempty"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	// Only populated by GetMovieDetails.
	Tagline    string  `json:"tagline,omitempty"`
	Runtime    int     `json:"runtime,omitempty"`
	Genres     []Genre `json:"genres,omitempty"`
	TrailerKey string  `json:"trailer_key,omitempty"`
	CastJSON   string  `json:"cast_json,omitempty"`
	// IMDb data. IMDbID comes with TMDB details; the rating is IMDb's own
	// (via OMDb) and is absent when unknown, so never confuse it with
	// VoteAverage, which is TMDB's community score.
	IMDbID     string  `json:"imdb_id,omitempty"`
	IMDbRating float64 `json:"imdb_rating,omitempty"`
	IMDbVotes  int     `json:"imdb_votes,omitempty"`
	// Stored with the IMDb rating (movie details only; lists leave it empty).
	OMDbDetails
	// TV series only (details).
	TVDetails
}

// TVDetails are the series-specific facts of a TV details response.
type TVDetails struct {
	NumberOfSeasons  int      `json:"number_of_seasons,omitempty"`
	NumberOfEpisodes int      `json:"number_of_episodes,omitempty"`
	Status           string   `json:"status,omitempty"` // "Returning Series", "Ended", "Canceled"...
	LastAirDate      string   `json:"last_air_date,omitempty"`
	Networks         []string `json:"networks,omitempty"`
	Creators         []string `json:"creators,omitempty"`
}

type MoviePage struct {
	Page         int     `json:"page"`
	TotalPages   int     `json:"total_pages"`
	TotalResults int     `json:"total_results"`
	Results      []Movie `json:"results"`
}

// MovieProvider is the external source of truth (TMDB).
type MovieProvider interface {
	GetPopularMovies(ctx context.Context, page int) (MoviePage, error)
	GetMovieDetails(ctx context.Context, id int) (Movie, error)
	SearchMovies(ctx context.Context, query string, page int) (MoviePage, error)
	GetPerson(ctx context.Context, id int) (Person, error)
}

// MovieStore is the L2 persistent cache for movies.
type MovieStore interface {
	UpsertMovie(ctx context.Context, m Movie) error
	GetMovie(ctx context.Context, id int) (Movie, time.Time, error)
}

// Cache stores JSON-serialisable values. Set keeps a fresh copy for ttl and a
// longer-lived stale copy that GetStale can serve while the upstream is down.
type Cache interface {
	Get(ctx context.Context, key string, dst any) (found bool, err error)
	GetStale(ctx context.Context, key string, dst any) (found bool, err error)
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
}
