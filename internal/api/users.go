package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// codeUserNotFound is the envelope code for a nonexistent member in
// /users/:id/library. Unexported to avoid clashing with domain codes owned
// by other files in this package.
const codeUserNotFound = "user_not_found"

type usersHandler struct{ db *gorm.DB }

// RegisterUserRoutes attaches the read-only member endpoints to the
// JWT-protected /api group. Deliberately no mutating routes:
// cross-user visibility is read-only.
func RegisterUserRoutes(grp *gin.RouterGroup, gdb *gorm.DB) {
	h := &usersHandler{db: gdb}
	grp.GET("/users", h.list)
	grp.GET("/users/:id/library", h.library)
}

type memberCounts struct {
	TV    int64 `json:"tv"`
	Books int64 `json:"books"`
}

type memberDTO struct {
	ID       uint         `json:"id"`
	Username string       `json:"username"`
	Counts   memberCounts `json:"counts"`
}

// list handles GET /api/users — instance members with tracked-item counts.
func (h *usersHandler) list(c *gin.Context) {
	ctx := c.Request.Context()

	var users []models.User
	if err := h.db.WithContext(ctx).Order("id").Find(&users).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading users")
		return
	}

	var rows []struct {
		UserID uint
		Type   string
		N      int64
	}
	err := h.db.WithContext(ctx).Model(&models.TrackingItem{}).
		Select("user_id, type, COUNT(*) AS n").
		Group("user_id, type").
		Scan(&rows).Error
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading item counts")
		return
	}
	counts := make(map[uint]memberCounts, len(users))
	for _, r := range rows {
		cts := counts[r.UserID]
		switch r.Type {
		case "TV":
			cts.TV = r.N
		case "BOOK":
			cts.Books = r.N
		}
		counts[r.UserID] = cts
	}

	out := make([]memberDTO, 0, len(users))
	for _, u := range users {
		out = append(out, memberDTO{ID: u.ID, Username: u.Username, Counts: counts[u.ID]})
	}
	c.JSON(http.StatusOK, out)
}

// userLibraryItem mirrors the owner library item shape ("same
// shape as own library endpoint").
type userLibraryItem struct {
	ID         uint      `json:"id"`
	Type       string    `json:"type"`
	ExternalID string    `json:"externalId"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// library handles GET /api/users/:id/library?type=&status= — a read-only
// view of another member's shelf. No PATCH/DELETE exist under /users.
func (h *usersHandler) library(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "user id must be a positive integer")
		return
	}
	ctx := c.Request.Context()

	var user models.User
	if err := h.db.WithContext(ctx).First(&user, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Error(c, http.StatusNotFound, codeUserNotFound, "no such user")
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "loading user")
		return
	}

	q := h.db.WithContext(ctx).Model(&models.TrackingItem{}).Where("user_id = ?", user.ID)
	if t := c.Query("type"); t != "" {
		if t != "TV" && t != "BOOK" {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, "type must be TV or BOOK")
			return
		}
		q = q.Where("type = ?", t)
	}
	if s := c.Query("status"); s != "" {
		switch s {
		case "WATCHING", "READING", "COMPLETED", "PLAN_TO":
			q = q.Where("status = ?", s)
		default:
			Error(c, http.StatusBadRequest, CodeInvalidRequest,
				"status must be one of WATCHING, READING, COMPLETED, PLAN_TO")
			return
		}
	}

	var items []models.TrackingItem
	if err := q.Order("updated_at DESC, id DESC").Find(&items).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading library")
		return
	}
	out := make([]userLibraryItem, 0, len(items))
	for _, it := range items {
		out = append(out, userLibraryItem{
			ID:         it.ID,
			Type:       it.Type,
			ExternalID: it.ExternalID,
			Title:      it.Title,
			Status:     it.Status,
			Progress:   it.Progress,
			UpdatedAt:  it.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}
