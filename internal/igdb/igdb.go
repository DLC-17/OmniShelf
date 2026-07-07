// Package igdb provides a client for the IGDB v4 API (game metadata).
//
// IGDB is authenticated through Twitch's OAuth client-credentials flow: the
// client id/secret are exchanged for a bearer token that is cached in memory
// and refreshed shortly before it expires. Queries use IGDB's Apicalypse body
// language. ScanDex already resolves a barcode to an IGDB id, so this client
// only needs to fetch a game's summary and cover by that id.
package igdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultAuthURL  = "https://id.twitch.tv/oauth2/token"
	defaultAPIURL   = "https://api.igdb.com/v4"
	defaultImageURL = "https://images.igdb.com/igdb/image/upload"
)

// ErrUnconfigured means no client id/secret were supplied at startup.
var ErrUnconfigured = errors.New("igdb: not configured")

// Client talks to the IGDB v4 API, managing its own Twitch OAuth token.
type Client struct {
	clientID     string
	clientSecret string
	authURL      string
	apiURL       string
	imageURL     string
	httpClient   *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

// Option customizes a Client.
type Option func(*Client)

// WithAuthURL overrides the Twitch OAuth token URL (tests).
func WithAuthURL(u string) Option {
	return func(c *Client) { c.authURL = u }
}

// WithAPIURL overrides the IGDB API base URL (tests).
func WithAPIURL(u string) Option {
	return func(c *Client) { c.apiURL = strings.TrimRight(u, "/") }
}

// WithImageBaseURL overrides the images.igdb.com base URL (tests).
func WithImageBaseURL(u string) Option {
	return func(c *Client) { c.imageURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient overrides the shared HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// New returns an IGDB client. When clientID or clientSecret is empty the client
// is considered unconfigured and GetGame returns ErrUnconfigured, so the games
// module degrades gracefully to ScanDex-only metadata.
func New(clientID, clientSecret string, opts ...Option) *Client {
	c := &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		authURL:      defaultAuthURL,
		apiURL:       defaultAPIURL,
		imageURL:     defaultImageURL,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Configured reports whether credentials were supplied.
func (c *Client) Configured() bool {
	return c.clientID != "" && c.clientSecret != ""
}

// Game is the subset of IGDB game metadata we consume.
type Game struct {
	ID           int
	Name         string
	Summary      string
	CoverImageID string // IGDB image_id, e.g. "co3p2d"; "" when no cover
}

// gamePayload models one entry of the /games response.
type gamePayload struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Cover   struct {
		ImageID string `json:"image_id"`
	} `json:"cover"`
}

// GetGame fetches a game's name, summary and cover image id by its IGDB id.
// A missing game yields (nil, nil) — not an error — so callers can keep the
// ScanDex title/platform without a cover or summary.
func (c *Client) GetGame(ctx context.Context, igdbID int) (*Game, error) {
	if !c.Configured() {
		return nil, ErrUnconfigured
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return nil, err
	}

	body := fmt.Sprintf("fields name,summary,cover.image_id; where id = %d;", igdbID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/games", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("igdb: build request: %w", err)
	}
	req.Header.Set("Client-ID", c.clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("igdb: request games: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("igdb: games returned status %d: %s", resp.StatusCode, string(raw))
	}

	var payloads []gamePayload
	if err := json.Unmarshal(raw, &payloads); err != nil {
		return nil, fmt.Errorf("igdb: decode games: %w", err)
	}
	if len(payloads) == 0 {
		return nil, nil
	}

	p := payloads[0]
	return &Game{
		ID:           p.ID,
		Name:         p.Name,
		Summary:      p.Summary,
		CoverImageID: p.Cover.ImageID,
	}, nil
}

// CoverURL returns the images.igdb.com URL for a cover image id at the given
// size (e.g. "t_cover_big"). The download must go through internal/images —
// never hotlink this URL from the frontend. Returns "" for an empty image id.
func (c *Client) CoverURL(imageID, size string) string {
	if imageID == "" {
		return ""
	}
	if size == "" {
		size = "t_cover_big"
	}
	return fmt.Sprintf("%s/%s/%s.jpg", c.imageURL, size, imageID)
}

// tokenResponse is the Twitch OAuth client-credentials payload.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"` // seconds
	TokenType   string `json:"token_type"`
}

// getToken returns a valid bearer token, fetching a fresh one when the cached
// token is absent or within 60s of expiry.
func (c *Client) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpiry.Add(-60*time.Second)) {
		return c.token, nil
	}

	q := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("igdb: build token request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("igdb: token request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("igdb: token returned status %d: %s", resp.StatusCode, string(raw))
	}

	var tr tokenResponse
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", fmt.Errorf("igdb: decode token: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("igdb: token response had no access_token")
	}

	c.token = tr.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return c.token, nil
}
