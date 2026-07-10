package discogs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBarcode = "602547790392"

// server stands in for the Discogs /database/search endpoint.
type server struct {
	body   string
	status int
	hits   int
	gotAuth string
	gotUA   string
}

func (s *server) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/database/search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.hits++
		s.gotAuth = r.Header.Get("Authorization")
		s.gotUA = r.Header.Get("User-Agent")
		if s.status != 0 {
			w.WriteHeader(s.status)
		}
		_, _ = w.Write([]byte(s.body))
	}
}

func newClient(t *testing.T, s *server) *Client {
	t.Helper()
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return New("tok123", WithBaseURL(srv.URL))
}

func TestLookupByBarcode(t *testing.T) {
	s := &server{body: `{"results":[{"id":8888,"title":"Adele - 25","year":"2015","cover_image":"http://img.test/25.jpg"}]}`}
	c := newClient(t, s)

	rel, err := c.LookupByBarcode(context.Background(), testBarcode)
	require.NoError(t, err)
	assert.Equal(t, 8888, rel.DiscogsID)
	assert.Equal(t, "Adele", rel.Artist)
	assert.Equal(t, "25", rel.Title)
	assert.Equal(t, 2015, rel.Year)
	assert.Equal(t, "http://img.test/25.jpg", rel.CoverURL)
	assert.Equal(t, testBarcode, rel.Barcode)
	assert.Equal(t, "Discogs token=tok123", s.gotAuth)
	assert.Contains(t, s.gotUA, "OmniShelf")
}

// A title without the " - " separator becomes the release title with no artist.
func TestLookupNoSeparator(t *testing.T) {
	s := &server{body: `{"results":[{"id":1,"title":"Untitled","year":""}]}`}
	c := newClient(t, s)

	rel, err := c.LookupByBarcode(context.Background(), testBarcode)
	require.NoError(t, err)
	assert.Equal(t, "", rel.Artist)
	assert.Equal(t, "Untitled", rel.Title)
	assert.Equal(t, 0, rel.Year, "non-numeric year is 0")
}

// The thumb is used when cover_image is absent.
func TestLookupCoverFallback(t *testing.T) {
	s := &server{body: `{"results":[{"id":2,"title":"A - B","thumb":"http://img.test/thumb.jpg"}]}`}
	c := newClient(t, s)

	rel, err := c.LookupByBarcode(context.Background(), testBarcode)
	require.NoError(t, err)
	assert.Equal(t, "http://img.test/thumb.jpg", rel.CoverURL)
}

func TestLookupNotFound(t *testing.T) {
	s := &server{body: `{"results":[]}`}
	c := newClient(t, s)

	_, err := c.LookupByBarcode(context.Background(), testBarcode)
	require.ErrorIs(t, err, ErrNotFound)
	var nf *NotFoundError
	require.ErrorAs(t, err, &nf)
	assert.Equal(t, testBarcode, nf.Barcode)
}

func TestLookupUnconfigured(t *testing.T) {
	c := New("")
	assert.False(t, c.Configured())
	_, err := c.LookupByBarcode(context.Background(), testBarcode)
	require.ErrorIs(t, err, ErrUnconfigured)
}
