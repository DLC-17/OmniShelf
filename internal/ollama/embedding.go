package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// EmbeddingRequest is the payload for Ollama's /api/embeddings endpoint.
type EmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// EmbeddingResponse is the response from Ollama's /api/embeddings endpoint.
type EmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

// EmbedRequest is the payload for Ollama's newer /api/embed endpoint.
type EmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// EmbedResponse is the response from Ollama's newer /api/embed endpoint.
type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

// GenerateEmbedding generates a vector embedding for the given text using Ollama.
func (c *Client) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrInvalidInput
	}

	// Try legacy /api/embeddings first
	emb, err := c.tryLegacyEmbeddings(ctx, text)
	if err == nil {
		return emb, nil
	}

	// If legacy endpoint is not found (404), try modern /api/embed
	if errors.Is(err, errEndpointNotFound) {
		return c.tryModernEmbed(ctx, text)
	}

	return nil, err
}

var errEndpointNotFound = errors.New("endpoint not found")

func (c *Client) tryLegacyEmbeddings(ctx context.Context, text string) ([]float32, error) {
	reqBody := EmbeddingRequest{
		Model:  c.embedModel,
		Prompt: text,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling embedding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusNotFound {
		return nil, errEndpointNotFound
	}

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(bodyBytes, &errResp) == nil && strings.Contains(strings.ToLower(errResp.Error), "not found") {
			return nil, fmt.Errorf("%w: %s", ErrModelNotFound, errResp.Error)
		}
		return nil, fmt.Errorf("%w: status %d (body: %s)", ErrEmbeddingFailed, httpResp.StatusCode, string(bodyBytes))
	}

	var res EmbeddingResponse
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, fmt.Errorf("unmarshaling embedding response: %w", err)
	}

	if res.Error != "" {
		if strings.Contains(strings.ToLower(res.Error), "not found") {
			return nil, fmt.Errorf("%w: %s", ErrModelNotFound, res.Error)
		}
		return nil, fmt.Errorf("%w: %s", ErrEmbeddingFailed, res.Error)
	}

	if len(res.Embedding) == 0 {
		return nil, fmt.Errorf("%w: received empty vector", ErrEmbeddingFailed)
	}

	return res.Embedding, nil
}

func (c *Client) tryModernEmbed(ctx context.Context, text string) ([]float32, error) {
	reqBody := EmbedRequest{
		Model: c.embedModel,
		Input: text,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling embed request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer httpResp.Body.Close()

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(bodyBytes, &errResp) == nil && strings.Contains(strings.ToLower(errResp.Error), "not found") {
			return nil, fmt.Errorf("%w: %s", ErrModelNotFound, errResp.Error)
		}
		return nil, fmt.Errorf("%w: status %d (body: %s)", ErrEmbeddingFailed, httpResp.StatusCode, string(bodyBytes))
	}

	var res EmbedResponse
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, fmt.Errorf("unmarshaling embed response: %w", err)
	}

	if res.Error != "" {
		if strings.Contains(strings.ToLower(res.Error), "not found") {
			return nil, fmt.Errorf("%w: %s", ErrModelNotFound, res.Error)
		}
		return nil, fmt.Errorf("%w: %s", ErrEmbeddingFailed, res.Error)
	}

	if len(res.Embeddings) == 0 || len(res.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("%w: received empty embeddings array", ErrEmbeddingFailed)
	}

	return res.Embeddings[0], nil
}
