package api

import (
	"net/http"
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
