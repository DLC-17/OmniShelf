package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tv"
)

// TV-domain machine error codes for the standard envelope.
// CodeNotFound ("not_found") is shared and declared in books.go.
const (
	CodeDuplicateItem   = "duplicate_item"
	CodeTMDBUnavailable = "tmdb_unavailable"
)

// RegisterTVRoutes attaches the TV endpoints to the JWT-guarded
// /api group returned by RegisterRoutes. Wired from main by the orchestrator.
func RegisterTVRoutes(grp *gin.RouterGroup, svc *tv.Service) {
	h := &tvHandler{svc: svc}
	grp.GET("/tv/search", h.search)
	grp.POST("/tv/shows", h.addShow)
	grp.GET("/tv/up-next", h.upNext)
	grp.GET("/tv/shows/:id/episodes", h.listEpisodes)
	grp.POST("/tv/episodes/:id/watch", h.markWatched)
	grp.POST("/tv/episodes/:id/rewatch", h.rewatch)
	grp.POST("/tv/episodes/:id/watch-through", h.watchThrough)
	grp.DELETE("/tv/episodes/:id/watch", h.unmarkWatched)
}

type tvHandler struct {
	svc *tv.Service
}

// ── response shapes ──

type tvSearchResult struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	FirstAirDate string `json:"firstAirDate"`
	PosterPath   string `json:"posterPath"`
}

type showDTO struct {
	ID         uint   `json:"id"`
	TMDBID     int    `json:"tmdbId"`
	Title      string `json:"title"`
	PosterPath string `json:"posterPath"` // relative path under /images, "" = placeholder
	Status     string `json:"status"`
}

type episodeDTO struct {
	ID      uint    `json:"id"`
	ShowID  uint    `json:"showId"`
	Season  int     `json:"season"`
	Number  int     `json:"number"`
	Title   string  `json:"title"`
	AirDate *string `json:"airDate"` // "YYYY-MM-DD", null = unannounced
}

// episodeWatchDTO is one row of the episode picker: the episode fields plus
// this user's watch state.
type episodeWatchDTO struct {
	episodeDTO
	Watched   bool    `json:"watched"`
	WatchedAt *string `json:"watchedAt"` // RFC3339, null when unwatched
}

type trackingItemDTO struct {
	ID         uint   `json:"id"`
	Type       string `json:"type"`
	ExternalID string `json:"externalId"`
	Title      string `json:"title"`
	Status     string `json:"status"`
}

func toShowDTO(s models.Show) showDTO {
	return showDTO{ID: s.ID, TMDBID: s.TMDBID, Title: s.Title, PosterPath: s.PosterPath, Status: s.Status}
}

func toEpisodeDTO(e models.Episode) episodeDTO {
	dto := episodeDTO{ID: e.ID, ShowID: e.ShowID, Season: e.Season, Number: e.Number, Title: e.Title}
	if e.AirDate != nil {
		d := e.AirDate.Format("2006-01-02")
		dto.AirDate = &d
	}
	return dto
}

func toItemDTO(i models.TrackingItem) trackingItemDTO {
	return trackingItemDTO{ID: i.ID, Type: i.Type, ExternalID: i.ExternalID, Title: i.Title, Status: i.Status}
}

// ── handlers ──

// search handles GET /api/tv/search?q= — server-side TMDB proxy (the API key
// never reaches the browser).
func (h *tvHandler) search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "query parameter q is required")
		return
	}
	res, err := h.svc.Search(c.Request.Context(), q)
	if err != nil {
		h.writeError(c, err)
		return
	}
	results := make([]tvSearchResult, 0, len(res.Results))
	for _, r := range res.Results {
		results = append(results, tvSearchResult{
			ID:           r.ID,
			Name:         r.Name,
			Overview:     r.Overview,
			FirstAirDate: r.FirstAirDate,
			PosterPath:   r.PosterPath,
		})
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// addShow handles POST /api/tv/shows {tmdbId}.
func (h *tvHandler) addShow(c *gin.Context) {
	var body struct {
		TMDBID int `json:"tmdbId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.TMDBID <= 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "body must include a positive tmdbId")
		return
	}
	res, err := h.svc.AddShow(c.Request.Context(), CurrentUserID(c), body.TMDBID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"show": toShowDTO(res.Show),
		"item": toItemDTO(res.Item),
	})
}

// upNext handles GET /api/tv/up-next?filter=recent|stale|unstarted. An unknown
// or absent filter defaults to "recent" (watched in the last 14 days).
func (h *tvHandler) upNext(c *gin.Context) {
	filter := tv.Recency(c.DefaultQuery("filter", string(tv.RecencyRecent)))
	switch filter {
	case tv.RecencyRecent, tv.RecencyStale, tv.RecencyUnstarted:
	default:
		filter = tv.RecencyRecent
	}
	entries, err := h.svc.UpNextByRecency(c.Request.Context(), CurrentUserID(c), filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	items := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		items = append(items, gin.H{
			"show":    toShowDTO(e.Show),
			"episode": toEpisodeDTO(e.Episode),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// listEpisodes handles GET /api/tv/shows/:id/episodes — every episode of the
// show with the caller's per-episode watched state, for the episode picker.
func (h *tvHandler) listEpisodes(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "show id must be a positive integer")
		return
	}
	states, err := h.svc.ListEpisodes(c.Request.Context(), CurrentUserID(c), uint(id))
	if err != nil {
		h.writeError(c, err)
		return
	}
	episodes := make([]episodeWatchDTO, 0, len(states))
	for _, st := range states {
		dto := episodeWatchDTO{episodeDTO: toEpisodeDTO(st.Episode), Watched: st.Watched}
		if st.WatchedAt != nil {
			w := st.WatchedAt.Format(time.RFC3339)
			dto.WatchedAt = &w
		}
		episodes = append(episodes, dto)
	}
	c.JSON(http.StatusOK, gin.H{"episodes": episodes})
}

// markWatched handles POST /api/tv/episodes/:id/watch. The response carries
// the show's new next-up episode so the UI can swap the card in place
// without a reload.
func (h *tvHandler) markWatched(c *gin.Context) {
	h.toggleWatch(c, h.svc.MarkWatched)
}

// rewatch handles POST /api/tv/episodes/:id/rewatch — re-stamp an already
// watched episode (or mark a fresh one) with the current time.
func (h *tvHandler) rewatch(c *gin.Context) {
	h.toggleWatch(c, h.svc.Rewatch)
}

// watchThrough handles POST /api/tv/episodes/:id/watch-through — mark this
// episode and every earlier aired episode of the show as watched.
func (h *tvHandler) watchThrough(c *gin.Context) {
	h.toggleWatch(c, h.svc.WatchThrough)
}

// unmarkWatched handles DELETE /api/tv/episodes/:id/watch.
func (h *tvHandler) unmarkWatched(c *gin.Context) {
	h.toggleWatch(c, h.svc.UnmarkWatched)
}

// toggleWatch shares the mark/unmark plumbing: parse the episode ID, run the
// operation for the authenticated user, and respond with the show's new
// next-up episode (null when none remains).
func (h *tvHandler) toggleWatch(c *gin.Context, op func(ctx context.Context, userID, episodeID uint) (*models.Episode, error)) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "episode id must be a positive integer")
		return
	}
	next, err := op(c.Request.Context(), CurrentUserID(c), uint(id))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if next == nil {
		c.JSON(http.StatusOK, gin.H{"nextUp": nil})
		return
	}
	dto := toEpisodeDTO(*next)
	c.JSON(http.StatusOK, gin.H{"nextUp": dto})
}

// writeError maps service errors onto the standard envelope: duplicate track
// → 409 with the existing item (E16); missing show/episode → 404; TMDB
// unreachable → 502 (E3); anything else → 500.
func (h *tvHandler) writeError(c *gin.Context, err error) {
	var conflict *tv.ConflictError
	var up *tv.UpstreamError
	switch {
	case errors.As(err, &conflict):
		c.JSON(http.StatusConflict, gin.H{
			"error":   CodeDuplicateItem,
			"message": "this show is already in your library",
			"item":    toItemDTO(conflict.Existing),
		})
	case errors.Is(err, tv.ErrNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "the requested show or episode does not exist")
	case errors.As(err, &up):
		Error(c, http.StatusBadGateway, CodeTMDBUnavailable, "TMDB unreachable, try again")
	default:
		Error(c, http.StatusInternalServerError, CodeInternal, "something went wrong")
	}
}
