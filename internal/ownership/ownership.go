// Package ownership is the shared, media-type-agnostic ownership-format surface.
// Ownership records which formats a user holds a tracked item in — for games
// {Physical, GOG}, and (via #11) for music {Vinyl, CD}. It is a MULTI-select
// over a FIXED option set per media type, so an item can carry several rows.
//
// Unlike tags (source-derived metadata shared across users, keyed by the shared
// cache row), ownership is a per-user fact about a tracked copy, so it keys off
// the TrackingItem primary key. The store mirrors the tags.Store pattern: it
// wraps the shared *gorm.DB, so any service can build one on the fly from its
// own handle (ownership.NewStore(s.db)) without a constructor change.
//
// Reuse note for #11 (music): add the media type and its allowed formats to
// allowedFormats below; nothing else in this package or the DTO plumbing needs
// to change.
package ownership

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// Media types stored in OwnershipFormat.MediaType; they mirror TrackingItem.Type.
const (
	TypeGame  = "GAME"
	TypeMusic = "MUSIC"
)

// Ownership format values. Games use Physical/GOG; music (#11) uses Vinyl/CD.
const (
	FormatPhysical = "Physical"
	FormatGOG      = "GOG"
	FormatVinyl    = "Vinyl"
	FormatCD       = "CD"
)

// allowedFormats is the FIXED option set per media type. It is the single
// extension point: #11 adds TypeMusic here. The order is canonical and drives
// the order formats are returned in for the DTO, so display is stable.
var allowedFormats = map[string][]string{
	TypeGame: {FormatPhysical, FormatGOG},
}

// ErrInvalidFormat means a requested format is not in the allowed set for the
// media type (including an unknown/unsupported media type). Callers translate
// it into a 400.
var ErrInvalidFormat = errors.New("ownership: invalid format for media type")

// Store persists ownership formats and answers ownership queries. It wraps the
// shared *gorm.DB.
type Store struct {
	db *gorm.DB
}

// NewStore wraps a *gorm.DB as an ownership store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// AllowedFormats returns the fixed option set for a media type (canonical
// order), or nil for an unsupported type. The returned slice is a copy, safe to
// mutate.
func AllowedFormats(mediaType string) []string {
	src, ok := allowedFormats[mediaType]
	if !ok {
		return nil
	}
	return append([]string(nil), src...)
}

// Set replaces the ownership formats on one tracking item with the given set.
// Formats are validated against the allowed set for the media type; an unknown
// media type or an out-of-set format yields ErrInvalidFormat and no write.
// Blank and duplicate formats are ignored. An empty (post-clean) set clears the
// item's ownership.
func (s *Store) Set(ctx context.Context, mediaType string, itemID uint, formats []string) error {
	if itemID == 0 {
		return fmt.Errorf("ownership: item id must be non-zero")
	}
	allowed, ok := allowedFormats[mediaType]
	if !ok {
		return fmt.Errorf("%w: %s has no ownership formats", ErrInvalidFormat, mediaType)
	}

	// Validate and dedupe against the allowed set, preserving canonical order.
	want := map[string]bool{}
	for _, f := range formats {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !contains(allowed, f) {
			return fmt.Errorf("%w: %q not allowed for %s", ErrInvalidFormat, f, mediaType)
		}
		want[f] = true
	}
	rows := make([]models.OwnershipFormat, 0, len(want))
	for _, f := range allowed {
		if want[f] {
			rows = append(rows, models.OwnershipFormat{MediaType: mediaType, ItemID: itemID, Format: f})
		}
	}

	// Replace the item's rows in one transaction so a partial write never leaves
	// a stale set.
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("media_type = ? AND item_id = ?", mediaType, itemID).
			Delete(&models.OwnershipFormat{}).Error; err != nil {
			return fmt.Errorf("ownership: clearing formats for %s %d: %w", mediaType, itemID, err)
		}
		if len(rows) == 0 {
			return nil
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return fmt.Errorf("ownership: writing formats for %s %d: %w", mediaType, itemID, err)
		}
		return nil
	})
}

// ForItems returns the ownership formats for each itemID of one media type, so
// the library DTO can attach ownership to a batch of items in a single query.
// Formats are returned in the allowed set's canonical order (stable). Items with
// no formats are simply absent from the map.
func (s *Store) ForItems(ctx context.Context, mediaType string, itemIDs []uint) (map[uint][]string, error) {
	out := map[uint][]string{}
	if len(itemIDs) == 0 {
		return out, nil
	}

	var rows []models.OwnershipFormat
	if err := s.db.WithContext(ctx).
		Where("media_type = ? AND item_id IN ?", mediaType, itemIDs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ownership: load for %s: %w", mediaType, err)
	}

	// Gather per item, then order by the allowed set so output is deterministic
	// regardless of row insertion order.
	held := map[uint]map[string]bool{}
	for _, r := range rows {
		if held[r.ItemID] == nil {
			held[r.ItemID] = map[string]bool{}
		}
		held[r.ItemID][r.Format] = true
	}
	order := allowedFormats[mediaType]
	for id, set := range held {
		for _, f := range order {
			if set[f] {
				out[id] = append(out[id], f)
			}
		}
	}
	return out, nil
}

// contains reports whether s is in xs.
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
