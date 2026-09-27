package domain

import (
	"context"
	"time"
)

// DiscoverFilter narrows TMDB's /discover/movie. The zero value is "popular
// movies with enough votes to be worth showing".
type DiscoverFilter struct {
	GenreID int // TMDB movie genre id; 0 = any
	Page    int
}

// Discoverer browses movies by filter (TMDB /discover/movie).
type Discoverer interface {
	DiscoverMovies(ctx context.Context, f DiscoverFilter) (MoviePage, error)
}

// TVProvider is the source of TV series (TMDB /tv). Results have MediaType "tv".
type TVProvider interface {
	GetPopularTV(ctx context.Context, page int) (MoviePage, error)
	GetTVDetails(ctx context.Context, id int) (Movie, error)
	// DiscoverTV is /discover/tv; GenreID is a TMDB *TV* genre id (they
	// differ from movie genres: "Sci-Fi & Fantasy" is 10765).
	DiscoverTV(ctx context.Context, f DiscoverFilter) (MoviePage, error)
	SearchTV(ctx context.Context, query string, page int) (MoviePage, error)
}

// TVStore is the L2 persistent cache for TV series, apart from movies: the
// ids would collide, and the user library references movies only.
type TVStore interface {
	UpsertTV(ctx context.Context, show Movie) error
	GetTV(ctx context.Context, id int) (Movie, time.Time, error)
}
