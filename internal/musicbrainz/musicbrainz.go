// Package musicbrainz provides a client for the MusicBrainz web service
// (https://musicbrainz.org), used to search albums by name and look one up by
// its MBID. MusicBrainz needs no API key but requires a descriptive
// User-Agent identifying the application and a contact address, per their
// policy; this client sends "OmniShelf/1.0 (<contact email>)" on every request.
//
// Albums are modeled as release-groups (the abstract album, independent of any
// specific pressing). Cover art is served separately by the Cover Art Archive,
// keyed by the same MBID; CoverURL builds that link for internal/images to
// download (never hotlink it from the frontend).
package musicbrainz

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
	defaultBaseURL     = "https://musicbrainz.org/ws/2"
	defaultCoverArtURL = "https://coverartarchive.org"
)

// ErrNotFound is returned when a release-group MBID does not exist.
var ErrNotFound = errors.New("musicbrainz: not found")

// Client talks to the MusicBrainz web service.
type Client struct {
	userAgent   string
	baseURL     string
	coverArtURL string
	httpClient  *http.Client
}

// Option customizes a Client.
type Option func(*Client)

// WithBaseURL overrides the MusicBrainz base URL (used by tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithCoverArtBaseURL overrides the coverartarchive.org base URL (tests).
func WithCoverArtBaseURL(u string) Option {
	return func(c *Client) { c.coverArtURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient overrides the shared HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// New returns a MusicBrainz client identifying itself as
// "OmniShelf/1.0 (<contactEmail>)" on every request.
func New(contactEmail string, opts ...Option) *Client {
	c := &Client{
		userAgent:   fmt.Sprintf("OmniShelf/1.0 (%s)", contactEmail),
		baseURL:     defaultBaseURL,
		coverArtURL: defaultCoverArtURL,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ReleaseGroup is the album metadata we consume. Year is derived from the
// first-release-date; 0 when unknown.
type ReleaseGroup struct {
	MBID   string
	Title  string
	Artist string
	Year   int
}

// searchResponse models the /release-group search payload subset we use.
type searchResponse struct {
	ReleaseGroups []releaseGroupPayload `json:"release-groups"`
}

// releaseGroupPayload models one release-group entry (search hit or lookup).
type releaseGroupPayload struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	FirstReleaseDate string `json:"first-release-date"`
	PrimaryType      string `json:"primary-type"`
	ArtistCredit     []struct {
		Name string `json:"name"`
	} `json:"artist-credit"`
}

func (p releaseGroupPayload) toReleaseGroup() ReleaseGroup {
	return ReleaseGroup{
		MBID:   p.ID,
		Title:  p.Title,
		Artist: joinArtistCredit(p.ArtistCredit),
		Year:   parseYear(p.FirstReleaseDate),
	}
}

// Search finds albums (release-groups) matching a free-text name query,
// returning up to limit results ordered by MusicBrainz relevance.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]ReleaseGroup, error) {
	if limit <= 0 {
		limit = 10
	}
	q := url.Values{
		"query": {query},
		"fmt":   {"json"},
		"limit": {strconv.Itoa(limit)},
		// dismax uses MusicBrainz's user-friendly parser, matching the bare query
		// against both the release-group title AND the artist name (the default
		// Lucene parser only searches the title field).
		"dismax": {"true"},
	}
	var sr searchResponse
	if err := c.getJSON(ctx, "/release-group?"+q.Encode(), &sr); err != nil {
		return nil, err
	}
	out := make([]ReleaseGroup, 0, len(sr.ReleaseGroups))
	for _, rg := range sr.ReleaseGroups {
		out = append(out, rg.toReleaseGroup())
	}
	return out, nil
}

// GetReleaseGroup looks a release-group up by MBID for its title, artist and
// year. A missing MBID yields ErrNotFound.
func (c *Client) GetReleaseGroup(ctx context.Context, mbid string) (*ReleaseGroup, error) {
	q := url.Values{
		"fmt": {"json"},
		"inc": {"artist-credits"},
	}
	var p releaseGroupPayload
	if err := c.getJSON(ctx, "/release-group/"+url.PathEscape(mbid)+"?"+q.Encode(), &p); err != nil {
		return nil, err
	}
	if p.ID == "" {
		return nil, ErrNotFound
	}
	rg := p.toReleaseGroup()
	return &rg, nil
}

// CoverURL returns the Cover Art Archive front-cover URL for a release-group
// MBID at the given size (250 or 500 px; defaults to 500). The download must go
// through internal/images. Returns "" for an empty MBID.
func (c *Client) CoverURL(mbid string, size int) string {
	if mbid == "" {
		return ""
	}
	if size != 250 && size != 500 {
		size = 500
	}
	return fmt.Sprintf("%s/release-group/%s/front-%d", c.coverArtURL, mbid, size)
}

// getJSON performs a GET with the mandatory User-Agent and decodes JSON.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("musicbrainz: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("musicbrainz: request %s: %w", path, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("musicbrainz: %s returned status %d: %s", path, resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("musicbrainz: decode %s: %w", path, err)
	}
	return nil
}

// joinArtistCredit concatenates the artist-credit names (a collaboration is
// split across multiple entries) into one display string.
func joinArtistCredit(credits []struct {
	Name string `json:"name"`
}) string {
	parts := make([]string, 0, len(credits))
	for _, c := range credits {
		if c.Name != "" {
			parts = append(parts, c.Name)
		}
	}
	return strings.Join(parts, ", ")
}

// parseYear extracts the leading year from a MusicBrainz date
// ("YYYY", "YYYY-MM" or "YYYY-MM-DD"); 0 when absent or unparseable.
func parseYear(date string) int {
	date = strings.TrimSpace(date)
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}
