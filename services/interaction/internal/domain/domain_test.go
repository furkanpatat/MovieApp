package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

func TestRatingValidation(t *testing.T) {
	ok := domain.RatingSubmitted{EventID: "e", MovieID: 1, UserID: "u", Score: 7}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*domain.RatingSubmitted){
		"score 0":       func(e *domain.RatingSubmitted) { e.Score = 0 },
		"score 11":      func(e *domain.RatingSubmitted) { e.Score = 11 },
		"negative":      func(e *domain.RatingSubmitted) { e.Score = -3 },
		"movie 0":       func(e *domain.RatingSubmitted) { e.MovieID = 0 },
		"blank user":    func(e *domain.RatingSubmitted) { e.UserID = "  " },
		"long user":     func(e *domain.RatingSubmitted) { e.UserID = strings.Repeat("x", 65) },
		"missing event": func(e *domain.RatingSubmitted) { e.EventID = "" },
	}
	for name, mut := range cases {
		e := ok
		mut(&e)
		if err := e.Validate(); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: want ErrInvalidInput, got %v", name, err)
		}
	}
	for _, s := range []int{1, 10} {
		e := ok
		e.Score = s
		if e.Validate() != nil {
			t.Errorf("score %d should be valid", s)
		}
	}
}

func TestCommentValidation(t *testing.T) {
	ok := domain.CommentAdded{EventID: "e", MovieID: 1, UserID: "u", Text: "nice"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []string{"", "   \n", strings.Repeat("a", 1001)}
	for _, txt := range bad {
		e := ok
		e.Text = txt
		if err := e.Validate(); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("text len %d: got %v", len(txt), err)
		}
	}
	e := ok
	e.Text = strings.Repeat("ş", 1000) // counted in runes, not bytes
	if err := e.Validate(); err != nil {
		t.Errorf("1000 multibyte runes should be valid: %v", err)
	}
}
