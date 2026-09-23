package domain

import (
	"context"
	"time"
)

// IMDbRating is IMDb's own rating for a title, plus the other details OMDb
// returns with it. Rating is 0 when IMDb has none yet (unreleased or too few
// votes); empty details mean OMDb had nothing ("N/A").
type IMDbRating struct {
	Rating float64
	Votes  int
	OMDbDetails
	FetchedAt time.Time
}

// OMDbDetails are facts TMDB doesn't give us, as OMDb formats them.
type OMDbDetails struct {
	Rated          string `json:"rated,omitempty"`           // age rating: "PG-13"
	RottenTomatoes string `json:"rotten_tomatoes,omitempty"` // Tomatometer: "85%"
	Metascore      int    `json:"metascore,omitempty"`       // Metacritic, 0-100
	Awards         string `json:"awards,omitempty"`
	Director       string `json:"director,omitempty"`
	Writer         string `json:"writer,omitempty"`
	BoxOffice      string `json:"box_office,omitempty"` // US gross: "$389,813,101"
	Country        string `json:"country,omitempty"`
	Language       string `json:"language,omitempty"`
}

// RatingProvider fetches IMDb ratings (OMDb). It returns ErrNotFound for an
// unknown IMDb id and ErrUnavailable when the service cannot answer.
type RatingProvider interface {
	GetIMDbRating(ctx context.Context, imdbID string) (IMDbRating, error)
}

// IMDbStore persists IMDb ratings by IMDb id, so each is fetched rarely.
type IMDbStore interface {
	// GetIMDbRating returns ErrNotFound when the rating was never fetched.
	GetIMDbRating(ctx context.Context, imdbID string) (IMDbRating, error)
	SaveIMDbRating(ctx context.Context, imdbID string, r IMDbRating) error
	// IMDbRatingsForMovies returns the known ratings of the given TMDB movie
	// ids (movies whose IMDb id and rating are both stored), keyed by movie id.
	IMDbRatingsForMovies(ctx context.Context, movieIDs []int) (map[int]IMDbRating, error)
}
