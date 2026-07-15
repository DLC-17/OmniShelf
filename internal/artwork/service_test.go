package artwork

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/models"
)

type fakeIGDB struct {
	games   map[int]*igdb.Game
	search  []igdb.SearchResult
	getError error
}

func (f *fakeIGDB) GetGame(_ context.Context, id int) (*igdb.Game, error) {
	if f.getError != nil {
		return nil, f.getError
	}
	g, ok := f.games[id]
	if !ok {
		return nil, nil
	}
	return g, nil
}

func (f *fakeIGDB) SearchGames(_ context.Context, _ string) ([]igdb.SearchResult, error) {
	if f.getError != nil {
		return nil, f.getError
	}
	return f.search, nil
}

func (f *fakeIGDB) CoverURL(imageID, _ string) string {
	if imageID == "" {
		return ""
	}
	return "http://images.test/" + imageID + ".jpg"
}

type fakeImages struct {
	savePath string
	err      error
}

func (f *fakeImages) Fetch(_ context.Context, _ *http.Client, url, kind, externalID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "/images/" + kind + "/" + externalID + ".jpg", nil
}

func (f *fakeImages) Save(_ io.Reader, kind, externalID string) (string, error) {
	return "", nil
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gdb
}

func TestRefreshGameWithIGDBID(t *testing.T) {
	gdb := testDB(t)
	ctx := context.Background()

	// Seed game and tracking item
	igdbID := 12345
	game := models.Game{
		IGDBID:    igdbID,
		Title:     "Awesome Game",
		CoverPath: "",
	}
	require.NoError(t, gdb.Create(&game).Error)

	item := models.TrackingItem{
		UserID:     1,
		Type:       typeGame,
		ExternalID: strconv.Itoa(igdbID),
		Title:      game.Title,
	}
	require.NoError(t, gdb.Create(&item).Error)

	ig := &fakeIGDB{
		games: map[int]*igdb.Game{
			igdbID: {
				ID:           igdbID,
				Name:         "Awesome Game",
				CoverImageID: "cover123",
			},
		},
	}
	imgs := &fakeImages{}

	svc := New(gdb, nil, ig, nil, imgs)
	path, err := svc.Refresh(ctx, 1, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "/images/game/igdb-12345.jpg", path)

	// Verify DB is updated
	var updated models.Game
	require.NoError(t, gdb.First(&updated, game.ID).Error)
	assert.Equal(t, "/images/game/igdb-12345.jpg", updated.CoverPath)
}

func TestRefreshGameResolvesMissingIGDBID(t *testing.T) {
	gdb := testDB(t)
	ctx := context.Background()

	// Seed game without IGDBID and tracking item with barcode
	barcode := "045496590420"
	game := models.Game{
		Barcode:   barcode,
		Title:     "Secret Game",
		CoverPath: "",
	}
	require.NoError(t, gdb.Create(&game).Error)

	item := models.TrackingItem{
		UserID:     1,
		Type:       typeGame,
		ExternalID: barcode,
		Title:      game.Title,
	}
	require.NoError(t, gdb.Create(&item).Error)

	ig := &fakeIGDB{
		search: []igdb.SearchResult{
			{
				ID:           9999,
				Name:         "Secret Game",
				CoverImageID: "secretcover",
			},
		},
		games: map[int]*igdb.Game{
			9999: {
				ID:           9999,
				Name:         "Secret Game",
				CoverImageID: "secretcover",
			},
		},
	}
	imgs := &fakeImages{}

	svc := New(gdb, nil, ig, nil, imgs)
	path, err := svc.Refresh(ctx, 1, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "/images/game/045496590420.jpg", path)

	// Verify DB resolved the IGDB ID and updated cover
	var updated models.Game
	require.NoError(t, gdb.First(&updated, game.ID).Error)
	assert.Equal(t, 9999, updated.IGDBID)
	assert.Equal(t, "/images/game/045496590420.jpg", updated.CoverPath)
}
