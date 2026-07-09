package igdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testServer stands in for both the Twitch token endpoint and the IGDB API.
type testServer struct {
	tokenHits int
	gameBody  string
}

func (ts *testServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			ts.tokenHits++
			_, _ = w.Write([]byte(`{"access_token":"tok123","expires_in":5000000,"token_type":"bearer"}`))
		case "/games":
			if r.Header.Get("Authorization") != "Bearer tok123" || r.Header.Get("Client-ID") != "cid" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(ts.gameBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func newClient(t *testing.T, ts *testServer) *Client {
	t.Helper()
	srv := httptest.NewServer(ts.handler())
	t.Cleanup(srv.Close)
	return New("cid", "secret",
		WithAuthURL(srv.URL+"/oauth2/token"),
		WithAPIURL(srv.URL),
	)
}

func TestGetGame(t *testing.T) {
	// first_release_date 1488499200 = 2017-03-03 UTC.
	ts := &testServer{gameBody: `[{"id":7346,"name":"The Legend of Zelda: Breath of the Wild","summary":"An open-world adventure.","first_release_date":1488499200,"cover":{"image_id":"co3p2d"},"genres":[{"name":"Adventure"},{"name":"Role-playing (RPG)"}],"keywords":[{"name":"open world"}]}]`}
	c := newClient(t, ts)

	game, err := c.GetGame(context.Background(), 7346)
	require.NoError(t, err)
	require.NotNil(t, game)
	assert.Equal(t, 7346, game.ID)
	assert.Equal(t, "The Legend of Zelda: Breath of the Wild", game.Name)
	assert.Equal(t, "An open-world adventure.", game.Summary)
	assert.Equal(t, "co3p2d", game.CoverImageID)
	assert.Equal(t, "2017-03-03", game.ReleaseDate)
	assert.Equal(t, []string{"Adventure", "Role-playing (RPG)"}, game.Genres)
	assert.Equal(t, []string{"open world"}, game.Keywords)
	// Tags flattens genres followed by keywords for tag persistence.
	assert.Equal(t, []string{"Adventure", "Role-playing (RPG)", "open world"}, game.Tags())
}

// A game with no first_release_date leaves ReleaseDate empty (not epoch zero).
func TestGetGameNoReleaseDate(t *testing.T) {
	ts := &testServer{gameBody: `[{"id":42,"name":"Unannounced","cover":{"image_id":"x"}}]`}
	c := newClient(t, ts)

	game, err := c.GetGame(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, game)
	assert.Equal(t, "", game.ReleaseDate)
}

// The token is fetched once and reused across calls.
func TestTokenCached(t *testing.T) {
	ts := &testServer{gameBody: `[{"id":1,"name":"X","cover":{"image_id":"abc"}}]`}
	c := newClient(t, ts)

	_, err := c.GetGame(context.Background(), 1)
	require.NoError(t, err)
	_, err = c.GetGame(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, 1, ts.tokenHits, "token should be cached and reused")
}

// An empty result set is not an error — the game just lacks IGDB metadata.
func TestGetGameEmpty(t *testing.T) {
	ts := &testServer{gameBody: `[]`}
	c := newClient(t, ts)

	game, err := c.GetGame(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, game)
}

func TestSearchGames(t *testing.T) {
	// The stub /games handler echoes gameBody for both GetGame and SearchGames.
	ts := &testServer{gameBody: `[{"id":7346,"name":"Zelda","first_release_date":1488499200,"cover":{"image_id":"co3p2d"}},{"id":1234,"name":"Zelda II"}]`}
	c := newClient(t, ts)

	results, err := c.SearchGames(context.Background(), "zelda")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, 7346, results[0].ID)
	assert.Equal(t, "Zelda", results[0].Name)
	assert.Equal(t, 2017, results[0].Year)
	assert.Equal(t, "co3p2d", results[0].CoverImageID)
	// No first_release_date → year 0.
	assert.Equal(t, 0, results[1].Year)
}

func TestSearchGamesUnconfigured(t *testing.T) {
	c := New("", "")
	_, err := c.SearchGames(context.Background(), "zelda")
	require.ErrorIs(t, err, ErrUnconfigured)
}

func TestSimilarGames(t *testing.T) {
	// The stub /games handler echoes gameBody; here it is the similar_games
	// expansion for two seed games. first_release_date 1136073600 = 2006-01-01.
	ts := &testServer{gameBody: `[
		{"id":7346,"similar_games":[
			{"id":1234,"name":"Okami","first_release_date":1136073600,"cover":{"image_id":"cov1"}},
			{"id":5678,"name":"Ico"}
		]},
		{"id":9999,"similar_games":[
			{"id":4321,"name":"Shadow of the Colossus","cover":{"image_id":"cov3"}}
		]}
	]`}
	c := newClient(t, ts)

	got, err := c.SimilarGames(context.Background(), []int{7346, 9999})
	require.NoError(t, err)
	require.Len(t, got, 2)

	seed := got[7346]
	require.Len(t, seed, 2)
	assert.Equal(t, 1234, seed[0].ID)
	assert.Equal(t, "Okami", seed[0].Name)
	assert.Equal(t, 2006, seed[0].Year)
	assert.Equal(t, "cov1", seed[0].CoverImageID)
	// No first_release_date → year 0; no cover → empty image id.
	assert.Equal(t, 0, seed[1].Year)
	assert.Equal(t, "", seed[1].CoverImageID)

	assert.Equal(t, 4321, got[9999][0].ID)
}

// An empty seed list short-circuits without a round-trip and yields an empty map.
func TestSimilarGamesEmptySeeds(t *testing.T) {
	ts := &testServer{gameBody: `[]`}
	c := newClient(t, ts)

	got, err := c.SimilarGames(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, 0, ts.tokenHits, "no seeds means no token fetch or request")
}

func TestSimilarGamesUnconfigured(t *testing.T) {
	c := New("", "")
	_, err := c.SimilarGames(context.Background(), []int{1})
	require.ErrorIs(t, err, ErrUnconfigured)
}

func TestCoverURL(t *testing.T) {
	c := New("cid", "secret")
	assert.Equal(t, "https://images.igdb.com/igdb/image/upload/t_cover_big/co3p2d.jpg", c.CoverURL("co3p2d", ""))
	assert.Equal(t, "", c.CoverURL("", ""))
}

func TestUnconfigured(t *testing.T) {
	c := New("", "")
	assert.False(t, c.Configured())
	_, err := c.GetGame(context.Background(), 1)
	require.ErrorIs(t, err, ErrUnconfigured)
}
