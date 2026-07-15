// Package scandex provides a client for the ScanDex video-game barcode
// database (https://scandex.gamery.app). A UPC/EAN barcode is resolved to a
// game title, platform and the matching IGDB id. ScanDex is barcode-only —
// there is no title-search endpoint — and every request is authenticated with
// a user id and access token.
//
// Missing barcodes yield ErrNotFound; a code outside the 8–14 digit range
// yields ErrInvalidBarcode; bad credentials yield ErrUnauthorized.
package scandex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://scandex.gamery.app/api/v2"

// Sentinel errors callers translate into the API envelope.
var (
	// ErrNotFound means ScanDex has no game for the barcode (HTTP 404).
	ErrNotFound = errors.New("scandex: no game for barcode")
	// ErrInvalidBarcode means the code is not 8–14 digits (HTTP 400).
	ErrInvalidBarcode = errors.New("scandex: invalid barcode")
	// ErrUnauthorized means the user id / access token was rejected (HTTP 401).
	ErrUnauthorized = errors.New("scandex: unauthorized")
	// ErrUnconfigured means no credentials were supplied at startup.
	ErrUnconfigured = errors.New("scandex: not configured")
)

// NotFoundError wraps ErrNotFound with the barcode that missed, so callers can
// echo it in the API error envelope.
type NotFoundError struct {
	Barcode string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("scandex: no game for barcode %s", e.Barcode)
}

// Unwrap lets errors.Is(err, ErrNotFound) match.
func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// Client talks to the ScanDex API.
type Client struct {
	userID      string
	accessToken string
	baseURL     string
	httpClient  *http.Client
}

// Option customizes a Client.
type Option func(*Client)

// WithBaseURL overrides the ScanDex base URL (used by tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient overrides the shared HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// New returns a ScanDex client. When userID or accessToken is empty the client
// is considered unconfigured and every Lookup returns ErrUnconfigured, so the
// games feature degrades to a clear error instead of a panic.
func New(userID, accessToken string, opts ...Option) *Client {
	c := &Client{
		userID:      userID,
		accessToken: accessToken,
		baseURL:     defaultBaseURL,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Configured reports whether credentials were supplied.
func (c *Client) Configured() bool {
	return c.userID != "" && c.accessToken != ""
}

// Game is the metadata ScanDex returns for one barcode. Cover art and
// descriptions are not part of the ScanDex payload (they live in IGDB, keyed
// by IGDBID), so those fields are intentionally absent.
type Game struct {
	Barcode    string
	Title      string
	Platform   string
	IGDBID     int
	PlatformID int
}

// lookupResponse models both ScanDex shapes: a hit carries igdb_metadata; a
// miss carries only a message.
type lookupResponse struct {
	ID           int    `json:"id"`
	Message      string `json:"message"`
	Error        string `json:"error"`
	IGDBMetadata *struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Platform struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"platform"`
	} `json:"igdb_metadata"`
}

// Lookup resolves a barcode to a game. The caller is expected to have already
// stripped separators; ScanDex validates the 8–14 digit length itself.
func (c *Client) Lookup(ctx context.Context, barcode string) (*Game, error) {
	if !c.Configured() {
		return nil, ErrUnconfigured
	}

	endpoint := c.baseURL + "/lookup?value=" + url.QueryEscape(barcode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("scandex: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("User-Id", c.userID)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scandex: request lookup: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to decode below
	case http.StatusNotFound:
		return nil, &NotFoundError{Barcode: barcode}
	case http.StatusBadRequest:
		return nil, fmt.Errorf("%w: %s", ErrInvalidBarcode, barcode)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("scandex: lookup returned status %d: %s", resp.StatusCode, string(body))
	}

	var lr lookupResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return nil, fmt.Errorf("scandex: decode lookup: %w", err)
	}
	// A 200 with no metadata is treated as a miss (defensive: ScanDex normally
	// answers 404, but has been observed returning 200 + {"message": ...}).
	if lr.IGDBMetadata == nil {
		return nil, &NotFoundError{Barcode: barcode}
	}

	return &Game{
		Barcode:    barcode,
		Title:      lr.IGDBMetadata.Name,
		Platform:   lr.IGDBMetadata.Platform.Name,
		IGDBID:     lr.IGDBMetadata.ID,
		PlatformID: lr.IGDBMetadata.Platform.ID,
	}, nil
}
