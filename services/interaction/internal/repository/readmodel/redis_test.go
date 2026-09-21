package readmodel_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
	"github.com/furkanpatat/movieapp/services/interaction/internal/repository/readmodel"
)

func setup(t *testing.T, keep int) (*readmodel.Model, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)
	return readmodel.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), keep), mr
}

func comment(i int) domain.Comment {
	return domain.Comment{ID: fmt.Sprint("c", i), UserID: "u", Text: fmt.Sprint("text ", i),
		CreatedAt: time.UnixMilli(1_700_000_000_000 + int64(i)*1000).UTC()}
}

func TestMissingModelIsReportedNotInvented(t *testing.T) {
	m, _ := setup(t, 5)
	ctx := context.Background()
	if _, found, err := m.Get(ctx, 1, 5); found || err != nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
	// Writes against a missing model must ask for Init, not create a partial model.
	if ok, err := m.ApplyRating(ctx, domain.RatingStats{MovieID: 1, TotalScore: 8, VoteCount: 1, Version: 1}); ok || err != nil {
		t.Fatalf("ApplyRating on missing model: ok=%v err=%v", ok, err)
	}
	if ok, err := m.AddComment(ctx, 1, comment(1)); ok || err != nil {
		t.Fatalf("AddComment on missing model: ok=%v err=%v", ok, err)
	}
	if _, found, _ := m.Get(ctx, 1, 5); found {
		t.Fatal("failed writes must not create a model")
	}
}

func TestInitApplyAndGet(t *testing.T) {
	m, _ := setup(t, 5)
	ctx := context.Background()
	if err := m.Init(ctx, domain.RatingStats{MovieID: 1, TotalScore: 15, VoteCount: 2, Version: 2}, []domain.Comment{comment(1), comment(2)}); err != nil {
		t.Fatal(err)
	}
	got, found, err := m.Get(ctx, 1, 5)
	if err != nil || !found || got.TotalVotes != 2 || got.AverageRating != 7.5 {
		t.Fatalf("%+v %v %v", got, found, err)
	}
	if len(got.RecentComments) != 2 || got.RecentComments[0].ID != "c2" { // newest first
		t.Fatalf("order: %+v", got.RecentComments)
	}

	if ok, err := m.ApplyRating(ctx, domain.RatingStats{MovieID: 1, TotalScore: 25, VoteCount: 3, Version: 3}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	got, _, _ = m.Get(ctx, 1, 5)
	if got.TotalVotes != 3 || got.AverageRating < 8.33 || got.AverageRating > 8.34 {
		t.Fatalf("%+v", got)
	}
}

func TestStaleAggregateCannotOverwriteNewer(t *testing.T) {
	m, _ := setup(t, 5)
	ctx := context.Background()
	_ = m.Init(ctx, domain.RatingStats{MovieID: 1, TotalScore: 30, VoteCount: 3, Version: 5}, nil)

	// A worker that read Postgres earlier arrives late with an older aggregate.
	if ok, err := m.ApplyRating(ctx, domain.RatingStats{MovieID: 1, TotalScore: 20, VoteCount: 2, Version: 4}); !ok || err != nil {
		t.Fatal(ok, err)
	}
	// ...and so does a slow rebuild.
	if err := m.Init(ctx, domain.RatingStats{MovieID: 1, TotalScore: 10, VoteCount: 1, Version: 3}, nil); err != nil {
		t.Fatal(err)
	}
	got, _, _ := m.Get(ctx, 1, 5)
	if got.TotalVotes != 3 {
		t.Fatalf("older version overwrote newer: %+v", got)
	}
}

func TestCommentsTrimAndIdempotent(t *testing.T) {
	m, mr := setup(t, 3)
	ctx := context.Background()
	_ = m.Init(ctx, domain.RatingStats{MovieID: 1}, nil)
	for i := 1; i <= 5; i++ {
		if ok, err := m.AddComment(ctx, 1, comment(i)); !ok || err != nil {
			t.Fatal(ok, err)
		}
	}
	_, _ = m.AddComment(ctx, 1, comment(5)) // redelivery
	_, _ = m.AddComment(ctx, 1, comment(5))

	if n, _ := mr.ZMembers("interaction:1:comments"); len(n) != 3 {
		t.Fatalf("retained %d, want 3 (trim + dedupe)", len(n))
	}
	got, _, _ := m.Get(ctx, 1, 100) // limit clamps to retained
	if len(got.RecentComments) != 3 || got.RecentComments[0].ID != "c5" || got.RecentComments[2].ID != "c3" {
		t.Fatalf("%+v", got.RecentComments)
	}
	got, _, _ = m.Get(ctx, 1, 2)
	if len(got.RecentComments) != 2 {
		t.Fatalf("limit not applied: %d", len(got.RecentComments))
	}
}

func TestInitTrimsToKeep(t *testing.T) {
	m, _ := setup(t, 2)
	var cs []domain.Comment
	for i := 1; i <= 6; i++ {
		cs = append(cs, comment(i))
	}
	_ = m.Init(context.Background(), domain.RatingStats{MovieID: 1}, cs)
	got, _, _ := m.Get(context.Background(), 1, 10)
	if len(got.RecentComments) != 2 || got.RecentComments[0].ID != "c6" {
		t.Fatalf("%+v", got.RecentComments)
	}
}
