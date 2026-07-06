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

// Sentinel errors returned by the auth service functions; the handlers
// translate them into envelope responses.
var (
	errInviteInvalid  = errors.New("invite code does not exist")
	errInviteUsed     = errors.New("invite code already used")
	errUsernameTaken  = errors.New("username already taken")
	errBadCredentials = errors.New("unknown username or wrong password")
)

type authHandler struct {
	db     *gorm.DB
	secret []byte
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

// register handles POST /api/auth/register.
func (a *authHandler) register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with username, password, and inviteCode")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	switch {
	case req.Username == "":
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "username must not be empty")
		return
	case len(req.Password) < minPasswordLen:
		Error(c, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLen))
		return
	case req.InviteCode == "":
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "inviteCode must not be empty")
		return
	}

	user, err := registerUser(c.Request.Context(), a.db, req.Username, req.Password, req.InviteCode)
	switch {
	case errors.Is(err, errInviteInvalid):
		Error(c, http.StatusBadRequest, CodeInviteInvalid, "invite code is not valid")
	case errors.Is(err, errInviteUsed):
		Error(c, http.StatusConflict, CodeInviteUsed, "invite code has already been used")
	case errors.Is(err, errUsernameTaken):
		Error(c, http.StatusConflict, CodeUsernameTaken, "username is already taken")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "registration failed")
	default:
		c.JSON(http.StatusCreated, userResponse{ID: user.ID, Username: user.Username})
	}
}

// login handles POST /api/auth/login.
func (a *authHandler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "request body must be JSON with username and password")
		return
	}

	user, err := authenticate(c.Request.Context(), a.db, strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		if errors.Is(err, errBadCredentials) {
			Error(c, http.StatusUnauthorized, CodeBadCredentials, "unknown username or wrong password")
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "login failed")
		return
	}

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
func authenticate(ctx context.Context, gdb *gorm.DB, username, password string) (*models.User, error) {
	var user models.User
	err := gdb.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errBadCredentials
		}
		return nil, fmt.Errorf("looking up user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, errBadCredentials
	}
	return &user, nil
}
