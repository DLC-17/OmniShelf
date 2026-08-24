// Package gog implements the GOG.com OAuth2 and library synchronization client
// for genuinely owned, DRM-free video game tracking.
package gog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// Public GOG Galaxy Client credentials used for OAuth authorization
	ClientID     = "46899977096215655"
	ClientSecret = "9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"
	RedirectURI  = "https://embed.gog.com/on_login_success?origin=client"

	AuthBaseURL  = "https://auth.gog.com"
	EmbedBaseURL = "https://embed.gog.com"
)

var (
	ErrNotAuthenticated   = errors.New("gog: not authenticated or token expired")
	ErrInvalidCode        = errors.New("gog: invalid authorization code")
	ErrInvalidCredentials = errors.New("gog: invalid email or password")
	ErrTwoFactorRequired  = errors.New("gog: two-factor authentication code required")
	ErrUpstream           = errors.New("gog: service unreachable")
)

// TokenResponse is returned by GOG auth token exchange.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	UserID       string `json:"user_id"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// UserData holds basic GOG account information.
type UserData struct {
	Username   string `json:"username"`
	UserID     string `json:"userId"`
	Country    string `json:"country"`
	IsLoggedIn bool   `json:"isLoggedIn"`
}

// OwnedGame represents a game owned in the user's GOG library.
type OwnedGame struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Slug     string   `json:"slug"`
	CoverURL string   `json:"image"`
	URL      string   `json:"url"`
	Tags     []string `json:"tags"`
}

// Client interacts with the GOG API.
type Client struct {
	httpClient *http.Client
}

// New creates a new GOG API client.
func New() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// AuthURL returns the standard GOG OAuth authorization URL for user login.
func AuthURL() string {
	q := url.Values{}
	q.Set("client_id", ClientID)
	q.Set("redirect_uri", RedirectURI)
	q.Set("response_type", "code")
	q.Set("layout", "client2")
	return fmt.Sprintf("%s/auth?%s", AuthBaseURL, q.Encode())
}

var loginTokenRegex = regexp.MustCompile(`name="login\[_token\]"\s+value="([^"]+)"`)
var secondFactorTokenRegex = regexp.MustCompile(`name="second_step_authentication\[_token\]"\s+value="([^"]+)"`)

// LoginWithCredentials performs a direct in-app login to GOG using email and password,
// capturing the resulting OAuth authorization code automatically without manual copy-pasting.
func (c *Client) LoginWithCredentials(ctx context.Context, email, password, twoFactorCode string) (*TokenResponse, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	authClient := &http.Client{
		Jar:     jar,
		Timeout: 25 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Stop redirect once we hit the on_login_success URL so we can read the code from the Location header
			if strings.Contains(req.URL.String(), "on_login_success") && strings.Contains(req.URL.String(), "code=") {
				return http.ErrUseLastResponse
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}

	// Step 1: GET the GOG auth page to obtain the CSRF login[_token] and session cookies
	initURL := AuthURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, initURL, nil)
	if err != nil {
		return nil, fmt.Errorf("init login request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := authClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading login page: %w", err)
	}

	bodyStr := string(bodyBytes)
	match := loginTokenRegex.FindStringSubmatch(bodyStr)
	if len(match) < 2 {
		return nil, fmt.Errorf("%w: could not find login token on GOG auth page", ErrUpstream)
	}
	loginToken := match[1]

	// Step 2: POST credentials to https://auth.gog.com/login_check
	form := url.Values{}
	form.Set("login[username]", strings.TrimSpace(email))
	form.Set("login[password]", password)
	form.Set("login[_token]", loginToken)
	form.Set("login[login]", "")
	form.Set("login[remember_me]", "1")

	loginReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/login_check", AuthBaseURL), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building login_check request: %w", err)
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	loginReq.Header.Set("Referer", initURL)

	loginResp, err := authClient.Do(loginReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer loginResp.Body.Close()

	loginBody, _ := io.ReadAll(loginResp.Body)
	loginBodyStr := string(loginBody)

	// Check if redirected to on_login_success with code
	loc := loginResp.Header.Get("Location")
	if loc == "" && loginResp.Request != nil && loginResp.Request.URL != nil {
		loc = loginResp.Request.URL.String()
	}

	if strings.Contains(loc, "code=") {
		return c.ExchangeCode(ctx, loc)
	}

	// Check if 2FA is required
	if strings.Contains(loginBodyStr, "second_step_authentication") || strings.Contains(loc, "two_step") {
		if strings.TrimSpace(twoFactorCode) == "" {
			return nil, ErrTwoFactorRequired
		}

		// Submit 2FA code
		twoStepTokenMatch := secondFactorTokenRegex.FindStringSubmatch(loginBodyStr)
		twoStepToken := ""
		if len(twoStepTokenMatch) >= 2 {
			twoStepToken = twoStepTokenMatch[1]
		}

		twoStepForm := url.Values{}
		twoStepForm.Set("second_step_authentication[token][letter_1]", "")
		twoStepForm.Set("second_step_authentication[token][letter_2]", "")
		twoStepForm.Set("second_step_authentication[token][letter_3]", "")
		twoStepForm.Set("second_step_authentication[token][letter_4]", "")
		twoStepForm.Set("second_step_authentication[send]", "")
		twoStepForm.Set("second_step_authentication[_token]", twoStepToken)
		twoStepForm.Set("second_step_authentication[token]", strings.TrimSpace(twoFactorCode))

		twoStepReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/login/two_step/check", AuthBaseURL), strings.NewReader(twoStepForm.Encode()))
		twoStepReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		twoStepReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

		twoStepResp, err := authClient.Do(twoStepReq)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
		}
		defer twoStepResp.Body.Close()

		twoStepLoc := twoStepResp.Header.Get("Location")
		if strings.Contains(twoStepLoc, "code=") {
			return c.ExchangeCode(ctx, twoStepLoc)
		}
	}

	// Check error text on GOG login page
	if strings.Contains(loginBodyStr, "Invalid details") || strings.Contains(loginBodyStr, "reCAPTCHA") || loginResp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidCredentials
	}

	return nil, fmt.Errorf("GOG login could not be completed automatically. Please check your credentials.")
}

// ExchangeCode exchanges an authorization code for an access token and refresh token.
func (c *Client) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, ErrInvalidCode
	}

	// If user pasted full redirect URL, extract the code parameter
	if strings.Contains(code, "code=") {
		if u, err := url.Parse(code); err == nil {
			if qCode := u.Query().Get("code"); qCode != "" {
				code = qCode
			}
		}
		if strings.Contains(code, "code=") {
			parts := strings.Split(code, "code=")
			if len(parts) > 1 {
				code = strings.Split(parts[1], "&")[0]
			}
		}
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, ErrInvalidCode
	}

	endpoint := fmt.Sprintf("%s/token", AuthBaseURL)
	q := url.Values{}
	q.Set("client_id", ClientID)
	q.Set("client_secret", ClientSecret)
	q.Set("grant_type", "authorization_code")
	q.Set("code", code)
	q.Set("redirect_uri", RedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?%s", endpoint, q.Encode()), nil)
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	if tr.Error != "" || tr.AccessToken == "" {
		return nil, fmt.Errorf("%w: %s - %s", ErrInvalidCode, tr.Error, tr.ErrorDesc)
	}

	return &tr, nil
}

// RefreshToken refreshes an expired access token using the refresh token.
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, ErrNotAuthenticated
	}

	endpoint := fmt.Sprintf("%s/token", AuthBaseURL)
	q := url.Values{}
	q.Set("client_id", ClientID)
	q.Set("client_secret", ClientSecret)
	q.Set("grant_type", "refresh_token")
	q.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s?%s", endpoint, q.Encode()), nil)
	if err != nil {
		return nil, fmt.Errorf("creating refresh request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading refresh response: %w", err)
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parsing refresh response: %w", err)
	}

	if tr.Error != "" || tr.AccessToken == "" {
		return nil, fmt.Errorf("%w: %s", ErrNotAuthenticated, tr.ErrorDesc)
	}

	return &tr, nil
}

// GetUserData fetches the current authenticated user's account details.
func (c *Client) GetUserData(ctx context.Context, accessToken string) (*UserData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/userData.json", EmbedBaseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrNotAuthenticated
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}

	var ud UserData
	if err := json.NewDecoder(resp.Body).Decode(&ud); err != nil {
		return nil, fmt.Errorf("decoding user data: %w", err)
	}

	return &ud, nil
}

type filteredGamesResponse struct {
	Products []struct {
		ID       int      `json:"id"`
		Title    string   `json:"title"`
		Slug     string   `json:"slug"`
		Image    string   `json:"image"`
		URL      string   `json:"url"`
		Category string   `json:"category"`
		Tags     []string `json:"tags"`
	} `json:"products"`
	TotalPages int `json:"totalPages"`
}

// GetOwnedGames retrieves all games genuinely owned in the authenticated user's GOG library.
func (c *Client) GetOwnedGames(ctx context.Context, accessToken string) ([]OwnedGame, error) {
	var allGames []OwnedGame
	page := 1

	for {
		// Use the private account library endpoint (mediaType=1 specifies games)
		endpoint := fmt.Sprintf("%s/account/getFilteredProducts?mediaType=1&page=%d", EmbedBaseURL, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			return nil, ErrNotAuthenticated
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			// If getFilteredProducts is not responding, fallback to user data games list
			return c.getOwnedGamesFallback(ctx, accessToken)
		}

		var fgr filteredGamesResponse
		err = json.NewDecoder(resp.Body).Decode(&fgr)
		resp.Body.Close()
		if err != nil {
			return c.getOwnedGamesFallback(ctx, accessToken)
		}

		for _, p := range fgr.Products {
			img := p.Image
			if img != "" && !strings.HasPrefix(img, "http") {
				img = "https:" + img
			}
			allGames = append(allGames, OwnedGame{
				ID:       p.ID,
				Title:    p.Title,
				Slug:     p.Slug,
				CoverURL: img,
				URL:      p.URL,
				Tags:     p.Tags,
			})
		}

		if page >= fgr.TotalPages || len(fgr.Products) == 0 {
			break
		}
		page++
	}

	return allGames, nil
}

type userDataGamesResponse struct {
	Owned []int `json:"owned"`
}

// getOwnedGamesFallback fetches owned game IDs from user data endpoint if filtered endpoint is unavailable.
func (c *Client) getOwnedGamesFallback(ctx context.Context, accessToken string) ([]OwnedGame, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/user/data/games", EmbedBaseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrNotAuthenticated
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}

	var udg userDataGamesResponse
	if err := json.NewDecoder(resp.Body).Decode(&udg); err != nil {
		return nil, fmt.Errorf("decoding fallback games: %w", err)
	}

	games := make([]OwnedGame, 0, len(udg.Owned))
	for _, id := range udg.Owned {
		games = append(games, OwnedGame{
			ID:    id,
			Title: fmt.Sprintf("GOG Game #%d", id),
			Slug:  strconv.Itoa(id),
		})
	}

	return games, nil
}
