package api

import (
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/musicbrainz"
	"github.com/davidlc1229/omnishelf/internal/openlibrary"
)

// maxCoverProxyBytes caps a proxied cover so a hostile/broken upstream cannot
// stream an unbounded body through us.
const maxCoverProxyBytes = 5 << 20 // 5 MiB

// coversHandler streams an upstream cover thumbnail for a SEARCH result through
// the backend. Search results are ephemeral (the item is not tracked yet, so its
// cover is not cached under /images), and the strict img-src 'self' CSP forbids
// the browser from hotlinking the source CDNs directly. Proxying keeps the image
// same-origin and lets us follow CoverArtArchive's redirect server-side. Only
// ids are accepted — never arbitrary URLs — so this cannot become an open proxy.
type coversHandler struct {
	igdb        *igdb.Client
	ol          *openlibrary.Client
	musicbrainz *musicbrainz.Client
	http        *http.Client
}

// RegisterCoverRoutes mounts the search cover proxy on the JWT-protected group.
func RegisterCoverRoutes(grp *gin.RouterGroup, ig *igdb.Client, ol *openlibrary.Client, mb *musicbrainz.Client) {
	h := &coversHandler{
		igdb:        ig,
		ol:          ol,
		musicbrainz: mb,
		http:        &http.Client{Timeout: 15 * time.Second},
	}
	grp.GET("/covers/:kind/:id", h.proxy)
}

var (
	reIGDBImageID = regexp.MustCompile(`^[a-zA-Z0-9_]+$`) // IGDB image_id, e.g. "co3p2d"
	reNumericID   = regexp.MustCompile(`^[0-9]+$`)        // OpenLibrary cover id
	reMBID        = regexp.MustCompile(`^[0-9a-fA-F-]+$`) // MusicBrainz release-group mbid
)

// proxy handles GET /api/covers/:kind/:id where kind is game|book|music and id
// is the source-specific cover key. It streams the upstream image bytes back
// with an image content-type, or 404 when the upstream has no cover.
func (h *coversHandler) proxy(c *gin.Context) {
	kind := c.Param("kind")
	id := c.Param("id")

	var url string
	switch kind {
	case "game":
		if !reIGDBImageID.MatchString(id) {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid cover id")
			return
		}
		url = h.igdb.CoverURL(id, "t_cover_big")
	case "book":
		if !reNumericID.MatchString(id) {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid cover id")
			return
		}
		n, _ := strconv.Atoi(id)
		url = h.ol.CoverURL(n, "M")
	case "music":
		if !reMBID.MatchString(id) {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid cover id")
			return
		}
		url = h.musicbrainz.CoverURL(id, 250)
	default:
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "unknown cover kind")
		return
	}
	if url == "" {
		c.Status(http.StatusNotFound)
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	resp, err := h.http.Do(req)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		c.Status(http.StatusNotFound)
		return
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		ct = "image/jpeg"
	}
	c.Header("Content-Type", ct)
	c.Header("Cache-Control", "public, max-age=86400")
	_, _ = io.CopyN(c.Writer, resp.Body, maxCoverProxyBytes)
}
