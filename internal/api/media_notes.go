package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
)

func RegisterDiaryRoutes(grp *gin.RouterGroup, gdb *gorm.DB) {
	h := &diaryHandler{db: gdb}
	grp.GET("/items/:id/diary", h.listNotes)
	grp.POST("/items/:id/diary", h.addNote)
	grp.DELETE("/diary/:noteId", h.deleteNote)
	grp.GET("/diary/sentiments", h.getSentiments)
}

type diaryHandler struct {
	db *gorm.DB
}

type noteDTO struct {
	ID          uint       `json:"id"`
	Body        string     `json:"body"`
	Sentiment   string     `json:"sentiment"`
	IsRewatch   bool       `json:"isRewatch"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func toNoteDTO(n models.Note) noteDTO {
	return noteDTO{
		ID:          n.ID,
		Body:        n.Body,
		Sentiment:   n.Sentiment,
		IsRewatch:   n.IsRewatch,
		CompletedAt: n.CompletedAt,
		CreatedAt:   n.CreatedAt,
		UpdatedAt:   n.UpdatedAt,
	}
}

func (h *diaryHandler) getSentiments(c *gin.Context) {
	c.JSON(http.StatusOK, models.UniversalSentiments)
}

func (h *diaryHandler) listNotes(c *gin.Context) {
	itemID, ok := uintParam(c, "id")
	if !ok {
		return
	}

	// Verify item ownership
	var item models.TrackingItem
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", itemID, CurrentUserID(c)).First(&item).Error; err != nil {
		Error(c, http.StatusNotFound, CodeNotFound, "item not found")
		return
	}

	var notes []models.Note
	if err := h.db.WithContext(c.Request.Context()).Where("item_id = ? AND user_id = ?", itemID, CurrentUserID(c)).Order("created_at desc").Find(&notes).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "fetching notes failed")
		return
	}

	out := make([]noteDTO, 0, len(notes))
	for _, n := range notes {
		out = append(out, toNoteDTO(n))
	}
	c.JSON(http.StatusOK, out)
}

type createNoteReq struct {
	Body        string     `json:"body"`
	Sentiment   string     `json:"sentiment"`   // Optional emotional tag (e.g. MIND_BLOWN, COZY, TEAR_JERKER, INSTANT_CLASSIC, THOUGHT_PROVOKING)
	IsRewatch   bool       `json:"isRewatch"`   // Optional re-watch/re-read/re-play/re-listen completion log flag
	CompletedAt *time.Time `json:"completedAt"` // Optional completion date
}

func (h *diaryHandler) addNote(c *gin.Context) {
	itemID, ok := uintParam(c, "id")
	if !ok {
		return
	}
	var req createNoteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Body) == "" && strings.TrimSpace(req.Sentiment) == "" && !req.IsRewatch {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "body, sentiment, or isRewatch is required")
		return
	}

	// Verify item ownership
	var item models.TrackingItem
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", itemID, CurrentUserID(c)).First(&item).Error; err != nil {
		Error(c, http.StatusNotFound, CodeNotFound, "item not found")
		return
	}

	completedAt := req.CompletedAt
	if req.IsRewatch && completedAt == nil {
		now := time.Now()
		completedAt = &now
	}

	note := models.Note{
		UserID:      CurrentUserID(c),
		ItemID:      itemID,
		MediaType:   item.Type,
		Body:        req.Body,
		Sentiment:   req.Sentiment,
		IsRewatch:   req.IsRewatch,
		CompletedAt: completedAt,
	}

	if err := h.db.WithContext(c.Request.Context()).Create(&note).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "creating note failed")
		return
	}

	c.JSON(http.StatusCreated, toNoteDTO(note))
}

func (h *diaryHandler) deleteNote(c *gin.Context) {
	noteID, ok := uintParam(c, "noteId")
	if !ok {
		return
	}

	res := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", noteID, CurrentUserID(c)).Delete(&models.Note{})
	if res.Error != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "deleting note failed")
		return
	}
	if res.RowsAffected == 0 {
		Error(c, http.StatusNotFound, CodeNotFound, "note not found")
		return
	}

	c.Status(http.StatusNoContent)
}
