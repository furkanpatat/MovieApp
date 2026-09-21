// Package tmdb is the TMDB HTTP client, guarded by a circuit breaker.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

const maxBody = 5 << 20 // 5 MiB

// Config configures the client.
type Config struct {
	BaseURL         string
	APIKey          string        // v3 api_key, or a v4 read token (starts with "eyJ")
	Timeout         time.Duration // per-request timeout (default 5s)
	BreakerFailures uint32        // consecutive failures that open the circuit (default 5)
	BreakerOpenFor  time.Duration // open duration before a half-open probe (default 30s)
	HTTPClient      *http.Client  // optional override
	Logger          *slog.Logger
}

// Client implements domain.MovieProvider.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	cb      *gobreaker.CircuitBreaker[struct{}]
	log     *slog.Logger
}

var _ domain.MovieProvider = (*Client)(nil)

func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.BreakerFailures == 0 {
		cfg.BreakerFailures = 5
	}
	if cfg.BreakerOpenFor <= 0 {
		cfg.BreakerOpenFor = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}
	log := cfg.Logger.With("component", "tmdb")

	cb := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        "tmdb",
		MaxRequests: 1, // one probe while half-open
		Timeout:     cfg.BreakerOpenFor,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= cfg.BreakerFailures
		},
		IsSuccessful: isSuccessful,
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker state change", "breaker", name, "from", from.String(), "to", to.String())
		},
	})

	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		http:    hc,
		cb:      cb,
		log:     log,
	}
}

// BreakerState exposes the breaker state for health checks and metrics.
func (c *Client) BreakerState() gobreaker.State { return c.cb.State() }

func (c *Client) GetPopularMovies(ctx context.Context, page int) (domain.MoviePage, error) {
	var w popularResponse
	q := url.Values{"page": {strconv.Itoa(page)}}
	if err := c.get(ctx, "/movie/popular", q, &w); err != nil {
		return domain.MoviePage{}, err
	}
	return w.toDomain(), nil
}

func (c *Client) GetMovieDetails(ctx context.Context, id int) (domain.Movie, error) {
	var w movieWire
	if err := c.get(ctx, "/movie/"+strconv.Itoa(id), nil, &w); err != nil {
		return domain.Movie{}, err
	}
	return w.toDomain(), nil
}

// get performs the request inside the circuit breaker and decodes JSON into out.
func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	_, err := c.cb.Execute(func() (struct{}, error) {
		return struct{}{}, c.do(ctx, path, q, out)
	})
	switch {
	case errors.Is(err, gobreaker.ErrOpenState), errors.Is(err, gobreaker.ErrTooManyRequests):
		return fmt.Errorf("%w: circuit breaker %s", domain.ErrUnavailable, c.cb.State())
	case err != nil:
		var ig ignoredError
		if errors.As(err, &ig) {
			return ig.err
		}
		return err
	}
	return nil
}

func (c *Client) do(ctx context.Context, path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	bearer := strings.HasPrefix(c.apiKey, "eyJ")
	if !bearer {
		q.Set("api_key", c.apiKey)
	}
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ignoredError{err}
	}
	req.Header.Set("Accept", "application/json")
	if bearer {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil { // the caller gave up; not TMDB's fault
			return ignoredError{ctx.Err()}
		}
		// Never log the URL: it carries the API key.
		return fmt.Errorf("%w: %s", domain.ErrUnavailable, redact(err))
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return ignoredError{domain.ErrNotFound}
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return fmt.Errorf("%w: tmdb status %d", domain.ErrUnavailable, resp.StatusCode)
	case resp.StatusCode >= 400:
		// Our request is wrong (bad key, bad params); retrying won't help but
		// it also says nothing about TMDB's health.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
		return ignoredError{fmt.Errorf("%w: tmdb status %d", domain.ErrUnavailable, resp.StatusCode)}
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return fmt.Errorf("%w: decode response: %v", domain.ErrUnavailable, err)
	}
	return nil
}

// ignoredError marks outcomes that must not count against the breaker.
type ignoredError struct{ err error }

func (e ignoredError) Error() string { return e.err.Error() }
func (e ignoredError) Unwrap() error { return e.err }

func isSuccessful(err error) bool {
	var ig ignoredError
	return err == nil || errors.As(err, &ig)
}

// redact strips the URL (and therefore the api_key) from net/http errors.
func redact(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}
