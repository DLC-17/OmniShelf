package books

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// Note-service sentinel errors translated by the API layer into envelopes.
var (
	// ErrEmptyNote means AddNote was called with a blank body.
	ErrEmptyNote = errors.New("books: note body is empty")
	// ErrNoteNotFound means the referenced note does not exist for this user
	// (including notes owned by someone else — existence is not leaked).
	ErrNoteNotFound = errors.New("books: note not found")
)

// maxNoteBody caps a single journal entry so one note cannot balloon the DB.
const maxNoteBody = 10000

// AddNote appends a timestamped journal entry to the user's book item. The item
// must belong to the caller and be a BOOK (notes are a book-only feature);
// otherwise ErrItemNotFound is returned so a foreign or non-book item is
// indistinguishable from a missing one. A blank body yields ErrEmptyNote.
func (s *Service) AddNote(ctx context.Context, userID, itemID uint, body string) (*models.BookNote, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, ErrEmptyNote
	}
	if len(body) > maxNoteBody {
		body = body[:maxNoteBody]
	}

	if _, err := s.userBookItem(ctx, userID, itemID); err != nil {
		return nil, err
	}

	note := models.BookNote{UserID: userID, ItemID: itemID, Body: body}
	if err := s.db.WithContext(ctx).Create(&note).Error; err != nil {
		return nil, fmt.Errorf("creating note for item %d: %w", itemID, err)
	}
	return &note, nil
}

// ListNotes returns the user's journal entries for their book item, newest
// first. The item must belong to the caller (else ErrItemNotFound).
func (s *Service) ListNotes(ctx context.Context, userID, itemID uint) ([]models.BookNote, error) {
	if _, err := s.userBookItem(ctx, userID, itemID); err != nil {
		return nil, err
	}

	notes := []models.BookNote{}
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND item_id = ?", userID, itemID).
		Order("created_at DESC, id DESC").
		Find(&notes).Error
	if err != nil {
		return nil, fmt.Errorf("listing notes for item %d: %w", itemID, err)
	}
	return notes, nil
}

// DeleteNote removes one journal entry. Both the note and its book item must
// belong to the caller; a foreign or missing note yields ErrNoteNotFound.
func (s *Service) DeleteNote(ctx context.Context, userID, itemID, noteID uint) error {
	if _, err := s.userBookItem(ctx, userID, itemID); err != nil {
		return err
	}

	res := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND item_id = ?", noteID, userID, itemID).
		Delete(&models.BookNote{})
	if res.Error != nil {
		return fmt.Errorf("deleting note %d: %w", noteID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNoteNotFound
	}
	return nil
}

// userBookItem loads a tracking item scoped to the user and guards that it is a
// BOOK. Someone else's item, a missing item, or a non-book item is
// indistinguishable (no existence leak), all surfacing ErrItemNotFound.
func (s *Service) userBookItem(ctx context.Context, userID, itemID uint) (*models.TrackingItem, error) {
	item, err := s.userItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}
	if item.Type != TypeBook {
		return nil, ErrItemNotFound
	}
	return item, nil
}
