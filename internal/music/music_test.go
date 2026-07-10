package music

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/discogs"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/musicbrainz"
)

const (
	testBarcode = "602547790392"
	testMBID    = "0a2d5b1c-1111-2222-3333-444455556666"
)

// fakeDiscogs is a canned DiscogsClient.
type fakeDiscogs struct {
	releases map[string]*discogs.Release
	err      error
	calls    int
}

func (f *fakeDiscogs) LookupByBarcode(_ context.Context, barcode string) (*discogs.Release, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	r, ok := f.releases[barcode]
	if !ok {
		return nil, &discogs.NotFoundError{Barcode: barcode}
	}
	return r, nil
}

// fakeMB is a canned MusicBrainzClient.
type fakeMB struct {
	search []musicbrainz.ReleaseGroup
	groups map[string]*musicbrainz.ReleaseGroup
	err    error
}

func (f *fakeMB) Search(_ context.Context, _ string, _ int) ([]musicbrainz.ReleaseGroup, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.search, nil
}

func (f *fakeMB) GetReleaseGroup(_ context.Context, mbid string) (*musicbrainz.ReleaseGroup, error) {
	if f.err != nil {
		return nil, f.err
	}
	g, ok := f.groups[mbid]
	if !ok {
		return nil, musicbrainz.ErrNotFound
	}
	return g, nil
}

func (f *fakeMB) CoverURL(mbid string, _ int) string {
	if mbid == "" {
		return ""
	}
	return "http://img.test/" + mbid + ".jpg"
}

// fakeImages is a canned ImageStore.
type fakeImages struct {
	err     error
	fetched []string
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

func fullRelease() *discogs.Release {
	return &discogs.Release{
		DiscogsID: 8888,
		Artist:    "Adele",
		Title:     "25",
		Year:      2015,
		CoverURL:  "http://img.test/25.jpg",
		Barcode:   testBarcode,
	}
}

func newService(t *testing.T, dc *fakeDiscogs, mb *fakeMB, imgs *fakeImages) (*Service, *gorm.DB) {
	t.Helper()
	gdb := testDB(t)
	var is ImageStore
	if imgs != nil {
		is = imgs
	}
	return NewService(gdb, dc, mb, is), gdb
}

func TestScanHappyPath(t *testing.T) {
	dc := &fakeDiscogs{releases: map[string]*discogs.Release{testBarcode: fullRelease()}}
	imgs := &fakeImages{}
	svc, gdb := newService(t, dc, &fakeMB{}, imgs)

	album, err := svc.Scan(context.Background(), "6-025477-90392-0") // separators normalize
	require.NoError(t, err)
	assert.Equal(t, "discogs:8888", album.ExternalID)
	assert.Equal(t, "Adele", album.Artist)
	assert.Equal(t, "25", album.Title)
	assert.Equal(t, 2015, album.Year)
	assert.Equal(t, testBarcode, album.Barcode)
	assert.Equal(t, "music/discogs_8888.jpg", album.CoverPath, "cover key is filesystem-safe")
	assert.Equal(t, []string{"discogs_8888"}, imgs.fetched)

	var count int64
	require.NoError(t, gdb.Model(&models.Album{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// A second scan of a cached barcode must not hit Discogs again (DB-first).
func TestScanDBFirst(t *testing.T) {
	dc := &fakeDiscogs{releases: map[string]*discogs.Release{testBarcode: fullRelease()}}
	svc, _ := newService(t, dc, &fakeMB{}, &fakeImages{})

	_, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)
	_, err = svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)
	assert.Equal(t, 1, dc.calls, "second scan should be served from cache")
}

func TestScanNotFound(t *testing.T) {
	dc := &fakeDiscogs{releases: map[string]*discogs.Release{}}
	svc, _ := newService(t, dc, &fakeMB{}, &fakeImages{})

	_, err := svc.Scan(context.Background(), testBarcode)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestScanInvalidBarcode(t *testing.T) {
	svc, _ := newService(t, &fakeDiscogs{}, &fakeMB{}, &fakeImages{})

	_, err := svc.Scan(context.Background(), "123") // too short
	require.ErrorIs(t, err, ErrInvalidBarcode)
}

func TestScanUnconfigured(t *testing.T) {
	dc := &fakeDiscogs{err: discogs.ErrUnconfigured}
	svc, _ := newService(t, dc, &fakeMB{}, &fakeImages{})

	_, err := svc.Scan(context.Background(), testBarcode)
	require.ErrorIs(t, err, ErrUnconfigured)
}

func TestScanUpstream(t *testing.T) {
	dc := &fakeDiscogs{err: errors.New("boom")}
	svc, _ := newService(t, dc, &fakeMB{}, &fakeImages{})

	_, err := svc.Scan(context.Background(), testBarcode)
	require.ErrorIs(t, err, ErrUpstream)
}

func TestTrackAndDuplicate(t *testing.T) {
	dc := &fakeDiscogs{releases: map[string]*discogs.Release{testBarcode: fullRelease()}}
	svc, _ := newService(t, dc, &fakeMB{}, &fakeImages{})

	album, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)

	item, err := svc.Track(context.Background(), 1, album.ID, "")
	require.NoError(t, err)
	assert.Equal(t, TypeMusic, item.Type)
	assert.Equal(t, "discogs:8888", item.ExternalID)
	assert.Equal(t, StatusListening, item.Status, "empty status defaults to LISTENING")

	existing, err := svc.Track(context.Background(), 1, album.ID, StatusListening)
	require.ErrorIs(t, err, ErrAlreadyTracked)
	require.NotNil(t, existing)
}

func TestTrackInvalidStatus(t *testing.T) {
	dc := &fakeDiscogs{releases: map[string]*discogs.Release{testBarcode: fullRelease()}}
	svc, _ := newService(t, dc, &fakeMB{}, &fakeImages{})
	album, err := svc.Scan(context.Background(), testBarcode)
	require.NoError(t, err)

	_, err = svc.Track(context.Background(), 1, album.ID, "READING")
	require.ErrorIs(t, err, ErrInvalidStatus)
}

func TestTrackAlbumNotFound(t *testing.T) {
	svc, _ := newService(t, &fakeDiscogs{}, &fakeMB{}, &fakeImages{})
	_, err := svc.Track(context.Background(), 1, 999, StatusListening)
	require.ErrorIs(t, err, ErrAlbumNotFound)
}

func TestSearch(t *testing.T) {
	mb := &fakeMB{search: []musicbrainz.ReleaseGroup{
		{MBID: testMBID, Artist: "The Beatles", Title: "Abbey Road", Year: 1969},
	}}
	svc, _ := newService(t, &fakeDiscogs{}, mb, &fakeImages{})

	res, err := svc.Search(context.Background(), "abbey road")
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, testMBID, res[0].MBID)
	assert.Equal(t, "The Beatles", res[0].Artist)

	_, err = svc.Search(context.Background(), "   ")
	require.ErrorIs(t, err, ErrInvalidQuery)
}

func TestAddByMusicBrainz(t *testing.T) {
	mb := &fakeMB{groups: map[string]*musicbrainz.ReleaseGroup{
		testMBID: {MBID: testMBID, Artist: "The Beatles", Title: "Abbey Road", Year: 1969},
	}}
	imgs := &fakeImages{}
	svc, gdb := newService(t, &fakeDiscogs{}, mb, imgs)

	res, err := svc.AddByMusicBrainz(context.Background(), 1, testMBID, "")
	require.NoError(t, err)
	assert.Equal(t, "mb:"+testMBID, res.Album.ExternalID)
	assert.Equal(t, "Abbey Road", res.Album.Title)
	assert.Equal(t, "music/mb_"+testMBID+".jpg", res.Album.CoverPath)
	assert.Equal(t, StatusListening, res.Item.Status)

	var albums int64
	require.NoError(t, gdb.Model(&models.Album{}).Count(&albums).Error)
	assert.Equal(t, int64(1), albums)

	// Adding again is a duplicate track (and reuses the cached album, no
	// second MusicBrainz round-trip needed).
	_, err = svc.AddByMusicBrainz(context.Background(), 1, testMBID, StatusListening)
	require.ErrorIs(t, err, ErrAlreadyTracked)
}

func TestAddByMusicBrainzNotFound(t *testing.T) {
	svc, _ := newService(t, &fakeDiscogs{}, &fakeMB{groups: map[string]*musicbrainz.ReleaseGroup{}}, &fakeImages{})

	_, err := svc.AddByMusicBrainz(context.Background(), 1, testMBID, "")
	require.ErrorIs(t, err, ErrNotFound)
}
