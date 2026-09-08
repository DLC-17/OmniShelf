package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/related"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// RegisterRelatedRoutes attaches related and universe graph routes to the API group.
func RegisterRelatedRoutes(grp *gin.RouterGroup, svc *related.Service, tmdbClient *tmdb.Client, igdbClient *igdb.Client) {
	h := &relatedHandler{
		svc:        svc,
		tmdbClient: tmdbClient,
		igdbClient: igdbClient,
	}
	grp.GET("/related", h.getRelated)
	grp.GET("/related/graph", h.getGraph)
	grp.GET("/related/recommendations", h.getRecommendations)
	grp.GET("/recommendations", h.getRecommendations)
	grp.POST("/related/recommendations/reject", h.rejectRecommendation)
	grp.POST("/recommendations/reject", h.rejectRecommendation)
	grp.POST("/related/sync", h.triggerSync)
}

type relatedHandler struct {
	svc        *related.Service
	tmdbClient *tmdb.Client
	igdbClient *igdb.Client
}

func (h *relatedHandler) getRelated(c *gin.Context) {
	mType := c.Query("type")
	externalID := c.Query("externalId")

	if mType == "" || externalID == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "type and externalId are required query parameters")
		return
	}

	items, err := h.svc.GetRelatedForMedia(c.Request.Context(), CurrentUserID(c), mType, externalID)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "failed to get related media")
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *relatedHandler) getGraph(c *gin.Context) {
	mType := c.Query("type")
	externalID := c.Query("externalId")

	if mType == "" || externalID == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "type and externalId are required query parameters")
		return
	}

	graph, err := h.svc.GetFranchiseGraph(c.Request.Context(), CurrentUserID(c), mType, externalID)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "failed to get franchise graph")
		return
	}

	c.JSON(http.StatusOK, graph)
}

func (h *relatedHandler) getRecommendations(c *gin.Context) {
	mType := c.Query("type")
	externalID := c.Query("externalId")

	if mType == "" || externalID == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "type and externalId are required query parameters")
		return
	}

	items, err := h.svc.GetRecommendationsForMedia(c.Request.Context(), CurrentUserID(c), mType, externalID, h.tmdbClient, h.igdbClient)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "failed to get recommended media")
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *relatedHandler) rejectRecommendation(c *gin.Context) {
	var body struct {
		Type       string `json:"type"`
		ExternalID string `json:"externalId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Type == "" || body.ExternalID == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "body must include type and externalId")
		return
	}
	if err := h.svc.DismissRecommendation(c.Request.Context(), CurrentUserID(c), body.Type, body.ExternalID); err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "failed to reject recommendation")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *relatedHandler) triggerSync(c *gin.Context) {
	go func() {
		_ = h.svc.SweepAndSyncAllRelations(c.Request.Context(), h.tmdbClient, h.igdbClient)
	}()
	c.JSON(http.StatusOK, gin.H{"status": "syncing", "message": "Relational sweep started in background"})
}
