package openlibrary

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testEmail = "admin@example.com"
const wantUserAgent = "OmniShelf/1.0 (admin@example.com)"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return b
}

// newFixtureServer serves recorded OpenLibrary responses and records the
// User-Agent of every request it sees.
func newFixtureServer(t *testing.T, routes map[string]string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var agents []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		agents = append(agents, r.Header.Get("User-Agent"))
		mu.Unlock()
		name, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "notfound"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture(t, name))
	}))
	get := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), agents...)
	}
	return srv, get
}

func TestGetByISBNFullRecord(t *testing.T) {
	srv, agents := newFixtureServer(t, map[string]string{
		"/isbn/9780140328721.json": "isbn_9780140328721.json",
		"/works/OL45804W.json":     "work_OL45804W.json",
		"/authors/OL34184A.json":   "author_OL34184A.json",
	})
	defer srv.Close()

	c := New(testEmail, WithBaseURL(srv.URL))
	book, err := c.GetByISBN(context.Background(), "9780140328721")
	require.NoError(t, err)

	assert.Equal(t, "9780140328721", book.ISBN13)
	assert.Equal(t, "Fantastic Mr Fox", book.Title)
	assert.Equal(t, []string{"Roald Dahl"}, book.Authors)
	assert.Contains(t, book.Description, "extremely clever anthropomorphized fox")
	assert.Equal(t, 96, book.PageCount)
	assert.Equal(t, 8739161, book.CoverID, "edition cover wins over work cover")
	assert.Equal(t, "/works/OL45804W", book.WorkKey)

	// Every single request must carry the mandatory User-Agent.
	got := agents()
	require.Len(t, got, 3, "edition + work + author requests")
	for _, ua := range got {
		assert.Equal(t, wantUserAgent, ua)
	}
}

func TestGetByISBNNotFound(t *testing.T) {
	srv, agents := newFixtureServer(t, nil)
	defer srv.Close()

	c := New(testEmail, WithBaseURL(srv.URL))
	book, err := c.GetByISBN(context.Background(), "9780000000000")
	require.Nil(t, book)
	require.Error(t, err)

	// E4: distinct typed error, matchable both ways, carrying the ISBN.
	assert.True(t, errors.Is(err, ErrNotFound))
	var nfe *NotFoundError
	require.True(t, errors.As(err, &nfe))
	assert.Equal(t, "9780000000000", nfe.ISBN)

	got := agents()
	require.NotEmpty(t, got)
	assert.Equal(t, wantUserAgent, got[0])
}

func TestGetByISBNPartialMetadata(t *testing.T) {
	// E5: edition with no work, no authors, no cover, no page count must
	// still return usable data — never block tracking on missing metadata.
	srv, _ := newFixtureServer(t, map[string]string{
		"/isbn/9999999999990.json": "isbn_partial_9999999999990.json",
	})
	defer srv.Close()

	c := New(testEmail, WithBaseURL(srv.URL))
	book, err := c.GetByISBN(context.Background(), "9999999999990")
	require.NoError(t, err)
	assert.Equal(t, "Obscure Self-Published Pamphlet", book.Title)
	assert.Equal(t, "9999999999990", book.ISBN13)
	assert.Empty(t, book.Authors)
	assert.Empty(t, book.Description)
	assert.Zero(t, book.CoverID)
	assert.Zero(t, book.PageCount)
	assert.Empty(t, c.CoverURL(book.CoverID, "L"), "no cover ID → no cover URL")
}

func TestGetByISBNWorkFetchFailureDegradesGracefully(t *testing.T) {
	// Work referenced but its fetch 404s: edition data alone is returned.
	srv, _ := newFixtureServer(t, map[string]string{
		"/isbn/9780140328721.json": "isbn_9780140328721.json",
		"/authors/OL34184A.json":   "author_OL34184A.json",
		// no /works/OL45804W.json route → 404
	})
	defer srv.Close()

	c := New(testEmail, WithBaseURL(srv.URL))
	book, err := c.GetByISBN(context.Background(), "9780140328721")
	require.NoError(t, err)
	assert.Equal(t, "Fantastic Mr Fox", book.Title)
	assert.Empty(t, book.Description)
	assert.Equal(t, []string{"Roald Dahl"}, book.Authors, "edition-level author still resolved")
}

func TestCoverURL(t *testing.T) {
	c := New(testEmail)
	assert.Equal(t, "https://covers.openlibrary.org/b/id/8739161-L.jpg", c.CoverURL(8739161, "L"))
	assert.Equal(t, "https://covers.openlibrary.org/b/id/8739161-L.jpg", c.CoverURL(8739161, ""), "default size L")
	assert.Equal(t, "https://covers.openlibrary.org/b/id/42-M.jpg", c.CoverURL(42, "M"))
	assert.Equal(t, "", c.CoverURL(0, "L"))

	custom := New(testEmail, WithCoverBaseURL("http://127.0.0.1:9/"))
	assert.Equal(t, "http://127.0.0.1:9/b/id/1-S.jpg", custom.CoverURL(1, "S"))
}
