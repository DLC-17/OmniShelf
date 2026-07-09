package api

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterHealthRoutes wires the unauthenticated liveness/readiness probe.
// It is attached to the bare engine — NOT the JWT-protected group — so the
// Docker HEALTHCHECK and TrueNAS can reach it without a session cookie.
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
// up in the container logs.
func (h *healthHandler) check(c *gin.Context) {
	// The probe is unauthenticated, so raw error strings (paths, driver
	// internals) are logged for the operator but never sent to the client.
	if err := h.pingDB(); err != nil {
		log.Printf("health: database ping failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
			"db":     "error",
			"detail": "database unavailable (see container logs)",
		})
		return
	}

	if err := h.checkImagesDir(); err != nil {
		log.Printf("health: images volume check failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
			"db":     "ok",
			"images": "error",
			"detail": "images volume unavailable (see container logs)",
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
