package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
)

const testSecret = "test-secret"

func newTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	// Close the pool before TempDir cleanup: on Windows an open SQLite
	// handle makes the directory removal fail.
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := gin.New()
	RegisterRoutes(r, gdb, &config.Config{JWTSecret: testSecret})
	return r, gdb
}

func seedInvite(t *testing.T, gdb *gorm.DB, code string) {
	t.Helper()
	require.NoError(t, gdb.Create(&models.InviteCode{Code: code}).Error)
}

func doJSON(r *gin.Engine, method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func registerBody(username, code string) map[string]string {
	return map[string]string{"username": username, "password": "hunter2hunter2", "inviteCode": code}
}

func assertEnvelope(t *testing.T, w *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body: %s", w.Body.String())
	assert.Equal(t, wantCode, env.Error)
	assert.NotEmpty(t, env.Message)
}

func TestRegisterHappyPath(t *testing.T) {
	r, gdb := newTestRouter(t)
	seedInvite(t, gdb, "CODE-HAPPY")

	w := doJSON(r, http.MethodPost, "/api/auth/register", registerBody("alice", "CODE-HAPPY"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var user models.User
	require.NoError(t, gdb.Where("username = ?", "alice").First(&user).Error)
	assert.NotEqual(t, "hunter2hunter2", user.PasswordHash)

	var invite models.InviteCode
	require.NoError(t, gdb.Where("code = ?", "CODE-HAPPY").First(&invite).Error)
	assert.True(t, invite.IsUsed, "invite must be consumed")
}

func TestRegisterUsedInvite(t *testing.T) {
	r, gdb := newTestRouter(t)
	seedInvite(t, gdb, "CODE-ONCE")

	w := doJSON(r, http.MethodPost, "/api/auth/register", registerBody("alice", "CODE-ONCE"))
	require.Equal(t, http.StatusCreated, w.Code)

	w = doJSON(r, http.MethodPost, "/api/auth/register", registerBody("bob", "CODE-ONCE"))
	require.Equal(t, http.StatusConflict, w.Code)
	assertEnvelope(t, w, CodeInviteUsed)
}

func TestRegisterUnknownInvite(t *testing.T) {
	r, _ := newTestRouter(t)
	w := doJSON(r, http.MethodPost, "/api/auth/register", registerBody("alice", "NO-SUCH-CODE"))
	require.Equal(t, http.StatusBadRequest, w.Code)
	assertEnvelope(t, w, CodeInviteInvalid)
}

func TestRegisterValidation(t *testing.T) {
	r, _ := newTestRouter(t)
	cases := []struct {
		name string
		body map[string]string
	}{
		{"empty username", map[string]string{"username": " ", "password": "longenough", "inviteCode": "X"}},
		{"short password", map[string]string{"username": "alice", "password": "short", "inviteCode": "X"}},
		{"empty invite", map[string]string{"username": "alice", "password": "longenough", "inviteCode": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(r, http.MethodPost, "/api/auth/register", tc.body)
			require.Equal(t, http.StatusBadRequest, w.Code)
			assertEnvelope(t, w, CodeInvalidRequest)
		})
	}
}

// TestRegisterInviteRace exercises E11: two concurrent registrations on the
// same code — exactly one succeeds, the loser gets 409.
func TestRegisterInviteRace(t *testing.T) {
	r, gdb := newTestRouter(t)
	seedInvite(t, gdb, "CODE-RACE")

	results := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = doJSON(r, http.MethodPost, "/api/auth/register",
				registerBody(fmt.Sprintf("racer%d", i), "CODE-RACE"))
		}(i)
	}
	wg.Wait()

	codes := []int{results[0].Code, results[1].Code}
	assert.ElementsMatch(t, []int{http.StatusCreated, http.StatusConflict}, codes,
		"bodies: %s | %s", results[0].Body.String(), results[1].Body.String())

	var users int64
	require.NoError(t, gdb.Model(&models.User{}).Count(&users).Error)
	assert.EqualValues(t, 1, users, "exactly one registration must win")
}

func TestLoginSetsHttpOnlyCookie(t *testing.T) {
	r, gdb := newTestRouter(t)
	seedInvite(t, gdb, "CODE-LOGIN")
	require.Equal(t, http.StatusCreated,
		doJSON(r, http.MethodPost, "/api/auth/register", registerBody("alice", "CODE-LOGIN")).Code)

	w := doJSON(r, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "alice", "password": "hunter2hunter2"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	ck := cookies[0]
	assert.Equal(t, CookieName, ck.Name)
	assert.NotEmpty(t, ck.Value)
	assert.True(t, ck.HttpOnly, "cookie must be HttpOnly")
	assert.Equal(t, http.SameSiteLaxMode, ck.SameSite)
	assert.False(t, ck.Secure, "Secure must be off so LAN HTTP works")

	// Wrong password → 401 envelope.
	w = doJSON(r, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "alice", "password": "wrong-password"})
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assertEnvelope(t, w, CodeBadCredentials)
}

func TestMeWithAndWithoutCookie(t *testing.T) {
	r, gdb := newTestRouter(t)
	seedInvite(t, gdb, "CODE-ME")
	require.Equal(t, http.StatusCreated,
		doJSON(r, http.MethodPost, "/api/auth/register", registerBody("alice", "CODE-ME")).Code)
	login := doJSON(r, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "alice", "password": "hunter2hunter2"})
	require.Equal(t, http.StatusOK, login.Code)
	cookie := login.Result().Cookies()[0]

	// With valid cookie → 200 {id, username}.
	w := doJSON(r, http.MethodGet, "/api/auth/me", nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var me userResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &me))
	assert.Equal(t, "alice", me.Username)
	assert.NotZero(t, me.ID)

	// Without cookie → 401 envelope (E12).
	w = doJSON(r, http.MethodGet, "/api/auth/me", nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assertEnvelope(t, w, CodeUnauthorized)

	// Garbage token → 401 envelope.
	w = doJSON(r, http.MethodGet, "/api/auth/me", nil,
		&http.Cookie{Name: CookieName, Value: "not-a-jwt"})
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assertEnvelope(t, w, CodeUnauthorized)
}

func TestExpiredToken(t *testing.T) {
	r, gdb := newTestRouter(t)
	seedInvite(t, gdb, "CODE-EXP")
	reg := doJSON(r, http.MethodPost, "/api/auth/register", registerBody("alice", "CODE-EXP"))
	require.Equal(t, http.StatusCreated, reg.Code)
	var created userResponse
	require.NoError(t, json.Unmarshal(reg.Body.Bytes(), &created))

	// Token issued 8 days ago is past the 7-day TTL.
	expired, err := newToken([]byte(testSecret), created.ID, time.Now().Add(-8*24*time.Hour))
	require.NoError(t, err)

	w := doJSON(r, http.MethodGet, "/api/auth/me", nil,
		&http.Cookie{Name: CookieName, Value: expired})
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assertEnvelope(t, w, CodeUnauthorized)
}

func TestLogoutClearsCookie(t *testing.T) {
	r, gdb := newTestRouter(t)
	seedInvite(t, gdb, "CODE-OUT")
	require.Equal(t, http.StatusCreated,
		doJSON(r, http.MethodPost, "/api/auth/register", registerBody("alice", "CODE-OUT")).Code)
	login := doJSON(r, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "alice", "password": "hunter2hunter2"})
	cookie := login.Result().Cookies()[0]

	w := doJSON(r, http.MethodPost, "/api/auth/logout", nil, cookie)
	require.Equal(t, http.StatusNoContent, w.Code)
	cleared := w.Result().Cookies()
	require.Len(t, cleared, 1)
	assert.Equal(t, CookieName, cleared[0].Name)
	assert.Empty(t, cleared[0].Value)
	assert.Negative(t, cleared[0].MaxAge, "cookie must be expired")
}
