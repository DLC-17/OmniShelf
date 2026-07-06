// Package tv implements the TV-domain service layer: TMDB search proxying,
// adding shows (metadata + episode persistence + poster caching + tracking),
// the Up Next query, and the one-tap watch toggle.
//
// HTTP handlers in internal/api stay thin and delegate here
// (business logic lives in /internal/* packages).
package tv

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// defaultImageBaseURL is where TMDB poster paths resolve to a real file.
// Overridable so tests point it at a fixture server.
const defaultImageBaseURL = "https://image.tmdb.org/t/p/w500"

// TMDB is the subset of *tmdb.Client the service needs. A small local
// interface so tests can substitute a fake.
type TMDB interface {
	SearchTV(ctx context.Context, query string) (*tmdb.SearchResponse, error)
	GetShow(ctx context.Context, id int) (*tmdb.Show, error)
	GetSeason(ctx context.Context, showID, seasonNum int) (*tmdb.Season, error)
}

// ImageStore is the subset of *images.Store the service needs.
type ImageStore interface {
	Fetch(ctx context.Context, httpClient *http.Client, url, kind, externalID string) (string, error)
}

// ConflictError reports a duplicate track attempt (E16). It carries the
// existing item so the API layer can return it alongside the 409.
type ConflictError struct {
	Existing models.TrackingItem
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("tv: show %s already tracked by user %d", e.Existing.ExternalID, e.Existing.UserID)
}

// UpstreamError wraps a TMDB failure during an interactive request; the API
// layer translates it to 502 (E3).
type UpstreamError struct {
	Err error
}

func (e *UpstreamError) Error() string { return "tv: tmdb upstream error: " + e.Err.Error() }
func (e *UpstreamError) Unwrap() error { return e.Err }

// ErrNotFound is returned when a referenced show or episode does not exist.
var ErrNotFound = errors.New("tv: not found")

// Service holds the TV-domain business logic.
type Service struct {
	db         *gorm.DB
	tmdb       TMDB
	images     ImageStore
	httpClient *http.Client
	imageBase  string
}

// Option customizes a Service.
type Option func(*Service)

// WithImageBaseURL overrides the TMDB image CDN base (tests only).
func WithImageBaseURL(u string) Option {
	return func(s *Service) { s.imageBase = u }
}

// WithHTTPClient overrides the HTTP client used for poster downloads.
func WithHTTPClient(h *http.Client) Option {
	return func(s *Service) { s.httpClient = h }
}

// New returns a TV service backed by the given DB, TMDB client, and image
// cache writer.
func New(gdb *gorm.DB, t TMDB, imgs ImageStore, opts ...Option) *Service {
	s := &Service{
		db:         gdb,
		tmdb:       t,
		images:     imgs,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		imageBase:  defaultImageBaseURL,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// upstream classifies a TMDB error: a clean 404 from TMDB means the show ID
// does not exist (→ ErrNotFound); anything else is an upstream outage (E3).
func upstream(err error) error {
	var se *tmdb.StatusError
	if errors.As(err, &se) && se.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return &UpstreamError{Err: err}
}

// Search proxies a TMDB TV search.
func (s *Service) Search(ctx context.Context, query string) (*tmdb.SearchResponse, error) {
	res, err := s.tmdb.SearchTV(ctx, query)
	if err != nil {
		return nil, upstream(err)
	}
	return res, nil
}

// AddResult is what AddShow returns on success.
type AddResult struct {
	Show models.Show
	Item models.TrackingItem
}

// AddShow fetches a show and all its seasons from TMDB, upserts the shared
// Show/Episode metadata cache, best-effort caches the poster (failure leaves
// PosterPath empty and does not fail the add, E13), and creates the user's
// WATCHING TrackingItem. A duplicate track returns *ConflictError (E16).
func (s *Service) AddShow(ctx context.Context, userID uint, tmdbID int) (*AddResult, error) {
	externalID := strconv.Itoa(tmdbID)

	// Early duplicate check saves the TMDB round-trips; the unique index
	// idx_user_media (via the OnConflict create below) remains the
	// race-safe guard.
	var existing models.TrackingItem
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND external_id = ?", userID, "TV", externalID).
		First(&existing).Error
	if err == nil {
		return nil, &ConflictError{Existing: existing}
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("tv: duplicate check: %w", err)
	}

	// Fetch everything from TMDB before touching the DB so an upstream
	// failure mid-way never leaves a half-imported show.
	detail, err := s.tmdb.GetShow(ctx, tmdbID)
	if err != nil {
		return nil, upstream(err)
	}
	seasons := make([]*tmdb.Season, 0, len(detail.Seasons))
	for _, ss := range detail.Seasons {
		season, err := s.tmdb.GetSeason(ctx, tmdbID, ss.SeasonNumber)
		if err != nil {
			return nil, upstream(err)
		}
		seasons = append(seasons, season)
	}

	var show models.Show
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		show = models.Show{
			TMDBID:       detail.ID,
			Title:        detail.Name,
			Status:       detail.Status,
			LastSyncedAt: time.Now(),
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tmdb_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "status", "last_synced_at"}),
		}).Create(&show).Error; err != nil {
			return fmt.Errorf("upsert show: %w", err)
		}
		// Re-read: on conflict SQLite does not reliably report the
		// pre-existing row's primary key back into the struct.
		if err := tx.Where("tmdb_id = ?", detail.ID).First(&show).Error; err != nil {
			return fmt.Errorf("reload show: %w", err)
		}

		for _, season := range seasons {
			for _, e := range season.Episodes {
				ep := models.Episode{
					ShowID:  show.ID,
					Season:  e.SeasonNumber,
					Number:  e.EpisodeNumber,
					Title:   e.Name,
					AirDate: parseAirDate(e.AirDate),
				}
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "show_id"}, {Name: "season"}, {Name: "number"}},
					DoUpdates: clause.AssignmentColumns([]string{"title", "air_date"}),
				}).Create(&ep).Error; err != nil {
					return fmt.Errorf("upsert episode s%02de%02d: %w", e.SeasonNumber, e.EpisodeNumber, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("tv: persist show %d: %w", tmdbID, err)
	}

	// Create the tracking link before spending time on the poster download.
	// OnConflict DoNothing + RowsAffected==0 detects a concurrent duplicate
	// without relying on driver-specific constraint error strings (E16).
	item := models.TrackingItem{
		UserID:     userID,
		Type:       "TV",
		ExternalID: externalID,
		Title:      detail.Name,
		Status:     "WATCHING",
	}
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
	if res.Error != nil {
		return nil, fmt.Errorf("tv: create tracking item: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		if err := s.db.WithContext(ctx).
			Where("user_id = ? AND type = ? AND external_id = ?", userID, "TV", externalID).
			First(&existing).Error; err != nil {
			return nil, fmt.Errorf("tv: load conflicting item: %w", err)
		}
		return nil, &ConflictError{Existing: existing}
	}

	// Best-effort poster caching (E13): a failed download must never fail
	// the add; the nightly sync retries missing artwork.
	if show.PosterPath == "" && detail.PosterPath != "" {
		rel, err := s.images.Fetch(ctx, s.httpClient, s.imageBase+detail.PosterPath, "tv", externalID)
		if err != nil {
			log.Printf("tv: poster download for show %d failed (will retry on nightly sync): %v", tmdbID, err)
		} else if err := s.db.WithContext(ctx).Model(&show).Update("poster_path", rel).Error; err != nil {
			return nil, fmt.Errorf("tv: save poster path: %w", err)
		} else {
			show.PosterPath = rel
		}
	}

	return &AddResult{Show: show, Item: item}, nil
}

// UpNextEntry is one Up Next card: a WATCHING show and its earliest aired,
// unwatched episode.
type UpNextEntry struct {
	Show    models.Show
	Episode models.Episode
}

// UpNext returns, for each of the user's WATCHING TV tracking
// items, the minimum (season, number) episode with a non-nil air date <= now
// and no EpisodeWatch row for this user. Shows with no such episode are
// omitted.
func (s *Service) UpNext(ctx context.Context, userID uint) ([]UpNextEntry, error) {
	var items []models.TrackingItem
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND status = ?", userID, "TV", "WATCHING").
		Order("title").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("tv: list watching items: %w", err)
	}

	entries := make([]UpNextEntry, 0, len(items))
	for _, it := range items {
		tmdbID, err := strconv.Atoi(it.ExternalID)
		if err != nil {
			// A TV item with a non-numeric external ID is corrupt data,
			// not a request failure — skip rather than break the dashboard.
			log.Printf("tv: tracking item %d has non-numeric external id %q", it.ID, it.ExternalID)
			continue
		}
		var show models.Show
		err = s.db.WithContext(ctx).Where("tmdb_id = ?", tmdbID).First(&show).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue // metadata cache missing (e.g., pruned) — nothing to surface
		}
		if err != nil {
			return nil, fmt.Errorf("tv: load show %d: %w", tmdbID, err)
		}
		ep, err := s.nextUp(ctx, userID, show.ID)
		if err != nil {
			return nil, err
		}
		if ep == nil {
			continue
		}
		entries = append(entries, UpNextEntry{Show: show, Episode: *ep})
	}
	return entries, nil
}

// MarkWatched records the episode as seen (idempotent: re-marking is a
// no-op, never a duplicate row) and returns the show's new next-up episode
// (nil when the show has no aired unwatched episodes left).
func (s *Service) MarkWatched(ctx context.Context, userID, episodeID uint) (*models.Episode, error) {
	ep, err := s.episode(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	watch := models.EpisodeWatch{UserID: userID, EpisodeID: episodeID, WatchedAt: time.Now()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&watch).Error; err != nil {
		return nil, fmt.Errorf("tv: mark watched: %w", err)
	}
	return s.nextUp(ctx, userID, ep.ShowID)
}

// UnmarkWatched removes the user's watch row for the episode (idempotent:
// un-watching an unwatched episode is a no-op) and returns the show's new
// next-up episode.
func (s *Service) UnmarkWatched(ctx context.Context, userID, episodeID uint) (*models.Episode, error) {
	ep, err := s.episode(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND episode_id = ?", userID, episodeID).
		Delete(&models.EpisodeWatch{}).Error; err != nil {
		return nil, fmt.Errorf("tv: unmark watched: %w", err)
	}
	return s.nextUp(ctx, userID, ep.ShowID)
}

// EpisodeState is one episode of a show plus this user's watch state, as
// returned by ListEpisodes for the expandable episode picker.
type EpisodeState struct {
	Episode   models.Episode
	Watched   bool
	WatchedAt *time.Time
}

// ListEpisodes returns every episode of a show in (season, number) order with
// the caller's per-episode watched flag. A missing show is ErrNotFound.
func (s *Service) ListEpisodes(ctx context.Context, userID, showID uint) ([]EpisodeState, error) {
	var show models.Show
	err := s.db.WithContext(ctx).First(&show, showID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: show %d", ErrNotFound, showID)
	}
	if err != nil {
		return nil, fmt.Errorf("tv: load show %d: %w", showID, err)
	}

	var eps []models.Episode
	if err := s.db.WithContext(ctx).
		Where("show_id = ?", showID).
		Order("season, number").
		Find(&eps).Error; err != nil {
		return nil, fmt.Errorf("tv: list episodes for show %d: %w", showID, err)
	}

	var watches []models.EpisodeWatch
	if err := s.db.WithContext(ctx).
		Joins("JOIN episodes e ON e.id = episode_watches.episode_id").
		Where("e.show_id = ? AND episode_watches.user_id = ?", showID, userID).
		Find(&watches).Error; err != nil {
		return nil, fmt.Errorf("tv: load watch state for show %d: %w", showID, err)
	}
	watchedAt := make(map[uint]time.Time, len(watches))
	for _, w := range watches {
		watchedAt[w.EpisodeID] = w.WatchedAt
	}

	states := make([]EpisodeState, 0, len(eps))
	for _, e := range eps {
		st := EpisodeState{Episode: e}
		if t, ok := watchedAt[e.ID]; ok {
			st.Watched = true
			when := t
			st.WatchedAt = &when
		}
		states = append(states, st)
	}
	return states, nil
}

// Rewatch (re-)stamps the episode as watched now. Unlike MarkWatched it is not
// a no-op when a watch row already exists: it refreshes WatchedAt so a rewatch
// bumps the episode to the top of the activity feed.
func (s *Service) Rewatch(ctx context.Context, userID, episodeID uint) (*models.Episode, error) {
	ep, err := s.episode(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	watch := models.EpisodeWatch{UserID: userID, EpisodeID: episodeID, WatchedAt: time.Now()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "episode_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"watched_at"}),
	}).Create(&watch).Error; err != nil {
		return nil, fmt.Errorf("tv: rewatch: %w", err)
	}
	return s.nextUp(ctx, userID, ep.ShowID)
}

// WatchThrough marks the target episode and every earlier aired episode of the
// same show as watched in one go — for the "I've seen everything up to here"
// choice. Already-watched episodes are left untouched (their timestamps are
// preserved). Returns the show's new next-up episode.
func (s *Service) WatchThrough(ctx context.Context, userID, episodeID uint) (*models.Episode, error) {
	target, err := s.episode(ctx, episodeID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var eps []models.Episode
	if err := s.db.WithContext(ctx).
		Where("show_id = ? AND air_date IS NOT NULL AND air_date <= ?", target.ShowID, now).
		Where("season < ? OR (season = ? AND number <= ?)", target.Season, target.Season, target.Number).
		Find(&eps).Error; err != nil {
		return nil, fmt.Errorf("tv: watch-through query: %w", err)
	}

	rows := make([]models.EpisodeWatch, 0, len(eps))
	for _, e := range eps {
		rows = append(rows, models.EpisodeWatch{UserID: userID, EpisodeID: e.ID, WatchedAt: now})
	}
	if len(rows) > 0 {
		if err := s.db.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&rows).Error; err != nil {
			return nil, fmt.Errorf("tv: watch-through insert: %w", err)
		}
	}
	return s.nextUp(ctx, userID, target.ShowID)
}

// episode loads an episode by ID, mapping a missing row to ErrNotFound.
func (s *Service) episode(ctx context.Context, id uint) (*models.Episode, error) {
	var ep models.Episode
	err := s.db.WithContext(ctx).First(&ep, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: episode %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("tv: load episode %d: %w", id, err)
	}
	return &ep, nil
}

// nextUp computes the Up Next episode for one show: the minimum
// (season, number) episode with air_date IS NOT NULL AND air_date <= now and
// no EpisodeWatch row for the user. Returns nil (no error) when none exists.
func (s *Service) nextUp(ctx context.Context, userID, showID uint) (*models.Episode, error) {
	var ep models.Episode
	err := s.db.WithContext(ctx).
		Where("show_id = ? AND air_date IS NOT NULL AND air_date <= ?", showID, time.Now()).
		Where("NOT EXISTS (SELECT 1 FROM episode_watches w WHERE w.episode_id = episodes.id AND w.user_id = ?)", userID).
		Order("season, number").
		First(&ep).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tv: next-up query for show %d: %w", showID, err)
	}
	return &ep, nil
}

// parseAirDate converts TMDB's "YYYY-MM-DD" (or "" for unannounced) into a
// nullable time. Unparseable values are treated as unannounced rather than
// failing the whole import.
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
