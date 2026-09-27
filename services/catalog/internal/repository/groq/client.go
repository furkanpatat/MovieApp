// Package groq is the Cinematic Concierge: a domain.ChatModel on Groq's
// OpenAI-compatible Chat Completions API. The model finds real TMDB
// ids with a search tool backed by our catalog, then answers in JSON mode.
package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// Wire types (only the fields we use).

type message struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content"` // null on a tool-calling assistant turn
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type tool struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// responseFormat {"type":"json_object"} is JSON mode: the reply is valid
// JSON. Groq cannot combine it with tools, so it is only set on the answer.
type responseFormat struct {
	Type string `json:"type"`
}

type completionRequest struct {
	Model               string          `json:"model"`
	Messages            []message       `json:"messages"`
	Tools               []tool          `json:"tools,omitempty"`
	ToolChoice          string          `json:"tool_choice,omitempty"`
	ResponseFormat      *responseFormat `json:"response_format,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	User                string          `json:"user,omitempty"`
}

type completionResponse struct {
	Choices []struct {
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

type apiError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
		// FailedGeneration is what the model wrote when JSON mode rejected it
		// (code json_validate_failed): often usable JSON with stray text.
		FailedGeneration string `json:"failed_generation"`
	} `json:"error"`
}

// jsonValidateError: JSON mode rejected the model's output (HTTP 400, code
// json_validate_failed). Generation holds that output.
type jsonValidateError struct{ Generation string }

func (e *jsonValidateError) Error() string { return "json_validate_failed" }

// errToolUseFailed: Groq rejected a malformed tool call the model generated
// (HTTP 400, code tool_use_failed). Small models do this now and then.
var errToolUseFailed = errors.New("tool_use_failed")

// client posts to /chat/completions.
type client struct {
	baseURL, apiKey string
	http            *http.Client
}

func (c *client) complete(ctx context.Context, req completionRequest) (message, string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return message{}, "", err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return message{}, "", err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(hreq)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return message{}, "", fmt.Errorf("%w: groq: %v", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return message{}, "", fmt.Errorf("%w: groq: read: %v", domain.ErrUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		var e apiError
		_ = json.Unmarshal(raw, &e)
		switch e.Error.Code {
		case "tool_use_failed":
			return message{}, "", fmt.Errorf("%w: %w", domain.ErrUnavailable, errToolUseFailed)
		case "json_validate_failed":
			return message{}, "", fmt.Errorf("%w: %w", domain.ErrUnavailable, &jsonValidateError{Generation: e.Error.FailedGeneration})
		}
		// 401 bad key, 429 rate limit, 5xx outage: all "unavailable" to our
		// caller; the log gets Groq's reason (never the key).
		return message{}, "", fmt.Errorf("%w: groq status %d: %s", domain.ErrUnavailable, resp.StatusCode, truncate(e.Error.Message, 300))
	}
	var out completionResponse
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return message{}, "", fmt.Errorf("%w: groq: unexpected response", domain.ErrUnavailable)
	}
	return out.Choices[0].Message, out.Choices[0].FinishReason, nil
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func ptr(s string) *string { return &s }

// defaultTimeout bounds one completion call.
const defaultTimeout = 45 * time.Second

func trimLower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
