package ownership

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gdb
}

func TestAllowedFormats(t *testing.T) {
	assert.Equal(t, []string{FormatPhysical, FormatGOG}, AllowedFormats(TypeGame))
	assert.Nil(t, AllowedFormats("BOOK")) // unsupported type

	// The returned slice is a copy: mutating it must not corrupt the source.
	got := AllowedFormats(TypeGame)
	got[0] = "mutated"
	assert.Equal(t, []string{FormatPhysical, FormatGOG}, AllowedFormats(TypeGame))
}

// Set writes one row per selected format and ForItems returns them in canonical
// order regardless of the order they were requested in; blanks/dupes collapse.
func TestSetAndForItems(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, TypeGame, 1, []string{"GOG", "Physical", "GOG", ""}))

	var count int64
	require.NoError(t, gdb.Model(&models.OwnershipFormat{}).Count(&count).Error)
	assert.Equal(t, int64(2), count) // deduped to two rows

	got, err := s.ForItems(ctx, TypeGame, []uint{1})
	require.NoError(t, err)
	assert.Equal(t, []string{FormatPhysical, FormatGOG}, got[1]) // canonical order
}

// Set replaces (not appends) an item's formats on each call, and an empty set
// clears them.
func TestSetReplacesAndClears(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, TypeGame, 5, []string{"Physical", "GOG"}))
	require.NoError(t, s.Set(ctx, TypeGame, 5, []string{"GOG"}))

	got, err := s.ForItems(ctx, TypeGame, []uint{5})
	require.NoError(t, err)
	assert.Equal(t, []string{FormatGOG}, got[5])

	require.NoError(t, s.Set(ctx, TypeGame, 5, nil))
	got, err = s.ForItems(ctx, TypeGame, []uint{5})
	require.NoError(t, err)
	_, ok := got[5]
	assert.False(t, ok, "cleared item is absent from the map")
}

// ForItems batches by item id and omits items with no formats.
func TestForItemsBatch(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, TypeGame, 1, []string{"Physical"}))
	require.NoError(t, s.Set(ctx, TypeGame, 2, []string{"Physical", "GOG"}))

	got, err := s.ForItems(ctx, TypeGame, []uint{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, []string{FormatPhysical}, got[1])
	assert.Equal(t, []string{FormatPhysical, FormatGOG}, got[2])
	_, ok := got[3]
	assert.False(t, ok, "an item with no formats is absent from the map")
}

// A format outside the allowed set is rejected and nothing is written.
func TestSetRejectsInvalidFormat(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	ctx := context.Background()

	err := s.Set(ctx, TypeGame, 1, []string{"Physical", "Steam"})
	require.ErrorIs(t, err, ErrInvalidFormat)

	var count int64
	require.NoError(t, gdb.Model(&models.OwnershipFormat{}).Count(&count).Error)
	assert.Equal(t, int64(0), count) // rejected before any write
}

// An unknown/unsupported media type has no allowed set and is rejected.
func TestSetRejectsUnknownMediaType(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	err := s.Set(context.Background(), "BOOK", 1, []string{"Physical"})
	require.ErrorIs(t, err, ErrInvalidFormat)
}

// A zero item id is rejected — ownership must attach to a persisted item.
func TestSetRejectsZeroID(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	err := s.Set(context.Background(), TypeGame, 0, []string{"Physical"})
	require.Error(t, err)
}
