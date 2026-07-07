// Package movies implements the movie-domain service layer: TMDB movie search
// proxying, adding movies (metadata + poster caching + tracking), and Discover
// recommendations. It mirrors internal/tv but a movie is a single unit — there
// are no seasons, episodes, or Up Next. Listing/updating/deleting a tracked
// movie goes through the shared library service (internal/books), which treats
// a movie as a TrackingItem of type "MOVIE".
package movies

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

const defaultImageBaseURL = "https://image.tmdb.org/t/p/w500"

// Type and default status for a tracked movie. A freshly-added movie is a
// watchlist entry (PLAN_TO); the user flips it to COMPLETED once watched.
const (
	typeMovie     = "MOVIE"
	statusDefault = "PLAN_TO"
)

// TMDB is the subset of *tmdb.Client the service needs.
type TMDB interface {
	SearchMovie(ctx context.Context, query string) (*tmdb.MovieSearchResponse, error)
	GetMovie(ctx context.Context, id int) (*tmdb.Movie, error)
	MovieRecommendations(ctx context.Context, movieID int) (*tmdb.MovieSearchResponse, error)
}

// ImageStore is the subset of *images.Store the service needs.
type ImageStore interface {
	Fetch(ctx context.Context, httpClient *http.Client, url, kind, externalID string) (string, error)
}

// ConflictError reports a duplicate track attempt; it carries the existing
// item so the API layer can return it alongside the 409.
type ConflictError struct {
	Existing models.TrackingItem
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("movies: movie %s already tracked by user %d", e.Existing.ExternalID, e.Existing.UserID)
}

// UpstreamError wraps a TMDB failure during an interactive request; the API
// layer translates it to 502.
type UpstreamError struct{ Err error }

func (e *UpstreamError) Error() string { return "movies: tmdb upstream error: " + e.Err.Error() }
func (e *UpstreamError) Unwrap() error { return e.Err }

// ErrNotFound is returned when a referenced movie does not exist on TMDB.
var ErrNotFound = errors.New("movies: not found")

// Service holds the movie-domain business logic.
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

// WithHTTPClient overrides the poster-download HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(s *Service) { s.httpClient = h }
}

// New returns a movie service.
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

// upstream classifies a TMDB error: a clean 404 means the movie id does not
// exist (→ ErrNotFound); anything else is an upstream outage.
func upstream(err error) error {
	var se *tmdb.StatusError
	if errors.As(err, &se) && se.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return &UpstreamError{Err: err}
}

// Search proxies a TMDB movie search.
func (s *Service) Search(ctx context.Context, query string) (*tmdb.MovieSearchResponse, error) {
	res, err := s.tmdb.SearchMovie(ctx, query)
	if err != nil {
		return nil, upstream(err)
	}
	return res, nil
}

// AddResult is what AddMovie returns on success.
type AddResult struct {
	Movie models.Movie
	Item  models.TrackingItem
}

// AddMovie fetches a movie from TMDB, upserts the shared Movie metadata cache,
// best-effort caches the poster (a failure leaves PosterPath empty and does not
// fail the add), and creates the user's PLAN_TO TrackingItem. A duplicate track
// returns *ConflictError. DB-first: an already-cached movie skips the TMDB
// round-trip.
func (s *Service) AddMovie(ctx context.Context, userID uint, tmdbID int) (*AddResult, error) {
	externalID := strconv.Itoa(tmdbID)

	var existing models.TrackingItem
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND external_id = ?", userID, typeMovie, externalID).
		First(&existing).Error
	if err == nil {
		return nil, &ConflictError{Existing: existing}
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("movies: duplicate check: %w", err)
	}

	// DB-first: reuse a cached movie without a TMDB round-trip.
	var movie models.Movie
	cacheErr := s.db.WithContext(ctx).Where("tmdb_id = ?", tmdbID).First(&movie).Error
	if errors.Is(cacheErr, gorm.ErrRecordNotFound) {
		detail, err := s.tmdb.GetMovie(ctx, tmdbID)
		if err != nil {
			return nil, upstream(err)
		}
		movie = models.Movie{
			TMDBID:       detail.ID,
			Title:        detail.Title,
			Overview:     detail.Overview,
			ReleaseDate:  detail.ReleaseDate,
			LastSyncedAt: time.Now(),
		}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tmdb_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "overview", "release_date", "last_synced_at"}),
		}).Create(&movie).Error; err != nil {
			return nil, fmt.Errorf("movies: upsert movie: %w", err)
		}
		if err := s.db.WithContext(ctx).Where("tmdb_id = ?", detail.ID).First(&movie).Error; err != nil {
			return nil, fmt.Errorf("movies: reload movie: %w", err)
		}
		// Best-effort poster caching.
		if movie.PosterPath == "" && detail.PosterPath != "" {
			rel, err := s.images.Fetch(ctx, s.httpClient, s.imageBase+detail.PosterPath, "movie", externalID)
			if err != nil {
				log.Printf("movies: poster download for movie %d failed: %v", tmdbID, err)
			} else if err := s.db.WithContext(ctx).Model(&movie).Update("poster_path", rel).Error; err != nil {
				return nil, fmt.Errorf("movies: save poster path: %w", err)
			} else {
				movie.PosterPath = rel
			}
		}
	} else if cacheErr != nil {
		return nil, fmt.Errorf("movies: cache lookup: %w", cacheErr)
	}

	// Create the tracking link. OnConflict DoNothing + RowsAffected==0 detects
	// a concurrent duplicate without driver-specific error strings.
	item := models.TrackingItem{
		UserID:     userID,
		Type:       typeMovie,
		ExternalID: externalID,
		Title:      movie.Title,
		Status:     statusDefault,
	}
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
	if res.Error != nil {
		return nil, fmt.Errorf("movies: create tracking item: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		if err := s.db.WithContext(ctx).
			Where("user_id = ? AND type = ? AND external_id = ?", userID, typeMovie, externalID).
			First(&existing).Error; err != nil {
			return nil, fmt.Errorf("movies: load conflicting item: %w", err)
		}
		return nil, &ConflictError{Existing: existing}
	}

	return &AddResult{Movie: movie, Item: item}, nil
}

// DiscoverItem is one movie recommendation for the Discover page.
type DiscoverItem struct {
	TMDBID      int
	Title       string
	Overview    string
	PosterPath  string // raw TMDB path (not cached; the UI thumbnails it)
	ReleaseDate string
	SuggestedBy string // title of the tracked movie this came from
}

const (
	maxDiscoverSources = 5
	maxDiscoverResults = 24
)

// Discover suggests movies based on what the user already tracks: TMDB
// recommendations for their most recently updated movies, excluding anything
// already tracked or previously rejected. A failing source is skipped.
func (s *Service) Discover(ctx context.Context, userID uint) ([]DiscoverItem, error) {
	var sources []models.TrackingItem
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND type = ?", userID, typeMovie).
		Order("updated_at DESC").Limit(maxDiscoverSources).
		Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("movies: discover sources: %w", err)
	}
	if len(sources) == 0 {
		return []DiscoverItem{}, nil
	}

	tracked, err := s.externalIDSet(ctx, userID, &models.TrackingItem{})
	if err != nil {
		return nil, err
	}
	rejected, err := s.externalIDSet(ctx, userID, &models.RejectedRec{})
	if err != nil {
		return nil, err
	}

	seen := map[int]bool{}
	out := make([]DiscoverItem, 0, maxDiscoverResults)
	for _, src := range sources {
		if len(out) >= maxDiscoverResults {
			break
		}
		srcID, convErr := strconv.Atoi(src.ExternalID)
		if convErr != nil {
			continue
		}
		resp, recErr := s.tmdb.MovieRecommendations(ctx, srcID)
		if recErr != nil {
			log.Printf("movies: recommendations for %d: %v", srcID, recErr)
			continue
		}
		for _, r := range resp.Results {
			if len(out) >= maxDiscoverResults {
				break
			}
			if tracked[r.ID] || rejected[r.ID] || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, DiscoverItem{
				TMDBID:      r.ID,
				Title:       r.Title,
				Overview:    r.Overview,
				PosterPath:  r.PosterPath,
				ReleaseDate: r.ReleaseDate,
				SuggestedBy: src.Title,
			})
		}
	}
	return out, nil
}

// externalIDSet loads the user's MOVIE external IDs from the given model
// (TrackingItem or RejectedRec) as a set of TMDB IDs.
func (s *Service) externalIDSet(ctx context.Context, userID uint, model any) (map[int]bool, error) {
	var rows []struct{ ExternalID string }
	if err := s.db.WithContext(ctx).Model(model).
		Where("user_id = ? AND type = ?", userID, typeMovie).
		Select("external_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("movies: load external ids: %w", err)
	}
	set := make(map[int]bool, len(rows))
	for _, r := range rows {
		if id, err := strconv.Atoi(r.ExternalID); err == nil {
			set[id] = true
		}
	}
	return set, nil
}

// RejectRec hides a movie suggestion so Discover will not surface it again.
func (s *Service) RejectRec(ctx context.Context, userID uint, tmdbID int) error {
	rec := models.RejectedRec{UserID: userID, Type: typeMovie, ExternalID: strconv.Itoa(tmdbID)}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error; err != nil {
		return fmt.Errorf("movies: reject recommendation: %w", err)
	}
	return nil
}
