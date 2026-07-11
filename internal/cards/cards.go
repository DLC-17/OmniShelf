// Package cards implements trading-card identification and tracking: a photo
// of a card is OCR'd through Google Cloud Vision, classified as Yu-Gi-Oh! or
// Pokémon by a deterministic waterfall over the text, resolved through the
// matching catalog (YGOPRODeck / the Pokémon TCG API) to a shared Card cache
// row, then linked to the user as a TrackingItem (Type "CARD", status OWNED).
// It mirrors internal/games and internal/music; the unified
// library/list/update/delete operations still live in the books service, which
// this package feeds by writing CARD tracking items.
//
// Every mutating operation is scoped by the caller-supplied userID, taken from
// the JWT — never from client input.
package cards

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/pokemontcg"
	"github.com/davidlc1229/omnishelf/internal/vision"
	"github.com/davidlc1229/omnishelf/internal/ygoprodeck"
)

// TypeCard is the TrackingItem.Type for trading cards. It mirrors
// books.TypeCard; kept here too so this package has no import cycle with
// books.
const TypeCard = "CARD"

// Supported card games, stored in Card.Game and echoed in scan results.
const (
	GameYugioh  = "YUGIOH"
	GamePokemon = "POKEMON"
)

// StatusOwned is the only card status: a card on the shelf is a card you own.
// It is the default when Add is called without a status.
const StatusOwned = "OWNED"

// visionTimeout bounds the Vision OCR round-trip per scan.
const visionTimeout = 8000 * time.Millisecond

// Sentinel errors translated by the API layer into envelope responses.
var (
	// ErrNotConfigured means Vision has no credentials, so card scans cannot
	// run.
	ErrNotConfigured = errors.New("cards: card scanning not configured")
	// ErrNoText means Vision detected no text on the image.
	ErrNoText = errors.New("cards: no text detected")
	// ErrUnsupportedCard means the OCR text matched no supported card game
	// (no Yu-Gi-Oh! set code and no Pokémon collector number).
	ErrUnsupportedCard = errors.New("cards: unsupported card")
	// ErrCardNotFound means the catalog lookup for the classified card came
	// back empty.
	ErrCardNotFound = errors.New("cards: card not found")
	// ErrUpstream means OCR or a catalog lookup failed for a non-404 reason.
	ErrUpstream = errors.New("cards: card service unavailable")
	// ErrAlreadyTracked means the user already tracks this card.
	ErrAlreadyTracked = errors.New("cards: card already tracked")
	// ErrInvalidStatus means the status is not valid for a card.
	ErrInvalidStatus = errors.New("cards: invalid status")
)

// NotFoundError wraps ErrCardNotFound with what the waterfall classified, so
// the API can echo the game and set code for the UI to show.
type NotFoundError struct {
	Game    string
	SetCode string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("cards: no %s card for set code %s", e.Game, e.SetCode)
}

// Unwrap lets errors.Is(err, ErrCardNotFound) match.
func (e *NotFoundError) Unwrap() error { return ErrCardNotFound }

// OCRClient is the slice of *vision.Client the service needs; tests
// substitute a fake. Its errors must satisfy errors.Is against the vision
// sentinels (ErrNotConfigured, ErrNoText).
type OCRClient interface {
	DetectText(ctx context.Context, imageBytes []byte) (string, error)
}

// YugiohClient is the slice of *ygoprodeck.Client the service needs; tests
// substitute a fake. A missing set code must yield an error satisfying
// errors.Is(err, ygoprodeck.ErrNotFound).
type YugiohClient interface {
	CardBySetCode(ctx context.Context, setCode string) (*ygoprodeck.Card, error)
}

// PokemonClient is the slice of *pokemontcg.Client the service needs; tests
// substitute a fake. An empty result must yield an error satisfying
// errors.Is(err, pokemontcg.ErrNotFound).
type PokemonClient interface {
	FindCard(ctx context.Context, name, number, printedTotal string) (*pokemontcg.Card, error)
}

// ImageStore is the slice of *images.Store the service needs; tests substitute
// a fake. Optional: when nil no artwork is downloaded.
type ImageStore interface {
	Fetch(ctx context.Context, httpClient *http.Client, url, kind, externalID string) (string, error)
}

// Service implements card scanning and tracking.
type Service struct {
	db         *gorm.DB
	ocr        OCRClient
	ygo        YugiohClient
	ptcg       PokemonClient
	images     ImageStore
	httpClient *http.Client
}

// NewService wires the service. images may be nil to disable artwork caching.
func NewService(gdb *gorm.DB, ocr OCRClient, ygo YugiohClient, ptcg PokemonClient, images ImageStore) *Service {
	return &Service{
		db:         gdb,
		ocr:        ocr,
		ygo:        ygo,
		ptcg:       ptcg,
		images:     images,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// ScanResult is what a successful card scan returns: the identified card,
// ready to be posted back through Add. CoverPath is a relative /images path
// (artwork is cached through internal/images, never hotlinked); "" means no
// artwork.
type ScanResult struct {
	Game       string
	ExternalID string // "ygo:<SetCode>" | "ptcg:<api card id>"
	Name       string
	CardType   string
	Race       string
	Artist     string // illustrator credit (Pokémon); "" when unknown
	SetCode    string
	SetName    string
	Price      float64
	CoverPath  string
}

// Classification regexps for the deterministic waterfall. A Yu-Gi-Oh! set
// code like "LOB-001"/"MRD-EN060" wins over a Pokémon collector number like
// "4/102"; the noise regexp strips HP/stage markers off a Pokémon card's name
// line.
var (
	ygoSetCodeRE    = regexp.MustCompile(`([A-Z0-9]{2,4}-[A-Z0-9]{3,5})`)
	pokemonNumberRE = regexp.MustCompile(`(\d{1,3}/\d{1,3})`)
	pokemonNoiseRE  = regexp.MustCompile(`(?i)\s*\d+\s*HP|\bSTAGE\s*[0-2]\b|\bBASIC\b`)
	// pokemonSkipRE matches OCR lines that are card furniture, never the name:
	// the evolution line and standalone category words. Vision often emits the
	// stage badge ("BASIC") as its own first line, so the name must be hunted
	// past such lines rather than assumed to be line 0.
	pokemonSkipRE = regexp.MustCompile(`(?i)EVOLVES\s+FROM|^\s*(POKÉMON|POKEMON|TRAINER|ITEM|SUPPORTER|STADIUM|ENERGY|TOOL)\s*$`)
)

// pokemonNameLines caps how far down the OCR block the name is hunted; on a
// real card the name sits in the top banner, so anything past the first few
// lines is attack/flavor text that would misidentify the card.
const pokemonNameLines = 6

// pokemonFurniture are cleaned lines that are card chrome rather than a name:
// a stage badge like "Basic Pokémon" survives the noise strip as the bare
// word "POKÉMON", which must not be mistaken for the card's name.
var pokemonFurniture = map[string]bool{
	"POKEMON":   true,
	"POKÉMON":   true,
	"TRAINER":   true,
	"ITEM":      true,
	"SUPPORTER": true,
	"STADIUM":   true,
	"ENERGY":    true,
	"TOOL":      true,
}

// pokemonName isolates the card name from the OCR lines: the first of the top
// pokemonNameLines lines that, after dropping furniture lines and stripping
// HP/stage noise, still holds something with letters in it that is not itself
// a furniture word.
func pokemonName(lines []string) string {
	for i, line := range lines {
		if i >= pokemonNameLines {
			break
		}
		if pokemonSkipRE.MatchString(line) {
			continue
		}
		cleaned := strings.Join(strings.Fields(pokemonNoiseRE.ReplaceAllString(line, " ")), " ")
		if len(cleaned) < 2 || !strings.ContainsFunc(cleaned, unicode.IsLetter) {
			continue
		}
		if pokemonFurniture[cleaned] {
			continue
		}
		return cleaned
	}
	return ""
}

// classification is the outcome of the waterfall over the OCR text: which game
// the card belongs to and the identifying strings the catalog dispatch needs.
type classification struct {
	Game    string
	SetCode string
	Name    string // Pokémon only: cleaned first OCR line
}

// classify runs the deterministic waterfall over the raw OCR block: uppercase
// the whole block, split it into lines, test for a Yu-Gi-Oh! set code first
// (short-circuits all further classification), then for a Pokémon collector
// number, whose card name is the first line cleaned of HP/STAGE/BASIC noise.
// Empty text yields ErrNoText; text matching neither game yields
// ErrUnsupportedCard.
func classify(text string) (*classification, error) {
	if strings.TrimSpace(text) == "" {
		return nil, ErrNoText
	}
	upper := strings.ToUpper(text)
	lines := strings.Split(upper, "\n")

	if code := ygoSetCodeRE.FindString(upper); code != "" {
		return &classification{Game: GameYugioh, SetCode: code}, nil
	}
	if num := pokemonNumberRE.FindString(upper); num != "" {
		return &classification{Game: GamePokemon, SetCode: num, Name: pokemonName(lines)}, nil
	}
	return nil, ErrUnsupportedCard
}

// Scan OCRs a card photo and identifies the card: Vision text detection (8s
// budget) → classify waterfall → catalog lookup through YGOPRODeck (by set
// code) or the Pokémon TCG API (by name + collector number). Artwork is
// cached through internal/images so CoverPath is a local /images path.
// Nothing is persisted: the client posts the result back through Add. A card
// the catalog does not know yields *NotFoundError (matching ErrCardNotFound)
// carrying the classified game and set code.
func (s *Service) Scan(ctx context.Context, imageBytes []byte) (*ScanResult, error) {
	if s.ocr == nil {
		return nil, ErrNotConfigured
	}

	octx, cancel := context.WithTimeout(ctx, visionTimeout)
	defer cancel()
	text, err := s.ocr.DetectText(octx, imageBytes)
	if err != nil {
		switch {
		case errors.Is(err, vision.ErrNotConfigured):
			return nil, ErrNotConfigured
		case errors.Is(err, vision.ErrNoText):
			return nil, ErrNoText
		default:
			return nil, errors.Join(ErrUpstream, err)
		}
	}

	cls, err := classify(text)
	if err != nil {
		return nil, err
	}
	// Server-side diagnostic: what the waterfall extracted drives the catalog
	// query, and OCR noise here is the usual cause of a lookup miss. The OCR
	// head shows what the name isolation had to work with.
	head := strings.Split(text, "\n")
	if len(head) > pokemonNameLines {
		head = head[:pokemonNameLines]
	}
	log.Printf("card scan classified: game=%s name=%q setCode=%q ocrHead=%q", cls.Game, cls.Name, cls.SetCode, strings.Join(head, " | "))
	if cls.Game == GameYugioh {
		return s.lookupYugioh(ctx, cls)
	}
	return s.lookupPokemon(ctx, cls)
}

// lookupYugioh resolves a classified Yu-Gi-Oh! card through YGOPRODeck by its
// printed set code.
func (s *Service) lookupYugioh(ctx context.Context, cls *classification) (*ScanResult, error) {
	card, err := s.ygo.CardBySetCode(ctx, cls.SetCode)
	if err != nil {
		if errors.Is(err, ygoprodeck.ErrNotFound) {
			return nil, &NotFoundError{Game: GameYugioh, SetCode: cls.SetCode}
		}
		return nil, errors.Join(ErrUpstream, err)
	}

	res := &ScanResult{
		Game:       GameYugioh,
		ExternalID: "ygo:" + cls.SetCode,
		Name:       card.Name,
		CardType:   card.Type,
		Race:       card.Race,
		SetCode:    cls.SetCode,
		SetName:    card.SetName,
		Price:      card.Price,
	}
	res.CoverPath = s.cacheArtwork(ctx, res.ExternalID, card.ImageURL)
	return res, nil
}

// lookupPokemon resolves a classified Pokémon card through the Pokémon TCG API
// by its cleaned name and collector number.
func (s *Service) lookupPokemon(ctx context.Context, cls *classification) (*ScanResult, error) {
	number, total := splitCollectorNumber(cls.SetCode)
	card, err := s.ptcg.FindCard(ctx, cls.Name, number, total)
	if err != nil {
		if errors.Is(err, pokemontcg.ErrNotFound) {
			return nil, &NotFoundError{Game: GamePokemon, SetCode: cls.SetCode}
		}
		return nil, errors.Join(ErrUpstream, err)
	}

	res := &ScanResult{
		Game:       GamePokemon,
		ExternalID: "ptcg:" + card.ID,
		Name:       card.Name,
		CardType:   pokemonCardType(card.Supertype, card.Subtypes),
		Artist:     card.Artist,
		SetCode:    cls.SetCode,
		SetName:    card.SetName,
		Price:      card.Price,
	}
	res.CoverPath = s.cacheArtwork(ctx, res.ExternalID, card.ImageURL)
	return res, nil
}

// splitCollectorNumber splits a printed "NNN/TTT" collector number into the
// card number (leading zeros stripped, so "004" matches the API's "4") and
// the printed set total.
func splitCollectorNumber(setCode string) (number, total string) {
	parts := strings.SplitN(setCode, "/", 2)
	number = strings.TrimLeft(parts[0], "0")
	if number == "" {
		number = "0"
	}
	if len(parts) == 2 {
		total = parts[1]
	}
	return number, total
}

// pokemonCardType renders the API's supertype/subtypes as one display string,
// e.g. "Pokémon — Stage 2".
func pokemonCardType(supertype string, subtypes []string) string {
	if len(subtypes) == 0 {
		return supertype
	}
	joined := strings.Join(subtypes, ", ")
	if supertype == "" {
		return joined
	}
	return supertype + " — " + joined
}

// cacheArtwork best-effort downloads the card's artwork through the image
// store and returns its relative /images path. The file is keyed by the
// card's ExternalID ("card/ygo_LOB-001.jpg"), so re-scans and Add land at a
// stable path. A nil store, empty URL or failed download logs and yields ""
// (the UI shows a placeholder).
func (s *Service) cacheArtwork(ctx context.Context, externalID, url string) string {
	if s.images == nil || url == "" {
		return ""
	}
	path, err := s.images.Fetch(ctx, s.httpClient, url, "card", coverKey(externalID))
	if err != nil {
		log.Printf("cards: artwork download for %s failed: %v", externalID, err)
		return ""
	}
	return path
}

// coverKey turns an ExternalID into a filesystem-safe artwork filename stem
// (the ":" separator is illegal in Windows filenames). Mirrors internal/music.
func coverKey(externalID string) string {
	return strings.ReplaceAll(externalID, ":", "_")
}

// Add upserts the shared Card cache row by ExternalID and creates the user's
// CARD TrackingItem — the persistence half of the scan flow (mirrors
// games.AddByIGDB). res is the ScanResult the client got back from Scan;
// status defaults to OWNED, the only valid card status. A duplicate returns
// the existing item alongside ErrAlreadyTracked so the API can answer 409.
func (s *Service) Add(ctx context.Context, userID uint, res ScanResult, status string) (*models.Card, *models.TrackingItem, error) {
	if status == "" {
		status = StatusOwned
	}
	if status != StatusOwned {
		return nil, nil, fmt.Errorf("%w: %q is not valid for cards", ErrInvalidStatus, status)
	}

	card := models.Card{
		ExternalID: res.ExternalID,
		Game:       res.Game,
		Name:       res.Name,
		CardType:   res.CardType,
		Race:       res.Race,
		Artist:     res.Artist,
		SetCode:    res.SetCode,
		SetName:    res.SetName,
		Price:      res.Price,
		CoverPath:  res.CoverPath,
	}
	if err := s.upsertCard(ctx, &card); err != nil {
		return nil, nil, err
	}

	item := models.TrackingItem{
		UserID:     userID,
		Type:       TypeCard,
		ExternalID: card.ExternalID,
		Title:      card.Name,
		Status:     status,
	}
	if err := s.db.WithContext(ctx).Create(&item).Error; err != nil {
		if isUniqueViolation(err) {
			var existing models.TrackingItem
			ferr := s.db.WithContext(ctx).
				Where("user_id = ? AND type = ? AND external_id = ?", userID, TypeCard, card.ExternalID).
				First(&existing).Error
			if ferr != nil {
				return nil, nil, fmt.Errorf("loading existing tracking item: %w", ferr)
			}
			return &card, &existing, ErrAlreadyTracked
		}
		return nil, nil, fmt.Errorf("creating tracking item: %w", err)
	}
	return &card, &item, nil
}

// upsertCard creates the Card row or refreshes an existing one for the same
// ExternalID. An already-cached artwork path is kept when the new scan has
// none; the price is refreshed (it is the market price at scan time).
func (s *Service) upsertCard(ctx context.Context, card *models.Card) error {
	var existing models.Card
	err := s.db.WithContext(ctx).Where(&models.Card{ExternalID: card.ExternalID}).First(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := s.db.WithContext(ctx).Create(card).Error; err != nil {
			// Concurrent add of the same card: the unique index makes one
			// Create lose; fall back to the winner's row.
			if isUniqueViolation(err) {
				return s.upsertCard(ctx, card)
			}
			return fmt.Errorf("creating card %s: %w", card.ExternalID, err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("looking up card %s: %w", card.ExternalID, err)
	}

	card.ID = existing.ID
	if card.CoverPath == "" {
		card.CoverPath = existing.CoverPath
	}
	if err := s.db.WithContext(ctx).Save(card).Error; err != nil {
		return fmt.Errorf("updating card %s: %w", card.ExternalID, err)
	}
	return nil
}

// isUniqueViolation detects SQLite unique-index failures; glebarez/sqlite
// surfaces them as plain error strings.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint")
}
