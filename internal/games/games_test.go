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
	"github.com/davidlc1229/omnishelf/internal/tags"
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
	games         map[int]*igdb.Game
	searchResults []igdb.SearchResult
	similar       map[int][]igdb.SimilarGame
	err           error
}

func (f *fakeEnricher) GetGame(_ context.Context, id int) (*igdb.Game, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.games[id], nil // nil map miss → (nil, nil), a valid "no metadata"
}

func (f *fakeEnricher) SearchGames(_ context.Context, _ string) ([]igdb.SearchResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.searchResults, nil
}

func (f *fakeEnricher) SimilarGames(_ context.Context, _ []int) (map[int][]igdb.SimilarGame, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.similar, nil
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
		7346: {ID: 7346, Name: "Zelda", Summary: "An open-world adventure.", CoverImageID: "co3p2d", ReleaseDate: "2017-03-03"},
	}}
	imgs := &fakeImages{}
	svc, _ := newEnrichedService(t, meta, enr, imgs)

	game, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)
	assert.Equal(t, "An open-world adventure.", game.Description)
	assert.Equal(t, "2017-03-03", game.ReleaseDate)
	assert.Equal(t, "game/"+testBarcode+".jpg", game.CoverPath)
	assert.Equal(t, []string{testBarcode}, imgs.fetched)
}

// IGDB genres + keywords are persisted as source-derived tags on the game row.
func TestScanPersistsIGDBTags(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{testBarcode: fullGame()}}
	enr := &fakeEnricher{games: map[int]*igdb.Game{
		7346: {
			ID:       7346,
			Name:     "Zelda",
			Genres:   []string{"Adventure", "Role-playing (RPG)"},
			Keywords: []string{"open world"},
		},
	}}
	svc, gdb := newEnrichedService(t, meta, enr, &fakeImages{})

	game, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)

	got, err := tags.NewStore(gdb).ForMedia(context.Background(), tags.TypeGame, []uint{game.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{"Adventure", "open world", "Role-playing (RPG)"}, got[game.ID])
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
	// GAME tracking items are keyed by the canonical IGDB id, not the barcode.
	assert.Equal(t, "7346", item.ExternalID)
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

// Search delegates to the IGDB enricher and returns its candidates.
func TestSearchDelegatesToIGDB(t *testing.T) {
	enr := &fakeEnricher{searchResults: []igdb.SearchResult{{ID: 7346, Name: "Zelda", Year: 2017}}}
	svc, _ := newEnrichedService(t, &fakeMetadata{}, enr, &fakeImages{})

	res, err := svc.Search(context.Background(), "zelda")
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, 7346, res[0].ID)
}

// A blank query never reaches IGDB.
func TestSearchEmptyQuery(t *testing.T) {
	enr := &fakeEnricher{}
	svc, _ := newEnrichedService(t, &fakeMetadata{}, enr, &fakeImages{})

	_, err := svc.Search(context.Background(), "   ")
	require.ErrorIs(t, err, ErrEmptyQuery)
}

// Without an enricher (no IGDB credentials) search is unavailable.
func TestSearchUnavailableWithoutEnricher(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{})

	_, err := svc.Search(context.Background(), "zelda")
	require.ErrorIs(t, err, ErrSearchUnavailable)
}

// AddByIGDB creates a barcode-less game keyed by its IGDB id and a PLAN_TO item.
func TestAddByIGDBCreatesBarcodelessGame(t *testing.T) {
	enr := &fakeEnricher{games: map[int]*igdb.Game{
		7346: {ID: 7346, Name: "Zelda", Summary: "An open-world adventure.", CoverImageID: "co3p2d"},
	}}
	imgs := &fakeImages{}
	svc, gdb := newEnrichedService(t, &fakeMetadata{}, enr, imgs)

	game, item, err := svc.AddByIGDB(context.Background(), 1, 7346, "")
	require.NoError(t, err)
	assert.Equal(t, 7346, game.IGDBID)
	assert.Equal(t, "", game.Barcode)
	assert.Equal(t, "Zelda", game.Title)
	assert.Equal(t, "game/igdb-7346.jpg", game.CoverPath)
	assert.Equal(t, StatusPlanTo, item.Status)
	assert.Equal(t, "7346", item.ExternalID)

	var count int64
	require.NoError(t, gdb.Model(&models.Game{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// A game scanned by barcode and later added by name share one cache row, keyed
// by IGDB id (DB-first, no duplicate).
func TestAddByIGDBReusesScannedRow(t *testing.T) {
	meta := &fakeMetadata{games: map[string]*scandex.Game{testBarcode: fullGame()}}
	enr := &fakeEnricher{games: map[int]*igdb.Game{7346: {ID: 7346, Name: "Zelda"}}}
	svc, gdb := newEnrichedService(t, meta, enr, &fakeImages{})

	scanned, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)

	game, _, err := svc.AddByIGDB(context.Background(), 1, 7346, StatusPlaying)
	require.NoError(t, err)
	assert.Equal(t, scanned.ID, game.ID)

	var count int64
	require.NoError(t, gdb.Model(&models.Game{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// Adding the same game by name twice reports ErrAlreadyTracked with the item.
func TestAddByIGDBAlreadyTracked(t *testing.T) {
	enr := &fakeEnricher{games: map[int]*igdb.Game{7346: {ID: 7346, Name: "Zelda"}}}
	svc, _ := newEnrichedService(t, &fakeMetadata{}, enr, &fakeImages{})

	_, _, err := svc.AddByIGDB(context.Background(), 1, 7346, StatusPlaying)
	require.NoError(t, err)
	_, item, err := svc.AddByIGDB(context.Background(), 1, 7346, StatusPlaying)
	require.ErrorIs(t, err, ErrAlreadyTracked)
	require.NotNil(t, item)
}

// Discover surfaces IGDB "similar games" seeded from a tracked game, filters the
// game the user already tracks, caches covers through internal/images, and tags
// each suggestion with the tracked game it came from.
func TestDiscoverSimilarGamesDedupesTracked(t *testing.T) {
	enr := &fakeEnricher{
		games: map[int]*igdb.Game{7346: {ID: 7346, Name: "Zelda"}},
		similar: map[int][]igdb.SimilarGame{
			7346: {
				{ID: 1234, Name: "Okami", Year: 2006, CoverImageID: "cov1"},
				{ID: 7346, Name: "Zelda", CoverImageID: "cov2"}, // self → filtered as tracked
			},
		},
	}
	imgs := &fakeImages{}
	svc, gdb := newEnrichedService(t, &fakeMetadata{}, enr, imgs)
	ctx := context.Background()

	// Track Zelda by IGDB id so it becomes a discover seed.
	_, _, err := svc.AddByIGDB(ctx, 1, 7346, StatusPlaying)
	require.NoError(t, err)
	require.NoError(t, gdb.Model(&models.TrackingItem{}).Where("user_id = ? AND external_id = ?", 1, "7346").Update("rating", 4).Error)

	items, err := svc.Discover(ctx, 1)
	require.NoError(t, err)
	require.Len(t, items, 1, "the tracked seed game is filtered from its own similars")
	assert.Equal(t, 1234, items[0].IGDBID)
	assert.Equal(t, "Okami", items[0].Title)
	assert.Equal(t, 2006, items[0].Year)
	assert.Equal(t, "Zelda", items[0].SuggestedBy)
	// Cover cached through internal/images, keyed by IGDB id (never hotlinked).
	assert.Equal(t, "game/igdb-1234.jpg", items[0].CoverPath)
	assert.Contains(t, imgs.fetched, "igdb-1234")
}

// A rejected game is not surfaced again by Discover.
func TestDiscoverExcludesRejectedGame(t *testing.T) {
	enr := &fakeEnricher{
		games:   map[int]*igdb.Game{7346: {ID: 7346, Name: "Zelda"}},
		similar: map[int][]igdb.SimilarGame{7346: {{ID: 1234, Name: "Okami"}}},
	}
	svc, _ := newEnrichedService(t, &fakeMetadata{}, enr, &fakeImages{})
	ctx := context.Background()

	_, _, err := svc.AddByIGDB(ctx, 1, 7346, StatusPlaying)
	require.NoError(t, err)

	require.NoError(t, svc.RejectRec(ctx, 1, 1234))
	items, err := svc.Discover(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// Without an enricher (no IGDB credentials) Discover degrades to empty, not an
// error.
func TestDiscoverWithoutEnricher(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{})
	items, err := svc.Discover(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, items)
}
