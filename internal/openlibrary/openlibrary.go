// Package openlibrary provides a client for OpenLibrary ISBN lookups.
//
// Every request carries the mandatory User-Agent
// "OmniShelf/1.0 (<contact email>)". Lookups
// return whatever metadata exists — missing works, authors or covers never
// block a result. A missing ISBN yields ErrNotFound.
package openlibrary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL  = "https://openlibrary.org"
	defaultCoverURL = "https://covers.openlibrary.org"
)

// ErrNotFound is returned when OpenLibrary has no record for an ISBN (E4).
var ErrNotFound = errors.New("openlibrary: not found")

// NotFoundError wraps ErrNotFound with the ISBN that missed, so callers can
// echo it in the API error envelope.
type NotFoundError struct {
	ISBN string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("openlibrary: no record for ISBN %s", e.ISBN)
}

// Unwrap lets errors.Is(err, ErrNotFound) match.
func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// Client talks to the OpenLibrary API.
type Client struct {
	userAgent  string
	baseURL    string
	coverURL   string
	httpClient *http.Client
}

// Option customizes a Client.
type Option func(*Client)

// WithBaseURL overrides the OpenLibrary base URL (used by tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithCoverBaseURL overrides the covers.openlibrary.org base URL (tests).
func WithCoverBaseURL(u string) Option {
	return func(c *Client) { c.coverURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient overrides the shared HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// New returns an OpenLibrary client identifying itself as
// "OmniShelf/1.0 (<contactEmail>)" on every request.
func New(contactEmail string, opts ...Option) *Client {
	c := &Client{
		userAgent:  fmt.Sprintf("OmniShelf/1.0 (%s)", contactEmail),
		baseURL:    defaultBaseURL,
		coverURL:   defaultCoverURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Book is the merged edition + work metadata for one ISBN. Any field may be
// zero-valued when OpenLibrary lacks the data (E5).
type Book struct {
	ISBN13      string
	Title       string
	Authors     []string
	Description string
	PageCount   int
	CoverID     int      // OpenLibrary cover ID; 0 = no cover known
	WorkKey     string
	Subjects    []string // OpenLibrary work subjects (source-derived tags); may be empty
}

// edition is the /isbn/{isbn}.json payload subset we use.
type edition struct {
	Title         string   `json:"title"`
	NumberOfPages int      `json:"number_of_pages"`
	Covers        []int    `json:"covers"`
	ISBN13        []string `json:"isbn_13"`
	Works         []struct {
		Key string `json:"key"`
	} `json:"works"`
	Authors []struct {
		Key string `json:"key"`
	} `json:"authors"`
}

// work is the /works/{id}.json payload subset we use. Description is either
// a plain string or {"type": ..., "value": ...}.
type work struct {
	Description json.RawMessage `json:"description"`
	Covers      []int           `json:"covers"`
	Subjects    []string        `json:"subjects"`
	Authors     []struct {
		Author struct {
			Key string `json:"key"`
		} `json:"author"`
	} `json:"authors"`
}

// author is the /authors/{id}.json payload subset we use.
type author struct {
	Name string `json:"name"`
}

// GetByISBN looks up an edition by ISBN and, when the edition references a
// Work, follows it for the description and author names. Partial metadata is
// returned as-is; only a missing edition record is an error (ErrNotFound).
func (c *Client) GetByISBN(ctx context.Context, isbn string) (*Book, error) {
	var ed edition
	if err := c.getJSON(ctx, "/isbn/"+isbn+".json", &ed); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, &NotFoundError{ISBN: isbn}
		}
		return nil, err
	}

	book := &Book{
		ISBN13:    isbn,
		Title:     ed.Title,
		PageCount: ed.NumberOfPages,
	}
	if len(ed.ISBN13) > 0 {
		book.ISBN13 = ed.ISBN13[0]
	}
	if len(ed.Covers) > 0 {
		book.CoverID = ed.Covers[0]
	}

	// Follow the work reference for description/authors when present.
	// Failures here degrade gracefully — the edition data alone is enough
	// to track the book (E5).
	if len(ed.Works) > 0 {
		book.WorkKey = ed.Works[0].Key
		var w work
		if err := c.getJSON(ctx, book.WorkKey+".json", &w); err == nil {
			book.Description = decodeDescription(w.Description)
			book.Subjects = w.Subjects
			if book.CoverID == 0 && len(w.Covers) > 0 {
				book.CoverID = w.Covers[0]
			}
			if len(ed.Authors) == 0 {
				for _, a := range w.Authors {
					if a.Author.Key != "" {
						ed.Authors = append(ed.Authors, struct {
							Key string `json:"key"`
						}{Key: a.Author.Key})
					}
				}
			}
		}
	}

	for _, a := range ed.Authors {
		var au author
		if err := c.getJSON(ctx, a.Key+".json", &au); err == nil && au.Name != "" {
			book.Authors = append(book.Authors, au.Name)
		}
	}

	return book, nil
}

// TitleResult is one work returned by a title search. A work groups together
// all editions of a book; the caller lists its editions to pick an ISBN.
type TitleResult struct {
	WorkKey      string // OpenLibrary work key, e.g. "/works/OL45804W"
	Title        string
	Authors      []string
	FirstYear    int // first publication year; 0 when unknown
	CoverID      int // OpenLibrary cover ID; 0 = no cover known
	EditionCount int
}

// Edition is one ISBN-bearing edition of a work, for the ISBN picker.
type Edition struct {
	ISBN13      string
	Title       string
	PublishDate string // free-form as OpenLibrary returns it, e.g. "2004"
	CoverID     int
}

// searchResponse is the /search.json payload subset we use.
type searchResponse struct {
	Docs []struct {
		Key              string   `json:"key"`
		Title            string   `json:"title"`
		AuthorName       []string `json:"author_name"`
		FirstPublishYear int      `json:"first_publish_year"`
		CoverI           int      `json:"cover_i"`
		EditionCount     int      `json:"edition_count"`
	} `json:"docs"`
}

// SearchByTitle returns up to 20 works matching a free-text title query. Only a
// transport/decode failure is an error; an empty result set yields an empty
// slice. Callers list a work's editions (ListEditions) to choose an ISBN.
func (c *Client) SearchByTitle(ctx context.Context, title string) ([]TitleResult, error) {
	q := url.Values{
		"title":  {title},
		"fields": {"key,title,author_name,first_publish_year,cover_i,edition_count"},
		"limit":  {"20"},
	}
	var resp searchResponse
	if err := c.getJSON(ctx, "/search.json?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	results := make([]TitleResult, 0, len(resp.Docs))
	for _, d := range resp.Docs {
		results = append(results, TitleResult{
			WorkKey:      d.Key,
			Title:        d.Title,
			Authors:      d.AuthorName,
			FirstYear:    d.FirstPublishYear,
			CoverID:      d.CoverI,
			EditionCount: d.EditionCount,
		})
	}
	return results, nil
}

// editionsResponse is the /works/{id}/editions.json payload subset we use.
type editionsResponse struct {
	Entries []struct {
		Title       string   `json:"title"`
		ISBN13      []string `json:"isbn_13"`
		PublishDate string   `json:"publish_date"`
		Covers      []int    `json:"covers"`
	} `json:"entries"`
}

// ListEditions returns the editions of a work that carry an ISBN-13 (the ones a
// user can actually track). workKey is the "/works/OL...W" key from a
// SearchByTitle result. Editions without an ISBN-13 are skipped.
func (c *Client) ListEditions(ctx context.Context, workKey string) ([]Edition, error) {
	key := strings.Trim(workKey, "/")
	var resp editionsResponse
	if err := c.getJSON(ctx, "/"+key+"/editions.json?limit=50", &resp); err != nil {
		return nil, err
	}
	editions := make([]Edition, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		if len(e.ISBN13) == 0 {
			continue
		}
		ed := Edition{
			ISBN13:      e.ISBN13[0],
			Title:       e.Title,
			PublishDate: e.PublishDate,
		}
		if len(e.Covers) > 0 {
			ed.CoverID = e.Covers[0]
		}
		editions = append(editions, ed)
	}
	return editions, nil
}

// CoverURL returns the covers.openlibrary.org URL for a cover ID at the
// given size ("S", "M" or "L"). The actual download must go through
// internal/images — never hotlink this URL from the frontend.
// Returns "" when the book has no known cover.
func (c *Client) CoverURL(coverID int, size string) string {
	if coverID == 0 {
		return ""
	}
	if size == "" {
		size = "L"
	}
	return fmt.Sprintf("%s/b/id/%d-%s.jpg", c.coverURL, coverID, size)
}

// getJSON performs a GET with the mandatory User-Agent and decodes JSON.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("openlibrary: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("openlibrary: request %s: %w", path, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("openlibrary: %s returned status %d: %s", path, resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("openlibrary: decode %s: %w", path, err)
	}
	return nil
}

// decodeDescription handles OpenLibrary's two description shapes: a bare
// string, or {"type": "/type/text", "value": "..."}.
func decodeDescription(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Value
	}
	return ""
}
