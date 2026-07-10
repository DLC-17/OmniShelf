// Package music implements album scanning, searching and tracking: a UPC/EAN
// barcode is resolved through Discogs to a shared Album cache row, or an album
// is found by name through MusicBrainz; either way it is linked to the user as
// a TrackingItem (Type "MUSIC"). It mirrors internal/games (barcode scan) and
// internal/movies (name search + add); the unified library/list/update/delete
// operations still live in the books service, which this package feeds by
// writing MUSIC tracking items. Albums are grouped by artist in the library.
//
// Every mutating operation is scoped by the caller-supplied userID, taken from
// the JWT — never from client input.
package music

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/discogs"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/musicbrainz"
)

// TypeMusic is the TrackingItem.Type for albums. It mirrors books.TypeMusic;
// kept here too so this package has no import cycle with books.
const TypeMusic = "MUSIC"

// Valid music statuses. A freshly-added album is LISTENING (the music-active
// verb, mirroring games' PLAYING); the shared PLAN_TO/COMPLETED/STOPPED apply
// too.
const (
	StatusListening = "LISTENING"
	StatusPlanTo    = "PLAN_TO"
	StatusCompleted = "COMPLETED"
	StatusStopped   = "STOPPED"
)

// Sentinel errors translated by the API layer into envelope responses.
var (
	// ErrInvalidBarcode means the scanned string is not a plausible 8–14 digit
	// UPC/EAN.
	ErrInvalidBarcode = errors.New("music: invalid barcode")
	// ErrNotFound means the metadata source has no album for the barcode/MBID.
	ErrNotFound = errors.New("music: no album found")
	// ErrUpstream means the lookup failed for a non-404 reason.
	ErrUpstream = errors.New("music: metadata service unavailable")
	// ErrUnconfigured means Discogs has no token, so barcode scans cannot run.
	ErrUnconfigured = errors.New("music: discogs not configured")
	// ErrAlbumNotFound means the referenced Album row does not exist.
	ErrAlbumNotFound = errors.New("music: album not found")
	// ErrAlreadyTracked means the user already tracks this album.
	ErrAlreadyTracked = errors.New("music: album already tracked")
	// ErrInvalidStatus means the status is not valid for an album.
	ErrInvalidStatus = errors.New("music: invalid status")
	// ErrInvalidQuery means an empty search query.
	ErrInvalidQuery = errors.New("music: empty search query")
	// ErrInvalidMBID means an empty MusicBrainz id.
	ErrInvalidMBID = errors.New("music: empty musicbrainz id")
)

// DiscogsClient is the slice of *discogs.Client the service needs; tests
// substitute a fake. A missing barcode must yield an error satisfying
// errors.Is(err, discogs.ErrNotFound).
type DiscogsClient interface {
	LookupByBarcode(ctx context.Context, barcode string) (*discogs.Release, error)
}

// MusicBrainzClient is the slice of *musicbrainz.Client the service needs;
// tests substitute a fake.
type MusicBrainzClient interface {
	Search(ctx context.Context, query string, limit int) ([]musicbrainz.ReleaseGroup, error)
	GetReleaseGroup(ctx context.Context, mbid string) (*musicbrainz.ReleaseGroup, error)
	CoverURL(mbid string, size int) string
}

// ImageStore is the slice of *images.Store the service needs; tests substitute
// a fake. Optional: when nil no cover is downloaded.
type ImageStore interface {
	Fetch(ctx context.Context, httpClient *http.Client, url, kind, externalID string) (string, error)
}

// Service implements album scanning, searching and tracking.
type Service struct {
	db         *gorm.DB
	discogs    DiscogsClient
	mb         MusicBrainzClient
	images     ImageStore
	httpClient *http.Client
}

// NewService wires the service. images may be nil to disable cover caching.
func NewService(gdb *gorm.DB, dc DiscogsClient, mb MusicBrainzClient, images ImageStore) *Service {
	return &Service{
		db:         gdb,
		discogs:    dc,
		mb:         mb,
		images:     images,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Scan resolves a barcode to an album through Discogs and upserts the shared
// Album cache row. DB-first: a barcode already cached is returned without a
// Discogs round-trip. A barcode unknown to Discogs yields ErrNotFound; an
// invalid code yields ErrInvalidBarcode; a missing token yields ErrUnconfigured.
func (s *Service) Scan(ctx context.Context, barcode string) (*models.Album, error) {
	norm, err := normalizeBarcode(barcode)
	if err != nil {
		return nil, err
	}

	var cached models.Album
	err = s.db.WithContext(ctx).Where(&models.Album{Barcode: norm}).First(&cached).Error
	if err == nil {
		return &cached, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("looking up cached album %s: %w", norm, err)
	}

	rel, err := s.discogs.LookupByBarcode(ctx, norm)
	if err != nil {
		switch {
		case errors.Is(err, discogs.ErrUnconfigured):
			return nil, ErrUnconfigured
		case errors.Is(err, discogs.ErrNotFound):
			return nil, fmt.Errorf("%w for barcode %s", ErrNotFound, norm)
		default:
			return nil, errors.Join(ErrUpstream, err)
		}
	}

	album := models.Album{
		ExternalID: discogsExternalID(rel.DiscogsID),
		Artist:     rel.Artist,
		Title:      rel.Title,
		Year:       rel.Year,
		Barcode:    norm,
		DiscogsID:  rel.DiscogsID,
	}
	s.cacheCover(ctx, &album, rel.CoverURL)
	if err := s.upsertAlbum(ctx, &album); err != nil {
		return nil, err
	}
	return &album, nil
}

// SearchResult is one MusicBrainz name-search hit.
type SearchResult struct {
	MBID   string
	Artist string
	Title  string
	Year   int
}

// Search finds albums by name through MusicBrainz.
func (s *Service) Search(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrInvalidQuery
	}
	groups, err := s.mb.Search(ctx, query, 12)
	if err != nil {
		return nil, errors.Join(ErrUpstream, err)
	}
	out := make([]SearchResult, 0, len(groups))
	for _, g := range groups {
		out = append(out, SearchResult{MBID: g.MBID, Artist: g.Artist, Title: g.Title, Year: g.Year})
	}
	return out, nil
}

// AddResult is what AddByMusicBrainz returns on success.
type AddResult struct {
	Album models.Album
	Item  models.TrackingItem
}

// AddByMusicBrainz looks a MusicBrainz release-group up (DB-first via the
// cached Album), caches its Cover Art Archive front cover best-effort, and
// creates the user's LISTENING TrackingItem. A duplicate returns the existing
// item alongside ErrAlreadyTracked so the API can answer 409.
func (s *Service) AddByMusicBrainz(ctx context.Context, userID uint, mbid, status string) (*AddResult, error) {
	mbid = strings.TrimSpace(mbid)
	if mbid == "" {
		return nil, ErrInvalidMBID
	}
	if status == "" {
		status = StatusListening
	}
	if !validStatus(status) {
		return nil, fmt.Errorf("%w: %q is not valid for music", ErrInvalidStatus, status)
	}

	externalID := mbExternalID(mbid)

	// DB-first: reuse a cached album without a MusicBrainz round-trip.
	var album models.Album
	cacheErr := s.db.WithContext(ctx).Where(&models.Album{ExternalID: externalID}).First(&album).Error
	if errors.Is(cacheErr, gorm.ErrRecordNotFound) {
		rg, err := s.mb.GetReleaseGroup(ctx, mbid)
		if err != nil {
			if errors.Is(err, musicbrainz.ErrNotFound) {
				return nil, fmt.Errorf("%w for mbid %s", ErrNotFound, mbid)
			}
			return nil, errors.Join(ErrUpstream, err)
		}
		album = models.Album{
			ExternalID:    externalID,
			Artist:        rg.Artist,
			Title:         rg.Title,
			Year:          rg.Year,
			MusicBrainzID: rg.MBID,
		}
		if url := s.mb.CoverURL(mbid, 500); url != "" {
			s.cacheCover(ctx, &album, url)
		}
		if err := s.upsertAlbum(ctx, &album); err != nil {
			return nil, err
		}
	} else if cacheErr != nil {
		return nil, fmt.Errorf("looking up cached album %s: %w", externalID, cacheErr)
	}

	item, err := s.track(ctx, userID, &album, status)
	if err != nil {
		if errors.Is(err, ErrAlreadyTracked) {
			return &AddResult{Album: album, Item: *item}, ErrAlreadyTracked
		}
		return nil, err
	}
	return &AddResult{Album: album, Item: *item}, nil
}

// Track creates the user's TrackingItem for a scanned album. A duplicate
// returns the existing item alongside ErrAlreadyTracked so the API can answer
// 409.
func (s *Service) Track(ctx context.Context, userID, albumID uint, status string) (*models.TrackingItem, error) {
	if status == "" {
		status = StatusListening
	}
	if !validStatus(status) {
		return nil, fmt.Errorf("%w: %q is not valid for music", ErrInvalidStatus, status)
	}

	var album models.Album
	if err := s.db.WithContext(ctx).First(&album, albumID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAlbumNotFound
		}
		return nil, fmt.Errorf("looking up album %d: %w", albumID, err)
	}
	return s.track(ctx, userID, &album, status)
}

// track links an album to the user, translating a unique-index collision into
// ErrAlreadyTracked with the existing row.
func (s *Service) track(ctx context.Context, userID uint, album *models.Album, status string) (*models.TrackingItem, error) {
	item := models.TrackingItem{
		UserID:     userID,
		Type:       TypeMusic,
		ExternalID: album.ExternalID,
		Title:      album.Title,
		Status:     status,
	}
	if err := s.db.WithContext(ctx).Create(&item).Error; err != nil {
		if isUniqueViolation(err) {
			var existing models.TrackingItem
			ferr := s.db.WithContext(ctx).
				Where("user_id = ? AND type = ? AND external_id = ?", userID, TypeMusic, album.ExternalID).
				First(&existing).Error
			if ferr != nil {
				return nil, fmt.Errorf("loading existing tracking item: %w", ferr)
			}
			return &existing, ErrAlreadyTracked
		}
		return nil, fmt.Errorf("creating tracking item: %w", err)
	}
	return &item, nil
}

// cacheCover downloads coverURL through the image store and sets album.CoverPath.
// It is best-effort: a nil store, empty URL or failed download logs and leaves
// CoverPath empty (the album is still saved with its metadata).
func (s *Service) cacheCover(ctx context.Context, album *models.Album, coverURL string) {
	if s.images == nil || coverURL == "" {
		return
	}
	path, err := s.images.Fetch(ctx, s.httpClient, coverURL, "music", coverKey(album.ExternalID))
	if err != nil {
		log.Printf("music: cover download for %s failed: %v", album.ExternalID, err)
		return
	}
	album.CoverPath = path
}

// upsertAlbum creates the Album row or refreshes an existing one for the same
// ExternalID. An already-cached cover is kept when the new download failed.
func (s *Service) upsertAlbum(ctx context.Context, album *models.Album) error {
	var existing models.Album
	err := s.db.WithContext(ctx).Where(&models.Album{ExternalID: album.ExternalID}).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := s.db.WithContext(ctx).Create(album).Error; err != nil {
			if isUniqueViolation(err) {
				return s.upsertAlbum(ctx, album)
			}
			return fmt.Errorf("creating album %s: %w", album.ExternalID, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("looking up album %s: %w", album.ExternalID, err)
	}

	album.ID = existing.ID
	if album.CoverPath == "" {
		album.CoverPath = existing.CoverPath
	}
	if err := s.db.WithContext(ctx).Save(album).Error; err != nil {
		return fmt.Errorf("updating album %s: %w", album.ExternalID, err)
	}
	return nil
}

// validStatus reports whether status is allowed for an album.
func validStatus(status string) bool {
	switch status {
	case StatusListening, StatusPlanTo, StatusCompleted, StatusStopped:
		return true
	default:
		return false
	}
}

// discogsExternalID / mbExternalID build the source-prefixed ExternalID reused
// as the MUSIC TrackingItem's ExternalID.
func discogsExternalID(discogsID int) string { return fmt.Sprintf("discogs:%d", discogsID) }
func mbExternalID(mbid string) string        { return "mb:" + mbid }

// coverKey turns an ExternalID into a filesystem-safe cover filename stem
// (the ":" separator is illegal in Windows filenames).
func coverKey(externalID string) string {
	return strings.ReplaceAll(externalID, ":", "_")
}

// normalizeBarcode strips hyphens/spaces and validates a plausible UPC/EAN:
// 8–14 digits. Mirrors internal/games.
func normalizeBarcode(barcode string) (string, error) {
	norm := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.TrimSpace(barcode))

	if len(norm) < 8 || len(norm) > 14 {
		return "", fmt.Errorf("%w: expected 8–14 digits, got %d", ErrInvalidBarcode, len(norm))
	}
	for _, r := range norm {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("%w: non-digit character", ErrInvalidBarcode)
		}
	}
	return norm, nil
}

// isUniqueViolation detects SQLite unique-index failures; glebarez/sqlite
// surfaces them as plain error strings.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint")
}
