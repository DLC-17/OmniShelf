package api

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterHealthRoutes wires the unauthenticated liveness/readiness probe.
// It is attached to the bare engine — NOT the JWT-protected group — so the
// Docker HEALTHCHECK and TrueNAS can reach it without a session cookie
// (.ai/testing_and_health.md "Container health").
func RegisterHealthRoutes(r *gin.Engine, gdb *gorm.DB, imagesDir string) {
	h := &healthHandler{db: gdb, imagesDir: imagesDir}
	r.GET("/api/health", h.check)
}

type healthHandler struct {
	db        *gorm.DB
	imagesDir string
}

// check performs a 1-row SQLite ping and verifies the images directory is a
// reachable, mounted volume. Both healthy → 200 {"status":"ok","db":"ok"};
// either unavailable → 503 with a human-readable detail so the failure shows
// up in the container logs / orchestrator (spec E1/E14).
func (h *healthHandler) check(c *gin.Context) {
	if err := h.pingDB(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
			"db":     "error",
			"detail": err.Error(),
		})
		return
	}

	if err := h.checkImagesDir(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
			"db":     "ok",
			"images": "error",
			"detail": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "db": "ok"})
}

// pingDB runs the cheapest possible query against the shared connection. A
// closed pool or an unwritable/locked database surfaces as an error here.
func (h *healthHandler) pingDB() error {
	var n int
	return h.db.Raw("SELECT 1").Scan(&n).Error
}

// checkImagesDir confirms the images volume is still mounted and is a
// directory. A vanished mount (E14) must fail the probe, not 200.
func (h *healthHandler) checkImagesDir() error {
	info, err := os.Stat(h.imagesDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &os.PathError{Op: "stat", Path: h.imagesDir, Err: os.ErrInvalid}
	}
	return nil
}
