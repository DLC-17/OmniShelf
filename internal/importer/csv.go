package importer

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path"
	"strings"
)

// TV Time exports vary between app versions, so files are recognized by
// their header row, not their filename, and headers are matched
// case-insensitively by name. Accepted column headers:
//
//	show title  : tv_show_name, show_name, series_name, show, name, title
//	season      : episode_season_number, season_number, season, season_no
//	episode     : episode_number, episode, number, episode_no
//	watched at  : watched_at, created_at, updated_at, watch_date, date, time
//	              (optional; rows without a parseable timestamp use "now")
//
// A file whose header has a title column plus season AND episode columns is
// treated as seen_episodes.csv; a file with a title column but no
// season+episode pair is treated as followed_shows.csv. Anything else is
// rejected up front with a 400 before any job is created.
var (
	titleHeaders   = []string{"tv_show_name", "show_name", "series_name", "show", "name", "title"}
	seasonHeaders  = []string{"episode_season_number", "season_number", "season", "season_no", "s_no"}
	episodeHeaders = []string{"episode_number", "episode", "number", "episode_no", "ep_no"}
	watchedHeaders = []string{"watched_at", "created_at", "updated_at", "watch_date", "date", "time"}

	// Goodreads library export columns (books). An ISBN column alongside a
	// title is the signal that distinguishes a book export from a TV export.
	authorHeaders = []string{"author", "authors"}
	isbn13Headers = []string{"isbn13"}
	isbnHeaders   = []string{"isbn"}
	shelfHeaders  = []string{"exclusive_shelf", "bookshelves", "shelf"}
	pagesHeaders  = []string{"number_of_pages", "pages", "page_count", "num_pages"}
)

// fileKind classifies a recognized CSV.
type fileKind int

const (
	kindFollowed  fileKind = iota // followed_shows.csv shape (TV)
	kindSeen                      // seen_episodes.csv shape (TV)
	kindGoodreads                 // Goodreads library export (books)
)

// UploadFile is one file received in the multipart upload (or extracted from
// an uploaded zip).
type UploadFile struct {
	Name string
	Data []byte
}

// parsedFile is a classified CSV with its raw data records. Records are kept
// raw here; per-row validation happens in the background job so malformed
// data rows can be skipped and counted there.
type parsedFile struct {
	name string
	kind fileKind
	// Season/episode number columns as candidate lists (in header order), so
	// exports that use either name — season_number/s_no, episode_number/ep_no —
	// resolve per row to the first populated one.
	titleIdx    int
	seasonIdxs  []int
	episodeIdxs []int
	watchedIdx  int // -1 when absent
	// Goodreads (book) column indices; -1 when absent.
	authorIdx int
	isbn13Idx int
	isbnIdx   int
	shelfIdx  int
	pagesIdx  int
	records   [][]string
}

// Payload is a fully classified upload ready for background processing.
type Payload struct {
	files     []parsedFile
	totalRows int
}

// TotalRows reports the number of data rows (valid or not) across all files.
func (p *Payload) TotalRows() int { return p.totalRows }

// ValidationError marks an upload rejected during up-front header
// validation; the API layer maps it to HTTP 400.
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

func validationErrorf(format string, args ...any) *ValidationError {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}

// ParseUpload validates and classifies the uploaded files. Zip archives are
// expanded and their .csv entries parsed. It returns a *ValidationError when
// no file has a recognizable TV Time header row.
func ParseUpload(files []UploadFile) (*Payload, error) {
	var csvs []UploadFile
	for _, f := range files {
		if isZip(f) {
			extracted, err := extractZip(f)
			if err != nil {
				return nil, err
			}
			csvs = append(csvs, extracted...)
		} else {
			csvs = append(csvs, f)
		}
	}
	if len(csvs) == 0 {
		return nil, validationErrorf("no files uploaded")
	}

	p := &Payload{}
	for _, f := range csvs {
		pf, err := parseCSV(f)
		if err != nil {
			return nil, err
		}
		p.files = append(p.files, *pf)
		p.totalRows += len(pf.records)
	}
	return p, nil
}

// isZip detects a zip upload by extension or magic bytes.
func isZip(f UploadFile) bool {
	if strings.EqualFold(path.Ext(f.Name), ".zip") {
		return true
	}
	return bytes.HasPrefix(f.Data, []byte("PK\x03\x04"))
}

// extractZip returns the .csv entries of a zip archive, skipping
// directories and macOS metadata entries.
func extractZip(f UploadFile) ([]UploadFile, error) {
	zr, err := zip.NewReader(bytes.NewReader(f.Data), int64(len(f.Data)))
	if err != nil {
		return nil, validationErrorf("%s: not a valid zip archive", f.Name)
	}
	var out []UploadFile
	for _, entry := range zr.File {
		name := entry.Name
		if entry.FileInfo().IsDir() ||
			strings.HasPrefix(name, "__MACOSX/") ||
			strings.HasPrefix(path.Base(name), ".") ||
			!strings.EqualFold(path.Ext(name), ".csv") {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return nil, validationErrorf("%s: cannot read zip entry %s", f.Name, name)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, validationErrorf("%s: cannot read zip entry %s", f.Name, name)
		}
		out = append(out, UploadFile{Name: path.Base(name), Data: data})
	}
	if len(out) == 0 {
		return nil, validationErrorf("%s: zip archive contains no .csv files", f.Name)
	}
	return out, nil
}

// parseCSV validates the header row and reads all data records. Records that
// fail CSV parsing are kept as empty records so the background job counts
// them as skipped malformed rows while Total stays accurate.
func parseCSV(f UploadFile) (*parsedFile, error) {
	r := csv.NewReader(bytes.NewReader(f.Data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, validationErrorf("%s: not a readable CSV file", f.Name)
	}

	pf := &parsedFile{
		name: f.Name, titleIdx: -1, watchedIdx: -1,
		authorIdx: -1, isbn13Idx: -1, isbnIdx: -1, shelfIdx: -1, pagesIdx: -1,
	}
	for i, h := range header {
		switch n := normalizeHeader(h); {
		case pf.titleIdx < 0 && matchesHeader(n, titleHeaders):
			pf.titleIdx = i
		case matchesHeader(n, seasonHeaders):
			pf.seasonIdxs = append(pf.seasonIdxs, i)
		case matchesHeader(n, episodeHeaders):
			pf.episodeIdxs = append(pf.episodeIdxs, i)
		case pf.watchedIdx < 0 && matchesHeader(n, watchedHeaders):
			pf.watchedIdx = i
		case pf.authorIdx < 0 && matchesHeader(n, authorHeaders):
			pf.authorIdx = i
		case pf.isbn13Idx < 0 && matchesHeader(n, isbn13Headers):
			pf.isbn13Idx = i
		case pf.isbnIdx < 0 && matchesHeader(n, isbnHeaders):
			pf.isbnIdx = i
		case pf.shelfIdx < 0 && matchesHeader(n, shelfHeaders):
			pf.shelfIdx = i
		case pf.pagesIdx < 0 && matchesHeader(n, pagesHeaders):
			pf.pagesIdx = i
		}
	}

	switch {
	case pf.titleIdx >= 0 && (pf.isbn13Idx >= 0 || pf.isbnIdx >= 0):
		// A title alongside an ISBN column is a Goodreads (book) export.
		pf.kind = kindGoodreads
	case pf.titleIdx >= 0 && len(pf.seasonIdxs) > 0 && len(pf.episodeIdxs) > 0:
		pf.kind = kindSeen
	case pf.titleIdx >= 0:
		pf.kind = kindFollowed
	default:
		return nil, validationErrorf(
			"%s: header row not recognized as a TV Time or Goodreads export (expected a title column, plus an ISBN column for books)", f.Name)
	}

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Malformed CSV line: keep a placeholder so it is skipped and
			// counted during processing rather than silently dropped.
			pf.records = append(pf.records, nil)
			continue
		}
		if isBlankRecord(rec) {
			continue
		}
		pf.records = append(pf.records, rec)
	}
	return pf, nil
}

// normalizeHeader lowercases and trims a header cell, stripping a UTF-8 BOM
// and surrounding quotes/whitespace.
func normalizeHeader(h string) string {
	h = strings.TrimPrefix(h, "\uFEFF")
	h = strings.Trim(h, ` "'`)
	h = strings.ToLower(strings.TrimSpace(h))
	// "Show Name" and "show_name" are the same header.
	return strings.ReplaceAll(h, " ", "_")
}

func matchesHeader(h string, accepted []string) bool {
	for _, a := range accepted {
		if h == a {
			return true
		}
	}
	return false
}

func isBlankRecord(rec []string) bool {
	for _, f := range rec {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}
