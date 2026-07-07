package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/movies"
)

// RegisterMovieRoutes attaches the movie endpoints to the JWT-guarded /api
// group returned by RegisterRoutes.
func RegisterMovieRoutes(grp *gin.RouterGroup, svc *movies.Service) {
	h := &moviesHandler{svc: svc}
	grp.GET("/movies/search", h.search)
	grp.POST("/movies", h.addMovie)
	grp.GET("/movies/discover", h.discover)
	grp.POST("/movies/discover/reject", h.rejectRec)
}

type moviesHandler struct {
	svc *movies.Service
}

type movieSearchResult struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Overview    string `json:"overview"`
	ReleaseDate string `json:"releaseDate"`
	PosterPath  string `json:"posterPath"`
}

type movieDTO struct {
	ID          uint   `json:"id"`
	TMDBID      int    `json:"tmdbId"`
	Title       string `json:"title"`
	PosterPath  string `json:"posterPath"`
	Overview    string `json:"overview"`
	ReleaseDate string `json:"releaseDate"`
}

func toMovieDTO(m models.Movie) movieDTO {
	return movieDTO{
		ID:          m.ID,
		TMDBID:      m.TMDBID,
		Title:       m.Title,
		PosterPath:  m.PosterPath,
		Overview:    m.Overview,
		ReleaseDate: m.ReleaseDate,
	}
}

// search handles GET /api/movies/search?q= — server-side TMDB proxy.
func (h *moviesHandler) search(c *gin.Context) {
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
	results := make([]movieSearchResult, 0, len(res.Results))
	for _, r := range res.Results {
		results = append(results, movieSearchResult{
			ID:          r.ID,
			Title:       r.Title,
			Overview:    r.Overview,
			ReleaseDate: r.ReleaseDate,
			PosterPath:  r.PosterPath,
		})
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// addMovie handles POST /api/movies {tmdbId}.
func (h *moviesHandler) addMovie(c *gin.Context) {
	var body struct {
		TMDBID int `json:"tmdbId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.TMDBID <= 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "body must include a positive tmdbId")
		return
	}
	res, err := h.svc.AddMovie(c.Request.Context(), CurrentUserID(c), body.TMDBID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"movie": toMovieDTO(res.Movie),
		"item":  toItemDTO(res.Item),
	})
}

// discover handles GET /api/movies/discover — movie suggestions based on what
// the user tracks, each tagged with the movie it was suggested from.
func (h *moviesHandler) discover(c *gin.Context) {
	items, err := h.svc.Discover(c.Request.Context(), CurrentUserID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		out = append(out, gin.H{
			"tmdbId":      it.TMDBID,
			"title":       it.Title,
			"overview":    it.Overview,
			"posterPath":  it.PosterPath,
			"releaseDate": it.ReleaseDate,
			"suggestedBy": it.SuggestedBy,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// rejectRec handles POST /api/movies/discover/reject {tmdbId}.
func (h *moviesHandler) rejectRec(c *gin.Context) {
	var body struct {
		TMDBID int `json:"tmdbId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.TMDBID <= 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "body must include a positive tmdbId")
		return
	}
	if err := h.svc.RejectRec(c.Request.Context(), CurrentUserID(c), body.TMDBID); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// writeError maps movie service errors onto the standard envelope: duplicate
// track → 409 with the existing item; missing movie → 404; TMDB unreachable →
// 502; anything else → 500.
func (h *moviesHandler) writeError(c *gin.Context, err error) {
	var conflict *movies.ConflictError
	var up *movies.UpstreamError
	switch {
	case errors.As(err, &conflict):
		c.JSON(http.StatusConflict, gin.H{
			"error":   CodeDuplicateItem,
			"message": "this movie is already in your library",
			"item":    toItemDTO(conflict.Existing),
		})
	case errors.Is(err, movies.ErrNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "the requested movie does not exist")
	case errors.As(err, &up):
		Error(c, http.StatusBadGateway, CodeTMDBUnavailable, "TMDB unreachable, try again")
	default:
		Error(c, http.StatusInternalServerError, CodeInternal, "something went wrong")
	}
}
