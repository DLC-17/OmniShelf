package importer_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/api"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/importer"
	"github.com/davidlc1229/omnishelf/internal/jobs"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// ── fixtures ──

// newTMDBServer serves the TMDB endpoints the importer hits. "Breaking Bad"
// (1396, 1 season × 3 episodes) is searchable; "The Wire" (999, 1 season ×
// 2 episodes) is NOT searchable — it only resolves via a manual mapping.
func newTMDBServer(t *testing.T) *httptest.Server {
	t.Helper()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(v))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/search/tv", func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(r.URL.Query().Get("query"))
		resp := tmdb.SearchResponse{Page: 1}
		if strings.Contains(q, "breaking") || strings.Contains(q, "braking") {
			resp.Results = []tmdb.SearchResult{{ID: 1396, Name: "Breaking Bad"}}
			resp.TotalResults = 1
		}
		writeJSON(w, resp)
	})
	mux.HandleFunc("/tv/1396", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, tmdb.Show{
			ID: 1396, Name: "Breaking Bad", Status: "Ended",
			Seasons: []tmdb.SeasonSummary{{SeasonNumber: 1, EpisodeCount: 3}},
		})
	})
	mux.HandleFunc("/tv/1396/season/1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, tmdb.Season{
			SeasonNumber: 1,
			Episodes: []tmdb.Episode{
				{SeasonNumber: 1, EpisodeNumber: 1, Name: "Pilot", AirDate: "2008-01-20"},
				{SeasonNumber: 1, EpisodeNumber: 2, Name: "Cat's in the Bag...", AirDate: "2008-01-27"},
				{SeasonNumber: 1, EpisodeNumber: 3, Name: "...And the Bag's in the River", AirDate: "2008-02-10"},
			},
		})
	})
	mux.HandleFunc("/tv/999", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, tmdb.Show{
			ID: 999, Name: "The Wire", Status: "Ended",
			Seasons: []tmdb.SeasonSummary{{SeasonNumber: 1, EpisodeCount: 2}},
		})
	})
	mux.HandleFunc("/tv/999/season/1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, tmdb.Season{
			SeasonNumber: 1,
			Episodes: []tmdb.Episode{
				{SeasonNumber: 1, EpisodeNumber: 1, Name: "The Target", AirDate: "2002-06-02"},
				{SeasonNumber: 1, EpisodeNumber: 2, Name: "The Detail", AirDate: "2002-06-09"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type env struct {
	router *gin.Engine
	db     *gorm.DB
	imp    *importer.Importer
}

// newEnv wires a real SQLite DB, a fixture TMDB client, the importer, and
// the import routes behind a stub auth middleware. The middleware stores the
// user ID under the same context key api.AuthRequired uses so
// api.CurrentUserID works; the user defaults to 1 and can be overridden per
// request with the X-Test-User header.
func newEnv(t *testing.T, opts ...func(*importer.Config)) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)

	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		// Close the pool so Windows can delete the temp DB file.
		if sqlDB, err := gdb.DB(); err == nil {
			sqlDB.Close()
		}
	})

	client := tmdb.New("test-key",
		tmdb.WithBaseURL(newTMDBServer(t).URL),
		tmdb.WithRateLimit(rate.NewLimiter(rate.Inf, 1)),
	)

	cfg := importer.Config{
		DB:        gdb,
		TMDB:      client,
		LockRetry: 5 * time.Millisecond,
		LockWait:  2 * time.Second,
	}
	for _, o := range opts {
		o(&cfg)
	}
	imp := importer.New(cfg)

	r := gin.New()
	grp := r.Group("/api", func(c *gin.Context) {
		uid := uint(1)
		if h := c.GetHeader("X-Test-User"); h != "" {
			n, err := strconv.Atoi(h)
			require.NoError(t, err)
			uid = uint(n)
		}
		c.Set("omnishelf_user_id", uid) // key from api.AuthRequired
	})
	api.RegisterImportRoutes(grp, imp)
	return &env{router: r, db: gdb, imp: imp}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

// multipartBody builds a multipart form with the given files under field
// name "files".
func multipartBody(t *testing.T, files map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for name, data := range files {
		fw, err := w.CreateFormFile("files", name)
		require.NoError(t, err)
		_, err = fw.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return body, w.FormDataContentType()
}

func (e *env) do(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e *env) upload(t *testing.T, files map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	body, ctype := multipartBody(t, files)
	req := httptest.NewRequest(http.MethodPost, "/api/tv/import", body)
	req.Header.Set("Content-Type", ctype)
	return e.do(t, req)
}

// startImport uploads and returns the accepted job ID.
func (e *env) startImport(t *testing.T, files map[string][]byte) uint {
	t.Helper()
	rec := e.upload(t, files)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var resp struct {
		JobID uint `json:"jobId"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotZero(t, resp.JobID)
	return resp.JobID
}

type jobResponse struct {
	JobID        uint     `json:"jobId"`
	Status       string   `json:"status"`
	Processed    int      `json:"processed"`
	Total        int      `json:"total"`
	Skipped      int      `json:"skipped"`
	NotesCreated int      `json:"notesCreated"`
	Unresolved   []string `json:"unresolved"`
	Error        string   `json:"error"`
}

func (e *env) getStatus(t *testing.T, jobID uint) jobResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tv/import/%d", jobID), nil)
	rec := e.do(t, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var jr jobResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jr))
	return jr
}

// waitForJob polls until the job leaves PENDING/RUNNING.
func (e *env) waitForJob(t *testing.T, jobID uint) jobResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		jr := e.getStatus(t, jobID)
		if jr.Status == importer.StatusDone || jr.Status == importer.StatusFailed {
			return jr
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %d did not finish: %+v", jobID, jr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func bothCSVs(t *testing.T) map[string][]byte {
	return map[string][]byte{
		"followed_shows.csv": fixture(t, "followed_shows.csv"),
		"seen_episodes.csv":  fixture(t, "seen_episodes.csv"),
	}
}

func count[T any](t *testing.T, gdb *gorm.DB, where ...any) int64 {
	t.Helper()
	var n int64
	q := gdb.Model(new(T))
	if len(where) > 0 {
		q = q.Where(where[0], where[1:]...)
	}
	require.NoError(t, q.Count(&n).Error)
	return n
}

// ── tests ──

// Fixture expectations: 8 total rows; 3 skipped (empty title, non-numeric
// season, episode S1E99 that TMDB doesn't have); "The Wire" unresolved with
// one pending watch; 2 Breaking Bad watches imported.
func TestImportHappyPathThenResolve(t *testing.T) {
	e := newEnv(t)
	jobID := e.startImport(t, bothCSVs(t))
	jr := e.waitForJob(t, jobID)

	assert.Equal(t, importer.StatusDone, jr.Status)
	assert.Equal(t, 8, jr.Total)
	assert.Equal(t, 8, jr.Processed)
	assert.Equal(t, 3, jr.Skipped)
	assert.Equal(t, []string{"The Wire"}, jr.Unresolved)

	assert.EqualValues(t, 1, count[models.Show](t, e.db))
	assert.EqualValues(t, 3, count[models.Episode](t, e.db))
	assert.EqualValues(t, 2, count[models.EpisodeWatch](t, e.db, "user_id = ?", 1))
	assert.EqualValues(t, 1, count[models.TrackingItem](t, e.db, "user_id = ? AND external_id = ?", 1, "1396"))

	// WatchedAt taken from the CSV timestamp.
	var watch models.EpisodeWatch
	require.NoError(t, e.db.Order("id").First(&watch).Error)
	assert.Equal(t, 2020, watch.WatchedAt.Year())

	// Manual resolution: The Wire → TMDB 999 imports the show, the tracking
	// item, and the held seen-episode row, and clears the unresolved bucket.
	body, err := json.Marshal(map[string]any{"mappings": map[string]int{"The Wire": 999}})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tv/import/%d/resolve", jobID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := e.do(t, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resolved jobResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resolved))
	assert.Empty(t, resolved.Unresolved)

	assert.EqualValues(t, 2, count[models.Show](t, e.db))
	assert.EqualValues(t, 3, count[models.EpisodeWatch](t, e.db, "user_id = ?", 1))
	assert.EqualValues(t, 1, count[models.TrackingItem](t, e.db, "user_id = ? AND external_id = ?", 1, "999"))
}

func TestZipUpload(t *testing.T) {
	e := newEnv(t)

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for name, data := range bothCSVs(t) {
		fw, err := zw.Create(name)
		require.NoError(t, err)
		_, err = fw.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	jobID := e.startImport(t, map[string][]byte{"tvtime_export.zip": buf.Bytes()})
	jr := e.waitForJob(t, jobID)
	assert.Equal(t, importer.StatusDone, jr.Status)
	assert.Equal(t, 8, jr.Total)
	assert.EqualValues(t, 2, count[models.EpisodeWatch](t, e.db))
}

func TestWrongHeaderRejectedBeforeJobCreation(t *testing.T) {
	e := newEnv(t)
	rec := e.upload(t, map[string][]byte{"wrong_header.csv": fixture(t, "wrong_header.csv")})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var envelope struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Equal(t, "invalid_import_file", envelope.Error)
	assert.NotEmpty(t, envelope.Message)
	assert.EqualValues(t, 0, count[models.ImportJob](t, e.db), "no job row created (E9)")
}

func TestNonCSVUploadRejected(t *testing.T) {
	e := newEnv(t)
	rec := e.upload(t, map[string][]byte{"notes.txt": []byte("this is not a csv at all\x00\x01")})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.EqualValues(t, 0, count[models.ImportJob](t, e.db))
}

func TestReimportCreatesNoDuplicateWatches(t *testing.T) {
	e := newEnv(t)

	first := e.waitForJob(t, e.startImport(t, bothCSVs(t)))
	require.Equal(t, importer.StatusDone, first.Status)
	require.EqualValues(t, 2, count[models.EpisodeWatch](t, e.db))

	second := e.waitForJob(t, e.startImport(t, bothCSVs(t)))
	assert.Equal(t, importer.StatusDone, second.Status)
	assert.EqualValues(t, 2, count[models.EpisodeWatch](t, e.db), "re-import is idempotent (E10)")
	assert.EqualValues(t, 3, count[models.Episode](t, e.db), "episodes upserted, not duplicated")
	assert.EqualValues(t, 1, count[models.TrackingItem](t, e.db, "external_id = ?", "1396"))
}

func TestMarkInterruptedFlipsRunningJobs(t *testing.T) {
	e := newEnv(t)
	running := &models.ImportJob{UserID: 1, Status: importer.StatusRunning, Total: 10}
	done := &models.ImportJob{UserID: 1, Status: importer.StatusDone, Total: 5, Processed: 5}
	require.NoError(t, e.db.Create(running).Error)
	require.NoError(t, e.db.Create(done).Error)

	require.NoError(t, importer.MarkInterrupted(e.db))

	var interrupted, finished models.ImportJob
	require.NoError(t, e.db.First(&interrupted, running.ID).Error)
	assert.Equal(t, importer.StatusFailed, interrupted.Status)
	assert.Equal(t, "interrupted", interrupted.Error)

	require.NoError(t, e.db.First(&finished, done.ID).Error)
	assert.Equal(t, importer.StatusDone, finished.Status, "finished jobs untouched")
}

func TestJobIsOwnerScoped(t *testing.T) {
	e := newEnv(t)
	jobID := e.startImport(t, bothCSVs(t))
	e.waitForJob(t, jobID)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tv/import/%d", jobID), nil)
	req.Header.Set("X-Test-User", "2")
	rec := e.do(t, req)
	assert.Equal(t, http.StatusNotFound, rec.Code, "other users' jobs are invisible")
}

// While the shared background-job lock is held by the sync
// engine, an import stays PENDING and retries; once the lock frees, it runs.
func TestImportWaitsForBusyLockThenRuns(t *testing.T) {
	e := newEnv(t)
	require.True(t, jobs.TryLock(), "test acquires the shared lock")
	released := false
	defer func() {
		if !released {
			jobs.Unlock()
		}
	}()

	jobID := e.startImport(t, bothCSVs(t))
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, importer.StatusPending, e.getStatus(t, jobID).Status, "job waits while lock is busy")

	jobs.Unlock()
	released = true
	jr := e.waitForJob(t, jobID)
	assert.Equal(t, importer.StatusDone, jr.Status)
}

// If the lock never frees within LockWait, the job fails gracefully instead
// of hanging forever.
func TestImportFailsGracefullyOnLockTimeout(t *testing.T) {
	e := newEnv(t, func(cfg *importer.Config) { cfg.LockWait = 30 * time.Millisecond })
	require.True(t, jobs.TryLock())
	defer jobs.Unlock()

	jobID := e.startImport(t, bothCSVs(t))
	jr := e.waitForJob(t, jobID)
	assert.Equal(t, importer.StatusFailed, jr.Status)
	assert.Contains(t, jr.Error, "lock")
}

// Resolve is rejected with 503 while the shared lock is held (E18) and with
// 409 while the job is still processing.
func TestResolveRespectsSharedLock(t *testing.T) {
	e := newEnv(t)
	jobID := e.startImport(t, bothCSVs(t))
	e.waitForJob(t, jobID)

	require.True(t, jobs.TryLock())
	defer jobs.Unlock()

	body, err := json.Marshal(map[string]any{"mappings": map[string]int{"The Wire": 999}})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/tv/import/%d/resolve", jobID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := e.do(t, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "10", rec.Header().Get("Retry-After"))
}

// TestGoodreadsBookImport imports a Goodreads library export: ISBN-13 rows
// become Book + BOOK tracking rows with the shelf mapped to a status, and a
// row without a usable ISBN-13 is skipped and counted.
func TestGoodreadsBookImport(t *testing.T) {
	e := newEnv(t)
	jobID := e.startImport(t, map[string][]byte{
		"goodreads_library_export.csv": fixture(t, "goodreads_library_export.csv"),
	})
	jr := e.waitForJob(t, jobID)

	assert.Equal(t, importer.StatusDone, jr.Status)
	assert.Equal(t, 4, jr.Total)
	assert.Equal(t, 4, jr.Processed)
	assert.Equal(t, 1, jr.Skipped, "the ISBN-less row is skipped")

	assert.EqualValues(t, 3, count[models.Book](t, e.db))
	assert.EqualValues(t, 3, count[models.TrackingItem](t, e.db, "user_id = ? AND type = ?", 1, "BOOK"))

	dune := bookItem(t, e.db, "9780441013593")
	assert.Equal(t, "Dune", dune.Title)
	assert.Equal(t, "COMPLETED", dune.Status)
	assert.Equal(t, 412, dune.Progress, "a finished book seeds progress to its page count")

	assert.Equal(t, "READING", bookItem(t, e.db, "9780547928227").Status)
	assert.Equal(t, "PLAN_TO", bookItem(t, e.db, "9780441569595").Status)

	var book models.Book
	require.NoError(t, e.db.Where("isbn13 = ?", "9780441013593").First(&book).Error)
	assert.Equal(t, "Frank Herbert", book.Authors)
	assert.Equal(t, 412, book.PageCount)
}

// TestGoodreadsReimportIsIdempotent guards the book upsert path.
func TestGoodreadsReimportIsIdempotent(t *testing.T) {
	e := newEnv(t)
	files := map[string][]byte{"goodreads_library_export.csv": fixture(t, "goodreads_library_export.csv")}
	e.waitForJob(t, e.startImport(t, files))
	e.waitForJob(t, e.startImport(t, files))

	assert.EqualValues(t, 3, count[models.Book](t, e.db), "re-import must not duplicate books")
	assert.EqualValues(t, 3, count[models.TrackingItem](t, e.db, "type = ?", "BOOK"))
}

func bookItem(t *testing.T, gdb *gorm.DB, isbn13 string) models.TrackingItem {
	t.Helper()
	var item models.TrackingItem
	require.NoError(t, gdb.Where("type = ? AND external_id = ?", "BOOK", isbn13).First(&item).Error)
	return item
}

// ── Goodreads notes import ──

// trackBook seeds a shared Book row plus a BOOK TrackingItem for the user, as
// a prior book scan would, so the notes import has something to match against.
func trackBook(t *testing.T, gdb *gorm.DB, userID uint, isbn13, title, authors string) models.TrackingItem {
	t.Helper()
	require.NoError(t, gdb.Create(&models.Book{ISBN13: isbn13, Title: title, Authors: authors}).Error)
	item := models.TrackingItem{UserID: userID, Type: "BOOK", ExternalID: isbn13, Title: title, Status: "READING"}
	require.NoError(t, gdb.Create(&item).Error)
	return item
}

// uploadNotes posts a Goodreads export to the notes-import endpoint.
func (e *env) uploadNotes(t *testing.T, files map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	body, ctype := multipartBody(t, files)
	req := httptest.NewRequest(http.MethodPost, "/api/books/notes/import", body)
	req.Header.Set("Content-Type", ctype)
	return e.do(t, req)
}

// startNotesImport uploads to the notes endpoint and returns the accepted job ID.
func (e *env) startNotesImport(t *testing.T, files map[string][]byte) uint {
	t.Helper()
	rec := e.uploadNotes(t, files)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var resp struct {
		JobID uint `json:"jobId"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotZero(t, resp.JobID)
	return resp.JobID
}

func notesCSV(t *testing.T) map[string][]byte {
	return map[string][]byte{"goodreads_notes_export.csv": fixture(t, "goodreads_notes_export.csv")}
}

// TestGoodreadsNotesImport imports Goodreads "My Review" text as book notes.
// The fixture exercises every outcome the report distinguishes: an ISBN-13
// match (Dune), a title+author fallback match on an ISBN-less row (The Hobbit),
// a reviewed-but-untracked book (Neuromancer → unmatched), and a row with no
// review (the second Dune row → skipped).
func TestGoodreadsNotesImport(t *testing.T) {
	e := newEnv(t)
	dune := trackBook(t, e.db, 1, "9780441013593", "Dune", "Frank Herbert")
	hobbit := trackBook(t, e.db, 1, "9780547928227", "The Hobbit", "J.R.R. Tolkien")

	jr := e.waitForJob(t, e.startNotesImport(t, notesCSV(t)))

	assert.Equal(t, importer.StatusDone, jr.Status)
	assert.Equal(t, 4, jr.Total)
	assert.Equal(t, 4, jr.Processed)
	assert.Equal(t, 2, jr.NotesCreated, "Dune (ISBN) + The Hobbit (title/author)")
	assert.Equal(t, 1, jr.Skipped, "the review-less Dune row")
	assert.Equal(t, []string{"Neuromancer"}, jr.Unresolved, "reviewed but not tracked")

	assert.EqualValues(t, 2, count[models.BookNote](t, e.db, "user_id = ?", 1))

	// The ISBN match backdates the note to Date Read (2019/05/01).
	var duneNote models.BookNote
	require.NoError(t, e.db.Where("item_id = ?", dune.ID).First(&duneNote).Error)
	assert.Equal(t, "A landmark of science fiction.", duneNote.Body)
	assert.Equal(t, 2019, duneNote.CreatedAt.Year())

	// The Hobbit row has no Date Read, so it backdates to Date Added (2020/03/03).
	var hobbitNote models.BookNote
	require.NoError(t, e.db.Where("item_id = ?", hobbit.ID).First(&hobbitNote).Error)
	assert.Equal(t, 2020, hobbitNote.CreatedAt.Year())
}

// TestGoodreadsNotesReimportIsIdempotent proves an identical review re-imported
// creates no duplicate note.
func TestGoodreadsNotesReimportIsIdempotent(t *testing.T) {
	e := newEnv(t)
	trackBook(t, e.db, 1, "9780441013593", "Dune", "Frank Herbert")
	trackBook(t, e.db, 1, "9780547928227", "The Hobbit", "J.R.R. Tolkien")

	first := e.waitForJob(t, e.startNotesImport(t, notesCSV(t)))
	require.Equal(t, 2, first.NotesCreated)

	second := e.waitForJob(t, e.startNotesImport(t, notesCSV(t)))
	assert.Equal(t, importer.StatusDone, second.Status)
	assert.Equal(t, 0, second.NotesCreated, "identical reviews are deduped")
	assert.EqualValues(t, 2, count[models.BookNote](t, e.db, "user_id = ?", 1), "no duplicate notes")
}

// TestNotesImportRejectsNonGoodreadsFile guards the notes endpoint's up-front
// validation: a TV Time export has no ISBN/My Review columns and is rejected
// with 400 before any job is created.
func TestNotesImportRejectsNonGoodreadsFile(t *testing.T) {
	e := newEnv(t)
	rec := e.uploadNotes(t, map[string][]byte{"followed_shows.csv": fixture(t, "followed_shows.csv")})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.EqualValues(t, 0, count[models.ImportJob](t, e.db), "no job row created")
}

// TestNotesImportOnlyMatchesOwnBooks confirms matching is user-scoped: a book
// tracked by another user is not a match, so its review is reported unmatched.
func TestNotesImportOnlyMatchesOwnBooks(t *testing.T) {
	e := newEnv(t)
	trackBook(t, e.db, 2, "9780441013593", "Dune", "Frank Herbert") // other user

	jr := e.waitForJob(t, e.startNotesImport(t, notesCSV(t)))
	assert.Equal(t, importer.StatusDone, jr.Status)
	assert.Equal(t, 0, jr.NotesCreated)
	assert.EqualValues(t, 0, count[models.BookNote](t, e.db))
	assert.Contains(t, jr.Unresolved, "Dune", "another user's book is not matched")
}

// TestSeenExportAddsSeriesAndWatches verifies a standalone TV Time "seen
// episodes" export (series_name + season_number/episode_number, with the
// s_no/ep_no columns present but empty) adds the series to the user's
// watchlist and marks the episodes watched — no separate followed file needed.
func TestSeenExportAddsSeriesAndWatches(t *testing.T) {
	e := newEnv(t)
	jobID := e.startImport(t, map[string][]byte{
		"tvtime_seen_export.csv": fixture(t, "tvtime_seen_export.csv"),
	})
	jr := e.waitForJob(t, jobID)

	assert.Equal(t, importer.StatusDone, jr.Status)
	assert.Equal(t, 2, jr.Total)
	assert.Equal(t, 2, jr.Processed)
	assert.Equal(t, 0, jr.Skipped)
	assert.Empty(t, jr.Unresolved, "Breaking Bad resolves on TMDB")

	// The series was added to the watchlist even though there was no followed file.
	assert.EqualValues(t, 1, count[models.TrackingItem](t, e.db, "user_id = ? AND type = ? AND external_id = ?", 1, "TV", "1396"))
	// Both episodes are marked watched, timestamped from created_at.
	assert.EqualValues(t, 2, count[models.EpisodeWatch](t, e.db, "user_id = ?", 1))
	var watch models.EpisodeWatch
	require.NoError(t, e.db.Order("id").First(&watch).Error)
	assert.Equal(t, 2008, watch.WatchedAt.Year())
}

// TestImportIsDBFirstViaAlias proves the pipeline is DB-first: with a cached
// show, its episodes, and a title alias already present, an import resolves the
// series and marks watches even though every TMDB call fails.
func TestImportIsDBFirstViaAlias(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(broken.Close)

	e := newEnv(t, func(c *importer.Config) {
		c.TMDB = tmdb.New("test-key",
			tmdb.WithBaseURL(broken.URL),
			tmdb.WithRateLimit(rate.NewLimiter(rate.Inf, 1)))
	})

	// Pre-seed the local cache and the title alias (as a prior import would).
	show := models.Show{TMDBID: 1396, Title: "Breaking Bad", Status: "Ended"}
	require.NoError(t, e.db.Create(&show).Error)
	require.NoError(t, e.db.Create(&models.Episode{ShowID: show.ID, Season: 1, Number: 1, Title: "Pilot"}).Error)
	require.NoError(t, e.db.Create(&models.Episode{ShowID: show.ID, Season: 1, Number: 2, Title: "Cat's in the Bag..."}).Error)
	require.NoError(t, e.db.Create(&models.ShowAlias{NormTitle: "breaking bad", TMDBID: 1396}).Error)

	jobID := e.startImport(t, map[string][]byte{
		"tvtime_seen_export.csv": fixture(t, "tvtime_seen_export.csv"),
	})
	jr := e.waitForJob(t, jobID)

	assert.Equal(t, importer.StatusDone, jr.Status)
	assert.Equal(t, 0, jr.Skipped)
	assert.Empty(t, jr.Unresolved, "alias + cache resolve the series without TMDB")
	assert.EqualValues(t, 1, count[models.Show](t, e.db), "no duplicate show")
	assert.EqualValues(t, 1, count[models.TrackingItem](t, e.db, "type = ? AND external_id = ?", "TV", "1396"))
	assert.EqualValues(t, 2, count[models.EpisodeWatch](t, e.db, "user_id = ?", 1))
}

// TestImportSavesAliasForReuse verifies a first import records the title→TMDB
// alias so it is available to later imports.
func TestImportSavesAliasForReuse(t *testing.T) {
	e := newEnv(t)
	e.waitForJob(t, e.startImport(t, map[string][]byte{
		"tvtime_seen_export.csv": fixture(t, "tvtime_seen_export.csv"),
	}))

	var alias models.ShowAlias
	require.NoError(t, e.db.Where("norm_title = ?", "breaking bad").First(&alias).Error)
	assert.Equal(t, 1396, alias.TMDBID)
}
