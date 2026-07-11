// Package pokemontcg provides a client for the Pokémon TCG API
// (https://api.pokemontcg.io), used to resolve a card's printed name and
// collector number ("NNN/TTT") to its full metadata, market price and artwork.
// An API key is optional: keyless requests work at lower rate limits.
package pokemontcg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.pokemontcg.io/v2"

// Sentinel errors callers translate into the API envelope.
var (
	// ErrNotFound means the API has no card for the name/number query.
	ErrNotFound = errors.New("pokemontcg: no card found")
	// ErrUpstream means the lookup failed for a non-404 reason.
	ErrUpstream = errors.New("pokemontcg: service unavailable")
)

// Client talks to the Pokémon TCG API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Option customizes a Client.
type Option func(*Client)

// WithBaseURL overrides the Pokémon TCG API base URL (tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient overrides the shared HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// New returns a Pokémon TCG API client. apiKey may be empty: the API works
// without one at lower rate limits, so no Configured gate is needed.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		// api.pokemontcg.io routinely stalls for two-digit seconds on a cold
		// query, so give it more headroom than the other catalogs get.
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Card is the Pokémon card metadata we consume. ImageURL is remote artwork
// that must be downloaded through internal/images — never hotlinked from the
// frontend.
type Card struct {
	ID             string // API card id, e.g. "base1-4"
	Name           string
	Supertype      string   // e.g. "Pokémon"
	Subtypes       []string // e.g. ["Stage 2"]
	Artist         string   // illustrator credit; may be empty
	SetName        string
	SetReleaseDate string  // "YYYY/MM/DD" as returned by the API
	Price          float64 // market price; 0 when unknown
	ImageURL       string  // images.large
}

// cardsResponse models the /cards payload subset we use.
type cardsResponse struct {
	Data []cardPayload `json:"data"`
}

// cardPayload is one /cards entry. Price fields are pointers so an absent
// tier is distinguishable from a zero price.
type cardPayload struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Supertype string   `json:"supertype"`
	Subtypes  []string `json:"subtypes"`
	Artist    string   `json:"artist"`
	Number    string   `json:"number"`
	Set       struct {
		Name         string `json:"name"`
		PrintedTotal int    `json:"printedTotal"`
		Total        int    `json:"total"`
		ReleaseDate  string `json:"releaseDate"`
	} `json:"set"`
	Images struct {
		Large string `json:"large"`
	} `json:"images"`
	TCGPlayer struct {
		Prices map[string]struct {
			Market *float64 `json:"market"`
		} `json:"prices"`
	} `json:"tcgplayer"`
	Cardmarket struct {
		Prices struct {
			AverageSellPrice *float64 `json:"averageSellPrice"`
		} `json:"prices"`
	} `json:"cardmarket"`
}

// FindCard searches for a card by its printed name and collector number.
// number is the part before the slash with leading zeros stripped ("4" for
// "004/102"); printedTotal is the part after it ("102"). The exact entry whose
// number equals number and whose set's printedTotal (or total) equals
// printedTotal wins; when no entry matches exactly the first result is used.
// An empty result set yields ErrNotFound.
func (c *Client) FindCard(ctx context.Context, name, number, printedTotal string) (*Card, error) {
	// The name comes from OCR: quotes, backslashes and other Lucene query
	// operators in it malform the API's q syntax (HTTP 400). Inside a quoted
	// phrase only `"` and `\` are unsafe, so strip those; an empty remainder
	// can never match a card, so report it as a miss rather than querying.
	name = strings.TrimSpace(strings.NewReplacer(`"`, "", `\`, "").Replace(name))
	if name == "" {
		return nil, fmt.Errorf("%w: empty card name after OCR cleanup", ErrNotFound)
	}
	q := url.Values{"q": {fmt.Sprintf(`name:"%s" number:%s`, name, number)}}

	// The API sporadically stalls past even the generous client timeout while
	// an immediate retry succeeds, so one transport-level failure gets a
	// second attempt before the scan is failed. The request must be rebuilt
	// per attempt: an *http.Request is single-use.
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/cards?"+q.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("pokemontcg: build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		if c.apiKey != "" {
			req.Header.Set("X-Api-Key", c.apiKey)
		}

		resp, err = c.httpClient.Do(req)
		if err == nil {
			break
		}
		if attempt >= 1 || ctx.Err() != nil {
			return nil, errors.Join(ErrUpstream, err)
		}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w for %q number %s", ErrNotFound, name, number)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: cards returned status %d: %s", ErrUpstream, resp.StatusCode, string(raw))
	}

	var payload cardsResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("%w: decode cards: %v", ErrUpstream, err)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("%w for %q number %s", ErrNotFound, name, number)
	}

	pick := payload.Data[0]
	if total, terr := strconv.Atoi(printedTotal); terr == nil {
		for _, d := range payload.Data {
			if d.Number == number && (d.Set.PrintedTotal == total || d.Set.Total == total) {
				pick = d
				break
			}
		}
	}

	return &Card{
		ID:             pick.ID,
		Name:           pick.Name,
		Supertype:      pick.Supertype,
		Subtypes:       pick.Subtypes,
		Artist:         pick.Artist,
		SetName:        pick.Set.Name,
		SetReleaseDate: pick.Set.ReleaseDate,
		Price:          pick.marketPrice(),
		ImageURL:       pick.Images.Large,
	}, nil
}

// marketPrice picks the card's market price: the first present of the
// TCGplayer normal/holofoil/reverseHolofoil tiers' market value, falling back
// to the Cardmarket average sell price. 0 when no source has a price.
func (p *cardPayload) marketPrice() float64 {
	for _, tier := range []string{"normal", "holofoil", "reverseHolofoil"} {
		if v, ok := p.TCGPlayer.Prices[tier]; ok && v.Market != nil {
			return *v.Market
		}
	}
	if p.Cardmarket.Prices.AverageSellPrice != nil {
		return *p.Cardmarket.Prices.AverageSellPrice
	}
	return 0
}
