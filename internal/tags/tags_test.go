package tags

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

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Time Travel", "time-travel"},
		{"time-travel", "time-travel"},
		{"  Sci-Fi  ", "sci-fi"},
		{"Role-playing (RPG)", "role-playing-rpg"},
		{"post-apocalyptic!!", "post-apocalyptic"},
		{"UPPER", "upper"},
		{"", ""},
		{"   ", ""},
		{"&&&", ""},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, Slugify(c.in), "Slugify(%q)", c.in)
	}
}

// Set creates shared Tag rows (one per slug) and links them to the media row;
// names differing only by slug-equivalent punctuation/case collapse to one tag.
func TestSetSharesTagsAndDedupes(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, TypeGame, 1, []string{"Adventure", "Time Travel", "time-travel", ""}))
	require.NoError(t, s.Set(ctx, TypeMovie, 7, []string{"Adventure", "Sci-Fi"}))

	// "Adventure" is shared across the game and the movie; "time-travel" only
	// created once despite two spellings → 3 distinct tags total.
	var tagCount int64
	require.NoError(t, gdb.Model(&models.Tag{}).Count(&tagCount).Error)
	assert.Equal(t, int64(3), tagCount)

	got, err := s.ForMedia(ctx, TypeGame, []uint{1})
	require.NoError(t, err)
	assert.Equal(t, []string{"Adventure", "Time Travel"}, got[1]) // sorted, deduped
}

// Set replaces (not appends) an item's tags on each call.
func TestSetReplaces(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, TypeTV, 5, []string{"Drama", "Crime"}))
	require.NoError(t, s.Set(ctx, TypeTV, 5, []string{"Comedy"}))

	got, err := s.ForMedia(ctx, TypeTV, []uint{5})
	require.NoError(t, err)
	assert.Equal(t, []string{"Comedy"}, got[5])
}

// ForMedia batches by media id and omits items with no tags.
func TestForMediaBatch(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, TypeBook, 1, []string{"Fantasy"}))
	require.NoError(t, s.Set(ctx, TypeBook, 2, []string{"Fantasy", "Adventure"}))

	got, err := s.ForMedia(ctx, TypeBook, []uint{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, []string{"Fantasy"}, got[1])
	assert.Equal(t, []string{"Adventure", "Fantasy"}, got[2])
	_, ok := got[3]
	assert.False(t, ok, "an item with no tags is absent from the map")
}

// MediaIDs is the reverse lookup (media carrying a tag), keyed by slug.
func TestMediaIDsByTag(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, TypeGame, 10, []string{"Open World"}))
	require.NoError(t, s.Set(ctx, TypeGame, 11, []string{"open-world", "RPG"}))
	require.NoError(t, s.Set(ctx, TypeMovie, 10, []string{"Open World"})) // different type, same slug

	ids, err := s.MediaIDs(ctx, TypeGame, "open-world")
	require.NoError(t, err)
	assert.Equal(t, []uint{10, 11}, ids)

	none, err := s.MediaIDs(ctx, TypeGame, "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, none)
}

// A zero media id is rejected — tags must attach to a persisted cache row.
func TestSetRejectsZeroID(t *testing.T) {
	gdb := testDB(t)
	s := NewStore(gdb)
	err := s.Set(context.Background(), TypeGame, 0, []string{"X"})
	require.Error(t, err)
}
