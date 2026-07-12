package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/davidlc1229/omnishelf/internal/importer"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// Import-domain machine error codes for the standard envelope.
// CodeNotFound is shared (declared in books.go); CodeTMDBUnavailable in tv.go.
const (
	CodeInvalidImportFile = "invalid_import_file"
	CodeImportNotFinished = "import_not_finished"
	CodeJobsBusy          = "jobs_busy"
)

// maxUploadBytes caps the total size of an import upload (raw CSVs or zip).
const maxUploadBytes = 64 << 20 // 64 MiB

// RegisterImportRoutes attaches the TV Time import endpoints to
// the JWT-guarded /api group returned by RegisterRoutes. Wired from main by
// the orchestrator, which must also call importer.MarkInterrupted(db) at
// startup.
func RegisterImportRoutes(grp *gin.RouterGroup, imp *importer.Importer) {
	h := &importHandler{imp: imp}
	grp.POST("/tv/import", h.create)
	grp.GET("/tv/import/:jobId", h.status)
	grp.POST("/tv/import/:jobId/resolve", h.resolve)

	// Goodreads "My Review" → book notes. Progress is polled through the same
	// owner-scoped job-status endpoint above (jobs are not TV-specific).
	grp.POST("/books/notes/import", h.createNotes)
}

type importHandler struct {
	imp *importer.Importer
}

// jobDTO is the progress-polling payload (plus the
// malformed-row skip count).
type jobDTO struct {
	JobID     uint   `json:"jobId"`
	Status    string `json:"status"`
	Processed int    `json:"processed"`
	Total     int    `json:"total"`
	Skipped   int    `json:"skipped"`
	// NotesCreated is the number of book notes written by a Goodreads-notes
	// import (0 for TV/library jobs).
	NotesCreated int      `json:"notesCreated"`
	Current      string   `json:"current,omitempty"` // title being imported right now
	Unresolved   []string `json:"unresolved"`
	Error        string   `json:"error,omitempty"`
}

func toJobDTO(job *models.ImportJob, skipped, notesCreated int, current string, unresolved []string) jobDTO {
	return jobDTO{
		JobID:        job.ID,
		Status:       job.Status,
		Processed:    job.Processed,
		Total:        job.Total,
		Skipped:      skipped,
		NotesCreated: notesCreated,
		Current:      current,
		Unresolved:   unresolved,
		Error:        job.Error,
	}
}

// create handles POST /api/tv/import: multipart upload of followed_shows.csv
// and/or seen_episodes.csv, raw or zipped. Header rows are validated before
// any job is created (400, spec E9); on success a PENDING ImportJob is
// created and {jobId} returned immediately.
func (h *importHandler) create(c *gin.Context) {
	files, ok := readUploadFiles(c)
	if !ok {
		return
	}
	payload, err := importer.ParseUpload(files)
	h.startFrom(c, payload, err)
}

// createNotes handles POST /api/books/notes/import: multipart upload of a
// Goodreads library export whose "My Review" column is imported as a book note
// on each matching tracked book. It mirrors create's upload/parse/report
// pattern; only the parser differs (notes mode).
func (h *importHandler) createNotes(c *gin.Context) {
	files, ok := readUploadFiles(c)
	if !ok {
		return
	}
	payload, err := importer.ParseNotesUpload(files)
	h.startFrom(c, payload, err)
}

// startFrom finishes a create/createNotes request: it maps parse errors to the
// standard envelope, launches the background job, and returns {jobId}.
func (h *importHandler) startFrom(c *gin.Context, payload *importer.Payload, parseErr error) {
	if parseErr != nil {
		var ve *importer.ValidationError
		if errors.As(parseErr, &ve) {
			Error(c, http.StatusBadRequest, CodeInvalidImportFile, ve.Error())
			return
		}
		Error(c, http.StatusInternalServerError, CodeInternal, "failed to parse upload")
		return
	}

	job, err := h.imp.StartImport(CurrentUserID(c), payload)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "failed to create import job")
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"jobId": job.ID})
}

// readUploadFiles reads the multipart "files" of an import upload, enforcing
// the 64 MiB total cap. It writes a 400 envelope and returns ok=false on any
// problem so the caller can just return.
func readUploadFiles(c *gin.Context) ([]importer.UploadFile, bool) {
	form, err := c.MultipartForm()
	if err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "expected a multipart file upload")
		return nil, false
	}

	var files []importer.UploadFile
	var total int64
	for _, headers := range form.File {
		for _, fh := range headers {
			total += fh.Size
			if total > maxUploadBytes {
				Error(c, http.StatusBadRequest, CodeInvalidImportFile, "upload exceeds the 64 MiB limit")
				return nil, false
			}
			src, err := fh.Open()
			if err != nil {
				Error(c, http.StatusBadRequest, CodeInvalidImportFile, fmt.Sprintf("cannot read uploaded file %s", fh.Filename))
				return nil, false
			}
			data, err := io.ReadAll(io.LimitReader(src, maxUploadBytes+1))
			if closeErr := src.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
			if err != nil {
				Error(c, http.StatusBadRequest, CodeInvalidImportFile, fmt.Sprintf("cannot read uploaded file %s", fh.Filename))
				return nil, false
			}
			files = append(files, importer.UploadFile{Name: fh.Filename, Data: data})
		}
	}
	if len(files) == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "no files uploaded")
		return nil, false
	}
	return files, true
}

// status handles GET /api/tv/import/:jobId (owner-scoped: other users' jobs
// return 404).
func (h *importHandler) status(c *gin.Context) {
	jobID, ok := parseJobID(c)
	if !ok {
		return
	}
	job, skipped, unresolved, err := h.imp.JobStatus(jobID, CurrentUserID(c))
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, toJobDTO(job, skipped, h.imp.NotesCreated(jobID), h.imp.CurrentItem(jobID), unresolved))
}

// resolveRequest maps unresolved titles to TMDB show IDs.
type resolveRequest struct {
	Mappings map[string]int `json:"mappings"`
}

// resolve handles POST /api/tv/import/:jobId/resolve.
func (h *importHandler) resolve(c *gin.Context) {
	jobID, ok := parseJobID(c)
	if !ok {
		return
	}
	var req resolveRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Mappings) == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, `expected {"mappings": {"<title>": <tmdbId>, ...}}`)
		return
	}
	for title, id := range req.Mappings {
		if id <= 0 {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("invalid tmdbId for %q", title))
			return
		}
	}

	userID := CurrentUserID(c)
	job, err := h.imp.Resolve(jobID, userID, req.Mappings)
	if err != nil {
		writeImportError(c, err)
		return
	}
	_, skipped, unresolved, err := h.imp.JobStatus(job.ID, userID)
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, toJobDTO(job, skipped, h.imp.NotesCreated(job.ID), "", unresolved))
}

func parseJobID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("jobId"), 10, 64)
	if err != nil || id == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid job id")
		return 0, false
	}
	return uint(id), true
}

// writeImportError maps importer sentinel errors onto the standard envelope.
func writeImportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, importer.ErrJobNotFound):
		Error(c, http.StatusNotFound, CodeNotFound, "import job not found")
	case errors.Is(err, importer.ErrJobNotFinished):
		Error(c, http.StatusConflict, CodeImportNotFinished, "import job is still processing; retry once it finishes")
	case errors.Is(err, importer.ErrBusy):
		c.Header("Retry-After", "10")
		Error(c, http.StatusServiceUnavailable, CodeJobsBusy, "a sync or import is running; retry shortly")
	case errors.Is(err, importer.ErrTMDB):
		Error(c, http.StatusBadGateway, CodeTMDBUnavailable, "TMDB unreachable, try again")
	default:
		Error(c, http.StatusInternalServerError, CodeInternal, "import operation failed")
	}
}
