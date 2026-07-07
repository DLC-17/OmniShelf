// Package games implements video-game scanning and tracking: a UPC/EAN barcode
// is resolved through ScanDex to a shared Game cache row, then linked to the
// user as a TrackingItem (Type "GAME"). It mirrors internal/books but for
// games; the unified library/list/update/delete operations still live in the
// books service, which this package feeds by writing GAME tracking items.
//
// Every mutating operation is scoped by the caller-supplied userID, taken from
// the JWT — never from client input.
package games

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/scandex"
)

// TypeGame is the TrackingItem.Type for games. It mirrors books.TypeGame; kept
// here too so this package has no import cycle with books.
const TypeGame = "GAME"

// Valid game statuses.
const (
	StatusPlanTo    = "PLAN_TO"
	StatusPlaying   = "PLAYING"
	StatusCompleted = "COMPLETED"
	StatusStopped   = "STOPPED"
)

// Sentinel errors translated by the API layer into envelope responses.
var (
	// ErrInvalidBarcode means the scanned string is not a plausible 8–14 digit
	// UPC/EAN.
	ErrInvalidBarcode = errors.New("games: invalid barcode")
	// ErrNotFound means ScanDex has no game for the barcode.
	ErrNotFound = errors.New("games: no game for barcode")
	// ErrUpstream means the ScanDex lookup failed for a non-404 reason.
	ErrUpstream = errors.New("games: metadata service unavailable")
	// ErrGameNotFound means the referenced Game row does not exist.
	ErrGameNotFound = errors.New("games: game not found")
	// ErrAlreadyTracked means the user already tracks this game.
	ErrAlreadyTracked = errors.New("games: game already tracked")
	// ErrInvalidStatus means the status is not valid for a game.
	ErrInvalidStatus = errors.New("games: invalid status")
)

// MetadataClient is the slice of *scandex.Client the service needs; tests
// substitute a fake. A missing barcode must yield an error satisfying
// errors.Is(err, scandex.ErrNotFound).
type MetadataClient interface {
	Lookup(ctx context.Context, barcode string) (*scandex.Game, error)
}

// Service implements game scanning and tracking.
type Service struct {
	db       *gorm.DB
	metadata MetadataClient
}

// NewService wires the service.
func NewService(gdb *gorm.DB, metadata MetadataClient) *Service {
	return &Service{db: gdb, metadata: metadata}
}

// Scan resolves a barcode to a game and upserts the shared Game cache row.
// DB-first: a barcode already cached is returned without a ScanDex round-trip.
// A barcode unknown to ScanDex yields ErrNotFound; an invalid code yields
// ErrInvalidBarcode.
func (s *Service) Scan(ctx context.Context, barcode string) (*models.Game, error) {
	norm, err := normalizeBarcode(barcode)
	if err != nil {
		return nil, err
	}

	var cached models.Game
	err = s.db.WithContext(ctx).Where(&models.Game{Barcode: norm}).First(&cached).Error
	if err == nil {
		return &cached, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("looking up cached game %s: %w", norm, err)
	}

	meta, err := s.metadata.Lookup(ctx, norm)
	if err != nil {
		switch {
		case errors.Is(err, scandex.ErrNotFound):
			return nil, fmt.Errorf("%w %s", ErrNotFound, norm)
		case errors.Is(err, scandex.ErrInvalidBarcode):
			return nil, fmt.Errorf("%w: %s", ErrInvalidBarcode, norm)
		default:
			return nil, errors.Join(ErrUpstream, err)
		}
	}

	game := models.Game{
		Barcode:  norm,
		Title:    meta.Title,
		Platform: meta.Platform,
		IGDBID:   meta.IGDBID,
	}
	if err := s.upsertGame(ctx, &game); err != nil {
		return nil, err
	}
	return &game, nil
}

// upsertGame creates the Game row or refreshes an existing one for the barcode.
func (s *Service) upsertGame(ctx context.Context, game *models.Game) error {
	var existing models.Game
	err := s.db.WithContext(ctx).Where(&models.Game{Barcode: game.Barcode}).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := s.db.WithContext(ctx).Create(game).Error; err != nil {
			if isUniqueViolation(err) {
				return s.upsertGame(ctx, game)
			}
			return fmt.Errorf("creating game %s: %w", game.Barcode, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("looking up game %s: %w", game.Barcode, err)
	}

	game.ID = existing.ID
	if game.CoverPath == "" {
		game.CoverPath = existing.CoverPath
	}
	if err := s.db.WithContext(ctx).Save(game).Error; err != nil {
		return fmt.Errorf("updating game %s: %w", game.Barcode, err)
	}
	return nil
}

// Track creates the user's TrackingItem for a scanned game. A duplicate returns
// the existing item alongside ErrAlreadyTracked so the API can answer 409.
func (s *Service) Track(ctx context.Context, userID, gameID uint, status string) (*models.TrackingItem, error) {
	if !validStatus(status) {
		return nil, fmt.Errorf("%w: %q is not valid for games", ErrInvalidStatus, status)
	}

	var game models.Game
	if err := s.db.WithContext(ctx).First(&game, gameID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGameNotFound
		}
		return nil, fmt.Errorf("looking up game %d: %w", gameID, err)
	}

	item := models.TrackingItem{
		UserID:     userID,
		Type:       TypeGame,
		ExternalID: game.Barcode,
		Title:      game.Title,
		Status:     status,
	}
	if err := s.db.WithContext(ctx).Create(&item).Error; err != nil {
		if isUniqueViolation(err) {
			var existing models.TrackingItem
			ferr := s.db.WithContext(ctx).
				Where("user_id = ? AND type = ? AND external_id = ?", userID, TypeGame, game.Barcode).
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

// validStatus reports whether status is allowed for a game.
func validStatus(status string) bool {
	switch status {
	case StatusPlanTo, StatusPlaying, StatusCompleted, StatusStopped:
		return true
	default:
		return false
	}
}

// normalizeBarcode strips hyphens/spaces and validates a plausible UPC/EAN:
// 8–14 digits. ScanDex enforces the same range server-side; validating here
// keeps a bad scan from spending a network round-trip.
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
