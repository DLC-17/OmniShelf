// Package sync implements the nightly TMDB synchronization engine.
//
// A run sweeps every show any user is WATCHING or PLAN_TO, refreshes show
// metadata and episode listings from TMDB, retries missing artwork, and
// records the outcome in a SyncLog row. Per-show failures are collected and
// never abort the run (E2); overlap with the CSV importer or a previous sync
// is prevented by the shared internal/jobs guard (E18).
//
// Rate limiting and 429 backoff live inside the TMDB client — no second
// layer is added here.
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/jobs"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// defaultImageBaseURL is the TMDB image CDN prefix joined with a show's
// poster_path to form a downloadable URL.
const defaultImageBaseURL = "https://image.tmdb.org/t/p/w500"

// ErrSkipped is returned by Run when another background job (a still-running
// sync or an in-flight import) holds the shared jobs guard (E18).
var ErrSkipped = errors.New("sync: run skipped: another background job is in flight")

// MetadataClient is the subset of *tmdb.Client the engine needs; tests
// substitute fakes or fixture-backed clients.
type MetadataClient interface {
	GetShow(ctx context.Context, id int) (*tmdb.Show, error)
	GetSeason(ctx context.Context, showID, seasonNum int) (*tmdb.Season, error)
}

// ArtworkStore is the subset of *images.Store the engine needs.
type ArtworkStore interface {
	Fetch(ctx context.Context, httpClient *http.Client, url, kind, externalID string) (string, error)
}

// Engine runs the nightly TMDB sync.
type Engine struct {
	db           *gorm.DB
	tmdb         MetadataClient
	images       ArtworkStore
	httpClient   *http.Client
	imageBaseURL string
}

// Option customizes an Engine.
type Option func(*Engine)

// WithHTTPClient overrides the HTTP client used for artwork downloads.
func WithHTTPClient(h *http.Client) Option {
	return func(e *Engine) { e.httpClient = h }
}

// WithImageBaseURL overrides the TMDB image CDN base URL (used by tests to
// point artwork downloads at an httptest.Server).
func WithImageBaseURL(u string) Option {
	return func(e *Engine) { e.imageBaseURL = u }
}

// New returns a sync Engine. db must be the application's shared *gorm.DB;
// tmdbClient and imagesStore are satisfied by *tmdb.Client and *images.Store.
func New(db *gorm.DB, tmdbClient MetadataClient, imagesStore ArtworkStore, opts ...Option) *Engine {
	e := &Engine{
		db:           db,
		tmdb:         tmdbClient,
		images:       imagesStore,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		imageBaseURL: defaultImageBaseURL,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Schedule registers the nightly 03:00 run on c. The orchestrating caller
// (cmd/omnishelf) owns starting and stopping the cron instance.
func (e *Engine) Schedule(c *cron.Cron) error {
	_, err := c.AddFunc("0 3 * * *", func() {
		if err := e.Run(context.Background()); err != nil && !errors.Is(err, ErrSkipped) {
			log.Printf("sync: nightly run failed: %v", err)
		}
	})
	if err != nil {
		return fmt.Errorf("sync: register cron entry: %w", err)
	}
	return nil
}

// Run executes one full synchronization sweep. It is directly callable (for
// tests and manual triggers) as well as from the cron schedule.
//
// If the shared background-job guard is held, Run logs and returns ErrSkipped
// without doing any work (E18). Per-show errors are collected into the
// SyncLog row and never abort the run; Run returns a non-nil error only for
// fatal failures (DB unavailable, panic).
func (e *Engine) Run(ctx context.Context) (err error) {
	if !jobs.TryLock() {
		log.Print(ErrSkipped.Error())
		return ErrSkipped
	}
	defer func() {
		// Background work must never crash the process.
		if r := recover(); r != nil {
			log.Printf("sync: recovered from panic: %v", r)
			err = fmt.Errorf("sync: panic during run: %v", r)
		}
		jobs.Unlock()
	}()

	start := time.Now()

	ids, err := e.collectShowIDs(ctx)
	if err != nil {
		return fmt.Errorf("sync: collect tracked shows: %w", err)
	}

	var runErrors []string
	synced := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			runErrors = append(runErrors, fmt.Sprintf("run aborted: %v", ctx.Err()))
			break
		}
		if showErr := e.syncShow(ctx, id); showErr != nil {
			// One failing show never aborts the run.
			log.Printf("sync: show %d: %v", id, showErr)
			runErrors = append(runErrors, fmt.Sprintf("show %d: %v", id, showErr))
			continue
		}
		synced++
	}

	if logErr := e.writeSyncLog(ctx, start, synced, runErrors); logErr != nil {
		return fmt.Errorf("sync: write sync log: %w", logErr)
	}
	log.Printf("sync: run complete: %d/%d shows synced, %d error(s)", synced, len(ids), len(runErrors))
	return nil
}

// collectShowIDs returns the distinct TMDB IDs of every show any user tracks
// with status WATCHING, PLAN_TO or COMPLETED, sorted for deterministic runs.
// COMPLETED shows are included so a newly aired episode is detected and can
// bump the show back to WATCHING.
func (e *Engine) collectShowIDs(ctx context.Context) ([]int, error) {
	var externalIDs []string
	err := e.db.WithContext(ctx).
		Model(&models.TrackingItem{}).
		Where("type = ? AND status IN ?", "TV", []string{"WATCHING", "PLAN_TO", "COMPLETED"}).
		Distinct().
		Pluck("external_id", &externalIDs).Error
	if err != nil {
		return nil, err
	}

	ids := make([]int, 0, len(externalIDs))
	for _, ext := range externalIDs {
		id, convErr := strconv.Atoi(ext)
		if convErr != nil {
			// Corrupt external ID: skip; nothing sensible to sync.
			log.Printf("sync: skipping non-numeric TV external_id %q", ext)
			continue
		}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids, nil
}

// episodeKey identifies an episode within a show by (season, number).
type episodeKey struct{ season, number int }

// syncShow refreshes one show: metadata upsert, per-season episode upserts,
// conservative pruning of upstream-deleted episodes, and missing-artwork
// retry. Season-level fetch failures are joined into the returned error but
// do not prevent applying the seasons that did fetch.
func (e *Engine) syncShow(ctx context.Context, tmdbID int) error {
	remote, err := e.tmdb.GetShow(ctx, tmdbID)
	if err != nil {
		return fmt.Errorf("fetch show: %w", err)
	}

	// Fetch every season listing up front (network), then apply all DB
	// mutations in one transaction. Only seasons whose listing was
	// definitively fetched participate in pruning (E17/E19 conservatism).
	upstream := make(map[episodeKey]tmdb.Episode)
	fetchedSeasons := make([]int, 0, len(remote.Seasons))
	var seasonErrs []error
	for _, s := range remote.Seasons {
		season, sErr := e.tmdb.GetSeason(ctx, tmdbID, s.SeasonNumber)
		if sErr != nil {
			seasonErrs = append(seasonErrs, fmt.Errorf("season %d: %w", s.SeasonNumber, sErr))
			continue
		}
		fetchedSeasons = append(fetchedSeasons, season.SeasonNumber)
		for _, ep := range season.Episodes {
			upstream[episodeKey{season.SeasonNumber, ep.EpisodeNumber}] = ep
		}
	}

	// Missing-artwork retry (E13): only when no poster is cached locally.
	// Failures are non-fatal — next night retries again.
	posterRel := ""
	var local models.Show
	findErr := e.db.WithContext(ctx).Where("tmdb_id = ?", tmdbID).First(&local).Error
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load show row: %w", findErr)
	}
	if local.PosterPath == "" && remote.PosterPath != "" {
		rel, imgErr := e.images.Fetch(ctx, e.httpClient, e.imageBaseURL+remote.PosterPath, "tv", strconv.Itoa(tmdbID))
		if imgErr != nil {
			log.Printf("sync: show %d: artwork fetch failed (will retry next run): %v", tmdbID, imgErr)
		} else {
			posterRel = rel
		}
	}

	var syncedShowID uint
	txErr := e.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		showRow, upErr := upsertShow(tx, tmdbID, remote, posterRel)
		if upErr != nil {
			return upErr
		}
		syncedShowID = showRow.ID
		if upErr := upsertEpisodes(tx, showRow.ID, upstream); upErr != nil {
			return upErr
		}
		return pruneDeletedEpisodes(tx, showRow.ID, fetchedSeasons, upstream)
	})
	if txErr != nil {
		return fmt.Errorf("persist: %w", txErr)
	}

	// A newly aired episode means anyone who had this show COMPLETED is no
	// longer caught up: bump them back to WATCHING.
	if recErr := reconcileCompletedToWatching(e.db.WithContext(ctx), syncedShowID, tmdbID); recErr != nil {
		return fmt.Errorf("reconcile completed shows: %w", recErr)
	}
	return errors.Join(seasonErrs...)
}

// reconcileCompletedToWatching flips a show's COMPLETED trackers back to
// WATCHING for any user who now has an aired, unwatched episode of it.
func reconcileCompletedToWatching(db *gorm.DB, showID uint, tmdbID int) error {
	var items []models.TrackingItem
	if err := db.
		Where("type = ? AND external_id = ? AND status = ?", "TV", strconv.Itoa(tmdbID), "COMPLETED").
		Find(&items).Error; err != nil {
		return fmt.Errorf("load completed trackers: %w", err)
	}
	now := time.Now()
	for _, it := range items {
		var count int64
		if err := db.Model(&models.Episode{}).
			Where("show_id = ? AND air_date IS NOT NULL AND air_date <= ?", showID, now).
			Where("NOT EXISTS (SELECT 1 FROM episode_watches w WHERE w.episode_id = episodes.id AND w.user_id = ?)", it.UserID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("count unwatched for user %d: %w", it.UserID, err)
		}
		if count > 0 {
			if err := db.Model(&models.TrackingItem{}).Where("id = ?", it.ID).
				Update("status", "WATCHING").Error; err != nil {
				return fmt.Errorf("bump user %d to WATCHING: %w", it.UserID, err)
			}
		}
	}
	return nil
}

// upsertShow creates or updates the shared Show metadata row, always bumping
// LastSyncedAt, and returns the current row.
func upsertShow(tx *gorm.DB, tmdbID int, remote *tmdb.Show, posterRel string) (*models.Show, error) {
	now := time.Now()
	var row models.Show
	err := tx.Where("tmdb_id = ?", tmdbID).First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		row = models.Show{
			TMDBID:       tmdbID,
			Title:        remote.Name,
			PosterPath:   posterRel,
			Status:       remote.Status,
			LastSyncedAt: now,
		}
		if createErr := tx.Create(&row).Error; createErr != nil {
			return nil, fmt.Errorf("create show: %w", createErr)
		}
		return &row, nil
	case err != nil:
		return nil, fmt.Errorf("find show: %w", err)
	}

	updates := map[string]any{"last_synced_at": now}
	if remote.Name != "" && row.Title != remote.Name {
		updates["title"] = remote.Name
	}
	if row.Status != remote.Status {
		updates["status"] = remote.Status
	}
	if posterRel != "" && row.PosterPath != posterRel {
		updates["poster_path"] = posterRel
	}
	if updErr := tx.Model(&row).Updates(updates).Error; updErr != nil {
		return nil, fmt.Errorf("update show: %w", updErr)
	}
	return &row, nil
}

// upsertEpisodes inserts new episodes and applies changed titles/air dates,
// matching on (ShowID, Season, Number).
func upsertEpisodes(tx *gorm.DB, showID uint, upstream map[episodeKey]tmdb.Episode) error {
	for key, ep := range upstream {
		var row models.Episode
		err := tx.Where("show_id = ? AND season = ? AND number = ?", showID, key.season, key.number).First(&row).Error
		newAir := parseAirDate(ep.AirDate)
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			row = models.Episode{
				ShowID:  showID,
				Season:  key.season,
				Number:  key.number,
				Title:   ep.Name,
				AirDate: newAir,
			}
			if createErr := tx.Create(&row).Error; createErr != nil {
				return fmt.Errorf("create episode s%02de%02d: %w", key.season, key.number, createErr)
			}
		case err != nil:
			return fmt.Errorf("find episode s%02de%02d: %w", key.season, key.number, err)
		default:
			if row.Title == ep.Name && timePtrEqual(row.AirDate, newAir) {
				continue
			}
			// Air-date moves (including into the future, E19) update the row
			// in place; existing EpisodeWatch rows are untouched.
			updates := map[string]any{"title": ep.Name, "air_date": newAir}
			if updErr := tx.Model(&row).Updates(updates).Error; updErr != nil {
				return fmt.Errorf("update episode s%02de%02d: %w", key.season, key.number, updErr)
			}
		}
	}
	return nil
}

// pruneDeletedEpisodes removes local episodes that a definitively fetched
// season listing no longer contains, together with their EpisodeWatch rows
// (E17). Seasons that failed to fetch — or vanished from the show's season
// list entirely — are left untouched, so deletions stay conservative (E19).
func pruneDeletedEpisodes(tx *gorm.DB, showID uint, fetchedSeasons []int, upstream map[episodeKey]tmdb.Episode) error {
	if len(fetchedSeasons) == 0 {
		return nil
	}
	var locals []models.Episode
	if err := tx.Where("show_id = ? AND season IN ?", showID, fetchedSeasons).Find(&locals).Error; err != nil {
		return fmt.Errorf("list local episodes: %w", err)
	}
	for _, ep := range locals {
		if _, ok := upstream[episodeKey{ep.Season, ep.Number}]; ok {
			continue
		}
		// Orphaned watch rows go first so a mid-transaction failure cannot
		// leave watches pointing at a deleted episode.
		if err := tx.Where("episode_id = ?", ep.ID).Delete(&models.EpisodeWatch{}).Error; err != nil {
			return fmt.Errorf("prune watches for episode %d: %w", ep.ID, err)
		}
		if err := tx.Delete(&models.Episode{}, ep.ID).Error; err != nil {
			return fmt.Errorf("delete episode %d: %w", ep.ID, err)
		}
	}
	return nil
}

// writeSyncLog records the run outcome. ShowCount is the number of shows
// synced without error; per-show failures are in Errors as a JSON array.
func (e *Engine) writeSyncLog(ctx context.Context, ranAt time.Time, showCount int, runErrors []string) error {
	if runErrors == nil {
		runErrors = []string{}
	}
	blob, err := json.Marshal(runErrors)
	if err != nil {
		return fmt.Errorf("marshal errors: %w", err)
	}
	row := models.SyncLog{RanAt: ranAt, ShowCount: showCount, Errors: string(blob)}
	// context.Background: the outcome row must be written even when the run
	// was cut short by ctx cancellation.
	if ctx.Err() != nil {
		ctx = context.Background()
	}
	return e.db.WithContext(ctx).Create(&row).Error
}

// parseAirDate converts TMDB's "YYYY-MM-DD" (or "" for unannounced) into the
// nullable air-date representation used by models.Episode.
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

// timePtrEqual reports whether two nullable timestamps are the same instant
// (or both unset).
func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
