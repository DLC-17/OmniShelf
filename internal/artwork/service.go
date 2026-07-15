// Package artwork implements refreshing and replacing the cover art of a
// tracked media item. Two operations are supported, both keyed by the user's
// TrackingItem:
//
//   - Refresh re-pulls the latest cover from the upstream source (TMDB for TV
//     and movies, IGDB for games, OpenLibrary for books) and overwrites the
//     cached image.
//   - Upload stores a user-supplied image as the cover.
//
// Cover art lives in the shared metadata cache (Show/Movie/Game/Book), one row
// per media across all users, so a refresh or upload changes the artwork for
// everyone who tracks that title — consistent with how the rest of the cache
// is shared.
package artwork

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/openlibrary"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// tmdbImageBase is where a TMDB poster path resolves to a real image file.
const tmdbImageBase = "https://image.tmdb.org/t/p/w500"

// Media types as stored in TrackingItem.Type (mirrors books/games constants,
// declared here so this package has no dependency on those service packages).
const (
	typeTV    = "TV"
	typeMovie = "MOVIE"
	typeGame  = "GAME"
	typeBook  = "BOOK"
)

// Sentinel errors the API layer maps onto the standard envelope.
var (
	// ErrItemNotFound means the tracking item does not exist for this user
	// (someone else's item is indistinguishable from a missing one).
	ErrItemNotFound = errors.New("artwork: tracking item not found")
	// ErrNoArtwork means the upstream source has no cover for this title, so a
	// refresh has nothing to pull.
	ErrNoArtwork = errors.New("artwork: no cover art available upstream")
	// ErrUpstream means an upstream metadata/image request failed.
	ErrUpstream = errors.New("artwork: upstream unavailable")
	// ErrUnsupported means the media type has no known artwork source.
	ErrUnsupported = errors.New("artwork: unsupported media type")
)

// TMDB is the slice of *tmdb.Client this package needs.
type TMDB interface {
	GetShow(ctx context.Context, id int) (*tmdb.Show, error)
	GetMovie(ctx context.Context, id int) (*tmdb.Movie, error)
}

// IGDB is the slice of *igdb.Client this package needs. Optional: when nil,
// game refresh reports ErrNoArtwork instead of panicking.
type IGDB interface {
	GetGame(ctx context.Context, igdbID int) (*igdb.Game, error)
	SearchGames(ctx context.Context, name string) ([]igdb.SearchResult, error)
	CoverURL(imageID, size string) string
}

// OpenLibrary is the slice of *openlibrary.Client this package needs.
type OpenLibrary interface {
	GetByISBN(ctx context.Context, isbn string) (*openlibrary.Book, error)
	CoverURL(coverID int, size string) string
}

// ImageStore is the slice of *images.Store this package needs.
type ImageStore interface {
	Fetch(ctx context.Context, httpClient *http.Client, url, kind, externalID string) (string, error)
	Save(r io.Reader, kind, externalID string) (string, error)
}

// Service refreshes and replaces tracked-item cover art.
type Service struct {
	db         *gorm.DB
	tmdb       TMDB
	igdb       IGDB
	openlib    OpenLibrary
	images     ImageStore
	httpClient *http.Client
	imageBase  string
}

// Option customizes a Service.
type Option func(*Service)

// WithTMDBImageBase overrides the TMDB image CDN base (tests only).
func WithTMDBImageBase(u string) Option {
	return func(s *Service) { s.imageBase = u }
}

// WithHTTPClient overrides the HTTP client used for cover downloads (tests).
func WithHTTPClient(h *http.Client) Option {
	return func(s *Service) { s.httpClient = h }
}

// New wires the service. igdb may be nil to disable game refresh.
func New(gdb *gorm.DB, t TMDB, ig IGDB, ol OpenLibrary, imgs ImageStore, opts ...Option) *Service {
	s := &Service{
		db:         gdb,
		tmdb:       t,
		igdb:       ig,
		openlib:    ol,
		images:     imgs,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		imageBase:  tmdbImageBase,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Refresh re-pulls the latest cover for the user's tracked item from its
// upstream source and overwrites the cached image, returning the new relative
// artwork path. ErrNoArtwork is returned when the source has no cover.
func (s *Service) Refresh(ctx context.Context, userID, itemID uint) (string, error) {
	item, err := s.userItem(ctx, userID, itemID)
	if err != nil {
		return "", err
	}

	switch item.Type {
	case typeTV:
		return s.refreshShow(ctx, item.ExternalID)
	case typeMovie:
		return s.refreshMovie(ctx, item.ExternalID)
	case typeGame:
		return s.refreshGame(ctx, item.ExternalID)
	case typeBook:
		return s.refreshBook(ctx, item.ExternalID)
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupported, item.Type)
	}
}

// Upload stores the bytes read from r as the cover for the user's tracked item
// and returns the new relative artwork path. The caller must have already
// validated that r is an image.
func (s *Service) Upload(ctx context.Context, userID, itemID uint, r io.Reader) (string, error) {
	item, err := s.userItem(ctx, userID, itemID)
	if err != nil {
		return "", err
	}

	kind, err := kindFor(item.Type)
	if err != nil {
		return "", err
	}
	rel, err := s.images.Save(r, kind, item.ExternalID)
	if err != nil {
		return "", fmt.Errorf("artwork: save upload: %w", err)
	}
	if err := s.setCover(ctx, item.Type, item.ExternalID, rel); err != nil {
		return "", err
	}
	return rel, nil
}

// ── per-type refresh ──

func (s *Service) refreshShow(ctx context.Context, externalID string) (string, error) {
	tmdbID, err := strconv.Atoi(externalID)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrUnsupported, externalID)
	}
	detail, err := s.tmdb.GetShow(ctx, tmdbID)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if detail.PosterPath == "" {
		return "", ErrNoArtwork
	}
	rel, err := s.images.Fetch(ctx, s.httpClient, s.imageBase+detail.PosterPath, "tv", externalID)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if err := s.setCover(ctx, typeTV, externalID, rel); err != nil {
		return "", err
	}
	return rel, nil
}

func (s *Service) refreshMovie(ctx context.Context, externalID string) (string, error) {
	tmdbID, err := strconv.Atoi(externalID)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrUnsupported, externalID)
	}
	detail, err := s.tmdb.GetMovie(ctx, tmdbID)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if detail.PosterPath == "" {
		return "", ErrNoArtwork
	}
	rel, err := s.images.Fetch(ctx, s.httpClient, s.imageBase+detail.PosterPath, "movie", externalID)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if err := s.setCover(ctx, typeMovie, externalID, rel); err != nil {
		return "", err
	}
	return rel, nil
}

func (s *Service) refreshGame(ctx context.Context, externalID string) (string, error) {
	if s.igdb == nil {
		return "", ErrNoArtwork
	}
	var game models.Game
	var err error
	err = s.db.WithContext(ctx).Where("barcode = ?", externalID).First(&game).Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		if id, parseErr := strconv.Atoi(externalID); parseErr == nil {
			err = s.db.WithContext(ctx).Where("igdb_id = ?", id).First(&game).Error
		}
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrNoArtwork
		}
		return "", fmt.Errorf("artwork: load game %s: %w", externalID, err)
	}

	if game.IGDBID == 0 && game.Title != "" {
		results, serr := s.igdb.SearchGames(ctx, game.Title)
		if serr == nil && len(results) > 0 {
			var bestMatch *igdb.SearchResult
			for _, r := range results {
				if strings.EqualFold(r.Name, game.Title) {
					bestMatch = &r
					break
				}
			}
			if bestMatch == nil {
				bestMatch = &results[0]
			}
			game.IGDBID = bestMatch.ID
			if uerr := s.db.WithContext(ctx).Model(&game).Update("igdb_id", game.IGDBID).Error; uerr != nil {
				log.Printf("artwork: failed to save resolved IGDB ID for game %s: %v", externalID, uerr)
			}
		}
	}

	if game.IGDBID == 0 {
		return "", ErrNoArtwork
	}
	detail, err := s.igdb.GetGame(ctx, game.IGDBID)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if detail == nil || detail.CoverImageID == "" {
		return "", ErrNoArtwork
	}
	url := s.igdb.CoverURL(detail.CoverImageID, "")
	key := game.Barcode
	if key == "" {
		key = "igdb-" + strconv.Itoa(game.IGDBID)
	}
	rel, err := s.images.Fetch(ctx, s.httpClient, url, "game", key)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if err := s.setCover(ctx, typeGame, externalID, rel); err != nil {
		return "", err
	}
	return rel, nil
}

func (s *Service) refreshBook(ctx context.Context, isbn string) (string, error) {
	detail, err := s.openlib.GetByISBN(ctx, isbn)
	if err != nil {
		if errors.Is(err, openlibrary.ErrNotFound) {
			return "", ErrNoArtwork
		}
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	url := s.openlib.CoverURL(detail.CoverID, "L")
	if url == "" {
		return "", ErrNoArtwork
	}
	rel, err := s.images.Fetch(ctx, s.httpClient, url, "book", isbn)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	if err := s.setCover(ctx, typeBook, isbn, rel); err != nil {
		return "", err
	}
	return rel, nil
}

// ── shared helpers ──

// setCover writes the new relative artwork path onto the shared cache row for
// the media, updating the column that type stores its cover in.
func (s *Service) setCover(ctx context.Context, typ, externalID, rel string) error {
	db := s.db.WithContext(ctx)
	var res *gorm.DB
	switch typ {
	case typeTV:
		tmdbID, _ := strconv.Atoi(externalID)
		res = db.Model(&models.Show{}).Where("tmdb_id = ?", tmdbID).Update("poster_path", rel)
	case typeMovie:
		tmdbID, _ := strconv.Atoi(externalID)
		res = db.Model(&models.Movie{}).Where("tmdb_id = ?", tmdbID).Update("poster_path", rel)
	case typeGame:
		res = db.Model(&models.Game{}).Where("barcode = ?", externalID).Update("cover_path", rel)
		if res.Error == nil && res.RowsAffected == 0 {
			if id, parseErr := strconv.Atoi(externalID); parseErr == nil {
				res = db.Model(&models.Game{}).Where("igdb_id = ?", id).Update("cover_path", rel)
			}
		}
	case typeBook:
		res = db.Model(&models.Book{}).Where("isbn13 = ?", externalID).Update("cover_path", rel)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupported, typ)
	}
	if res.Error != nil {
		return fmt.Errorf("artwork: update %s cover: %w", typ, res.Error)
	}
	if res.RowsAffected == 0 {
		// No cache row yet (e.g. metadata was pruned). The image is written but
		// nothing references it — treat as "nothing to update", not fatal.
		return ErrNoArtwork
	}
	return nil
}

// kindFor maps a media type to the image-store "kind" subdirectory.
func kindFor(typ string) (string, error) {
	switch typ {
	case typeTV:
		return "tv", nil
	case typeMovie:
		return "movie", nil
	case typeGame:
		return "game", nil
	case typeBook:
		return "book", nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupported, typ)
	}
}

// userItem loads a tracking item scoped to the user; someone else's item is
// indistinguishable from a missing one (no existence leak).
func (s *Service) userItem(ctx context.Context, userID, itemID uint) (*models.TrackingItem, error) {
	var item models.TrackingItem
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", itemID, userID).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrItemNotFound
		}
		return nil, fmt.Errorf("artwork: load item %d: %w", itemID, err)
	}
	return &item, nil
}
