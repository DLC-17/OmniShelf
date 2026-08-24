package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// RegisterCollectionRoutes attaches the collections endpoints to the JWT-guarded /api group.
func RegisterCollectionRoutes(grp *gin.RouterGroup, gdb *gorm.DB) {
	h := &collectionsHandler{db: gdb}
	grp.GET("/collections", h.list)
	grp.POST("/collections", h.create)
	grp.PUT("/collections/:id", h.update)
	grp.DELETE("/collections/:id", h.deleteCollection)
	grp.POST("/collections/:id/items", h.addItem)
	grp.DELETE("/collections/:id/items/:itemId", h.removeItem)
}

type collectionsHandler struct {
	db *gorm.DB
}

type collectionDTO struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toCollectionDTO(c models.UserCollection) collectionDTO {
	return collectionDTO{
		ID:        c.ID,
		Name:      c.Name,
		Slug:      c.Slug,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

func (h *collectionsHandler) list(c *gin.Context) {
	userID := CurrentUserID(c)
	var collections []models.UserCollection
	if err := h.db.WithContext(c.Request.Context()).Where("user_id = ?", userID).Find(&collections).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "fetching collections failed")
		return
	}
	out := make([]collectionDTO, 0, len(collections))
	for _, coll := range collections {
		out = append(out, toCollectionDTO(coll))
	}
	c.JSON(http.StatusOK, out)
}

type createCollectionReq struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug" binding:"required"`
}

func (h *collectionsHandler) create(c *gin.Context) {
	var req createCollectionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "name and slug are required")
		return
	}

	coll := models.UserCollection{
		UserID: CurrentUserID(c),
		Name:   req.Name,
		Slug:   req.Slug,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&coll).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "creating collection failed")
		return
	}
	c.JSON(http.StatusCreated, toCollectionDTO(coll))
}

type updateCollectionReq struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug" binding:"required"`
}

func (h *collectionsHandler) update(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		return
	}
	var req updateCollectionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "name and slug are required")
		return
	}

	var coll models.UserCollection
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, CurrentUserID(c)).First(&coll).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Error(c, http.StatusNotFound, CodeNotFound, "collection not found")
		} else {
			Error(c, http.StatusInternalServerError, CodeInternal, "fetching collection failed")
		}
		return
	}

	coll.Name = req.Name
	coll.Slug = req.Slug
	if err := h.db.WithContext(c.Request.Context()).Save(&coll).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "updating collection failed")
		return
	}
	c.JSON(http.StatusOK, toCollectionDTO(coll))
}

func (h *collectionsHandler) deleteCollection(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		return
	}
	userID := CurrentUserID(c)
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&models.UserCollection{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		// Also delete CollectionItem links
		if err := tx.Where("collection_id = ?", id).Delete(&models.CollectionItem{}).Error; err != nil {
			return err
		}
		return nil
	})
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "collection not found")
	case err != nil:
		Error(c, http.StatusInternalServerError, CodeInternal, "deleting collection failed")
	default:
		c.Status(http.StatusNoContent)
	}
}

type addItemReq struct {
	ItemID uint `json:"itemId" binding:"required"`
}

func (h *collectionsHandler) addItem(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		return
	}
	var req addItemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "itemId is required")
		return
	}

	// Verify the user owns the collection
	var coll models.UserCollection
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, CurrentUserID(c)).First(&coll).Error; err != nil {
		Error(c, http.StatusNotFound, CodeNotFound, "collection not found")
		return
	}

	// Verify the user owns the item
	var item models.TrackingItem
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", req.ItemID, CurrentUserID(c)).First(&item).Error; err != nil {
		Error(c, http.StatusNotFound, CodeNotFound, "item not found")
		return
	}

	collItem := models.CollectionItem{
		CollectionID: id,
		ItemID:       req.ItemID,
		AddedAt:      time.Now(),
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&collItem).Error; err != nil {
		// Swallow unique constraint violations (item already in collection)
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			c.JSON(http.StatusOK, gin.H{"success": true})
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "adding item to collection failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *collectionsHandler) removeItem(c *gin.Context) {
	id, ok := uintParam(c, "id")
	if !ok {
		return
	}
	itemId, ok := uintParam(c, "itemId")
	if !ok {
		return
	}

	// Verify the user owns the collection
	var coll models.UserCollection
	if err := h.db.WithContext(c.Request.Context()).Where("id = ? AND user_id = ?", id, CurrentUserID(c)).First(&coll).Error; err != nil {
		Error(c, http.StatusNotFound, CodeNotFound, "collection not found")
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Where("collection_id = ? AND item_id = ?", id, itemId).Delete(&models.CollectionItem{}).Error; err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "removing item failed")
		return
	}
	c.Status(http.StatusNoContent)
}

func uintParam(c *gin.Context, param string) (uint, bool) {
	valStr := c.Param(param)
	val, err := strconv.ParseUint(valStr, 10, 32)
	if err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid parameter")
		return 0, false
	}
	return uint(val), true
}
