package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/games"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// Machine error codes for the game endpoints.
const (
	CodeGameNotFound = "game_not_found"
)

// gamesHandler serves the game scan/track and name-search/add endpoints.
type gamesHandler struct {
	svc *games.Service
}

// RegisterGameRoutes attaches the game endpoints to the JWT-protected /api
// group returned by RegisterRoutes.
func RegisterGameRoutes(grp *gin.RouterGroup, svc *games.Service) {
	h := &gamesHandler{svc: svc}
	grp.POST("/games/scan", h.scan)
	grp.POST("/games/track", h.track)
	grp.GET("/games/search", h.search)
	grp.POST("/games/add", h.add)
	grp.GET("/games/discover", h.discover)
	grp.POST("/games/discover/reject", h.rejectRec)
}

type gameScanRequest struct {
	Barcode string `json:"barcode"`
}

type gameTrackRequest struct {
	GameID uint   `json:"gameId"`
	Status string `json:"status"`
}

type gameAddRequest struct {
	IGDBID int    `json:"igdbId"`
	Status string `json:"status"`
}

// gameSearchResult is one IGDB name-search hit. The igdbId is the canonical
// identity the client posts back to /api/games/add.
type gameSearchResult struct {
	IGDBID int    `json:"igdbId"`
	Name   string `json:"name"`
	Year   int    `json:"year"`
}

// gameResponse is the JSON shape of a Game payload.
type gameResponse struct {
	ID          uint   `json:"id"`
	Barcode     string `json:"barcode"`
	Title       string `json:"title"`
	Platform    string `json:"platform"`
	CoverPath   string `json:"coverPath"`
	IGDBID      int    `json:"igdbId"`
	Description string `json:"description"`
}

func toGameResponse(g *models.Game) gameResponse {
	return gameResponse{
		ID:          g.ID,
		Barcode:     g.Barcode,
		Title:       g.Title,
		Platform:    g.Platform,
		CoverPath:   g.CoverPath,
		IGDBID:      g.IGDBID,
		Description: g.Description,
	}
}

// scan handles POST /api/games/scan {barcode}.
func (h *gamesHandler) scan(c *gin.Context) {
	var req gameScanRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Barcode == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with a non-empty barcode")
		return
	}

	game, err := h.svc.Scan(c.Request.Context(), req.Barcode)
	switch {
	case errors.Is(err, games.ErrInvalidBarcode):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "barcode must be an 8–14 digit UPC/EAN")
	case errors.Is(err, games.ErrNotFound):
		// Echo the barcode so the UI can surface it for a manual retry.
		c.JSON(http.StatusNotFound, gin.H{
			"error":   CodeGameNotFound,
			"message": "no game found for this barcode",
			"barcode": req.Barcode,
		})
	case errors.Is(err, games.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "ScanDex is unreachable, try again")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "scan failed")
	default:
		c.JSON(http.StatusOK, toGameResponse(game))
	}
}

// track handles POST /api/games/track {gameId, status}.
func (h *gamesHandler) track(c *gin.Context) {
	var req gameTrackRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.GameID == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with gameId and status")
		return
	}

	item, err := h.svc.Track(c.Request.Context(), CurrentUserID(c), req.GameID, req.Status)
	switch {
	case errors.Is(err, games.ErrInvalidStatus):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "status must be PLAYING, PLAN_TO, COMPLETED, or STOPPED")
	case errors.Is(err, games.ErrGameNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "game does not exist; scan it first")
	case errors.Is(err, games.ErrAlreadyTracked):
		c.JSON(http.StatusConflict, gin.H{
			"error":   CodeAlreadyTracked,
			"message": "you already track this game",
			"item":    toItemResponse(item),
		})
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "tracking failed")
	default:
		c.JSON(http.StatusCreated, toItemResponse(item))
	}
}

// search handles GET /api/games/search?q= — an IGDB name-search proxy for the
// add-by-name flow.
func (h *gamesHandler) search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "query parameter q is required")
		return
	}

	results, err := h.svc.Search(c.Request.Context(), q)
	switch {
	case errors.Is(err, games.ErrSearchUnavailable):
		Error(c, http.StatusServiceUnavailable, CodeUpstreamError, "game search is not configured")
	case errors.Is(err, games.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "IGDB is unreachable, try again")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "search failed")
	default:
		out := make([]gameSearchResult, 0, len(results))
		for _, r := range results {
			out = append(out, gameSearchResult{IGDBID: r.ID, Name: r.Name, Year: r.Year})
		}
		c.JSON(http.StatusOK, gin.H{"results": out})
	}
}

// add handles POST /api/games/add {igdbId, status?} — add a game by name-search
// pick, keyed by its IGDB id. Returns the shared game plus the new tracking item
// (mirrors the movie add flow).
func (h *gamesHandler) add(c *gin.Context) {
	var req gameAddRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.IGDBID <= 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with a positive igdbId")
		return
	}

	game, item, err := h.svc.AddByIGDB(c.Request.Context(), CurrentUserID(c), req.IGDBID, req.Status)
	switch {
	case errors.Is(err, games.ErrInvalidStatus):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "status must be PLAYING, PLAN_TO, COMPLETED, or STOPPED")
	case errors.Is(err, games.ErrSearchUnavailable):
		Error(c, http.StatusServiceUnavailable, CodeUpstreamError, "game search is not configured")
	case errors.Is(err, games.ErrGameNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "no game found for that IGDB id")
	case errors.Is(err, games.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "IGDB is unreachable, try again")
	case errors.Is(err, games.ErrAlreadyTracked):
		c.JSON(http.StatusConflict, gin.H{
			"error":   CodeAlreadyTracked,
			"message": "you already track this game",
			"item":    toItemResponse(item),
		})
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "adding game failed")
	default:
		c.JSON(http.StatusCreated, gin.H{
			"game": toGameResponse(game),
			"item": toItemResponse(item),
		})
	}
}

// discover handles GET /api/games/discover — game suggestions via IGDB "similar
// games" seeded from the user's tracked games, each tagged with the game it was
// suggested from. Covers are pre-cached through internal/images; coverPath is a
// relative /images path.
func (h *gamesHandler) discover(c *gin.Context) {
	items, err := h.svc.Discover(c.Request.Context(), CurrentUserID(c))
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading suggestions failed")
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		out = append(out, gin.H{
			"igdbId":      it.IGDBID,
			"title":       it.Title,
			"year":        it.Year,
			"coverPath":   it.CoverPath,
			"suggestedBy": it.SuggestedBy,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// rejectRec handles POST /api/games/discover/reject {igdbId} — hide a suggestion.
func (h *gamesHandler) rejectRec(c *gin.Context) {
	var body struct {
		IGDBID int `json:"igdbId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.IGDBID <= 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "body must include a positive igdbId")
		return
	}
	if err := h.svc.RejectRec(c.Request.Context(), CurrentUserID(c), body.IGDBID); err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "could not reject suggestion")
		return
	}
	c.Status(http.StatusNoContent)
}
