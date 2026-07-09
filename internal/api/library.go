package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/books"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/ownership"
)

// libraryHandler serves the unified shelf endpoints:
// GET /api/library, PATCH /api/items/:id, DELETE /api/items/:id.
type libraryHandler struct {
	svc *books.Service
}

// RegisterLibraryRoutes attaches the library endpoints to the JWT-protected
// /api group returned by RegisterRoutes.
func RegisterLibraryRoutes(grp *gin.RouterGroup, svc *books.Service) {
	h := &libraryHandler{svc: svc}
	grp.GET("/library", h.list)
	grp.PATCH("/items/:id", h.update)
	grp.PUT("/items/:id/ownership", h.setOwnership)
	grp.DELETE("/items/:id", h.remove)
}

// itemResponse is the JSON shape of a TrackingItem payload, enriched with the
// cached artwork and (for books) the metadata the library grid + detail view
// need. Book-only fields are empty/zero for TV items.
type itemResponse struct {
	ID          uint      `json:"id"`
	Type        string    `json:"type"`
	ExternalID  string    `json:"externalId"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Progress    int       `json:"progress"`
	Rating      int       `json:"rating"`
	ArtworkPath string    `json:"artworkPath"`
	ShowID      uint      `json:"showId"`
	Authors     string    `json:"authors"`
	PageCount   int       `json:"pageCount"`
	Description string    `json:"description"`
	Platform    string    `json:"platform"`
	Tags        []string  `json:"tags"`      // source-derived tags/keywords; [] when none
	Ownership   []string  `json:"ownership"` // user-selected ownership formats (games); [] when none
	UpdatedAt   time.Time `json:"updatedAt"`
}

func toItemResponse(item *models.TrackingItem) itemResponse {
	return itemResponse{
		ID:         item.ID,
		Type:       item.Type,
		ExternalID: item.ExternalID,
		Title:      item.Title,
		Status:     item.Status,
		Progress:   item.Progress,
		Rating:     item.Rating,
		Tags:       []string{}, // never null; overwritten with the real set for library rows
		Ownership:  []string{}, // never null; overwritten with the real set for library rows
		UpdatedAt:  item.UpdatedAt,
	}
}

func toLibraryResponse(e *books.LibraryEntry) itemResponse {
	r := toItemResponse(&e.Item)
	r.ArtworkPath = e.ArtworkPath
	r.ShowID = e.ShowID
	r.Authors = e.Authors
	r.PageCount = e.PageCount
	r.Description = e.Description
	r.Platform = e.Platform
	if e.Tags != nil {
		r.Tags = e.Tags
	}
	if e.Ownership != nil {
		r.Ownership = e.Ownership
	}
	return r
}

// updateItemRequest is the PATCH body; pointer fields distinguish "absent"
// from zero values.
type updateItemRequest struct {
	Status   *string `json:"status"`
	Progress *int    `json:"progress"`
	Rating   *int    `json:"rating"`
}

// list handles GET /api/library?type=&status= — the current user's shelf,
// enriched with artwork and book metadata.
func (h *libraryHandler) list(c *gin.Context) {
	entries, err := h.svc.ListLibrary(c.Request.Context(), CurrentUserID(c),
		c.Query("type"), c.Query("status"))
	switch {
	case errors.Is(err, books.ErrInvalidFilter):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "type must be TV, BOOK, or GAME; status must be WATCHING, READING, PLAYING, PLAN_TO, COMPLETED, or STOPPED")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "listing library failed")
	default:
		out := make([]itemResponse, 0, len(entries))
		for i := range entries {
			out = append(out, toLibraryResponse(&entries[i]))
		}
		c.JSON(http.StatusOK, out)
	}
}

// update handles PATCH /api/items/:id {status?, progress?}.
func (h *libraryHandler) update(c *gin.Context) {
	itemID, ok := itemIDParam(c)
	if !ok {
		return
	}
	var req updateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with status and/or progress")
		return
	}

	item, err := h.svc.UpdateItem(c.Request.Context(), CurrentUserID(c), itemID, req.Status, req.Progress, req.Rating)
	switch {
	case errors.Is(err, books.ErrEmptyUpdate):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "provide status, progress, and/or rating")
	case errors.Is(err, books.ErrInvalidStatus):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "status is not valid for this item's type")
	case errors.Is(err, books.ErrInvalidProgress):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "progress must be a non-negative page number and only applies to books")
	case errors.Is(err, books.ErrInvalidRating):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "rating must be between 0 and 5")
	case errors.Is(err, books.ErrItemNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "tracking item not found")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "updating item failed")
	default:
		c.JSON(http.StatusOK, toItemResponse(item))
	}
}

// ownershipRequest is the PUT /items/:id/ownership body: the full replacement
// set of ownership formats (multi-select). An empty array clears ownership.
type ownershipRequest struct {
	Formats []string `json:"formats"`
}

// setOwnership handles PUT /api/items/:id/ownership {formats: []}. It replaces
// the tracked item's ownership formats and returns the normalized set.
func (h *libraryHandler) setOwnership(c *gin.Context) {
	itemID, ok := itemIDParam(c)
	if !ok {
		return
	}
	var req ownershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with a formats array")
		return
	}

	formats, err := h.svc.SetOwnership(c.Request.Context(), CurrentUserID(c), itemID, req.Formats)
	switch {
	case errors.Is(err, ownership.ErrInvalidFormat):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "formats must be a subset of the allowed set for this item's type (games: Physical, GOG)")
	case errors.Is(err, books.ErrItemNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "tracking item not found")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "updating ownership failed")
	default:
		c.JSON(http.StatusOK, gin.H{"ownership": formats})
	}
}

// remove handles DELETE /api/items/:id.
func (h *libraryHandler) remove(c *gin.Context) {
	itemID, ok := itemIDParam(c)
	if !ok {
		return
	}
	err := h.svc.DeleteItem(c.Request.Context(), CurrentUserID(c), itemID)
	switch {
	case errors.Is(err, books.ErrItemNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "tracking item not found")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "deleting item failed")
	default:
		c.Status(http.StatusNoContent)
	}
}

// itemIDParam parses the :id path parameter, writing the envelope itself on
// failure.
func itemIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "item id must be a positive integer")
		return 0, false
	}
	return uint(id), true
}
