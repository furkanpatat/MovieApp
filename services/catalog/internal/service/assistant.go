package service

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// ChatResponse is the reply plus the recommended movies, already resolved so
// the client can render cards without a request per movie.
type ChatResponse struct {
	Message  string         `json:"message"` // Markdown
	MovieIDs []int          `json:"movie_ids"`
	Movies   []domain.Movie `json:"movies"`
	Demo     bool           `json:"demo,omitempty"` // canned replies, no LLM configured
}

// Assistant is the movie recommendation chat.
type Assistant struct {
	model   domain.ChatModel
	catalog *Catalog
	timeout time.Duration // per movie lookup
	demo    bool
}

func NewAssistant(model domain.ChatModel, catalog *Catalog) *Assistant {
	return &Assistant{model: model, catalog: catalog, timeout: 5 * time.Second}
}

// AsDemo marks replies as canned (the mock model), so the UI can say so.
func (a *Assistant) AsDemo() *Assistant { a.demo = true; return a }

// maxPathLen bounds the route the client reports.
const maxPathLen = 300

// Chat answers the conversation. path is the app route the user is on
// (e.g. /movies/27205), used to tell the model what they are looking at;
// locale is the UI language to answer in ("en", the default, or "tr").
func (a *Assistant) Chat(ctx context.Context, userID string, history []domain.ChatMessage, path, locale string) (ChatResponse, error) {
	if err := validateHistory(history); err != nil {
		return ChatResponse{}, err
	}
	switch locale {
	case "":
		locale = domain.LocaleEN
	case domain.LocaleEN, domain.LocaleTR:
	default:
		return ChatResponse{}, fmt.Errorf("%w: locale must be %q or %q", domain.ErrInvalidInput, domain.LocaleEN, domain.LocaleTR)
	}
	if len(path) > maxPathLen {
		return ChatResponse{}, fmt.Errorf("%w: path is too long", domain.ErrInvalidInput)
	}
	page, pageIDs := a.pageContext(ctx, path)
	r, err := a.model.Reply(ctx, domain.ChatRequest{UserID: userID, History: history, Page: page, PageMovieIDs: pageIDs, Locale: locale})
	if err != nil {
		return ChatResponse{}, err
	}

	ids := uniquePositive(r.MovieIDs, domain.MaxChatMovies)
	return ChatResponse{Message: r.Message, MovieIDs: ids, Movies: a.resolve(ctx, ids), Demo: a.demo}, nil
}

// pageContext describes the page at path for the model, from our own data:
// the client only says where it is, never what is there. Unknown or failing
// pages give no context rather than an error.
func (a *Assistant) pageContext(ctx context.Context, path string) (string, []int) {
	u, err := url.Parse(path)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") { // app routes only
		return "", nil
	}
	c, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	seg := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch {
	case u.Path == "/":
		return "The home page (trending movies).", nil
	case len(seg) == 2 && seg[0] == "movies":
		id, err := strconv.Atoi(seg[1])
		if err != nil {
			return "", nil
		}
		m, err := a.catalog.GetMovieDetails(c, id)
		if err != nil {
			return "", nil
		}
		return describeMovie(m), []int{m.ID}
	case len(seg) == 2 && seg[0] == "person":
		id, err := strconv.Atoi(seg[1])
		if err != nil {
			return "", nil
		}
		p, err := a.catalog.GetPerson(c, id)
		if err != nil {
			return "", nil
		}
		return describePerson(p)
	case u.Path == "/search":
		if q := oneLine(u.Query().Get("q"), 100); q != "" {
			return fmt.Sprintf("Search results for %q.", q), nil
		}
		return "The search page.", nil
	case u.Path == "/my-list":
		return "Their watchlist (My List).", nil
	case u.Path == "/discover":
		return "The Discover feed of trailers.", nil
	case u.Path == "/profile":
		return "Their profile page.", nil
	}
	return "", nil
}

func describeMovie(m domain.Movie) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A movie page: %q", m.Title)
	if len(m.ReleaseDate) >= 4 {
		fmt.Fprintf(&b, " (%s)", m.ReleaseDate[:4])
	}
	fmt.Fprintf(&b, ", id %d.", m.ID)
	if len(m.Genres) > 0 {
		names := make([]string, len(m.Genres))
		for i, g := range m.Genres {
			names[i] = g.Name
		}
		fmt.Fprintf(&b, " Genres: %s.", strings.Join(names, ", "))
	}
	if m.Director != "" {
		fmt.Fprintf(&b, " Director: %s.", m.Director)
	}
	if m.IMDbRating > 0 {
		fmt.Fprintf(&b, " IMDb %.1f.", m.IMDbRating)
	}
	if m.Overview != "" {
		fmt.Fprintf(&b, " Plot: %s", oneLine(m.Overview, 400))
	}
	return b.String()
}

func describePerson(p domain.Person) (string, []int) {
	var b strings.Builder
	fmt.Fprintf(&b, "A person page: %s", p.Name)
	if p.KnownForDepartment != "" {
		fmt.Fprintf(&b, " (%s)", p.KnownForDepartment)
	}
	b.WriteString(". Their movies:")
	var ids []int
	for _, c := range p.Credits[:min(len(p.Credits), 10)] {
		year := ""
		if len(c.ReleaseDate) >= 4 {
			year = c.ReleaseDate[:4] + ", "
		}
		fmt.Fprintf(&b, " %q (%sid %d);", c.Title, year, c.ID)
		ids = append(ids, c.ID)
	}
	return b.String(), ids
}

// oneLine flattens whitespace (no smuggled line breaks) and truncates.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// resolve looks the movies up through the catalog's caches, in parallel,
// keeping the model's order. Ids that fail (a model can invent one) are
// dropped rather than failing the reply.
func (a *Assistant) resolve(ctx context.Context, ids []int) []domain.Movie {
	found := make([]*domain.Movie, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, cancel := context.WithTimeout(ctx, a.timeout)
			defer cancel()
			m, err := a.catalog.GetMovieDetails(c, id)
			if err != nil {
				a.catalog.log.Debug("chat: skipping movie", "id", id, "error", err)
				return
			}
			s := summary(m)
			found[i] = &s
		}()
	}
	wg.Wait()
	movies := make([]domain.Movie, 0, len(ids))
	for _, m := range found {
		if m != nil {
			movies = append(movies, *m)
		}
	}
	return movies
}

// summary keeps what a card renders; cast, trailer and OMDb facts stay on
// the details endpoint.
func summary(m domain.Movie) domain.Movie {
	return domain.Movie{
		ID: m.ID, Title: m.Title, Overview: m.Overview, PosterPath: m.PosterPath, BackdropPath: m.BackdropPath,
		ReleaseDate: m.ReleaseDate, VoteAverage: m.VoteAverage, VoteCount: m.VoteCount,
		IMDbRating: m.IMDbRating, IMDbVotes: m.IMDbVotes,
	}
}

func validateHistory(h []domain.ChatMessage) error {
	switch {
	case len(h) == 0:
		return fmt.Errorf("%w: messages are required", domain.ErrInvalidInput)
	case len(h) > domain.MaxChatMessages:
		return fmt.Errorf("%w: at most %d messages", domain.ErrInvalidInput, domain.MaxChatMessages)
	case h[len(h)-1].Role != domain.RoleUser:
		return fmt.Errorf("%w: the last message must be from the user", domain.ErrInvalidInput)
	}
	for _, m := range h {
		if m.Role != domain.RoleUser && m.Role != domain.RoleAssistant {
			return fmt.Errorf("%w: role must be %q or %q", domain.ErrInvalidInput, domain.RoleUser, domain.RoleAssistant)
		}
		if strings.TrimSpace(m.Content) == "" || utf8.RuneCountInString(m.Content) > domain.MaxChatMessageLen {
			return fmt.Errorf("%w: each message must be 1..%d characters", domain.ErrInvalidInput, domain.MaxChatMessageLen)
		}
	}
	return nil
}

func uniquePositive(ids []int, limit int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, id := range ids {
		if id > 0 && !seen[id] && len(out) < limit {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
