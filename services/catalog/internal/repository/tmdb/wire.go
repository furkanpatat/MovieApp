package tmdb

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// TMDB response shapes, kept private so the domain stays independent of them.

type movieWire struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	ReleaseDate  string  `json:"release_date"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	Tagline      string  `json:"tagline"`
	Runtime      int     `json:"runtime"`
	IMDbID       string  `json:"imdb_id"`
	Genres       []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"genres"`
	Videos struct {
		Results []struct {
			Key  string `json:"key"`
			Type string `json:"type"`
			Site string `json:"site"`
		} `json:"results"`
	} `json:"videos"`
	Credits struct {
		Cast []any `json:"cast"`
	} `json:"credits"`
}

func (w movieWire) toDomain() domain.Movie {
	m := domain.Movie{
		ID: w.ID, Title: w.Title, Overview: w.Overview, PosterPath: w.PosterPath,
		BackdropPath: w.BackdropPath,
		ReleaseDate:  w.ReleaseDate, VoteAverage: w.VoteAverage, VoteCount: w.VoteCount,
		Tagline: w.Tagline, Runtime: w.Runtime, IMDbID: w.IMDbID,
	}
	for _, g := range w.Genres {
		m.Genres = append(m.Genres, domain.Genre{ID: g.ID, Name: g.Name})
	}

	// Find YouTube trailer
	for _, v := range w.Videos.Results {
		if v.Site == "YouTube" && v.Type == "Trailer" {
			m.TrailerKey = v.Key
			break
		}
	}

	// Encode cast
	if len(w.Credits.Cast) > 0 {
		b, _ := json.Marshal(w.Credits.Cast)
		m.CastJSON = string(b)
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

// maxCredits caps a filmography: enough for a rich row, small enough to cache.
const maxCredits = 60

type creditWire struct {
	movieWire
	MediaType  string  `json:"media_type"`
	Popularity float64 `json:"popularity"`
	Character  string  `json:"character"`
	Job        string  `json:"job"`
}

type personWire struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Biography          string `json:"biography"`
	ProfilePath        string `json:"profile_path"`
	Birthday           string `json:"birthday"`
	Deathday           string `json:"deathday"`
	PlaceOfBirth       string `json:"place_of_birth"`
	KnownForDepartment string `json:"known_for_department"`
	CombinedCredits    struct {
		Cast []creditWire `json:"cast"`
		Crew []creditWire `json:"crew"`
	} `json:"combined_credits"`
}

// toDomain keeps the person's movies (TV is dropped: the app has no TV pages),
// one entry per movie, most popular first. For an actor the role comes from
// the cast credit; a crew-only credit (director...) carries the job instead.
func (w personWire) toDomain() domain.Person {
	p := domain.Person{
		ID: w.ID, Name: w.Name, Biography: w.Biography, ProfilePath: w.ProfilePath,
		Birthday: w.Birthday, Deathday: w.Deathday, PlaceOfBirth: w.PlaceOfBirth,
		KnownForDepartment: w.KnownForDepartment, Credits: []domain.Credit{},
	}
	type ranked struct {
		c   domain.Credit
		pop float64
	}
	byMovie := map[int]*ranked{}
	var order []int
	add := func(cw creditWire, cast bool) {
		if cw.MediaType != "movie" || cw.ID == 0 || cw.PosterPath == "" {
			return
		}
		if r, ok := byMovie[cw.ID]; ok {
			if !cast && r.c.Job == "" && r.c.Character == "" {
				r.c.Job = cw.Job
			}
			return
		}
		c := domain.Credit{Movie: cw.toDomain()}
		if cast {
			c.Character = cw.Character
		} else {
			c.Job = cw.Job
		}
		byMovie[cw.ID] = &ranked{c: c, pop: cw.Popularity}
		order = append(order, cw.ID)
	}
	for _, cw := range w.CombinedCredits.Cast {
		add(cw, true)
	}
	for _, cw := range w.CombinedCredits.Crew {
		add(cw, false)
	}
	rs := make([]*ranked, 0, len(order))
	for _, id := range order {
		rs = append(rs, byMovie[id])
	}
	slices.SortStableFunc(rs, func(a, b *ranked) int { return cmp.Compare(b.pop, a.pop) })
	for _, r := range rs[:min(len(rs), maxCredits)] {
		p.Credits = append(p.Credits, r.c)
	}
	return p
}
