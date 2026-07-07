package scandex

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient wires a Client at a test server with canned credentials.
func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New("42", "tok", WithBaseURL(srv.URL))
}

func TestLookupHit(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/lookup", r.URL.Path)
		assert.Equal(t, "045496590420", r.URL.Query().Get("value"))
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.Equal(t, "42", r.Header.Get("User-Id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 8279,
			"source": "import",
			"igdb_metadata": {
				"id": 7346,
				"name": "The Legend of Zelda: Breath of the Wild",
				"platform": {"id": 130, "name": "Nintendo Switch"}
			}
		}`))
	})

	game, err := c.Lookup(context.Background(), "045496590420")
	require.NoError(t, err)
	assert.Equal(t, "045496590420", game.Barcode)
	assert.Equal(t, "The Legend of Zelda: Breath of the Wild", game.Title)
	assert.Equal(t, "Nintendo Switch", game.Platform)
	assert.Equal(t, 7346, game.IGDBID)
	assert.Equal(t, 130, game.PlatformID)
}

func TestLookupNotFound404(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "This barcode does not exist yet."}`))
	})

	_, err := c.Lookup(context.Background(), "711719521099")
	require.ErrorIs(t, err, ErrNotFound)
	var nf *NotFoundError
	require.True(t, errors.As(err, &nf))
	assert.Equal(t, "711719521099", nf.Barcode)
}

// A 200 carrying only a message (no igdb_metadata) is still a miss.
func TestLookupNotFound200(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message": "This barcode does not exist yet."}`))
	})

	_, err := c.Lookup(context.Background(), "711719521099")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestLookupInvalidBarcode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "The parameter 'value' is not valid."}`))
	})

	_, err := c.Lookup(context.Background(), "zelda")
	require.ErrorIs(t, err, ErrInvalidBarcode)
}

func TestLookupUnauthorized(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := c.Lookup(context.Background(), "045496590420")
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestLookupUnconfigured(t *testing.T) {
	c := New("", "")
	assert.False(t, c.Configured())
	_, err := c.Lookup(context.Background(), "045496590420")
	require.ErrorIs(t, err, ErrUnconfigured)
}
