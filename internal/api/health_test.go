package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
)

// healthTestRouter builds an engine with only the (unauthenticated) health
// route registered, plus a real SQLite DB and a writable images dir.
func healthTestRouter(t *testing.T) (*gin.Engine, *gorm.DB, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	imagesDir := t.TempDir()
	r := gin.New()
	RegisterHealthRoutes(r, gdb, imagesDir)
	return r, gdb, imagesDir
}

// TestHealthOK: healthy DB + images dir → 200 {"status":"ok","db":"ok"}.
// No cookie is sent, proving the route is reachable without a JWT.
func TestHealthOK(t *testing.T) {
	r, _, _ := healthTestRouter(t)

	w := doJSON(r, http.MethodGet, "/api/health", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "ok", body["db"])
}

// TestHealthDBDown: closing the pool makes the 1-row ping fail → 503 with a
// detail message and db:"error".
func TestHealthDBDown(t *testing.T) {
	r, gdb, _ := healthTestRouter(t)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	w := doJSON(r, http.MethodGet, "/api/health", nil)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "unavailable", body["status"])
	assert.Equal(t, "error", body["db"])
	assert.NotEmpty(t, body["detail"])
}

// TestHealthImagesDirMissing: an unmounted/absent images volume (E14) fails
// the probe → 503 while the DB still reports ok.
func TestHealthImagesDirMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	r := gin.New()
	RegisterHealthRoutes(r, gdb, missing)

	w := doJSON(r, http.MethodGet, "/api/health", nil)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "unavailable", body["status"])
	assert.Equal(t, "ok", body["db"])
	assert.Equal(t, "error", body["images"])
	assert.NotEmpty(t, body["detail"])
}
