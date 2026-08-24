package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// validThemes is the authoritative list of supported theme identifiers.
var validThemes = []string{"dark-espresso", "midnight-oled", "cream-paper", "dracula", "obsidian"}

// RegisterThemeRoutes registers the theme persistence endpoints.
// It should be called on the protected router group.
func RegisterThemeRoutes(grp *gin.RouterGroup, db *gorm.DB) {
	h := &themeHandler{db: db}
	grp.GET("/themes", h.listThemes)
	grp.GET("/user/theme", h.getTheme)
	grp.PATCH("/user/theme", h.updateTheme)
}

type themeHandler struct {
	db *gorm.DB
}

type themeResponse struct {
	Theme string `json:"theme"`
}

type themeUpdatePayload struct {
	Theme string `json:"theme" binding:"required,oneof=dark-espresso midnight-oled cream-paper dracula obsidian"`
}

// listThemes returns the list of supported theme identifiers.
func (h *themeHandler) listThemes(c *gin.Context) {
	c.JSON(http.StatusOK, validThemes)
}

// getTheme returns the current authenticated user's theme.
func (h *themeHandler) getTheme(c *gin.Context) {
	uid := CurrentUserID(c)
	var user models.User
	if err := h.db.First(&user, uid).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			Error(c, http.StatusNotFound, codeUserNotFound, "user not found")
		} else {
			Error(c, http.StatusInternalServerError, CodeInternal, "loading user theme")
		}
		return
	}
	// If Theme is empty (unlikely due to DB default), fallback to "dark-espresso"
	theme := user.Theme
	if theme == "" {
		theme = "dark-espresso"
	}
	c.JSON(http.StatusOK, themeResponse{Theme: theme})
}

// updateTheme updates the authenticated user's theme.
func (h *themeHandler) updateTheme(c *gin.Context) {
	uid := CurrentUserID(c)
	var payload themeUpdatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		AbortError(c, http.StatusBadRequest, CodeInvalidRequest, "invalid theme payload")
		return
	}
	// Update the user record
	if err := h.db.Model(&models.User{}).Where("id = ?", uid).Update("theme", payload.Theme).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "updating user theme")
		return
	}
	c.JSON(http.StatusOK, themeResponse{Theme: payload.Theme})
}
