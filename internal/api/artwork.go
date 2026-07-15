package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/artwork"
)

// maxCoverUploadBytes caps a user-uploaded cover so a huge file cannot exhaust disk
// or memory. Cover art is small; 8 MiB is generous.
const maxCoverUploadBytes = 8 << 20

// CodeNoArtwork is returned when a refresh finds no upstream cover.
const CodeNoArtwork = "no_artwork"

// artworkHandler serves cover-art refresh and upload for a tracked item.
type artworkHandler struct {
	svc *artwork.Service
}

// RegisterArtworkRoutes attaches the artwork endpoints to the JWT-protected
// /api group returned by RegisterRoutes.
func RegisterArtworkRoutes(grp *gin.RouterGroup, svc *artwork.Service) {
	h := &artworkHandler{svc: svc}
	grp.POST("/items/:id/artwork/refresh", h.refresh)
	grp.PUT("/items/:id/artwork", h.upload)
}

// refresh handles POST /api/items/:id/artwork/refresh — re-pull the latest
// cover from the item's upstream source.
func (h *artworkHandler) refresh(c *gin.Context) {
	itemID, ok := itemIDParam(c)
	if !ok {
		return
	}
	rel, err := h.svc.Refresh(c.Request.Context(), CurrentUserID(c), itemID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"artworkPath": rel})
}

// upload handles PUT /api/items/:id/artwork — replace the cover with a
// user-supplied image sent as multipart form field "image".
func (h *artworkHandler) upload(c *gin.Context) {
	itemID, ok := itemIDParam(c)
	if !ok {
		return
	}

	// Cap the request body before touching it so an oversized upload is
	// rejected without being buffered to disk.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCoverUploadBytes)
	fileHeader, err := c.FormFile("image")
	if err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest,
			"send the cover as multipart form field \"image\" (max 8 MiB)")
		return
	}
	if !strings.HasPrefix(fileHeader.Header.Get("Content-Type"), "image/") {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "uploaded file must be an image")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "could not read the uploaded file")
		return
	}
	defer func() { _ = file.Close() }()

	rel, err := h.svc.Upload(c.Request.Context(), CurrentUserID(c), itemID, file)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"artworkPath": rel})
}

// writeError maps artwork service errors onto the standard envelope.
func (h *artworkHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, artwork.ErrItemNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "tracking item not found")
	case errors.Is(err, artwork.ErrNoArtwork):
		Error(c, http.StatusUnprocessableEntity, CodeNoArtwork, "no cover art is available for this item")
	case errors.Is(err, artwork.ErrUnsupported):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "this item type has no cover art source")
	case errors.Is(err, artwork.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "the artwork source is unavailable, try again")
	default:
		Error(c, http.StatusInternalServerError, CodeInternal, "something went wrong")
	}
}
