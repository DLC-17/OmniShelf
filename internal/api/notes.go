package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/books"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// CodeNoteNotFound is the envelope code for a nonexistent journal entry.
const CodeNoteNotFound = "note_not_found"

// notesHandler serves the per-user book journal:
// GET/POST /api/items/:id/notes and DELETE /api/items/:id/notes/:noteId.
type notesHandler struct {
	svc *books.Service
}

// RegisterNoteRoutes attaches the book-notes endpoints to the JWT-protected
// /api group returned by RegisterRoutes.
func RegisterNoteRoutes(grp *gin.RouterGroup, svc *books.Service) {
	h := &notesHandler{svc: svc}
	grp.GET("/items/:id/notes", h.list)
	grp.POST("/items/:id/notes", h.add)
	grp.DELETE("/items/:id/notes/:noteId", h.remove)
}

// noteResponse is the JSON shape of a BookNote payload.
type noteResponse struct {
	ID        uint      `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toNoteResponse(n *models.BookNote) noteResponse {
	return noteResponse{
		ID:        n.ID,
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
}

type addNoteRequest struct {
	Body string `json:"body"`
}

// list handles GET /api/items/:id/notes — the user's journal for their book
// item, newest first.
func (h *notesHandler) list(c *gin.Context) {
	itemID, ok := itemIDParam(c)
	if !ok {
		return
	}
	notes, err := h.svc.ListNotes(c.Request.Context(), CurrentUserID(c), itemID)
	switch {
	case errors.Is(err, books.ErrItemNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "tracking item not found")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "listing notes failed")
	default:
		out := make([]noteResponse, 0, len(notes))
		for i := range notes {
			out = append(out, toNoteResponse(&notes[i]))
		}
		c.JSON(http.StatusOK, out)
	}
}

// add handles POST /api/items/:id/notes {body}.
func (h *notesHandler) add(c *gin.Context) {
	itemID, ok := itemIDParam(c)
	if !ok {
		return
	}
	var req addNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with a non-empty body")
		return
	}

	note, err := h.svc.AddNote(c.Request.Context(), CurrentUserID(c), itemID, req.Body)
	switch {
	case errors.Is(err, books.ErrEmptyNote):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "note body must not be empty")
	case errors.Is(err, books.ErrItemNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "tracking item not found")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "adding note failed")
	default:
		c.JSON(http.StatusCreated, toNoteResponse(note))
	}
}

// remove handles DELETE /api/items/:id/notes/:noteId.
func (h *notesHandler) remove(c *gin.Context) {
	itemID, ok := itemIDParam(c)
	if !ok {
		return
	}
	noteID, err := strconv.ParseUint(c.Param("noteId"), 10, 32)
	if err != nil || noteID == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "note id must be a positive integer")
		return
	}

	err = h.svc.DeleteNote(c.Request.Context(), CurrentUserID(c), itemID, uint(noteID))
	switch {
	case errors.Is(err, books.ErrItemNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "tracking item not found")
	case errors.Is(err, books.ErrNoteNotFound):
		Error(c, http.StatusNotFound, CodeNoteNotFound, "note not found")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "deleting note failed")
	default:
		c.Status(http.StatusNoContent)
	}
}
