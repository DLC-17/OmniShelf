package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientDefaults(t *testing.T) {
	c := New("")
	assert.Equal(t, DefaultBaseURL, c.BaseURL())
	assert.Equal(t, DefaultModel, c.Model())
	assert.Equal(t, DefaultEmbedModel, c.EmbedModel())
}

func TestNewClientEnvVars(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://ollama-server:11434/")
	t.Setenv("OLLAMA_MODEL", "mistral")
	t.Setenv("OLLAMA_EMBED_MODEL", "all-minilm")

	c := New("")
	assert.Equal(t, "http://ollama-server:11434", c.BaseURL())
	assert.Equal(t, "mistral", c.Model())
	assert.Equal(t, "all-minilm", c.EmbedModel())
}

func TestNewClientOptions(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}
	c := New("http://localhost:8000/",
		WithModel("llama3"),
		WithEmbedModel("nomic-custom"),
		WithHTTPClient(customClient),
		WithTimeout(10*time.Second),
	)

	assert.Equal(t, "http://localhost:8000", c.BaseURL())
	assert.Equal(t, "llama3", c.Model())
	assert.Equal(t, "nomic-custom", c.EmbedModel())
	assert.Equal(t, 10*time.Second, c.httpClient.Timeout)
}

func TestIsAvailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := New(ts.URL)
	assert.True(t, c.IsAvailable(context.Background()))

	badClient := New("http://127.0.0.1:54321")
	assert.False(t, badClient.IsAvailable(context.Background()))
}

func TestVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "0.1.30"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := New(ts.URL)
	ver, err := c.Version(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "0.1.30", ver)

	badClient := New("http://127.0.0.1:54321")
	_, err = badClient.Version(context.Background())
	assert.ErrorIs(t, err, ErrServiceUnavailable)
}

func TestGenerateRecapSuccess(t *testing.T) {
	var receivedReq GenerateRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/generate", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		err := json.NewDecoder(r.Body).Decode(&receivedReq)
		require.NoError(t, err)

		resp := GenerateResponse{
			Model:    "llama3.2",
			Response: "The team finds an unexpected key card inside the department.",
			Done:     true,
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := New(ts.URL)
	recap, err := c.GenerateRecap(context.Background(), "Severance", "Mark discovers something strange.", 1, 4)
	require.NoError(t, err)
	assert.Equal(t, "The team finds an unexpected key card inside the department.", recap)

	assert.Contains(t, receivedReq.Prompt, "Title: Severance")
	assert.Contains(t, receivedReq.Prompt, "Season: 1, Episode: 4")
	assert.Contains(t, receivedReq.Prompt, "Overview / Context: Mark discovers something strange.")
	assert.Contains(t, receivedReq.System, "spoiler-free")
}

func TestGenerateRecapChapterAndOverview(t *testing.T) {
	var receivedReq GenerateRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedReq)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(GenerateResponse{
			Response: "Frodo departs the Shire with his companions.",
			Done:     true,
		})
	}))
	defer ts.Close()

	c := New(ts.URL)
	recap, err := c.GenerateRecap(context.Background(), "The Fellowship of the Ring", "A party begins.", 0, 3)
	require.NoError(t, err)
	assert.Equal(t, "Frodo departs the Shire with his companions.", recap)
	assert.Contains(t, receivedReq.Prompt, "Chapter / Episode: 3")
}

func TestGenerateRecapInvalidInput(t *testing.T) {
	c := New("http://localhost:11434")
	_, err := c.GenerateRecap(context.Background(), "", "", 0, 0)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestGenerateRecapModelNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model 'llama3.2' not found, try pulling it first"})
	}))
	defer ts.Close()

	c := New(ts.URL)
	_, err := c.GenerateRecap(context.Background(), "Test Title", "Test overview", 1, 1)
	assert.ErrorIs(t, err, ErrModelNotFound)
}

func TestGenerateRecapServiceUnavailable(t *testing.T) {
	c := New("http://127.0.0.1:54321")
	_, err := c.GenerateRecap(context.Background(), "Test Title", "Test overview", 1, 1)
	assert.ErrorIs(t, err, ErrServiceUnavailable)
}

func TestGenerateEmbeddingLegacyEndpoint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/embeddings", r.URL.Path)
		var req EmbeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "cyberpunk detective neon noir", req.Prompt)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(EmbeddingResponse{
			Embedding: []float32{0.12, -0.34, 0.56},
		})
	}))
	defer ts.Close()

	c := New(ts.URL)
	emb, err := c.GenerateEmbedding(context.Background(), "cyberpunk detective neon noir")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.12, -0.34, 0.56}, emb)
}

func TestGenerateEmbeddingModernEmbedFallback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/embeddings" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/api/embed" {
			var req EmbedRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "space opera epic", req.Input)

			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(EmbedResponse{
				Embeddings: [][]float32{{0.99, -0.42, 0.11}},
			})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	c := New(ts.URL)
	emb, err := c.GenerateEmbedding(context.Background(), "space opera epic")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.99, -0.42, 0.11}, emb)
}

func TestGenerateEmbeddingInvalidInput(t *testing.T) {
	c := New("http://localhost:11434")
	_, err := c.GenerateEmbedding(context.Background(), "   ")
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestGenerateEmbeddingModelNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model 'nomic-embed-text' not found"})
	}))
	defer ts.Close()

	c := New(ts.URL)
	_, err := c.GenerateEmbedding(context.Background(), "test text")
	assert.Error(t, err)
}
