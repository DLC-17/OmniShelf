package api

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
)

func RegisterStatsRoutes(grp *gin.RouterGroup, gdb *gorm.DB) {
	h := &statsHandler{db: gdb}
	grp.GET("/stats/time-spent", h.timeSpent)
	grp.GET("/stats/heatmap", h.heatmap)
	grp.GET("/stats/badges", h.badges)
	grp.GET("/stats/wrapped", h.wrapped)
}

type statsHandler struct {
	db *gorm.DB
}

type timeSpentRes struct {
	MinutesTV    int `json:"minutesTv"`
	MinutesMovie int `json:"minutesMovie"`
	MinutesBook  int `json:"minutesBook"`
	MinutesGame  int `json:"minutesGame"`
}

func (h *statsHandler) timeSpent(c *gin.Context) {
	uid := CurrentUserID(c)

	// 1. TV Shows: sum actual episode runtimes (fallback to 45 min per episode if episode runtime is 0)
	var tvRes struct {
		TotalMinutes int
	}
	h.db.Table("episode_watches").
		Select("SUM(CASE WHEN episodes.runtime > 0 THEN episodes.runtime ELSE 45 END) as total_minutes").
		Joins("JOIN episodes ON episode_watches.episode_id = episodes.id").
		Where("episode_watches.user_id = ?", uid).
		Scan(&tvRes)

	// 2. Movies: sum movie runtimes for completed movies (fallback to 120 min if movie runtime is 0)
	var movieRes struct {
		TotalMinutes int
	}
	h.db.Table("tracking_items").
		Select("SUM(CASE WHEN movies.runtime > 0 THEN movies.runtime ELSE 120 END) as total_minutes").
		Joins("JOIN movies ON movies.tmdb_id = CAST(tracking_items.external_id AS INTEGER)").
		Where("tracking_items.user_id = ? AND tracking_items.type = ? AND tracking_items.status = ?", uid, "MOVIE", "COMPLETED").
		Scan(&movieRes)

	// 3. Books: sum page counts (1 page = 1 min)
	var bRes struct {
		TotalPages int
	}
	h.db.Table("tracking_items").
		Select("SUM(books.page_count) as total_pages").
		Joins("JOIN books ON tracking_items.external_id = books.isbn13").
		Where("tracking_items.user_id = ? AND tracking_items.type = ?", uid, "BOOK").
		Scan(&bRes)

	// 4. Games: sum game completion runtimes for completed games (fallback to 1200 min / 20 hrs if runtime is 0)
	var gameRes struct {
		TotalMinutes int
	}
	h.db.Table("tracking_items").
		Select("SUM(CASE WHEN games.runtime > 0 THEN games.runtime ELSE 1200 END) as total_minutes").
		Joins("JOIN games ON games.igdb_id = CAST(tracking_items.external_id AS INTEGER) OR games.barcode = tracking_items.external_id").
		Where("tracking_items.user_id = ? AND tracking_items.type = ? AND tracking_items.status = ?", uid, "GAME", "COMPLETED").
		Scan(&gameRes)

	res := timeSpentRes{
		MinutesTV:    tvRes.TotalMinutes,
		MinutesMovie: movieRes.TotalMinutes,
		MinutesBook:  bRes.TotalPages,
		MinutesGame:  gameRes.TotalMinutes,
	}
	c.JSON(http.StatusOK, res)
}

type heatmapEntry struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int    `json:"count"`
}

func (h *statsHandler) heatmap(c *gin.Context) {
	uid := CurrentUserID(c)
	now := time.Now()
	cutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var epDates []time.Time
	h.db.Model(&models.EpisodeWatch{}).
		Where("user_id = ? AND watched_at >= ?", uid, cutoff).
		Pluck("watched_at", &epDates)

	var trDates []time.Time
	h.db.Model(&models.TrackingItem{}).
		Where("user_id = ? AND updated_at >= ?", uid, cutoff).
		Pluck("updated_at", &trDates)

	counts := make(map[string]int)
	for _, t := range epDates {
		counts[t.Format("2006-01-02")]++
	}
	for _, t := range trDates {
		counts[t.Format("2006-01-02")]++
	}

	res := make([]heatmapEntry, 0, len(counts))
	for k, v := range counts {
		res = append(res, heatmapEntry{Date: k, Count: v})
	}
	c.JSON(http.StatusOK, res)
}

type badge struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	EarnedAt    string `json:"earnedAt"`
	Icon        string `json:"icon"`
}

func (h *statsHandler) badges(c *gin.Context) {
	uid := CurrentUserID(c)

	type badgeRow struct {
		Slug        string
		Name        string
		Description string
		EarnedAt    time.Time
		IconPath    string
	}
	var rows []badgeRow
	h.db.Table("user_badges").
		Select("badges.slug, badges.name, badges.description, user_badges.earned_at, badges.icon_path").
		Joins("JOIN badges ON badges.id = user_badges.badge_id").
		Where("user_badges.user_id = ?", uid).
		Scan(&rows)

	res := make([]badge, 0, len(rows))
	for _, r := range rows {
		icon := r.IconPath
		if icon == "" {
			icon = "🏆"
		}
		res = append(res, badge{
			ID:          r.Slug,
			Name:        r.Name,
			Description: r.Description,
			EarnedAt:    r.EarnedAt.Format(time.RFC3339),
			Icon:        icon,
		})
	}
	c.JSON(http.StatusOK, res)
}

type CompletionBreakdown struct {
	Episodes int `json:"episodes"`
	Movies   int `json:"movies"`
	Games    int `json:"games"`
	Books    int `json:"books"`
	Albums   int `json:"albums"`
}

type WrappedTimeSpent struct {
	TotalMinutes int     `json:"totalMinutes"`
	TotalHours   float64 `json:"totalHours"`
	TotalDays    float64 `json:"totalDays"`
	Formatted    string  `json:"formatted"`
	MinutesTV    int     `json:"minutesTv"`
	MinutesMovie int     `json:"minutesMovie"`
	MinutesBook  int     `json:"minutesBook"`
	MinutesGame  int     `json:"minutesGame"`
	MinutesMusic int     `json:"minutesMusic"`
}

type CreatorStat struct {
	Name     string `json:"name"`
	Category string `json:"category"` // "Author", "Developer", "Artist"
	Count    int    `json:"count"`
}

type GenreStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type MonthlyHeatmapEntry struct {
	Month     int                 `json:"month"`
	MonthName string              `json:"monthName"`
	Count     int                 `json:"count"`
	Breakdown CompletionBreakdown `json:"breakdown"`
}

type BusiestMonth struct {
	Month     int    `json:"month"`
	MonthName string `json:"monthName"`
	Count     int    `json:"count"`
}

type PhysicalCollectionStats struct {
	TotalValue float64 `json:"totalValue"`
	TotalItems int     `json:"totalItems"`
	CardValue  float64 `json:"cardValue"`
	CardCount  int     `json:"cardCount"`
	VinylValue float64 `json:"vinylValue"`
	VinylCount int     `json:"vinylCount"`
	GameValue  float64 `json:"gameValue"`
	GameCount  int     `json:"gameCount"`
}

type WrappedResponse struct {
	Year               int                     `json:"year"`
	TotalCompleted     int                     `json:"totalCompleted"`
	Breakdown          CompletionBreakdown     `json:"breakdown"`
	TimeSpent          WrappedTimeSpent        `json:"timeSpent"`
	TopCreators        []CreatorStat           `json:"topCreators"`
	TopGenres          []GenreStat             `json:"topGenres"`
	MonthlyHeatmap     []MonthlyHeatmapEntry   `json:"monthlyHeatmap"`
	BusiestMonth       BusiestMonth            `json:"busiestMonth"`
	PhysicalCollection PhysicalCollectionStats `json:"physicalCollection"`
}

func formatDuration(minutes int) string {
	if minutes <= 0 {
		return "0 minutes"
	}
	days := minutes / (24 * 60)
	rem := minutes % (24 * 60)
	hours := rem / 60
	mins := rem % 60

	var parts []string
	if days > 0 {
		if days == 1 {
			parts = append(parts, "1 day")
		} else {
			parts = append(parts, fmt.Sprintf("%d days", days))
		}
	}
	if hours > 0 {
		if hours == 1 {
			parts = append(parts, "1 hour")
		} else {
			parts = append(parts, fmt.Sprintf("%d hours", hours))
		}
	}
	if mins > 0 {
		if mins == 1 {
			parts = append(parts, "1 minute")
		} else {
			parts = append(parts, fmt.Sprintf("%d minutes", mins))
		}
	}
	if len(parts) == 0 {
		return "0 minutes"
	}
	return strings.Join(parts, ", ")
}

func (h *statsHandler) wrapped(c *gin.Context) {
	uid := CurrentUserID(c)

	yearStr := c.Query("year")
	year := time.Now().Year()
	if yearStr != "" {
		parsedYear, err := strconv.Atoi(yearStr)
		if err != nil || parsedYear < 1900 || parsedYear > 2100 {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid year parameter")
			return
		}
		year = parsedYear
	}

	startOfYear := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	endOfYear := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC)

	monthlyBreakdown := make(map[int]*CompletionBreakdown)
	for m := 1; m <= 12; m++ {
		monthlyBreakdown[m] = &CompletionBreakdown{}
	}

	// 1. TV episodes watched in year
	type epRow struct {
		WatchedAt time.Time
		Runtime   int
		ShowID    uint
	}
	var epRows []epRow
	h.db.Table("episode_watches").
		Select("episode_watches.watched_at, CASE WHEN episodes.runtime > 0 THEN episodes.runtime ELSE 45 END as runtime, episodes.show_id").
		Joins("JOIN episodes ON episode_watches.episode_id = episodes.id").
		Where("episode_watches.user_id = ? AND episode_watches.watched_at >= ? AND episode_watches.watched_at < ?", uid, startOfYear, endOfYear).
		Scan(&epRows)

	tvMinutes := 0
	showIDsMap := make(map[uint]bool)
	for _, r := range epRows {
		tvMinutes += r.Runtime
		showIDsMap[r.ShowID] = true
		m := int(r.WatchedAt.Month())
		if m >= 1 && m <= 12 {
			monthlyBreakdown[m].Episodes++
		}
	}
	tvCount := len(epRows)

	// 2. Movies completed in year
	type movieRow struct {
		MovieID   uint
		Runtime   int
		UpdatedAt time.Time
	}
	var movieRows []movieRow
	h.db.Table("tracking_items").
		Select("movies.id as movie_id, CASE WHEN movies.runtime > 0 THEN movies.runtime ELSE 120 END as runtime, tracking_items.updated_at").
		Joins("JOIN movies ON movies.tmdb_id = CAST(tracking_items.external_id AS INTEGER)").
		Where("tracking_items.user_id = ? AND tracking_items.type = ? AND tracking_items.status = ? AND tracking_items.updated_at >= ? AND tracking_items.updated_at < ?",
			uid, "MOVIE", "COMPLETED", startOfYear, endOfYear).
		Scan(&movieRows)

	movieMinutes := 0
	movieIDsMap := make(map[uint]bool)
	for _, r := range movieRows {
		movieMinutes += r.Runtime
		movieIDsMap[r.MovieID] = true
		m := int(r.UpdatedAt.Month())
		if m >= 1 && m <= 12 {
			monthlyBreakdown[m].Movies++
		}
	}
	movieCount := len(movieRows)

	// 3. Books completed in year
	type bookRow struct {
		BookID    uint
		PageCount int
		Authors   string
		UpdatedAt time.Time
	}
	var bookRows []bookRow
	h.db.Table("tracking_items").
		Select("books.id as book_id, books.page_count, books.authors, tracking_items.updated_at").
		Joins("JOIN books ON tracking_items.external_id = books.isbn13").
		Where("tracking_items.user_id = ? AND tracking_items.type = ? AND tracking_items.status = ? AND tracking_items.updated_at >= ? AND tracking_items.updated_at < ?",
			uid, "BOOK", "COMPLETED", startOfYear, endOfYear).
		Scan(&bookRows)

	bookMinutes := 0
	bookIDsMap := make(map[uint]bool)
	authorCounts := make(map[string]int)
	for _, r := range bookRows {
		bookMinutes += r.PageCount
		bookIDsMap[r.BookID] = true
		m := int(r.UpdatedAt.Month())
		if m >= 1 && m <= 12 {
			monthlyBreakdown[m].Books++
		}
		if r.Authors != "" {
			for _, a := range strings.Split(r.Authors, ",") {
				auth := strings.TrimSpace(a)
				if auth != "" {
					authorCounts[auth]++
				}
			}
		}
	}
	bookCount := len(bookRows)

	// 4. Games completed in year
	type gameRow struct {
		GameID    uint
		Runtime   int
		Developer string
		Publisher string
		UpdatedAt time.Time
	}
	var gameRows []gameRow
	h.db.Table("tracking_items").
		Select("games.id as game_id, CASE WHEN games.runtime > 0 THEN games.runtime ELSE 1200 END as runtime, games.developer, games.publisher, tracking_items.updated_at").
		Joins("JOIN games ON games.igdb_id = CAST(tracking_items.external_id AS INTEGER) OR games.barcode = tracking_items.external_id").
		Where("tracking_items.user_id = ? AND tracking_items.type = ? AND tracking_items.status = ? AND tracking_items.updated_at >= ? AND tracking_items.updated_at < ?",
			uid, "GAME", "COMPLETED", startOfYear, endOfYear).
		Scan(&gameRows)

	gameMinutes := 0
	gameIDsMap := make(map[uint]bool)
	developerCounts := make(map[string]int)
	for _, r := range gameRows {
		gameMinutes += r.Runtime
		gameIDsMap[r.GameID] = true
		m := int(r.UpdatedAt.Month())
		if m >= 1 && m <= 12 {
			monthlyBreakdown[m].Games++
		}
		dev := strings.TrimSpace(r.Developer)
		if dev == "" {
			dev = strings.TrimSpace(r.Publisher)
		}
		if dev != "" {
			developerCounts[dev]++
		}
	}
	gameCount := len(gameRows)

	// 5. Music completed in year
	type albumRow struct {
		AlbumID   uint
		Artist    string
		UpdatedAt time.Time
	}
	var albumRows []albumRow
	h.db.Table("tracking_items").
		Select("albums.id as album_id, albums.artist, tracking_items.updated_at").
		Joins("JOIN albums ON albums.external_id = tracking_items.external_id").
		Where("tracking_items.user_id = ? AND tracking_items.type = ? AND tracking_items.status = ? AND tracking_items.updated_at >= ? AND tracking_items.updated_at < ?",
			uid, "MUSIC", "COMPLETED", startOfYear, endOfYear).
		Scan(&albumRows)

	albumMinutes := len(albumRows) * 45
	artistCounts := make(map[string]int)
	for _, r := range albumRows {
		m := int(r.UpdatedAt.Month())
		if m >= 1 && m <= 12 {
			monthlyBreakdown[m].Albums++
		}
		artist := strings.TrimSpace(r.Artist)
		if artist != "" {
			artistCounts[artist]++
		}
	}
	albumCount := len(albumRows)

	// Total completions
	totalCompleted := tvCount + movieCount + gameCount + bookCount + albumCount

	// Time spent
	totalMinutes := tvMinutes + movieMinutes + bookMinutes + gameMinutes + albumMinutes
	totalHours := math.Round((float64(totalMinutes)/60.0)*100) / 100
	totalDays := math.Round((float64(totalMinutes)/(24.0*60.0))*100) / 100

	timeSpent := WrappedTimeSpent{
		TotalMinutes: totalMinutes,
		TotalHours:   totalHours,
		TotalDays:    totalDays,
		Formatted:    formatDuration(totalMinutes),
		MinutesTV:    tvMinutes,
		MinutesMovie: movieMinutes,
		MinutesBook:  bookMinutes,
		MinutesGame:  gameMinutes,
		MinutesMusic: albumMinutes,
	}

	// Top Creators
	creators := make([]CreatorStat, 0)
	for auth, cnt := range authorCounts {
		creators = append(creators, CreatorStat{Name: auth, Category: "Author", Count: cnt})
	}
	for dev, cnt := range developerCounts {
		creators = append(creators, CreatorStat{Name: dev, Category: "Developer", Count: cnt})
	}
	for art, cnt := range artistCounts {
		creators = append(creators, CreatorStat{Name: art, Category: "Artist", Count: cnt})
	}
	sort.Slice(creators, func(i, j int) bool {
		if creators[i].Count != creators[j].Count {
			return creators[i].Count > creators[j].Count
		}
		return creators[i].Name < creators[j].Name
	})
	if len(creators) > 10 {
		creators = creators[:10]
	}

	// Top Genres (from TV shows, Movies, Games, Books)
	genreCounts := make(map[string]int)

	collectTags := func(mediaType string, idMap map[uint]bool) {
		if len(idMap) == 0 {
			return
		}
		ids := make([]uint, 0, len(idMap))
		for id := range idMap {
			ids = append(ids, id)
		}
		var tagNames []string
		h.db.Table("media_tags").
			Select("tags.name").
			Joins("JOIN tags ON tags.id = media_tags.tag_id").
			Where("media_tags.media_type = ? AND media_tags.media_id IN ?", mediaType, ids).
			Pluck("tags.name", &tagNames)
		for _, tn := range tagNames {
			if tn != "" {
				genreCounts[tn]++
			}
		}
	}

	collectTags("TV", showIDsMap)
	collectTags("MOVIE", movieIDsMap)
	collectTags("GAME", gameIDsMap)
	collectTags("BOOK", bookIDsMap)

	genres := make([]GenreStat, 0)
	for name, cnt := range genreCounts {
		genres = append(genres, GenreStat{Name: name, Count: cnt})
	}
	sort.Slice(genres, func(i, j int) bool {
		if genres[i].Count != genres[j].Count {
			return genres[i].Count > genres[j].Count
		}
		return genres[i].Name < genres[j].Name
	})
	if len(genres) > 10 {
		genres = genres[:10]
	}

	// Monthly Heatmap & Busiest Month
	monthlyHeatmap := make([]MonthlyHeatmapEntry, 12)
	busiestMonth := BusiestMonth{
		Month:     1,
		MonthName: "January",
		Count:     0,
	}

	for m := 1; m <= 12; m++ {
		bd := *monthlyBreakdown[m]
		cnt := bd.Episodes + bd.Movies + bd.Games + bd.Books + bd.Albums
		monthName := time.Month(m).String()
		monthlyHeatmap[m-1] = MonthlyHeatmapEntry{
			Month:     m,
			MonthName: monthName,
			Count:     cnt,
			Breakdown: bd,
		}
		if cnt > busiestMonth.Count {
			busiestMonth = BusiestMonth{
				Month:     m,
				MonthName: monthName,
				Count:     cnt,
			}
		}
	}

	// Physical collection estimated value added in year
	// 1. Cards
	var cardRes struct {
		Count      int
		TotalValue float64
	}
	h.db.Table("tracking_items").
		Select("COUNT(tracking_items.id) as count, COALESCE(SUM(cards.price), 0) as total_value").
		Joins("JOIN cards ON cards.external_id = tracking_items.external_id").
		Where("tracking_items.user_id = ? AND tracking_items.type = ? AND tracking_items.updated_at >= ? AND tracking_items.updated_at < ?",
			uid, "CARD", startOfYear, endOfYear).
		Scan(&cardRes)

	// 2. Vinyl
	var vinylRes struct {
		Count int
	}
	h.db.Table("tracking_items").
		Select("COUNT(DISTINCT tracking_items.id) as count").
		Joins("JOIN ownership_formats ON ownership_formats.media_type = ? AND ownership_formats.item_id = tracking_items.id AND ownership_formats.format = ?", "MUSIC", "Vinyl").
		Where("tracking_items.user_id = ? AND tracking_items.type = ? AND tracking_items.updated_at >= ? AND tracking_items.updated_at < ?",
			uid, "MUSIC", startOfYear, endOfYear).
		Scan(&vinylRes)
	vinylValue := float64(vinylRes.Count) * 30.0

	// 3. Physical Games
	var physGameRes struct {
		Count int
	}
	h.db.Table("tracking_items").
		Select("COUNT(DISTINCT tracking_items.id) as count").
		Joins("JOIN ownership_formats ON ownership_formats.media_type = ? AND ownership_formats.item_id = tracking_items.id AND ownership_formats.format = ?", "GAME", "Physical").
		Where("tracking_items.user_id = ? AND tracking_items.type = ? AND tracking_items.updated_at >= ? AND tracking_items.updated_at < ?",
			uid, "GAME", startOfYear, endOfYear).
		Scan(&physGameRes)
	gameValue := float64(physGameRes.Count) * 60.0

	cardValue := math.Round(cardRes.TotalValue*100) / 100
	vinylValue = math.Round(vinylValue*100) / 100
	gameValue = math.Round(gameValue*100) / 100
	totalPhysValue := math.Round((cardValue+vinylValue+gameValue)*100) / 100
	totalPhysItems := cardRes.Count + vinylRes.Count + physGameRes.Count

	physStats := PhysicalCollectionStats{
		TotalValue: totalPhysValue,
		TotalItems: totalPhysItems,
		CardValue:  cardValue,
		CardCount:  cardRes.Count,
		VinylValue: vinylValue,
		VinylCount: vinylRes.Count,
		GameValue:  gameValue,
		GameCount:  physGameRes.Count,
	}

	res := WrappedResponse{
		Year:           year,
		TotalCompleted: totalCompleted,
		Breakdown: CompletionBreakdown{
			Episodes: tvCount,
			Movies:   movieCount,
			Games:    gameCount,
			Books:    bookCount,
			Albums:   albumCount,
		},
		TimeSpent:          timeSpent,
		TopCreators:        creators,
		TopGenres:          genres,
		MonthlyHeatmap:     monthlyHeatmap,
		BusiestMonth:       busiestMonth,
		PhysicalCollection: physStats,
	}

	c.JSON(http.StatusOK, res)
}

