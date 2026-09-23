// Package omdb fetches IMDb ratings from the OMDb API (omdbapi.com).
package omdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

type Config struct {
	BaseURL string        // default https://www.omdbapi.com/
	APIKey  string        // required
	Timeout time.Duration // per request (default 3s)
	// Cooldown: after a failure (outage, daily limit, bad key) no request is
	// made for this long; callers get ErrUnavailable at once (default 10m).
	Cooldown   time.Duration
	HTTPClient *http.Client
}

// Client implements domain.RatingProvider.
type Client struct {
	baseURL  string
	apiKey   string
	http     *http.Client
	cooldown time.Duration
	now      func() time.Time

	mu        sync.Mutex
	pausedTil time.Time
}

var _ domain.RatingProvider = (*Client)(nil)

func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://www.omdbapi.com/"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 10 * time.Minute
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}
	return &Client{baseURL: cfg.BaseURL, apiKey: cfg.APIKey, http: hc, cooldown: cfg.Cooldown, now: time.Now}
}

type response struct {
	Response   string `json:"Response"`
	Error      string `json:"Error"`
	IMDbRating string `json:"imdbRating"`
	IMDbVotes  string `json:"imdbVotes"`
	Rated      string `json:"Rated"`
	Metascore  string `json:"Metascore"`
	Awards     string `json:"Awards"`
	Director   string `json:"Director"`
	Writer     string `json:"Writer"`
	BoxOffice  string `json:"BoxOffice"`
	Country    string `json:"Country"`
	Language   string `json:"Language"`
	Ratings    []struct {
		Source string `json:"Source"`
		Value  string `json:"Value"`
	} `json:"Ratings"`
}

// known maps OMDb's "N/A" (and blanks) to "".
func known(s string) string {
	s = strings.TrimSpace(s)
	if s == "N/A" {
		return ""
	}
	return s
}

func (c *Client) GetIMDbRating(ctx context.Context, imdbID string) (domain.IMDbRating, error) {
	if !strings.HasPrefix(imdbID, "tt") {
		return domain.IMDbRating{}, domain.ErrNotFound
	}
	c.mu.Lock()
	paused := c.now().Before(c.pausedTil)
	c.mu.Unlock()
	if paused {
		return domain.IMDbRating{}, fmt.Errorf("%w: omdb cooling down", domain.ErrUnavailable)
	}

	r, err := c.fetch(ctx, imdbID)
	if errors.Is(err, domain.ErrUnavailable) && ctx.Err() == nil {
		c.mu.Lock()
		c.pausedTil = c.now().Add(c.cooldown)
		c.mu.Unlock()
	}
	return r, err
}

func (c *Client) fetch(ctx context.Context, imdbID string) (domain.IMDbRating, error) {
	u := c.baseURL + "?" + url.Values{"apikey": {c.apiKey}, "i": {imdbID}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return domain.IMDbRating{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) { // never surface the URL: it carries the key
			err = ue.Err
		}
		return domain.IMDbRating{}, fmt.Errorf("%w: omdb: %v", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()

	var body response
	derr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body)
	switch {
	case resp.StatusCode != http.StatusOK:
		// 401 is both "Invalid API key!" and "Request limit reached!".
		return domain.IMDbRating{}, fmt.Errorf("%w: omdb status %d %s", domain.ErrUnavailable, resp.StatusCode, body.Error)
	case derr != nil:
		return domain.IMDbRating{}, fmt.Errorf("%w: omdb decode: %v", domain.ErrUnavailable, derr)
	case body.Response != "True":
		if strings.Contains(strings.ToLower(body.Error), "not found") || strings.Contains(body.Error, "Incorrect IMDb ID") {
			return domain.IMDbRating{}, domain.ErrNotFound
		}
		return domain.IMDbRating{}, fmt.Errorf("%w: omdb: %s", domain.ErrUnavailable, body.Error)
	}

	var out domain.IMDbRating
	if f, err := strconv.ParseFloat(body.IMDbRating, 64); err == nil { // "N/A" -> no rating yet
		out.Rating = f
	}
	if n, err := strconv.Atoi(strings.ReplaceAll(body.IMDbVotes, ",", "")); err == nil {
		out.Votes = n
	}
	if n, err := strconv.Atoi(body.Metascore); err == nil {
		out.Metascore = n
	}
	for _, r := range body.Ratings {
		if r.Source == "Rotten Tomatoes" {
			out.RottenTomatoes = known(r.Value)
		}
	}
	out.Rated = known(body.Rated)
	if out.Rated == "Not Rated" || out.Rated == "Unrated" {
		out.Rated = ""
	}
	out.Awards, out.Director, out.Writer = known(body.Awards), known(body.Director), known(body.Writer)
	out.BoxOffice, out.Country, out.Language = known(body.BoxOffice), known(body.Country), known(body.Language)
	return out, nil
}
