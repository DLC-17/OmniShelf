// Package books implements the book-tracking and library service layer:
// ISBN scan → OpenLibrary lookup → shared Book cache row + cached cover,
// per-user TrackingItems, and the unified library operations (list, update
// status/progress, untrack). Handlers in internal/api stay thin and translate
// this package's sentinel errors into the standard envelope.
//
// Every mutating operation is scoped by the caller-supplied userID, which
// handlers take from the JWT — never from client input. Items
// belonging to another user surface as ErrItemNotFound so their existence is
// not leaked.
package books

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/openlibrary"
	"github.com/davidlc1229/omnishelf/internal/ownership"
	"github.com/davidlc1229/omnishelf/internal/tags"
)

// Media types stored in TrackingItem.Type.
const (
	TypeTV    = "TV"
	TypeBook  = "BOOK"
	TypeGame  = "GAME"
	TypeMovie = "MOVIE"
	TypeMusic = "MUSIC"
	TypeCard  = "CARD"
)

// Tracking statuses. Books use READING; TV uses WATCHING; games use PLAYING;
// music uses LISTENING; cards use OWNED (a card on the shelf is a card you
// own). COMPLETED, PLAN_TO ("not started") and STOPPED (dropped) apply to all.
const (
	StatusWatching  = "WATCHING"
	StatusReading   = "READING"
	StatusPlaying   = "PLAYING"
	StatusListening = "LISTENING"
	StatusOwned     = "OWNED"
	StatusCompleted = "COMPLETED"
	StatusPlanTo    = "PLAN_TO"
	StatusStopped   = "STOPPED"
)

// Sentinel errors translated by the API layer into envelope responses.
var (
	// ErrInvalidISBN means the scanned string is not ISBN-13 shaped.
	ErrInvalidISBN = errors.New("books: invalid ISBN-13")
	// ErrNotFound means OpenLibrary has no record for the ISBN (E4).
	ErrNotFound = errors.New("books: no record for ISBN")
	// ErrUpstream means the metadata lookup failed for a non-404 reason.
	ErrUpstream = errors.New("books: metadata service unavailable")
	// ErrBookNotFound means the referenced Book row does not exist.
	ErrBookNotFound = errors.New("books: book not found")
	// ErrAlreadyTracked means the user already tracks this media (E16).
	ErrAlreadyTracked = errors.New("books: item already tracked")
	// ErrInvalidStatus means the status is not valid for the media type.
	ErrInvalidStatus = errors.New("books: invalid status")
	// ErrInvalidProgress means progress is negative or set on a TV item
	// (TV progress is derived from EpisodeWatch counts, never stored).
	ErrInvalidProgress = errors.New("books: invalid progress")
	// ErrInvalidRating means a rating outside the 0–5 range.
	ErrInvalidRating = errors.New("books: invalid rating")
	// ErrInvalidOwnership means an ownership format not valid for the item's type
	// (or set on a type that has no ownership vocabulary).
	ErrInvalidOwnership = errors.New("books: invalid ownership")
	// ErrEmptyUpdate means a PATCH carried no updatable fields.
	ErrEmptyUpdate = errors.New("books: no fields to update")
	// ErrInvalidFilter means an unknown type/status library filter value.
	ErrInvalidFilter = errors.New("books: invalid filter")
	// ErrItemNotFound means the tracking item does not exist for this user
	// (including items owned by someone else — existence is not leaked).
	ErrItemNotFound = errors.New("books: tracking item not found")
	// ErrEmptyQuery means a title search / editions lookup was called with a
	// blank query.
	ErrEmptyQuery = errors.New("books: empty query")
)

// MetadataClient is the slice of *openlibrary.Client the service needs;
// tests substitute a fake. A missing ISBN must yield an error satisfying
// errors.Is(err, openlibrary.ErrNotFound).
type MetadataClient interface {
	GetByISBN(ctx context.Context, isbn string) (*openlibrary.Book, error)
	SearchByTitle(ctx context.Context, title string) ([]openlibrary.TitleResult, error)
	SearchByAuthor(ctx context.Context, authorName string) ([]openlibrary.TitleResult, error)
	SearchBySubject(ctx context.Context, subject string) ([]openlibrary.TitleResult, error)
	ListEditions(ctx context.Context, workKey string) ([]openlibrary.Edition, error)
	CoverURL(coverID int, size string) string
}

// ImageStore is the slice of *images.Store the service needs; tests
// substitute a fake.
type ImageStore interface {
	Fetch(ctx context.Context, httpClient *http.Client, url, kind, externalID string) (string, error)
}

// Service implements book scanning/tracking and library operations.
type Service struct {
	db         *gorm.DB
	metadata   MetadataClient
	images     ImageStore
	httpClient *http.Client
}

// NewService wires the service. The internal HTTP client is used only for
// cover downloads through the image store.
func NewService(gdb *gorm.DB, metadata MetadataClient, images ImageStore) *Service {
	return &Service{
		db:         gdb,
		metadata:   metadata,
		images:     images,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Scan looks an ISBN up on OpenLibrary and upserts the shared Book cache row.
// Partial metadata is saved as-is; a failed cover download
// is logged and leaves CoverPath empty for the nightly retry. An ISBN
// unknown to OpenLibrary yields ErrNotFound.
func (s *Service) Scan(ctx context.Context, isbn string) (*models.Book, error) {
	norm, err := normalizeISBN13(isbn)
	if err != nil {
		return nil, err
	}

	// DB-first: if this ISBN is already in the shared Book cache, return it
	// without an OpenLibrary round-trip (and without spending its rate limit).
	var cached models.Book
	err = s.db.WithContext(ctx).Where(&models.Book{ISBN13: norm}).First(&cached).Error
	if err == nil {
		return &cached, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("looking up cached book %s: %w", norm, err)
	}

	meta, err := s.metadata.GetByISBN(ctx, norm)
	if err != nil {
		if errors.Is(err, openlibrary.ErrNotFound) {
			return nil, fmt.Errorf("%w %s", ErrNotFound, norm)
		}
		return nil, errors.Join(ErrUpstream, err)
	}

	isbn13 := meta.ISBN13
	if isbn13 == "" {
		isbn13 = norm
	}

	// Cover download is best-effort: failure never blocks tracking (E13).
	coverPath := ""
	if url := s.metadata.CoverURL(meta.CoverID, "L"); url != "" {
		coverPath, err = s.images.Fetch(ctx, s.httpClient, url, "book", isbn13)
		if err != nil {
			log.Printf("books: cover download for %s failed (nightly sync will retry): %v", isbn13, err)
			coverPath = ""
		}
	}

	book := models.Book{
		ISBN13:      isbn13,
		Title:       meta.Title,
		Authors:     strings.Join(meta.Authors, ", "),
		CoverPath:   coverPath,
		PageCount:   meta.PageCount,
		Description: meta.Description,
	}
	if book.CoverPath == "" || book.Description == "" || book.PageCount == 0 {
		s.enrichFromGoogleBooks(ctx, isbn13, &book)
	}
	if err := s.upsertBook(ctx, &book); err != nil {
		return nil, err
	}
	// Persist source-derived tags (OpenLibrary subjects) once the row has an
	// ID. Best-effort: a tag failure must not fail the scan.
	if len(meta.Subjects) > 0 {
		if err := tags.NewStore(s.db).Set(ctx, tags.TypeBook, book.ID, meta.Subjects); err != nil {
			log.Printf("books: persisting tags for %s failed: %v", book.ISBN13, err)
		}
	}
	return &book, nil
}

// upsertBook creates the Book row or refreshes an existing one for the same
// ISBN. An already-cached cover is kept when the new download failed.
func (s *Service) upsertBook(ctx context.Context, book *models.Book) error {
	var existing models.Book
	err := s.db.WithContext(ctx).Where(&models.Book{ISBN13: book.ISBN13}).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := s.db.WithContext(ctx).Create(book).Error; err != nil {
			// Concurrent scan of the same ISBN: the unique index makes one
			// Create lose; fall back to the winner's row.
			if isUniqueViolation(err) {
				return s.upsertBook(ctx, book)
			}
			return fmt.Errorf("creating book %s: %w", book.ISBN13, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("looking up book %s: %w", book.ISBN13, err)
	}

	book.ID = existing.ID
	if book.CoverPath == "" {
		book.CoverPath = existing.CoverPath
	}
	if err := s.db.WithContext(ctx).Save(book).Error; err != nil {
		return fmt.Errorf("updating book %s: %w", book.ISBN13, err)
	}
	return nil
}

// Track creates the user's TrackingItem for a scanned book. A duplicate returns the existing item
// alongside ErrAlreadyTracked so the API can answer 409 with it.
func (s *Service) Track(ctx context.Context, userID, bookID uint, status string) (*models.TrackingItem, error) {
	if !validStatus(TypeBook, status) {
		return nil, fmt.Errorf("%w: %q is not valid for books", ErrInvalidStatus, status)
	}

	var book models.Book
	if err := s.db.WithContext(ctx).First(&book, bookID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBookNotFound
		}
		return nil, fmt.Errorf("looking up book %d: %w", bookID, err)
	}

	item := models.TrackingItem{
		UserID:     userID,
		Type:       TypeBook,
		ExternalID: book.ISBN13,
		Title:      book.Title,
		Status:     status,
	}
	if err := s.db.WithContext(ctx).Create(&item).Error; err != nil {
		if isUniqueViolation(err) {
			var existing models.TrackingItem
			ferr := s.db.WithContext(ctx).
				Where("user_id = ? AND type = ? AND external_id = ?", userID, TypeBook, book.ISBN13).
				First(&existing).Error
			if ferr != nil {
				return nil, fmt.Errorf("loading existing tracking item: %w", ferr)
			}
			return &existing, ErrAlreadyTracked
		}
		return nil, fmt.Errorf("creating tracking item: %w", err)
	}
	return &item, nil
}

// SearchTitle returns OpenLibrary works matching a free-text query by TITLE or
// AUTHOR, for the add-by-name flow. It runs both an OpenLibrary title search and
// an author search and merges the results (title matches first), deduped by work
// key. A blank query yields ErrEmptyQuery; an upstream failure is wrapped in
// ErrUpstream. The caller lists a work's editions (ListEditions) then adds the
// chosen ISBN through the existing Scan + Track path.
func (s *Service) SearchTitle(ctx context.Context, query string) ([]openlibrary.TitleResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyQuery
	}
	byTitle, err := s.metadata.SearchByTitle(ctx, query)
	if err != nil {
		return nil, errors.Join(ErrUpstream, err)
	}
	byAuthor, err := s.metadata.SearchByAuthor(ctx, query)
	if err != nil {
		return nil, errors.Join(ErrUpstream, err)
	}

	const maxResults = 20
	merged := make([]openlibrary.TitleResult, 0, maxResults)
	seen := make(map[string]bool)
	for _, group := range [][]openlibrary.TitleResult{byTitle, byAuthor} {
		for _, r := range group {
			if seen[r.WorkKey] {
				continue
			}
			seen[r.WorkKey] = true
			merged = append(merged, r)
			if len(merged) == maxResults {
				return merged, nil
			}
		}
	}
	return merged, nil
}

// ListEditions returns the ISBN-bearing editions of a work so the user can pick
// which edition (ISBN-13) to track. workKey is a SearchTitle result's WorkKey.
func (s *Service) ListEditions(ctx context.Context, workKey string) ([]openlibrary.Edition, error) {
	workKey = strings.TrimSpace(workKey)
	if workKey == "" {
		return nil, ErrEmptyQuery
	}
	editions, err := s.metadata.ListEditions(ctx, workKey)
	if err != nil {
		return nil, errors.Join(ErrUpstream, err)
	}
	return editions, nil
}

// ListItems returns the user's tracking items, optionally filtered by type
// ("TV"/"BOOK") and status, newest activity first.
func (s *Service) ListItems(ctx context.Context, userID uint, typ, status string) ([]models.TrackingItem, error) {
	if typ != "" && typ != TypeTV && typ != TypeBook && typ != TypeGame && typ != TypeMovie && typ != TypeMusic && typ != TypeCard {
		return nil, fmt.Errorf("%w: unknown type %q", ErrInvalidFilter, typ)
	}
	if status != "" && !isKnownStatus(status) {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalidFilter, status)
	}

	q := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}

	items := []models.TrackingItem{}
	if err := q.Order("title COLLATE NOCASE, id").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("listing items for user %d: %w", userID, err)
	}
	return items, nil
}

// LibraryEntry is a tracking item enriched with the artwork and (for books)
// the metadata needed by the library grid and its expandable detail view.
type LibraryEntry struct {
	Item        models.TrackingItem
	ArtworkPath string   // relative /images path; "" = placeholder
	ShowID      uint     // internal Show.ID for TV items (0 for books)
	Authors     string   // books only
	PageCount   int      // books only
	Description string   // books only
	Platform    string   // games: platform; cards: set name (or printed set code)
	Artist      string   // music: artist; cards: illustrator credit
	Year        int      // music only
	Price       float64  // cards only: market price at scan time
	SetCode     string   // cards only: printed set/collector code, e.g. "LOB-001" or "10/182"
	Tags        []string // source-derived tags/keywords; never nil (empty when none)
	Ownership   []string // user-selected ownership formats (games/music); never nil
}

// ListLibrary is ListItems plus the cached artwork and book metadata, joined
// in from the shared Show/Book caches with two batch queries.
func (s *Service) ListLibrary(ctx context.Context, userID uint, typ, status string) ([]LibraryEntry, error) {
	items, err := s.ListItems(ctx, userID, typ, status)
	if err != nil {
		return nil, err
	}

	// Collect the external IDs to look up per media type.
	var tmdbIDs []int
	var movieTMDBIDs []int
	var isbns []string
	var gameIGDBIDs []int
	var gameBarcodes []string
	var albumExtIDs []string
	var cardExtIDs []string
	for _, it := range items {
		switch it.Type {
		case TypeTV:
			if id, convErr := strconv.Atoi(it.ExternalID); convErr == nil {
				tmdbIDs = append(tmdbIDs, id)
			}
		case TypeMovie:
			if id, convErr := strconv.Atoi(it.ExternalID); convErr == nil {
				movieTMDBIDs = append(movieTMDBIDs, id)
			}
		case TypeBook:
			isbns = append(isbns, it.ExternalID)
		case TypeGame:
			// GAME items are keyed by IGDB id (games.gameExternalID). Legacy
			// items keyed by a barcode parse to a number that matches no IGDB
			// id and simply fall through to the placeholder (backfill gap).
			if id, convErr := strconv.Atoi(it.ExternalID); convErr == nil && len(it.ExternalID) < 9 {
				gameIGDBIDs = append(gameIGDBIDs, id)
			} else {
				gameBarcodes = append(gameBarcodes, it.ExternalID)
			}
		case TypeMusic:
			albumExtIDs = append(albumExtIDs, it.ExternalID)
		case TypeCard:
			cardExtIDs = append(cardExtIDs, it.ExternalID)
		}
	}

	shows := map[string]models.Show{}
	if len(tmdbIDs) > 0 {
		var rows []models.Show
		if err := s.db.WithContext(ctx).Where("tmdb_id IN ?", tmdbIDs).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("loading show artwork: %w", err)
		}
		for _, sh := range rows {
			shows[strconv.Itoa(sh.TMDBID)] = sh
		}
	}
	booksByISBN := map[string]models.Book{}
	if len(isbns) > 0 {
		var rows []models.Book
		if err := s.db.WithContext(ctx).Where("isbn13 IN ?", isbns).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("loading book metadata: %w", err)
		}
		for _, b := range rows {
			booksByISBN[b.ISBN13] = b
		}
	}
	gamesByIGDB := map[string]models.Game{}
	gamesByBarcode := map[string]models.Game{}
	if len(gameIGDBIDs) > 0 || len(gameBarcodes) > 0 {
		var rows []models.Game
		q := s.db.WithContext(ctx)
		if len(gameIGDBIDs) > 0 && len(gameBarcodes) > 0 {
			q = q.Where("igdb_id IN ? OR barcode IN ?", gameIGDBIDs, gameBarcodes)
		} else if len(gameIGDBIDs) > 0 {
			q = q.Where("igdb_id IN ?", gameIGDBIDs)
		} else {
			q = q.Where("barcode IN ?", gameBarcodes)
		}
		if err := q.Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("loading game metadata: %w", err)
		}
		for _, g := range rows {
			if g.IGDBID != 0 {
				gamesByIGDB[strconv.Itoa(g.IGDBID)] = g
			}
			if g.Barcode != "" {
				gamesByBarcode[g.Barcode] = g
			}
		}
	}
	moviesByTMDB := map[string]models.Movie{}
	if len(movieTMDBIDs) > 0 {
		var rows []models.Movie
		if err := s.db.WithContext(ctx).Where("tmdb_id IN ?", movieTMDBIDs).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("loading movie metadata: %w", err)
		}
		for _, m := range rows {
			moviesByTMDB[strconv.Itoa(m.TMDBID)] = m
		}
	}
	albumsByExtID := map[string]models.Album{}
	if len(albumExtIDs) > 0 {
		var rows []models.Album
		if err := s.db.WithContext(ctx).Where("external_id IN ?", albumExtIDs).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("loading album metadata: %w", err)
		}
		for _, a := range rows {
			albumsByExtID[a.ExternalID] = a
		}
	}
	cardsByExtID := map[string]models.Card{}
	if len(cardExtIDs) > 0 {
		var rows []models.Card
		if err := s.db.WithContext(ctx).Where("external_id IN ?", cardExtIDs).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("loading card metadata: %w", err)
		}
		for _, cd := range rows {
			cardsByExtID[cd.ExternalID] = cd
		}
	}

	out := make([]LibraryEntry, 0, len(items))
	// mediaKey records the shared cache-row primary key backing each entry, so
	// its source-derived tags can be batch-loaded per media type below.
	mediaKey := make([]uint, len(items))
	tagIDsByType := map[string][]uint{}
	for i := range items {
		it := items[i]
		entry := LibraryEntry{Item: it, Tags: []string{}, Ownership: []string{}}
		switch it.Type {
		case TypeTV:
			if sh, ok := shows[it.ExternalID]; ok {
				entry.ArtworkPath = sh.PosterPath
				entry.ShowID = sh.ID
				mediaKey[i] = sh.ID
			}
		case TypeBook:
			if b, ok := booksByISBN[it.ExternalID]; ok {
				entry.ArtworkPath = b.CoverPath
				entry.Authors = b.Authors
				entry.PageCount = b.PageCount
				entry.Description = b.Description
				mediaKey[i] = b.ID
			}
		case TypeGame:
			var g models.Game
			var ok bool
			if g, ok = gamesByIGDB[it.ExternalID]; !ok {
				g, ok = gamesByBarcode[it.ExternalID]
			}
			if ok {
				entry.ArtworkPath = g.CoverPath
				entry.Platform = g.Platform
				entry.Description = g.Description
				mediaKey[i] = g.ID
			}
		case TypeMovie:
			if m, ok := moviesByTMDB[it.ExternalID]; ok {
				entry.ArtworkPath = m.PosterPath
				entry.Description = m.Overview
				mediaKey[i] = m.ID
			}
		case TypeMusic:
			if a, ok := albumsByExtID[it.ExternalID]; ok {
				entry.ArtworkPath = a.CoverPath
				entry.Artist = a.Artist
				entry.Year = a.Year
			}
		case TypeCard:
			if cd, ok := cardsByExtID[it.ExternalID]; ok {
				entry.ArtworkPath = cd.CoverPath
				// Cards reuse the games' Platform slot for their set (name
				// when the catalog provides one, else the printed set code),
				// the music Artist slot for the illustrator credit, and
				// Description for the full type/set/artist line. SetCode is
				// the display collector code ("10/182", leading zeros
				// dropped) the grid shows and the UI groups sets by.
				entry.Platform = cd.SetName
				if entry.Platform == "" {
					entry.Platform = cd.SetCode
				}
				entry.Artist = cd.Artist
				entry.SetCode = displayCardCode(cd.SetCode)
				entry.Description = cardDescription(&cd)
				entry.Price = cd.Price
			}
		}
		if mediaKey[i] != 0 {
			tagIDsByType[it.Type] = append(tagIDsByType[it.Type], mediaKey[i])
		}
		out = append(out, entry)
	}

	// Attach source-derived tags with one batch query per media type.
	store := tags.NewStore(s.db)
	tagsByType := map[string]map[uint][]string{}
	for typ, ids := range tagIDsByType {
		byID, err := store.ForMedia(ctx, typ, ids)
		if err != nil {
			return nil, err
		}
		tagsByType[typ] = byID
	}
	for i := range out {
		if mediaKey[i] == 0 {
			continue
		}
		if byID, ok := tagsByType[out[i].Item.Type]; ok {
			if names, ok := byID[mediaKey[i]]; ok {
				out[i].Tags = names
			}
		}
	}

	// Attach user-selected ownership formats. Unlike tags (keyed by the shared
	// cache row), ownership is per tracking item, so it batches by TrackingItem.ID
	// and needs no cache-row join. Only media types with a fixed format set carry
	// ownership; today that is GAME (#11 adds MUSIC).
	ownItemIDs := map[string][]uint{}
	for i := range out {
		switch out[i].Item.Type {
		case TypeGame, TypeMusic, TypeCard:
			ownItemIDs[out[i].Item.Type] = append(ownItemIDs[out[i].Item.Type], out[i].Item.ID)
		}
	}
	if len(ownItemIDs) > 0 {
		store := ownership.NewStore(s.db)
		byItem := map[uint][]string{}
		for typ, ids := range ownItemIDs {
			formatsByItem, err := store.ForItems(ctx, typ, ids)
			if err != nil {
				return nil, err
			}
			for id, formats := range formatsByItem {
				byItem[id] = formats
			}
		}
		for i := range out {
			if formats, ok := byItem[out[i].Item.ID]; ok {
				out[i].Ownership = formats
			}
		}
	}
	return out, nil
}

// UpdateItem patches status and/or progress on the user's tracking item.
// Status must be valid for the item's media type; progress is a
// page number and only meaningful for books (TV progress is derived from
// EpisodeWatch rows, never stored).
func (s *Service) UpdateItem(ctx context.Context, userID, itemID uint, status *string, progress, rating *int) (*models.TrackingItem, error) {
	if status == nil && progress == nil && rating == nil {
		return nil, ErrEmptyUpdate
	}

	item, err := s.userItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}

	if status != nil {
		if !validStatus(item.Type, *status) {
			return nil, fmt.Errorf("%w: %q is not valid for type %s", ErrInvalidStatus, *status, item.Type)
		}
		// TV COMPLETED is system-derived from watched episodes (see
		// tv.reconcileStatus), never a manual action — the only manual TV stop is
		// STOPPED. Reject a client attempt to set it directly.
		if item.Type == TypeTV && *status == StatusCompleted {
			return nil, fmt.Errorf("%w: TV completion is derived from watched episodes, not set manually", ErrInvalidStatus)
		}
		item.Status = *status
	}
	if progress != nil {
		if item.Type != TypeBook {
			return nil, fmt.Errorf("%w: progress is only stored for books", ErrInvalidProgress)
		}
		if *progress < 0 {
			return nil, fmt.Errorf("%w: page number must not be negative", ErrInvalidProgress)
		}
		item.Progress = *progress
	}
	if rating != nil {
		if *rating < 0 || *rating > 5 {
			return nil, fmt.Errorf("%w: rating must be between 0 and 5", ErrInvalidRating)
		}
		item.Rating = *rating
	}

	if err := s.db.WithContext(ctx).Save(item).Error; err != nil {
		return nil, fmt.Errorf("updating item %d: %w", itemID, err)
	}
	return item, nil
}

// SetOwnership replaces the ownership formats on the user's tracked item and
// returns the normalized set (canonical order). Ownership is only defined for
// media types with a fixed format set (games: Physical, GOG); an item of any
// other type, or a format outside the set, yields ownership.ErrInvalidFormat.
// The item is scoped to userID exactly like UpdateItem, so another user's item
// surfaces as ErrItemNotFound.
func (s *Service) SetOwnership(ctx context.Context, userID, itemID uint, formats []string) ([]string, error) {
	item, err := s.userItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}
	store := ownership.NewStore(s.db)
	if err := store.Set(ctx, item.Type, item.ID, formats); err != nil {
		return nil, err
	}
	byItem, err := store.ForItems(ctx, item.Type, []uint{item.ID})
	if err != nil {
		return nil, err
	}
	out := byItem[item.ID]
	if out == nil {
		out = []string{} // never nil, so the handler serializes [] not null
	}
	return out, nil
}

// DeleteItem untracks an item for the user. For TV items the
// user's EpisodeWatch rows for that show are removed too; shared Show, Book,
// and Episode metadata is always kept.
func (s *Service) DeleteItem(ctx context.Context, userID, itemID uint) error {
	item, err := s.userItem(ctx, userID, itemID)
	if err != nil {
		return err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if item.Type == TypeTV {
			if err := deleteShowWatches(tx, userID, item.ExternalID); err != nil {
				return err
			}
		}
		if item.Type == TypeBook {
			// Journal entries are per-user and tied to this item; prune them so
			// untracking leaves no orphaned notes.
			if err := tx.Where("user_id = ? AND item_id = ?", userID, item.ID).
				Delete(&models.BookNote{}).Error; err != nil {
				return fmt.Errorf("deleting notes for item %d: %w", item.ID, err)
			}
		}
		// Ownership rows are keyed by this tracking item; drop them so a later
		// item reusing the id never inherits stale formats.
		if err := tx.Where("media_type = ? AND item_id = ?", item.Type, item.ID).
			Delete(&models.OwnershipFormat{}).Error; err != nil {
			return fmt.Errorf("deleting ownership for item %d: %w", item.ID, err)
		}
		if err := tx.Delete(&models.TrackingItem{}, item.ID).Error; err != nil {
			return fmt.Errorf("deleting item %d: %w", item.ID, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("untracking item %d: %w", itemID, err)
	}
	return nil
}

// deleteShowWatches removes one user's EpisodeWatch rows for the show with
// the given TMDB ID (stored as TrackingItem.ExternalID). Episodes and the
// Show row itself are shared metadata and stay.
func deleteShowWatches(tx *gorm.DB, userID uint, externalID string) error {
	tmdbID, err := strconv.Atoi(externalID)
	if err != nil {
		// Malformed external ID: nothing to prune, still allow untracking.
		return nil
	}
	var show models.Show
	if err := tx.Where(&models.Show{TMDBID: tmdbID}, "TMDBID").First(&show).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("looking up show %d: %w", tmdbID, err)
	}
	episodeIDs := tx.Model(&models.Episode{}).Select("id").Where("show_id = ?", show.ID)
	if err := tx.Where("user_id = ? AND episode_id IN (?)", userID, episodeIDs).
		Delete(&models.EpisodeWatch{}).Error; err != nil {
		return fmt.Errorf("deleting episode watches: %w", err)
	}
	return nil
}

// userItem loads a tracking item scoped to the user. Someone else's item is
// indistinguishable from a missing one (no existence leak).
func (s *Service) userItem(ctx context.Context, userID, itemID uint) (*models.TrackingItem, error) {
	var item models.TrackingItem
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", itemID, userID).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrItemNotFound
		}
		return nil, fmt.Errorf("looking up item %d: %w", itemID, err)
	}
	return &item, nil
}

// DiscoverItem is one book recommendation for the Discover page: an OpenLibrary
// work the user does not track yet, tagged with the author or subject it was
// suggested from. Identity is the WorkKey ("/works/OL...W"); the add-to-library
// flow lists the work's editions to pick an ISBN. CoverPath is a relative
// /images path (OpenLibrary covers are cached through internal/images, never
// hotlinked from the frontend); "" means the UI shows a placeholder.
type DiscoverItem struct {
	WorkKey     string
	Title       string
	Authors     string // comma-joined
	Year        int    // first publication year; 0 when unknown
	CoverPath   string
	Summary     string // the work's opening sentence; "" when OpenLibrary has none
	SuggestedBy string // "books by <author>" or an OpenLibrary subject
}

const (
	maxBookDiscoverSources  = 5  // most-recent tracked books to seed from
	maxBookDiscoverAuthors  = 5  // distinct authors to pull more works by
	maxBookDiscoverSubjects = 3  // distinct subjects to pull more works from
	maxBookDiscoverResults  = 24 // total suggestions returned
)

// Discover suggests books via an author/subject heuristic seeded from the user's
// most recently updated tracked books: more works by the same authors and in the
// same OpenLibrary subjects. It excludes books the user already tracks (matched
// by normalized title, since tracked books are keyed by ISBN and candidates are
// works) or has previously rejected (by work key), dedupes, and caches each
// suggestion's cover through internal/images. A failing lookup is skipped, not
// fatal. Returns an empty slice when the user tracks no books.
func (s *Service) Discover(ctx context.Context, userID uint) ([]DiscoverItem, error) {
	// Seeds: the most recently updated tracked books, for their authors/subjects.
	var sources []models.TrackingItem
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND rating >= 4", userID, TypeBook).
		Order("updated_at DESC").Limit(maxBookDiscoverSources).
		Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("books: discover sources: %w", err)
	}
	if len(sources) == 0 {
		return []DiscoverItem{}, nil
	}

	// Load the seed books' cache rows to read their authors and, via their ids,
	// their source-derived subjects.
	isbns := make([]string, 0, len(sources))
	for _, src := range sources {
		isbns = append(isbns, src.ExternalID)
	}
	var seedBooks []models.Book
	if err := s.db.WithContext(ctx).Where("isbn13 IN ?", isbns).Find(&seedBooks).Error; err != nil {
		return nil, fmt.Errorf("books: load seed books: %w", err)
	}

	authors := make([]string, 0, maxBookDiscoverAuthors)
	authorSeen := map[string]bool{}
	bookIDs := make([]uint, 0, len(seedBooks))
	for _, b := range seedBooks {
		bookIDs = append(bookIDs, b.ID)
		for _, a := range splitAuthors(b.Authors) {
			key := strings.ToLower(a)
			if authorSeen[key] || len(authors) >= maxBookDiscoverAuthors {
				continue
			}
			authorSeen[key] = true
			authors = append(authors, a)
		}
	}
	subjects := s.discoverSubjects(ctx, bookIDs)

	// Dedupe sets: every tracked book title (normalized) and every rejected work.
	trackedTitles, err := s.trackedBookTitleSet(ctx, userID)
	if err != nil {
		return nil, err
	}
	rejected, err := s.bookRejectedSet(ctx, userID)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	out := make([]DiscoverItem, 0, maxBookDiscoverResults)
	for _, a := range authors {
		if len(out) >= maxBookDiscoverResults {
			break
		}
		works, werr := s.metadata.SearchByAuthor(ctx, a)
		if werr != nil {
			log.Printf("books: discover by author %q: %v", a, werr)
			continue
		}
		s.appendDiscoverCandidates(ctx, works, "books by "+a, trackedTitles, rejected, seen, &out)
	}
	for _, subj := range subjects {
		if len(out) >= maxBookDiscoverResults {
			break
		}
		works, werr := s.metadata.SearchBySubject(ctx, subj)
		if werr != nil {
			log.Printf("books: discover by subject %q: %v", subj, werr)
			continue
		}
		s.appendDiscoverCandidates(ctx, works, subj, trackedTitles, rejected, seen, &out)
	}
	return out, nil
}

// appendDiscoverCandidates filters works against the tracked/rejected/seen sets,
// caches their covers, and appends them (tagged with suggestedBy) to out until
// the result cap is reached.
func (s *Service) appendDiscoverCandidates(ctx context.Context, works []openlibrary.TitleResult, suggestedBy string, trackedTitles, rejected, seen map[string]bool, out *[]DiscoverItem) {
	for _, w := range works {
		if len(*out) >= maxBookDiscoverResults {
			return
		}
		if w.WorkKey == "" || rejected[w.WorkKey] || seen[w.WorkKey] {
			continue
		}
		if trackedTitles[normalizeTitle(w.Title)] {
			continue
		}
		seen[w.WorkKey] = true
		*out = append(*out, DiscoverItem{
			WorkKey:     w.WorkKey,
			Title:       w.Title,
			Authors:     strings.Join(w.Authors, ", "),
			Year:        w.FirstYear,
			CoverPath:   s.discoverCover(ctx, w.CoverID),
			Summary:     w.FirstSentence,
			SuggestedBy: suggestedBy,
		})
	}
}

// discoverSubjects returns up to maxBookDiscoverSubjects distinct source-derived
// subjects across the given seed book ids (OpenLibrary subjects persisted as
// tags at scan time). An empty id list or a tag error yields no subjects — the
// author heuristic alone still produces suggestions.
func (s *Service) discoverSubjects(ctx context.Context, bookIDs []uint) []string {
	if len(bookIDs) == 0 {
		return nil
	}
	byID, err := tags.NewStore(s.db).ForMedia(ctx, tags.TypeBook, bookIDs)
	if err != nil {
		log.Printf("books: discover subjects lookup failed: %v", err)
		return nil
	}
	subjects := make([]string, 0, maxBookDiscoverSubjects)
	subjSeen := map[string]bool{}
	for _, names := range byID {
		for _, n := range names {
			key := strings.ToLower(n)
			if subjSeen[key] || len(subjects) >= maxBookDiscoverSubjects {
				continue
			}
			subjSeen[key] = true
			subjects = append(subjects, n)
		}
	}
	return subjects
}

// discoverCover best-effort caches a suggestion's OpenLibrary cover through
// internal/images and returns its relative path. The file is keyed by the cover
// id ("book/olcover-<id>.jpg") so discover covers never collide with ISBN-keyed
// covers of tracked books. A missing cover, nil image store, or failed download
// yields "" (the UI shows a placeholder).
func (s *Service) discoverCover(ctx context.Context, coverID int) string {
	if s.images == nil || coverID == 0 {
		return ""
	}
	url := s.metadata.CoverURL(coverID, "L")
	if url == "" {
		return ""
	}
	path, err := s.images.Fetch(ctx, s.httpClient, url, "book", "olcover-"+strconv.Itoa(coverID))
	if err != nil {
		log.Printf("books: discover cover download for %d failed: %v", coverID, err)
		return ""
	}
	return path
}

// trackedBookTitleSet is the set of the user's tracked book titles, normalized,
// for deduping suggestions (candidates are works, tracked books are ISBNs, so
// title is the shared key).
func (s *Service) trackedBookTitleSet(ctx context.Context, userID uint) (map[string]bool, error) {
	var rows []struct{ Title string }
	if err := s.db.WithContext(ctx).Model(&models.TrackingItem{}).
		Where("user_id = ? AND type = ?", userID, TypeBook).
		Select("title").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("books: load tracked titles: %w", err)
	}
	set := make(map[string]bool, len(rows))
	for _, r := range rows {
		set[normalizeTitle(r.Title)] = true
	}
	return set, nil
}

// bookRejectedSet is the set of work keys the user has dismissed from book
// Discover.
func (s *Service) bookRejectedSet(ctx context.Context, userID uint) (map[string]bool, error) {
	var rows []struct{ ExternalID string }
	if err := s.db.WithContext(ctx).Model(&models.RejectedRec{}).
		Where("user_id = ? AND type = ?", userID, TypeBook).
		Select("external_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("books: load rejected recs: %w", err)
	}
	set := make(map[string]bool, len(rows))
	for _, r := range rows {
		set[r.ExternalID] = true
	}
	return set, nil
}

// RejectRec hides a book suggestion (by work key) so Discover will not surface
// it again.
func (s *Service) RejectRec(ctx context.Context, userID uint, workKey string) error {
	workKey = strings.TrimSpace(workKey)
	if workKey == "" {
		return fmt.Errorf("%w: work key is required", ErrEmptyQuery)
	}
	rec := models.RejectedRec{UserID: userID, Type: TypeBook, ExternalID: workKey}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error; err != nil {
		return fmt.Errorf("books: reject recommendation: %w", err)
	}
	return nil
}

// cardTypeLine renders a card's type and (for Yu-Gi-Oh!) race as one display
// string, e.g. "Normal Monster · Dragon" or "Pokémon — Stage 2".
func cardTypeLine(cd *models.Card) string {
	switch {
	case cd.Race == "":
		return cd.CardType
	case cd.CardType == "":
		return cd.Race
	default:
		return cd.CardType + " · " + cd.Race
	}
}

// displayCardCode renders a card's printed code for display: a Pokémon
// collector number drops the leading zeros ("010/182" → "10/182"); anything
// else (Yu-Gi-Oh! set codes) passes through unchanged.
func displayCardCode(code string) string {
	num, total, found := strings.Cut(code, "/")
	if !found {
		return code
	}
	trimmed := strings.TrimLeft(num, "0")
	if trimmed == "" {
		trimmed = "0"
	}
	return trimmed + "/" + total
}

// cardDescription renders the card detail line: type/race, the set with its
// display collector code, and the illustrator credit — e.g.
// "Pokémon — Basic · Paradox Rift 10/182 · Illus. Saya Tsuruta". Absent
// pieces are dropped.
func cardDescription(cd *models.Card) string {
	set := strings.TrimSpace(cd.SetName + " " + displayCardCode(cd.SetCode))
	parts := []string{cardTypeLine(cd), set}
	if cd.Artist != "" {
		parts = append(parts, "Illus. "+cd.Artist)
	}
	nonEmpty := parts[:0]
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, " · ")
}

// splitAuthors splits a comma-joined author string ("A, B") into trimmed,
// non-empty names.
func splitAuthors(joined string) []string {
	parts := strings.Split(joined, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if name := strings.TrimSpace(p); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// normalizeTitle lower-cases and trims a title for dedupe comparison, collapsing
// internal whitespace so "The  Hobbit" and "The Hobbit" match.
func normalizeTitle(title string) string {
	return strings.Join(strings.Fields(strings.ToLower(title)), " ")
}

// validStatus reports whether status is allowed for the media type:
// TV/MOVIE → WATCHING, BOOK → READING, GAME → PLAYING, MUSIC → LISTENING,
// CARD → OWNED, plus the shared COMPLETED/PLAN_TO/STOPPED for all.
func validStatus(typ, status string) bool {
	switch status {
	case StatusCompleted, StatusPlanTo, StatusStopped:
		return true
	case StatusWatching:
		return typ == TypeTV || typ == TypeMovie
	case StatusReading:
		return typ == TypeBook
	case StatusPlaying:
		return typ == TypeGame
	case StatusListening:
		return typ == TypeMusic
	case StatusOwned:
		return typ == TypeCard
	default:
		return false
	}
}

// isKnownStatus reports whether status is any valid tracking status (used
// for the type-agnostic library filter).
func isKnownStatus(status string) bool {
	return validStatus(TypeTV, status) || validStatus(TypeBook, status) ||
		validStatus(TypeGame, status) || validStatus(TypeMusic, status) ||
		validStatus(TypeCard, status)
}

// normalizeISBN13 strips hyphens/spaces and validates ISBN-13 shape:
// exactly 13 digits with a 978/979 Bookland prefix.
func normalizeISBN13(isbn string) (string, error) {
	norm := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.TrimSpace(isbn))

	if len(norm) != 13 {
		return "", fmt.Errorf("%w: expected 13 digits, got %d", ErrInvalidISBN, len(norm))
	}
	for _, r := range norm {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("%w: non-digit character", ErrInvalidISBN)
		}
	}
	if !strings.HasPrefix(norm, "978") && !strings.HasPrefix(norm, "979") {
		return "", fmt.Errorf("%w: missing 978/979 prefix", ErrInvalidISBN)
	}
	return norm, nil
}

// isUniqueViolation detects SQLite unique-index failures; glebarez/sqlite
// surfaces them as plain error strings (no GORM error translation).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint")
}

type googleBooksResponse struct {
	Items []struct {
		VolumeInfo struct {
			Title       string   `json:"title"`
			Authors     []string `json:"authors"`
			Description string   `json:"description"`
			PageCount   int      `json:"pageCount"`
			ImageLinks  struct {
				Thumbnail string `json:"thumbnail"`
			} `json:"imageLinks"`
		} `json:"volumeInfo"`
	} `json:"items"`
}

func (s *Service) enrichFromGoogleBooks(ctx context.Context, isbn string, book *models.Book) {
	url := fmt.Sprintf("https://www.googleapis.com/books/v1/volumes?q=isbn:%s", isbn)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "OmniShelf/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var data googleBooksResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return
	}

	if len(data.Items) == 0 {
		return
	}

	info := data.Items[0].VolumeInfo
	if book.Title == "" {
		book.Title = info.Title
	}
	if book.Authors == "" && len(info.Authors) > 0 {
		book.Authors = strings.Join(info.Authors, ", ")
	}
	if book.Description == "" {
		book.Description = info.Description
	}
	if book.PageCount == 0 {
		book.PageCount = info.PageCount
	}

	if book.CoverPath == "" && info.ImageLinks.Thumbnail != "" {
		thumb := info.ImageLinks.Thumbnail
		if strings.HasPrefix(thumb, "http://") {
			thumb = "https://" + thumb[7:]
		}
		path, err := s.images.Fetch(ctx, s.httpClient, thumb, "book", isbn)
		if err == nil {
			book.CoverPath = path
		}
	}
}
