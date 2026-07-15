package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return b
}

// newTestClient returns a client pointed at srv with an effectively
// unlimited rate limiter and near-zero backoff so tests run fast.
func newTestClient(srv *httptest.Server, opts ...Option) *Client {
	base := []Option{
		WithBaseURL(srv.URL),
		WithRateLimit(rate.NewLimiter(rate.Inf, 1)),
		WithBackoffBase(time.Millisecond),
	}
	return New("test-key", append(base, opts...)...)
}

func TestSearchTV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/search/tv", r.URL.Path)
		assert.Equal(t, "game of thrones", r.URL.Query().Get("query"))
		assert.Equal(t, "test-key", r.URL.Query().Get("api_key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "search_tv.json"))
	}))
	defer srv.Close()

	res, err := newTestClient(srv).SearchTV(context.Background(), "game of thrones")
	require.NoError(t, err)
	require.Len(t, res.Results, 2)
	assert.Equal(t, 1399, res.Results[0].ID)
	assert.Equal(t, "Game of Thrones", res.Results[0].Name)
	assert.Equal(t, "/1XS1oqL89opfnbLl8WnZY1O1uJx.jpg", res.Results[0].PosterPath)
	// null poster_path in fixture decodes to empty string
	assert.Equal(t, "", res.Results[1].PosterPath)
	assert.Equal(t, 2, res.TotalResults)
}

func TestSearchMovie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/search/movie", r.URL.Path)
		assert.Equal(t, "inception", r.URL.Query().Get("query"))
		assert.Equal(t, "test-key", r.URL.Query().Get("api_key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"page":1,"results":[{"id":27205,"title":"Inception","overview":"A thief.","release_date":"2010-07-15","poster_path":"/x.jpg"}],"total_results":1,"total_pages":1}`))
	}))
	defer srv.Close()

	res, err := newTestClient(srv).SearchMovie(context.Background(), "inception")
	require.NoError(t, err)
	require.Len(t, res.Results, 1)
	assert.Equal(t, 27205, res.Results[0].ID)
	assert.Equal(t, "Inception", res.Results[0].Title)
	assert.Equal(t, "2010-07-15", res.Results[0].ReleaseDate)
	assert.Equal(t, "/x.jpg", res.Results[0].PosterPath)
}

func TestGetMovie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/movie/27205", r.URL.Path)
		assert.Equal(t, "keywords", r.URL.Query().Get("append_to_response"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":27205,"title":"Inception","overview":"A thief.","status":"Released","release_date":"2010-07-15","poster_path":"/x.jpg","keywords":{"keywords":[{"id":1,"name":"dream"},{"id":2,"name":"heist"}]}}`))
	}))
	defer srv.Close()

	m, err := newTestClient(srv).GetMovie(context.Background(), 27205)
	require.NoError(t, err)
	assert.Equal(t, 27205, m.ID)
	assert.Equal(t, "Inception", m.Title)
	assert.Equal(t, "Released", m.Status)
	assert.Equal(t, "/x.jpg", m.PosterPath)
	assert.Equal(t, []string{"dream", "heist"}, m.TagNames())
}

func TestMovieRecommendations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/movie/27205/recommendations", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"page":1,"results":[{"id":348350,"title":"Solo","release_date":"2018-05-15","poster_path":"/s.jpg"}],"total_results":1,"total_pages":1}`))
	}))
	defer srv.Close()

	res, err := newTestClient(srv).MovieRecommendations(context.Background(), 27205)
	require.NoError(t, err)
	require.Len(t, res.Results, 1)
	assert.Equal(t, 348350, res.Results[0].ID)
	assert.Equal(t, "Solo", res.Results[0].Title)
}

// TV keywords arrive under keywords.results (movies use keywords.keywords);
// GetShow requests them via append_to_response and TagNames flattens them.
func TestGetShowKeywords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/tv/1399", r.URL.Path)
		assert.Equal(t, "keywords", r.URL.Query().Get("append_to_response"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1399,"name":"Game of Thrones","status":"Ended","keywords":{"results":[{"id":1,"name":"dragon"},{"id":2,"name":"based on novel or book"}]}}`))
	}))
	defer srv.Close()

	show, err := newTestClient(srv).GetShow(context.Background(), 1399)
	require.NoError(t, err)
	assert.Equal(t, []string{"dragon", "based on novel or book"}, show.TagNames())
}

func TestGetShow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/tv/1399", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "show_1399.json"))
	}))
	defer srv.Close()

	show, err := newTestClient(srv).GetShow(context.Background(), 1399)
	require.NoError(t, err)
	assert.Equal(t, 1399, show.ID)
	assert.Equal(t, "Game of Thrones", show.Name)
	assert.Equal(t, "Ended", show.Status)
	assert.Equal(t, "/1XS1oqL89opfnbLl8WnZY1O1uJx.jpg", show.PosterPath)
	require.Len(t, show.Seasons, 3)
	assert.Equal(t, 1, show.Seasons[1].SeasonNumber)
	assert.Equal(t, 10, show.Seasons[1].EpisodeCount)
}

func TestGetSeason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/tv/1399/season/1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "season_1399_1.json"))
	}))
	defer srv.Close()

	season, err := newTestClient(srv).GetSeason(context.Background(), 1399, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, season.SeasonNumber)
	require.Len(t, season.Episodes, 3)
	assert.Equal(t, "Winter Is Coming", season.Episodes[0].Name)
	assert.Equal(t, "2011-04-17", season.Episodes[0].AirDate)
	// null air_date (unannounced episode) decodes to empty string
	assert.Equal(t, "", season.Episodes[2].AirDate)
}

func Test429BackoffThenSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"status_code":25,"status_message":"Your request count is over the allowed limit."}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "show_1399.json"))
	}))
	defer srv.Close()

	show, err := newTestClient(srv).GetShow(context.Background(), 1399)
	require.NoError(t, err)
	assert.Equal(t, 1399, show.ID)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls), "expected two 429s then a success")
}

func Test429ThreeTimesFails(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := newTestClient(srv).GetShow(context.Background(), 1399)
	require.Error(t, err)
	var se *StatusError
	require.True(t, errors.As(err, &se))
	assert.Equal(t, http.StatusTooManyRequests, se.StatusCode)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls), "must stop after 3 attempts")
}

func TestRateLimiterHonored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "show_1399.json"))
	}))
	defer srv.Close()

	// 20 req/s limiter, burst 1: 5 sequential requests must take at least
	// 4 inter-request intervals (~200ms). Generous lower bound of 150ms to
	// avoid timer-resolution flakiness.
	c := New("test-key",
		WithBaseURL(srv.URL),
		WithRateLimit(rate.NewLimiter(rate.Limit(20), 1)),
		WithBackoffBase(time.Millisecond),
	)
	start := time.Now()
	for i := 0; i < 5; i++ {
		_, err := c.GetShow(context.Background(), 1399)
		require.NoError(t, err)
	}
	assert.GreaterOrEqual(t, time.Since(start), 150*time.Millisecond,
		"5 requests through a 20 req/s limiter should be paced")
}

func TestDefaultRateLimiterIsFourPerSecond(t *testing.T) {
	c := New("k")
	assert.Equal(t, rate.Limit(4), c.limiter.Limit())
}

func TestNon200Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status_code":34,"status_message":"The resource you requested could not be found."}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv).GetShow(context.Background(), 999999)
	require.Error(t, err)
	var se *StatusError
	require.True(t, errors.As(err, &se))
	assert.Equal(t, http.StatusNotFound, se.StatusCode)
}
