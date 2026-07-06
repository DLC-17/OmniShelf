package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/config"
)

// CookieName is the session cookie holding the JWT
// (.ai/architecture.md: HttpOnly + SameSite=Lax, named omnishelf_token).
const CookieName = "omnishelf_token"

// tokenTTL is the JWT lifetime (7-day expiry).
const tokenTTL = 7 * 24 * time.Hour

// userIDKey is the Gin context key under which the authenticated user's ID
// is stored by AuthRequired.
const userIDKey = "omnishelf_user_id"

// RegisterRoutes wires the auth endpoints and returns the JWT-protected
// /api route group. Later tasks (tv, books, feed, ...) attach their routes
// to the returned group so every /api route except /api/auth/login and
// /api/auth/register is guarded by the auth middleware.
func RegisterRoutes(r *gin.Engine, gdb *gorm.DB, cfg *config.Config) *gin.RouterGroup {
	secret := []byte(cfg.JWTSecret)
	a := &authHandler{db: gdb, secret: secret}

	r.POST("/api/auth/register", a.register)
	r.POST("/api/auth/login", a.login)

	protected := r.Group("/api", AuthRequired(secret))
	protected.POST("/auth/logout", a.logout)
	protected.GET("/auth/me", a.me)
	return protected
}

// AuthRequired validates the JWT from the omnishelf_token cookie and stores
// the user ID in the Gin context. Missing, malformed, or expired tokens get
// a 401 in the standard envelope (E12).
func AuthRequired(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(CookieName)
		if err != nil {
			AbortError(c, http.StatusUnauthorized, CodeUnauthorized, "authentication required")
			return
		}
		userID, err := parseToken(secret, raw)
		if err != nil {
			AbortError(c, http.StatusUnauthorized, CodeUnauthorized, "invalid or expired session")
			return
		}
		c.Set(userIDKey, userID)
		c.Next()
	}
}

// CurrentUserID returns the authenticated user's ID placed in the context by
// AuthRequired. It must only be called from handlers behind the middleware.
func CurrentUserID(c *gin.Context) uint {
	return c.GetUint(userIDKey)
}

// newToken issues a 7-day HMAC-SHA256 JWT whose subject is the user ID.
func newToken(secret []byte, userID uint, now time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatUint(uint64(userID), 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}

// parseToken validates signature, algorithm, and expiry, returning the user
// ID from the subject claim.
func parseToken(secret []byte, raw string) (uint, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return 0, fmt.Errorf("parsing token: %w", err)
	}
	id, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid subject claim %q", claims.Subject)
	}
	return uint(id), nil
}

// setSessionCookie writes (or, with an empty token and negative maxAge,
// clears) the session cookie. Secure is deliberately false: the app is
// served over plain HTTP on the LAN; Tailscale terminates HTTPS externally.
func setSessionCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CookieName, token, maxAge, "/", "", false, true)
}
