package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// Keyword picks: what the assistant answers when the language model can't
// (rate limited, down, or a reply it couldn't use). No AI: the last message
// is matched against genre and mood words (English and Turkish), a
// "like <movie>" (or a bare title) finds that movie's main genre, and
// anything else gets what's trending. All from the catalog's own caches.

// keywordPicks is how many movies a keyword reply recommends.
const keywordPicks = 5

type genreWords struct {
	id     int
	en, tr string   // the genre's name in replies
	words  []string // lowercase stems, matched as word prefixes
}

// Order matters: the first genre found in the message wins, so the more
// specific ones come first ("sci-fi action" is sci-fi, not action).
var genreKeywords = []genreWords{
	{878, "science fiction", "bilim kurgu", []string{"sci-fi", "scifi", "sci fi", "science fiction", "space", "alien", "robot", "future", "cyberpunk", "bilim kurgu", "bilimkurgu", "uzay", "uzaylı", "gelecek", "robot"}},
	{27, "horror", "korku", []string{"horror", "scary", "creepy", "terrifying", "haunted", "zombie", "korku", "korkunç", "ürkütücü", "zombi"}},
	{35, "comedy", "komedi", []string{"funny", "comedy", "comedies", "laugh", "hilarious", "lighthearted", "komik", "komedi", "gül", "eğlenceli"}},
	{16, "animation", "animasyon", []string{"animated", "animation", "anime", "cartoon", "pixar", "animasyon", "çizgi"}},
	{10749, "romance", "romantik", []string{"romance", "romantic", "love story", "date night", "romantik", "aşk"}},
	{53, "thriller", "gerilim", []string{"thriller", "suspense", "tense", "twist", "gerilim", "gerilimli", "heyecanlı"}},
	{80, "crime", "suç", []string{"crime", "heist", "gangster", "mafia", "detective", "suç", "soygun", "mafya", "dedektif"}},
	{9648, "mystery", "gizem", []string{"mystery", "whodunit", "puzzle", "mind-bending", "mind bending", "gizem", "gizemli"}},
	{14, "fantasy", "fantastik", []string{"fantasy", "magic", "dragon", "wizard", "fantastik", "büyü", "ejderha"}},
	{12, "adventure", "macera", []string{"adventure", "quest", "epic", "macera", "serüven"}},
	{10751, "family", "aile", []string{"family", "kids", "children", "aile", "çocuk"}},
	{99, "documentary", "belgesel", []string{"documentary", "documentaries", "true story", "belgesel", "gerçek hikaye"}},
	{10752, "war", "savaş", []string{"war", "battle", "soldier", "savaş", "asker"}},
	{36, "history", "tarih", []string{"history", "historical", "period piece", "tarih", "tarihi"}},
	{37, "western", "western", []string{"western", "cowboy", "kovboy"}},
	{10402, "music", "müzik", []string{"music", "musical", "concert", "müzik", "müzikal"}},
	{28, "action", "aksiyon", []string{"action", "fight", "explosion", "adrenaline", "high-octane", "aksiyon", "dövüş", "patlama", "adrenalin"}},
	{18, "drama", "dram", []string{"drama", "emotional", "moving", "tearjerker", "dram", "duygusal", "hüzünlü"}},
}

// likeRE finds "like Heat", "similar to Heat", "Heat gibi", "Heat'e benzer".
var likeRE = regexp.MustCompile(`(?i)(?:like|similar to|such as)\s+(.{2,60})$|^(.{2,60}?)\s*(?:'[a-zçğıöşü]+)?\s+(?:gibi|benzeri|tarzı|benzer)\b`)

// fillerRE strips request words around a bare title ("recommend me Heat").
var fillerRE = regexp.MustCompile(`(?i)\b(recommend|suggest|me|a|an|the|some|movie|movies|film|films|please|öner|önerir misin|film|filmi|filmler|bana|bir)\b`)

// normalize lowercases for matching (Turkish "İ" lowercases to "i̇" in Go:
// drop the combining dot) and collapses whitespace.
func normalize(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "i̇", "i")
	return strings.Join(strings.Fields(s), " ")
}

// matchGenre is the first genre whose word starts a word of msg.
func matchGenre(msg string) (genreWords, bool) {
	text := " " + normalize(msg)
	for _, g := range genreKeywords {
		for _, w := range g.words {
			if strings.Contains(text, " "+w) {
				return g, true
			}
		}
	}
	return genreWords{}, false
}

// keywordReply answers without the model: see the file comment.
func (a *Assistant) keywordReply(ctx context.Context, msg, locale string) domain.ChatReply {
	tr := locale == domain.LocaleTR
	lead := "*The AI concierge is busy right now, so here are picks based on your keywords.*"
	if tr {
		lead = "*AI asistan şu anda yoğun, o yüzden anahtar kelimelerine göre seçtiklerim bunlar.*"
	}

	// "like <movie>" (or a bare title): movies sharing its main genre.
	if title, explicit := likedTitle(msg); title != "" {
		if seed, ok := a.findMovie(ctx, title, explicit); ok && len(seed.Genres) > 0 {
			g := seed.Genres[0]
			if picks := a.discoverPicks(ctx, g.ID, seed.ID); len(picks) > 0 {
				intro := fmt.Sprintf("If you liked **%s**, try these:", seed.Title)
				if tr {
					intro = fmt.Sprintf("**%s** sevdiysen bunları dene:", seed.Title)
				}
				return reply(lead, intro, picks)
			}
		}
	}
	if g, ok := matchGenre(msg); ok {
		if picks := a.discoverPicks(ctx, g.id, 0); len(picks) > 0 {
			intro := fmt.Sprintf("Popular **%s** picks:", g.en)
			if tr {
				intro = fmt.Sprintf("Popüler **%s** seçkileri:", g.tr)
			}
			return reply(lead, intro, picks)
		}
	}
	// Nothing recognizable: what's trending.
	intro := "Here's what everyone is watching right now:"
	if tr {
		intro = "Şu an herkesin izlediği filmler:"
	}
	var picks []domain.Movie
	if p, err := a.catalog.GetPopularMovies(ctx, 1); err == nil {
		picks = topWithPosters(p.Results, 0)
	}
	return reply(lead, intro, picks)
}

// likedTitle is the movie a message compares to ("like Heat": explicit), or
// the whole message as a possible title when it has no genre words and is
// short enough to be one.
func likedTitle(msg string) (title string, explicit bool) {
	msg = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(msg), "?!.…"))
	if m := likeRE.FindStringSubmatch(msg); m != nil {
		for _, g := range m[1:] {
			if t := strings.Trim(strings.TrimSpace(g), `"'“”`); t != "" {
				return t, true
			}
		}
	}
	if _, isGenre := matchGenre(msg); isGenre {
		return "", false
	}
	bare := strings.Join(strings.Fields(fillerRE.ReplaceAllString(msg, " ")), " ")
	if n := len(strings.Fields(bare)); n >= 1 && n <= 5 {
		return bare, false
	}
	return "", false
}

// findMovie is the best search hit for title, with its details (genres).
// A bare (not "like ...") title must really match: "what should I watch
// tonight" is a question, not the movie "Tonight".
func (a *Assistant) findMovie(ctx context.Context, title string, explicit bool) (domain.Movie, bool) {
	p, err := a.catalog.SearchMovies(ctx, title, 1)
	if err != nil || len(p.Results) == 0 {
		return domain.Movie{}, false
	}
	hit := p.Results[0]
	if !explicit && normalize(hit.Title) != normalize(title) {
		return domain.Movie{}, false
	}
	m, err := a.catalog.GetMovieDetails(ctx, hit.ID)
	return m, err == nil
}

// discoverPicks is the top of the genre's discovery feed, minus skipID.
func (a *Assistant) discoverPicks(ctx context.Context, genreID, skipID int) []domain.Movie {
	p, err := a.catalog.DiscoverMovies(ctx, genreID, 1)
	if err != nil {
		return nil
	}
	return topWithPosters(p.Results, skipID)
}

func topWithPosters(movies []domain.Movie, skipID int) []domain.Movie {
	out := make([]domain.Movie, 0, keywordPicks)
	for _, m := range movies {
		if m.ID != skipID && m.PosterPath != "" && len(out) < keywordPicks {
			out = append(out, m)
		}
	}
	return out
}

// reply renders the picks as the assistant's Markdown, cards to follow.
func reply(lead, intro string, picks []domain.Movie) domain.ChatReply {
	var b strings.Builder
	b.WriteString(lead + "\n\n" + intro + "\n")
	ids := make([]int, 0, len(picks))
	for _, m := range picks {
		year := ""
		if len(m.ReleaseDate) >= 4 {
			year = " (" + m.ReleaseDate[:4] + ")"
		}
		fmt.Fprintf(&b, "\n- **%s**%s", m.Title, year)
		ids = append(ids, m.ID)
	}
	return domain.ChatReply{Message: b.String(), MovieIDs: ids}
}
