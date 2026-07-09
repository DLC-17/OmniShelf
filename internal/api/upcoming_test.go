package api

import (
	"encoding/json"
	"net/http"
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

func upcomingTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := gin.New()
	grp := RegisterRoutes(r, gdb, &config.Config{JWTSecret: testSecret})
	RegisterUpcomingRoutes(grp, gdb)
	return r, gdb
}

type upcomingResponse struct {
	TV     []upcomingItem `json:"tv"`
	Movies []upcomingItem `json:"movies"`
	Games  []upcomingItem `json:"games"`
	Books  []upcomingItem `json:"books"`
}

func TestUpcoming(t *testing.T) {
	r, gdb := upcomingTestRouter(t)
	userID, cookie := socialSeedUser(t, gdb, "alice")

	past := time.Now().Add(-30 * 24 * time.Hour)
	soon := time.Now().Add(7 * 24 * time.Hour)
	later := time.Now().Add(40 * 24 * time.Hour)

	// A completed show with one aired and two future episodes.
	show := models.Show{TMDBID: 1399, Title: "House of the Dragon", PosterPath: "tv/1399.jpg"}
	require.NoError(t, gdb.Create(&show).Error)
	require.NoError(t, gdb.Create(&[]models.Episode{
		{ShowID: show.ID, Season: 1, Number: 10, Title: "Aired", AirDate: &past},
		{ShowID: show.ID, Season: 2, Number: 2, Title: "Second", AirDate: &later},
		{ShowID: show.ID, Season: 2, Number: 1, Title: "Premiere", AirDate: &soon},
	}).Error)
	require.NoError(t, gdb.Create(&models.TrackingItem{
		UserID: userID, Type: "TV", ExternalID: "1399", Title: "House of the Dragon", Status: "COMPLETED",
	}).Error)

	// A plan-to show with a future episode must NOT appear on the board.
	planShow := models.Show{TMDBID: 2000, Title: "Planned Show"}
	require.NoError(t, gdb.Create(&planShow).Error)
	require.NoError(t, gdb.Create(&models.Episode{
		ShowID: planShow.ID, Season: 1, Number: 1, Title: "Pilot", AirDate: &soon,
	}).Error)
	require.NoError(t, gdb.Create(&models.TrackingItem{
		UserID: userID, Type: "TV", ExternalID: "2000", Title: "Planned Show", Status: "PLAN_TO",
	}).Error)

	// A tracked movie releasing in the future, and one already released.
	require.NoError(t, gdb.Create(&[]models.Movie{
		{TMDBID: 500, Title: "Future Film", ReleaseDate: later.Format("2006-01-02")},
		{TMDBID: 501, Title: "Old Film", ReleaseDate: past.Format("2006-01-02")},
	}).Error)
	require.NoError(t, gdb.Create(&[]models.TrackingItem{
		{UserID: userID, Type: "MOVIE", ExternalID: "500", Title: "Future Film", Status: "PLAN_TO"},
		{UserID: userID, Type: "MOVIE", ExternalID: "501", Title: "Old Film", Status: "PLAN_TO"},
	}).Error)

	// A tracked game releasing in the future, and one already released.
	require.NoError(t, gdb.Create(&[]models.Game{
		{Barcode: "111111111111", Title: "Future Game", Platform: "Switch 2", CoverPath: "game/fut.jpg", ReleaseDate: later.Format("2006-01-02")},
		{Barcode: "222222222222", Title: "Old Game", Platform: "PS5", ReleaseDate: past.Format("2006-01-02")},
	}).Error)
	require.NoError(t, gdb.Create(&[]models.TrackingItem{
		{UserID: userID, Type: "GAME", ExternalID: "111111111111", Title: "Future Game", Status: "PLAN_TO"},
		{UserID: userID, Type: "GAME", ExternalID: "222222222222", Title: "Old Game", Status: "COMPLETED"},
	}).Error)

	w := doJSON(r, http.MethodGet, "/api/upcoming", nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got upcomingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	// TV: only the show's single next upcoming episode (the earliest), not
	// every future episode — S02E02 is dropped in favor of S02E01.
	require.Len(t, got.TV, 1)
	assert.Equal(t, "House of the Dragon", got.TV[0].Title)
	assert.Equal(t, "tv/1399.jpg", got.TV[0].PosterPath)
	assert.Equal(t, "S02E01 · Premiere", got.TV[0].Detail)
	assert.Equal(t, soon.Format("2006-01-02"), got.TV[0].Date)

	// Movies: only the future release.
	require.Len(t, got.Movies, 1)
	assert.Equal(t, "Future Film", got.Movies[0].Title)
	assert.Equal(t, later.Format("2006-01-02"), got.Movies[0].Date)

	// Games: only the future release, with platform as the detail line.
	require.Len(t, got.Games, 1)
	assert.Equal(t, "Future Game", got.Games[0].Title)
	assert.Equal(t, "game/fut.jpg", got.Games[0].PosterPath)
	assert.Equal(t, "Switch 2", got.Games[0].Detail)
	assert.Equal(t, later.Format("2006-01-02"), got.Games[0].Date)

	// Books tab is always present but empty (no cached dates).
	assert.Empty(t, got.Books)
}

func TestUpcomingRequiresAuth(t *testing.T) {
	r, _ := upcomingTestRouter(t)
	w := doJSON(r, http.MethodGet, "/api/upcoming", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
