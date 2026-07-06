package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/books"
	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/images"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/openlibrary"
)

const (
	fixtureISBN      = "9780306406157"
	fixturePartial   = "9791234567890"
	fixtureUntracked = "9780306406164" // valid shape, absent from the fixture server
)

// newOpenLibraryFixture serves recorded OpenLibrary responses over httptest
// (testing standard: never hit real APIs).
func newOpenLibraryFixture(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
	mux.HandleFunc("/isbn/"+fixtureISBN+".json", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"title":"Networking Basics","number_of_pages":320,"covers":[12345],
			"isbn_13":["`+fixtureISBN+`"],"works":[{"key":"/works/OW1"}],"authors":[{"key":"/authors/OA1"}]}`)
	})
	mux.HandleFunc("/works/OW1.json", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"description":"A classic.","covers":[12345]}`)
	})
	mux.HandleFunc("/authors/OA1.json", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"name":"Jane Doe"}`)
	})
	// E5 fixture: edition with a bare title — no work, cover, authors, pages.
	mux.HandleFunc("/isbn/"+fixturePartial+".json", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"title":"Bare Edition"}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newCoverFixture serves a fake JPEG for any path.
func newCoverFixture(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8fake-jpeg"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newBooksEnv builds a router with auth + book + library routes backed by a
// real openlibrary.Client pointed at the fixture servers.
func newBooksEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	ol := openlibrary.New("test@example.com",
		openlibrary.WithBaseURL(newOpenLibraryFixture(t).URL),
		openlibrary.WithCoverBaseURL(newCoverFixture(t).URL))
	svc := books.NewService(gdb, ol, images.New(t.TempDir()))

	r := gin.New()
	protected := RegisterRoutes(r, gdb, &config.Config{JWTSecret: testSecret})
	RegisterBookRoutes(protected, svc)
	RegisterLibraryRoutes(protected, svc)
	return r, gdb
}

// loginAs registers (with a fresh invite) and logs a user in, returning the
// session cookie and user ID.
func loginAs(t *testing.T, r *gin.Engine, gdb *gorm.DB, username string) (*http.Cookie, uint) {
	t.Helper()
	code := "CODE-" + username
	seedInvite(t, gdb, code)
	reg := doJSON(r, http.MethodPost, "/api/auth/register", registerBody(username, code))
	require.Equal(t, http.StatusCreated, reg.Code, reg.Body.String())
	var created userResponse
	require.NoError(t, json.Unmarshal(reg.Body.Bytes(), &created))
	login := doJSON(r, http.MethodPost, "/api/auth/login",
		map[string]string{"username": username, "password": "hunter2hunter2"})
	require.Equal(t, http.StatusOK, login.Code)
	return login.Result().Cookies()[0], created.ID
}

func scanBook(t *testing.T, r *gin.Engine, cookie *http.Cookie, isbn string) bookResponse {
	t.Helper()
	w := doJSON(r, http.MethodPost, "/api/books/scan", map[string]string{"isbn": isbn}, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var book bookResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &book))
	return book
}

func TestScanEndpointHappyPath(t *testing.T) {
	r, gdb := newBooksEnv(t)
	cookie, _ := loginAs(t, r, gdb, "alice")

	book := scanBook(t, r, cookie, fixtureISBN)
	assert.Equal(t, fixtureISBN, book.ISBN13)
	assert.Equal(t, "Networking Basics", book.Title)
	assert.Equal(t, "Jane Doe", book.Authors)
	assert.Equal(t, 320, book.PageCount)
	assert.Equal(t, "book/"+fixtureISBN+".jpg", book.CoverPath, "cover cached via images store")
	assert.NotZero(t, book.ID)
}

// E4: unknown ISBN → 404 envelope carrying the scanned ISBN.
func TestScanEndpointUnknownISBN(t *testing.T) {
	r, gdb := newBooksEnv(t)
	cookie, _ := loginAs(t, r, gdb, "alice")

	w := doJSON(r, http.MethodPost, "/api/books/scan", map[string]string{"isbn": fixtureUntracked}, cookie)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		ISBN    string `json:"isbn"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, CodeBookNotFound, env.Error)
	assert.Equal(t, fixtureUntracked, env.ISBN, "ISBN must be echoed for the manual-entry form")
	assert.NotEmpty(t, env.Message)
}

func TestScanEndpointValidation(t *testing.T) {
	r, gdb := newBooksEnv(t)
	cookie, _ := loginAs(t, r, gdb, "alice")

	for _, isbn := range []string{"", "not-an-isbn", "1234567890123"} {
		w := doJSON(r, http.MethodPost, "/api/books/scan", map[string]string{"isbn": isbn}, cookie)
		require.Equal(t, http.StatusBadRequest, w.Code, "isbn %q: %s", isbn, w.Body.String())
		assertEnvelope(t, w, CodeInvalidRequest)
	}

	// No session cookie → 401 from the middleware.
	w := doJSON(r, http.MethodPost, "/api/books/scan", map[string]string{"isbn": fixtureISBN})
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assertEnvelope(t, w, CodeUnauthorized)
}

func TestTrackEndpoint(t *testing.T) {
	r, gdb := newBooksEnv(t)
	cookie, userID := loginAs(t, r, gdb, "alice")
	book := scanBook(t, r, cookie, fixtureISBN)

	// Happy path.
	w := doJSON(r, http.MethodPost, "/api/books/track",
		map[string]any{"bookId": book.ID, "status": "READING"}, cookie)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var item itemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &item))
	assert.Equal(t, "BOOK", item.Type)
	assert.Equal(t, fixtureISBN, item.ExternalID)

	// The item belongs to the JWT user, not any client-supplied ID.
	var stored models.TrackingItem
	require.NoError(t, gdb.First(&stored, item.ID).Error)
	assert.Equal(t, userID, stored.UserID)

	// E16: duplicate → 409 with the existing item.
	w = doJSON(r, http.MethodPost, "/api/books/track",
		map[string]any{"bookId": book.ID, "status": "PLAN_TO"}, cookie)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var conflict struct {
		Error string       `json:"error"`
		Item  itemResponse `json:"item"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &conflict))
	assert.Equal(t, CodeAlreadyTracked, conflict.Error)
	assert.Equal(t, item.ID, conflict.Item.ID)

	// Invalid status → 400; TV-only status is invalid for books.
	for _, status := range []string{"WATCHING", "reading", ""} {
		w = doJSON(r, http.MethodPost, "/api/books/track",
			map[string]any{"bookId": book.ID, "status": status}, cookie)
		require.Equal(t, http.StatusBadRequest, w.Code, "status %q", status)
		assertEnvelope(t, w, CodeInvalidRequest)
	}

	// Unknown book → 404.
	w = doJSON(r, http.MethodPost, "/api/books/track",
		map[string]any{"bookId": 9999, "status": "READING"}, cookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertEnvelope(t, w, CodeNotFound)
}

// E5: a record with only a title still scans and tracks.
func TestPartialMetadataStillTracks(t *testing.T) {
	r, gdb := newBooksEnv(t)
	cookie, _ := loginAs(t, r, gdb, "alice")

	book := scanBook(t, r, cookie, fixturePartial)
	assert.Equal(t, "Bare Edition", book.Title)
	assert.Empty(t, book.Authors)
	assert.Empty(t, book.CoverPath)
	assert.Zero(t, book.PageCount)

	w := doJSON(r, http.MethodPost, "/api/books/track",
		map[string]any{"bookId": book.ID, "status": "PLAN_TO"}, cookie)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestLibraryEndpoint(t *testing.T) {
	r, gdb := newBooksEnv(t)
	cookie, userID := loginAs(t, r, gdb, "alice")
	otherCookie, _ := loginAs(t, r, gdb, "bob")

	book := scanBook(t, r, cookie, fixtureISBN)
	w := doJSON(r, http.MethodPost, "/api/books/track",
		map[string]any{"bookId": book.ID, "status": "READING"}, cookie)
	require.Equal(t, http.StatusCreated, w.Code)
	// A TV item for the same user, seeded directly.
	require.NoError(t, gdb.Create(&models.TrackingItem{
		UserID: userID, Type: "TV", ExternalID: "1399", Title: "GoT", Status: "WATCHING",
	}).Error)

	list := func(query string, cookie *http.Cookie) []itemResponse {
		t.Helper()
		w := doJSON(r, http.MethodGet, "/api/library"+query, nil, cookie)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var items []itemResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
		return items
	}

	assert.Len(t, list("", cookie), 2)
	booksOnly := list("?type=BOOK", cookie)
	require.Len(t, booksOnly, 1)
	assert.Equal(t, fixtureISBN, booksOnly[0].ExternalID)
	assert.Len(t, list("?type=TV&status=WATCHING", cookie), 1)
	assert.Len(t, list("?status=COMPLETED", cookie), 0)
	assert.Len(t, list("", otherCookie), 0, "library is scoped to the current user")

	w = doJSON(r, http.MethodGet, "/api/library?type=MOVIE", nil, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assertEnvelope(t, w, CodeInvalidRequest)
}

func TestPatchItemEndpoint(t *testing.T) {
	r, gdb := newBooksEnv(t)
	cookie, _ := loginAs(t, r, gdb, "alice")
	otherCookie, _ := loginAs(t, r, gdb, "bob")

	book := scanBook(t, r, cookie, fixtureISBN)
	w := doJSON(r, http.MethodPost, "/api/books/track",
		map[string]any{"bookId": book.ID, "status": "READING"}, cookie)
	require.Equal(t, http.StatusCreated, w.Code)
	var item itemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &item))
	path := fmt.Sprintf("/api/items/%d", item.ID)

	// Status + page progress update.
	w = doJSON(r, http.MethodPatch, path, map[string]any{"status": "COMPLETED", "progress": 320}, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var updated itemResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, "COMPLETED", updated.Status)
	assert.Equal(t, 320, updated.Progress)

	// Invalid status for a book → 400.
	w = doJSON(r, http.MethodPatch, path, map[string]any{"status": "WATCHING"}, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assertEnvelope(t, w, CodeInvalidRequest)

	// Negative progress and empty patch → 400.
	w = doJSON(r, http.MethodPatch, path, map[string]any{"progress": -3}, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)
	w = doJSON(r, http.MethodPatch, path, map[string]any{}, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Cross-user PATCH → 404, existence not leaked.
	w = doJSON(r, http.MethodPatch, path, map[string]any{"status": "PLAN_TO"}, otherCookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertEnvelope(t, w, CodeNotFound)
}

func TestDeleteItemEndpoint(t *testing.T) {
	r, gdb := newBooksEnv(t)
	cookie, userID := loginAs(t, r, gdb, "alice")
	otherCookie, otherID := loginAs(t, r, gdb, "bob")

	// Seed a TV show with episodes and watches for both users.
	show := &models.Show{TMDBID: 1399, Title: "Game of Thrones"}
	require.NoError(t, gdb.Create(show).Error)
	ep := &models.Episode{ShowID: show.ID, Season: 1, Number: 1, Title: "Winter Is Coming"}
	require.NoError(t, gdb.Create(ep).Error)
	item := &models.TrackingItem{UserID: userID, Type: "TV", ExternalID: "1399", Title: "GoT", Status: "WATCHING"}
	require.NoError(t, gdb.Create(item).Error)
	now := time.Now()
	require.NoError(t, gdb.Create(&models.EpisodeWatch{UserID: userID, EpisodeID: ep.ID, WatchedAt: now}).Error)
	require.NoError(t, gdb.Create(&models.EpisodeWatch{UserID: otherID, EpisodeID: ep.ID, WatchedAt: now}).Error)

	path := fmt.Sprintf("/api/items/%d", item.ID)

	// Cross-user DELETE → 404 and nothing is removed.
	w := doJSON(r, http.MethodDelete, path, nil, otherCookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertEnvelope(t, w, CodeNotFound)

	// Owner DELETE → 204; own watches gone, episodes/show/other watches stay.
	w = doJSON(r, http.MethodDelete, path, nil, cookie)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	var myWatches, otherWatches, episodes int64
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).Where("user_id = ?", userID).Count(&myWatches).Error)
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).Where("user_id = ?", otherID).Count(&otherWatches).Error)
	require.NoError(t, gdb.Model(&models.Episode{}).Count(&episodes).Error)
	assert.EqualValues(t, 0, myWatches)
	assert.EqualValues(t, 1, otherWatches)
	assert.EqualValues(t, 1, episodes, "shared episode metadata must stay")

	// Deleting again → 404. Malformed id → 400.
	w = doJSON(r, http.MethodDelete, path, nil, cookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	w = doJSON(r, http.MethodDelete, "/api/items/abc", nil, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assertEnvelope(t, w, CodeInvalidRequest)
}
