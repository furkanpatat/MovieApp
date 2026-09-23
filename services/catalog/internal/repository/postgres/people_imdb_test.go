package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

func TestPeopleRoundTrip(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	id := 900_000_000 + rand.IntN(99_999_999)
	t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM people WHERE id = $1`, id) })

	if _, _, err := f.repo.GetPerson(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing person: %v", err)
	}
	in := domain.Person{ID: id, Name: "Ada", Biography: "bio", ProfilePath: "/p.jpg", Birthday: "1990-01-02",
		Credits: []domain.Credit{{Movie: domain.Movie{ID: 1, Title: "One", PosterPath: "/1.jpg"}, Character: "Lead"}}}
	if err := f.repo.UpsertPerson(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, at, err := f.repo.GetPerson(ctx, id)
	if err != nil || time.Since(at) > time.Minute {
		t.Fatalf("%v, fetched_at %v", err, at)
	}
	if got.Name != "Ada" || got.Birthday != "1990-01-02" || got.Deathday != "" || len(got.Credits) != 1 ||
		got.Credits[0].Title != "One" || got.Credits[0].Character != "Lead" {
		t.Fatalf("got %+v", got)
	}
}

func TestIMDbRatings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	imdbID := fmt.Sprintf("tt9%08d", rand.IntN(99_999_999))
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM imdb_ratings WHERE imdb_id = $1`, imdbID)
	})

	withIMDb := f.movie(t, "Linked")
	unlinked := f.movie(t, "Unlinked")
	if _, err := f.pool.Exec(ctx, `UPDATE movies SET imdb_id = $1 WHERE id = $2`, imdbID, withIMDb); err != nil {
		t.Fatal(err)
	}

	if _, err := f.repo.GetIMDbRating(ctx, imdbID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing rating: %v", err)
	}
	details := domain.OMDbDetails{Rated: "PG-13", RottenTomatoes: "85%", Metascore: 67, Awards: "1 win",
		Director: "D", Writer: "W", BoxOffice: "$1", Country: "US", Language: "English"}
	if err := f.repo.SaveIMDbRating(ctx, imdbID, domain.IMDbRating{Rating: 8.1, Votes: 42, OMDbDetails: details}); err != nil {
		t.Fatal(err)
	}
	r, err := f.repo.GetIMDbRating(ctx, imdbID)
	if err != nil || r.Rating != 8.1 || r.Votes != 42 || r.FetchedAt.IsZero() || r.OMDbDetails != details {
		t.Fatalf("got %+v, %v", r, err)
	}

	known, err := f.repo.IMDbRatingsForMovies(ctx, []int{withIMDb, unlinked})
	if err != nil || len(known) != 1 || known[withIMDb].Rating != 8.1 {
		t.Fatalf("known = %+v, %v", known, err)
	}

	// Details read the rating through the movie's IMDb id.
	m, _, err := f.repo.GetMovie(ctx, withIMDb)
	if err != nil || m.IMDbID != imdbID || m.IMDbRating != 8.1 || m.IMDbVotes != 42 || m.OMDbDetails != details {
		t.Fatalf("movie = %+v, %v", m, err)
	}

	// "No rating yet" is remembered, and not listed.
	if err := f.repo.SaveIMDbRating(ctx, imdbID, domain.IMDbRating{}); err != nil {
		t.Fatal(err)
	}
	if known, _ := f.repo.IMDbRatingsForMovies(ctx, []int{withIMDb}); len(known) != 0 {
		t.Fatalf("unrated title listed: %+v", known)
	}
}
