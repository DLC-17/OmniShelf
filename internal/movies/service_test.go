package movies

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// fakeTMDB is a canned movie TMDB client.
type fakeTMDB struct {
	movies  map[int]*tmdb.Movie
	recs    map[int][]tmdb.MovieResult
	getHits int
}

func (f *fakeTMDB) SearchMovie(_ context.Context, query string) (*tmdb.MovieSearchResponse, error) {
	return &tmdb.MovieSearchResponse{Results: []tmdb.MovieResult{{ID: 27205, Title: "Inception"}}}, nil
}

func (f *fakeTMDB) GetMovie(_ context.Context, id int) (*tmdb.Movie, error) {
	f.getHits++
	m, ok := f.movies[id]
	if !ok {
		return nil, &tmdb.StatusError{StatusCode: http.StatusNotFound}
	}
	return m, nil
}

func (f *fakeTMDB) MovieRecommendations(_ context.Context, id int) (*tmdb.MovieSearchResponse, error) {
	return &tmdb.MovieSearchResponse{Results: f.recs[id]}, nil
}

// fakeImages is a canned ImageStore.
type fakeImages struct {
	fetched []string
	err     error
}

func (f *fakeImages) Fetch(_ context.Context, _ *http.Client, _, kind, externalID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.fetched = append(f.fetched, externalID)
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

func inception() *tmdb.Movie {
	return &tmdb.Movie{ID: 27205, Title: "Inception", Overview: "A thief.", Status: "Released", ReleaseDate: "2010-07-15", PosterPath: "/x.jpg"}
}

func newService(t *testing.T, tm *fakeTMDB, imgs *fakeImages) (*Service, *gorm.DB) {
	t.Helper()
	gdb := testDB(t)
	return New(gdb, tm, imgs, WithImageBaseURL("http://img.test")), gdb
}

func TestAddMovieHappyPath(t *testing.T) {
	tm := &fakeTMDB{movies: map[int]*tmdb.Movie{27205: inception()}}
	imgs := &fakeImages{}
	svc, gdb := newService(t, tm, imgs)

	res, err := svc.AddMovie(context.Background(), 1, 27205)
	require.NoError(t, err)
	assert.Equal(t, "Inception", res.Movie.Title)
	assert.Equal(t, "A thief.", res.Movie.Overview)
	assert.Equal(t, "movie/27205.jpg", res.Movie.PosterPath)
	assert.Equal(t, "MOVIE", res.Item.Type)
	assert.Equal(t, "PLAN_TO", res.Item.Status)
	assert.Equal(t, []string{"27205"}, imgs.fetched)

	var count int64
	require.NoError(t, gdb.Model(&models.Movie{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAddMovieDuplicate(t *testing.T) {
	tm := &fakeTMDB{movies: map[int]*tmdb.Movie{27205: inception()}}
	svc, _ := newService(t, tm, &fakeImages{})

	_, err := svc.AddMovie(context.Background(), 1, 27205)
	require.NoError(t, err)
	_, err = svc.AddMovie(context.Background(), 1, 27205)
	var conflict *ConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, "27205", conflict.Existing.ExternalID)
}

// A second add of a cached movie by another user must not re-hit TMDB.
func TestAddMovieDBFirst(t *testing.T) {
	tm := &fakeTMDB{movies: map[int]*tmdb.Movie{27205: inception()}}
	svc, _ := newService(t, tm, &fakeImages{})

	_, err := svc.AddMovie(context.Background(), 1, 27205)
	require.NoError(t, err)
	_, err = svc.AddMovie(context.Background(), 2, 27205)
	require.NoError(t, err)
	assert.Equal(t, 1, tm.getHits, "cached movie should skip the TMDB round-trip")
}

func TestAddMovieNotFound(t *testing.T) {
	tm := &fakeTMDB{movies: map[int]*tmdb.Movie{}}
	svc, _ := newService(t, tm, &fakeImages{})

	_, err := svc.AddMovie(context.Background(), 1, 999)
	require.ErrorIs(t, err, ErrNotFound)
}

// A failed poster download must not fail the add.
func TestAddMoviePosterBestEffort(t *testing.T) {
	tm := &fakeTMDB{movies: map[int]*tmdb.Movie{27205: inception()}}
	imgs := &fakeImages{err: assertErr}
	svc, _ := newService(t, tm, imgs)

	res, err := svc.AddMovie(context.Background(), 1, 27205)
	require.NoError(t, err)
	assert.Equal(t, "", res.Movie.PosterPath)
}

func TestDiscover(t *testing.T) {
	tm := &fakeTMDB{
		movies: map[int]*tmdb.Movie{27205: inception()},
		recs: map[int][]tmdb.MovieResult{
			27205: {{ID: 348350, Title: "Solo"}, {ID: 27205, Title: "Inception"}}, // self is filtered as tracked
		},
	}
	svc, _ := newService(t, tm, &fakeImages{})
	_, err := svc.AddMovie(context.Background(), 1, 27205)
	require.NoError(t, err)

	items, err := svc.Discover(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 348350, items[0].TMDBID)
	assert.Equal(t, "Inception", items[0].SuggestedBy)
}

func TestRejectRecExcludesFromDiscover(t *testing.T) {
	tm := &fakeTMDB{
		movies: map[int]*tmdb.Movie{27205: inception()},
		recs:   map[int][]tmdb.MovieResult{27205: {{ID: 348350, Title: "Solo"}}},
	}
	svc, _ := newService(t, tm, &fakeImages{})
	_, err := svc.AddMovie(context.Background(), 1, 27205)
	require.NoError(t, err)

	require.NoError(t, svc.RejectRec(context.Background(), 1, 348350))
	items, err := svc.Discover(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// assertErr is a sentinel used to force the poster download to fail.
var assertErr = &posterErr{}

type posterErr struct{}

func (*posterErr) Error() string { return "boom" }
