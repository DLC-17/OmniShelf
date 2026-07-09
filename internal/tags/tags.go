// Package tags is the shared, media-type-agnostic tag surface. Tags are
// source-derived keywords/genres (TMDB keywords, IGDB genres/keywords,
// OpenLibrary subjects) — never user-created. Every media domain (tv, movies,
// games, books) persists a cache row's tags through this package during
// enrichment, and the library service reads them back for the DTO.
//
// A Tag row is shared across items (one per normalized slug); a MediaTag row
// links a tag to one shared metadata cache row (Show/Movie/Game/Book) by its
// primary key. This join is the query surface future per-type filters (#13) and
// search (#14) build on.
package tags

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// Media types stored in MediaTag.MediaType; they mirror TrackingItem.Type.
const (
	TypeTV    = "TV"
	TypeMovie = "MOVIE"
	TypeGame  = "GAME"
	TypeBook  = "BOOK"
)

// Store persists source-derived tags and answers tag queries. It wraps the
// shared *gorm.DB, so any service can build one on the fly from its own handle
// (tags.NewStore(s.db)) without a constructor-signature change.
type Store struct {
	db *gorm.DB
}

// NewStore wraps a *gorm.DB as a tag store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Slugify normalizes a tag name into its lookup key: lower-cased, with each run
// of non-alphanumeric characters collapsed to a single hyphen and the ends
// trimmed. "Time Travel" and "time-travel" both map to "time-travel", so a
// keyword shared across sources yields one Tag row.
func Slugify(name string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevHyphen = false
			continue
		}
		if !prevHyphen && b.Len() > 0 {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// Set replaces the tags on one media cache row with the given names. Tag rows
// are created on demand (deduped by slug) and reused across items. Blank or
// duplicate names are ignored. It is best-effort by contract of its callers:
// enrichment paths log and continue rather than fail an add on a tag error.
func (s *Store) Set(ctx context.Context, mediaType string, mediaID uint, names []string) error {
	if mediaID == 0 {
		return fmt.Errorf("tags: media id must be non-zero")
	}

	// Resolve each unique slug to a Tag id, creating missing rows.
	tagIDs := make([]uint, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		slug := Slugify(name)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		id, err := s.upsertTag(ctx, strings.TrimSpace(name), slug)
		if err != nil {
			return err
		}
		tagIDs = append(tagIDs, id)
	}

	// Replace the item's links in one transaction so a partial write never
	// leaves a stale set.
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("media_type = ? AND media_id = ?", mediaType, mediaID).
			Delete(&models.MediaTag{}).Error; err != nil {
			return fmt.Errorf("tags: clearing links for %s %d: %w", mediaType, mediaID, err)
		}
		if len(tagIDs) == 0 {
			return nil
		}
		links := make([]models.MediaTag, 0, len(tagIDs))
		for _, id := range tagIDs {
			links = append(links, models.MediaTag{TagID: id, MediaType: mediaType, MediaID: mediaID})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&links).Error; err != nil {
			return fmt.Errorf("tags: linking %s %d: %w", mediaType, mediaID, err)
		}
		return nil
	})
}

// upsertTag returns the id of the Tag for slug, creating it when absent. A
// concurrent create is resolved by re-reading the winning row.
func (s *Store) upsertTag(ctx context.Context, name, slug string) (uint, error) {
	tag := models.Tag{Name: name, Slug: slug}
	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&tag).Error
	if err != nil {
		return 0, fmt.Errorf("tags: upsert %q: %w", slug, err)
	}
	if tag.ID != 0 {
		return tag.ID, nil
	}
	// OnConflict DoNothing left ID unset (row already existed): read it back.
	var existing models.Tag
	if err := s.db.WithContext(ctx).Where("slug = ?", slug).First(&existing).Error; err != nil {
		return 0, fmt.Errorf("tags: load existing %q: %w", slug, err)
	}
	return existing.ID, nil
}

// ForMedia returns the tag names for each mediaID of one media type, so the
// library DTO can attach tags to a batch of items in a single query. Names are
// sorted so the DTO output is stable. Items with no tags are simply absent from
// the map.
func (s *Store) ForMedia(ctx context.Context, mediaType string, mediaIDs []uint) (map[uint][]string, error) {
	out := map[uint][]string{}
	if len(mediaIDs) == 0 {
		return out, nil
	}

	var rows []struct {
		MediaID uint
		Name    string
	}
	if err := s.db.WithContext(ctx).
		Model(&models.MediaTag{}).
		Select("media_tags.media_id AS media_id, tags.name AS name").
		Joins("JOIN tags ON tags.id = media_tags.tag_id").
		Where("media_tags.media_type = ? AND media_tags.media_id IN ?", mediaType, mediaIDs).
		Order("tags.name COLLATE NOCASE").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("tags: load for %s: %w", mediaType, err)
	}
	for _, r := range rows {
		out[r.MediaID] = append(out[r.MediaID], r.Name)
	}
	return out, nil
}

// MediaIDs returns the primary keys of the cache rows of one media type that
// carry the tag with the given slug. This is the reverse of ForMedia and the
// query surface future per-type tag filters (#13) and search (#14) build on
// (join these ids back to Show/Movie/Game/Book by primary key). An unknown slug
// yields an empty slice, not an error.
func (s *Store) MediaIDs(ctx context.Context, mediaType, slug string) ([]uint, error) {
	var ids []uint
	if err := s.db.WithContext(ctx).
		Model(&models.MediaTag{}).
		Select("media_tags.media_id").
		Joins("JOIN tags ON tags.id = media_tags.tag_id").
		Where("media_tags.media_type = ? AND tags.slug = ?", mediaType, slug).
		Order("media_tags.media_id").
		Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("tags: media ids for %s/%s: %w", mediaType, slug, err)
	}
	return ids, nil
}
