package books

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/openlibrary"
)

// trackedBook scans testISBN and tracks it for userID, returning the item ID.
func trackedBook(t *testing.T, svc *Service, userID uint) uint {
	t.Helper()
	ctx := context.Background()
	book, err := svc.Scan(ctx, testISBN)
	require.NoError(t, err)
	item, err := svc.Track(ctx, userID, book.ID, StatusReading)
	require.NoError(t, err)
	return item.ID
}

func TestAddAndListNotesNewestFirst(t *testing.T) {
	meta := &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}
	svc, _ := newTestService(t, meta, &fakeImages{})
	ctx := context.Background()
	itemID := trackedBook(t, svc, 1)

	first, err := svc.AddNote(ctx, 1, itemID, "  started chapter one  ")
	require.NoError(t, err)
	assert.Equal(t, "started chapter one", first.Body, "body is trimmed")
	second, err := svc.AddNote(ctx, 1, itemID, "finished it")
	require.NoError(t, err)

	notes, err := svc.ListNotes(ctx, 1, itemID)
	require.NoError(t, err)
	require.Len(t, notes, 2)
	// Newest first: the tiebreaker on id keeps ordering stable even when the
	// two rows share a created_at second.
	assert.Equal(t, second.ID, notes[0].ID)
	assert.Equal(t, first.ID, notes[1].ID)
}

func TestAddNoteEmptyBodyRejected(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}, &fakeImages{})
	itemID := trackedBook(t, svc, 1)

	for _, body := range []string{"", "   ", "\n\t"} {
		_, err := svc.AddNote(context.Background(), 1, itemID, body)
		assert.ErrorIs(t, err, ErrEmptyNote, "body %q", body)
	}
}

// A user must not attach notes to (or read notes on) another user's item, nor a
// nonexistent one; both surface ErrItemNotFound (no existence leak).
func TestNotesOwnershipGuard(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}, &fakeImages{})
	ctx := context.Background()
	itemID := trackedBook(t, svc, 1) // owned by user 1

	_, err := svc.AddNote(ctx, 2, itemID, "sneaky")
	assert.ErrorIs(t, err, ErrItemNotFound)
	_, err = svc.ListNotes(ctx, 2, itemID)
	assert.ErrorIs(t, err, ErrItemNotFound)
	_, err = svc.AddNote(ctx, 1, 9999, "no such item")
	assert.ErrorIs(t, err, ErrItemNotFound)
}

// Notes are a book-only feature: a non-book item rejects note operations as if
// it did not exist.
func TestNotesRejectNonBookItem(t *testing.T) {
	svc, gdb := newTestService(t, &fakeMetadata{}, &fakeImages{})
	tv := models.TrackingItem{UserID: 1, Type: TypeTV, ExternalID: "1399", Title: "GoT", Status: StatusWatching}
	require.NoError(t, gdb.Create(&tv).Error)

	_, err := svc.AddNote(context.Background(), 1, tv.ID, "no notes on TV")
	assert.ErrorIs(t, err, ErrItemNotFound)
}

func TestDeleteNoteScopedToOwner(t *testing.T) {
	svc, gdb := newTestService(t, &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}, &fakeImages{})
	ctx := context.Background()
	itemID := trackedBook(t, svc, 1)
	note, err := svc.AddNote(ctx, 1, itemID, "keep me")
	require.NoError(t, err)

	// A foreign user cannot delete it.
	err = svc.DeleteNote(ctx, 2, itemID, note.ID)
	assert.ErrorIs(t, err, ErrItemNotFound)
	// A missing note id for the owner is a not-found.
	err = svc.DeleteNote(ctx, 1, itemID, 9999)
	assert.ErrorIs(t, err, ErrNoteNotFound)

	// The owner deletes it; a second delete reports not-found.
	require.NoError(t, svc.DeleteNote(ctx, 1, itemID, note.ID))
	err = svc.DeleteNote(ctx, 1, itemID, note.ID)
	assert.ErrorIs(t, err, ErrNoteNotFound)

	var count int64
	require.NoError(t, gdb.Model(&models.BookNote{}).Count(&count).Error)
	assert.Zero(t, count)
}

// Untracking a book prunes its journal entries.
func TestDeleteBookItemPrunesNotes(t *testing.T) {
	svc, gdb := newTestService(t, &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}, &fakeImages{})
	ctx := context.Background()
	itemID := trackedBook(t, svc, 1)
	_, err := svc.AddNote(ctx, 1, itemID, "will be pruned")
	require.NoError(t, err)

	require.NoError(t, svc.DeleteItem(ctx, 1, itemID))

	var count int64
	require.NoError(t, gdb.Model(&models.BookNote{}).Count(&count).Error)
	assert.Zero(t, count)
}
