package domain

import "context"

// Chat roles, as LLM APIs name them.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

const (
	MaxChatMessages   = 20   // history sent per request (the client trims older turns)
	MaxChatMessageLen = 2000 // runes per message
	MaxChatMovies     = 8    // movies rendered per reply
)

// ChatMessage is one turn of the conversation. The server is stateless: the
// client sends the recent history with every request.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is everything the model gets for one reply.
type ChatRequest struct {
	UserID  string
	History []ChatMessage
	// Page describes what the user is looking at (built by the server from
	// the route, never taken verbatim from the client). Empty when unknown.
	Page string
	// PageMovieIDs are movies on that page, which the model may recommend
	// without searching for them first.
	PageMovieIDs []int
	// Locale is the UI language the reply is written in: LocaleEN or LocaleTR.
	Locale string
}

// UI languages the assistant answers in.
const (
	LocaleEN = "en"
	LocaleTR = "tr"
)

// ChatReply is what the model returns: text (markdown), plus TMDB ids of the
// movies it recommends (rendered as cards by the client).
type ChatReply struct {
	Message  string `json:"message"`
	MovieIDs []int  `json:"movie_ids"`
}

// MovieSearcher is the catalog search a model uses to find real TMDB ids.
type MovieSearcher interface {
	SearchMovies(ctx context.Context, query string, page int) (MoviePage, error)
}

// ChatModel produces the assistant's next reply.
type ChatModel interface {
	Reply(ctx context.Context, req ChatRequest) (ChatReply, error)
}
