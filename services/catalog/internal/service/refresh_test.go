package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
	"github.com/furkanpatat/movieapp/services/catalog/internal/service"
)

func TestRefreshWarmsPopularAndDetails(t *testing.T) {
	p := &fakeProvider{}
	svc, mr := newSvc(t, p, nil)
	ctx := context.Background()

	res, err := svc.RefreshPopular(ctx, service.RefreshOptions{Pages: 2, WarmDetails: true})
	if err != nil {
		t.Fatal(err)
	}
	// fakeProvider returns movie id 1 on every page: 2 popular calls + 1 deduped details call.
	if res.Pages != 2 || res.Details != 1 || p.calls.Load() != 3 {
		t.Fatalf("res=%+v calls=%d", res, p.calls.Load())
	}

	// Everything is now served from Redis with no further TMDB calls.
	_, _ = svc.GetPopularMovies(ctx, 1)
	_, _ = svc.GetPopularMovies(ctx, 2)
	_, _ = svc.GetMovieDetails(ctx, 1)
	if p.calls.Load() != 3 {
		t.Fatalf("warm cache was not used: calls=%d", p.calls.Load())
	}
	if ttl := mr.TTL("catalog:popular:1"); ttl != time.Hour {
		t.Fatalf("ttl = %v", ttl)
	}
}

func TestRefreshOverwritesExistingEntry(t *testing.T) {
	p := &fakeProvider{}
	svc, mr := newSvc(t, p, nil)
	ctx := context.Background()
	_, _ = svc.GetPopularMovies(ctx, 1)
	mr.FastForward(50 * time.Minute) // nearly expired

	if _, err := svc.RefreshPopular(ctx, service.RefreshOptions{Pages: 1}); err != nil {
		t.Fatal(err)
	}
	if ttl := mr.TTL("catalog:popular:1"); ttl != time.Hour {
		t.Fatalf("refresh should reset ttl, got %v", ttl)
	}
}

func TestRefreshSkipsDetailsWhenDisabled(t *testing.T) {
	p := &fakeProvider{}
	svc, _ := newSvc(t, p, nil)
	res, err := svc.RefreshPopular(context.Background(), service.RefreshOptions{Pages: 1})
	if err != nil || res.Details != 0 || p.calls.Load() != 1 {
		t.Fatalf("res=%+v calls=%d err=%v", res, p.calls.Load(), err)
	}
}

func TestRefreshStopsOnUpstreamOutage(t *testing.T) {
	p := &fakeProvider{}
	p.fail(domain.ErrUnavailable)
	svc, _ := newSvc(t, p, nil)
	_, err := svc.RefreshPopular(context.Background(), service.RefreshOptions{Pages: 5, WarmDetails: true})
	if !errors.Is(err, domain.ErrUnavailable) || p.calls.Load() != 1 {
		t.Fatalf("want abort after first call, err=%v calls=%d", err, p.calls.Load())
	}
}

func TestRefreshKeepsStaleDataDuringOutage(t *testing.T) {
	p := &fakeProvider{}
	svc, mr := newSvc(t, p, nil)
	ctx := context.Background()
	_, _ = svc.RefreshPopular(ctx, service.RefreshOptions{Pages: 1})
	mr.FastForward(2 * time.Hour)
	p.fail(domain.ErrUnavailable)
	if _, err := svc.RefreshPopular(ctx, service.RefreshOptions{Pages: 1}); err == nil {
		t.Fatal("expected error")
	}
	if pg, err := svc.GetPopularMovies(ctx, 1); err != nil || len(pg.Results) != 1 {
		t.Fatalf("failed refresh must not destroy stale copy: %+v %v", pg, err)
	}
}

func TestRefreshValidationAndCancellation(t *testing.T) {
	svc, _ := newSvc(t, &fakeProvider{}, nil)
	if _, err := svc.RefreshPopular(context.Background(), service.RefreshOptions{Pages: 0}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.RefreshPopular(ctx, service.RefreshOptions{Pages: 3, Pause: time.Second}); err == nil {
		t.Fatal("cancelled context should abort")
	}
}
