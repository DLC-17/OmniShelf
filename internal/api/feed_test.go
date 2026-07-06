package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// ── shared fixtures for the social endpoints (feed.go / users.go) ──

func socialTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := gin.New()
	grp := RegisterRoutes(r, gdb, &config.Config{JWTSecret: testSecret})
	RegisterFeedRoutes(grp, gdb)
	RegisterUserRoutes(grp, gdb)
	return r, gdb
}

// socialSeedUser inserts a user directly and mints a valid session cookie.
func socialSeedUser(t *testing.T, gdb *gorm.DB, username string) (uint, *http.Cookie) {
	t.Helper()
	u := models.User{Username: username, PasswordHash: "irrelevant"}
	require.NoError(t, gdb.Create(&u).Error)
	tok, err := newToken([]byte(testSecret), u.ID, time.Now())
	require.NoError(t, err)
	return u.ID, &http.Cookie{Name: CookieName, Value: tok}
}

func socialSeedShow(t *testing.T, gdb *gorm.DB, tmdbID int, title string, episodes int) []models.Episode {
	t.Helper()
	show := models.Show{TMDBID: tmdbID, Title: title}
	require.NoError(t, gdb.Create(&show).Error)
	eps := make([]models.Episode, episodes)
	for i := range eps {
		eps[i] = models.Episode{ShowID: show.ID, Season: 1, Number: i + 1}
	}
	require.NoError(t, gdb.Create(&eps).Error)
	return eps
}

func socialSeedWatch(t *testing.T, gdb *gorm.DB, userID, episodeID uint, at time.Time) {
	t.Helper()
	require.NoError(t, gdb.Create(&models.EpisodeWatch{UserID: userID, EpisodeID: episodeID, WatchedAt: at}).Error)
}

// socialSeedItem creates a TrackingItem and pins its UpdatedAt to `at`
// (UpdateColumn bypasses GORM's auto-update of the timestamp).
func socialSeedItem(t *testing.T, gdb *gorm.DB, userID uint, typ, extID, title, status string, at time.Time) {
	t.Helper()
	it := models.TrackingItem{UserID: userID, Type: typ, ExternalID: extID, Title: title, Status: status}
	require.NoError(t, gdb.Create(&it).Error)
	require.NoError(t, gdb.Model(&models.TrackingItem{}).Where("id = ?", it.ID).
		UpdateColumn("updated_at", at).Error)
}

type feedPageEntry struct {
	User struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
	Action string `json:"action"`
	Media  struct {
		Type  string `json:"type"`
		Title string `json:"title"`
		ID    string `json:"id"`
	} `json:"media"`
	Timestamp time.Time `json:"timestamp"`
}

type feedPage struct {
	Entries    []feedPageEntry `json:"entries"`
	NextBefore *string         `json:"nextBefore"`
}

func getFeed(t *testing.T, r *gin.Engine, cookie *http.Cookie, query url.Values) feedPage {
	t.Helper()
	w := doJSON(r, http.MethodGet, "/api/feed?"+query.Encode(), nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page feedPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	return page
}

// seedInterleavedFeed builds two users' interleaved activity, including a
// five-row identical-timestamp cluster to exercise the cursor tiebreaker.
// It returns the routers' expected actions in feed (newest-first) order and
// the tie-cluster timestamp.
func seedInterleavedFeed(t *testing.T, gdb *gorm.DB, u1, u2 uint) ([]string, time.Time) {
	t.Helper()
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	tie := base.Add(10 * time.Minute)
	eps := socialSeedShow(t, gdb, 1399, "Game of Thrones", 8)

	socialSeedWatch(t, gdb, u1, eps[0].ID, base.Add(1*time.Minute)) // w1
	socialSeedWatch(t, gdb, u2, eps[1].ID, base.Add(2*time.Minute)) // w2
	socialSeedWatch(t, gdb, u1, eps[2].ID, base.Add(3*time.Minute)) // w3
	socialSeedItem(t, gdb, u1, "TV", "2316", "Severance", "WATCHING", base.Add(4*time.Minute))
	socialSeedItem(t, gdb, u2, "BOOK", "9780441172719", "Dune", "COMPLETED", base.Add(5*time.Minute))
	// Identical-timestamp cluster: three watches + two items.
	socialSeedWatch(t, gdb, u1, eps[3].ID, tie) // w4
	socialSeedWatch(t, gdb, u2, eps[4].ID, tie) // w5
	socialSeedWatch(t, gdb, u1, eps[5].ID, tie) // w6
	socialSeedItem(t, gdb, u1, "BOOK", "9780553283686", "Hyperion", "READING", tie)
	socialSeedItem(t, gdb, u2, "TV", "1438", "The Wire", "PLAN_TO", tie)

	// Total order: timestamp desc, watches before items on ties, row id desc.
	want := []string{
		"watched S01E06 of Game of Thrones", // w6 (u1)
		"watched S01E05 of Game of Thrones", // w5 (u2)
		"watched S01E04 of Game of Thrones", // w4 (u1)
		"plans to watch The Wire",           // i4 (u2)
		"is reading Hyperion",               // i3 (u1)
		"finished book Dune",                // i2 (u2)
		"is watching Severance",             // i1 (u1)
		"watched S01E03 of Game of Thrones", // w3 (u1)
		"watched S01E02 of Game of Thrones", // w2 (u2)
		"watched S01E01 of Game of Thrones", // w1 (u1)
	}
	return want, tie
}

func feedActions(page feedPage) []string {
	out := make([]string, 0, len(page.Entries))
	for _, e := range page.Entries {
		out = append(out, e.Action)
	}
	return out
}

// ── tests ──

func TestFeedMergesUsersReverseChronological(t *testing.T) {
	r, gdb := socialTestRouter(t)
	u1, cookie := socialSeedUser(t, gdb, "alice")
	u2, _ := socialSeedUser(t, gdb, "bob")
	want, _ := seedInterleavedFeed(t, gdb, u1, u2)

	page := getFeed(t, r, cookie, url.Values{"limit": {"100"}})
	require.Equal(t, want, feedActions(page))
	assert.Nil(t, page.NextBefore)

	// Strictly non-increasing timestamps.
	for i := 1; i < len(page.Entries); i++ {
		assert.False(t, page.Entries[i].Timestamp.After(page.Entries[i-1].Timestamp),
			"entry %d is newer than entry %d", i, i-1)
	}

	// Both users appear; entry fields are populated per spec §2.7.
	first := page.Entries[0]
	assert.Equal(t, u1, first.User.ID)
	assert.Equal(t, "alice", first.User.Username)
	assert.Equal(t, "TV", first.Media.Type)
	assert.Equal(t, "Game of Thrones", first.Media.Title)
	assert.Equal(t, "1399", first.Media.ID)

	wire := page.Entries[3]
	assert.Equal(t, u2, wire.User.ID)
	assert.Equal(t, "bob", wire.User.Username)
	assert.Equal(t, "TV", wire.Media.Type)
	assert.Equal(t, "1438", wire.Media.ID)

	dune := page.Entries[5]
	assert.Equal(t, "BOOK", dune.Media.Type)
	assert.Equal(t, "9780441172719", dune.Media.ID)
}

// TestFeedCursorPagination pages through the feed with limit=3 (the page
// boundary falls inside the identical-timestamp cluster) and asserts the
// concatenated pages exactly equal the single full fetch: no duplicates,
// no gaps.
func TestFeedCursorPagination(t *testing.T) {
	r, gdb := socialTestRouter(t)
	u1, cookie := socialSeedUser(t, gdb, "alice")
	u2, _ := socialSeedUser(t, gdb, "bob")
	want, _ := seedInterleavedFeed(t, gdb, u1, u2)

	var paged []string
	q := url.Values{"limit": {"3"}}
	for pages := 0; ; pages++ {
		require.Less(t, pages, 10, "pagination did not terminate")
		page := getFeed(t, r, cookie, q)
		paged = append(paged, feedActions(page)...)
		if page.NextBefore == nil {
			break
		}
		q = url.Values{"limit": {"3"}, "before": {*page.NextBefore}}
	}
	assert.Equal(t, want, paged, "pages must concatenate to the full feed with no duplicates or gaps")
}

// TestFeedBareTimestampCursor uses a plain RFC3339 `before` (spec §2.7):
// entries strictly before that instant — the whole tie cluster is excluded.
func TestFeedBareTimestampCursor(t *testing.T) {
	r, gdb := socialTestRouter(t)
	u1, cookie := socialSeedUser(t, gdb, "alice")
	u2, _ := socialSeedUser(t, gdb, "bob")
	want, tie := seedInterleavedFeed(t, gdb, u1, u2)

	page := getFeed(t, r, cookie, url.Values{"before": {tie.Format(time.RFC3339Nano)}})
	assert.Equal(t, want[5:], feedActions(page), "tie-cluster rows must be strictly excluded")
}

func TestFeedLimitDefaultsAndClamping(t *testing.T) {
	r, gdb := socialTestRouter(t)
	u1, cookie := socialSeedUser(t, gdb, "alice")
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 120; i++ {
		socialSeedItem(t, gdb, u1, "BOOK", fmt.Sprintf("978%010d", i), fmt.Sprintf("Book %d", i),
			"READING", base.Add(time.Duration(i)*time.Second))
	}

	// Default limit 20.
	page := getFeed(t, r, cookie, url.Values{})
	assert.Len(t, page.Entries, 20)
	require.NotNil(t, page.NextBefore)

	// limit above the cap is clamped to 100.
	page = getFeed(t, r, cookie, url.Values{"limit": {"500"}})
	assert.Len(t, page.Entries, 100)

	// Invalid limits and cursors → 400 envelope.
	for _, qs := range []string{"limit=0", "limit=-5", "limit=abc", "before=not-a-time"} {
		w := doJSON(r, http.MethodGet, "/api/feed?"+qs, nil, cookie)
		require.Equal(t, http.StatusBadRequest, w.Code, qs)
		assertEnvelope(t, w, CodeInvalidRequest)
	}
}

func TestFeedRequiresAuth(t *testing.T) {
	r, _ := socialTestRouter(t)
	w := doJSON(r, http.MethodGet, "/api/feed", nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assertEnvelope(t, w, CodeUnauthorized)
}

func TestFeedEmpty(t *testing.T) {
	r, gdb := socialTestRouter(t)
	_, cookie := socialSeedUser(t, gdb, "alice")
	page := getFeed(t, r, cookie, url.Values{})
	assert.NotNil(t, page.Entries, "entries must serialize as [], not null")
	assert.Empty(t, page.Entries)
	assert.Nil(t, page.NextBefore)
}
