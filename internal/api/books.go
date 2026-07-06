package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/books"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// Machine error codes for the books and library endpoints.
const (
	CodeBookNotFound   = "book_not_found"
	CodeAlreadyTracked = "already_tracked"
	CodeNotFound       = "not_found"
	CodeUpstreamError  = "upstream_error"
)

// booksHandler serves POST /api/books/scan and /api/books/track.
type booksHandler struct {
	svc *books.Service
}

// RegisterBookRoutes attaches the book endpoints to the JWT-protected /api
// group returned by RegisterRoutes.
func RegisterBookRoutes(grp *gin.RouterGroup, svc *books.Service) {
	h := &booksHandler{svc: svc}
	grp.POST("/books/scan", h.scan)
	grp.POST("/books/track", h.track)
}

type scanRequest struct {
	ISBN string `json:"isbn"`
}

type trackRequest struct {
	BookID uint   `json:"bookId"`
	Status string `json:"status"`
}

// bookResponse is the JSON shape of a Book payload.
type bookResponse struct {
	ID        uint   `json:"id"`
	ISBN13    string `json:"isbn13"`
	Title     string `json:"title"`
	Authors   string `json:"authors"`
	CoverPath string `json:"coverPath"`
	PageCount int    `json:"pageCount"`
}

func toBookResponse(b *models.Book) bookResponse {
	return bookResponse{
		ID:        b.ID,
		ISBN13:    b.ISBN13,
		Title:     b.Title,
		Authors:   b.Authors,
		CoverPath: b.CoverPath,
		PageCount: b.PageCount,
	}
}

// scan handles POST /api/books/scan {isbn} (spec §2.5 step 3).
func (h *booksHandler) scan(c *gin.Context) {
	var req scanRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ISBN == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with a non-empty isbn")
		return
	}

	book, err := h.svc.Scan(c.Request.Context(), req.ISBN)
	switch {
	case errors.Is(err, books.ErrInvalidISBN):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "isbn must be a 13-digit ISBN-13 (978/979 prefix)")
	case errors.Is(err, books.ErrNotFound):
		// E4: echo the ISBN so the UI can pre-fill the manual-entry form.
		c.JSON(http.StatusNotFound, gin.H{
			"error":   CodeBookNotFound,
			"message": "no book found for this ISBN",
			"isbn":    req.ISBN,
		})
	case errors.Is(err, books.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "OpenLibrary is unreachable, try again")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "scan failed")
	default:
		c.JSON(http.StatusOK, toBookResponse(book))
	}
}

// track handles POST /api/books/track {bookId, status} (spec §2.5 step 4).
// The tracking item is always created for the JWT user (hard rule 7).
func (h *booksHandler) track(c *gin.Context) {
	var req trackRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.BookID == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with bookId and status")
		return
	}

	item, err := h.svc.Track(c.Request.Context(), CurrentUserID(c), req.BookID, req.Status)
	switch {
	case errors.Is(err, books.ErrInvalidStatus):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "status must be READING, PLAN_TO, or COMPLETED")
	case errors.Is(err, books.ErrBookNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "book does not exist; scan it first")
	case errors.Is(err, books.ErrAlreadyTracked):
		// E16: 409 with the existing item.
		c.JSON(http.StatusConflict, gin.H{
			"error":   CodeAlreadyTracked,
			"message": "you already track this book",
			"item":    toItemResponse(item),
		})
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "tracking failed")
	default:
		c.JSON(http.StatusCreated, toItemResponse(item))
	}
}
