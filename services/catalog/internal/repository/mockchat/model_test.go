package mockchat

import (
	"context"
	"testing"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

func TestPicksByKeywordInTheLastMessage(t *testing.T) {
	ask := func(msgs ...string) domain.ChatReply {
		var h []domain.ChatMessage
		for _, m := range msgs {
			h = append(h, domain.ChatMessage{Role: domain.RoleUser, Content: m})
		}
		r, err := Model{}.Reply(context.Background(), domain.ChatRequest{UserID: "u", History: h})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := ask("hi"); r.MovieIDs[0] != 27205 || r.Message == "" {
		t.Fatalf("greeting: %+v", r)
	}
	if r := ask("hi", "Something SCARY please"); r.MovieIDs[0] != 694 {
		t.Fatalf("horror: %+v", r)
	}
	if r := ask("a space adventure"); r.MovieIDs[0] != 157336 {
		t.Fatalf("sci-fi: %+v", r)
	}
	// Callers may keep the slice: it must not alias the curated list.
	r := ask("comedy")
	r.MovieIDs[0] = -1
	if ask("comedy").MovieIDs[0] == -1 {
		t.Fatal("reply shares the curated slice")
	}
}

func TestTurkishDemoReplies(t *testing.T) {
	r, err := Model{}.Reply(context.Background(), domain.ChatRequest{
		History: []domain.ChatMessage{{Role: domain.RoleUser, Content: "komik bir şey"}}, Locale: domain.LocaleTR,
	})
	if err != nil || r.Message != "Keyfini yerine getirecek birkaç komedi:" || len(r.MovieIDs) == 0 {
		t.Fatalf("%+v %v", r, err)
	}
}
