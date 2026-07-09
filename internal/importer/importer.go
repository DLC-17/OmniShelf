// Package importer implements the TV Time CSV import pipeline.
//
// Uploads are header-validated up front (400 pre-job, E9), then processed by
// a background goroutine in chunks of 50 rows with progress persisted to the
// ImportJob row between chunks. Titles that cannot be matched on TMDB land
// in an UNRESOLVED bucket for manual mapping (E8). Re-imports are idempotent:
// shows, episodes, tracking items and episode watches are all upserted
// against their unique indexes. Jobs left RUNNING by a restart are marked
// FAILED "interrupted" at startup via MarkInterrupted (E10).
package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/images"
	"github.com/davidlc1229/omnishelf/internal/jobs"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// ImportJob statuses (models.ImportJob.Status).
const (
	StatusPending = "PENDING"
	StatusRunning = "RUNNING"
	StatusDone    = "DONE"
	StatusFailed  = "FAILED"
)

// chunkSize is the number of CSV rows processed between progress writes
// (chunks of ~50 rows so restarts are diagnosable).
const chunkSize = 50

// defaultImageBaseURL is the TMDB poster CDN prefix.
const defaultImageBaseURL = "https://image.tmdb.org/t/p/w500"

// Sentinel errors surfaced by Resolve/JobStatus; the API layer maps them to
// HTTP statuses.
var (
	ErrJobNotFound    = errors.New("import job not found")
	ErrJobNotFinished = errors.New("import job still processing")
	ErrBusy           = errors.New("another background job is running")
	ErrTMDB           = errors.New("tmdb lookup failed")
)

// Config configures an Importer. DB and TMDB are required; Images is
// optional (posters are skipped when nil, and poster failures never fail an
// import).
type Config struct {
	DB     *gorm.DB
	TMDB   *tmdb.Client
	Images *images.Store

	// ImageBaseURL prefixes TMDB poster paths; defaults to the w500 CDN.
	ImageBaseURL string
	// HTTPClient is used for poster downloads; defaults to a 15s-timeout client.
	HTTPClient *http.Client

	// LockRetry / LockWait tune how a PENDING job waits for the shared
	// background-job lock. Defaults: retry every 2s, give up
	// after 15 minutes and mark the job FAILED.
	LockRetry time.Duration
	LockWait  time.Duration
}

// Importer owns TV Time import jobs.
//
// Two pieces of per-job state are held in memory only, because the fixed
// ImportJob schema has no column for them: the malformed-row skip count and
// the seen-episode rows belonging to unresolved titles (replayed when the
// user resolves a title). After a restart the skip count reads 0 and
// resolving an interrupted job's title imports the show/tracking item but
// cannot replay its watches — the job is already FAILED "interrupted" (E10)
// and the documented recovery is an idempotent re-upload.
type Importer struct {
	db           *gorm.DB
	tmdb         *tmdb.Client
	images       *images.Store
	httpClient   *http.Client
	imageBaseURL string
	lockRetry    time.Duration
	lockWait     time.Duration

	mu           sync.Mutex
	skipped      map[uint]int
	notesCreated map[uint]int                       // jobID → book notes created (notes import)
	current      map[uint]string                    // jobID → title currently being imported
	pending      map[uint]map[string][]pendingWatch // jobID → normalized title → rows
}

// pendingWatch is a seen-episode row waiting for its title to be resolved.
type pendingWatch struct {
	Season    int
	Episode   int
	WatchedAt time.Time
}

// New returns an Importer, applying config defaults.
func New(cfg Config) *Importer {
	imp := &Importer{
		db:           cfg.DB,
		tmdb:         cfg.TMDB,
		images:       cfg.Images,
		httpClient:   cfg.HTTPClient,
		imageBaseURL: cfg.ImageBaseURL,
		lockRetry:    cfg.LockRetry,
		lockWait:     cfg.LockWait,
		skipped:      make(map[uint]int),
		notesCreated: make(map[uint]int),
		current:      make(map[uint]string),
		pending:      make(map[uint]map[string][]pendingWatch),
	}
	if imp.httpClient == nil {
		imp.httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	if imp.imageBaseURL == "" {
		imp.imageBaseURL = defaultImageBaseURL
	}
	if imp.lockRetry <= 0 {
		imp.lockRetry = 2 * time.Second
	}
	if imp.lockWait <= 0 {
		imp.lockWait = 15 * time.Minute
	}
	return imp
}

// MarkInterrupted flips jobs stuck RUNNING (a container restart killed their
// goroutine) to FAILED "interrupted". Call once at startup before
// serving traffic.
func MarkInterrupted(db *gorm.DB) error {
	res := db.Model(&models.ImportJob{}).
		Where("status = ?", StatusRunning).
		Updates(map[string]any{"status": StatusFailed, "error": "interrupted"})
	if res.Error != nil {
		return fmt.Errorf("importer: marking interrupted jobs: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		log.Printf("importer: marked %d interrupted import job(s) FAILED", res.RowsAffected)
	}
	return nil
}

// StartImport creates a PENDING ImportJob for the payload and launches the
// background goroutine that processes it. It returns immediately with the
// job so the handler can respond with {jobId}.
func (imp *Importer) StartImport(userID uint, p *Payload) (*models.ImportJob, error) {
	job := &models.ImportJob{
		UserID:     userID,
		Status:     StatusPending,
		Total:      p.TotalRows(),
		Unresolved: "[]",
	}
	if err := imp.db.Create(job).Error; err != nil {
		return nil, fmt.Errorf("importer: creating job: %w", err)
	}
	go imp.run(job.ID, userID, p)
	return job, nil
}

// JobStatus returns the job (owner-scoped: other users get ErrJobNotFound),
// its in-memory skipped-row count, and the unresolved titles.
func (imp *Importer) JobStatus(jobID, userID uint) (*models.ImportJob, int, []string, error) {
	job, err := imp.loadJob(jobID, userID)
	if err != nil {
		return nil, 0, nil, err
	}
	imp.mu.Lock()
	skipped := imp.skipped[jobID]
	imp.mu.Unlock()
	return job, skipped, decodeUnresolved(job.Unresolved), nil
}

// CurrentItem returns the title the job is importing right now (empty once the
// job is finished or between chunks).
func (imp *Importer) CurrentItem(jobID uint) string {
	imp.mu.Lock()
	defer imp.mu.Unlock()
	return imp.current[jobID]
}

// NotesCreated returns how many book notes a finished notes-import job wrote
// (0 for TV/library jobs). Like the skip count it lives in memory only, so it
// reads 0 after a restart.
func (imp *Importer) NotesCreated(jobID uint) int {
	imp.mu.Lock()
	defer imp.mu.Unlock()
	return imp.notesCreated[jobID]
}

func (imp *Importer) setCurrent(jobID uint, title string) {
	imp.mu.Lock()
	imp.current[jobID] = title
	imp.mu.Unlock()
}

// Resolve imports the shows the user manually mapped (title → TMDB ID),
// replays any seen-episode rows held for those titles, and removes them from
// the job's unresolved list.
//
// It takes the shared background-job lock non-blockingly; if the nightly
// sync or another import holds it, ErrBusy is returned and the client should
// retry.
func (imp *Importer) Resolve(jobID, userID uint, mappings map[string]int) (*models.ImportJob, error) {
	job, err := imp.loadJob(jobID, userID)
	if err != nil {
		return nil, err
	}
	if job.Status != StatusDone && job.Status != StatusFailed {
		return nil, ErrJobNotFinished
	}
	if !jobs.TryLock() {
		return nil, ErrBusy
	}
	defer jobs.Unlock()

	ctx := context.Background()
	unresolved := decodeUnresolved(job.Unresolved)

	for title, tmdbID := range mappings {
		norm := normalizeTitle(title)
		show, epIDs, err := imp.importShow(ctx, tmdbID)
		if err != nil {
			// Persist what already succeeded before surfacing the failure.
			imp.saveUnresolved(job, unresolved)
			return nil, fmt.Errorf("%w: %q → %d: %v", ErrTMDB, title, tmdbID, err)
		}
		// Remember the hand-mapped title so a future import resolves it
		// automatically.
		imp.saveAlias(norm, tmdbID)
		if err := imp.ensureTracking(userID, show); err != nil {
			imp.saveUnresolved(job, unresolved)
			return nil, err
		}

		imp.mu.Lock()
		rows := imp.pending[jobID][norm]
		delete(imp.pending[jobID], norm)
		imp.mu.Unlock()
		for _, w := range rows {
			if epID, ok := epIDs[epKey{w.Season, w.Episode}]; ok {
				if err := imp.upsertWatch(userID, epID, w.WatchedAt); err != nil {
					return nil, err
				}
			}
		}

		unresolved = removeTitle(unresolved, norm)
	}
	if err := imp.saveUnresolved(job, unresolved); err != nil {
		return nil, err
	}
	return job, nil
}

// ── background processing ──

// resolvedShow caches one imported show for the duration of a run.
type resolvedShow struct {
	show    *models.Show
	epIDs   map[epKey]uint
	tracked bool
}

// epKey addresses an episode within a show.
type epKey struct{ Season, Number int }

// runState accumulates per-run bookkeeping.
type runState struct {
	jobID      uint
	userID     uint
	cache      map[string]*resolvedShow  // normalized title → show (nil = unresolvable)
	unresolved []string                  // original titles, first-seen order
	pending    map[string][]pendingWatch // normalized title → held watch rows
	skipped    int
	processed  int
	// Notes import only: the caller's tracked books, indexed for matching, and
	// the running count of notes created.
	books        *bookIndex
	notesCreated int
}

// run processes an import job in a background goroutine, guarded by
// recover() and the shared job lock.
// If the lock is busy the job stays PENDING and acquisition is retried every
// lockRetry until lockWait elapses, after which the job fails gracefully —
// documented choice: bounded retry rather than unbounded waiting, so a wedged
// sync cannot strand a PENDING job forever.
func (imp *Importer) run(jobID, userID uint, p *Payload) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("importer: job %d panicked: %v", jobID, r)
			imp.failJob(jobID, fmt.Sprintf("internal error: %v", r))
		}
	}()

	deadline := time.Now().Add(imp.lockWait)
	for !jobs.TryLock() {
		if time.Now().After(deadline) {
			imp.failJob(jobID, "timed out waiting for the background job lock (a sync or import was running)")
			return
		}
		time.Sleep(imp.lockRetry)
	}
	defer jobs.Unlock()

	if err := imp.db.Model(&models.ImportJob{}).Where("id = ?", jobID).
		Update("status", StatusRunning).Error; err != nil {
		log.Printf("importer: job %d: setting RUNNING: %v", jobID, err)
		return
	}

	ctx := context.Background()
	st := &runState{
		jobID:   jobID,
		userID:  userID,
		cache:   make(map[string]*resolvedShow),
		pending: make(map[string][]pendingWatch),
	}
	// Clear the "currently importing" label once the run ends, however it ends.
	defer imp.setCurrent(jobID, "")

	for _, f := range p.files {
		for start := 0; start < len(f.records); start += chunkSize {
			end := start + chunkSize
			if end > len(f.records) {
				end = len(f.records)
			}
			for _, rec := range f.records[start:end] {
				if err := imp.processRow(ctx, &f, rec, st); err != nil {
					imp.failJob(jobID, err.Error())
					return
				}
			}
			st.processed += end - start
			imp.persistProgress(jobID, st)
		}
	}

	imp.mu.Lock()
	imp.skipped[jobID] = st.skipped
	imp.notesCreated[jobID] = st.notesCreated
	imp.pending[jobID] = st.pending
	imp.mu.Unlock()

	if err := imp.db.Model(&models.ImportJob{}).Where("id = ?", jobID).
		Updates(map[string]any{
			"status":     StatusDone,
			"processed":  st.processed,
			"unresolved": encodeUnresolved(st.unresolved),
		}).Error; err != nil {
		log.Printf("importer: job %d: finalizing: %v", jobID, err)
	}
}

// processRow handles one CSV data record. Malformed rows are skipped and
// counted (E9); titles TMDB cannot match go to the unresolved bucket, along
// with their watch rows (E8). Only database errors abort the job.
func (imp *Importer) processRow(ctx context.Context, f *parsedFile, rec []string, st *runState) error {
	title := fieldAt(rec, f.titleIdx)
	if title == "" {
		st.skipped++
		return nil
	}
	// Surface what is being imported right now for the progress UI.
	imp.setCurrent(st.jobID, title)

	switch f.kind {
	case kindGoodreads:
		return imp.importBookRow(f, rec, st)

	case kindGoodreadsNotes:
		return imp.importNoteRow(f, rec, st)

	case kindFollowed:
		rs, ok := imp.resolveTitle(ctx, title, st)
		if !ok {
			return nil // recorded unresolved
		}
		if !rs.tracked {
			if err := imp.ensureTracking(st.userID, rs.show); err != nil {
				return err
			}
			rs.tracked = true
		}

	case kindSeen:
		season, ok1 := firstIntField(rec, f.seasonIdxs)
		episode, ok2 := firstIntField(rec, f.episodeIdxs)
		if !ok1 || !ok2 || season < 0 || episode < 1 {
			st.skipped++
			return nil
		}
		watchedAt := parseWatchedAt(fieldAt(rec, f.watchedIdx))

		norm := normalizeTitle(title)
		rs, ok := imp.resolveTitle(ctx, title, st)
		if !ok {
			// Held in memory and replayed if the user resolves the title.
			st.pending[norm] = append(st.pending[norm], pendingWatch{season, episode, watchedAt})
			return nil
		}
		// A seen-episode row is enough to put the series on the user's shelf,
		// so a standalone seen export populates the library, not just watches.
		if !rs.tracked {
			if err := imp.ensureTracking(st.userID, rs.show); err != nil {
				return err
			}
			rs.tracked = true
		}
		epID, found := rs.epIDs[epKey{season, episode}]
		if !found {
			// Episode does not exist on TMDB (e.g., renumbered): skip+count.
			st.skipped++
			return nil
		}
		return imp.upsertWatch(st.userID, epID, watchedAt)
	}
	return nil
}

// resolveTitle matches a title against TMDB (exact, then fuzzy above
// threshold) and imports the show on first sight. TMDB errors and no-match
// results both record the title as unresolved rather than failing the job
// (E8) — a transient TMDB outage should not abort an otherwise-good import.
func (imp *Importer) resolveTitle(ctx context.Context, title string, st *runState) (*resolvedShow, bool) {
	norm := normalizeTitle(title)
	if rs, seen := st.cache[norm]; seen {
		return rs, rs != nil
	}

	markUnresolved := func() {
		st.cache[norm] = nil
		st.unresolved = append(st.unresolved, title)
	}

	// DB-first: a title resolved by a previous import maps straight to a TMDB
	// id, and importShow then serves it from the cache with no TMDB calls.
	if tmdbID, ok := imp.lookupAlias(norm); ok {
		if show, epIDs, err := imp.importShow(ctx, tmdbID); err == nil {
			rs := &resolvedShow{show: show, epIDs: epIDs}
			st.cache[norm] = rs
			return rs, true
		}
		// A stale alias (e.g. TMDB unreachable for an uncached show) falls
		// through to a fresh search below.
	}

	sr, err := imp.tmdb.SearchTV(ctx, title)
	if err != nil {
		log.Printf("importer: search %q: %v", title, err)
		markUnresolved()
		return nil, false
	}
	tmdbID := chooseMatch(title, sr.Results)
	if tmdbID == 0 {
		markUnresolved()
		return nil, false
	}
	show, epIDs, err := imp.importShow(ctx, tmdbID)
	if err != nil {
		log.Printf("importer: import show %q (%d): %v", title, tmdbID, err)
		markUnresolved()
		return nil, false
	}
	imp.saveAlias(norm, tmdbID) // remember for next time
	rs := &resolvedShow{show: show, epIDs: epIDs}
	st.cache[norm] = rs
	return rs, true
}

// importShow returns the Show and its episode-ID lookup keyed by (season,
// number). It is DB-first: a show already cached with its episodes is served
// from the database with no TMDB calls. Otherwise it fetches from TMDB and
// upserts against the unique indexes (shows.tmdb_id, episodes idx_show_ep) so
// re-imports and overlap with the sync engine's rows are safe.
func (imp *Importer) importShow(ctx context.Context, tmdbID int) (*models.Show, map[epKey]uint, error) {
	// DB-first: reuse the cached show + episodes when present.
	var cached models.Show
	err := imp.db.Where(&models.Show{TMDBID: tmdbID}).First(&cached).Error
	if err == nil {
		if epIDs, ok := imp.loadEpisodeIDs(cached.ID); ok {
			return &cached, epIDs, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, fmt.Errorf("cache lookup for show %d: %w", tmdbID, err)
	}

	detail, err := imp.tmdb.GetShow(ctx, tmdbID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching show %d: %w", tmdbID, err)
	}

	var show models.Show
	err = imp.db.Where(&models.Show{TMDBID: tmdbID}).
		Assign(map[string]any{
			"title":          detail.Name,
			"status":         detail.Status,
			"last_synced_at": time.Now(),
		}).
		FirstOrCreate(&show).Error
	if err != nil {
		return nil, nil, fmt.Errorf("upserting show %d: %w", tmdbID, err)
	}

	// Poster download is best-effort: failure leaves an empty path and the
	// nightly sync retries missing artwork (E13).
	if imp.images != nil && detail.PosterPath != "" && show.PosterPath == "" {
		rel, err := imp.images.Fetch(ctx, imp.httpClient, imp.imageBaseURL+detail.PosterPath, "tv", strconv.Itoa(tmdbID))
		if err != nil {
			log.Printf("importer: poster for show %d: %v", tmdbID, err)
		} else if err := imp.db.Model(&show).Update("poster_path", rel).Error; err != nil {
			return nil, nil, fmt.Errorf("saving poster path for show %d: %w", tmdbID, err)
		}
	}

	epIDs := make(map[epKey]uint)
	for _, s := range detail.Seasons {
		season, err := imp.tmdb.GetSeason(ctx, tmdbID, s.SeasonNumber)
		if err != nil {
			return nil, nil, fmt.Errorf("fetching show %d season %d: %w", tmdbID, s.SeasonNumber, err)
		}
		for _, e := range season.Episodes {
			var ep models.Episode
			err := imp.db.Where(&models.Episode{ShowID: show.ID, Season: e.SeasonNumber, Number: e.EpisodeNumber}).
				Assign(map[string]any{"title": e.Name, "air_date": parseAirDate(e.AirDate)}).
				FirstOrCreate(&ep).Error
			if err != nil {
				return nil, nil, fmt.Errorf("upserting show %d S%02dE%02d: %w", tmdbID, e.SeasonNumber, e.EpisodeNumber, err)
			}
			epIDs[epKey{e.SeasonNumber, e.EpisodeNumber}] = ep.ID
		}
	}
	return &show, epIDs, nil
}

// importBookRow imports one Goodreads library row: it upserts the shared Book
// (from the CSV's own title/author/pages — no network lookup, cover left for a
// later rescan) and the user's BOOK tracking item, mapping the Goodreads shelf
// to a status. Rows without a usable ISBN-13 are skipped and counted.
func (imp *Importer) importBookRow(f *parsedFile, rec []string, st *runState) error {
	isbn13 := cleanISBN(fieldAt(rec, f.isbn13Idx))
	if len(isbn13) != 13 {
		if alt := cleanISBN(fieldAt(rec, f.isbnIdx)); len(alt) == 13 {
			isbn13 = alt
		} else {
			st.skipped++ // older titles often have no ISBN-13
			return nil
		}
	}

	title := fieldAt(rec, f.titleIdx)
	author := fieldAt(rec, f.authorIdx)
	pages, _ := strconv.Atoi(fieldAt(rec, f.pagesIdx))
	if pages < 0 {
		pages = 0
	}
	status := mapShelfStatus(fieldAt(rec, f.shelfIdx))

	var book models.Book
	if err := imp.db.Where(&models.Book{ISBN13: isbn13}).
		Assign(map[string]any{"title": title, "authors": author, "page_count": pages}).
		FirstOrCreate(&book).Error; err != nil {
		return fmt.Errorf("upserting book %s: %w", isbn13, err)
	}

	progress := 0
	if status == "COMPLETED" && pages > 0 {
		progress = pages
	}
	item := models.TrackingItem{}
	if err := imp.db.Where(&models.TrackingItem{
		UserID: st.userID, Type: "BOOK", ExternalID: isbn13,
	}).Attrs(map[string]any{
		"title":    title,
		"status":   status,
		"progress": progress,
	}).FirstOrCreate(&item).Error; err != nil {
		return fmt.Errorf("upserting book tracking item %s: %w", isbn13, err)
	}
	return nil
}

// cleanISBN strips Goodreads' Excel armor (="…") plus hyphens and spaces,
// keeping digits and a trailing X (ISBN-10 check char).
func cleanISBN(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == 'X' || r == 'x':
			b.WriteRune('X')
		}
	}
	return b.String()
}

// mapShelfStatus maps a Goodreads "Exclusive Shelf" value to a tracking status.
func mapShelfStatus(shelf string) string {
	switch strings.ToLower(strings.TrimSpace(shelf)) {
	case "read":
		return "COMPLETED"
	case "currently-reading", "currently_reading", "reading":
		return "READING"
	case "to-read", "to_read", "want-to-read":
		return "PLAN_TO"
	default:
		return "READING"
	}
}

// importNoteRow maps one Goodreads row's My Review to a BookNote on the user's
// matching tracked BOOK item. Rows without a review are skipped and counted;
// reviewed rows whose book the user does not track land in the unresolved
// bucket (reported as skipped-unmatched). Importing new books is out of scope —
// this only annotates books already on the shelf.
func (imp *Importer) importNoteRow(f *parsedFile, rec []string, st *runState) error {
	review := fieldAt(rec, f.reviewIdx)
	if review == "" {
		st.skipped++ // rows without a review produce no note
		return nil
	}

	if st.books == nil {
		idx, err := imp.loadBookIndex(st.userID)
		if err != nil {
			return err
		}
		st.books = idx
	}

	item, ok := st.books.match(f, rec)
	if !ok {
		// The reviewed book is not tracked; surface its title so the user can
		// see which reviews were not imported.
		st.unresolved = append(st.unresolved, fieldAt(rec, f.titleIdx))
		return nil
	}

	// Backdate to Date Read, then Date Added, then fall back to now.
	createdAt := parseGoodreadsDate(fieldAt(rec, f.dateReadIdx))
	if createdAt.IsZero() {
		createdAt = parseGoodreadsDate(fieldAt(rec, f.dateAddedIdx))
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	// Dedupe: an identical note body already on the item means this review was
	// imported before, so re-imports create no duplicates.
	var existing int64
	if err := imp.db.Model(&models.BookNote{}).
		Where("user_id = ? AND item_id = ? AND body = ?", st.userID, item.ID, review).
		Count(&existing).Error; err != nil {
		return fmt.Errorf("checking existing note for item %d: %w", item.ID, err)
	}
	if existing > 0 {
		return nil
	}

	note := models.BookNote{
		UserID:    st.userID,
		ItemID:    item.ID,
		Body:      review,
		CreatedAt: createdAt, // GORM keeps a non-zero CreatedAt (backdated)
	}
	if err := imp.db.Create(&note).Error; err != nil {
		return fmt.Errorf("creating note for item %d: %w", item.ID, err)
	}
	st.notesCreated++
	return nil
}

// bookIndex indexes a user's tracked BOOK items for matching Goodreads rows:
// by ISBN-13 (the item's external id) first, then normalized title+author,
// then normalized title alone.
type bookIndex struct {
	byISBN        map[string]models.TrackingItem
	byTitleAuthor map[string]models.TrackingItem
	byTitle       map[string]models.TrackingItem
}

// loadBookIndex builds the match index from the user's BOOK tracking items,
// joining the shared Book cache for author strings.
func (imp *Importer) loadBookIndex(userID uint) (*bookIndex, error) {
	var items []models.TrackingItem
	if err := imp.db.Where("user_id = ? AND type = ?", userID, "BOOK").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("loading tracked books for user %d: %w", userID, err)
	}

	// Author strings live on the shared Book cache, keyed by ISBN-13.
	isbns := make([]string, 0, len(items))
	for _, it := range items {
		isbns = append(isbns, it.ExternalID)
	}
	authorByISBN := make(map[string]string, len(items))
	if len(isbns) > 0 {
		var books []models.Book
		if err := imp.db.Select("isbn13", "authors").Where("isbn13 IN ?", isbns).Find(&books).Error; err != nil {
			return nil, fmt.Errorf("loading book authors: %w", err)
		}
		for _, b := range books {
			authorByISBN[b.ISBN13] = b.Authors
		}
	}

	idx := &bookIndex{
		byISBN:        make(map[string]models.TrackingItem, len(items)),
		byTitleAuthor: make(map[string]models.TrackingItem, len(items)),
		byTitle:       make(map[string]models.TrackingItem, len(items)),
	}
	for _, it := range items {
		idx.byISBN[it.ExternalID] = it
		nt := normalizeTitle(it.Title)
		if nt == "" {
			continue
		}
		// First writer wins so an ambiguous later title cannot displace an
		// earlier item.
		if _, seen := idx.byTitle[nt]; !seen {
			idx.byTitle[nt] = it
		}
		if a := normalizeTitle(authorByISBN[it.ExternalID]); a != "" {
			key := nt + "|" + a
			if _, seen := idx.byTitleAuthor[key]; !seen {
				idx.byTitleAuthor[key] = it
			}
		}
	}
	return idx, nil
}

// match resolves a Goodreads row to a tracked BOOK item: ISBN-13 (from the
// ISBN13 column, falling back to a 13-digit ISBN column) wins; otherwise a
// normalized title+author match; otherwise a normalized title alone.
func (bi *bookIndex) match(f *parsedFile, rec []string) (models.TrackingItem, bool) {
	isbn13 := cleanISBN(fieldAt(rec, f.isbn13Idx))
	if len(isbn13) != 13 {
		if alt := cleanISBN(fieldAt(rec, f.isbnIdx)); len(alt) == 13 {
			isbn13 = alt
		}
	}
	if len(isbn13) == 13 {
		if it, ok := bi.byISBN[isbn13]; ok {
			return it, true
		}
	}

	nt := normalizeTitle(fieldAt(rec, f.titleIdx))
	if nt == "" {
		return models.TrackingItem{}, false
	}
	if a := normalizeTitle(fieldAt(rec, f.authorIdx)); a != "" {
		if it, ok := bi.byTitleAuthor[nt+"|"+a]; ok {
			return it, true
		}
	}
	if it, ok := bi.byTitle[nt]; ok {
		return it, true
	}
	return models.TrackingItem{}, false
}

// goodreadsDateFormats are the layouts Goodreads uses for Date Read / Date
// Added; the slash form is the common export shape.
var goodreadsDateFormats = []string{
	"2006/01/02",
	"2006-01-02",
}

// parseGoodreadsDate parses a Goodreads date cell, returning the zero time when
// it is empty or unparseable so the caller can fall back to the next source.
func parseGoodreadsDate(s string) time.Time {
	for _, layout := range goodreadsDateFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// loadEpisodeIDs builds the (season, number) → episode-ID lookup for a cached
// show. The bool is false when the show has no episodes yet, so the caller
// falls back to a TMDB fetch.
func (imp *Importer) loadEpisodeIDs(showID uint) (map[epKey]uint, bool) {
	var eps []models.Episode
	if err := imp.db.Select("id", "season", "number").
		Where("show_id = ?", showID).Find(&eps).Error; err != nil {
		return nil, false
	}
	if len(eps) == 0 {
		return nil, false
	}
	m := make(map[epKey]uint, len(eps))
	for _, e := range eps {
		m[epKey{e.Season, e.Number}] = e.ID
	}
	return m, true
}

// lookupAlias returns the TMDB id a normalized title previously resolved to.
func (imp *Importer) lookupAlias(norm string) (int, bool) {
	var a models.ShowAlias
	if err := imp.db.Where("norm_title = ?", norm).First(&a).Error; err != nil {
		return 0, false
	}
	return a.TMDBID, true
}

// saveAlias remembers that a normalized title maps to a TMDB id so future
// imports skip the TMDB search. Best-effort: a failure here never fails an
// import.
func (imp *Importer) saveAlias(norm string, tmdbID int) {
	alias := models.ShowAlias{NormTitle: norm}
	if err := imp.db.Where(&models.ShowAlias{NormTitle: norm}).
		Attrs(map[string]any{"tmdb_id": tmdbID}).
		FirstOrCreate(&alias).Error; err != nil {
		log.Printf("importer: saving title alias %q → %d: %v", norm, tmdbID, err)
	}
}

// ensureTracking upserts the user's TrackingItem for a show against the
// idx_user_media unique index; an existing row (any status) is left as-is.
func (imp *Importer) ensureTracking(userID uint, show *models.Show) error {
	item := models.TrackingItem{}
	err := imp.db.Where(&models.TrackingItem{
		UserID:     userID,
		Type:       "TV",
		ExternalID: strconv.Itoa(show.TMDBID),
	}).Attrs(map[string]any{
		"title":  show.Title,
		"status": "WATCHING",
	}).FirstOrCreate(&item).Error
	if err != nil {
		return fmt.Errorf("upserting tracking item for show %d: %w", show.TMDBID, err)
	}
	return nil
}

// upsertWatch inserts an EpisodeWatch if the user has none for the episode;
// re-imports therefore never create duplicates (idx_user_ep, E10).
func (imp *Importer) upsertWatch(userID, episodeID uint, watchedAt time.Time) error {
	watch := models.EpisodeWatch{}
	err := imp.db.Where(&models.EpisodeWatch{UserID: userID, EpisodeID: episodeID}).
		Attrs(map[string]any{"watched_at": watchedAt}).
		FirstOrCreate(&watch).Error
	if err != nil {
		return fmt.Errorf("upserting episode watch: %w", err)
	}
	return nil
}

// ── persistence helpers ──

func (imp *Importer) loadJob(jobID, userID uint) (*models.ImportJob, error) {
	var job models.ImportJob
	err := imp.db.Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("importer: loading job %d: %w", jobID, err)
	}
	return &job, nil
}

func (imp *Importer) persistProgress(jobID uint, st *runState) {
	if err := imp.db.Model(&models.ImportJob{}).Where("id = ?", jobID).
		Updates(map[string]any{
			"processed":  st.processed,
			"unresolved": encodeUnresolved(st.unresolved),
		}).Error; err != nil {
		log.Printf("importer: job %d: persisting progress: %v", jobID, err)
	}
}

func (imp *Importer) failJob(jobID uint, msg string) {
	if err := imp.db.Model(&models.ImportJob{}).Where("id = ?", jobID).
		Updates(map[string]any{"status": StatusFailed, "error": msg}).Error; err != nil {
		log.Printf("importer: job %d: marking FAILED: %v", jobID, err)
	}
}

func (imp *Importer) saveUnresolved(job *models.ImportJob, unresolved []string) error {
	job.Unresolved = encodeUnresolved(unresolved)
	if err := imp.db.Model(&models.ImportJob{}).Where("id = ?", job.ID).
		Update("unresolved", job.Unresolved).Error; err != nil {
		return fmt.Errorf("importer: saving unresolved list: %w", err)
	}
	return nil
}

func encodeUnresolved(titles []string) string {
	if titles == nil {
		titles = []string{}
	}
	b, err := json.Marshal(titles)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeUnresolved(raw string) []string {
	var titles []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &titles); err != nil {
			log.Printf("importer: corrupt unresolved JSON %q: %v", raw, err)
		}
	}
	if titles == nil {
		titles = []string{}
	}
	return titles
}

func removeTitle(titles []string, norm string) []string {
	out := titles[:0]
	for _, t := range titles {
		if normalizeTitle(t) != norm {
			out = append(out, t)
		}
	}
	return out
}

// ── row field helpers ──

func fieldAt(rec []string, idx int) string {
	if idx < 0 || idx >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[idx])
}

// firstIntField returns the first parseable integer among the candidate
// columns (e.g. season_number, then s_no), so exports that fill either column
// name resolve correctly per row.
func firstIntField(rec []string, idxs []int) (int, bool) {
	for _, idx := range idxs {
		if v := fieldAt(rec, idx); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// watchedAtFormats are the timestamp layouts seen in TV Time exports.
var watchedAtFormats = []string{
	"2006-01-02 15:04:05",
	time.RFC3339,
	"2006-01-02",
}

// parseWatchedAt parses a watched-at cell, falling back to now — a missing
// or odd timestamp should not discard a real watch.
func parseWatchedAt(s string) time.Time {
	for _, layout := range watchedAtFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}

// parseAirDate parses TMDB's YYYY-MM-DD air date; empty/invalid → nil
// (unannounced).
func parseAirDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}
