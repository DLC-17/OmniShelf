package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// fakeArtwork records Fetch calls and returns a canned relative path.
type fakeArtwork struct {
	calls []string
	err   error
}

func (f *fakeArtwork) Fetch(_ context.Context, _ *http.Client, url, kind, externalID string) (string, error) {
	f.calls = append(f.calls, url)
	if f.err != nil {
		return "", f.err
	}
	return kind + "/" + externalID + ".jpg", nil
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(v))
}

// newEngine wires a real tmdb.Client at the fixture server plus a fresh
// temp-file SQLite DB.
func newEngine(t *testing.T, handler http.Handler) (*Engine, *gorm.DB, *fakeArtwork) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := tmdb.New("test-key",
		tmdb.WithBaseURL(srv.URL),
		tmdb.WithRateLimit(rate.NewLimiter(rate.Inf, 1)),
		tmdb.WithBackoffBase(time.Millisecond),
	)
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		// Close the pool so Windows can delete the temp db file.
		if sqlDB, dbErr := gdb.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	art := &fakeArtwork{}
	return New(gdb, client, art), gdb, art
}

func track(t *testing.T, gdb *gorm.DB, userID uint, tmdbID int, status string) {
	t.Helper()
	require.NoError(t, gdb.Create(&models.TrackingItem{
		UserID:     userID,
		Type:       "TV",
		ExternalID: fmt.Sprintf("%d", tmdbID),
		Title:      fmt.Sprintf("Show %d", tmdbID),
		Status:     status,
	}).Error)
}

func date(t *testing.T, s string) *time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return &ts
}

// showFixture builds a handler serving one show with one season.
func showFixture(t *testing.T, id int, poster string, episodes []tmdb.Episode) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/tv/%d", id), func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, tmdb.Show{
			ID: id, Name: fmt.Sprintf("Show %d", id), Status: "Returning Series", PosterPath: poster,
			Seasons: []tmdb.SeasonSummary{{ID: 10, SeasonNumber: 1, EpisodeCount: len(episodes)}},
		})
	})
	mux.HandleFunc(fmt.Sprintf("/tv/%d/season/1", id), func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, tmdb.Season{ID: 10, SeasonNumber: 1, Episodes: episodes})
	})
	return mux
}

func loadSyncLogs(t *testing.T, gdb *gorm.DB) []models.SyncLog {
	t.Helper()
	var logs []models.SyncLog
	require.NoError(t, gdb.Find(&logs).Error)
	return logs
}

func syncErrors(t *testing.T, l models.SyncLog) []string {
	t.Helper()
	var errs []string
	require.NoError(t, json.Unmarshal([]byte(l.Errors), &errs))
	return errs
}

func TestRunUpsertsNewShowAndEpisodes(t *testing.T) {
	handler := showFixture(t, 1, "/p.jpg", []tmdb.Episode{
		{SeasonNumber: 1, EpisodeNumber: 1, Name: "Pilot", AirDate: "2024-01-01"},
		{SeasonNumber: 1, EpisodeNumber: 2, Name: "Two", AirDate: ""},
	})
	eng, gdb, art := newEngine(t, handler)
	track(t, gdb, 1, 1, "WATCHING")

	require.NoError(t, eng.Run(context.Background()))

	var show models.Show
	require.NoError(t, gdb.Where("tmdb_id = ?", 1).First(&show).Error)
	assert.Equal(t, "Show 1", show.Title)
	assert.Equal(t, "Returning Series", show.Status)
	assert.Equal(t, "tv/1.jpg", show.PosterPath, "missing artwork fetched and stored")
	assert.False(t, show.LastSyncedAt.IsZero())
	require.Len(t, art.calls, 1)

	var eps []models.Episode
	require.NoError(t, gdb.Where("show_id = ?", show.ID).Order("number").Find(&eps).Error)
	require.Len(t, eps, 2)
	assert.Equal(t, "Pilot", eps[0].Title)
	require.NotNil(t, eps[0].AirDate)
	assert.True(t, eps[0].AirDate.Equal(*date(t, "2024-01-01")))
	assert.Nil(t, eps[1].AirDate, "unannounced air date stays nil")

	logs := loadSyncLogs(t, gdb)
	require.Len(t, logs, 1)
	assert.Equal(t, 1, logs[0].ShowCount)
	assert.Empty(t, syncErrors(t, logs[0]))
}

func TestRunAppliesAirDateAndTitleChanges(t *testing.T) {
	handler := showFixture(t, 1, "", []tmdb.Episode{
		{SeasonNumber: 1, EpisodeNumber: 1, Name: "Renamed Pilot", AirDate: "2024-02-15"},
	})
	eng, gdb, _ := newEngine(t, handler)
	track(t, gdb, 1, 1, "PLAN_TO")

	show := models.Show{TMDBID: 1, Title: "Old Title", Status: "In Production"}
	require.NoError(t, gdb.Create(&show).Error)
	ep := models.Episode{ShowID: show.ID, Season: 1, Number: 1, Title: "Pilot", AirDate: date(t, "2024-01-01")}
	require.NoError(t, gdb.Create(&ep).Error)

	require.NoError(t, eng.Run(context.Background()))

	var got models.Episode
	require.NoError(t, gdb.First(&got, ep.ID).Error) // same row, not a duplicate
	assert.Equal(t, "Renamed Pilot", got.Title)
	require.NotNil(t, got.AirDate)
	assert.True(t, got.AirDate.Equal(*date(t, "2024-02-15")))

	var count int64
	require.NoError(t, gdb.Model(&models.Episode{}).Where("show_id = ?", show.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	var gotShow models.Show
	require.NoError(t, gdb.First(&gotShow, show.ID).Error)
	assert.Equal(t, "Show 1", gotShow.Title)
	assert.Equal(t, "Returning Series", gotShow.Status)
}

func TestRunOneFailingShowDoesNotAbortRun(t *testing.T) {
	good := showFixture(t, 2, "", []tmdb.Episode{
		{SeasonNumber: 1, EpisodeNumber: 1, Name: "Ep1", AirDate: "2024-01-01"},
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/tv/1", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	mux.Handle("/", good)
	eng, gdb, _ := newEngine(t, mux)
	track(t, gdb, 1, 1, "WATCHING")
	track(t, gdb, 2, 2, "WATCHING") // different user, same sweep

	require.NoError(t, eng.Run(context.Background()), "per-show failure must not abort the run")

	var show2 models.Show
	require.NoError(t, gdb.Where("tmdb_id = ?", 2).First(&show2).Error, "healthy show still synced")
	var count int64
	require.NoError(t, gdb.Model(&models.Episode{}).Where("show_id = ?", show2.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	logs := loadSyncLogs(t, gdb)
	require.Len(t, logs, 1)
	errs := syncErrors(t, logs[0])
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "show 1")
	assert.Equal(t, 1, logs[0].ShowCount, "only the healthy show counts as synced")
}

func TestRunConcurrentSecondRunSkips(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	base := showFixture(t, 1, "", []tmdb.Episode{
		{SeasonNumber: 1, EpisodeNumber: 1, Name: "Ep1", AirDate: "2024-01-01"},
	})
	mux := http.NewServeMux()
	var once bool
	mux.HandleFunc("/tv/1", func(w http.ResponseWriter, r *http.Request) {
		if !once {
			once = true
			close(entered)
			<-release
		}
		base.ServeHTTP(w, r)
	})
	mux.Handle("/", base)
	eng, gdb, _ := newEngine(t, mux)
	track(t, gdb, 1, 1, "WATCHING")

	firstDone := make(chan error, 1)
	go func() { firstDone <- eng.Run(context.Background()) }()

	<-entered // first run is inside TMDB fetch, holding the jobs lock
	err := eng.Run(context.Background())
	require.ErrorIs(t, err, ErrSkipped, "overlapping run must skip (E18)")

	close(release)
	require.NoError(t, <-firstDone)

	logs := loadSyncLogs(t, gdb)
	assert.Len(t, logs, 1, "skipped run writes no SyncLog")
}

func TestRunPrunesUpstreamDeletedEpisodeAndWatches(t *testing.T) {
	// Upstream season 1 now only has episode 1; local knows episodes 1 and 2
	// (both watched) plus a watched episode in season 2, which the show
	// listing no longer mentions at all.
	handler := showFixture(t, 1, "", []tmdb.Episode{
		{SeasonNumber: 1, EpisodeNumber: 1, Name: "Ep1", AirDate: "2024-01-01"},
	})
	eng, gdb, _ := newEngine(t, handler)
	track(t, gdb, 1, 1, "WATCHING")

	show := models.Show{TMDBID: 1, Title: "Show 1"}
	require.NoError(t, gdb.Create(&show).Error)
	ep1 := models.Episode{ShowID: show.ID, Season: 1, Number: 1, Title: "Ep1", AirDate: date(t, "2024-01-01")}
	ep2 := models.Episode{ShowID: show.ID, Season: 1, Number: 2, Title: "Ep2", AirDate: date(t, "2024-01-08")}
	s2e1 := models.Episode{ShowID: show.ID, Season: 2, Number: 1, Title: "S2E1", AirDate: date(t, "2024-06-01")}
	for _, e := range []*models.Episode{&ep1, &ep2, &s2e1} {
		require.NoError(t, gdb.Create(e).Error)
		require.NoError(t, gdb.Create(&models.EpisodeWatch{UserID: 1, EpisodeID: e.ID, WatchedAt: time.Now()}).Error)
	}

	require.NoError(t, eng.Run(context.Background()))

	var eps []models.Episode
	require.NoError(t, gdb.Where("show_id = ?", show.ID).Find(&eps).Error)
	ids := map[uint]bool{}
	for _, e := range eps {
		ids[e.ID] = true
	}
	assert.True(t, ids[ep1.ID], "still-listed episode kept")
	assert.False(t, ids[ep2.ID], "upstream-deleted episode removed (E17)")
	assert.True(t, ids[s2e1.ID], "season absent from listing is not pruned (conservative)")

	var watches []models.EpisodeWatch
	require.NoError(t, gdb.Find(&watches).Error)
	watched := map[uint]bool{}
	for _, w := range watches {
		watched[w.EpisodeID] = true
	}
	assert.True(t, watched[ep1.ID])
	assert.False(t, watched[ep2.ID], "orphaned watch row pruned with its episode (E17)")
	assert.True(t, watched[s2e1.ID])
}

func TestRunSeasonFetchFailureDoesNotPrune(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tv/1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, tmdb.Show{
			ID: 1, Name: "Show 1", Status: "Returning Series",
			Seasons: []tmdb.SeasonSummary{{ID: 10, SeasonNumber: 1, EpisodeCount: 2}},
		})
	})
	mux.HandleFunc("/tv/1/season/1", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	eng, gdb, _ := newEngine(t, mux)
	track(t, gdb, 1, 1, "WATCHING")

	show := models.Show{TMDBID: 1, Title: "Show 1"}
	require.NoError(t, gdb.Create(&show).Error)
	ep := models.Episode{ShowID: show.ID, Season: 1, Number: 1, Title: "Ep1", AirDate: date(t, "2024-01-01")}
	require.NoError(t, gdb.Create(&ep).Error)

	require.NoError(t, eng.Run(context.Background()))

	var count int64
	require.NoError(t, gdb.Model(&models.Episode{}).Where("show_id = ?", show.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "no season listing fetched, so nothing is pruned")

	logs := loadSyncLogs(t, gdb)
	require.Len(t, logs, 1)
	errs := syncErrors(t, logs[0])
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "season 1")
}

func TestRunKeepsWatchedEpisodeWhenAirDateMovesFuture(t *testing.T) {
	future := time.Now().AddDate(0, 6, 0).Format("2006-01-02")
	handler := showFixture(t, 1, "", []tmdb.Episode{
		{SeasonNumber: 1, EpisodeNumber: 1, Name: "Ep1", AirDate: future},
	})
	eng, gdb, _ := newEngine(t, handler)
	track(t, gdb, 1, 1, "WATCHING")

	show := models.Show{TMDBID: 1, Title: "Show 1"}
	require.NoError(t, gdb.Create(&show).Error)
	ep := models.Episode{ShowID: show.ID, Season: 1, Number: 1, Title: "Ep1", AirDate: date(t, "2024-01-01")}
	require.NoError(t, gdb.Create(&ep).Error)
	watch := models.EpisodeWatch{UserID: 1, EpisodeID: ep.ID, WatchedAt: time.Now()}
	require.NoError(t, gdb.Create(&watch).Error)

	require.NoError(t, eng.Run(context.Background()))

	var got models.Episode
	require.NoError(t, gdb.First(&got, ep.ID).Error, "episode retained despite future air date (E19)")
	require.NotNil(t, got.AirDate)
	assert.True(t, got.AirDate.After(time.Now()), "air date updated to the future value")

	var gotWatch models.EpisodeWatch
	require.NoError(t, gdb.First(&gotWatch, watch.ID).Error, "watch row kept — user really watched it (E19)")
}

func TestScheduleRegistersNightlyEntry(t *testing.T) {
	handler := showFixture(t, 1, "", nil)
	eng, _, _ := newEngine(t, handler)

	c := cron.New()
	require.NoError(t, eng.Schedule(c))
	require.Len(t, c.Entries(), 1)

	// The registered spec must be 03:00 daily: from an arbitrary reference
	// time, the next activation is the following 03:00.
	ref := time.Date(2026, 7, 5, 12, 0, 0, 0, time.Local)
	next := c.Entries()[0].Schedule.Next(ref)
	assert.Equal(t, time.Date(2026, 7, 6, 3, 0, 0, 0, time.Local), next)
}
