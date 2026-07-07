// Package books implements the book-tracking and library service layer:
// ISBN scan → OpenLibrary lookup → shared Book cache row + cached cover,
// per-user TrackingItems, and the unified library operations (list, update
// status/progress, untrack). Handlers in internal/api stay thin and translate
// this package's sentinel errors into the standard envelope.
//
// Every mutating operation is scoped by the caller-supplied userID, which
// handlers take from the JWT — never from client input. Items
// belonging to another user surface as ErrItemNotFound so their existence is
// not leaked.
package books

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/openlibrary"
)

// Media types stored in TrackingItem.Type.
const (
	TypeTV   = "TV"
	TypeBook = "BOOK"
)

// Tracking statuses. Books use READING; TV uses WATCHING.
const (
	StatusWatching  = "WATCHING"
	StatusReading   = "READING"
	StatusCompleted = "COMPLETED"
	StatusPlanTo    = "PLAN_TO"
)

// Sentinel errors translated by the API layer into envelope responses.
var (
	// ErrInvalidISBN means the scanned string is not ISBN-13 shaped.
	ErrInvalidISBN = errors.New("books: invalid ISBN-13")
	// ErrNotFound means OpenLibrary has no record for the ISBN (E4).
	ErrNotFound = errors.New("books: no record for ISBN")
	// ErrUpstream means the metadata lookup failed for a non-404 reason.
	ErrUpstream = errors.New("books: metadata service unavailable")
	// ErrBookNotFound means the referenced Book row does not exist.
	ErrBookNotFound = errors.New("books: book not found")
	// ErrAlreadyTracked means the user already tracks this media (E16).
	ErrAlreadyTracked = errors.New("books: item already tracked")
	// ErrInvalidStatus means the status is not valid for the media type.
	ErrInvalidStatus = errors.New("books: invalid status")
	// ErrInvalidProgress means progress is negative or set on a TV item
	// (TV progress is derived from EpisodeWatch counts, never stored).
	ErrInvalidProgress = errors.New("books: invalid progress")
	// ErrEmptyUpdate means a PATCH carried neither status nor progress.
	ErrEmptyUpdate = errors.New("books: no fields to update")
	// ErrInvalidFilter means an unknown type/status library filter value.
	ErrInvalidFilter = errors.New("books: invalid filter")
	// ErrItemNotFound means the tracking item does not exist for this user
	// (including items owned by someone else — existence is not leaked).
	ErrItemNotFound = errors.New("books: tracking item not found")
)

// MetadataClient is the slice of *openlibrary.Client the service needs;
// tests substitute a fake. A missing ISBN must yield an error satisfying
// errors.Is(err, openlibrary.ErrNotFound).
type MetadataClient interface {
	GetByISBN(ctx context.Context, isbn string) (*openlibrary.Book, error)
	CoverURL(coverID int, size string) string
}

// ImageStore is the slice of *images.Store the service needs; tests
// substitute a fake.
type ImageStore interface {
	Fetch(ctx context.Context, httpClient *http.Client, url, kind, externalID string) (string, error)
}

// Service implements book scanning/tracking and library operations.
type Service struct {
	db         *gorm.DB
	metadata   MetadataClient
	images     ImageStore
	httpClient *http.Client
}

// NewService wires the service. The internal HTTP client is used only for
// cover downloads through the image store.
func NewService(gdb *gorm.DB, metadata MetadataClient, images ImageStore) *Service {
	return &Service{
		db:         gdb,
		metadata:   metadata,
		images:     images,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Scan looks an ISBN up on OpenLibrary and upserts the shared Book cache row.
// Partial metadata is saved as-is; a failed cover download
// is logged and leaves CoverPath empty for the nightly retry. An ISBN
// unknown to OpenLibrary yields ErrNotFound.
func (s *Service) Scan(ctx context.Context, isbn string) (*models.Book, error) {
	norm, err := normalizeISBN13(isbn)
	if err != nil {
		return nil, err
	}

	// DB-first: if this ISBN is already in the shared Book cache, return it
	// without an OpenLibrary round-trip (and without spending its rate limit).
	var cached models.Book
	err = s.db.WithContext(ctx).Where(&models.Book{ISBN13: norm}).First(&cached).Error
	if err == nil {
		return &cached, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("looking up cached book %s: %w", norm, err)
	}

	meta, err := s.metadata.GetByISBN(ctx, norm)
	if err != nil {
		if errors.Is(err, openlibrary.ErrNotFound) {
			return nil, fmt.Errorf("%w %s", ErrNotFound, norm)
		}
		return nil, errors.Join(ErrUpstream, err)
	}

	isbn13 := meta.ISBN13
	if isbn13 == "" {
		isbn13 = norm
	}

	// Cover download is best-effort: failure never blocks tracking (E13).
	coverPath := ""
	if url := s.metadata.CoverURL(meta.CoverID, "L"); url != "" {
		coverPath, err = s.images.Fetch(ctx, s.httpClient, url, "book", isbn13)
		if err != nil {
			log.Printf("books: cover download for %s failed (nightly sync will retry): %v", isbn13, err)
			coverPath = ""
		}
	}

	book := models.Book{
		ISBN13:    isbn13,
		Title:     meta.Title,
		Authors:   strings.Join(meta.Authors, ", "),
		CoverPath: coverPath,
		PageCount: meta.PageCount,
	}
	if err := s.upsertBook(ctx, &book); err != nil {
		return nil, err
	}
	return &book, nil
}

// upsertBook creates the Book row or refreshes an existing one for the same
// ISBN. An already-cached cover is kept when the new download failed.
func (s *Service) upsertBook(ctx context.Context, book *models.Book) error {
	var existing models.Book
	err := s.db.WithContext(ctx).Where(&models.Book{ISBN13: book.ISBN13}).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := s.db.WithContext(ctx).Create(book).Error; err != nil {
			// Concurrent scan of the same ISBN: the unique index makes one
			// Create lose; fall back to the winner's row.
			if isUniqueViolation(err) {
				return s.upsertBook(ctx, book)
			}
			return fmt.Errorf("creating book %s: %w", book.ISBN13, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("looking up book %s: %w", book.ISBN13, err)
	}

	book.ID = existing.ID
	if book.CoverPath == "" {
		book.CoverPath = existing.CoverPath
	}
	if err := s.db.WithContext(ctx).Save(book).Error; err != nil {
		return fmt.Errorf("updating book %s: %w", book.ISBN13, err)
	}
	return nil
}

// Track creates the user's TrackingItem for a scanned book. A duplicate returns the existing item
// alongside ErrAlreadyTracked so the API can answer 409 with it.
func (s *Service) Track(ctx context.Context, userID, bookID uint, status string) (*models.TrackingItem, error) {
	if !validStatus(TypeBook, status) {
		return nil, fmt.Errorf("%w: %q is not valid for books", ErrInvalidStatus, status)
	}

	var book models.Book
	if err := s.db.WithContext(ctx).First(&book, bookID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBookNotFound
		}
		return nil, fmt.Errorf("looking up book %d: %w", bookID, err)
	}

	item := models.TrackingItem{
		UserID:     userID,
		Type:       TypeBook,
		ExternalID: book.ISBN13,
		Title:      book.Title,
		Status:     status,
	}
	if err := s.db.WithContext(ctx).Create(&item).Error; err != nil {
		if isUniqueViolation(err) {
			var existing models.TrackingItem
			ferr := s.db.WithContext(ctx).
				Where("user_id = ? AND type = ? AND external_id = ?", userID, TypeBook, book.ISBN13).
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

// ListItems returns the user's tracking items, optionally filtered by type
// ("TV"/"BOOK") and status, newest activity first.
func (s *Service) ListItems(ctx context.Context, userID uint, typ, status string) ([]models.TrackingItem, error) {
	if typ != "" && typ != TypeTV && typ != TypeBook {
		return nil, fmt.Errorf("%w: unknown type %q", ErrInvalidFilter, typ)
	}
	if status != "" && !isKnownStatus(status) {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalidFilter, status)
	}

	q := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}

	items := []models.TrackingItem{}
	if err := q.Order("updated_at DESC, id DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("listing items for user %d: %w", userID, err)
	}
	return items, nil
}

// UpdateItem patches status and/or progress on the user's tracking item.
// Status must be valid for the item's media type; progress is a
// page number and only meaningful for books (TV progress is derived from
// EpisodeWatch rows, never stored).
func (s *Service) UpdateItem(ctx context.Context, userID, itemID uint, status *string, progress *int) (*models.TrackingItem, error) {
	if status == nil && progress == nil {
		return nil, ErrEmptyUpdate
	}

	item, err := s.userItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}

	if status != nil {
		if !validStatus(item.Type, *status) {
			return nil, fmt.Errorf("%w: %q is not valid for type %s", ErrInvalidStatus, *status, item.Type)
		}
		item.Status = *status
	}
	if progress != nil {
		if item.Type != TypeBook {
			return nil, fmt.Errorf("%w: progress is only stored for books", ErrInvalidProgress)
		}
		if *progress < 0 {
			return nil, fmt.Errorf("%w: page number must not be negative", ErrInvalidProgress)
		}
		item.Progress = *progress
	}

	if err := s.db.WithContext(ctx).Save(item).Error; err != nil {
		return nil, fmt.Errorf("updating item %d: %w", itemID, err)
	}
	return item, nil
}

// DeleteItem untracks an item for the user. For TV items the
// user's EpisodeWatch rows for that show are removed too; shared Show, Book,
// and Episode metadata is always kept.
func (s *Service) DeleteItem(ctx context.Context, userID, itemID uint) error {
	item, err := s.userItem(ctx, userID, itemID)
	if err != nil {
		return err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if item.Type == TypeTV {
			if err := deleteShowWatches(tx, userID, item.ExternalID); err != nil {
				return err
			}
		}
		if err := tx.Delete(&models.TrackingItem{}, item.ID).Error; err != nil {
			return fmt.Errorf("deleting item %d: %w", item.ID, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("untracking item %d: %w", itemID, err)
	}
	return nil
}

// deleteShowWatches removes one user's EpisodeWatch rows for the show with
// the given TMDB ID (stored as TrackingItem.ExternalID). Episodes and the
// Show row itself are shared metadata and stay.
func deleteShowWatches(tx *gorm.DB, userID uint, externalID string) error {
	tmdbID, err := strconv.Atoi(externalID)
	if err != nil {
		// Malformed external ID: nothing to prune, still allow untracking.
		return nil
	}
	var show models.Show
	if err := tx.Where(&models.Show{TMDBID: tmdbID}, "TMDBID").First(&show).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("looking up show %d: %w", tmdbID, err)
	}
	episodeIDs := tx.Model(&models.Episode{}).Select("id").Where("show_id = ?", show.ID)
	if err := tx.Where("user_id = ? AND episode_id IN (?)", userID, episodeIDs).
		Delete(&models.EpisodeWatch{}).Error; err != nil {
		return fmt.Errorf("deleting episode watches: %w", err)
	}
	return nil
}

// userItem loads a tracking item scoped to the user. Someone else's item is
// indistinguishable from a missing one (no existence leak).
func (s *Service) userItem(ctx context.Context, userID, itemID uint) (*models.TrackingItem, error) {
	var item models.TrackingItem
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", itemID, userID).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrItemNotFound
		}
		return nil, fmt.Errorf("looking up item %d: %w", itemID, err)
	}
	return &item, nil
}

// validStatus reports whether status is allowed for the media type:
// TV → WATCHING/COMPLETED/PLAN_TO, BOOK → READING/COMPLETED/PLAN_TO.
func validStatus(typ, status string) bool {
	switch status {
	case StatusCompleted, StatusPlanTo:
		return true
	case StatusWatching:
		return typ == TypeTV
	case StatusReading:
		return typ == TypeBook
	default:
		return false
	}
}

// isKnownStatus reports whether status is any valid tracking status (used
// for the type-agnostic library filter).
func isKnownStatus(status string) bool {
	return validStatus(TypeTV, status) || validStatus(TypeBook, status)
}

// normalizeISBN13 strips hyphens/spaces and validates ISBN-13 shape:
// exactly 13 digits with a 978/979 Bookland prefix.
func normalizeISBN13(isbn string) (string, error) {
	norm := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.TrimSpace(isbn))

	if len(norm) != 13 {
		return "", fmt.Errorf("%w: expected 13 digits, got %d", ErrInvalidISBN, len(norm))
	}
	for _, r := range norm {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("%w: non-digit character", ErrInvalidISBN)
		}
	}
	if !strings.HasPrefix(norm, "978") && !strings.HasPrefix(norm, "979") {
		return "", fmt.Errorf("%w: missing 978/979 prefix", ErrInvalidISBN)
	}
	return norm, nil
}

// isUniqueViolation detects SQLite unique-index failures; glebarez/sqlite
// surfaces them as plain error strings (no GORM error translation).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint")
}
