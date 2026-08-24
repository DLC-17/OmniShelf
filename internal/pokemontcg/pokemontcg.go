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

// sanitizeLucene cleans Lucene query operator characters that cause api.pokemontcg.io
// to crash with HTTP 500.
func sanitizeLucene(s string) string {
	repl := strings.NewReplacer(
		`"`, " ", `\`, " ", `/`, " ", `:`, " ", `+`, " ", `-`, " ", `!`, " ", `(`, " ", `)`, " ",
		`{`, " ", `}`, " ", `[`, " ", `]`, " ", `^`, " ", `~`, " ", `*`, " ", `?`, " ", `&`, " ", `|`, " ",
	)
	return strings.Join(strings.Fields(repl.Replace(s)), " ")
}

// queryCards executes a single search query against the Pokémon TCG API with retries for transient 5xx/429 errors.
func (c *Client) queryCards(ctx context.Context, query string) ([]cardPayload, error) {
	q := url.Values{"q": {query}}
	reqURL := c.baseURL + "/cards?" + q.Encode()

	var resp *http.Response
	var raw []byte
	var err error

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*500) * time.Millisecond):
			}
		}

		req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if rerr != nil {
			return nil, fmt.Errorf("pokemontcg: build request: %w", rerr)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "OmniShelf/1.0 (Media Tracker)")
		if c.apiKey != "" {
			req.Header.Set("X-Api-Key", c.apiKey)
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			continue
		}

		raw, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<22))
		_ = resp.Body.Close()

		// Retry on Cloudflare / upstream transient server errors
		if resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusInternalServerError {
			continue
		}

		// Non-retryable status or success
		break
	}

	if err != nil {
		return nil, errors.Join(ErrUpstream, err)
	}
	if resp == nil {
		return nil, ErrUpstream
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: cards returned status %d: %s", ErrUpstream, resp.StatusCode, string(raw))
	}

	var payload cardsResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("%w: decode cards: %v", ErrUpstream, err)
	}
	return payload.Data, nil
}

// FindCard searches for a card by its printed name and collector number.
// number is the part before the slash with leading zeros stripped ("4" for
// "004/102"); printedTotal is the part after it ("102").
//
// Because api.pokemontcg.io frequently returns HTTP 500 when Lucene queries
// encounter special characters or OCR noise, FindCard tries queries in a
// fallback waterfall:
//  1. name:"<cleaned name>" number:<number>
//  2. name:<first word>* number:<number>
//  3. number:<number> (filtered client-side by set total / card name)
func (c *Client) FindCard(ctx context.Context, name, number, printedTotal string) (*Card, error) {
	cleanName := sanitizeLucene(name)
	cleanNum := sanitizeLucene(number)
	if cleanNum == "" && cleanName == "" {
		return nil, fmt.Errorf("%w: empty card name and number after OCR cleanup", ErrNotFound)
	}

	words := strings.Fields(cleanName)
	firstWord := ""
	lastWord := ""
	if len(words) > 0 {
		firstWord = words[0]
		lastWord = words[len(words)-1]
	}

	var queries []string
	if cleanName != "" && cleanNum != "" {
		queries = append(queries, fmt.Sprintf(`name:"%s" number:%s`, cleanName, cleanNum))
	}
	if lastWord != "" && cleanNum != "" && lastWord != cleanName {
		queries = append(queries, fmt.Sprintf(`name:%s* number:%s`, lastWord, cleanNum))
	}
	if firstWord != "" && firstWord != lastWord && cleanNum != "" {
		queries = append(queries, fmt.Sprintf(`name:%s* number:%s`, firstWord, cleanNum))
	}
	if cleanNum != "" {
		queries = append(queries, fmt.Sprintf(`number:%s`, cleanNum))
	}
	if cleanName != "" && len(queries) == 0 {
		queries = append(queries, fmt.Sprintf(`name:"%s"`, cleanName))
	}

	var data []cardPayload
	var lastErr error

	for _, q := range queries {
		results, err := c.queryCards(ctx, q)
		if err == nil && len(results) > 0 {
			data = results
			lastErr = nil
			break
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			lastErr = err
		}
	}

	if len(data) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("%w for %q number %s", ErrNotFound, name, number)
	}

	// Pick the highest scoring match from the candidate results.
	total, _ := strconv.Atoi(printedTotal)
	bestPick := data[0]
	bestScore := -1

	for _, d := range data {
		score := 0
		upperName := strings.ToUpper(d.Name)

		// Exact collector number match
		if cleanNum != "" && d.Number == cleanNum {
			score += 50
		}

		// Set total match
		if total > 0 && (d.Set.PrintedTotal == total || d.Set.Total == total) {
			score += 40
		}

		// Word-level name matching (prioritize species/sub-names)
		for _, w := range words {
			upperW := strings.ToUpper(w)
			if len(upperW) < 2 {
				continue
			}
			if upperName == upperW {
				score += 100 // exact full name match
			} else if strings.Contains(upperName, upperW) {
				// Substring match (e.g., "ZORUA" inside "Hisuian Zorua" or "N's Zorua")
				score += 60
			}
		}

		if score > bestScore {
			bestScore = score
			bestPick = d
		}
	}

	return &Card{
		ID:             bestPick.ID,
		Name:           bestPick.Name,
		Supertype:      bestPick.Supertype,
		Subtypes:       bestPick.Subtypes,
		Artist:         bestPick.Artist,
		SetName:        bestPick.Set.Name,
		SetReleaseDate: bestPick.Set.ReleaseDate,
		Price:          bestPick.marketPrice(),
		ImageURL:       bestPick.Images.Large,
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
