package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GenerateRequest defines the payload sent to Ollama's /api/generate endpoint.
type GenerateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	System  string         `json:"system,omitempty"`
	Stream  bool           `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

// GenerateResponse defines the payload returned by Ollama's /api/generate endpoint.
type GenerateResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	Error     string `json:"error,omitempty"`
}

const spoilerFreeSystemPrompt = `You are OmniShelf AI, a spoiler-free personal media companion.
Your mission is to generate a concise (2 to 4 sentences), compelling recap or refresher for the given episode, chapter, or media item.
CRITICAL RULES:
1. Strictly avoid spoilers for subsequent episodes, unread chapters, or future plot developments.
2. Focus on re-orienting the user to the premise, key themes, and current narrative stakes without spoiling climactic plot twists or surprise endings.
3. Keep the tone engaging, helpful, and concise.`

// GenerateRecap produces an offline spoiler-free summary/recap for TV episodes,
// book chapters, or general media context.
func (c *Client) GenerateRecap(ctx context.Context, title, overview string, season, episode int) (string, error) {
	title = strings.TrimSpace(title)
	overview = strings.TrimSpace(overview)

	if title == "" && overview == "" {
		return "", ErrInvalidInput
	}

	var sb strings.Builder
	if title != "" {
		sb.WriteString(fmt.Sprintf("Title: %s\n", title))
	}
	if season > 0 && episode > 0 {
		sb.WriteString(fmt.Sprintf("Season: %d, Episode: %d\n", season, episode))
	} else if episode > 0 {
		sb.WriteString(fmt.Sprintf("Chapter / Episode: %d\n", episode))
	}
	if overview != "" {
		sb.WriteString(fmt.Sprintf("Overview / Context: %s\n", overview))
	}
	sb.WriteString("\nPlease write a concise, spoiler-free recap or refresher based on this context.")

	reqBody := GenerateRequest{
		Model:  c.model,
		Prompt: sb.String(),
		System: spoilerFreeSystemPrompt,
		Stream: false,
		Options: map[string]any{
			"temperature": 0.3,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling generate request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer httpResp.Body.Close()

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(bodyBytes, &errResp) == nil && strings.Contains(strings.ToLower(errResp.Error), "not found") {
			return "", fmt.Errorf("%w: %s", ErrModelNotFound, errResp.Error)
		}
		return "", fmt.Errorf("%w: status %d (body: %s)", ErrGenerationFailed, httpResp.StatusCode, string(bodyBytes))
	}

	var genResp GenerateResponse
	if err := json.Unmarshal(bodyBytes, &genResp); err != nil {
		return "", fmt.Errorf("unmarshaling generate response: %w", err)
	}

	if genResp.Error != "" {
		if strings.Contains(strings.ToLower(genResp.Error), "not found") {
			return "", fmt.Errorf("%w: %s", ErrModelNotFound, genResp.Error)
		}
		return "", fmt.Errorf("%w: %s", ErrGenerationFailed, genResp.Error)
	}

	recap := strings.TrimSpace(genResp.Response)
	if recap == "" {
		return "", fmt.Errorf("%w: received empty recap text", ErrGenerationFailed)
	}

	return recap, nil
}
