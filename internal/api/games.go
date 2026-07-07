package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/games"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// Machine error codes for the game endpoints.
const (
	CodeGameNotFound = "game_not_found"
)

// gamesHandler serves POST /api/games/scan and /api/games/track.
type gamesHandler struct {
	svc *games.Service
}

// RegisterGameRoutes attaches the game endpoints to the JWT-protected /api
// group returned by RegisterRoutes.
func RegisterGameRoutes(grp *gin.RouterGroup, svc *games.Service) {
	h := &gamesHandler{svc: svc}
	grp.POST("/games/scan", h.scan)
	grp.POST("/games/track", h.track)
}

type gameScanRequest struct {
	Barcode string `json:"barcode"`
}

type gameTrackRequest struct {
	GameID uint   `json:"gameId"`
	Status string `json:"status"`
}

// gameResponse is the JSON shape of a Game payload.
type gameResponse struct {
	ID        uint   `json:"id"`
	Barcode   string `json:"barcode"`
	Title     string `json:"title"`
	Platform  string `json:"platform"`
	CoverPath string `json:"coverPath"`
	IGDBID    int    `json:"igdbId"`
}

func toGameResponse(g *models.Game) gameResponse {
	return gameResponse{
		ID:        g.ID,
		Barcode:   g.Barcode,
		Title:     g.Title,
		Platform:  g.Platform,
		CoverPath: g.CoverPath,
		IGDBID:    g.IGDBID,
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
