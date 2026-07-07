package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// userPath renders a user ID as a path segment.
func userPath(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

type memberPayload struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Counts   struct {
		TV    int64 `json:"tv"`
		Books int64 `json:"books"`
	} `json:"counts"`
}

type libraryItemPayload struct {
	ID         uint      `json:"id"`
	Type       string    `json:"type"`
	ExternalID string    `json:"externalId"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func TestUsersListWithCounts(t *testing.T) {
	r, gdb := socialTestRouter(t)
	u1, cookie := socialSeedUser(t, gdb, "alice")
	u2, _ := socialSeedUser(t, gdb, "bob")
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	socialSeedItem(t, gdb, u1, "TV", "1399", "Game of Thrones", "WATCHING", base)
	socialSeedItem(t, gdb, u1, "TV", "2316", "Severance", "PLAN_TO", base)
	socialSeedItem(t, gdb, u1, "BOOK", "9780441172719", "Dune", "READING", base)
	socialSeedItem(t, gdb, u2, "BOOK", "9780553283686", "Hyperion", "COMPLETED", base)

	w := doJSON(r, http.MethodGet, "/api/users", nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var members []memberPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &members))
	require.Len(t, members, 2)

	assert.Equal(t, u1, members[0].ID)
	assert.Equal(t, "alice", members[0].Username)
	assert.EqualValues(t, 2, members[0].Counts.TV)
	assert.EqualValues(t, 1, members[0].Counts.Books)

	assert.Equal(t, u2, members[1].ID)
	assert.Equal(t, "bob", members[1].Username)
	assert.EqualValues(t, 0, members[1].Counts.TV)
	assert.EqualValues(t, 1, members[1].Counts.Books)
}

func TestUsersRequiresAuth(t *testing.T) {
	r, _ := socialTestRouter(t)
	for _, path := range []string{"/api/users", "/api/users/1/library"} {
		w := doJSON(r, http.MethodGet, path, nil)
		require.Equal(t, http.StatusUnauthorized, w.Code, path)
		assertEnvelope(t, w, CodeUnauthorized)
	}
}

// TestUserLibraryReadOnlyView: alice reads bob's shelf (cross-user reads are
// allowed), with type/status filters, same shape as /api/library.
func TestUserLibraryReadOnlyView(t *testing.T) {
	r, gdb := socialTestRouter(t)
	_, aliceCookie := socialSeedUser(t, gdb, "alice")
	bob, _ := socialSeedUser(t, gdb, "bob")
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	socialSeedItem(t, gdb, bob, "TV", "1438", "The Wire", "COMPLETED", base.Add(time.Minute))
	socialSeedItem(t, gdb, bob, "BOOK", "9780441172719", "Dune", "READING", base.Add(2*time.Minute))

	fetch := func(query string) []libraryItemPayload {
		w := doJSON(r, http.MethodGet, "/api/users/"+userPath(bob)+"/library"+query, nil, aliceCookie)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var items []libraryItemPayload
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
		return items
	}

	all := fetch("")
	require.Len(t, all, 2)
	// Alphabetical by title; every field of the shared item shape is populated.
	assert.Equal(t, "Dune", all[0].Title)
	assert.Equal(t, "BOOK", all[0].Type)
	assert.Equal(t, "9780441172719", all[0].ExternalID)
	assert.Equal(t, "READING", all[0].Status)
	assert.Equal(t, "The Wire", all[1].Title)

	tvOnly := fetch("?type=TV")
	require.Len(t, tvOnly, 1)
	assert.Equal(t, "The Wire", tvOnly[0].Title)

	completed := fetch("?status=COMPLETED")
	require.Len(t, completed, 1)
	assert.Equal(t, "The Wire", completed[0].Title)

	both := fetch("?type=BOOK&status=READING")
	require.Len(t, both, 1)
	assert.Equal(t, "Dune", both[0].Title)

	// Invalid filters → 400 envelope.
	for _, q := range []string{"?type=MOVIE", "?status=BINGING"} {
		w := doJSON(r, http.MethodGet, "/api/users/"+userPath(bob)+"/library"+q, nil, aliceCookie)
		require.Equal(t, http.StatusBadRequest, w.Code, q)
		assertEnvelope(t, w, CodeInvalidRequest)
	}
}

func TestUserLibraryUnknownUser(t *testing.T) {
	r, gdb := socialTestRouter(t)
	_, cookie := socialSeedUser(t, gdb, "alice")

	w := doJSON(r, http.MethodGet, "/api/users/9999/library", nil, cookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertEnvelope(t, w, codeUserNotFound)

	w = doJSON(r, http.MethodGet, "/api/users/not-a-number/library", nil, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assertEnvelope(t, w, CodeInvalidRequest)
}

// TestUsersRoutesAreReadOnly asserts no mutating verbs are routed under
// /api/users (visibility only; writes stay own-account).
func TestUsersRoutesAreReadOnly(t *testing.T) {
	r, gdb := socialTestRouter(t)
	bob, cookie := socialSeedUser(t, gdb, "bob")
	socialSeedItem(t, gdb, bob, "TV", "1438", "The Wire", "COMPLETED",
		time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, path := range []string{"/api/users", "/api/users/" + userPath(bob), "/api/users/" + userPath(bob) + "/library"} {
			w := doJSON(r, method, path, map[string]string{"status": "PLAN_TO"}, cookie)
			assert.Equal(t, http.StatusNotFound, w.Code, "%s %s must not be routed", method, path)
		}
	}
}
