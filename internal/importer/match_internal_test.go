package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

func TestChooseMatch(t *testing.T) {
	results := []tmdb.SearchResult{
		{ID: 1396, Name: "Breaking Bad"},
		{ID: 2, Name: "Metastasis"},
	}
	tests := []struct {
		name  string
		title string
		want  int
	}{
		{"exact", "Breaking Bad", 1396},
		{"exact case/punct insensitive", "  breaking BAD!  ", 1396},
		{"fuzzy above threshold", "Braking Bad", 1396},
		{"below threshold", "Completely Different Show", 0},
		{"empty title", "   ", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, chooseMatch(tt.title, results))
		})
	}
	assert.Equal(t, 0, chooseMatch("Breaking Bad", nil), "no results → unresolved")
}

func TestNormalizeTitle(t *testing.T) {
	assert.Equal(t, "the office us", normalizeTitle("The Office (US)"))
	assert.Equal(t, "breaking bad", normalizeTitle("  Breaking   Bad! "))
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

func TestParseUploadClassification(t *testing.T) {
	p, err := ParseUpload([]UploadFile{
		{Name: "followed_shows.csv", Data: readFixture(t, "followed_shows.csv")},
		{Name: "seen_episodes.csv", Data: readFixture(t, "seen_episodes.csv")},
	})
	require.NoError(t, err)
	require.Len(t, p.files, 2)
	assert.Equal(t, kindFollowed, p.files[0].kind)
	assert.Equal(t, kindSeen, p.files[1].kind)
	// 3 followed data rows + 5 seen data rows (malformed rows still count).
	assert.Equal(t, 8, p.TotalRows())
	assert.Equal(t, 3, p.files[1].watchedIdx, "created_at accepted as watched-at column")
}

func TestParseUploadHeaderAliases(t *testing.T) {
	csv := "Show Name,Season,Episode,Watched_At\nBreaking Bad,1,1,2020-01-01\n"
	p, err := ParseUpload([]UploadFile{{Name: "x.csv", Data: []byte(csv)}})
	require.NoError(t, err)
	require.Len(t, p.files, 1)
	assert.Equal(t, kindSeen, p.files[0].kind)
}

func TestParseUploadRejectsUnknownHeader(t *testing.T) {
	_, err := ParseUpload([]UploadFile{{Name: "wrong_header.csv", Data: readFixture(t, "wrong_header.csv")}})
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
}
