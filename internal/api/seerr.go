package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/seerr"
)

// RegisterSeerrRoutes attaches the Seerr endpoints to the JWT-guarded /api group.
func RegisterSeerrRoutes(grp *gin.RouterGroup, client *seerr.Client) {
	h := &seerrHandler{client: client}
	grp.GET("/seerr/configured", h.configured)
	grp.GET("/seerr/status/:tmdbId", h.status)
	grp.POST("/seerr/request", h.requestMedia)
}

type seerrHandler struct {
	client *seerr.Client
}

type configuredResponse struct {
	Configured bool `json:"configured"`
}

func (h *seerrHandler) configured(c *gin.Context) {
	c.JSON(http.StatusOK, configuredResponse{
		Configured: h.client.Configured(),
	})
}

type statusResponse struct {
	Available bool `json:"available"`
	Requested bool `json:"requested"`
}

func (h *seerrHandler) status(c *gin.Context) {
	if !h.client.Configured() {
		// Just return false for everything if not configured
		c.JSON(http.StatusOK, statusResponse{Available: false, Requested: false})
		return
	}

	tmdbID, err := strconv.Atoi(c.Param("tmdbId"))
	if err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid tmdbId")
		return
	}

	mediaType := c.Query("type")
	if mediaType != "movie" && mediaType != "tv" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "type must be 'movie' or 'tv'")
		return
	}

	status, err := h.client.CheckAvailability(c.Request.Context(), mediaType, tmdbID)
	if err != nil {
		log.Printf("seerr: status check for %s/%d failed: %v", mediaType, tmdbID, err)
		Error(c, http.StatusInternalServerError, CodeInternal, "checking availability failed")
		return
	}

	c.JSON(http.StatusOK, statusResponse{
		Available: status.IsAvailable(),
		Requested: status.IsRequested(),
	})
}

type requestPayload struct {
	TMDBID int    `json:"tmdbId" binding:"required"`
	Type   string `json:"type" binding:"required,oneof=movie tv"`
}

func (h *seerrHandler) requestMedia(c *gin.Context) {
	if !h.client.Configured() {
		Error(c, http.StatusServiceUnavailable, CodeInternal, "seerr is not configured")
		return
	}

	var req requestPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with tmdbId and type (movie or tv)")
		return
	}

	err := h.client.RequestMedia(c.Request.Context(), req.Type, req.TMDBID)
	if err != nil {
		log.Printf("seerr: request %s/%d failed: %v", req.Type, req.TMDBID, err)
		Error(c, http.StatusInternalServerError, CodeInternal, "media request failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
