// Package ollama provides an HTTP client for local Ollama AI instances.
// It supports spoiler-free recap generation and local vector embedding generation.
package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the standard local Ollama address.
	DefaultBaseURL = "http://localhost:11434"
	// DefaultModel is the default LLM model for text recap generation.
	DefaultModel = "llama3.2"
	// DefaultEmbedModel is the default embedding model for vector generation.
	DefaultEmbedModel = "nomic-embed-text"
	// DefaultTimeout is the default HTTP timeout for generation requests.
	DefaultTimeout = 60 * time.Second
)

// Sentinel errors returned by the ollama client.
var (
	ErrInvalidInput       = errors.New("invalid input: title and content cannot be empty")
	ErrServiceUnavailable = errors.New("ollama service is unreachable")
	ErrModelNotFound      = errors.New("specified ollama model was not found")
	ErrGenerationFailed   = errors.New("ollama text generation failed")
	ErrEmbeddingFailed    = errors.New("ollama embedding generation failed")
)

// Client communicates with a local or remote Ollama server.
type Client struct {
	baseURL    string
	model      string
	embedModel string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the Ollama base URL.
func WithBaseURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.baseURL = strings.TrimRight(url, "/")
		}
	}
}

// WithModel overrides the LLM generation model.
func WithModel(model string) Option {
	return func(c *Client) {
		if model != "" {
			c.model = model
		}
	}
}

// WithEmbedModel overrides the embedding model.
func WithEmbedModel(embedModel string) Option {
	return func(c *Client) {
		if embedModel != "" {
			c.embedModel = embedModel
		}
	}
}

// WithHTTPClient overrides the internal http.Client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithTimeout overrides the default HTTP client timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if c.httpClient == nil {
			c.httpClient = &http.Client{Timeout: timeout}
		} else {
			c.httpClient.Timeout = timeout
		}
	}
}

// New returns a new Ollama client. If baseURL is empty, it falls back to
// the OLLAMA_BASE_URL environment variable or DefaultBaseURL.
func New(baseURL string, opts ...Option) *Client {
	if baseURL == "" {
		baseURL = os.Getenv("OLLAMA_BASE_URL")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = DefaultModel
	}

	embedModel := os.Getenv("OLLAMA_EMBED_MODEL")
	if embedModel == "" {
		embedModel = DefaultEmbedModel
	}

	c := &Client{
		baseURL:    baseURL,
		model:      model,
		embedModel: embedModel,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Model returns the default generation model.
func (c *Client) Model() string {
	return c.model
}

// EmbedModel returns the default embedding model.
func (c *Client) EmbedModel() string {
	return c.embedModel
}

// IsAvailable checks if the Ollama server is running and reachable.
func (c *Client) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Version returns the Ollama version string if reachable.
func (c *Client) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/version", nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", ErrServiceUnavailable, resp.StatusCode)
	}

	var res struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decoding version response: %w", err)
	}
	return res.Version, nil
}
