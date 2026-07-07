package games

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/scandex"
)

const testBarcode = "045496590420"

// fakeMetadata is a canned MetadataClient.
type fakeMetadata struct {
	games map[string]*scandex.Game
	err   error
	calls int
}

func (f *fakeMetadata) Lookup(_ context.Context, barcode string) (*scandex.Game, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	g, ok := f.games[barcode]
	if !ok {
		return nil, &scandex.NotFoundError{Barcode: barcode}
	}
	return g, nil
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

func fullGame() *scandex.Game {
	return &scandex.Game{
		Barcode:  testBarcode,
		Title:    "The Legend of Zelda: Breath of the Wild",
		Platform: "Nintendo Switch",
		IGDBID:   7346,
	}
}

// fakeEnricher is a canned IGDB Enricher.
type fakeEnricher struct {
	games map[int]*igdb.Game
	err   error
}

func (f *fakeEnricher) GetGame(_ context.Context, id int) (*igdb.Game, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.games[id], nil // nil map miss → (nil, nil), a valid "no metadata"
}

func (f *fakeEnricher) CoverURL(imageID, _ string) string {
	if imageID == "" {
		return ""
	}
	return "http://img.test/" + imageID + ".jpg"
}

// fakeImages is a canned ImageStore.
type fakeImages struct {
	err     error
	fetched []string
}

func (f *fakeImages) Fetch(_ context.Context, _ *http.Client, _, kind, externalID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.fetched = append(f.fetched, externalID)
	return kind + "/" + externalID + ".jpg", nil
}

func newTestService(t *testing.T, meta *fakeMetadata) (*Service, *gorm.DB) {
	t.Helper()
	gdb := testDB(t)
	return NewService(gdb, meta, nil, nil), gdb
}

func newEnrichedService(t *testing.T, meta *fakeMetadata, enr *fakeEnricher, imgs *fakeImages) (*Service, *gorm.DB) {
	t.Helper()
	gdb := testDB(t)
	return NewService(gdb, meta, enr, imgs), gdb
}

func TestScanHappyPath(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{testBarcode: fullGame()}}
	svc, gdb := newTestService(t, meta)

	game, err := svc.Scan(context.Background(), "0-45496-59042-0") // separators normalize
	require.NoError(t, err)
	assert.Equal(t, testBarcode, game.Barcode)
	assert.Equal(t, "The Legend of Zelda: Breath of the Wild", game.Title)
	assert.Equal(t, "Nintendo Switch", game.Platform)
	assert.Equal(t, 7346, game.IGDBID)

	var count int64
	require.NoError(t, gdb.Model(&models.Game{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// A second scan of a cached barcode must not hit ScanDex again (DB-first).
func TestScanDBFirst(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{testBarcode: fullGame()}}
	svc, _ := newTestService(t, meta)

	_, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)
	_, err = svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)
	assert.Equal(t, 1, meta.calls, "second scan should be served from cache")
}

// IGDB enrichment adds a summary and downloads the cover.
func TestScanEnrichesFromIGDB(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{testBarcode: fullGame()}}
	enr := &fakeEnricher{games: map[int]*igdb.Game{
		7346: {ID: 7346, Name: "Zelda", Summary: "An open-world adventure.", CoverImageID: "co3p2d"},
	}}
	imgs := &fakeImages{}
	svc, _ := newEnrichedService(t, meta, enr, imgs)

	game, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)
	assert.Equal(t, "An open-world adventure.", game.Description)
	assert.Equal(t, "game/"+testBarcode+".jpg", game.CoverPath)
	assert.Equal(t, []string{testBarcode}, imgs.fetched)
}

// A failing IGDB lookup must not fail the scan — the game is still saved.
func TestScanEnrichmentBestEffort(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{testBarcode: fullGame()}}
	enr := &fakeEnricher{err: errors.New("igdb down")}
	svc, _ := newEnrichedService(t, meta, enr, &fakeImages{})

	game, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)
	assert.Equal(t, "The Legend of Zelda: Breath of the Wild", game.Title)
	assert.Equal(t, "", game.Description)
	assert.Equal(t, "", game.CoverPath)
}

func TestScanNotFound(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{}}
	svc, _ := newTestService(t, meta)

	_, err := svc.Scan(context.Background(), "711719521099")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestScanInvalidBarcode(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{})

	_, err := svc.Scan(context.Background(), "123") // too short
	require.ErrorIs(t, err, ErrInvalidBarcode)
}

func TestScanUpstream(t *testing.T) {
	meta := &fakeMetadata{err: errors.New("boom")}
	svc, _ := newTestService(t, meta)

	_, err := svc.Scan(context.Background(), testBarcode)
	require.ErrorIs(t, err, ErrUpstream)
}

func TestTrackHappyPath(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{testBarcode: fullGame()}}
	svc, _ := newTestService(t, meta)

	game, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)

	item, err := svc.Track(context.Background(), 1, game.ID, StatusPlaying)
	require.NoError(t, err)
	assert.Equal(t, TypeGame, item.Type)
	assert.Equal(t, testBarcode, item.ExternalID)
	assert.Equal(t, StatusPlaying, item.Status)
}

func TestTrackInvalidStatus(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{testBarcode: fullGame()}}
	svc, _ := newTestService(t, meta)
	game, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)

	_, err = svc.Track(context.Background(), 1, game.ID, "READING")
	require.ErrorIs(t, err, ErrInvalidStatus)
}

func TestTrackGameNotFound(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{})
	_, err := svc.Track(context.Background(), 1, 999, StatusPlaying)
	require.ErrorIs(t, err, ErrGameNotFound)
}

func TestTrackAlreadyTracked(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{testBarcode: fullGame()}}
	svc, _ := newTestService(t, meta)
	game, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)

	_, err = svc.Track(context.Background(), 1, game.ID, StatusPlaying)
	require.NoError(t, err)
	existing, err := svc.Track(context.Background(), 1, game.ID, StatusPlaying)
	require.ErrorIs(t, err, ErrAlreadyTracked)
	require.NotNil(t, existing)
}
