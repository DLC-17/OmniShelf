package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/books"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tags"
)

func setupTestLibraryRouter(t *testing.T, userID uint) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	svc := books.NewService(gdb, nil, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(userIDKey, userID)
		c.Next()
	})

	apiGrp := r.Group("/api")
	RegisterLibraryRoutes(apiGrp, svc)

	return r, gdb
}

func TestLibraryPaginationAndSearch(t *testing.T) {
	r, gdb := setupTestLibraryRouter(t, 1)

	// Seed 5 books for user 1
	for i := 1; i <= 5; i++ {
		b := models.Book{
			ISBN13:      fmt.Sprintf("978000000000%d", i),
			Title:       fmt.Sprintf("Book %c", 'A'+i-1),
			Authors:     fmt.Sprintf("Author %d", i),
			Description: fmt.Sprintf("A long description for book %d that goes on and on to test description truncation behavior in list responses.", i),
		}
		require.NoError(t, gdb.Create(&b).Error)

		item := models.TrackingItem{
			UserID:     1,
			Type:       "BOOK",
			ExternalID: b.ISBN13,
			Title:      b.Title,
			Status:     "READING",
		}
		require.NoError(t, gdb.Create(&item).Error)
	}

	// 1. Fetch first page with limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/library?type=BOOK&limit=2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var page1 struct {
		Items      []itemResponse `json:"items"`
		TotalCount int64          `json:"totalCount"`
		HasMore    bool           `json:"hasMore"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page1))
	assert.Equal(t, int64(5), page1.TotalCount)
	assert.True(t, page1.HasMore)
	assert.Len(t, page1.Items, 2)
	assert.Equal(t, "Book A", page1.Items[0].Title)
	assert.Equal(t, "Book B", page1.Items[1].Title)

	// 2. Fetch second page with after cursor
	afterID := page1.Items[1].ID
	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/library?type=BOOK&limit=2&after=%d", afterID), nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var page2 struct {
		Items      []itemResponse `json:"items"`
		TotalCount int64          `json:"totalCount"`
		HasMore    bool           `json:"hasMore"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &page2))
	assert.Equal(t, int64(5), page2.TotalCount)
	assert.True(t, page2.HasMore)
	assert.Len(t, page2.Items, 2)
	assert.Equal(t, "Book C", page2.Items[0].Title)
	assert.Equal(t, "Book D", page2.Items[1].Title)

	// 3. Search query
	reqSearch := httptest.NewRequest(http.MethodGet, "/api/library?type=BOOK&search=Book%20E", nil)
	wSearch := httptest.NewRecorder()
	r.ServeHTTP(wSearch, reqSearch)
	assert.Equal(t, http.StatusOK, wSearch.Code)

	var searchPage struct {
		Items      []itemResponse `json:"items"`
		TotalCount int64          `json:"totalCount"`
		HasMore    bool           `json:"hasMore"`
	}
	require.NoError(t, json.Unmarshal(wSearch.Body.Bytes(), &searchPage))
	assert.Equal(t, int64(1), searchPage.TotalCount)
	assert.False(t, searchPage.HasMore)
	assert.Len(t, searchPage.Items, 1)
	assert.Equal(t, "Book E", searchPage.Items[0].Title)

	// 4. Test GET /api/items/:id
	reqItem := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/items/%d", page1.Items[0].ID), nil)
	wItem := httptest.NewRecorder()
	r.ServeHTTP(wItem, reqItem)
	assert.Equal(t, http.StatusOK, wItem.Code)

	var singleItem itemResponse
	require.NoError(t, json.Unmarshal(wItem.Body.Bytes(), &singleItem))
	assert.Equal(t, "Book A", singleItem.Title)
	assert.Contains(t, singleItem.Description, "A long description")

	// 5. Test GET /api/items/:id with invalid ID
	reqNotFound := httptest.NewRequest(http.MethodGet, "/api/items/99999", nil)
	wNotFound := httptest.NewRecorder()
	r.ServeHTTP(wNotFound, reqNotFound)
	assert.Equal(t, http.StatusNotFound, wNotFound.Code)

	// 6. Test invalid cursor (After pointing to non-existent ID) returns 400 Bad Request
	reqBadCursor := httptest.NewRequest(http.MethodGet, "/api/library?type=BOOK&after=999999", nil)
	wBadCursor := httptest.NewRecorder()
	r.ServeHTTP(wBadCursor, reqBadCursor)
	assert.Equal(t, http.StatusBadRequest, wBadCursor.Code)

	// 7. Test tag filtering with pagination
	// Tag Book A (id 1), Book B (id 2), Book C (id 3) with "Sci-Fi"
	tagStore := tags.NewStore(gdb)
	var allBooks []models.Book
	require.NoError(t, gdb.Order("id").Find(&allBooks).Error)
	require.Len(t, allBooks, 5)
	for i := 0; i < 3; i++ {
		require.NoError(t, tagStore.Set(context.Background(), tags.TypeBook, allBooks[i].ID, []string{"Sci-Fi"}))
	}
	// Tag Book D with "Fantasy"
	require.NoError(t, tagStore.Set(context.Background(), tags.TypeBook, allBooks[3].ID, []string{"Fantasy"}))

	// Request page 1 with tag=sci-fi and limit=2: should return 2 items, totalCount=3, hasMore=true
	reqTagP1 := httptest.NewRequest(http.MethodGet, "/api/library?type=BOOK&tag=sci-fi&limit=2", nil)
	wTagP1 := httptest.NewRecorder()
	r.ServeHTTP(wTagP1, reqTagP1)
	assert.Equal(t, http.StatusOK, wTagP1.Code)

	var tagPage1 struct {
		Items      []itemResponse `json:"items"`
		TotalCount int64          `json:"totalCount"`
		HasMore    bool           `json:"hasMore"`
	}
	require.NoError(t, json.Unmarshal(wTagP1.Body.Bytes(), &tagPage1))
	assert.Equal(t, int64(3), tagPage1.TotalCount)
	assert.True(t, tagPage1.HasMore)
	assert.Len(t, tagPage1.Items, 2)
	assert.Equal(t, "Book A", tagPage1.Items[0].Title)
	assert.Equal(t, "Book B", tagPage1.Items[1].Title)

	// Request page 2 with tag=sci-fi, after=cursor, limit=2: should return 1 item, totalCount=3, hasMore=false
	tagAfterID := tagPage1.Items[1].ID
	reqTagP2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/library?type=BOOK&tag=sci-fi&limit=2&after=%d", tagAfterID), nil)
	wTagP2 := httptest.NewRecorder()
	r.ServeHTTP(wTagP2, reqTagP2)
	assert.Equal(t, http.StatusOK, wTagP2.Code)

	var tagPage2 struct {
		Items      []itemResponse `json:"items"`
		TotalCount int64          `json:"totalCount"`
		HasMore    bool           `json:"hasMore"`
	}
	require.NoError(t, json.Unmarshal(wTagP2.Body.Bytes(), &tagPage2))
	assert.Equal(t, int64(3), tagPage2.TotalCount)
	assert.False(t, tagPage2.HasMore)
	assert.Len(t, tagPage2.Items, 1)
	assert.Equal(t, "Book C", tagPage2.Items[0].Title)

	// 8. Test cross-media search (Game developer search)
	game := models.Game{
		ID:        100,
		Title:     "Portal 2",
		Developer: "Valve Corporation",
	}
	require.NoError(t, gdb.Create(&game).Error)
	gameItem := models.TrackingItem{
		UserID:     1,
		Type:       "GAME",
		ExternalID: "100",
		Title:      game.Title,
		Status:     "PLAYING",
	}
	require.NoError(t, gdb.Create(&gameItem).Error)

	reqGameSearch := httptest.NewRequest(http.MethodGet, "/api/library?type=GAME&search=Valve", nil)
	wGameSearch := httptest.NewRecorder()
	r.ServeHTTP(wGameSearch, reqGameSearch)
	assert.Equal(t, http.StatusOK, wGameSearch.Code)

	var gameSearchPage struct {
		Items      []itemResponse `json:"items"`
		TotalCount int64          `json:"totalCount"`
		HasMore    bool           `json:"hasMore"`
	}
	require.NoError(t, json.Unmarshal(wGameSearch.Body.Bytes(), &gameSearchPage))
	assert.Equal(t, int64(1), gameSearchPage.TotalCount)
	assert.Len(t, gameSearchPage.Items, 1)
	assert.Equal(t, "Portal 2", gameSearchPage.Items[0].Title)
}
