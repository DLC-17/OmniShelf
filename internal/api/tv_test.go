package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/images"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
	"github.com/davidlc1229/omnishelf/internal/tv"
)

// tvAirDay formats an air date offset in days from today, TMDB-style.
func tvAirDay(offset int) string {
	return time.Now().AddDate(0, 0, offset).Format("2006-01-02")
}

// newTMDBFixture serves recorded-style TMDB JSON for show 100 (two seasons:
// S1 fully aired, S2 with one aired, one future, one unannounced episode)
// plus its poster image. No live API is ever hit (testing standard).
func newTMDBFixture(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(v))
	}
	mux.HandleFunc("/search/tv", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, tmdb.SearchResponse{
			Page:         1,
			TotalResults: 1,
			TotalPages:   1,
			Results: []tmdb.SearchResult{
				{ID: 100, Name: "Fixture Show", Overview: "A test show", FirstAirDate: tvAirDay(-30), PosterPath: "/p100.jpg"},
			},
		})
	})
	mux.HandleFunc("/tv/100", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, tmdb.Show{
			ID: 100, Name: "Fixture Show", Status: "Returning Series", PosterPath: "/p100.jpg",
			Seasons: []tmdb.SeasonSummary{
				{SeasonNumber: 1, EpisodeCount: 2},
				{SeasonNumber: 2, EpisodeCount: 3},
			},
		})
	})
	mux.HandleFunc("/tv/100/season/1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, tmdb.Season{SeasonNumber: 1, Episodes: []tmdb.Episode{
			{SeasonNumber: 1, EpisodeNumber: 1, Name: "S1E1", AirDate: tvAirDay(-30)},
			{SeasonNumber: 1, EpisodeNumber: 2, Name: "S1E2", AirDate: tvAirDay(-20)},
		}})
	})
	mux.HandleFunc("/tv/100/season/2", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, tmdb.Season{SeasonNumber: 2, Episodes: []tmdb.Episode{
			{SeasonNumber: 2, EpisodeNumber: 1, Name: "S2E1", AirDate: tvAirDay(-5)},
			{SeasonNumber: 2, EpisodeNumber: 2, Name: "S2E2", AirDate: tvAirDay(30)},
			{SeasonNumber: 2, EpisodeNumber: 3, Name: "S2E3", AirDate: ""},
		}})
	})
	mux.HandleFunc("/p100.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg-bytes"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"status_message":"not found"}`, http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newTVEnv wires a real router (auth + TV routes), a real SQLite temp DB,
// the real TMDB client pointed at the fixture server, and a real image
// store rooted at a temp dir. Returns the images root for cache assertions.
func newTVEnv(t *testing.T, fixtureURL string) (*gin.Engine, *gorm.DB, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	imagesRoot := t.TempDir()
	client := tmdb.New("test-key", tmdb.WithBaseURL(fixtureURL))
	svc := tv.New(gdb, client, images.New(imagesRoot), tv.WithImageBaseURL(fixtureURL))

	r := gin.New()
	grp := RegisterRoutes(r, gdb, &config.Config{JWTSecret: testSecret})
	RegisterTVRoutes(grp, svc)
	return r, gdb, imagesRoot
}

// tvLogin registers (via a fresh invite) and logs in a user, returning the
// session cookie.
func tvLogin(t *testing.T, r *gin.Engine, gdb *gorm.DB, username string) *http.Cookie {
	t.Helper()
	code := "TV-" + username
	seedInvite(t, gdb, code)
	w := doJSON(r, http.MethodPost, "/api/auth/register", registerBody(username, code))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	w = doJSON(r, http.MethodPost, "/api/auth/login",
		map[string]string{"username": username, "password": "hunter2hunter2"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

type tvUpNextPayload struct {
	Items []struct {
		Show struct {
			ID         uint   `json:"id"`
			TMDBID     int    `json:"tmdbId"`
			Title      string `json:"title"`
			PosterPath string `json:"posterPath"`
		} `json:"show"`
		Episode tvEpisodePayload `json:"episode"`
	} `json:"items"`
}

type tvEpisodePayload struct {
	ID      uint    `json:"id"`
	ShowID  uint    `json:"showId"`
	Season  int     `json:"season"`
	Number  int     `json:"number"`
	Title   string  `json:"title"`
	AirDate *string `json:"airDate"`
}

func tvAddShow(t *testing.T, r *gin.Engine, cookie *http.Cookie) {
	t.Helper()
	w := doJSON(r, http.MethodPost, "/api/tv/shows", map[string]int{"tmdbId": 100}, cookie)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func tvGetUpNext(t *testing.T, r *gin.Engine, cookie *http.Cookie) tvUpNextPayload {
	t.Helper()
	w := doJSON(r, http.MethodGet, "/api/tv/up-next", nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page tvUpNextPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	return page
}

// ── tests ──

func TestTVRoutesRequireAuth(t *testing.T) {
	r, _, _ := newTVEnv(t, newTMDBFixture(t).URL)
	for _, req := range []struct{ method, path string }{
		{http.MethodGet, "/api/tv/search?q=x"},
		{http.MethodPost, "/api/tv/shows"},
		{http.MethodGet, "/api/tv/up-next"},
		{http.MethodPost, "/api/tv/episodes/1/watch"},
		{http.MethodDelete, "/api/tv/episodes/1/watch"},
	} {
		w := doJSON(r, req.method, req.path, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", req.method, req.path)
		assertEnvelope(t, w, CodeUnauthorized)
	}
}

func TestTVSearchEndpoint(t *testing.T) {
	r, gdb, _ := newTVEnv(t, newTMDBFixture(t).URL)
	cookie := tvLogin(t, r, gdb, "searcher")

	w := doJSON(r, http.MethodGet, "/api/tv/search?q=fixture", nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Results []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Results, 1)
	assert.Equal(t, 100, body.Results[0].ID)
	assert.Equal(t, "Fixture Show", body.Results[0].Name)

	// Missing/blank q → 400 envelope.
	w = doJSON(r, http.MethodGet, "/api/tv/search?q=+", nil, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assertEnvelope(t, w, CodeInvalidRequest)
}

func TestTVAddShowEndpoint(t *testing.T) {
	r, gdb, imagesRoot := newTVEnv(t, newTMDBFixture(t).URL)
	cookie := tvLogin(t, r, gdb, "adder")

	w := doJSON(r, http.MethodPost, "/api/tv/shows", map[string]int{"tmdbId": 100}, cookie)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var body struct {
		Show struct {
			TMDBID     int    `json:"tmdbId"`
			Title      string `json:"title"`
			PosterPath string `json:"posterPath"`
		} `json:"show"`
		Item struct {
			Type       string `json:"type"`
			ExternalID string `json:"externalId"`
			Status     string `json:"status"`
		} `json:"item"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 100, body.Show.TMDBID)
	assert.Equal(t, "Fixture Show", body.Show.Title)
	assert.Equal(t, "tv/100.jpg", body.Show.PosterPath)
	assert.Equal(t, "TV", body.Item.Type)
	assert.Equal(t, "100", body.Item.ExternalID)
	assert.Equal(t, "WATCHING", body.Item.Status)

	// Poster really cached on disk under the images root.
	_, err := os.Stat(filepath.Join(imagesRoot, "tv", "100.jpg"))
	require.NoError(t, err, "poster must be downloaded to the image cache")

	// Duplicate add → 409 with the existing item (E16).
	w = doJSON(r, http.MethodPost, "/api/tv/shows", map[string]int{"tmdbId": 100}, cookie)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assertEnvelope(t, w, CodeDuplicateItem)
	var dup struct {
		Item struct {
			ExternalID string `json:"externalId"`
		} `json:"item"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &dup))
	assert.Equal(t, "100", dup.Item.ExternalID, "409 body must carry the existing item")

	// Bad bodies → 400.
	for _, bad := range []any{nil, map[string]int{"tmdbId": 0}, map[string]string{"tmdbId": "x"}} {
		w = doJSON(r, http.MethodPost, "/api/tv/shows", bad, cookie)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assertEnvelope(t, w, CodeInvalidRequest)
	}
}

// TestTVUpstreamDown exercises E3: TMDB unreachable during an interactive
// call → 502 in the standard envelope.
func TestTVUpstreamDown(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close() // connection refused from here on
	r, gdb, _ := newTVEnv(t, dead.URL)
	cookie := tvLogin(t, r, gdb, "outage")

	w := doJSON(r, http.MethodGet, "/api/tv/search?q=x", nil, cookie)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	assertEnvelope(t, w, CodeTMDBUnavailable)

	w = doJSON(r, http.MethodPost, "/api/tv/shows", map[string]int{"tmdbId": 100}, cookie)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	assertEnvelope(t, w, CodeTMDBUnavailable)
}

func TestTVUpNextAndWatchToggle(t *testing.T) {
	r, gdb, _ := newTVEnv(t, newTMDBFixture(t).URL)
	cookie := tvLogin(t, r, gdb, "watcher")
	tvAddShow(t, r, cookie)

	// Up Next starts at S1E1; future (S2E2) and unannounced (S2E3)
	// episodes must never surface.
	page := tvGetUpNext(t, r, cookie)
	require.Len(t, page.Items, 1)
	first := page.Items[0].Episode
	assert.Equal(t, 1, first.Season)
	assert.Equal(t, 1, first.Number)
	assert.Equal(t, "Fixture Show", page.Items[0].Show.Title)

	// One-tap watch → response carries the new next-up (S1E2) so the UI
	// swaps the card in place.
	w := doJSON(r, http.MethodPost, fmt.Sprintf("/api/tv/episodes/%d/watch", first.ID), nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var toggled struct {
		NextUp *tvEpisodePayload `json:"nextUp"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &toggled))
	require.NotNil(t, toggled.NextUp)
	assert.Equal(t, 1, toggled.NextUp.Season)
	assert.Equal(t, 2, toggled.NextUp.Number)

	// Idempotent: re-marking the same episode changes nothing.
	w = doJSON(r, http.MethodPost, fmt.Sprintf("/api/tv/episodes/%d/watch", first.ID), nil, cookie)
	require.Equal(t, http.StatusOK, w.Code)
	var watchRows int64
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).Where("episode_id = ?", first.ID).Count(&watchRows).Error)
	assert.EqualValues(t, 1, watchRows)

	// Un-watch restores S1E1 as next-up, in the response and in Up Next.
	w = doJSON(r, http.MethodDelete, fmt.Sprintf("/api/tv/episodes/%d/watch", first.ID), nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &toggled))
	require.NotNil(t, toggled.NextUp)
	assert.Equal(t, first.ID, toggled.NextUp.ID)
	page = tvGetUpNext(t, r, cookie)
	require.Len(t, page.Items, 1)
	assert.Equal(t, first.ID, page.Items[0].Episode.ID)

	// Watching every aired episode empties Up Next (show omitted) and the
	// final toggle response has nextUp: null.
	for {
		page = tvGetUpNext(t, r, cookie)
		if len(page.Items) == 0 {
			break
		}
		w = doJSON(r, http.MethodPost, fmt.Sprintf("/api/tv/episodes/%d/watch", page.Items[0].Episode.ID), nil, cookie)
		require.Equal(t, http.StatusOK, w.Code)
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &toggled))
	assert.Nil(t, toggled.NextUp, "last aired watch must report nextUp null")

	// Aired episodes only: exactly 3 of the 5 fixture episodes were watchable.
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).Count(&watchRows).Error)
	assert.EqualValues(t, 3, watchRows)
}

func TestTVWatchValidation(t *testing.T) {
	r, gdb, _ := newTVEnv(t, newTMDBFixture(t).URL)
	cookie := tvLogin(t, r, gdb, "validator")

	// Non-numeric id → 400.
	w := doJSON(r, http.MethodPost, "/api/tv/episodes/abc/watch", nil, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assertEnvelope(t, w, CodeInvalidRequest)

	// Unknown episode → 404.
	w = doJSON(r, http.MethodPost, "/api/tv/episodes/424242/watch", nil, cookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertEnvelope(t, w, CodeNotFound)

	w = doJSON(r, http.MethodDelete, "/api/tv/episodes/424242/watch", nil, cookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertEnvelope(t, w, CodeNotFound)
}
