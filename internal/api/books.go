package api

import (
	"errors"
	"net/http"
	"strings"

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
	grp.GET("/books/search", h.search)
	grp.GET("/books/editions", h.editions)
	grp.GET("/books/discover", h.discover)
	grp.POST("/books/discover/reject", h.rejectRec)
}

// bookSearchResult is one work from an OpenLibrary title search. The workKey is
// used to list editions; the client then adds a chosen edition's ISBN via the
// existing /books/scan + /books/track path.
type bookSearchResult struct {
	WorkKey      string `json:"workKey"`
	Title        string `json:"title"`
	Authors      string `json:"authors"` // comma-joined
	FirstYear    int    `json:"firstYear"`
	EditionCount int    `json:"editionCount"`
	CoverID      int    `json:"coverId"` // OpenLibrary cover id for the cover proxy; 0 when none
}

// bookEdition is one ISBN-bearing edition offered in the edition picker.
type bookEdition struct {
	ISBN13      string `json:"isbn13"`
	Title       string `json:"title"`
	PublishDate string `json:"publishDate"`
}

// search handles GET /api/books/search?q= — an OpenLibrary title-search proxy.
func (h *booksHandler) search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "query parameter q is required")
		return
	}

	results, err := h.svc.SearchTitle(c.Request.Context(), q)
	switch {
	case errors.Is(err, books.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "OpenLibrary is unreachable, try again")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "search failed")
	default:
		out := make([]bookSearchResult, 0, len(results))
		for _, r := range results {
			out = append(out, bookSearchResult{
				WorkKey:      r.WorkKey,
				Title:        r.Title,
				Authors:      strings.Join(r.Authors, ", "),
				FirstYear:    r.FirstYear,
				EditionCount: r.EditionCount,
				CoverID:      r.CoverID,
			})
		}
		c.JSON(http.StatusOK, gin.H{"results": out})
	}
}

// editions handles GET /api/books/editions?workKey= — the ISBN picker for a
// title-search work.
func (h *booksHandler) editions(c *gin.Context) {
	workKey := strings.TrimSpace(c.Query("workKey"))
	if workKey == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "query parameter workKey is required")
		return
	}

	editions, err := h.svc.ListEditions(c.Request.Context(), workKey)
	switch {
	case errors.Is(err, books.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "OpenLibrary is unreachable, try again")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "loading editions failed")
	default:
		out := make([]bookEdition, 0, len(editions))
		for _, e := range editions {
			out = append(out, bookEdition{ISBN13: e.ISBN13, Title: e.Title, PublishDate: e.PublishDate})
		}
		c.JSON(http.StatusOK, gin.H{"editions": out})
	}
}

// discover handles GET /api/books/discover — book suggestions via an
// author/subject heuristic over the user's tracked books, each tagged with what
// it was suggested from. Covers are pre-cached through internal/images;
// coverPath is a relative /images path. workKey is the identity the client adds
// (via the editions → scan → track flow).
func (h *booksHandler) discover(c *gin.Context) {
	items, err := h.svc.Discover(c.Request.Context(), CurrentUserID(c))
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading suggestions failed")
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		out = append(out, gin.H{
			"workKey":     it.WorkKey,
			"title":       it.Title,
			"authors":     it.Authors,
			"year":        it.Year,
			"coverPath":   it.CoverPath,
			"suggestedBy": it.SuggestedBy,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// rejectRec handles POST /api/books/discover/reject {workKey} — hide a
// suggestion so it is not surfaced again.
func (h *booksHandler) rejectRec(c *gin.Context) {
	var body struct {
		WorkKey string `json:"workKey"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.WorkKey) == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "body must include a non-empty workKey")
		return
	}
	if err := h.svc.RejectRec(c.Request.Context(), CurrentUserID(c), body.WorkKey); err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "could not reject suggestion")
		return
	}
	c.Status(http.StatusNoContent)
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

// scan handles POST /api/books/scan {isbn}.
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

// track handles POST /api/books/track {bookId, status}.
// The tracking item is always created for the JWT user.
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
