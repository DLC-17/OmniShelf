package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// bcryptCost is fixed at 12.
const bcryptCost = 12

// minPasswordLen is the minimum accepted password length.
const minPasswordLen = 8

// maxPasswordLen matches bcrypt's 72-byte input limit; longer passwords must
// be rejected up front or GenerateFromPassword fails at registration time.
const maxPasswordLen = 72

// maxUsernameLen bounds usernames so they stay renderable in the UI and
// cannot be abused to bloat the database.
const maxUsernameLen = 32

// Sentinel errors returned by the auth service functions; the handlers
// translate them into envelope responses.
var (
	errInviteInvalid  = errors.New("invite code does not exist")
	errInviteUsed     = errors.New("invite code already used")
	errUsernameTaken  = errors.New("username already taken")
	errBadCredentials = errors.New("unknown username or wrong password")
)

// dummyHash is a bcrypt hash of a fixed internal string, computed once at
// process start. authenticate() compares against it when a username is not
// found so that the "unknown username" and "wrong password" code paths take
// statistically the same amount of time. Without this, the missing bcrypt
// call on the not-found path is a timing side-channel: an attacker can
// enumerate valid usernames purely from response latency even though both
// cases return the identical error message and HTTP status.
var dummyHash = mustDummyHash()

func mustDummyHash() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("omnishelf-timing-defense-fixed-value"), bcryptCost)
	if err != nil {
		// bcrypt only fails here on inputs longer than 72 bytes or an invalid
		// cost; the literal above is fixed and short, so this is unreachable.
		panic(fmt.Sprintf("generating dummy bcrypt hash: %v", err))
	}
	return h
}

type authHandler struct {
	db      *gorm.DB
	secret  []byte
	limiter *failureLimiter
}

type registerRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	InviteCode string `json:"inviteCode"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

// validUsername reports whether the (already trimmed) username is within
// length bounds and free of control characters.
func validUsername(name string) bool {
	if name == "" || len(name) > maxUsernameLen {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// register handles POST /api/auth/register. Failed invite-code guesses are
// rate limited per source IP so single-use codes cannot be brute forced.
func (a *authHandler) register(c *gin.Context) {
	if a.limiter.blocked(c.ClientIP()) {
		Error(c, http.StatusTooManyRequests, CodeRateLimited, "too many failed attempts; try again later")
		return
	}

	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with username, password, and inviteCode")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	switch {
	case !validUsername(req.Username):
		Error(c, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("username must be 1–%d characters without control characters", maxUsernameLen))
		return
	case len(req.Password) < minPasswordLen:
		Error(c, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLen))
		return
	case len(req.Password) > maxPasswordLen:
		Error(c, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("password must be at most %d bytes", maxPasswordLen))
		return
	case req.InviteCode == "":
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "inviteCode must not be empty")
		return
	}

	user, err := registerUser(c.Request.Context(), a.db, req.Username, req.Password, req.InviteCode)
	switch {
	case errors.Is(err, errInviteInvalid):
		a.limiter.fail(c.ClientIP())
		Error(c, http.StatusBadRequest, CodeInviteInvalid, "invite code is not valid")
	case errors.Is(err, errInviteUsed):
		a.limiter.fail(c.ClientIP())
		Error(c, http.StatusConflict, CodeInviteUsed, "invite code has already been used")
	case errors.Is(err, errUsernameTaken):
		Error(c, http.StatusConflict, CodeUsernameTaken, "username is already taken")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "registration failed")
	default:
		c.JSON(http.StatusCreated, userResponse{ID: user.ID, Username: user.Username})
	}
}

// login handles POST /api/auth/login. Failed attempts are rate limited per
// source IP to slow password brute force; a successful login resets the
// budget.
func (a *authHandler) login(c *gin.Context) {
	if a.limiter.blocked(c.ClientIP()) {
		Error(c, http.StatusTooManyRequests, CodeRateLimited, "too many failed attempts; try again later")
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with username and password")
		return
	}

	user, err := authenticate(c.Request.Context(), a.db, strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		if errors.Is(err, errBadCredentials) {
			a.limiter.fail(c.ClientIP())
			Error(c, http.StatusUnauthorized, CodeBadCredentials, "unknown username or wrong password")
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "login failed")
		return
	}
	a.limiter.reset(c.ClientIP())

	token, err := newToken(a.secret, user.ID, time.Now())
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "login failed")
		return
	}
	setSessionCookie(c, token, int(tokenTTL.Seconds()))
	c.JSON(http.StatusOK, userResponse{ID: user.ID, Username: user.Username})
}

// logout handles POST /api/auth/logout: clears the session cookie.
func (a *authHandler) logout(c *gin.Context) {
	setSessionCookie(c, "", -1)
	c.Status(http.StatusNoContent)
}

// me handles GET /api/auth/me: returns the authenticated user.
func (a *authHandler) me(c *gin.Context) {
	var user models.User
	err := a.db.WithContext(c.Request.Context()).First(&user, CurrentUserID(c)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Token is valid but the account is gone (e.g., deleted row).
			Error(c, http.StatusUnauthorized, CodeUnauthorized, "account no longer exists")
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "lookup failed")
		return
	}
	c.JSON(http.StatusOK, userResponse{ID: user.ID, Username: user.Username})
}

// registerUser consumes the invite code and creates the user inside a single
// transaction. The invite is claimed with a conditional update
// (WHERE is_used = false), so of two concurrent registrations on the same
// code exactly one wins and the loser sees errInviteUsed (E11).
func registerUser(ctx context.Context, gdb *gorm.DB, username, password, inviteCode string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	user := &models.User{Username: username, PasswordHash: string(hash)}
	err = gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.InviteCode{}).
			Where("code = ? AND is_used = ?", inviteCode, false).
			Update("is_used", true)
		if res.Error != nil {
			return fmt.Errorf("claiming invite code: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			// Distinguish "never existed" from "already consumed".
			var n int64
			if err := tx.Model(&models.InviteCode{}).Where("code = ?", inviteCode).Count(&n).Error; err != nil {
				return fmt.Errorf("checking invite code: %w", err)
			}
			if n == 0 {
				return errInviteInvalid
			}
			return errInviteUsed
		}
		if err := tx.Create(user).Error; err != nil {
			// glebarez/sqlite surfaces the unique index as a plain error
			// string; db.Open does not enable GORM error translation.
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				return errUsernameTaken
			}
			return fmt.Errorf("creating user: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

// authenticate verifies username/password, returning errBadCredentials for
// both unknown users and wrong passwords (no username enumeration).
//
// When the username does not exist, a bcrypt comparison against dummyHash is
// performed and discarded. This keeps the "unknown username" and "wrong
// password" branches statistically indistinguishable by response time:
// without it, the not-found path returns after a single indexed SELECT while
// the wrong-password path additionally pays bcrypt's ~100 ms cost, creating
// a timing side-channel that reveals valid usernames even when the error text
// and HTTP status are identical for both cases.
func authenticate(ctx context.Context, gdb *gorm.DB, username, password string) (*models.User, error) {
	var user models.User
	err := gdb.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			//nolint:errcheck // timing defense only — result is intentionally discarded
			bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return nil, errBadCredentials
		}
		return nil, fmt.Errorf("looking up user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, errBadCredentials
	}
	return &user, nil
}
