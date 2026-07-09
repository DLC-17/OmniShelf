package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/books"
	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// notesTestRouter wires the note endpoints against a real DB. The book service
// needs no metadata/image clients here: the note handlers only touch the DB.
func notesTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := gin.New()
	grp := RegisterRoutes(r, gdb, &config.Config{JWTSecret: testSecret})
	RegisterNoteRoutes(grp, books.NewService(gdb, nil, nil))
	return r, gdb
}

// seedBookItem inserts a BOOK TrackingItem for the user and returns its ID.
func seedBookItem(t *testing.T, gdb *gorm.DB, userID uint) uint {
	t.Helper()
	it := models.TrackingItem{UserID: userID, Type: "BOOK", ExternalID: "9780441172719", Title: "Dune", Status: "READING"}
	require.NoError(t, gdb.Create(&it).Error)
	return it.ID
}

type notePayload struct {
	ID        uint      `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func TestNotesAddListDeleteFlow(t *testing.T) {
	r, gdb := notesTestRouter(t)
	uid, cookie := socialSeedUser(t, gdb, "alice")
	itemID := seedBookItem(t, gdb, uid)
	base := "/api/items/" + userPath(itemID) + "/notes"

	// Add two notes.
	w := doJSON(r, http.MethodPost, base, map[string]string{"body": "started reading"}, cookie)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created notePayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "started reading", created.Body)
	assert.NotZero(t, created.ID)

	w = doJSON(r, http.MethodPost, base, map[string]string{"body": "loved the ending"}, cookie)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// List: newest first.
	w = doJSON(r, http.MethodGet, base, nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var notes []notePayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &notes))
	require.Len(t, notes, 2)
	assert.Equal(t, "loved the ending", notes[0].Body)
	assert.Equal(t, "started reading", notes[1].Body)

	// Delete the first-created note.
	w = doJSON(r, http.MethodDelete, base+"/"+userPath(created.ID), nil, cookie)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	w = doJSON(r, http.MethodGet, base, nil, cookie)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &notes))
	require.Len(t, notes, 1)
	assert.Equal(t, "loved the ending", notes[0].Body)
}

func TestNotesEmptyBodyRejected(t *testing.T) {
	r, gdb := notesTestRouter(t)
	uid, cookie := socialSeedUser(t, gdb, "alice")
	itemID := seedBookItem(t, gdb, uid)

	w := doJSON(r, http.MethodPost, "/api/items/"+userPath(itemID)+"/notes",
		map[string]string{"body": "   "}, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)
	assertEnvelope(t, w, CodeInvalidRequest)
}

// A user cannot read, add, or delete notes on another user's item; it looks
// like a missing item (no existence leak).
func TestNotesOwnershipEnforced(t *testing.T) {
	r, gdb := notesTestRouter(t)
	alice, _ := socialSeedUser(t, gdb, "alice")
	_, bobCookie := socialSeedUser(t, gdb, "bob")
	itemID := seedBookItem(t, gdb, alice)
	base := "/api/items/" + userPath(itemID) + "/notes"

	w := doJSON(r, http.MethodGet, base, nil, bobCookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertEnvelope(t, w, CodeNotFound)

	w = doJSON(r, http.MethodPost, base, map[string]string{"body": "hi"}, bobCookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertEnvelope(t, w, CodeNotFound)
}

func TestNotesRequireAuth(t *testing.T) {
	r, gdb := notesTestRouter(t)
	uid, _ := socialSeedUser(t, gdb, "alice")
	itemID := seedBookItem(t, gdb, uid)

	w := doJSON(r, http.MethodGet, "/api/items/"+userPath(itemID)+"/notes", nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assertEnvelope(t, w, CodeUnauthorized)
}

func TestNotesDeleteUnknownNote(t *testing.T) {
	r, gdb := notesTestRouter(t)
	uid, cookie := socialSeedUser(t, gdb, "alice")
	itemID := seedBookItem(t, gdb, uid)

	w := doJSON(r, http.MethodDelete, "/api/items/"+userPath(itemID)+"/notes/9999", nil, cookie)
	require.Equal(t, http.StatusNotFound, w.Code)
	assertEnvelope(t, w, CodeNoteNotFound)
}
