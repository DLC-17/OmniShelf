package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
)

func setupTestAdminEnv(t *testing.T, activeUser models.User) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, gdb.Create(&activeUser).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(userIDKey, activeUser.ID)
		c.Next()
	})

	apiGrp := r.Group("/api")
	RegisterAdminRoutes(apiGrp, gdb)

	auth := &authHandler{db: gdb, secret: []byte("test-jwt-secret-key-32-bytes!!")}
	apiGrp.POST("/auth/change-password", auth.changePassword)

	return r, gdb
}

func TestAdminRoutes_ForbiddenForRegularUser(t *testing.T) {
	regularUser := models.User{
		ID:           2,
		Username:     "regular",
		PasswordHash: "fakehash",
		IsAdmin:      false,
	}
	r, _ := setupTestAdminEnv(t, regularUser)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, CodeForbidden, env["error"])
}

func TestAdminRoutes_ListUsers(t *testing.T) {
	admin := models.User{
		ID:           1,
		Username:     "admin_user",
		PasswordHash: "fakehash",
		IsAdmin:      true,
	}
	r, gdb := setupTestAdminEnv(t, admin)

	u2 := models.User{Username: "bob", PasswordHash: "fakehash", IsAdmin: false}
	require.NoError(t, gdb.Create(&u2).Error)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var users []adminUserDTO
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &users))
	assert.Len(t, users, 2)
	assert.Equal(t, "admin_user", users[0].Username)
	assert.True(t, users[0].IsAdmin)
	assert.Equal(t, "bob", users[1].Username)
	assert.False(t, users[1].IsAdmin)
}

func TestAdminRoutes_ResetPassword(t *testing.T) {
	admin := models.User{ID: 1, Username: "admin_user", PasswordHash: "fakehash", IsAdmin: true}
	r, gdb := setupTestAdminEnv(t, admin)

	initialHash, _ := bcrypt.GenerateFromPassword([]byte("initial_pwd"), bcryptCost)
	target := models.User{ID: 2, Username: "target", PasswordHash: string(initialHash), IsAdmin: false}
	require.NoError(t, gdb.Create(&target).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/2/reset-password", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updated models.User
	require.NoError(t, gdb.First(&updated, 2).Error)
	assert.True(t, updated.MustChangePassword)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("admin")))
}

func TestAdminRoutes_SelfResetForbidden(t *testing.T) {
	admin := models.User{ID: 1, Username: "admin_user", PasswordHash: "fakehash", IsAdmin: true}
	r, _ := setupTestAdminEnv(t, admin)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/1/reset-password", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminRoutes_ToggleAdmin(t *testing.T) {
	admin := models.User{ID: 1, Username: "admin_user", PasswordHash: "fakehash", IsAdmin: true}
	r, gdb := setupTestAdminEnv(t, admin)

	target := models.User{ID: 2, Username: "target", PasswordHash: "fakehash", IsAdmin: false}
	require.NoError(t, gdb.Create(&target).Error)

	// Promote
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/2/toggle-admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var updated models.User
	require.NoError(t, gdb.First(&updated, 2).Error)
	assert.True(t, updated.IsAdmin)

	// Demote
	req2 := httptest.NewRequest(http.MethodPost, "/api/admin/users/2/toggle-admin", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	require.NoError(t, gdb.First(&updated, 2).Error)
	assert.False(t, updated.IsAdmin)
}

func TestAuth_ChangePassword(t *testing.T) {
	user := models.User{
		ID:                 2,
		Username:           "bob",
		PasswordHash:       "fakehash",
		MustChangePassword: true,
	}
	r, gdb := setupTestAdminEnv(t, user)

	body, _ := json.Marshal(map[string]string{"newPassword": "brandNewSecurePassword123!"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updated models.User
	require.NoError(t, gdb.First(&updated, 2).Error)
	assert.False(t, updated.MustChangePassword)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("brandNewSecurePassword123!")))
}

func TestAdminBootstrapMigration(t *testing.T) {
	tempDir := t.TempDir()
	gdb, err := db.Open(tempDir)
	require.NoError(t, err)

	// Seed user with is_admin = false
	u := models.User{ID: 1, Username: "first_user", PasswordHash: "hash", IsAdmin: false}
	require.NoError(t, gdb.Create(&u).Error)

	// Close DB and reopen to trigger startup migration
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	gdb2, err := db.Open(tempDir)
	require.NoError(t, err)
	sqlDB2, err := gdb2.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB2.Close() })

	var reloaded models.User
	require.NoError(t, gdb2.First(&reloaded, 1).Error)
	assert.True(t, reloaded.IsAdmin)
}

