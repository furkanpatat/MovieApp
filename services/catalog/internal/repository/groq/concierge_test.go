package groq

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeSearch returns one movie per known title.
type fakeSearch struct {
	mu      sync.Mutex
	queries []string
}

var catalog = map[string]domain.Movie{
	"heat":       {ID: 949, Title: "Heat", ReleaseDate: "1995-12-15", VoteAverage: 7.9},
	"collateral": {ID: 1538, Title: "Collateral", ReleaseDate: "2004-08-05", IMDbRating: 7.5},
}

func (f *fakeSearch) SearchMovies(_ context.Context, q string, _ int) (domain.MoviePage, error) {
	f.mu.Lock()
	f.queries = append(f.queries, q)
	f.mu.Unlock()
	if m, ok := catalog[strings.ToLower(q)]; ok {
		return domain.MoviePage{Results: []domain.Movie{m}}, nil
	}
	return domain.MoviePage{}, nil
}

// fakeGroq plays back scripted completions and records the requests.
type fakeGroq struct {
	t        *testing.T
	mu       sync.Mutex
	requests []completionRequest
	replies  []func(req completionRequest) (int, string)
}

func (f *fakeGroq) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer gsk-test" {
		f.t.Errorf("bad request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
	}
	var req completionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	n := len(f.requests)
	f.mu.Unlock()
	next := f.replies[min(n, len(f.replies))-1]
	status, body := next(req)
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func newConcierge(t *testing.T, replies ...func(completionRequest) (int, string)) (*Concierge, *fakeGroq, *fakeSearch) {
	f := &fakeGroq{t: t, replies: replies}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	s := &fakeSearch{}
	return New(Config{APIKey: "gsk-test", Model: "test-model", BaseURL: srv.URL + "/v1", Logger: quiet}, s), f, s
}

func toolCalls(queries ...string) func(completionRequest) (int, string) {
	return func(completionRequest) (int, string) {
		var calls []map[string]any
		for i, q := range queries {
			args, _ := json.Marshal(map[string]string{"query": q})
			calls = append(calls, map[string]any{"id": "call_" + string(rune('a'+i)), "type": "function",
				"function": map[string]any{"name": "search_movies", "arguments": string(args)}})
		}
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "content": nil, "tool_calls": calls}}}})
		return 200, string(b)
	}
}

func final(content string) func(completionRequest) (int, string) {
	return func(completionRequest) (int, string) {
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}})
		return 200, string(b)
	}
}

var hi = domain.ChatRequest{UserID: "u1", History: []domain.ChatMessage{{Role: "user", Content: "Crime thrillers like Michael Mann's"}}}

func TestSearchesThenAnswersWithGroundedIDs(t *testing.T) {
	k, f, s := newConcierge(t,
		toolCalls("Heat", "Collateral", "Nonexistent"),
		// 603 was never looked up: it must be dropped even though it's a real id.
		final("```json\n"+`{"message":"**Heat** and **Collateral**: Mann at his sharpest.","movie_ids":[949,603,1538]}`+"\n```"),
	)
	r, err := k.Reply(context.Background(), hi)
	if err != nil {
		t.Fatal(err)
	}
	if r.Message == "" || len(r.MovieIDs) != 2 || r.MovieIDs[0] != 949 || r.MovieIDs[1] != 1538 {
		t.Fatalf("reply %+v", r)
	}
	if len(s.queries) != 3 || len(f.requests) != 2 {
		t.Fatalf("searches %v, %d requests (JSON on its own needs no answer step)", s.queries, len(f.requests))
	}

	first := f.requests[0]
	if first.Model != "test-model" || first.User != "u1" || first.ToolChoice != "auto" || len(first.Tools) != 1 || first.ResponseFormat != nil {
		t.Fatalf("first request %+v", first)
	}
	if sys := *first.Messages[0].Content; first.Messages[0].Role != "system" || !strings.HasPrefix(sys, Persona) {
		t.Fatalf("system prompt %q", sys)
	}

	// Second request: assistant tool_calls turn, then one tool result per call.
	second := f.requests[1].Messages
	tail := second[len(second)-4:]
	if tail[0].Role != "assistant" || len(tail[0].ToolCalls) != 3 {
		t.Fatalf("assistant turn %+v", tail[0])
	}
	var res struct {
		Results []hit `json:"results"`
	}
	if err := json.Unmarshal([]byte(*tail[1].Content), &res); err != nil || tail[1].Role != "tool" || tail[1].ToolCallID != "call_a" ||
		len(res.Results) != 1 || res.Results[0].ID != 949 || res.Results[0].Year != "1995" {
		t.Fatalf("tool result %+v %v", res, err)
	}
	if !strings.Contains(*tail[3].Content, `"results":[]`) {
		t.Fatalf("empty search result %q", *tail[3].Content)
	}
}

// Prose instead of JSON: one more request, in JSON mode, without tools, with
// the searches in the prompt.
func TestProseIsReformattedInJSONMode(t *testing.T) {
	k, f, _ := newConcierge(t,
		toolCalls("Heat"),
		final("Try **Heat**, it's great."),
		final(`{"message":"Try **Heat**.","movie_ids":[949]}`),
	)
	r, err := k.Reply(context.Background(), hi)
	if err != nil || len(r.MovieIDs) != 1 || r.MovieIDs[0] != 949 {
		t.Fatalf("%+v %v", r, err)
	}
	if len(f.requests) != 3 {
		t.Fatalf("%d requests", len(f.requests))
	}
	last := f.requests[2]
	if last.ResponseFormat == nil || last.ResponseFormat.Type != "json_object" || len(last.Tools) != 0 || last.ToolChoice != "" {
		t.Fatalf("answer request %+v", last)
	}
	for _, m := range last.Messages {
		if m.Role == "tool" || len(m.ToolCalls) > 0 {
			t.Fatalf("answer request replays tool turns: %+v", m)
		}
	}
	if sys := *last.Messages[0].Content; !strings.Contains(sys, `"id":949`) || !strings.Contains(sys, "JSON") {
		t.Fatalf("answer prompt %q", sys)
	}
}

func TestMalformedToolCallStillAnswers(t *testing.T) {
	k, f, _ := newConcierge(t,
		toolCalls("Heat"),
		func(completionRequest) (int, string) {
			return 400, `{"error":{"message":"Failed to call a function.","type":"invalid_request_error","code":"tool_use_failed"}}`
		},
		final(`{"message":"**Heat**.","movie_ids":[949]}`),
	)
	r, err := k.Reply(context.Background(), hi)
	if err != nil || len(r.MovieIDs) != 1 || len(f.requests) != 3 || f.requests[2].ResponseFormat == nil {
		t.Fatalf("%+v %v, %d requests", r, err, len(f.requests))
	}
}

func TestPageContextIsInThePromptAndItsMoviesNeedNoSearch(t *testing.T) {
	k, f, _ := newConcierge(t, final(`{"message":"You're on it already.","movie_ids":[27205]}`))
	req := hi
	req.Page, req.PageMovieIDs = `A movie page: "Inception" (2010), id 27205.`, []int{27205}
	r, err := k.Reply(context.Background(), req)
	if err != nil || len(r.MovieIDs) != 1 || r.MovieIDs[0] != 27205 {
		t.Fatalf("%+v %v", r, err)
	}
	if !strings.Contains(*f.requests[0].Messages[0].Content, "Inception") {
		t.Fatal("page context missing from the system prompt")
	}
}

func TestRoundLimitForcesAnAnswer(t *testing.T) {
	k, f, _ := newConcierge(t,
		toolCalls("Heat"), toolCalls("Heat"), toolCalls("Heat"),
		final(`{"message":"Heat.","movie_ids":[949]}`),
	)
	r, err := k.Reply(context.Background(), hi)
	if err != nil || len(r.MovieIDs) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if n := len(f.requests); n != maxRounds+1 || f.requests[n-1].ResponseFormat == nil || len(f.requests[n-1].Tools) != 0 {
		t.Fatalf("%d requests, last %+v", n, f.requests[n-1])
	}
}

func TestFailuresAreUnavailable(t *testing.T) {
	cases := map[string][]func(completionRequest) (int, string){
		"bad key": {func(completionRequest) (int, string) {
			return 401, `{"error":{"message":"Invalid API Key","code":"invalid_api_key"}}`
		}},
		"rate limit": {func(completionRequest) (int, string) {
			return 429, `{"error":{"message":"Rate limit reached"}}`
		}},
		"not json":   {final(`Sure! Here are some movies`)},
		"no message": {final(`{"message":"","movie_ids":[]}`)},
		"cut off": {final("prose"), func(completionRequest) (int, string) {
			return 200, `{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"{\"message\":\"Heat is"}}]}`
		}},
	}
	for name, replies := range cases {
		k, _, _ := newConcierge(t, replies...)
		_, err := k.Reply(context.Background(), hi)
		if !errors.Is(err, domain.ErrUnavailable) || strings.Contains(err.Error(), "gsk-test") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTurkishLocaleAsksForTurkish(t *testing.T) {
	k, f, _ := newConcierge(t, final(`{"message":"**Heat** harika.","movie_ids":[]}`))
	req := hi
	req.Locale = domain.LocaleTR
	if _, err := k.Reply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	sys := *f.requests[0].Messages[0].Content
	if !strings.Contains(sys, "Turkish (Türkçe)") || strings.Contains(sys, `"message" in English`) {
		t.Fatalf("system prompt lacks the Turkish rule:\n%s", sys)
	}

	k, f, _ = newConcierge(t, final(`{"message":"Heat.","movie_ids":[]}`))
	if _, err := k.Reply(context.Background(), hi); err != nil {
		t.Fatal(err)
	}
	if sys := *f.requests[0].Messages[0].Content; !strings.Contains(sys, `"message" in English`) {
		t.Fatalf("default should be English:\n%s", sys)
	}
}

func jsonValidateFailed(generation string) func(completionRequest) (int, string) {
	return func(completionRequest) (int, string) {
		b, _ := json.Marshal(map[string]any{"error": map[string]any{
			"message": "Failed to validate JSON.", "type": "invalid_request_error",
			"code": "json_validate_failed", "failed_generation": generation,
		}})
		return 400, string(b)
	}
}

func TestRejectedJSONIsSalvagedOrRetried(t *testing.T) {
	// The rejected output holds usable JSON: used as is.
	k, f, _ := newConcierge(t, toolCalls("Heat"), final("prose"),
		jsonValidateFailed("Here you go:\n{\"message\":\"**Heat**.\",\"movie_ids\":[949]}\nEnjoy!"))
	r, err := k.Reply(context.Background(), hi)
	if err != nil || len(r.MovieIDs) != 1 || len(f.requests) != 3 {
		t.Fatalf("salvage: %+v %v, %d requests", r, err, len(f.requests))
	}

	// Nothing usable: asked again, once.
	k, f, _ = newConcierge(t, toolCalls("Heat"), final("prose"), jsonValidateFailed("Sure! Heat is great"),
		final(`{"message":"**Heat**.","movie_ids":[949]}`))
	if r, err = k.Reply(context.Background(), hi); err != nil || len(r.MovieIDs) != 1 || len(f.requests) != 4 {
		t.Fatalf("retry: %+v %v, %d requests", r, err, len(f.requests))
	}

	// Still failing: unavailable, without looping.
	k, f, _ = newConcierge(t, final("prose"), jsonValidateFailed("nope"))
	if _, err = k.Reply(context.Background(), hi); !errors.Is(err, domain.ErrUnavailable) || len(f.requests) != 3 {
		t.Fatalf("give up: %v, %d requests", err, len(f.requests))
	}
}

func TestRateLimitCarriesRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"Rate limit reached"}}`)
	}))
	t.Cleanup(srv.Close)
	k := New(Config{APIKey: "gsk-test", Model: "m", BaseURL: srv.URL + "/v1", Logger: quiet}, &fakeSearch{})
	_, err := k.Reply(context.Background(), hi)
	var rl *domain.RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != 7*time.Second || !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}
