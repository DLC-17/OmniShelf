package musicbrainz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMBID = "0a2d5b1c-1111-2222-3333-444455556666"

// server stands in for the MusicBrainz web service.
type server struct {
	searchBody string
	lookupBody string
	gotUA      string
}

func (s *server) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.gotUA = r.Header.Get("User-Agent")
		switch {
		case r.URL.Path == "/release-group" && r.URL.Query().Get("query") != "":
			_, _ = w.Write([]byte(s.searchBody))
		case strings.HasPrefix(r.URL.Path, "/release-group/"):
			if s.lookupBody == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(s.lookupBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func newClient(t *testing.T, s *server) *Client {
	t.Helper()
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return New("dev@example.com", WithBaseURL(srv.URL))
}

func TestSearch(t *testing.T) {
	s := &server{searchBody: `{"release-groups":[
		{"id":"` + testMBID + `","title":"Abbey Road","first-release-date":"1969-09-26","primary-type":"Album","artist-credit":[{"name":"The Beatles"}]},
		{"id":"x","title":"Collab","first-release-date":"2001","artist-credit":[{"name":"A"},{"name":"B"}]}
	]}`}
	c := newClient(t, s)

	res, err := c.Search(context.Background(), "abbey road", 10)
	require.NoError(t, err)
	require.Len(t, res, 2)
	assert.Equal(t, testMBID, res[0].MBID)
	assert.Equal(t, "Abbey Road", res[0].Title)
	assert.Equal(t, "The Beatles", res[0].Artist)
	assert.Equal(t, 1969, res[0].Year)
	assert.Equal(t, "A, B", res[1].Artist, "collaborations join artist-credit names")
	assert.Equal(t, 2001, res[1].Year, "year-only date parses")
	assert.Contains(t, s.gotUA, "OmniShelf")
	assert.Contains(t, s.gotUA, "dev@example.com")
}

func TestGetReleaseGroup(t *testing.T) {
	s := &server{lookupBody: `{"id":"` + testMBID + `","title":"Abbey Road","first-release-date":"1969-09-26","artist-credit":[{"name":"The Beatles"}]}`}
	c := newClient(t, s)

	rg, err := c.GetReleaseGroup(context.Background(), testMBID)
	require.NoError(t, err)
	assert.Equal(t, testMBID, rg.MBID)
	assert.Equal(t, "The Beatles", rg.Artist)
	assert.Equal(t, 1969, rg.Year)
}

func TestGetReleaseGroupNotFound(t *testing.T) {
	s := &server{} // empty lookupBody → 404
	c := newClient(t, s)

	_, err := c.GetReleaseGroup(context.Background(), testMBID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCoverURL(t *testing.T) {
	c := New("dev@example.com")
	assert.Equal(t, "https://coverartarchive.org/release-group/"+testMBID+"/front-500", c.CoverURL(testMBID, 0))
	assert.Equal(t, "https://coverartarchive.org/release-group/"+testMBID+"/front-250", c.CoverURL(testMBID, 250))
	assert.Equal(t, "", c.CoverURL("", 500))
}
