package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
)

// fakeLibrary behaves like the Postgres store: writes fail with
// ErrMovieNotStored until the movie is in `stored`.
type fakeLibrary struct {
	stored  map[int]bool
	list    map[int]bool
	ratings map[int]int
	writes  int
}

func newFakeLibrary() *fakeLibrary {
	return &fakeLibrary{stored: map[int]bool{}, list: map[int]bool{}, ratings: map[int]int{}}
}

func (f *fakeLibrary) AddToWatchlist(_ context.Context, _ string, id int) (domain.WatchlistItem, error) {
	f.writes++
	if !f.stored[id] {
		return domain.WatchlistItem{}, domain.ErrMovieNotStored
	}
	f.list[id] = true
	return domain.WatchlistItem{Movie: domain.Movie{ID: id}}, nil
}
func (f *fakeLibrary) RemoveFromWatchlist(_ context.Context, _ string, id int) error {
	delete(f.list, id)
	return nil
}
func (f *fakeLibrary) GetUserWatchlist(context.Context, string) ([]domain.WatchlistItem, error) {
	return nil, nil
}
func (f *fakeLibrary) UpsertUserRating(_ context.Context, _ string, id, rating int) (domain.UserRating, error) {
	f.writes++
	if !f.stored[id] {
		return domain.UserRating{}, domain.ErrMovieNotStored
	}
	f.ratings[id] = rating
	return domain.UserRating{Movie: domain.Movie{ID: id}, Rating: rating}, nil
}
func (f *fakeLibrary) GetUserRatings(context.Context, string) ([]domain.UserRating, error) {
	return nil, nil
}

type fakeStorer struct {
	lib   *fakeLibrary
	calls int
	err   error
}

func (s *fakeStorer) EnsureStored(_ context.Context, id int) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	s.lib.stored[id] = true
	return nil
}

const alice = "7f2c1a4e-0000-4000-8000-000000000001"

func TestLibraryStoresUnknownMovieThenRetries(t *testing.T) {
	lib := newFakeLibrary()
	storer := &fakeStorer{lib: lib}
	svc := service.NewLibrary(lib, storer)
	ctx := context.Background()

	if _, err := svc.AddToWatchlist(ctx, alice, 42); err != nil {
		t.Fatal(err)
	}
	if !lib.list[42] || storer.calls != 1 || lib.writes != 2 {
		t.Fatalf("list=%v ensure calls=%d writes=%d", lib.list, storer.calls, lib.writes)
	}

	// Already stored: one write, no fetch.
	if _, err := svc.RateMovie(ctx, alice, 42, 8); err != nil {
		t.Fatal(err)
	}
	if lib.ratings[42] != 8 || storer.calls != 1 || lib.writes != 3 {
		t.Fatalf("ratings=%v ensure calls=%d writes=%d", lib.ratings, storer.calls, lib.writes)
	}
}

func TestLibraryReportsMovieThatCannotBeStored(t *testing.T) {
	lib := newFakeLibrary()
	svc := service.NewLibrary(lib, &fakeStorer{lib: lib, err: domain.ErrNotFound})
	if _, err := svc.RateMovie(context.Background(), alice, 999, 5); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestLibraryValidatesInput(t *testing.T) {
	lib := newFakeLibrary()
	svc := service.NewLibrary(lib, &fakeStorer{lib: lib})
	ctx := context.Background()
	for name, err := range map[string]error{
		"rating 0":        func() error { _, err := svc.RateMovie(ctx, alice, 1, 0); return err }(),
		"rating 11":       func() error { _, err := svc.RateMovie(ctx, alice, 1, 11); return err }(),
		"movie 0 rate":    func() error { _, err := svc.RateMovie(ctx, alice, 0, 5); return err }(),
		"movie 0 add":     func() error { _, err := svc.AddToWatchlist(ctx, alice, 0); return err }(),
		"movie -1 remove": svc.RemoveFromWatchlist(ctx, alice, -1),
	} {
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}
	if lib.writes != 0 {
		t.Fatalf("invalid input reached the store %d times", lib.writes)
	}
}

func TestEnsureStored(t *testing.T) {
	ctx := context.Background()

	t.Run("already stored: no provider call", func(t *testing.T) {
		p, st := &fakeProvider{}, &fakeStore{}
		_ = st.UpsertMovie(ctx, domain.Movie{ID: 3, Title: "Stored"})
		svc, _ := newSvc(t, p, st)
		if err := svc.EnsureStored(ctx, 3); err != nil {
			t.Fatal(err)
		}
		if p.calls.Load() != 0 {
			t.Fatalf("provider called %d times", p.calls.Load())
		}
	})

	t.Run("missing: fetched and stored, even when L1 has it", func(t *testing.T) {
		p, st := &fakeProvider{}, &fakeStore{}
		svc, _ := newSvc(t, p, st)
		// Warm L1 only (as a cache filled before L2 existed would be).
		if _, err := svc.GetMovieDetails(ctx, 5); err != nil {
			t.Fatal(err)
		}
		delete(st.movies, 5)

		if err := svc.EnsureStored(ctx, 5); err != nil {
			t.Fatal(err)
		}
		if _, ok := st.movies[5]; !ok {
			t.Fatal("movie not stored")
		}
	})

	t.Run("unknown to TMDB", func(t *testing.T) {
		p := &fakeProvider{}
		p.fail(domain.ErrNotFound)
		svc, _ := newSvc(t, p, &fakeStore{})
		if err := svc.EnsureStored(ctx, 404); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
}
