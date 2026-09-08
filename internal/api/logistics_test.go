package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/logistics"
	"github.com/davidlc1229/omnishelf/internal/models"
)

func setupTestLogisticsRouter(t *testing.T, userID uint) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(userIDKey, userID)
		c.Next()
	})

	apiGrp := r.Group("/api")
	svc := logistics.NewService(gdb)
	RegisterLogisticsRoutes(apiGrp, svc)

	// Seed items
	items := []models.TrackingItem{
		{
			UserID:     userID,
			Type:       "BOOK",
			ExternalID: "9780441172719",
			Title:      "Dune",
			Location:   "Shelf A",
			Status:     "READING",
			UpdatedAt:  time.Now(),
		},
		{
			UserID:     userID,
			Type:       "GAME",
			ExternalID: "119133",
			Title:      "Elden Ring",
			Location:   "Shelf A",
			Status:     "PLAYING",
			UpdatedAt:  time.Now(),
		},
		{
			UserID:     userID,
			Type:       "CARD",
			ExternalID: "ptcg:base1-4",
			Title:      "Charizard",
			Location:   "Binder 1 (P1/S1)",
			Status:     "OWNED",
			UpdatedAt:  time.Now(),
		},
		{
			UserID:     userID,
			Type:       "MOVIE",
			ExternalID: "603",
			Title:      "The Matrix",
			Location:   "Living Room Cabinet",
			Status:     "COMPLETED",
			UpdatedAt:  time.Now(),
		},
	}
	for _, it := range items {
		require.NoError(t, gdb.Create(&it).Error)
	}

	return r, gdb
}

func TestListLocationsAPI(t *testing.T) {
	r, _ := setupTestLogisticsRouter(t, 1)

	// 1. List all locations
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/logistics/locations", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res []locationSummaryDTO
	err := json.Unmarshal(w.Body.Bytes(), &res)
	require.NoError(t, err)
	assert.Len(t, res, 3) // "Binder 1 (P1/S1)", "Living Room Cabinet", "Shelf A"

	// 2. Filter by type=BOOK
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/logistics/locations?type=BOOK", nil)
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var res2 []locationSummaryDTO
	err = json.Unmarshal(w2.Body.Bytes(), &res2)
	require.NoError(t, err)
	assert.Len(t, res2, 1)
	assert.Equal(t, "Shelf A", res2[0].Location)
}

func TestGetLocationAPI(t *testing.T) {
	r, _ := setupTestLogisticsRouter(t, 1)

	// 1. Get existing location
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/logistics/locations/Shelf%20A", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var res locationSummaryDTO
	err := json.Unmarshal(w.Body.Bytes(), &res)
	require.NoError(t, err)
	assert.Equal(t, "Shelf A", res.Location)
	assert.Equal(t, 2, res.ItemCount)

	// 2. Get non-existent location
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/logistics/locations/NonExistent", nil)
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusNotFound, w2.Code)
}

func TestGetLabelsAPI(t *testing.T) {
	r, _ := setupTestLogisticsRouter(t, 1)

	// 1. Default SVG response
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/logistics/labels?format=avery5160", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/svg+xml; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "<svg")
	assert.Contains(t, w.Body.String(), "Shelf A")

	// 2. JSON response (as=json)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/logistics/labels?format=thermal4x6&location=Shelf%20A&as=json", nil)
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var res labelsResponseDTO
	err := json.Unmarshal(w2.Body.Bytes(), &res)
	require.NoError(t, err)
	assert.Equal(t, "thermal4x6", res.Format)
	assert.Equal(t, 1, res.LabelCount)
	assert.Contains(t, res.SVG, `width="4in"`)

	// 3. Invalid format
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/logistics/labels?format=invalid_format", nil)
	r.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusBadRequest, w3.Code)
}

func TestPostLabelsAPI(t *testing.T) {
	r, _ := setupTestLogisticsRouter(t, 1)

	body := generateLabelsRequest{
		Format: "thermal2x1",
		CustomLabels: []customLabelRequest{
			{
				Location:      "Custom Drawer 1",
				Title:         "Handhelds",
				ContainerType: "DRAWER",
			},
		},
		As: "json",
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/logistics/labels", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var res labelsResponseDTO
	err := json.Unmarshal(w.Body.Bytes(), &res)
	require.NoError(t, err)
	assert.Equal(t, "thermal2x1", res.Format)
	assert.Equal(t, 1, res.LabelCount)
	assert.Contains(t, res.SVG, `width="2in"`)
	assert.Contains(t, res.SVG, "Handhelds")

	// Direct SVG output on POST
	body.As = "svg"
	bodyBytes2, _ := json.Marshal(body)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/logistics/labels", bytes.NewReader(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.True(t, strings.HasPrefix(w2.Header().Get("Content-Type"), "image/svg+xml"))
	assert.Contains(t, w2.Body.String(), "<svg")
}
