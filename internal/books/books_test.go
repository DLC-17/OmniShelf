package books

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
	"github.com/davidlc1229/omnishelf/internal/openlibrary"
)

const (
	testISBN        = "9780306406157"
	testPartialISBN = "9791234567890"
)

// fakeMetadata is a canned MetadataClient.
type fakeMetadata struct {
	books     map[string]*openlibrary.Book
	titles    []openlibrary.TitleResult
	byAuthor  map[string][]openlibrary.TitleResult
	bySubject map[string][]openlibrary.TitleResult
	editions  map[string][]openlibrary.Edition
	err       error
}

func (f *fakeMetadata) GetByISBN(_ context.Context, isbn string) (*openlibrary.Book, error) {
	if f.err != nil {
		return nil, f.err
	}
	b, ok := f.books[isbn]
	if !ok {
		return nil, &openlibrary.NotFoundError{ISBN: isbn}
	}
	return b, nil
}

func (f *fakeMetadata) SearchByTitle(_ context.Context, _ string) ([]openlibrary.TitleResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.titles, nil
}

func (f *fakeMetadata) SearchByAuthor(_ context.Context, authorName string) ([]openlibrary.TitleResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byAuthor[authorName], nil
}

func (f *fakeMetadata) SearchBySubject(_ context.Context, subject string) ([]openlibrary.TitleResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.bySubject[subject], nil
}

func (f *fakeMetadata) ListEditions(_ context.Context, workKey string) ([]openlibrary.Edition, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.editions[workKey], nil
}

func (f *fakeMetadata) CoverURL(coverID int, _ string) string {
	if coverID == 0 {
		return ""
	}
	return "http://covers.test/cover.jpg"
}

// fakeImages is a canned ImageStore.
type fakeImages struct {
	err     error
	fetched []string // externalIDs fetched
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
	// Close before TempDir cleanup: an open SQLite handle blocks directory
	// removal on Windows.
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gdb
}

func fullMeta() *openlibrary.Book {
	return &openlibrary.Book{
		ISBN13:    testISBN,
		Title:     "Networking Basics",
		Authors:   []string{"Jane Doe", "John Roe"},
		PageCount: 320,
		CoverID:   12345,
	}
}

func newTestService(t *testing.T, meta *fakeMetadata, imgs *fakeImages) (*Service, *gorm.DB) {
	t.Helper()
	gdb := testDB(t)
	return NewService(gdb, meta, imgs), gdb
}

func TestScanHappyPath(t *testing.T) {
	meta := &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}
	imgs := &fakeImages{}
	svc, gdb := newTestService(t, meta, imgs)

	book, err := svc.Scan(context.Background(), "978-0-306-40615-7") // hyphenated input normalizes
	require.NoError(t, err)
	assert.Equal(t, testISBN, book.ISBN13)
	assert.Equal(t, "Networking Basics", book.Title)
	assert.Equal(t, "Jane Doe, John Roe", book.Authors)
	assert.Equal(t, 320, book.PageCount)
	assert.Equal(t, "book/"+testISBN+".jpg", book.CoverPath)
	assert.Equal(t, []string{testISBN}, imgs.fetched)

	var stored models.Book
	require.NoError(t, gdb.Where(&models.Book{ISBN13: testISBN}).First(&stored).Error)
	assert.Equal(t, book.ID, stored.ID)
}

// OpenLibrary subjects are persisted as source-derived tags on the book and
// surfaced on the library DTO by ListLibrary.
func TestScanPersistsSubjectsAndListsTags(t *testing.T) {
	meta := fullMeta()
	meta.Subjects = []string{"Computer networks", "Fiction"}
	svc, _ := newTestService(t, &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: meta}}, &fakeImages{})
	ctx := context.Background()

	book, err := svc.Scan(ctx, testISBN)
	require.NoError(t, err)
	_, err = svc.Track(ctx, 1, book.ID, StatusReading)
	require.NoError(t, err)

	entries, err := svc.ListLibrary(ctx, 1, TypeBook, "")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, []string{"Computer networks", "Fiction"}, entries[0].Tags)
}

// An item with no source tags serializes an empty (non-nil) tag list.
func TestListLibraryTagsDefaultEmpty(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}, &fakeImages{})
	ctx := context.Background()

	book, err := svc.Scan(ctx, testISBN)
	require.NoError(t, err)
	_, err = svc.Track(ctx, 1, book.ID, StatusReading)
	require.NoError(t, err)

	entries, err := svc.ListLibrary(ctx, 1, TypeBook, "")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.NotNil(t, entries[0].Tags)
	assert.Empty(t, entries[0].Tags)
}

func TestScanInvalidISBN(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{}, &fakeImages{})
	for _, isbn := range []string{"", "12345", "12345678901234", "978030640615X", "1234567890123"} {
		_, err := svc.Scan(context.Background(), isbn)
		assert.ErrorIs(t, err, ErrInvalidISBN, "isbn %q", isbn)
	}
}

func TestScanUnknownISBN(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{books: map[string]*openlibrary.Book{}}, &fakeImages{})
	_, err := svc.Scan(context.Background(), testISBN)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestScanUpstreamFailure(t *testing.T) {
	meta := &fakeMetadata{err: errors.New("connection refused")}
	svc, _ := newTestService(t, meta, &fakeImages{})
	_, err := svc.Scan(context.Background(), testISBN)
	assert.ErrorIs(t, err, ErrUpstream)
}

// E13: a failed cover download saves the book with an empty CoverPath.
func TestScanCoverFailureIsNonFatal(t *testing.T) {
	meta := &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}
	imgs := &fakeImages{err: errors.New("cover host down")}
	svc, _ := newTestService(t, meta, imgs)

	book, err := svc.Scan(context.Background(), testISBN)
	require.NoError(t, err)
	assert.Empty(t, book.CoverPath)
	assert.Equal(t, "Networking Basics", book.Title)
}

// E5: metadata with only a title (no work, cover, authors, pages) still saves.
func TestScanPartialMetadata(t *testing.T) {
	meta := &fakeMetadata{books: map[string]*openlibrary.Book{
		testPartialISBN: {ISBN13: testPartialISBN, Title: "Bare Edition"},
	}}
	imgs := &fakeImages{}
	svc, _ := newTestService(t, meta, imgs)

	book, err := svc.Scan(context.Background(), testPartialISBN)
	require.NoError(t, err)
	assert.Equal(t, "Bare Edition", book.Title)
	assert.Empty(t, book.Authors)
	assert.Empty(t, book.CoverPath)
	assert.Zero(t, book.PageCount)
	assert.Empty(t, imgs.fetched, "no cover ID means no download attempt")
}

// Re-scanning is DB-first: the cached row is returned as-is (no OpenLibrary
// refresh) and is never duplicated.
func TestRescanReturnsCachedRow(t *testing.T) {
	meta := &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}
	imgs := &fakeImages{}
	svc, gdb := newTestService(t, meta, imgs)

	first, err := svc.Scan(context.Background(), testISBN)
	require.NoError(t, err)
	require.NotEmpty(t, first.CoverPath)

	// Upstream metadata changes, but a DB-first re-scan ignores it.
	meta.books[testISBN].Title = "Networking Basics, 2nd Ed."
	second, err := svc.Scan(context.Background(), testISBN)
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "must not duplicate")
	assert.Equal(t, "Networking Basics", second.Title, "cached row returned unchanged")
	assert.Equal(t, first.CoverPath, second.CoverPath)

	var n int64
	require.NoError(t, gdb.Model(&models.Book{}).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

func seedBook(t *testing.T, gdb *gorm.DB) *models.Book {
	t.Helper()
	book := &models.Book{ISBN13: testISBN, Title: "Networking Basics"}
	require.NoError(t, gdb.Create(book).Error)
	return book
}

func TestTrackHappyAndInvalidStatus(t *testing.T) {
	svc, gdb := newTestService(t, &fakeMetadata{}, &fakeImages{})
	book := seedBook(t, gdb)

	item, err := svc.Track(context.Background(), 1, book.ID, StatusReading)
	require.NoError(t, err)
	assert.Equal(t, TypeBook, item.Type)
	assert.Equal(t, testISBN, item.ExternalID)
	assert.Equal(t, uint(1), item.UserID)

	for _, status := range []string{"WATCHING", "reading", "DONE", ""} {
		_, err := svc.Track(context.Background(), 1, book.ID, status)
		assert.ErrorIs(t, err, ErrInvalidStatus, "status %q", status)
	}
}

func TestTrackUnknownBook(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{}, &fakeImages{})
	_, err := svc.Track(context.Background(), 1, 999, StatusReading)
	assert.ErrorIs(t, err, ErrBookNotFound)
}

// E16: duplicate track returns ErrAlreadyTracked with the existing item.
func TestTrackDuplicate(t *testing.T) {
	svc, gdb := newTestService(t, &fakeMetadata{}, &fakeImages{})
	book := seedBook(t, gdb)

	first, err := svc.Track(context.Background(), 1, book.ID, StatusReading)
	require.NoError(t, err)

	dup, err := svc.Track(context.Background(), 1, book.ID, StatusPlanTo)
	assert.ErrorIs(t, err, ErrAlreadyTracked)
	require.NotNil(t, dup)
	assert.Equal(t, first.ID, dup.ID)
	assert.Equal(t, StatusReading, dup.Status, "existing item is returned unchanged")

	// A different user may track the same book.
	_, err = svc.Track(context.Background(), 2, book.ID, StatusPlanTo)
	assert.NoError(t, err)
}

func seedItem(t *testing.T, gdb *gorm.DB, userID uint, typ, externalID, status string) *models.TrackingItem {
	t.Helper()
	item := &models.TrackingItem{UserID: userID, Type: typ, ExternalID: externalID, Title: "t-" + externalID, Status: status}
	require.NoError(t, gdb.Create(item).Error)
	return item
}

func TestListItemsFilters(t *testing.T) {
	svc, gdb := newTestService(t, &fakeMetadata{}, &fakeImages{})
	seedItem(t, gdb, 1, TypeBook, "isbn-1", StatusReading)
	seedItem(t, gdb, 1, TypeTV, "100", StatusWatching)
	seedItem(t, gdb, 1, TypeTV, "200", StatusCompleted)
	seedItem(t, gdb, 2, TypeBook, "isbn-2", StatusReading) // other user

	ctx := context.Background()
	all, err := svc.ListItems(ctx, 1, "", "")
	require.NoError(t, err)
	assert.Len(t, all, 3, "only the requesting user's items")

	tv, err := svc.ListItems(ctx, 1, TypeTV, "")
	require.NoError(t, err)
	assert.Len(t, tv, 2)

	watching, err := svc.ListItems(ctx, 1, TypeTV, StatusWatching)
	require.NoError(t, err)
	require.Len(t, watching, 1)
	assert.Equal(t, "100", watching[0].ExternalID)

	_, err = svc.ListItems(ctx, 1, "PODCAST", "")
	assert.ErrorIs(t, err, ErrInvalidFilter)
	_, err = svc.ListItems(ctx, 1, "", "BINGING")
	assert.ErrorIs(t, err, ErrInvalidFilter)
}

func TestUpdateItem(t *testing.T) {
	svc, gdb := newTestService(t, &fakeMetadata{}, &fakeImages{})
	bookItem := seedItem(t, gdb, 1, TypeBook, "isbn-1", StatusReading)
	tvItem := seedItem(t, gdb, 1, TypeTV, "100", StatusWatching)
	ctx := context.Background()

	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	// Book: status + page progress.
	updated, err := svc.UpdateItem(ctx, 1, bookItem.ID, strPtr(StatusCompleted), intPtr(320), nil)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, updated.Status)
	assert.Equal(t, 320, updated.Progress)

	// TV: valid status change.
	updated, err = svc.UpdateItem(ctx, 1, tvItem.ID, strPtr(StatusPlanTo), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, StatusPlanTo, updated.Status)

	// Rating: a 1–5 self-rating on either media type.
	updated, err = svc.UpdateItem(ctx, 1, tvItem.ID, nil, nil, intPtr(4))
	require.NoError(t, err)
	assert.Equal(t, 4, updated.Rating)
	_, err = svc.UpdateItem(ctx, 1, bookItem.ID, nil, nil, intPtr(6))
	assert.ErrorIs(t, err, ErrInvalidRating, "rating is capped at 5")

	// Invalid status per type.
	_, err = svc.UpdateItem(ctx, 1, tvItem.ID, strPtr(StatusReading), nil, nil)
	assert.ErrorIs(t, err, ErrInvalidStatus, "READING is not a TV status")
	_, err = svc.UpdateItem(ctx, 1, bookItem.ID, strPtr(StatusWatching), nil, nil)
	assert.ErrorIs(t, err, ErrInvalidStatus, "WATCHING is not a book status")

	// Progress rules.
	_, err = svc.UpdateItem(ctx, 1, tvItem.ID, nil, intPtr(5), nil)
	assert.ErrorIs(t, err, ErrInvalidProgress, "TV progress is derived, not stored")
	_, err = svc.UpdateItem(ctx, 1, bookItem.ID, nil, intPtr(-1), nil)
	assert.ErrorIs(t, err, ErrInvalidProgress)

	// Ownership: multi-select over the fixed set (music), via SetOwnership.
	musicItem := seedItem(t, gdb, 1, TypeMusic, "mb:abc", StatusListening)
	formats, err := svc.SetOwnership(ctx, 1, musicItem.ID, []string{"CD", "Vinyl"})
	require.NoError(t, err)
	assert.Equal(t, []string{"Vinyl", "CD"}, formats, "returned in allowed order")
	_, err = svc.SetOwnership(ctx, 1, musicItem.ID, []string{"Cassette"})
	assert.Error(t, err, "Cassette is not a music format")
	_, err = svc.SetOwnership(ctx, 1, bookItem.ID, []string{"CD"})
	assert.Error(t, err, "books have no ownership vocabulary")

	// Empty patch.
	_, err = svc.UpdateItem(ctx, 1, bookItem.ID, nil, nil, nil)
	assert.ErrorIs(t, err, ErrEmptyUpdate)

	// Cross-user: user 2 must see user 1's item as missing (no leak).
	_, err = svc.UpdateItem(ctx, 2, bookItem.ID, strPtr(StatusPlanTo), nil, nil)
	assert.ErrorIs(t, err, ErrItemNotFound)
}

func TestDeleteItemCrossUser(t *testing.T) {
	svc, gdb := newTestService(t, &fakeMetadata{}, &fakeImages{})
	item := seedItem(t, gdb, 1, TypeBook, "isbn-1", StatusReading)

	err := svc.DeleteItem(context.Background(), 2, item.ID)
	assert.ErrorIs(t, err, ErrItemNotFound)

	require.NoError(t, svc.DeleteItem(context.Background(), 1, item.ID))
	assert.ErrorIs(t, gdb.First(&models.TrackingItem{}, item.ID).Error, gorm.ErrRecordNotFound)
}

// DELETE of a TV item removes only that user's watches; the shared Show and
// Episode rows and other users' watches stay.
func TestDeleteTVItemRemovesWatchesKeepsEpisodes(t *testing.T) {
	svc, gdb := newTestService(t, &fakeMetadata{}, &fakeImages{})

	show := &models.Show{TMDBID: 1399, Title: "Game of Thrones"}
	require.NoError(t, gdb.Create(show).Error)
	ep1 := &models.Episode{ShowID: show.ID, Season: 1, Number: 1, Title: "Winter Is Coming"}
	ep2 := &models.Episode{ShowID: show.ID, Season: 1, Number: 2, Title: "The Kingsroad"}
	require.NoError(t, gdb.Create(ep1).Error)
	require.NoError(t, gdb.Create(ep2).Error)

	item := seedItem(t, gdb, 1, TypeTV, "1399", StatusWatching)
	seedItem(t, gdb, 2, TypeTV, "1399", StatusWatching)
	now := time.Now()
	require.NoError(t, gdb.Create(&models.EpisodeWatch{UserID: 1, EpisodeID: ep1.ID, WatchedAt: now}).Error)
	require.NoError(t, gdb.Create(&models.EpisodeWatch{UserID: 1, EpisodeID: ep2.ID, WatchedAt: now}).Error)
	require.NoError(t, gdb.Create(&models.EpisodeWatch{UserID: 2, EpisodeID: ep1.ID, WatchedAt: now}).Error)

	require.NoError(t, svc.DeleteItem(context.Background(), 1, item.ID))

	var myWatches, otherWatches, episodes, shows int64
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).Where("user_id = ?", 1).Count(&myWatches).Error)
	require.NoError(t, gdb.Model(&models.EpisodeWatch{}).Where("user_id = ?", 2).Count(&otherWatches).Error)
	require.NoError(t, gdb.Model(&models.Episode{}).Count(&episodes).Error)
	require.NoError(t, gdb.Model(&models.Show{}).Count(&shows).Error)

	assert.EqualValues(t, 0, myWatches, "deleting user's watches must be gone")
	assert.EqualValues(t, 1, otherWatches, "other users' watches must stay")
	assert.EqualValues(t, 2, episodes, "shared episode metadata must stay")
	assert.EqualValues(t, 1, shows, "shared show metadata must stay")

	assert.ErrorIs(t, gdb.First(&models.TrackingItem{}, item.ID).Error, gorm.ErrRecordNotFound)
}

// TestScanIsDBFirst proves a cached ISBN is served from the DB without any
// OpenLibrary call: the second scan succeeds even though metadata now errors.
func TestScanIsDBFirst(t *testing.T) {
	meta := &fakeMetadata{books: map[string]*openlibrary.Book{testISBN: fullMeta()}}
	svc, _ := newTestService(t, meta, &fakeImages{})

	first, err := svc.Scan(context.Background(), testISBN)
	require.NoError(t, err)

	// Break the metadata client; a cached ISBN must still resolve.
	meta.err = errors.New("openlibrary must not be called for a cached ISBN")
	second, err := svc.Scan(context.Background(), testISBN)
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, "Networking Basics", second.Title)
}

// SearchTitle delegates to OpenLibrary and returns its works.
func TestSearchTitleDelegates(t *testing.T) {
	meta := &fakeMetadata{titles: []openlibrary.TitleResult{
		{WorkKey: "/works/OL1W", Title: "Networking Basics", Authors: []string{"Jane Doe"}, FirstYear: 2004, EditionCount: 3},
	}}
	svc, _ := newTestService(t, meta, &fakeImages{})

	res, err := svc.SearchTitle(context.Background(), "networking")
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, "/works/OL1W", res[0].WorkKey)
}

// A blank title never reaches OpenLibrary.
func TestSearchTitleEmptyQuery(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{}, &fakeImages{})

	_, err := svc.SearchTitle(context.Background(), "   ")
	require.ErrorIs(t, err, ErrEmptyQuery)
}

// ListEditions returns the ISBN-bearing editions for a work's picker.
func TestListEditions(t *testing.T) {
	meta := &fakeMetadata{editions: map[string][]openlibrary.Edition{
		"/works/OL1W": {{ISBN13: testISBN, Title: "Networking Basics", PublishDate: "2004"}},
	}}
	svc, _ := newTestService(t, meta, &fakeImages{})

	res, err := svc.ListEditions(context.Background(), "/works/OL1W")
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, testISBN, res[0].ISBN13)
}

// A blank work key is rejected before any upstream call.
func TestListEditionsEmptyQuery(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{}, &fakeImages{})

	_, err := svc.ListEditions(context.Background(), "")
	require.ErrorIs(t, err, ErrEmptyQuery)
}

// Discover surfaces more works by the tracked book's author, caches their
// covers, dedupes the already-tracked title, and tags each with its source.
func TestDiscoverByAuthorDedupesTracked(t *testing.T) {
	meta := &fakeMetadata{
		books: map[string]*openlibrary.Book{testISBN: fullMeta()},
		byAuthor: map[string][]openlibrary.TitleResult{
			"Jane Doe": {
				{WorkKey: "/works/OL1W", Title: "Networking Basics", Authors: []string{"Jane Doe"}}, // already tracked (same title)
				{WorkKey: "/works/OL2W", Title: "Advanced Networking", Authors: []string{"Jane Doe"}, FirstYear: 2010, CoverID: 999},
			},
		},
	}
	imgs := &fakeImages{}
	svc, _ := newTestService(t, meta, imgs)
	ctx := context.Background()

	book, err := svc.Scan(ctx, testISBN)
	require.NoError(t, err)
	_, err = svc.Track(ctx, 1, book.ID, StatusReading)
	require.NoError(t, err)

	items, err := svc.Discover(ctx, 1)
	require.NoError(t, err)
	require.Len(t, items, 1, "the already-tracked title is filtered out")
	assert.Equal(t, "/works/OL2W", items[0].WorkKey)
	assert.Equal(t, "Advanced Networking", items[0].Title)
	assert.Equal(t, "books by Jane Doe", items[0].SuggestedBy)
	// Cover is cached through internal/images, keyed by cover id (never hotlinked).
	assert.Equal(t, "book/olcover-999.jpg", items[0].CoverPath)
	assert.Equal(t, []string{"olcover-999"}, imgs.fetched)
}

// A rejected work is not surfaced again by Discover.
func TestDiscoverExcludesRejectedBook(t *testing.T) {
	meta := &fakeMetadata{
		books: map[string]*openlibrary.Book{testISBN: fullMeta()},
		byAuthor: map[string][]openlibrary.TitleResult{
			"Jane Doe": {{WorkKey: "/works/OL2W", Title: "Advanced Networking", Authors: []string{"Jane Doe"}}},
		},
	}
	svc, _ := newTestService(t, meta, &fakeImages{})
	ctx := context.Background()

	book, err := svc.Scan(ctx, testISBN)
	require.NoError(t, err)
	_, err = svc.Track(ctx, 1, book.ID, StatusReading)
	require.NoError(t, err)

	require.NoError(t, svc.RejectRec(ctx, 1, "/works/OL2W"))
	items, err := svc.Discover(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, items)
}

// With no tracked books there is nothing to seed from.
func TestDiscoverNoSourcesBook(t *testing.T) {
	svc, _ := newTestService(t, &fakeMetadata{}, &fakeImages{})
	items, err := svc.Discover(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, items)
}
