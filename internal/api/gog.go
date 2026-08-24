// Package api implements the GOG OAuth2 connection and library synchronization endpoints.
package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davidlc1229/omnishelf/internal/games"
	"github.com/davidlc1229/omnishelf/internal/gog"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/ownership"
	"github.com/davidlc1229/omnishelf/internal/tags"
)

type gogHandler struct {
	db        *gorm.DB
	gamesSvc  *games.Service
	gogClient *gog.Client
	envToken  string
}

// RegisterGOGRoutes attaches the GOG OAuth and synchronization endpoints to the router.
func RegisterGOGRoutes(grp *gin.RouterGroup, gdb *gorm.DB, gamesSvc *games.Service, envToken string) {
	h := &gogHandler{
		db:        gdb,
		gamesSvc:  gamesSvc,
		gogClient: gog.New(),
		envToken:  strings.TrimSpace(envToken),
	}

	g := grp.Group("/gog")
	{
		g.GET("/status", h.status)
		g.GET("/auth-url", h.authURL)
		g.GET("/popup", h.popupPage)
		g.GET("/callback", h.callback)
		g.POST("/login", h.directLogin)
		g.POST("/connect", h.connect)
		g.POST("/sync", h.sync)
		g.DELETE("/disconnect", h.disconnect)
	}
}

// status returns the current GOG connection status for the authenticated user.
func (h *gogHandler) status(c *gin.Context) {
	userID := CurrentUserID(c)

	var accounts []models.UserGOGAccount
	err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ?", userID).
		Limit(1).
		Find(&accounts).Error

	if err == nil && len(accounts) > 0 && accounts[0].AccessToken != "" {
		c.JSON(http.StatusOK, gin.H{
			"connected":    true,
			"username":     accounts[0].GOGUsername,
			"source":       "oauth",
			"lastSyncedAt": accounts[0].LastSyncedAt,
		})
		return
	}

	// If no user-specific OAuth, check if an instance-wide environment token exists
	if h.envToken != "" {
		c.JSON(http.StatusOK, gin.H{
			"connected":    true,
			"username":     "Instance Token",
			"source":       "env",
			"lastSyncedAt": nil,
		})
		return
	}

	// Otherwise not connected, but application continues to function normally
	c.JSON(http.StatusOK, gin.H{
		"connected":    false,
		"username":     "",
		"source":       "none",
		"lastSyncedAt": nil,
	})
}

// authURL returns the official GOG OAuth authorization URL to initiate login.
func (h *gogHandler) authURL(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"authUrl": gog.AuthURL(),
	})
}

// popupPage serves a dedicated sign-in window that authenticates GOG and posts back to the opener.
func (h *gogHandler) popupPage(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Sign in with GOG - OmniShelf</title>
  <style>
    :root {
      --bg: #21120f;
      --surface: #2e1815;
      --surface-alt: #37201b;
      --border: #4c322b;
      --text: #ffe0b5;
      --muted: #c8a889;
      --accent: #d9b095;
      --danger: #ca2e55;
      --confirm: #bdb246;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 1.5rem;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: var(--bg);
      color: var(--text);
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
    }
    .card {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 2rem;
      width: 100%;
      max-width: 380px;
      box-shadow: 0 12px 30px rgba(0,0,0,0.6);
    }
    .header {
      text-align: center;
      margin-bottom: 1.5rem;
    }
    .header h2 { margin: 0 0 0.35rem; font-size: 1.35rem; }
    .header p { margin: 0; color: var(--muted); font-size: 0.88rem; }
    .form-group { margin-bottom: 1rem; }
    label { display: block; margin-bottom: 0.35rem; font-size: 0.85rem; font-weight: 600; }
    input {
      width: 100%;
      padding: 0.65rem 0.85rem;
      border-radius: 8px;
      border: 1px solid var(--border);
      background: var(--surface-alt);
      color: var(--text);
      font-size: 0.95rem;
    }
    input:focus { outline: none; border-color: var(--accent); }
    button {
      width: 100%;
      padding: 0.75rem;
      border-radius: 8px;
      border: none;
      background: var(--accent);
      color: var(--bg);
      font-weight: 650;
      font-size: 0.95rem;
      cursor: pointer;
      margin-top: 0.5rem;
      transition: opacity 0.15s ease;
    }
    button:hover { opacity: 0.9; }
    button:disabled { opacity: 0.5; cursor: not-allowed; }
    .msg {
      margin-top: 1rem;
      font-size: 0.85rem;
      text-align: center;
      padding: 0.5rem;
      border-radius: 6px;
      display: none;
    }
    .msg.error { display: block; background: rgba(202, 46, 85, 0.2); color: #ff859d; border: 1px solid var(--danger); }
    .msg.success { display: block; background: rgba(189, 178, 70, 0.2); color: #e5db70; border: 1px solid var(--confirm); }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <h2>🎮 Sign in to GOG</h2>
      <p>Connect your DRM-free games library</p>
    </div>
    <form id="gog-form">
      <div class="form-group" id="email-group">
        <label for="email">GOG Email</label>
        <input type="email" id="email" name="username" required placeholder="name@example.com" autofocus autocomplete="username" />
      </div>
      <div class="form-group" id="password-group">
        <label for="password">Password</label>
        <input type="password" id="password" name="password" required placeholder="••••••••••••" autocomplete="current-password" />
      </div>
      <div class="form-group" id="twofactor-group" style="display:none;">
        <label for="twofactor">2-Step Verification Code</label>
        <input type="text" id="twofactor" name="twofactor" placeholder="Enter verification code" autocomplete="one-time-code" />
      </div>
      <button type="submit" id="submit-btn">Sign In</button>
      <div id="msg-box" class="msg"></div>
    </form>
  </div>
  <script>
    const form = document.getElementById('gog-form');
    const submitBtn = document.getElementById('submit-btn');
    const msgBox = document.getElementById('msg-box');
    const twoFactorGroup = document.getElementById('twofactor-group');
    const twoFactorInput = document.getElementById('twofactor');

    let require2FA = false;

    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      msgBox.style.display = 'none';
      submitBtn.disabled = true;
      submitBtn.textContent = 'Connecting...';

      const email = document.getElementById('email').value.trim();
      const password = document.getElementById('password').value;
      const twoFactorCode = twoFactorInput.value.trim();

      try {
        const res = await fetch('/api/gog/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ email, password, twoFactorCode: twoFactorCode || undefined })
        });

        const data = await res.json();

        if (data.twoFactorRequired) {
          require2FA = true;
          twoFactorGroup.style.display = 'block';
          twoFactorInput.required = true;
          twoFactorInput.focus();
          submitBtn.disabled = false;
          submitBtn.textContent = 'Verify & Sign In';
          msgBox.className = 'msg success';
          msgBox.textContent = 'Enter the verification code sent to your email/app';
          msgBox.style.display = 'block';
          return;
        }

        if (!res.ok || data.error) {
          throw new Error(data.message || 'Login failed. Please check your credentials.');
        }

        // Success!
        msgBox.className = 'msg success';
        msgBox.textContent = '✓ Connected! Syncing library...';
        msgBox.style.display = 'block';
        submitBtn.textContent = '✓ Connected';

        if (window.opener) {
          window.opener.postMessage({ type: 'GOG_CONNECTED', username: data.username }, '*');
        }

        setTimeout(() => {
          window.close();
        }, 1000);
      } catch (err) {
        msgBox.className = 'msg error';
        msgBox.textContent = err.message || 'Connection error. Please try again.';
        msgBox.style.display = 'block';
        submitBtn.disabled = false;
        submitBtn.textContent = require2FA ? 'Verify & Sign In' : 'Sign In';
      }
    });
  </script>
</body>
</html>`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

// callback handles OAuth redirect callback when code is returned in query parameters.
func (h *gogHandler) callback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.String(http.StatusBadRequest, "Missing authorization code.")
		return
	}

	userID := CurrentUserID(c)
	ctx := c.Request.Context()

	tokens, err := h.gogClient.ExchangeCode(ctx, code)
	if err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("GOG token exchange failed: %v", err))
		return
	}

	userData, _ := h.gogClient.GetUserData(ctx, tokens.AccessToken)
	username := ""
	if userData != nil && userData.Username != "" {
		username = userData.Username
	} else {
		username = fmt.Sprintf("GOG User #%d", userID)
	}

	var expiresAt *time.Time
	if tokens.ExpiresIn > 0 {
		exp := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
		expiresAt = &exp
	}

	account := models.UserGOGAccount{
		UserID:       userID,
		GOGUsername:  username,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    expiresAt,
		UpdatedAt:    time.Now(),
	}

	_ = h.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"gog_username", "access_token", "refresh_token", "expires_at", "updated_at",
			}),
		}).
		Create(&account).Error

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>GOG Connected</title></head>
<body style="background:#21120f;color:#ffe0b5;display:flex;flex-direction:column;align-items:center;justify-content:center;height:100vh;font-family:sans-serif;">
  <h2>✓ Successfully Connected to GOG!</h2>
  <p>Logged in as %s. Closing window...</p>
  <script>
    if (window.opener) {
      window.opener.postMessage({ type: 'GOG_CONNECTED', username: '%s' }, '*');
    }
    setTimeout(() => window.close(), 1000);
  </script>
</body>
</html>`, username, username)

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

type directLoginPayload struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	TwoFactorCode string `json:"twoFactorCode"`
}

// directLogin logs into GOG directly with email/password and automatically captures tokens.
func (h *gogHandler) directLogin(c *gin.Context) {
	userID := CurrentUserID(c)
	ctx := c.Request.Context()

	var payload directLoginPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid JSON payload")
		return
	}

	if strings.TrimSpace(payload.Email) == "" || strings.TrimSpace(payload.Password) == "" {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "Email and password are required")
		return
	}

	tokens, err := h.gogClient.LoginWithCredentials(ctx, payload.Email, payload.Password, payload.TwoFactorCode)
	if err != nil {
		if errors.Is(err, gog.ErrTwoFactorRequired) {
			c.JSON(http.StatusOK, gin.H{
				"twoFactorRequired": true,
				"message":           "Two-factor authentication required. Please enter the verification code sent to your email or authenticator app.",
			})
			return
		}
		if errors.Is(err, gog.ErrInvalidCredentials) {
			Error(c, http.StatusUnauthorized, CodeBadCredentials, "Invalid GOG email or password.")
			return
		}
		Error(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}

	userData, err := h.gogClient.GetUserData(ctx, tokens.AccessToken)
	username := ""
	if err == nil && userData != nil && userData.Username != "" {
		username = userData.Username
	} else {
		username = payload.Email
	}

	var expiresAt *time.Time
	if tokens.ExpiresIn > 0 {
		exp := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
		expiresAt = &exp
	}

	account := models.UserGOGAccount{
		UserID:       userID,
		GOGUsername:  username,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    expiresAt,
		UpdatedAt:    time.Now(),
	}

	err = h.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"gog_username", "access_token", "refresh_token", "expires_at", "updated_at",
			}),
		}).
		Create(&account).Error

	if err != nil {
		log.Printf("gog: saving account failed for user %d: %v", userID, err)
		Error(c, http.StatusInternalServerError, CodeInternal, "saving GOG account failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connected": true,
		"username":  username,
		"source":    "oauth",
		"message":   fmt.Sprintf("Successfully signed in as %s!", username),
	})
}

type connectPayload struct {
	Code        string `json:"code"`
	AccessToken string `json:"accessToken"`
}

// connect links a GOG account via OAuth authorization code or manual access token.
func (h *gogHandler) connect(c *gin.Context) {
	userID := CurrentUserID(c)
	ctx := c.Request.Context()

	var payload connectPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "invalid JSON payload")
		return
	}

	var accessToken string
	var refreshToken string
	var expiresAt *time.Time

	if payload.Code != "" {
		// Exchange OAuth authorization code
		tokens, err := h.gogClient.ExchangeCode(ctx, payload.Code)
		if err != nil {
			log.Printf("gog: code exchange failed for user %d: %v", userID, err)
			Error(c, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("GOG authorization failed: %v", err))
			return
		}
		accessToken = tokens.AccessToken
		refreshToken = tokens.RefreshToken
		if tokens.ExpiresIn > 0 {
			exp := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
			expiresAt = &exp
		}
	} else if payload.AccessToken != "" {
		// Manual access token provided
		accessToken = strings.TrimSpace(payload.AccessToken)
	} else {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "either code or accessToken is required")
		return
	}

	// Validate token by querying GOG account details
	userData, err := h.gogClient.GetUserData(ctx, accessToken)
	username := ""
	if err == nil && userData != nil && userData.Username != "" {
		username = userData.Username
	} else if username == "" {
		username = fmt.Sprintf("GOG User #%d", userID)
	}

	// Upsert UserGOGAccount record
	account := models.UserGOGAccount{
		UserID:       userID,
		GOGUsername:  username,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		UpdatedAt:    time.Now(),
	}

	err = h.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"gog_username", "access_token", "refresh_token", "expires_at", "updated_at",
			}),
		}).
		Create(&account).Error

	if err != nil {
		log.Printf("gog: saving account failed for user %d: %v", userID, err)
		Error(c, http.StatusInternalServerError, CodeInternal, "saving GOG account failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connected": true,
		"username":  username,
		"source":    "oauth",
		"message":   "GOG account connected successfully",
	})
}

// sync fetches all owned DRM-free games from GOG and imports/merges them into the user's library.
func (h *gogHandler) sync(c *gin.Context) {
	userID := CurrentUserID(c)
	ctx := c.Request.Context()

	var accounts []models.UserGOGAccount
	err := h.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Limit(1).
		Find(&accounts).Error

	var account models.UserGOGAccount
	if err == nil && len(accounts) > 0 {
		account = accounts[0]
	}

	accessToken := ""
	if account.ID != 0 && account.AccessToken != "" {
		// If token expired and refresh token is available, refresh it
		if account.RefreshToken != "" && account.ExpiresAt != nil && time.Now().After(*account.ExpiresAt) {
			tokens, err := h.gogClient.RefreshToken(ctx, account.RefreshToken)
			if err == nil && tokens != nil {
				account.AccessToken = tokens.AccessToken
				if tokens.RefreshToken != "" {
					account.RefreshToken = tokens.RefreshToken
				}
				if tokens.ExpiresIn > 0 {
					exp := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
					account.ExpiresAt = &exp
				}
				account.UpdatedAt = time.Now()
				_ = h.db.WithContext(ctx).Save(&account).Error
			}
		}
		accessToken = account.AccessToken
	}

	if accessToken == "" && h.envToken != "" {
		accessToken = h.envToken
	}

	if accessToken == "" {
		Error(c, http.StatusUnauthorized, CodeUnauthorized, "no GOG account connected and no GOG_ACCESS_TOKEN configured")
		return
	}

	// Fetch owned games from GOG
	ownedGames, err := h.gogClient.GetOwnedGames(ctx, accessToken)
	if err != nil {
		log.Printf("gog: fetching owned games failed for user %d: %v", userID, err)
		if errors.Is(err, gog.ErrNotAuthenticated) {
			Error(c, http.StatusUnauthorized, CodeUnauthorized, "GOG session expired; please reconnect your account")
			return
		}
		Error(c, http.StatusBadGateway, CodeInternal, fmt.Sprintf("GOG API error: %v", err))
		return
	}

	now := time.Now()
	ownershipStore := ownership.NewStore(h.db)
	importedCount := 0

	for _, og := range ownedGames {
		gogIDStr := strconv.Itoa(og.ID)

		// 1. Locate or create the Game record in the database
		var game models.Game
		err := h.db.WithContext(ctx).
			Where("gog_id = ?", gogIDStr).
			First(&game).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Also check if game already exists by exact title match
			err = h.db.WithContext(ctx).
				Where("title = ? COLLATE NOCASE", og.Title).
				First(&game).Error
		}

		var gameTags []string

		if errors.Is(err, gorm.ErrRecordNotFound) {
			game = models.Game{
				Title: og.Title,
				GOGID: gogIDStr,
			}
			if h.gamesSvc != nil {
				gameTags = h.gamesSvc.EnrichGOGGame(ctx, &game, og.CoverURL, og.Tags)
			}
			if game.CoverPath == "" && og.CoverURL != "" {
				game.CoverPath = og.CoverURL
			}
			if createErr := h.db.WithContext(ctx).Create(&game).Error; createErr != nil {
				log.Printf("gog sync: creating game %s failed: %v", og.Title, createErr)
				continue
			}
		} else if err == nil {
			// Backfill GOGID or missing metadata/cover on existing game record
			needsUpdate := false
			if game.GOGID == "" {
				game.GOGID = gogIDStr
				needsUpdate = true
			}
			if (game.CoverPath == "" || game.Description == "") && h.gamesSvc != nil {
				gameTags = h.gamesSvc.EnrichGOGGame(ctx, &game, og.CoverURL, og.Tags)
				if game.CoverPath == "" && og.CoverURL != "" {
					game.CoverPath = og.CoverURL
				}
				needsUpdate = true
			}
			if needsUpdate {
				_ = h.db.WithContext(ctx).Save(&game).Error
			}
		}

		// Persist tags if any were resolved
		if len(gameTags) > 0 && game.ID != 0 {
			_ = tags.NewStore(h.db).Set(ctx, tags.TypeGame, game.ID, gameTags)
		}

		// 2. Locate or create TrackingItem for the user
		var item models.TrackingItem
		trackErr := h.db.WithContext(ctx).
			Where("user_id = ? AND type = ? AND external_id = ?", userID, games.TypeGame, strconv.FormatUint(uint64(game.ID), 10)).
			First(&item).Error

		if errors.Is(trackErr, gorm.ErrRecordNotFound) {
			item = models.TrackingItem{
				UserID:     userID,
				Type:       games.TypeGame,
				ExternalID: strconv.FormatUint(uint64(game.ID), 10),
				Title:      game.Title,
				Status:     games.StatusPlanTo,
				UpdatedAt:  now,
			}
			if createErr := h.db.WithContext(ctx).Create(&item).Error; createErr != nil {
				log.Printf("gog sync: tracking item for game %s failed: %v", game.Title, createErr)
				continue
			}
			importedCount++
		}

		// 3. Ensure GOG ownership format is attached
		formats, _ := ownershipStore.ForItems(ctx, games.TypeGame, []uint{item.ID})
		hasGOG := false
		currentFormats := formats[item.ID]
		for _, f := range currentFormats {
			if f == ownership.FormatGOG {
				hasGOG = true
				break
			}
		}
		if !hasGOG {
			newFormats := append(currentFormats, ownership.FormatGOG)
			_ = ownershipStore.Set(ctx, games.TypeGame, item.ID, newFormats)
		}
	}

	// Update account last synced timestamp
	if account.ID != 0 {
		account.LastSyncedAt = &now
		_ = h.db.WithContext(ctx).Model(&account).Update("last_synced_at", now).Error
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"importedCount": importedCount,
		"totalOwned":    len(ownedGames),
		"lastSyncedAt":  now,
		"message":       fmt.Sprintf("Synced %d DRM-free games from GOG (%d newly added to library)", len(ownedGames), importedCount),
	})
}

// disconnect removes the linked GOG OAuth account for the authenticated user.
func (h *gogHandler) disconnect(c *gin.Context) {
	userID := CurrentUserID(c)

	err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ?", userID).
		Delete(&models.UserGOGAccount{}).Error

	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "disconnecting GOG account failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "GOG account disconnected",
	})
}
