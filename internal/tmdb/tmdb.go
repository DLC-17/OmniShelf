// Package tmdb provides a rate-limited client for the TMDB v3 API.
//
// All calls run server-side only (the API key never reaches the browser),
// are limited to ~4 requests/second, and retry with exponential backoff on
// HTTP 429 (3 attempts max, spec E2).
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

const defaultBaseURL = "https://api.themoviedb.org/3"

// maxAttempts is the total number of tries for a request that keeps
// returning 429 (spec E2: exponential backoff, 3 attempts).
const maxAttempts = 3

// Client talks to the TMDB v3 API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter
	// backoffBase is the first retry delay; doubled each attempt.
	// Configurable so tests do not sleep for real seconds.
	backoffBase time.Duration
}

// Option customizes a Client.
type Option func(*Client)

// WithBaseURL overrides the TMDB API base URL (used by tests to point at an
// httptest.Server).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

// WithHTTPClient overrides the shared HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithRateLimit overrides the request rate limiter.
func WithRateLimit(l *rate.Limiter) Option {
	return func(c *Client) { c.limiter = l }
}

// WithBackoffBase overrides the initial 429 backoff delay (tests only).
func WithBackoffBase(d time.Duration) Option {
	return func(c *Client) { c.backoffBase = d }
}

// New returns a TMDB client. The zero-config client uses the public TMDB v3
// endpoint, a shared 15s-timeout HTTP client, and a ~4 req/s rate limit.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:      apiKey,
		baseURL:     defaultBaseURL,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		limiter:     rate.NewLimiter(rate.Limit(4), 1),
		backoffBase: 500 * time.Millisecond,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// StatusError is returned when TMDB responds with an unexpected HTTP status.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("tmdb: unexpected status %d: %s", e.StatusCode, e.Body)
}

// SearchResult is one show entry from a TMDB TV search.
type SearchResult struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	FirstAirDate string `json:"first_air_date"`
	PosterPath   string `json:"poster_path"`
}

// SearchResponse is the TMDB TV search payload.
type SearchResponse struct {
	Page         int            `json:"page"`
	Results      []SearchResult `json:"results"`
	TotalPages   int            `json:"total_pages"`
	TotalResults int            `json:"total_results"`
}

// SeasonSummary is a season entry inside a show's details.
type SeasonSummary struct {
	ID           int    `json:"id"`
	SeasonNumber int    `json:"season_number"`
	Name         string `json:"name"`
	EpisodeCount int    `json:"episode_count"`
	AirDate      string `json:"air_date"`
}

// Show is the TMDB TV show detail payload.
type Show struct {
	ID           int             `json:"id"`
	Name         string          `json:"name"`
	Overview     string          `json:"overview"`
	Status       string          `json:"status"` // "Returning Series", "Ended", ...
	FirstAirDate string          `json:"first_air_date"`
	PosterPath   string          `json:"poster_path"`
	Seasons      []SeasonSummary `json:"seasons"`
}

// Episode is one episode inside a season detail payload.
type Episode struct {
	ID            int    `json:"id"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	AirDate       string `json:"air_date"` // "YYYY-MM-DD" or "" when unannounced
}

// Season is the TMDB season detail payload (episode listing with air dates).
type Season struct {
	ID           int       `json:"id"`
	SeasonNumber int       `json:"season_number"`
	Name         string    `json:"name"`
	AirDate      string    `json:"air_date"`
	Episodes     []Episode `json:"episodes"`
}

// MovieResult is one movie entry from a TMDB movie search or recommendation
// list. Movies use title/release_date where TV uses name/first_air_date.
type MovieResult struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Overview    string `json:"overview"`
	ReleaseDate string `json:"release_date"`
	PosterPath  string `json:"poster_path"`
}

// MovieSearchResponse is the TMDB movie search / recommendations payload.
type MovieSearchResponse struct {
	Page         int           `json:"page"`
	Results      []MovieResult `json:"results"`
	TotalPages   int           `json:"total_pages"`
	TotalResults int           `json:"total_results"`
}

// Movie is the TMDB movie detail payload.
type Movie struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Overview    string `json:"overview"`
	Status      string `json:"status"` // "Released", "Post Production", ...
	ReleaseDate string `json:"release_date"`
	PosterPath  string `json:"poster_path"`
}

// SearchMovie searches TMDB movies by title.
func (c *Client) SearchMovie(ctx context.Context, query string) (*MovieSearchResponse, error) {
	var out MovieSearchResponse
	q := url.Values{"query": {query}}
	if err := c.get(ctx, "/search/movie", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMovie fetches a movie's details (title, overview, poster, release date).
func (c *Client) GetMovie(ctx context.Context, id int) (*Movie, error) {
	var out Movie
	if err := c.get(ctx, fmt.Sprintf("/movie/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MovieRecommendations returns TMDB's "recommended" movies for a given movie.
func (c *Client) MovieRecommendations(ctx context.Context, movieID int) (*MovieSearchResponse, error) {
	var out MovieSearchResponse
	if err := c.get(ctx, fmt.Sprintf("/movie/%d/recommendations", movieID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SearchTV searches TMDB TV shows by name.
func (c *Client) SearchTV(ctx context.Context, query string) (*SearchResponse, error) {
	var out SearchResponse
	q := url.Values{"query": {query}}
	if err := c.get(ctx, "/search/tv", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetShow fetches a show's details, including its seasons list, poster path
// and airing status.
func (c *Client) GetShow(ctx context.Context, id int) (*Show, error) {
	var out Show
	if err := c.get(ctx, fmt.Sprintf("/tv/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Recommendations returns TMDB's "recommended" TV shows for a given show,
// in the same shape as a search result.
func (c *Client) Recommendations(ctx context.Context, showID int) (*SearchResponse, error) {
	var out SearchResponse
	if err := c.get(ctx, fmt.Sprintf("/tv/%d/recommendations", showID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSeason fetches one season of a show with its full episode list
// (including air dates).
func (c *Client) GetSeason(ctx context.Context, showID, seasonNum int) (*Season, error) {
	var out Season
	if err := c.get(ctx, fmt.Sprintf("/tv/%d/season/%d", showID, seasonNum), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// get performs a rate-limited GET with 429 backoff and decodes JSON into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	if query == nil {
		query = url.Values{}
	}
	query.Set("api_key", c.apiKey)
	reqURL := c.baseURL + path + "?" + query.Encode()

	delay := c.backoffBase
	for attempt := 1; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("tmdb: rate limiter wait: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return fmt.Errorf("tmdb: build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("tmdb: request %s: %w", path, err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if attempt >= maxAttempts {
				return &StatusError{StatusCode: resp.StatusCode, Body: string(body)}
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return fmt.Errorf("tmdb: backoff interrupted: %w", ctx.Err())
			}
			delay *= 2
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return &StatusError{StatusCode: resp.StatusCode, Body: string(body)}
		}

		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("tmdb: decode %s: %w", path, err)
		}
		return nil
	}
}
