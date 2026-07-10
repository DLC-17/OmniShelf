package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/music"
)

// Machine error codes for the music endpoints.
const (
	CodeMusicNotFound     = "music_not_found"
	CodeMusicUnconfigured = "music_unconfigured"
)

// musicHandler serves the music endpoints: barcode scan (Discogs), name search
// (MusicBrainz), and the two add paths (track a scanned album / add a searched
// one).
type musicHandler struct {
	svc *music.Service
}

// RegisterMusicRoutes attaches the music endpoints to the JWT-protected /api
// group returned by RegisterRoutes. Mirrors RegisterGameRoutes +
// RegisterMovieRoutes.
func RegisterMusicRoutes(grp *gin.RouterGroup, svc *music.Service) {
	h := &musicHandler{svc: svc}
	grp.POST("/music/scan", h.scan)
	grp.POST("/music/track", h.track)
	grp.GET("/music/search", h.search)
	grp.POST("/music", h.add)
}

type musicScanRequest struct {
	Barcode string `json:"barcode"`
}

type musicTrackRequest struct {
	AlbumID uint   `json:"albumId"`
	Status  string `json:"status"`
}

type musicAddRequest struct {
	MBID   string `json:"mbid"`
	Status string `json:"status"`
}

// albumResponse is the JSON shape of an Album payload.
type albumResponse struct {
	ID            uint   `json:"id"`
	ExternalID    string `json:"externalId"`
	Artist        string `json:"artist"`
	Title         string `json:"title"`
	Year          int    `json:"year"`
	CoverPath     string `json:"coverPath"`
	Barcode       string `json:"barcode"`
	DiscogsID     int    `json:"discogsId"`
	MusicBrainzID string `json:"musicBrainzId"`
}

func toAlbumResponse(a *models.Album) albumResponse {
	return albumResponse{
		ID:            a.ID,
		ExternalID:    a.ExternalID,
		Artist:        a.Artist,
		Title:         a.Title,
		Year:          a.Year,
		CoverPath:     a.CoverPath,
		Barcode:       a.Barcode,
		DiscogsID:     a.DiscogsID,
		MusicBrainzID: a.MusicBrainzID,
	}
}

type musicSearchResult struct {
	MBID   string `json:"mbid"`
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Year   int    `json:"year"`
}

// scan handles POST /api/music/scan {barcode}.
func (h *musicHandler) scan(c *gin.Context) {
	var req musicScanRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Barcode == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with a non-empty barcode")
		return
	}

	album, err := h.svc.Scan(c.Request.Context(), req.Barcode)
	switch {
	case errors.Is(err, music.ErrInvalidBarcode):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "barcode must be an 8–14 digit UPC/EAN")
	case errors.Is(err, music.ErrUnconfigured):
		Error(c, http.StatusServiceUnavailable, CodeMusicUnconfigured, "Discogs is not configured; set OMNISHELF_DISCOGS_TOKEN to scan albums")
	case errors.Is(err, music.ErrNotFound):
		// Echo the barcode so the UI can surface it for a manual retry.
		c.JSON(http.StatusNotFound, gin.H{
			"error":   CodeMusicNotFound,
			"message": "no album found for this barcode",
			"barcode": req.Barcode,
		})
	case errors.Is(err, music.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "Discogs is unreachable, try again")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "scan failed")
	default:
		c.JSON(http.StatusOK, toAlbumResponse(album))
	}
}

// track handles POST /api/music/track {albumId, status} — shelve a scanned album.
func (h *musicHandler) track(c *gin.Context) {
	var req musicTrackRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.AlbumID == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with albumId and status")
		return
	}

	item, err := h.svc.Track(c.Request.Context(), CurrentUserID(c), req.AlbumID, req.Status)
	switch {
	case errors.Is(err, music.ErrInvalidStatus):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "status must be LISTENING, PLAN_TO, COMPLETED, or STOPPED")
	case errors.Is(err, music.ErrAlbumNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "album does not exist; scan it first")
	case errors.Is(err, music.ErrAlreadyTracked):
		c.JSON(http.StatusConflict, gin.H{
			"error":   CodeAlreadyTracked,
			"message": "you already track this album",
			"item":    toItemResponse(item),
		})
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "tracking failed")
	default:
		c.JSON(http.StatusCreated, toItemResponse(item))
	}
}

// search handles GET /api/music/search?q= — MusicBrainz name search proxy.
func (h *musicHandler) search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "query parameter q is required")
		return
	}
	res, err := h.svc.Search(c.Request.Context(), q)
	switch {
	case errors.Is(err, music.ErrInvalidQuery):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "query parameter q is required")
	case errors.Is(err, music.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "MusicBrainz is unreachable, try again")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "search failed")
	default:
		results := make([]musicSearchResult, 0, len(res))
		for _, r := range res {
			results = append(results, musicSearchResult{MBID: r.MBID, Artist: r.Artist, Title: r.Title, Year: r.Year})
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
	}
}

// add handles POST /api/music {mbid, status} — add a MusicBrainz-searched album.
func (h *musicHandler) add(c *gin.Context) {
	var req musicAddRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.MBID) == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with a non-empty mbid")
		return
	}

	res, err := h.svc.AddByMusicBrainz(c.Request.Context(), CurrentUserID(c), req.MBID, req.Status)
	switch {
	case errors.Is(err, music.ErrInvalidStatus):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "status must be LISTENING, PLAN_TO, COMPLETED, or STOPPED")
	case errors.Is(err, music.ErrNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "no album found for this MusicBrainz id")
	case errors.Is(err, music.ErrUpstream):
		Error(c, http.StatusBadGateway, CodeUpstreamError, "MusicBrainz is unreachable, try again")
	case errors.Is(err, music.ErrAlreadyTracked):
		c.JSON(http.StatusConflict, gin.H{
			"error":   CodeAlreadyTracked,
			"message": "you already track this album",
			"album":   toAlbumResponse(&res.Album),
			"item":    toItemResponse(&res.Item),
		})
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "adding album failed")
	default:
		c.JSON(http.StatusCreated, gin.H{
			"album": toAlbumResponse(&res.Album),
			"item":  toItemResponse(&res.Item),
		})
	}
}
