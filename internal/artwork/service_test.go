package artwork

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/openlibrary"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// ── fakes ──

type fakeTMDB struct {
	show  *tmdb.Show
	movie *tmdb.Movie
	err   error
}

func (f *fakeTMDB) GetShow(context.Context, int) (*tmdb.Show, error)   { return f.show, f.err }
func (f *fakeTMDB) GetMovie(context.Context, int) (*tmdb.Movie, error) { return f.movie, f.err }

type fakeIGDB struct {
	game *igdb.Game
	err  error
}

func (f *fakeIGDB) GetGame(context.Context, int) (*igdb.Game, error) { return f.game, f.err }
func (f *fakeIGDB) CoverURL(imageID, _ string) string {
	if imageID == "" {
		return ""
	}
	return "http://img.test/" + imageID + ".jpg"
}

type fakeOpenLib struct {
	book *openlibrary.Book
	err  error
}

func (f *fakeOpenLib) GetByISBN(context.Context, string) (*openlibrary.Book, error) {
	return f.book, f.err
}
func (f *fakeOpenLib) CoverURL(coverID int, _ string) string {
	if coverID == 0 {
		return ""
	}
	return "http://cover.test/id.jpg"
}

// fakeImages records the last Fetch/Save call and returns a canned rel path.
type fakeImages struct {
	fetchURL   string
	savedBytes string
	err        error
}

func (f *fakeImages) Fetch(_ context.Context, _ *http.Client, url, kind, externalID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.fetchURL = url
	return kind + "/" + externalID + ".jpg", nil
}

func (f *fakeImages) Save(r io.Reader, kind, externalID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	b, _ := io.ReadAll(r)
	f.savedBytes = string(b)
	return kind + "/" + externalID + ".jpg", nil
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

func seedItem(t *testing.T, gdb *gorm.DB, userID uint, typ, extID string) models.TrackingItem {
	t.Helper()
	it := models.TrackingItem{UserID: userID, Type: typ, ExternalID: extID, Title: "X", Status: "PLAN_TO"}
	require.NoError(t, gdb.Create(&it).Error)
	return it
}

// ── refresh ──

func TestRefreshShow(t *testing.T) {
	gdb := testDB(t)
	require.NoError(t, gdb.Create(&models.Show{TMDBID: 1399, Title: "HotD", PosterPath: "tv/1399.jpg"}).Error)
	item := seedItem(t, gdb, 1, "TV", "1399")

	imgs := &fakeImages{}
	svc := New(gdb, &fakeTMDB{show: &tmdb.Show{ID: 1399, PosterPath: "/new.jpg"}}, nil, nil, imgs,
		WithTMDBImageBase("https://cdn/"))

	rel, err := svc.Refresh(context.Background(), 1, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "tv/1399.jpg", rel)
	assert.Equal(t, "https://cdn//new.jpg", imgs.fetchURL)

	var show models.Show
	require.NoError(t, gdb.Where("tmdb_id = ?", 1399).First(&show).Error)
	assert.Equal(t, "tv/1399.jpg", show.PosterPath)
}

func TestRefreshMovieNoArtwork(t *testing.T) {
	gdb := testDB(t)
	require.NoError(t, gdb.Create(&models.Movie{TMDBID: 500, Title: "Film"}).Error)
	item := seedItem(t, gdb, 1, "MOVIE", "500")

	svc := New(gdb, &fakeTMDB{movie: &tmdb.Movie{ID: 500, PosterPath: ""}}, nil, nil, &fakeImages{})

	_, err := svc.Refresh(context.Background(), 1, item.ID)
	require.ErrorIs(t, err, ErrNoArtwork)
}

func TestRefreshGame(t *testing.T) {
	gdb := testDB(t)
	require.NoError(t, gdb.Create(&models.Game{Barcode: "123", Title: "Zelda", IGDBID: 7346}).Error)
	item := seedItem(t, gdb, 1, "GAME", "123")

	imgs := &fakeImages{}
	svc := New(gdb, &fakeTMDB{}, &fakeIGDB{game: &igdb.Game{ID: 7346, CoverImageID: "co3p2d"}}, nil, imgs)

	rel, err := svc.Refresh(context.Background(), 1, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "game/123.jpg", rel)
	assert.Equal(t, "http://img.test/co3p2d.jpg", imgs.fetchURL)

	var game models.Game
	require.NoError(t, gdb.Where("barcode = ?", "123").First(&game).Error)
	assert.Equal(t, "game/123.jpg", game.CoverPath)
}

func TestRefreshGameUnconfiguredIGDB(t *testing.T) {
	gdb := testDB(t)
	require.NoError(t, gdb.Create(&models.Game{Barcode: "123", Title: "Zelda", IGDBID: 7346}).Error)
	item := seedItem(t, gdb, 1, "GAME", "123")

	svc := New(gdb, &fakeTMDB{}, nil, nil, &fakeImages{}) // igdb nil
	_, err := svc.Refresh(context.Background(), 1, item.ID)
	require.ErrorIs(t, err, ErrNoArtwork)
}

func TestRefreshBook(t *testing.T) {
	gdb := testDB(t)
	require.NoError(t, gdb.Create(&models.Book{ISBN13: "9780441172719", Title: "Dune"}).Error)
	item := seedItem(t, gdb, 1, "BOOK", "9780441172719")

	imgs := &fakeImages{}
	svc := New(gdb, &fakeTMDB{}, nil, &fakeOpenLib{book: &openlibrary.Book{CoverID: 42}}, imgs)

	rel, err := svc.Refresh(context.Background(), 1, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "book/9780441172719.jpg", rel)

	var book models.Book
	require.NoError(t, gdb.Where("isbn13 = ?", "9780441172719").First(&book).Error)
	assert.Equal(t, "book/9780441172719.jpg", book.CoverPath)
}

func TestRefreshUpstreamError(t *testing.T) {
	gdb := testDB(t)
	require.NoError(t, gdb.Create(&models.Show{TMDBID: 1399, Title: "HotD"}).Error)
	item := seedItem(t, gdb, 1, "TV", "1399")

	svc := New(gdb, &fakeTMDB{err: errors.New("tmdb down")}, nil, nil, &fakeImages{})
	_, err := svc.Refresh(context.Background(), 1, item.ID)
	require.ErrorIs(t, err, ErrUpstream)
}

// A refresh of another user's item is indistinguishable from a missing one.
func TestRefreshItemNotFound(t *testing.T) {
	gdb := testDB(t)
	item := seedItem(t, gdb, 2, "TV", "1399") // owned by user 2

	svc := New(gdb, &fakeTMDB{}, nil, nil, &fakeImages{})
	_, err := svc.Refresh(context.Background(), 1, item.ID) // user 1 asks
	require.ErrorIs(t, err, ErrItemNotFound)
}

// ── upload ──

func TestUpload(t *testing.T) {
	gdb := testDB(t)
	require.NoError(t, gdb.Create(&models.Show{TMDBID: 1399, Title: "HotD"}).Error)
	item := seedItem(t, gdb, 1, "TV", "1399")

	imgs := &fakeImages{}
	svc := New(gdb, &fakeTMDB{}, nil, nil, imgs)

	rel, err := svc.Upload(context.Background(), 1, item.ID, strings.NewReader("PNGDATA"))
	require.NoError(t, err)
	assert.Equal(t, "tv/1399.jpg", rel)
	assert.Equal(t, "PNGDATA", imgs.savedBytes)

	var show models.Show
	require.NoError(t, gdb.Where("tmdb_id = ?", 1399).First(&show).Error)
	assert.Equal(t, "tv/1399.jpg", show.PosterPath)
}
