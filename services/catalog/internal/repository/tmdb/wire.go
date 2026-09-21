package tmdb

import "github.com/furkanpatat/movieapp/services/catalog/internal/domain"

// TMDB response shapes, kept private so the domain stays independent of them.

type movieWire struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	Overview    string  `json:"overview"`
	PosterPath  string  `json:"poster_path"`
	ReleaseDate string  `json:"release_date"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
	Tagline     string  `json:"tagline"`
	Runtime     int     `json:"runtime"`
	Genres      []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"genres"`
}

func (w movieWire) toDomain() domain.Movie {
	m := domain.Movie{
		ID: w.ID, Title: w.Title, Overview: w.Overview, PosterPath: w.PosterPath,
		ReleaseDate: w.ReleaseDate, VoteAverage: w.VoteAverage, VoteCount: w.VoteCount,
		Tagline: w.Tagline, Runtime: w.Runtime,
	}
	for _, g := range w.Genres {
		m.Genres = append(m.Genres, domain.Genre{ID: g.ID, Name: g.Name})
	}
	return m
}

type popularResponse struct {
	Page         int         `json:"page"`
	TotalPages   int         `json:"total_pages"`
	TotalResults int         `json:"total_results"`
	Results      []movieWire `json:"results"`
}

func (w popularResponse) toDomain() domain.MoviePage {
	p := domain.MoviePage{Page: w.Page, TotalPages: w.TotalPages, TotalResults: w.TotalResults}
	p.Results = make([]domain.Movie, 0, len(w.Results))
	for _, m := range w.Results {
		p.Results = append(p.Results, m.toDomain())
	}
	return p
}
