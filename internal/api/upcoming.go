package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// upcomingLimit caps how many future items each media tab returns, newest
// air/release date first.
const upcomingLimit = 100

// upcomingHandler serves GET /upcoming — the "what's coming soon" board for
// media the user follows. Like the feed it is derived entirely from existing
// cached metadata (Episode.AirDate, Movie.ReleaseDate) with no extra table.
type upcomingHandler struct{ db *gorm.DB }

// RegisterUpcomingRoutes attaches GET /upcoming to the JWT-protected /api group.
func RegisterUpcomingRoutes(grp *gin.RouterGroup, gdb *gorm.DB) {
	h := &upcomingHandler{db: gdb}
	grp.GET("/upcoming", h.list)
}

// upcomingItem is one soon-to-release row on a media tab.
type upcomingItem struct {
	Type       string `json:"type"`       // "TV" | "MOVIE"
	Title      string `json:"title"`      // show or movie title
	PosterPath string `json:"posterPath"` // relative path under /images, "" = placeholder
	Date       string `json:"date"`       // "YYYY-MM-DD" release/air date
	Detail     string `json:"detail"`     // TV: "S04E01 · Name"; movies: ""
}

// list handles GET /api/upcoming. The response groups items by media type so
// the UI can render one tab per type. Games and Books are always present (as
// empty arrays): neither ScanDex/OpenLibrary stores a release date in our
// cache, and both are scan-based (already-released) media, so there is nothing
// upcoming to surface — the tab exists for parity, not data.
func (h *upcomingHandler) list(c *gin.Context) {
	userID := CurrentUserID(c)
	ctx := c.Request.Context()

	tv, err := h.tvUpcoming(ctx, userID)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading upcoming releases")
		return
	}
	movies, err := h.movieUpcoming(ctx, userID)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading upcoming releases")
		return
	}
	gms, err := h.gameUpcoming(ctx, userID)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading upcoming releases")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tv":     tv,
		"movies": movies,
		"games":  gms,
		"books":  []upcomingItem{},
	})
}

// tvUpcomingRow is the join projection for a future episode of a followed show.
type tvUpcomingRow struct {
	Title      string
	PosterPath string
	Season     int
	Number     int
	EpTitle    string `gorm:"column:ep_title"`
	AirDate    time.Time
}

// tvUpcoming returns the single next upcoming episode (earliest air_date > now)
// for each show the user is actively watching or has completed, soonest first.
// Only one row per show is returned — the next episode to air — rather than
// every future episode. Watching and completed are the shows a user still cares
// about; plan-to and stopped shows are excluded from the upcoming board.
func (h *upcomingHandler) tvUpcoming(ctx context.Context, userID uint) ([]upcomingItem, error) {
	now := time.Now()
	var rows []tvUpcomingRow
	if err := h.db.WithContext(ctx).Table("tracking_items").
		Select("shows.title, shows.poster_path, episodes.season, episodes.number, "+
			"episodes.title AS ep_title, episodes.air_date").
		Joins("JOIN shows ON shows.tmdb_id = CAST(tracking_items.external_id AS INTEGER)").
		Joins("JOIN episodes ON episodes.show_id = shows.id").
		Where("tracking_items.user_id = ? AND tracking_items.type = ?", userID, "TV").
		Where("tracking_items.status IN ?", []string{"WATCHING", "COMPLETED"}).
		Where("episodes.air_date IS NOT NULL AND episodes.air_date > ?", now).
		// Keep only the show's earliest future episode: no other future episode
		// of the same show airs sooner (ties broken by season, then number).
		Where("NOT EXISTS (SELECT 1 FROM episodes e2 WHERE e2.show_id = shows.id "+
			"AND e2.air_date IS NOT NULL AND e2.air_date > ? AND "+
			"(e2.air_date < episodes.air_date OR (e2.air_date = episodes.air_date AND "+
			"(e2.season < episodes.season OR (e2.season = episodes.season AND e2.number < episodes.number)))))", now).
		Order("episodes.air_date ASC, shows.title COLLATE NOCASE").
		Limit(upcomingLimit).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("querying upcoming episodes: %w", err)
	}

	out := make([]upcomingItem, 0, len(rows))
	for _, r := range rows {
		code := fmt.Sprintf("S%02dE%02d", r.Season, r.Number)
		detail := code
		if r.EpTitle != "" {
			detail = code + " · " + r.EpTitle
		}
		out = append(out, upcomingItem{
			Type:       "TV",
			Title:      r.Title,
			PosterPath: r.PosterPath,
			Date:       r.AirDate.Format("2006-01-02"),
			Detail:     detail,
		})
	}
	return out, nil
}

// movieUpcomingRow is the join projection for a future movie release.
type movieUpcomingRow struct {
	Title       string
	PosterPath  string
	ReleaseDate string
}

// movieUpcoming returns movies the user tracks whose release date is in the
// future, soonest first. ReleaseDate is stored as a "YYYY-MM-DD" string, so a
// lexicographic comparison against today's date is a correct date comparison.
func (h *upcomingHandler) movieUpcoming(ctx context.Context, userID uint) ([]upcomingItem, error) {
	today := time.Now().Format("2006-01-02")
	var rows []movieUpcomingRow
	if err := h.db.WithContext(ctx).Table("tracking_items").
		Select("movies.title, movies.poster_path, movies.release_date").
		Joins("JOIN movies ON movies.tmdb_id = CAST(tracking_items.external_id AS INTEGER)").
		Where("tracking_items.user_id = ? AND tracking_items.type = ?", userID, "MOVIE").
		Where("movies.release_date > ?", today).
		Order("movies.release_date ASC, movies.title COLLATE NOCASE").
		Limit(upcomingLimit).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("querying upcoming movies: %w", err)
	}

	out := make([]upcomingItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, upcomingItem{
			Type:       "MOVIE",
			Title:      r.Title,
			PosterPath: r.PosterPath,
			Date:       r.ReleaseDate,
		})
	}
	return out, nil
}

// gameUpcomingRow is the join projection for a future game release.
type gameUpcomingRow struct {
	Title       string
	CoverPath   string
	Platform    string
	ReleaseDate string
}

// gameUpcoming returns games the user tracks whose IGDB release date is in the
// future, soonest first. ReleaseDate is stored as a "YYYY-MM-DD" string, so a
// lexicographic comparison against today's date is a correct date comparison.
// A game with no cached release date (older scans, or IGDB had none) is skipped.
func (h *upcomingHandler) gameUpcoming(ctx context.Context, userID uint) ([]upcomingItem, error) {
	today := time.Now().Format("2006-01-02")
	var rows []gameUpcomingRow
	if err := h.db.WithContext(ctx).Table("tracking_items").
		Select("games.title, games.cover_path, games.platform, games.release_date").
		Joins("JOIN games ON games.barcode = tracking_items.external_id").
		Where("tracking_items.user_id = ? AND tracking_items.type = ?", userID, "GAME").
		Where("games.release_date != '' AND games.release_date > ?", today).
		Order("games.release_date ASC, games.title COLLATE NOCASE").
		Limit(upcomingLimit).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("querying upcoming games: %w", err)
	}

	out := make([]upcomingItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, upcomingItem{
			Type:       "GAME",
			Title:      r.Title,
			PosterPath: r.CoverPath,
			Date:       r.ReleaseDate,
			Detail:     r.Platform,
		})
	}
	return out, nil
}
