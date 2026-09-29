package postgres_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

func TestTVRoundTrip(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	id := 900_000_000 + rand.IntN(99_999_999)
	imdbID := "tt9" + time.Now().Format("150405.000000")
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM tv_shows WHERE id = $1`, id)
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM imdb_ratings WHERE imdb_id = $1`, imdbID)
	})

	if _, _, err := f.repo.GetTV(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing show: %v", err)
	}
	in := domain.Movie{ID: id, MediaType: domain.MediaTV, Title: "Show", Overview: "o", PosterPath: "/p.jpg",
		ReleaseDate: "2011-04-17", VoteAverage: 8.4, VoteCount: 900, Runtime: 60, IMDbID: imdbID,
		Genres: []domain.Genre{{ID: 18, Name: "Drama"}}, CastJSON: `[{"id":1}]`,
		TVDetails: domain.TVDetails{NumberOfSeasons: 8, NumberOfEpisodes: 73, Status: "Ended",
			LastAirDate: "2019-05-19", Networks: []string{"HBO"}, Creators: []string{"A", "B"}}}
	if err := f.repo.UpsertTV(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SaveIMDbRating(ctx, imdbID, domain.IMDbRating{Rating: 9.2, Votes: 2_000_000}); err != nil {
		t.Fatal(err)
	}
	got, at, err := f.repo.GetTV(ctx, id)
	if err != nil || time.Since(at) > time.Minute {
		t.Fatalf("%v, fetched_at %v", err, at)
	}
	if got.MediaType != domain.MediaTV || got.Title != "Show" || got.ReleaseDate != "2011-04-17" || got.Runtime != 60 ||
		got.VoteAverage != 8.4 || got.IMDbRating != 9.2 || len(got.Genres) != 1 || got.CastJSON == "" {
		t.Fatalf("got %+v", got)
	}
	if got.NumberOfSeasons != 8 || got.NumberOfEpisodes != 73 || got.Status != "Ended" || got.LastAirDate != "2019-05-19" ||
		len(got.Networks) != 1 || len(got.Creators) != 2 {
		t.Fatalf("tv details %+v", got.TVDetails)
	}

	// Empty lists round-trip as nil (a minimal show).
	in = domain.Movie{ID: id, Title: "Bare", Overview: ""}
	if err := f.repo.UpsertTV(ctx, in); err != nil {
		t.Fatal(err)
	}
	if got, _, err = f.repo.GetTV(ctx, id); err != nil || got.Genres != nil || got.Networks != nil || got.IMDbRating != 0 {
		t.Fatalf("bare %+v %v", got, err)
	}
}
