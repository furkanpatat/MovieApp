package tmdb

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// TMDB response shapes, kept private so the domain stays independent of them.

type genresWire []struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (w genresWire) toDomain() []domain.Genre {
	var out []domain.Genre
	for _, g := range w {
		out = append(out, domain.Genre{ID: g.ID, Name: g.Name})
	}
	return out
}

type videosWire struct {
	Results []struct {
		Key  string `json:"key"`
		Type string `json:"type"`
		Site string `json:"site"`
	} `json:"results"`
}

// trailer is the first YouTube trailer, if any.
func (w videosWire) trailer() string {
	for _, v := range w.Results {
		if v.Site == "YouTube" && v.Type == "Trailer" {
			return v.Key
		}
	}
	return ""
}

type creditsWire struct {
	Cast []any `json:"cast"`
}

// castJSON passes the cast through as TMDB sends it ("" when empty).
func (w creditsWire) castJSON() string {
	if len(w.Cast) == 0 {
		return ""
	}
	b, _ := json.Marshal(w.Cast)
	return string(b)
}

type movieWire struct {
	ID           int         `json:"id"`
	Title        string      `json:"title"`
	Overview     string      `json:"overview"`
	PosterPath   string      `json:"poster_path"`
	BackdropPath string      `json:"backdrop_path"`
	ReleaseDate  string      `json:"release_date"`
	VoteAverage  float64     `json:"vote_average"`
	VoteCount    int         `json:"vote_count"`
	Tagline      string      `json:"tagline"`
	Runtime      int         `json:"runtime"`
	IMDbID       string      `json:"imdb_id"`
	Genres       genresWire  `json:"genres"`
	Videos       videosWire  `json:"videos"`
	Credits      creditsWire `json:"credits"`
}

func (w movieWire) toDomain() domain.Movie {
	return domain.Movie{
		ID: w.ID, MediaType: domain.MediaMovie, Title: w.Title, Overview: w.Overview, PosterPath: w.PosterPath,
		BackdropPath: w.BackdropPath,
		ReleaseDate:  w.ReleaseDate, VoteAverage: w.VoteAverage, VoteCount: w.VoteCount,
		Tagline: w.Tagline, Runtime: w.Runtime, IMDbID: w.IMDbID,
		Genres: w.Genres.toDomain(), TrailerKey: w.Videos.trailer(), CastJSON: w.Credits.castJSON(),
	}
}

// tvWire is a TV series (lists and /tv/{id}); TMDB names its fields differently.
type tvWire struct {
	ID               int         `json:"id"`
	Name             string      `json:"name"`
	Overview         string      `json:"overview"`
	PosterPath       string      `json:"poster_path"`
	BackdropPath     string      `json:"backdrop_path"`
	FirstAirDate     string      `json:"first_air_date"`
	LastAirDate      string      `json:"last_air_date"`
	VoteAverage      float64     `json:"vote_average"`
	VoteCount        int         `json:"vote_count"`
	Tagline          string      `json:"tagline"`
	Status           string      `json:"status"`
	NumberOfSeasons  int         `json:"number_of_seasons"`
	NumberOfEpisodes int         `json:"number_of_episodes"`
	EpisodeRunTime   []int       `json:"episode_run_time"`
	Genres           genresWire  `json:"genres"`
	Videos           videosWire  `json:"videos"`
	Credits          creditsWire `json:"credits"`
	Networks         []struct {
		Name string `json:"name"`
	} `json:"networks"`
	CreatedBy []struct {
		Name string `json:"name"`
	} `json:"created_by"`
	ExternalIDs struct {
		IMDbID string `json:"imdb_id"`
	} `json:"external_ids"`
}

func (w tvWire) toDomain() domain.Movie {
	m := domain.Movie{
		ID: w.ID, MediaType: domain.MediaTV, Title: w.Name, Overview: w.Overview, PosterPath: w.PosterPath,
		BackdropPath: w.BackdropPath, ReleaseDate: w.FirstAirDate, VoteAverage: w.VoteAverage, VoteCount: w.VoteCount,
		Tagline: w.Tagline, IMDbID: w.ExternalIDs.IMDbID,
		Genres: w.Genres.toDomain(), TrailerKey: w.Videos.trailer(), CastJSON: w.Credits.castJSON(),
		TVDetails: domain.TVDetails{
			NumberOfSeasons: w.NumberOfSeasons, NumberOfEpisodes: w.NumberOfEpisodes,
			Status: w.Status, LastAirDate: w.LastAirDate,
		},
	}
	if len(w.EpisodeRunTime) > 0 {
		m.Runtime = w.EpisodeRunTime[0]
	}
	for _, n := range w.Networks {
		m.Networks = append(m.Networks, n.Name)
	}
	for _, c := range w.CreatedBy {
		m.Creators = append(m.Creators, c.Name)
	}
	return m
}

type tvPageWire struct {
	Page         int      `json:"page"`
	TotalPages   int      `json:"total_pages"`
	TotalResults int      `json:"total_results"`
	Results      []tvWire `json:"results"`
}

func (w tvPageWire) toDomain() domain.MoviePage {
	p := domain.MoviePage{Page: w.Page, TotalPages: w.TotalPages, TotalResults: w.TotalResults}
	p.Results = make([]domain.Movie, 0, len(w.Results))
	for _, s := range w.Results {
		p.Results = append(p.Results, s.toDomain())
	}
	return p
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
