// Package seerr provides a client for the Seerr / Overseerr API.
// It is used to query media availability on a local server and request downloads.
package seerr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/davidlc1229/omnishelf/internal/config"
)

// Client talks to the Seerr API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New returns a new Seerr client.
func New(cfg *config.Config) *Client {
	return &Client{
		baseURL:    cfg.SeerrURL,
		apiKey:     cfg.SeerrAPIKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Configured returns true if both URL and API key are provided.
func (c *Client) Configured() bool {
	return c.baseURL != "" && c.apiKey != ""
}

// StatusError is returned when Seerr responds with an unexpected HTTP status.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("seerr: unexpected status %d: %s", e.StatusCode, e.Body)
}

// MediaStatus represents the availability of a media item on the Seerr server.
// Overseerr uses numeric statuses: 1=UNKNOWN, 2=PENDING, 3=PROCESSING,
// 4=PARTIALLY_AVAILABLE, 5=AVAILABLE.
type MediaStatus struct {
	MediaInfo struct {
		Status int `json:"status"`
	} `json:"mediaInfo"`
}

// IsAvailable returns true if the media is partially or fully available.
func (m *MediaStatus) IsAvailable() bool {
	return m.MediaInfo.Status == 4 || m.MediaInfo.Status == 5
}

// IsRequested returns true if the media has been requested or is processing.
func (m *MediaStatus) IsRequested() bool {
	return m.MediaInfo.Status == 2 || m.MediaInfo.Status == 3
}

// CheckAvailability queries Seerr for a media item's status.
// mediaType must be "movie" or "tv".
func (c *Client) CheckAvailability(ctx context.Context, mediaType string, tmdbID int) (*MediaStatus, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("seerr is not configured")
	}

	var out MediaStatus
	path := fmt.Sprintf("/api/v1/%s/%d", mediaType, tmdbID)
	
	// A 404 from this endpoint means the media is entirely unknown to Seerr
	// (not requested, not available). We handle it gracefully.
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	if err != nil {
		if se, ok := err.(*StatusError); ok && se.StatusCode == http.StatusNotFound {
			return &MediaStatus{}, nil // default 0 status (UNKNOWN)
		}
		return nil, err
	}
	return &out, nil
}

// RequestPayload is the JSON body for creating a media request.
type RequestPayload struct {
	MediaID   int    `json:"mediaId"`
	MediaType string `json:"mediaType"` // "movie" or "tv"
}

// RequestMedia sends a request to Seerr to download the given media.
func (c *Client) RequestMedia(ctx context.Context, mediaType string, tmdbID int) error {
	if !c.Configured() {
		return fmt.Errorf("seerr is not configured")
	}

	payload := RequestPayload{
		MediaID:   tmdbID,
		MediaType: mediaType,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("seerr: marshal request: %w", err)
	}

	return c.do(ctx, http.MethodPost, "/api/v1/request", bodyBytes, nil)
}

// do performs the HTTP request and unmarshals the JSON response into out (if out != nil).
func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	reqURL := c.baseURL + path
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("seerr: build request: %w", err)
	}
	
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("seerr: request %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &StatusError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("seerr: decode %s: %w", path, err)
		}
	}
	return nil
}
