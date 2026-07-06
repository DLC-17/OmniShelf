package tv

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// ── fakes ──

// fakeTMDB is an in-memory TMDB implementation.
type fakeTMDB struct {
	shows   map[int]*tmdb.Show
	seasons map[int]map[int]*tmdb.Season // showID → seasonNumber → season
	err     error                        // when set, every call fails with it
}

func (f *fakeTMDB) SearchTV(_ context.Context, query string) (*tmdb.SearchResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &tmdb.SearchResponse{Results: []tmdb.SearchResult{{ID: 1399, Name: query}}}, nil
}

func (f *fakeTMDB) GetShow(_ context.Context, id int) (*tmdb.Show, error) {
	if f.err != nil {
		return nil, f.err
	}
	s, ok := f.shows[id]
	if !ok {
		return nil, &tmdb.StatusError{StatusCode: http.StatusNotFound, Body: "not found"}
	}
	return s, nil
}

func (f *fakeTMDB) GetSeason(_ context.Context, showID, seasonNum int) (*tmdb.Season, error) {
	if f.err != nil {
		return nil, f.err
	}
	season, ok := f.seasons[showID][seasonNum]
	if !ok {
		return nil, &tmdb.StatusError{StatusCode: http.StatusNotFound, Body: "not found"}
	}
	return season, nil
}

// fakeImages records fetches; when fail is set every Fetch errors (E13 path).
type fakeImages struct {
	fail    bool
	fetched []string
}

func (f *fakeImages) Fetch(_ context.Context, _ *http.Client, url, kind, externalID string) (string, error) {
	if f.fail {
		return "", errors.New("image server down")
	}
	f.fetched = append(f.fetched, url)
	return kind + "/" + externalID + ".jpg", nil
}

// ── fixtures ──

func day(offset int) string {
	return time.Now().AddDate(0, 0, offset).Format("2006-01-02")
}

// twoSeasonShow returns a fixture show 100 with S1 (2 aired episodes),
// S2 (1 aired, 1 future, 1 unannounced air date).
func twoSeasonShow() *fakeTMDB {
	return &fakeTMDB{
		shows: map[int]*tmdb.Show{
			100: {
				ID: 100, Name: "Fixture Show", Status: "Returning Series", PosterPath: "/p100.jpg",
				Seasons: []tmdb.SeasonSummary{
					{SeasonNumber: 1, EpisodeCount: 2},
					{SeasonNumber: 2, EpisodeCount: 3},
				},
			},
		},
		seasons: map[int]map[int]*tmdb.Season{
			100: {
				1: {SeasonNumber: 1, Episodes: []tmdb.Episode{
					{SeasonNumber: 1, EpisodeNumber: 1, Name: "S1E1", AirDate: day(-30)},
					{SeasonNumber: 1, EpisodeNumber: 2, Name: "S1E2", AirDate: day(-20)},
				}},
				2: {SeasonNumber: 2, Episodes: []tmdb.Episode{
					{SeasonNumber: 2, EpisodeNumber: 1, Name: "S2E1", AirDate: day(-5)},
					{SeasonNumber: 2, EpisodeNumber: 2, Name: "S2E2", AirDate: day(30)},
					{SeasonNumber: 2, EpisodeNumber: 3, Name: "S2E3", AirDate: ""},
				}},
			},
		},
	}
}

func newTestService(t *testing.T, tm TMDB, imgs ImageStore) (*Service, *gorm.DB) {
	t.Helper()
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	// Close the pool before TempDir cleanup: on Windows an open SQLite
	// handle makes the directory removal fail.
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return New(gdb, tm, imgs), gdb
}

const userID = uint(1)

// ── AddShow ──

func TestAddShowPersistsEverything(t *testing.T) {
	imgs := &fakeImages{}
	svc, gdb := newTestService(t, twoSeasonShow(), imgs)

	res, err := svc.AddShow(context.Background(), userID, 100)
	require.NoError(t, err)

	assert.Equal(t, "Fixture Show", res.Show.Title)
	assert.Equal(t, 100, res.Show.TMDBID)
	assert.Equal(t, "tv/100.jpg", res.Show.PosterPath, "poster must be cached and stored as relative path")
	require.Len(t, imgs.fetched, 1)

	assert.Equal(t, "TV", res.Item.Type)
	assert.Equal(t, "100", res.Item.ExternalID)
	assert.Equal(t, "WATCHING", res.Item.Status)
	assert.Equal(t, userID, res.Item.UserID)

	var count int64
	require.NoError(t, gdb.Model(&models.Episode{}).Where("show_id = ?", res.Show.ID).Count(&count).Error)
	assert.EqualValues(t, 5, count, "all episodes across both seasons persisted")

	var unannounced models.Episode
	require.NoError(t, gdb.Where("show_id = ? AND season = 2 AND number = 3", res.Show.ID).First(&unannounced).Error)
	assert.Nil(t, unannounced.AirDate, "empty TMDB air date must persist as nil")
}

func TestAddShowDuplicateConflict(t *testing.T) {
	svc, _ := newTestService(t, twoSeasonShow(), &fakeImages{})

	first, err := svc.AddShow(context.Background(), userID, 100)
	require.NoError(t, err)

	_, err = svc.AddShow(context.Background(), userID, 100)
	var conflict *ConflictError
	require.ErrorAs(t, err, &conflict, "duplicate add must return typed conflict (E16)")
	assert.Equal(t, first.Item.ID, conflict.Existing.ID)

	// A different user tracking the same show is not a conflict.
	_, err = svc.AddShow(context.Background(), userID+1, 100)
	require.NoError(t, err)
}

func TestAddShowPosterFailureDoesNotFailAdd(t *testing.T) {
	svc, _ := newTestService(t, twoSeasonShow(), &fakeImages{fail: true})

	res, err := svc.AddShow(context.Background(), userID, 100)
	require.NoError(t, err, "poster download failure must not fail the add (E13)")
	assert.Empty(t, res.Show.PosterPath, "failed poster leaves PosterPath empty")
	assert.Equal(t, "WATCHING", res.Item.Status)
}

func TestAddShowTMDBDown(t *testing.T) {
	svc, _ := newTestService(t, &fakeTMDB{err: errors.New("connection refused")}, &fakeImages{})

	_, err := svc.AddShow(context.Background(), userID, 100)
	var up *UpstreamError
	require.ErrorAs(t, err, &up, "TMDB outage must surface as UpstreamError (E3)")
}

func TestAddShowUnknownTMDBID(t *testing.T) {
	svc, _ := newTestService(t, twoSeasonShow(), &fakeImages{})

	_, err := svc.AddShow(context.Background(), userID, 999)
	require.ErrorIs(t, err, ErrNotFound, "TMDB 404 maps to ErrNotFound")
}

// ── Search ──

func TestSearchProxiesAndWrapsErrors(t *testing.T) {
	svc, _ := newTestService(t, twoSeasonShow(), &fakeImages{})
	res, err := svc.Search(context.Background(), "fixture")
	require.NoError(t, err)
	require.Len(t, res.Results, 1)

	svcDown, _ := newTestService(t, &fakeTMDB{err: errors.New("boom")}, &fakeImages{})
	_, err = svcDown.Search(context.Background(), "fixture")
	var up *UpstreamError
	require.ErrorAs(t, err, &up)
}

// ── Up Next ──

func addFixtureShow(t *testing.T, svc *Service) *AddResult {
	t.Helper()
	res, err := svc.AddShow(context.Background(), userID, 100)
	require.NoError(t, err)
	return res
}

func episodeByNumber(t *testing.T, gdb *gorm.DB, showID uint, season, number int) models.Episode {
	t.Helper()
	var ep models.Episode
	require.NoError(t, gdb.Where("show_id = ? AND season = ? AND number = ?", showID, season, number).First(&ep).Error)
	return ep
}

func TestUpNextEarliestAiredUnwatched(t *testing.T) {
	svc, _ := newTestService(t, twoSeasonShow(), &fakeImages{})
	addFixtureShow(t, svc)

	entries, err := svc.UpNext(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 1, entries[0].Episode.Season)
	assert.Equal(t, 1, entries[0].Episode.Number)
	assert.Equal(t, "Fixture Show", entries[0].Show.Title)
}

func TestUpNextCrossesSeasonBoundary(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)

	// Watch all of season 1 → next up is S2E1.
	for _, n := range []int{1, 2} {
		ep := episodeByNumber(t, gdb, res.Show.ID, 1, n)
		_, err := svc.MarkWatched(context.Background(), userID, ep.ID)
		require.NoError(t, err)
	}

	entries, err := svc.UpNext(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 2, entries[0].Episode.Season)
	assert.Equal(t, 1, entries[0].Episode.Number)
}

func TestUpNextOmitsFullyWatchedAndSkipsUnaired(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)

	// Watch every aired episode. S2E2 (future) and S2E3 (nil air date)
	// remain unwatched but must never surface.
	for _, sn := range [][2]int{{1, 1}, {1, 2}, {2, 1}} {
		ep := episodeByNumber(t, gdb, res.Show.ID, sn[0], sn[1])
		_, err := svc.MarkWatched(context.Background(), userID, ep.ID)
		require.NoError(t, err)
	}

	entries, err := svc.UpNext(context.Background(), userID)
	require.NoError(t, err)
	assert.Empty(t, entries, "show with no aired unwatched episodes is omitted")
}

func TestUpNextIgnoresNonWatchingAndOtherUsers(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)

	// Another user's watches must not affect this user's Up Next.
	ep := episodeByNumber(t, gdb, res.Show.ID, 1, 1)
	require.NoError(t, gdb.Create(&models.EpisodeWatch{UserID: 99, EpisodeID: ep.ID, WatchedAt: time.Now()}).Error)

	entries, err := svc.UpNext(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 1, entries[0].Episode.Number, "other users' watches are irrelevant")

	// Setting the item to COMPLETED removes the show from Up Next.
	require.NoError(t, gdb.Model(&models.TrackingItem{}).
		Where("user_id = ? AND external_id = ?", userID, "100").
		Update("status", "COMPLETED").Error)
	entries, err = svc.UpNext(context.Background(), userID)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// ── watch toggle ──

func TestMarkWatchedIdempotentAndReturnsNext(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)
	s1e1 := episodeByNumber(t, gdb, res.Show.ID, 1, 1)

	next, err := svc.MarkWatched(context.Background(), userID, s1e1.ID)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, 2, next.Number, "next-up advances to S1E2")

	// Re-marking is a no-op: same next-up, still one watch row.
	next, err = svc.MarkWatched(context.Background(), userID, s1e1.ID)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, 2, next.Number)

	var rows int64
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).
		Where("user_id = ? AND episode_id = ?", userID, s1e1.ID).Count(&rows).Error)
	assert.EqualValues(t, 1, rows, "idempotent upsert must not duplicate watch rows")
}

func TestMarkWatchedLastAiredReturnsNil(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)

	var next *models.Episode
	for _, sn := range [][2]int{{1, 1}, {1, 2}, {2, 1}} {
		ep := episodeByNumber(t, gdb, res.Show.ID, sn[0], sn[1])
		var err error
		next, err = svc.MarkWatched(context.Background(), userID, ep.ID)
		require.NoError(t, err)
	}
	assert.Nil(t, next, "marking the final aired episode leaves no next-up")
}

func TestUnmarkWatchedRestoresUpNext(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)
	s1e1 := episodeByNumber(t, gdb, res.Show.ID, 1, 1)

	_, err := svc.MarkWatched(context.Background(), userID, s1e1.ID)
	require.NoError(t, err)

	next, err := svc.UnmarkWatched(context.Background(), userID, s1e1.ID)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, 1, next.Season)
	assert.Equal(t, 1, next.Number, "unmark restores the episode as next-up")

	entries, err := svc.UpNext(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, s1e1.ID, entries[0].Episode.ID)

	// Un-watching an unwatched episode is a no-op, not an error.
	_, err = svc.UnmarkWatched(context.Background(), userID, s1e1.ID)
	require.NoError(t, err)
}

func TestWatchUnknownEpisode(t *testing.T) {
	svc, _ := newTestService(t, twoSeasonShow(), &fakeImages{})
	_, err := svc.MarkWatched(context.Background(), userID, 424242)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = svc.UnmarkWatched(context.Background(), userID, 424242)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestWatchedEpisodeMovedToFutureKeepsWatchRow exercises E19: an episode
// watched early (e.g., early release) whose air date later moves to the
// future keeps its watch row and never re-enters Up Next.
func TestWatchedEpisodeMovedToFutureKeepsWatchRow(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)
	s1e1 := episodeByNumber(t, gdb, res.Show.ID, 1, 1)

	_, err := svc.MarkWatched(context.Background(), userID, s1e1.ID)
	require.NoError(t, err)

	// Air date moves to the future after being watched.
	future := time.Now().AddDate(0, 0, 60)
	require.NoError(t, gdb.Model(&models.Episode{}).Where("id = ?", s1e1.ID).
		Update("air_date", &future).Error)

	var rows int64
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).
		Where("user_id = ? AND episode_id = ?", userID, s1e1.ID).Count(&rows).Error)
	assert.EqualValues(t, 1, rows, "watch row must be kept (E19)")

	entries, err := svc.UpNext(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, 2, entries[0].Episode.Number, "Up Next unaffected: advances to S1E2")
}

// TestAddShowIsRepeatableForSecondUser guards the shared-metadata upsert
// path: the second add must reuse (not duplicate) Show/Episode rows.
func TestAddShowSharedMetadataUpsert(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	first := addFixtureShow(t, svc)

	second, err := svc.AddShow(context.Background(), userID+1, 100)
	require.NoError(t, err)
	assert.Equal(t, first.Show.ID, second.Show.ID, "shared Show row reused")

	var shows, eps int64
	require.NoError(t, gdb.Model(&models.Show{}).Count(&shows).Error)
	require.NoError(t, gdb.Model(&models.Episode{}).Count(&eps).Error)
	assert.EqualValues(t, 1, shows)
	assert.EqualValues(t, 5, eps, "episodes must be upserted, not duplicated")
}

// ── episode picker: list / rewatch / watch-through ──

func TestListEpisodesReturnsAllWithWatchState(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)
	s1e1 := episodeByNumber(t, gdb, res.Show.ID, 1, 1)
	_, err := svc.MarkWatched(context.Background(), userID, s1e1.ID)
	require.NoError(t, err)

	states, err := svc.ListEpisodes(context.Background(), userID, res.Show.ID)
	require.NoError(t, err)
	require.Len(t, states, 5, "all five fixture episodes returned")

	// Ordered by (season, number).
	assert.Equal(t, 1, states[0].Episode.Season)
	assert.Equal(t, 1, states[0].Episode.Number)
	assert.Equal(t, 2, states[4].Episode.Season)
	assert.Equal(t, 3, states[4].Episode.Number)

	assert.True(t, states[0].Watched, "S1E1 is watched")
	require.NotNil(t, states[0].WatchedAt)
	assert.False(t, states[1].Watched, "S1E2 is unwatched")
	assert.Nil(t, states[1].WatchedAt)
}

func TestListEpisodesUnknownShow(t *testing.T) {
	svc, _ := newTestService(t, twoSeasonShow(), &fakeImages{})
	_, err := svc.ListEpisodes(context.Background(), userID, 424242)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRewatchRefreshesTimestamp(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)
	s1e1 := episodeByNumber(t, gdb, res.Show.ID, 1, 1)

	_, err := svc.MarkWatched(context.Background(), userID, s1e1.ID)
	require.NoError(t, err)

	// Backdate the watch so the rewatch is unambiguously newer.
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).
		Where("user_id = ? AND episode_id = ?", userID, s1e1.ID).
		Update("watched_at", old).Error)

	_, err = svc.Rewatch(context.Background(), userID, s1e1.ID)
	require.NoError(t, err)

	var rows int64
	var watch models.EpisodeWatch
	require.NoError(t, gdb.Where("user_id = ? AND episode_id = ?", userID, s1e1.ID).First(&watch).Error)
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).
		Where("user_id = ? AND episode_id = ?", userID, s1e1.ID).Count(&rows).Error)
	assert.EqualValues(t, 1, rows, "rewatch upserts, never duplicates")
	assert.True(t, watch.WatchedAt.After(old), "rewatch advances WatchedAt")
}

func TestRewatchUnknownEpisode(t *testing.T) {
	svc, _ := newTestService(t, twoSeasonShow(), &fakeImages{})
	_, err := svc.Rewatch(context.Background(), userID, 999999)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestWatchThroughMarksAllPriorAired(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)
	s2e1 := episodeByNumber(t, gdb, res.Show.ID, 2, 1)

	next, err := svc.WatchThrough(context.Background(), userID, s2e1.ID)
	require.NoError(t, err)
	assert.Nil(t, next, "no aired unwatched episodes remain after catching up through S2E1")

	// S1E1, S1E2, S2E1 (all aired, <= S2E1) are watched; the future S2E2 and
	// unannounced S2E3 are not.
	var count int64
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).
		Where("user_id = ?", userID).Count(&count).Error)
	assert.EqualValues(t, 3, count, "exactly the three aired episodes up to S2E1 are marked")

	for _, sn := range [][2]int{{2, 2}, {2, 3}} {
		ep := episodeByNumber(t, gdb, res.Show.ID, sn[0], sn[1])
		var rows int64
		require.NoError(t, gdb.Model(&models.EpisodeWatch{}).
			Where("user_id = ? AND episode_id = ?", userID, ep.ID).Count(&rows).Error)
		assert.EqualValues(t, 0, rows, "unaired episodes are never bulk-marked")
	}
}

func TestWatchThroughIsIdempotent(t *testing.T) {
	svc, gdb := newTestService(t, twoSeasonShow(), &fakeImages{})
	res := addFixtureShow(t, svc)
	s2e1 := episodeByNumber(t, gdb, res.Show.ID, 2, 1)

	_, err := svc.WatchThrough(context.Background(), userID, s2e1.ID)
	require.NoError(t, err)
	_, err = svc.WatchThrough(context.Background(), userID, s2e1.ID)
	require.NoError(t, err)

	var count int64
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).
		Where("user_id = ?", userID).Count(&count).Error)
	assert.EqualValues(t, 3, count, "re-running watch-through does not duplicate rows")
}
