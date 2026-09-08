package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// defaultResetPassword is the temporary password applied during administrator resets.
const defaultResetPassword = "admin"

type adminHandler struct {
	db *gorm.DB
}

// RegisterAdminRoutes attaches the user administration endpoints under /api/admin,
// guarded by AdminRequired.
func RegisterAdminRoutes(grp *gin.RouterGroup, gdb *gorm.DB) {
	h := &adminHandler{db: gdb}
	admin := grp.Group("/admin", AdminRequired(gdb))
	admin.GET("/users", h.listUsers)
	admin.POST("/users/:id/reset-password", h.resetPassword)
	admin.POST("/users/:id/toggle-admin", h.toggleAdmin)
}

type adminUserDTO struct {
	ID                 uint      `json:"id"`
	Username           string    `json:"username"`
	IsAdmin            bool      `json:"isAdmin"`
	MustChangePassword bool      `json:"mustChangePassword"`
	CreatedAt          time.Time `json:"createdAt"`
}

// listUsers handles GET /api/admin/users — returns all registered users.
func (h *adminHandler) listUsers(c *gin.Context) {
	var users []models.User
	if err := h.db.WithContext(c.Request.Context()).Order("id ASC").Find(&users).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "listing users failed")
		return
	}
	out := make([]adminUserDTO, 0, len(users))
	for _, u := range users {
		out = append(out, adminUserDTO{
			ID:                 u.ID,
			Username:           u.Username,
			IsAdmin:            u.IsAdmin,
			MustChangePassword: u.MustChangePassword,
			CreatedAt:          u.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

// resetPassword handles POST /api/admin/users/:id/reset-password.
// Resets the target user's password to "admin" and sets must_change_password = true.
// Prevents admins from resetting their own password via the panel.
func (h *adminHandler) resetPassword(c *gin.Context) {
	targetID, ok := userIDParam(c)
	if !ok {
		return
	}
	if targetID == CurrentUserID(c) {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "administrators cannot reset their own password via the admin panel")
		return
	}

	var target models.User
	if err := h.db.WithContext(c.Request.Context()).First(&target, targetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Error(c, http.StatusNotFound, CodeNotFound, "user not found")
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "loading user failed")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(defaultResetPassword), bcryptCost)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "hashing password failed")
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Model(&target).Updates(map[string]any{
		"password_hash":        string(hash),
		"must_change_password": true,
	}).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "resetting password failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password reset to temporary value"})
}

// toggleAdmin handles POST /api/admin/users/:id/toggle-admin.
// Promotes a regular user to admin or demotes an admin to regular user.
// Prevents admins from modifying their own admin status.
func (h *adminHandler) toggleAdmin(c *gin.Context) {
	targetID, ok := userIDParam(c)
	if !ok {
		return
	}
	if targetID == CurrentUserID(c) {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "administrators cannot modify their own admin privileges")
		return
	}

	var target models.User
	if err := h.db.WithContext(c.Request.Context()).First(&target, targetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Error(c, http.StatusNotFound, CodeNotFound, "user not found")
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "loading user failed")
		return
	}

	newStatus := !target.IsAdmin
	if err := h.db.WithContext(c.Request.Context()).Model(&target).Update("is_admin", newStatus).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "updating admin status failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{"isAdmin": newStatus})
}

func userIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "user id must be a positive integer")
		return 0, false
	}
	return uint(id), true
}
