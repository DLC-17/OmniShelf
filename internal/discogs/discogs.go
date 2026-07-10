// Package discogs provides a client for the Discogs database
// (https://www.discogs.com), used to resolve an album's UPC/EAN barcode to its
// artist, title, year and cover art. Discogs is barcode-searchable through its
// /database/search endpoint; every request is authenticated with a personal
// access token and carries a descriptive User-Agent per Discogs policy.
//
// Missing barcodes yield ErrNotFound; a missing token makes the client
// unconfigured so the music module degrades to a clear error instead of
// failing startup.
package discogs

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
	"time"
)

const (
	defaultBaseURL = "https://api.discogs.com"
	userAgent      = "OmniShelf/1.0 (+https://github.com/davidlc1229/omnishelf)"
)

// Sentinel errors callers translate into the API envelope.
var (
	// ErrNotFound means Discogs has no release for the barcode.
	ErrNotFound = errors.New("discogs: no release for barcode")
	// ErrUnconfigured means no access token was supplied at startup.
	ErrUnconfigured = errors.New("discogs: not configured")
)

// NotFoundError wraps ErrNotFound with the barcode that missed, so callers can
// echo it in the API error envelope.
type NotFoundError struct {
	Barcode string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("discogs: no release for barcode %s", e.Barcode)
}

// Unwrap lets errors.Is(err, ErrNotFound) match.
func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// Client talks to the Discogs API.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// Option customizes a Client.
type Option func(*Client)

// WithBaseURL overrides the Discogs base URL (used by tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient overrides the shared HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// New returns a Discogs client. When token is empty the client is considered
// unconfigured and LookupByBarcode returns ErrUnconfigured.
func New(token string, opts ...Option) *Client {
	c := &Client{
		token:      token,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Configured reports whether an access token was supplied.
func (c *Client) Configured() bool {
	return c.token != ""
}

// Release is the album metadata Discogs returns for one barcode. CoverURL is a
// remote image URL that must be downloaded through internal/images — never
// hotlinked from the frontend.
type Release struct {
	DiscogsID int
	Artist    string
	Title     string
	Year      int
	CoverURL  string
	Barcode   string
}

// searchResponse models the /database/search payload subset we use. Discogs
// formats a search hit's title as "Artist - Release Title"; year is a string.
type searchResponse struct {
	Results []struct {
		ID         int    `json:"id"`
		Title      string `json:"title"`
		Year       string `json:"year"`
		CoverImage string `json:"cover_image"`
		Thumb      string `json:"thumb"`
	} `json:"results"`
}

// LookupByBarcode searches Discogs for a release matching the barcode and
// returns the first hit. A barcode with no matches yields *NotFoundError.
func (c *Client) LookupByBarcode(ctx context.Context, barcode string) (*Release, error) {
	if !c.Configured() {
		return nil, ErrUnconfigured
	}

	q := url.Values{
		"barcode":  {barcode},
		"type":     {"release"},
		"per_page": {"5"},
	}
	endpoint := c.baseURL + "/database/search?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("discogs: build request: %w", err)
	}
	req.Header.Set("Authorization", "Discogs token="+c.token)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discogs: request search: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discogs: search returned status %d: %s", resp.StatusCode, string(body))
	}

	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("discogs: decode search: %w", err)
	}
	if len(sr.Results) == 0 {
		return nil, &NotFoundError{Barcode: barcode}
	}

	r := sr.Results[0]
	artist, title := splitArtistTitle(r.Title)
	cover := r.CoverImage
	if cover == "" {
		cover = r.Thumb
	}
	rel := &Release{
		DiscogsID: r.ID,
		Artist:    artist,
		Title:     title,
		Year:      parseYear(r.Year),
		CoverURL:  cover,
		Barcode:   barcode,
	}
	return rel, nil
}

// splitArtistTitle splits a Discogs "Artist - Release Title" search title on
// its first " - ". A title with no separator is returned whole as the release
// title with an empty artist.
func splitArtistTitle(s string) (artist, title string) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, " - "); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+3:])
	}
	return "", s
}

// parseYear parses Discogs' string year; a non-numeric or empty value is 0.
func parseYear(s string) int {
	y, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return y
}
