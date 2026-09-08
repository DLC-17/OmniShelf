package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/ownership"
	"github.com/davidlc1229/omnishelf/internal/tags"
)

func setupTestStatsRouter(t *testing.T, userID uint) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(userIDKey, userID)
		c.Next()
	})

	apiGrp := r.Group("/api")
	RegisterStatsRoutes(apiGrp, gdb)

	return r, gdb
}

func TestFormatDuration(t *testing.T) {
	assert.Equal(t, "0 minutes", formatDuration(0))
	assert.Equal(t, "0 minutes", formatDuration(-5))
	assert.Equal(t, "1 minute", formatDuration(1))
	assert.Equal(t, "45 minutes", formatDuration(45))
	assert.Equal(t, "1 hour", formatDuration(60))
	assert.Equal(t, "1 hour, 30 minutes", formatDuration(90))
	assert.Equal(t, "2 hours, 1 minute", formatDuration(121))
	assert.Equal(t, "1 day", formatDuration(1440))
	assert.Equal(t, "1 day, 1 hour", formatDuration(1500))
	assert.Equal(t, "1 day, 1 hour, 15 minutes", formatDuration(1515))
	assert.Equal(t, "2 days", formatDuration(2880))
	assert.Equal(t, "2 days, 3 hours, 45 minutes", formatDuration(2880+180+45))
}

func TestWrappedEmptyLibrary(t *testing.T) {
	r, _ := setupTestStatsRouter(t, 1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stats/wrapped?year=2024", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res WrappedResponse
	err := json.Unmarshal(w.Body.Bytes(), &res)
	require.NoError(t, err)

	assert.Equal(t, 2024, res.Year)
	assert.Equal(t, 0, res.TotalCompleted)
	assert.Equal(t, 0, res.Breakdown.Episodes)
	assert.Equal(t, 0, res.Breakdown.Movies)
	assert.Equal(t, 0, res.Breakdown.Games)
	assert.Equal(t, 0, res.Breakdown.Books)
	assert.Equal(t, 0, res.Breakdown.Albums)

	assert.Equal(t, 0, res.TimeSpent.TotalMinutes)
	assert.Equal(t, 0.0, res.TimeSpent.TotalHours)
	assert.Equal(t, 0.0, res.TimeSpent.TotalDays)
	assert.Equal(t, "0 minutes", res.TimeSpent.Formatted)

	assert.Empty(t, res.TopCreators)
	assert.Empty(t, res.TopGenres)

	assert.Len(t, res.MonthlyHeatmap, 12)
	for i, m := range res.MonthlyHeatmap {
		assert.Equal(t, i+1, m.Month)
		assert.Equal(t, 0, m.Count)
	}

	assert.Equal(t, 1, res.BusiestMonth.Month)
	assert.Equal(t, "January", res.BusiestMonth.MonthName)
	assert.Equal(t, 0, res.BusiestMonth.Count)

	assert.Equal(t, 0.0, res.PhysicalCollection.TotalValue)
	assert.Equal(t, 0, res.PhysicalCollection.TotalItems)
}

func TestWrappedInvalidYear(t *testing.T) {
	r, _ := setupTestStatsRouter(t, 1)

	for _, invalid := range []string{"abc", "1800", "2500", "-2024"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/stats/wrapped?year="+invalid, nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var errRes map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errRes))
		assert.Equal(t, CodeInvalidRequest, errRes["error"])
	}
}

func TestWrappedComprehensive(t *testing.T) {
	r, gdb := setupTestStatsRouter(t, 1)
	ctx := t.Context()
	tagStore := tags.NewStore(gdb)
	ownerStore := ownership.NewStore(gdb)

	// 1. TV Setup
	show1 := models.Show{TMDBID: 101, Title: "Severance"}
	require.NoError(t, gdb.Create(&show1).Error)
	require.NoError(t, tagStore.Set(ctx, tags.TypeTV, show1.ID, []string{"Sci-Fi", "Mystery"}))

	ep1 := models.Episode{ShowID: show1.ID, Season: 1, Number: 1, Title: "Good News About Hell", Runtime: 50}
	ep2 := models.Episode{ShowID: show1.ID, Season: 1, Number: 2, Title: "Half Loop", Runtime: 0} // fallback to 45
	require.NoError(t, gdb.Create(&ep1).Error)
	require.NoError(t, gdb.Create(&ep2).Error)

	show2 := models.Show{TMDBID: 102, Title: "Breaking Bad"}
	require.NoError(t, gdb.Create(&show2).Error)
	require.NoError(t, tagStore.Set(ctx, tags.TypeTV, show2.ID, []string{"Drama", "Crime"}))

	ep3 := models.Episode{ShowID: show2.ID, Season: 1, Number: 1, Title: "Pilot", Runtime: 58}
	ep2023 := models.Episode{ShowID: show1.ID, Season: 1, Number: 3, Title: "Past Episode", Runtime: 40}
	require.NoError(t, gdb.Create(&ep3).Error)
	require.NoError(t, gdb.Create(&ep2023).Error)

	// Watches in 2024
	watch1 := models.EpisodeWatch{UserID: 1, EpisodeID: ep1.ID, WatchedAt: time.Date(2024, time.March, 15, 12, 0, 0, 0, time.UTC)}
	watch2 := models.EpisodeWatch{UserID: 1, EpisodeID: ep2.ID, WatchedAt: time.Date(2024, time.March, 16, 12, 0, 0, 0, time.UTC)}
	watch3 := models.EpisodeWatch{UserID: 1, EpisodeID: ep3.ID, WatchedAt: time.Date(2024, time.April, 10, 12, 0, 0, 0, time.UTC)}
	// Watch in 2023 (should be ignored for 2024)
	watch2023 := models.EpisodeWatch{UserID: 1, EpisodeID: ep2023.ID, WatchedAt: time.Date(2023, time.December, 31, 23, 59, 0, 0, time.UTC)}
	// Watch by another user (should be ignored)
	watchOtherUser := models.EpisodeWatch{UserID: 2, EpisodeID: ep1.ID, WatchedAt: time.Date(2024, time.March, 15, 12, 0, 0, 0, time.UTC)}
	require.NoError(t, gdb.Create(&[]models.EpisodeWatch{watch1, watch2, watch3, watch2023, watchOtherUser}).Error)

	// 2. Movies Setup
	movie1 := models.Movie{TMDBID: 693134, Title: "Dune: Part Two", Runtime: 166}
	movie2 := models.Movie{TMDBID: 27205, Title: "Inception", Runtime: 0} // fallback to 120
	movie2023 := models.Movie{TMDBID: 9999, Title: "Old Movie", Runtime: 100}
	require.NoError(t, gdb.Create(&movie1).Error)
	require.NoError(t, gdb.Create(&movie2).Error)
	require.NoError(t, gdb.Create(&movie2023).Error)
	require.NoError(t, tagStore.Set(ctx, tags.TypeMovie, movie1.ID, []string{"Sci-Fi", "Adventure"}))
	require.NoError(t, tagStore.Set(ctx, tags.TypeMovie, movie2.ID, []string{"Sci-Fi"}))

	movieItem1 := models.TrackingItem{
		UserID:     1,
		Type:       "MOVIE",
		ExternalID: "693134",
		Title:      "Dune: Part Two",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.March, 20, 10, 0, 0, 0, time.UTC),
	}
	movieItem2 := models.TrackingItem{
		UserID:     1,
		Type:       "MOVIE",
		ExternalID: "27205",
		Title:      "Inception",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.May, 10, 10, 0, 0, 0, time.UTC),
	}
	movieItem2023 := models.TrackingItem{
		UserID:     1,
		Type:       "MOVIE",
		ExternalID: "9999",
		Title:      "Old Movie",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2023, time.November, 1, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, gdb.Create(&[]models.TrackingItem{movieItem1, movieItem2, movieItem2023}).Error)


	// 3. Books Setup
	book1 := models.Book{ISBN13: "9780765326355", Title: "The Way of Kings", Authors: "Brandon Sanderson", PageCount: 1007}
	book2 := models.Book{ISBN13: "9780765326362", Title: "Words of Radiance", Authors: "Brandon Sanderson", PageCount: 1087}
	book3 := models.Book{ISBN13: "9780060853983", Title: "Good Omens", Authors: "Neil Gaiman, Terry Pratchett", PageCount: 432}
	require.NoError(t, gdb.Create(&book1).Error)
	require.NoError(t, gdb.Create(&book2).Error)
	require.NoError(t, gdb.Create(&book3).Error)
	require.NoError(t, tagStore.Set(ctx, tags.TypeBook, book1.ID, []string{"Fantasy", "Epic"}))
	require.NoError(t, tagStore.Set(ctx, tags.TypeBook, book2.ID, []string{"Fantasy"}))
	require.NoError(t, tagStore.Set(ctx, tags.TypeBook, book3.ID, []string{"Fantasy", "Comedy"}))

	bookItem1 := models.TrackingItem{
		UserID:     1,
		Type:       "BOOK",
		ExternalID: "9780765326355",
		Title:      "The Way of Kings",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.March, 25, 10, 0, 0, 0, time.UTC),
	}
	bookItem2 := models.TrackingItem{
		UserID:     1,
		Type:       "BOOK",
		ExternalID: "9780765326362",
		Title:      "Words of Radiance",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.June, 15, 10, 0, 0, 0, time.UTC),
	}
	bookItem3 := models.TrackingItem{
		UserID:     1,
		Type:       "BOOK",
		ExternalID: "9780060853983",
		Title:      "Good Omens",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.June, 20, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, gdb.Create(&[]models.TrackingItem{bookItem1, bookItem2, bookItem3}).Error)

	// 4. Games Setup
	game1 := models.Game{IGDBID: 119133, Title: "Elden Ring", Developer: "FromSoftware", Runtime: 3600}
	game2 := models.Game{IGDBID: 2155, Title: "Dark Souls", Developer: "FromSoftware", Runtime: 2400}
	game3 := models.Game{IGDBID: 11208, Title: "Hollow Knight", Developer: "Team Cherry", Runtime: 1800}
	require.NoError(t, gdb.Create(&game1).Error)
	require.NoError(t, gdb.Create(&game2).Error)
	require.NoError(t, gdb.Create(&game3).Error)
	require.NoError(t, tagStore.Set(ctx, tags.TypeGame, game1.ID, []string{"RPG", "Action"}))
	require.NoError(t, tagStore.Set(ctx, tags.TypeGame, game2.ID, []string{"RPG"}))
	require.NoError(t, tagStore.Set(ctx, tags.TypeGame, game3.ID, []string{"Metroidvania", "Action"}))

	gameItem1 := models.TrackingItem{
		UserID:     1,
		Type:       "GAME",
		ExternalID: "119133",
		Title:      "Elden Ring",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.March, 10, 10, 0, 0, 0, time.UTC),
	}
	gameItem2 := models.TrackingItem{
		UserID:     1,
		Type:       "GAME",
		ExternalID: "2155",
		Title:      "Dark Souls",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.July, 12, 10, 0, 0, 0, time.UTC),
	}
	gameItem3 := models.TrackingItem{
		UserID:     1,
		Type:       "GAME",
		ExternalID: "11208",
		Title:      "Hollow Knight",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.July, 20, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, gdb.Create(&gameItem1).Error)
	require.NoError(t, gdb.Create(&gameItem2).Error)
	require.NoError(t, gdb.Create(&gameItem3).Error)
	// Mark gameItem1 as physically owned
	require.NoError(t, ownerStore.Set(ctx, ownership.TypeGame, gameItem1.ID, []string{ownership.FormatPhysical}))

	// 5. Music Setup
	album1 := models.Album{ExternalID: "discogs:1", Artist: "Pink Floyd", Title: "The Dark Side of the Moon"}
	album2 := models.Album{ExternalID: "discogs:2", Artist: "Pink Floyd", Title: "Wish You Were Here"}
	require.NoError(t, gdb.Create(&album1).Error)
	require.NoError(t, gdb.Create(&album2).Error)

	albumItem1 := models.TrackingItem{
		UserID:     1,
		Type:       "MUSIC",
		ExternalID: "discogs:1",
		Title:      "The Dark Side of the Moon",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.March, 5, 10, 0, 0, 0, time.UTC),
	}
	albumItem2 := models.TrackingItem{
		UserID:     1,
		Type:       "MUSIC",
		ExternalID: "discogs:2",
		Title:      "Wish You Were Here",
		Status:     "COMPLETED",
		UpdatedAt:  time.Date(2024, time.March, 12, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, gdb.Create(&albumItem1).Error)
	require.NoError(t, gdb.Create(&albumItem2).Error)
	// Mark albumItem1 as Vinyl
	require.NoError(t, ownerStore.Set(ctx, ownership.TypeMusic, albumItem1.ID, []string{ownership.FormatVinyl}))

	// 6. Cards Setup
	card1 := models.Card{ExternalID: "ygo:LOB-001", Name: "Blue-Eyes White Dragon", Game: "YUGIOH", Price: 50.0}
	card2 := models.Card{ExternalID: "ptcg:base1-4", Name: "Charizard", Game: "POKEMON", Price: 250.0}
	require.NoError(t, gdb.Create(&card1).Error)
	require.NoError(t, gdb.Create(&card2).Error)

	cardItem1 := models.TrackingItem{
		UserID:     1,
		Type:       "CARD",
		ExternalID: "ygo:LOB-001",
		Title:      "Blue-Eyes White Dragon",
		Status:     "OWNED",
		UpdatedAt:  time.Date(2024, time.February, 10, 10, 0, 0, 0, time.UTC),
	}
	cardItem2 := models.TrackingItem{
		UserID:     1,
		Type:       "CARD",
		ExternalID: "ptcg:base1-4",
		Title:      "Charizard",
		Status:     "OWNED",
		UpdatedAt:  time.Date(2024, time.August, 15, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, gdb.Create(&cardItem1).Error)
	require.NoError(t, gdb.Create(&cardItem2).Error)


	// Execute GET /api/stats/wrapped?year=2024
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stats/wrapped?year=2024", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res WrappedResponse
	err := json.Unmarshal(w.Body.Bytes(), &res)
	require.NoError(t, err)

	// Verify Year and Counts
	assert.Equal(t, 2024, res.Year)
	assert.Equal(t, 13, res.TotalCompleted)
	assert.Equal(t, 3, res.Breakdown.Episodes)
	assert.Equal(t, 2, res.Breakdown.Movies)
	assert.Equal(t, 3, res.Breakdown.Games)
	assert.Equal(t, 3, res.Breakdown.Books)
	assert.Equal(t, 2, res.Breakdown.Albums)

	// Verify Time Spent
	// TV: 50 + 45 + 58 = 153 min
	// Movies: 166 + 120 = 286 min
	// Books: 1007 + 1087 + 432 = 2526 min
	// Games: 3600 + 2400 + 1800 = 7800 min
	// Music: 2 * 45 = 90 min
	// Total = 153 + 286 + 2526 + 7800 + 90 = 10855 min
	assert.Equal(t, 153, res.TimeSpent.MinutesTV)
	assert.Equal(t, 286, res.TimeSpent.MinutesMovie)
	assert.Equal(t, 2526, res.TimeSpent.MinutesBook)
	assert.Equal(t, 7800, res.TimeSpent.MinutesGame)
	assert.Equal(t, 90, res.TimeSpent.MinutesMusic)
	assert.Equal(t, 10855, res.TimeSpent.TotalMinutes)
	assert.Equal(t, 180.92, res.TimeSpent.TotalHours)
	assert.Equal(t, 7.54, res.TimeSpent.TotalDays)
	assert.Equal(t, "7 days, 12 hours, 55 minutes", res.TimeSpent.Formatted)

	// Verify Top Creators
	// Brandon Sanderson (2, Author), FromSoftware (2, Developer), Pink Floyd (2, Artist),
	// Neil Gaiman (1, Author), Terry Pratchett (1, Author), Team Cherry (1, Developer)
	require.NotEmpty(t, res.TopCreators)
	creatorMap := make(map[string]CreatorStat)
	for _, c := range res.TopCreators {
		creatorMap[c.Name] = c
	}
	assert.Equal(t, 2, creatorMap["Brandon Sanderson"].Count)
	assert.Equal(t, "Author", creatorMap["Brandon Sanderson"].Category)
	assert.Equal(t, 2, creatorMap["FromSoftware"].Count)
	assert.Equal(t, "Developer", creatorMap["FromSoftware"].Category)
	assert.Equal(t, 2, creatorMap["Pink Floyd"].Count)
	assert.Equal(t, "Artist", creatorMap["Pink Floyd"].Category)
	assert.Equal(t, 1, creatorMap["Team Cherry"].Count)
	assert.Equal(t, "Developer", creatorMap["Team Cherry"].Category)

	// Verify Top Genres
	require.NotEmpty(t, res.TopGenres)
	genreMap := make(map[string]int)
	for _, g := range res.TopGenres {
		genreMap[g.Name] = g.Count
	}
	assert.GreaterOrEqual(t, genreMap["Sci-Fi"], 2) // Severance, Dune, Inception
	assert.GreaterOrEqual(t, genreMap["Fantasy"], 2)
	assert.GreaterOrEqual(t, genreMap["RPG"], 2)

	// Verify Monthly Heatmap
	assert.Len(t, res.MonthlyHeatmap, 12)
	// March: 2 episodes + 1 movie + 1 book + 1 game + 2 albums = 7 completions
	assert.Equal(t, 7, res.MonthlyHeatmap[2].Count) // March is index 2
	assert.Equal(t, "March", res.MonthlyHeatmap[2].MonthName)
	assert.Equal(t, 2, res.MonthlyHeatmap[2].Breakdown.Episodes)
	assert.Equal(t, 1, res.MonthlyHeatmap[2].Breakdown.Movies)
	assert.Equal(t, 1, res.MonthlyHeatmap[2].Breakdown.Books)
	assert.Equal(t, 1, res.MonthlyHeatmap[2].Breakdown.Games)
	assert.Equal(t, 2, res.MonthlyHeatmap[2].Breakdown.Albums)

	// Verify Busiest Month
	assert.Equal(t, 3, res.BusiestMonth.Month)
	assert.Equal(t, "March", res.BusiestMonth.MonthName)
	assert.Equal(t, 7, res.BusiestMonth.Count)

	// Verify Physical Collection Added in 2024
	// Cards: 2 cards = $300.00
	// Vinyl: 1 vinyl = $30.00
	// Game: 1 physical game = $60.00
	// Total: $390.00, 4 items
	assert.Equal(t, 300.0, res.PhysicalCollection.CardValue)
	assert.Equal(t, 2, res.PhysicalCollection.CardCount)
	assert.Equal(t, 30.0, res.PhysicalCollection.VinylValue)
	assert.Equal(t, 1, res.PhysicalCollection.VinylCount)
	assert.Equal(t, 60.0, res.PhysicalCollection.GameValue)
	assert.Equal(t, 1, res.PhysicalCollection.GameCount)
	assert.Equal(t, 390.0, res.PhysicalCollection.TotalValue)
	assert.Equal(t, 4, res.PhysicalCollection.TotalItems)
}

func TestStatsTimeSpentAndHeatmap(t *testing.T) {
	r, gdb := setupTestStatsRouter(t, 1)

	// Test GET /api/stats/time-spent
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stats/time-spent", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var ts timeSpentRes
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ts))
	assert.Equal(t, 0, ts.MinutesTV)

	// Test GET /api/stats/heatmap
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/stats/heatmap", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test GET /api/stats/badges
	badgeModel := models.Badge{Slug: "master-watcher", Name: "Master Watcher", Description: "Watched 100 episodes"}
	require.NoError(t, gdb.Create(&badgeModel).Error)
	userBadge := models.UserBadge{UserID: 1, BadgeID: badgeModel.ID, EarnedAt: time.Now()}
	require.NoError(t, gdb.Create(&userBadge).Error)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/stats/badges", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var badges []badge
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &badges))
	require.Len(t, badges, 1)
	assert.Equal(t, "master-watcher", badges[0].ID)
	assert.Equal(t, "Master Watcher", badges[0].Name)
}
