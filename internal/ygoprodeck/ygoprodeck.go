// Package ygoprodeck provides a client for the YGOPRODeck card database
// (https://db.ygoprodeck.com), used to resolve a Yu-Gi-Oh! card's printed set
// code (e.g. "LOB-001") to its name, type, race, market price and artwork.
// The API needs no authentication.
package ygoprodeck

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

const defaultBaseURL = "https://db.ygoprodeck.com/api/v7"

// Sentinel errors callers translate into the API envelope.
var (
	// ErrNotFound means YGOPRODeck has no card for the set code.
	ErrNotFound = errors.New("ygoprodeck: no card for set code")
	// ErrUpstream means the lookup failed for a non-404 reason.
	ErrUpstream = errors.New("ygoprodeck: service unavailable")
)

// Client talks to the YGOPRODeck API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option customizes a Client.
type Option func(*Client)

// WithBaseURL overrides the YGOPRODeck base URL (tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient overrides the shared HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// New returns a YGOPRODeck client.
func New(opts ...Option) *Client {
	c := &Client{
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Card is the Yu-Gi-Oh! card metadata we consume. ImageURL is remote artwork
// that must be downloaded through internal/images — never hotlinked from the
// frontend.
type Card struct {
	Name     string
	Type     string  // e.g. "Normal Monster"
	Race     string  // YGO race/attribute line, e.g. "Dragon"
	SetName  string  // e.g. "Legend of Blue Eyes White Dragon"
	Rarity   string  // printing rarity for this set code, e.g. "Ultra Rare"
	Price    float64 // set-specific price when known, else TCGplayer market; 0 when unknown
	ImageURL string
}

// setInfo models the /cardsetsinfo.php payload: the printing identified by one
// exact set code, carrying the canonical card id the detail lookup needs plus
// the set-specific name/rarity/price.
type setInfo struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	SetName   string `json:"set_name"`
	SetCode   string `json:"set_code"`
	SetRarity string `json:"set_rarity"`
	SetPrice  string `json:"set_price"`
}

// cardinfoResponse models the /cardinfo.php payload subset we use. Prices come
// back as strings in their JSON.
type cardinfoResponse struct {
	Data []struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Race       string `json:"race"`
		CardImages []struct {
			ImageURL string `json:"image_url"`
		} `json:"card_images"`
		CardPrices []struct {
			TCGPlayerPrice string `json:"tcgplayer_price"`
		} `json:"card_prices"`
	} `json:"data"`
}

// CardBySetCode looks a card up by its printed set code (e.g. "LOB-001") in
// two steps: /cardsetsinfo.php?setcode= resolves the exact printing (card id,
// set name, rarity and the set-specific price — cardinfo's own cardset filter
// matches set NAMES, not codes), then /cardinfo.php?id= fills in the card's
// type, race, artwork and fallback price. A set code YGOPRODeck does not know
// yields ErrNotFound (the API answers a no-match query with 400 and an error
// body). A missing or unparseable price is not an error: the card is returned
// with Price 0.
func (c *Client) CardBySetCode(ctx context.Context, setCode string) (*Card, error) {
	var info setInfo
	q := url.Values{"setcode": {setCode}}
	if err := c.getJSON(ctx, "/cardsetsinfo.php?"+q.Encode(), setCode, &info); err != nil {
		return nil, err
	}
	if info.ID == 0 {
		return nil, fmt.Errorf("%w %s", ErrNotFound, setCode)
	}

	card := &Card{Name: info.Name, SetName: info.SetName, Rarity: info.SetRarity}
	if p, perr := strconv.ParseFloat(info.SetPrice, 64); perr == nil && p > 0 {
		card.Price = p
	}

	var payload cardinfoResponse
	q = url.Values{"id": {strconv.Itoa(info.ID)}}
	if err := c.getJSON(ctx, "/cardinfo.php?"+q.Encode(), setCode, &payload); err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("%w %s", ErrNotFound, setCode)
	}

	d := payload.Data[0]
	card.Type = d.Type
	card.Race = d.Race
	if card.Name == "" {
		card.Name = d.Name
	}
	if len(d.CardImages) > 0 {
		card.ImageURL = d.CardImages[0].ImageURL
	}
	if card.Price == 0 && len(d.CardPrices) > 0 {
		if p, perr := strconv.ParseFloat(d.CardPrices[0].TCGPlayerPrice, 64); perr == nil {
			card.Price = p
		}
	}
	return card, nil
}

// getJSON GETs one YGOPRODeck endpoint and decodes its JSON body into out. A
// 400/404 answer means the set code missed (ErrNotFound); any other non-200
// status, transport failure or undecodable body is ErrUpstream.
func (c *Client) getJSON(ctx context.Context, pathAndQuery, setCode string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+pathAndQuery, nil)
	if err != nil {
		return fmt.Errorf("ygoprodeck: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Join(ErrUpstream, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w %s", ErrNotFound, setCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s returned status %d: %s", ErrUpstream, pathAndQuery, resp.StatusCode, string(raw))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: decode %s: %v", ErrUpstream, pathAndQuery, err)
	}
	return nil
}
