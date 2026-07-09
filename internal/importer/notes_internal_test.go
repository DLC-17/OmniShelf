package importer

import (
	"testing"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// notesFile is a parsedFile shaped like the Goodreads notes export the match
// logic reads: columns title, author, isbn, isbn13 at indices 0..3.
func notesFile() *parsedFile {
	return &parsedFile{titleIdx: 0, authorIdx: 1, isbnIdx: 2, isbn13Idx: 3}
}

// testBookIndex mirrors what loadBookIndex builds for a user tracking Dune and
// The Hobbit, so match's precedence (ISBN → title+author → title) can be tested
// without a database.
func testBookIndex() *bookIndex {
	dune := models.TrackingItem{ID: 1, Title: "Dune", ExternalID: "9780441013593"}
	hobbit := models.TrackingItem{ID: 2, Title: "The Hobbit", ExternalID: "9780547928227"}
	return &bookIndex{
		byISBN: map[string]models.TrackingItem{"9780441013593": dune},
		byTitleAuthor: map[string]models.TrackingItem{
			"dune|frank herbert":       dune,
			"the hobbit|j r r tolkien": hobbit,
		},
		byTitle: map[string]models.TrackingItem{
			"dune":       dune,
			"the hobbit": hobbit,
		},
	}
}

func TestBookIndexMatch(t *testing.T) {
	idx := testBookIndex()
	tests := []struct {
		name    string
		rec     []string // title, author, isbn, isbn13
		wantID  uint
		wantHit bool
	}{
		{"isbn13 match", []string{"Dune", "Frank Herbert", `="0441013597"`, `="9780441013593"`}, 1, true},
		{"isbn13 from isbn column fallback", []string{"D", "H", `="9780441013593"`, `=""`}, 1, true},
		{"title+author fallback when isbn absent", []string{"The Hobbit", "J.R.R. Tolkien", `=""`, `=""`}, 2, true},
		{"title-only fallback when author differs", []string{"Dune", "Someone Else", `=""`, `=""`}, 1, true},
		{"no match", []string{"Ulysses", "James Joyce", `=""`, `=""`}, 0, false},
		{"blank title is no match", []string{"", "", `=""`, `=""`}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := idx.match(notesFile(), tt.rec)
			if ok != tt.wantHit {
				t.Fatalf("match hit = %v, want %v", ok, tt.wantHit)
			}
			if ok && got.ID != tt.wantID {
				t.Fatalf("matched item %d, want %d", got.ID, tt.wantID)
			}
		})
	}
}

func TestParseGoodreadsDate(t *testing.T) {
	tests := []struct {
		in       string
		wantZero bool
		wantYear int
	}{
		{"2019/05/01", false, 2019},
		{"2020-03-03", false, 2020},
		{"", true, 0},
		{"not a date", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := parseGoodreadsDate(tt.in)
			if got.IsZero() != tt.wantZero {
				t.Fatalf("IsZero = %v, want %v", got.IsZero(), tt.wantZero)
			}
			if !tt.wantZero && got.Year() != tt.wantYear {
				t.Fatalf("year = %d, want %d", got.Year(), tt.wantYear)
			}
		})
	}
}
