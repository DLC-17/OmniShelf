package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/logistics"
)

// RegisterLogisticsRoutes attaches the logistics endpoints to the JWT-guarded /api group.
func RegisterLogisticsRoutes(grp *gin.RouterGroup, svc *logistics.Service) {
	h := &logisticsHandler{svc: svc}
	grp.GET("/logistics/locations", h.listLocations)
	grp.GET("/logistics/locations/:name", h.getLocation)
	grp.GET("/logistics/labels", h.getLabels)
	grp.POST("/logistics/labels", h.generateLabels)
}

type logisticsHandler struct {
	svc *logistics.Service
}

type locationSummaryDTO struct {
	Location      string                   `json:"location"`
	ContainerType string                   `json:"containerType"`
	ItemCount     int                      `json:"itemCount"`
	ItemTypes     map[string]int           `json:"itemTypes"`
	FilterURL     string                   `json:"filterUrl"`
	BinderDetails *logistics.BinderDetails `json:"binderDetails,omitempty"`
	Items         []containerItemDTO       `json:"items,omitempty"`
}

type containerItemDTO struct {
	ID          uint      `json:"id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	ExternalID  string    `json:"externalId"`
	Status      string    `json:"status"`
	SubLocation string    `json:"subLocation,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func toLocationSummaryDTO(s logistics.LocationSummary) locationSummaryDTO {
	items := make([]containerItemDTO, 0, len(s.Items))
	for _, it := range s.Items {
		items = append(items, containerItemDTO{
			ID:          it.ID,
			Type:        it.Type,
			Title:       it.Title,
			ExternalID:  it.ExternalID,
			Status:      it.Status,
			SubLocation: it.SubLocation,
			UpdatedAt:   it.UpdatedAt,
		})
	}
	return locationSummaryDTO{
		Location:      s.Location,
		ContainerType: string(s.ContainerType),
		ItemCount:     s.ItemCount,
		ItemTypes:     s.ItemTypes,
		FilterURL:     s.FilterURL,
		BinderDetails: s.BinderDetails,
		Items:         items,
	}
}

// listLocations handles GET /api/logistics/locations.
func (h *logisticsHandler) listLocations(c *gin.Context) {
	userID := CurrentUserID(c)
	filter := logistics.LocationFilter{
		Type:          c.Query("type"),
		ContainerType: logistics.ContainerType(c.Query("containerType")),
	}
	if filter.ContainerType == "" {
		filter.ContainerType = logistics.ContainerType(c.Query("container_type"))
	}

	locations, err := h.svc.ListLocations(c.Request.Context(), userID, filter)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "listing locations failed")
		return
	}

	out := make([]locationSummaryDTO, 0, len(locations))
	for _, loc := range locations {
		out = append(out, toLocationSummaryDTO(loc))
	}
	c.JSON(http.StatusOK, out)
}

// getLocation handles GET /api/logistics/locations/:name.
func (h *logisticsHandler) getLocation(c *gin.Context) {
	userID := CurrentUserID(c)
	name := c.Param("name")
	if strings.TrimSpace(name) == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "location name required")
		return
	}

	details, err := h.svc.GetLocationDetails(c.Request.Context(), userID, name)
	switch {
	case errors.Is(err, logistics.ErrLocationNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "location not found")
	case errors.Is(err, logistics.ErrEmptyLocation):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "location name cannot be empty")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "fetching location details failed")
	default:
		c.JSON(http.StatusOK, toLocationSummaryDTO(*details))
	}
}

type generateLabelsRequest struct {
	Format         string               `json:"format"`
	Locations      []string             `json:"locations"`
	BaseURL        string               `json:"baseUrl"`
	IncludeBarcode *bool                `json:"includeBarcode"`
	IncludeQR      *bool                `json:"includeQR"`
	CustomLabels   []customLabelRequest `json:"customLabels"`
	As             string               `json:"as"` // "svg" or "json"
}

type customLabelRequest struct {
	Location      string `json:"location"`
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle"`
	ContainerType string `json:"containerType"`
	Barcode       string `json:"barcode"`
}

type labelsResponseDTO struct {
	Format     string `json:"format"`
	LabelCount int    `json:"labelCount"`
	SVG        string `json:"svg"`
}

// getLabels handles GET /api/logistics/labels.
func (h *logisticsHandler) getLabels(c *gin.Context) {
	userID := CurrentUserID(c)
	formatStr := c.DefaultQuery("format", string(logistics.FormatAvery5160))
	format := logistics.LabelFormat(formatStr)

	baseURL := c.Query("baseUrl")
	if baseURL == "" {
		baseURL = c.Query("base_url")
	}
	if baseURL == "" {
		// Infer base URL from request host if possible
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		if c.Request.Host != "" {
			baseURL = scheme + "://" + c.Request.Host
		}
	}

	var locations []string
	if loc := c.Query("location"); loc != "" {
		locations = append(locations, loc)
	}
	if locs := c.Query("locations"); locs != "" {
		for _, s := range strings.Split(locs, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				locations = append(locations, s)
			}
		}
	}

	includeBarcode := true
	if bc := c.Query("includeBarcode"); bc == "false" || c.Query("barcode") == "false" {
		includeBarcode = false
	}
	includeQR := true
	if qr := c.Query("includeQR"); qr == "false" || c.Query("qr") == "false" {
		includeQR = false
	}

	opts := logistics.LabelOptions{
		Format:         format,
		BaseURL:        baseURL,
		IncludeBarcode: includeBarcode,
		IncludeQR:      includeQR,
	}

	svg, finalLabels, err := h.svc.GenerateLabelsForUser(c.Request.Context(), userID, opts, locations, nil)
	if err != nil {
		if errors.Is(err, logistics.ErrInvalidFormat) {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid label format (supported: avery5160, thermal4x6, thermal2x1)")
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "generating labels failed")
		return
	}

	as := strings.ToLower(c.Query("as"))
	accept := c.GetHeader("Accept")
	if as == "json" || (as == "" && strings.Contains(accept, "application/json") && !strings.Contains(accept, "image/svg+xml")) {
		c.JSON(http.StatusOK, labelsResponseDTO{
			Format:     string(format),
			LabelCount: len(finalLabels),
			SVG:        svg,
		})
		return
	}

	c.Header("Content-Type", "image/svg+xml; charset=utf-8")
	c.String(http.StatusOK, svg)
}

// generateLabels handles POST /api/logistics/labels.
func (h *logisticsHandler) generateLabels(c *gin.Context) {
	userID := CurrentUserID(c)
	var req generateLabelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid request body")
		return
	}

	formatStr := req.Format
	if formatStr == "" {
		formatStr = string(logistics.FormatAvery5160)
	}
	format := logistics.LabelFormat(formatStr)

	baseURL := req.BaseURL
	if baseURL == "" {
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		if c.Request.Host != "" {
			baseURL = scheme + "://" + c.Request.Host
		}
	}

	includeBarcode := true
	if req.IncludeBarcode != nil {
		includeBarcode = *req.IncludeBarcode
	}
	includeQR := true
	if req.IncludeQR != nil {
		includeQR = *req.IncludeQR
	}

	opts := logistics.LabelOptions{
		Format:         format,
		BaseURL:        baseURL,
		IncludeBarcode: includeBarcode,
		IncludeQR:      includeQR,
	}

	var customLabels []logistics.ShelfLabel
	for _, cl := range req.CustomLabels {
		if strings.TrimSpace(cl.Location) == "" {
			continue
		}
		cType := logistics.ContainerType(cl.ContainerType)
		if cType == "" {
			cType = logistics.DetectContainerType(cl.Location)
		}
		customLabels = append(customLabels, logistics.ShelfLabel{
			Location:      cl.Location,
			Title:         cl.Title,
			Subtitle:      cl.Subtitle,
			ContainerType: cType,
			Barcode:       cl.Barcode,
		})
	}

	svg, finalLabels, err := h.svc.GenerateLabelsForUser(c.Request.Context(), userID, opts, req.Locations, customLabels)
	if err != nil {
		if errors.Is(err, logistics.ErrInvalidFormat) {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid label format (supported: avery5160, thermal4x6, thermal2x1)")
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "generating labels failed")
		return
	}

	as := strings.ToLower(req.As)
	accept := c.GetHeader("Accept")
	if as == "json" || (as == "" && strings.Contains(accept, "application/json") && !strings.Contains(accept, "image/svg+xml")) {
		c.JSON(http.StatusOK, labelsResponseDTO{
			Format:     string(format),
			LabelCount: len(finalLabels),
			SVG:        svg,
		})
		return
	}

	c.Header("Content-Type", "image/svg+xml; charset=utf-8")
	c.String(http.StatusOK, svg)
}
