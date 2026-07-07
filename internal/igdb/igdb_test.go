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
	ts := &testServer{gameBody: `[{"id":7346,"name":"The Legend of Zelda: Breath of the Wild","summary":"An open-world adventure.","cover":{"image_id":"co3p2d"}}]`}
	c := newClient(t, ts)

	game, err := c.GetGame(context.Background(), 7346)
	require.NoError(t, err)
	require.NotNil(t, game)
	assert.Equal(t, 7346, game.ID)
	assert.Equal(t, "The Legend of Zelda: Breath of the Wild", game.Name)
	assert.Equal(t, "An open-world adventure.", game.Summary)
	assert.Equal(t, "co3p2d", game.CoverImageID)
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
