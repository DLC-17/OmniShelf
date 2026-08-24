package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/movies"
	"github.com/davidlc1229/omnishelf/internal/tv"
)

// RegisterWebhookRoutes attaches the media server webhook endpoint to the router.
func RegisterWebhookRoutes(r *gin.Engine, gdb *gorm.DB, tvSvc *tv.Service, movieSvc *movies.Service, cfg *config.Config) {
	h := &webhookHandler{
		db:       gdb,
		tvSvc:    tvSvc,
		movieSvc: movieSvc,
		secret:   []byte(cfg.JWTSecret),
	}
	r.POST("/api/webhooks/mediaserver", h.handleMediaServer)
}

type webhookHandler struct {
	db       *gorm.DB
	tvSvc    *tv.Service
	movieSvc *movies.Service
	secret   []byte
}

// rawWebhookPayload unifies the possible JSON fields sent by Jellyfin, Emby, Plex, Tautulli, etc.
type rawWebhookPayload struct {
	Event            string `json:"event"`
	NotificationType string `json:"NotificationType"`
	EventUpper       string `json:"Event"`
	Action           string `json:"action"`

	PlayedToCompletion *bool `json:"PlayedToCompletion"`
	Played             *bool `json:"Played"`

	ItemType          string `json:"ItemType"`
	Type              string `json:"Type"`
	MediaType         string `json:"media_type"`
	Name              string `json:"Name"`
	Title             string `json:"title"`
	SeriesName        string `json:"SeriesName"`
	SeasonNumber      *int   `json:"SeasonNumber"`
	EpisodeNumber     *int   `json:"EpisodeNumber"`
	ParentIndexNumber *int   `json:"ParentIndexNumber"`
	IndexNumber       *int   `json:"IndexNumber"`
	ProviderTmdb      string `json:"Provider_tmdb"`
	SeriesTmdb        string `json:"Series_tmdb"`
	TmdbID            any    `json:"TmdbId"`
	SeriesTmdbID      any    `json:"SeriesTmdbId"`

	NotificationUsername string `json:"NotificationUsername"`
	UserId               string `json:"UserId"`
	Username             string `json:"username"`
	User                 *struct {
		Id   string `json:"Id"`
		Name string `json:"Name"`
	} `json:"User"`
	Account *struct {
		Id    any    `json:"id"`
		Title string `json:"title"`
		Name  string `json:"name"`
	} `json:"Account"`

	Item *struct {
		Type               string            `json:"Type"`
		Name               string            `json:"Name"`
		SeriesName         string            `json:"SeriesName"`
		SeasonNumber       *int              `json:"SeasonNumber"`
		EpisodeNumber      *int              `json:"EpisodeNumber"`
		IndexNumber        *int              `json:"IndexNumber"`
		ParentIndexNumber  *int              `json:"ParentIndexNumber"`
		ProviderIds        map[string]string `json:"ProviderIds"`
		SeriesProviderIds  map[string]string `json:"SeriesProviderIds"`
		PlayedToCompletion *bool             `json:"PlayedToCompletion"`
		Played             *bool             `json:"Played"`
	} `json:"Item"`

	Metadata *struct {
		LibrarySectionType string `json:"librarySectionType"`
		Type               string `json:"type"`
		Title              string `json:"title"`
		GrandparentTitle   string `json:"grandparentTitle"`
		ParentTitle        string `json:"parentTitle"`
		Index              *int   `json:"index"`
		ParentIndex        *int   `json:"parentIndex"`
		Year               int    `json:"year"`
		Guid               string `json:"guid"`
		GrandparentGuid    string `json:"grandparentGuid"`
		Guids              []struct {
			ID string `json:"id"`
		} `json:"Guids"`
		GrandparentGuids []struct {
			ID string `json:"id"`
		} `json:"GrandparentGuids"`
		ViewOffset int64 `json:"viewOffset"`
		Duration   int64 `json:"duration"`
	} `json:"Metadata"`

	PlaybackInfo *struct {
		PlayedToCompletion *bool `json:"PlayedToCompletion"`
	} `json:"PlaybackInfo"`
}

var tmdbRegex = regexp.MustCompile(`(?i)(?:themoviedb|tmdb)://(?:movie/|tv/)?([0-9]+)`)
var tvdbRegex = regexp.MustCompile(`(?i)(?:thetvdb|tvdb)://(?:series/)?([0-9]+)`)

// handleMediaServer receives inbound webhooks from Jellyfin, Emby, and Plex.
func (h *webhookHandler) handleMediaServer(c *gin.Context) {
	ctx := c.Request.Context()

	// 1. Parse raw payload: handle multipart (Plex "payload" form field) or direct JSON
	var payloadBytes []byte
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		payloadStr := c.PostForm("payload")
		if payloadStr != "" {
			payloadBytes = []byte(payloadStr)
		}
	}
	if len(payloadBytes) == 0 {
		var err error
		payloadBytes, err = io.ReadAll(io.LimitReader(c.Request.Body, 2<<20)) // 2 MiB limit
		if err != nil {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, "reading request body failed")
			return
		}
	}

	if len(payloadBytes) == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "empty webhook payload")
		return
	}

	var payload rawWebhookPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "malformed JSON payload")
		return
	}

	// 2. Identify the target user
	user, err := h.resolveUser(c, &payload)
	if err != nil {
		Error(c, http.StatusUnauthorized, CodeUnauthorized, err.Error())
		return
	}

	// 3. Check event type
	event := h.normalizeEvent(&payload)
	if h.isIgnoredEvent(event, &payload) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ignored",
			"message": fmt.Sprintf("event %q is not a playback completion event", event),
		})
		return
	}

	// 4. Determine media type: Movie vs TV/Episode
	isMovie, isTV := h.detectMediaType(&payload)

	if isMovie {
		h.processMovie(c, ctx, user.ID, &payload)
		return
	}

	if isTV {
		h.processEpisode(c, ctx, user.ID, &payload)
		return
	}

	// Unknown or unsupported media type
	c.JSON(http.StatusOK, gin.H{
		"status":  "ignored",
		"message": "unrecognized or unsupported media type",
	})
}

// normalizeEvent extracts and normalizes the event name.
func (h *webhookHandler) normalizeEvent(p *rawWebhookPayload) string {
	raw := p.Event
	if raw == "" {
		raw = p.NotificationType
	}
	if raw == "" {
		raw = p.EventUpper
	}
	if raw == "" {
		raw = p.Action
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, ".", "")
	raw = strings.ReplaceAll(raw, "_", "")
	raw = strings.ReplaceAll(raw, "-", "")
	raw = strings.ReplaceAll(raw, " ", "")
	return raw
}

// isIgnoredEvent checks if the event should be skipped without error.
func (h *webhookHandler) isIgnoredEvent(event string, p *rawWebhookPayload) bool {
	// Active playback events to ignore
	switch event {
	case "playbackstart", "playbackprogress", "playbackpause", "playbackunpause",
		"mediaplay", "mediapause", "mediaresume", "mediarate":
		return true
	}

	// If PlayedToCompletion is explicitly set to false and event is not a scrobble or item added
	if p.PlayedToCompletion != nil && !*p.PlayedToCompletion && event != "mediascrobble" && event != "scrobble" && event != "itemadded" {
		return true
	}
	if p.Item != nil && p.Item.PlayedToCompletion != nil && !*p.Item.PlayedToCompletion && event != "mediascrobble" && event != "scrobble" && event != "itemadded" {
		return true
	}
	if p.PlaybackInfo != nil && p.PlaybackInfo.PlayedToCompletion != nil && !*p.PlaybackInfo.PlayedToCompletion && event != "mediascrobble" && event != "scrobble" && event != "itemadded" {
		return true
	}

	return false
}

// resolveUser determines which OmniShelf user this webhook applies to.
func (h *webhookHandler) resolveUser(c *gin.Context, p *rawWebhookPayload) (*models.User, error) {
	ctx := c.Request.Context()

	// 1. Check query parameter `token`, `user_id`, `username`
	token := c.Query("token")
	if token == "" {
		token = c.Query("webhook_token")
	}
	if token == "" {
		authHdr := c.GetHeader("Authorization")
		if strings.HasPrefix(strings.ToLower(authHdr), "bearer ") {
			token = strings.TrimSpace(authHdr[7:])
		}
	}
	if token == "" {
		token = c.GetHeader("X-Omnishelf-Token")
	}
	if token == "" {
		token = c.GetHeader("X-Webhook-Token")
	}
	if token == "" {
		if rawCookie, err := c.Cookie(CookieName); err == nil && rawCookie != "" {
			token = rawCookie
		}
	}

	if token != "" {
		// Attempt parsing as JWT
		if uid, err := parseToken(h.secret, token); err == nil && uid > 0 {
			var user models.User
			if err := h.db.WithContext(ctx).First(&user, uid).Error; err == nil {
				return &user, nil
			}
		}
		// Attempt numeric user ID
		if uid, err := strconv.ParseUint(token, 10, 64); err == nil && uid > 0 {
			var user models.User
			if err := h.db.WithContext(ctx).First(&user, uint(uid)).Error; err == nil {
				return &user, nil
			}
		}
		// Attempt matching username
		var user models.User
		if err := h.db.WithContext(ctx).Where("username = ? COLLATE NOCASE", token).First(&user).Error; err == nil {
			return &user, nil
		}
	}

	// 2. Check query param / header user_id
	if uidStr := c.Query("user_id"); uidStr != "" {
		if uid, err := strconv.ParseUint(uidStr, 10, 64); err == nil && uid > 0 {
			var user models.User
			if err := h.db.WithContext(ctx).First(&user, uint(uid)).Error; err == nil {
				return &user, nil
			}
		}
	}

	// 3. Check query param / header username
	if uname := c.Query("username"); uname != "" {
		var user models.User
		if err := h.db.WithContext(ctx).Where("username = ? COLLATE NOCASE", strings.TrimSpace(uname)).First(&user).Error; err == nil {
			return &user, nil
		}
	}

	// 4. Check payload user identifiers
	var candidates []string
	if p.NotificationUsername != "" {
		candidates = append(candidates, p.NotificationUsername)
	}
	if p.Username != "" {
		candidates = append(candidates, p.Username)
	}
	if p.User != nil && p.User.Name != "" {
		candidates = append(candidates, p.User.Name)
	}
	if p.Account != nil {
		if p.Account.Title != "" {
			candidates = append(candidates, p.Account.Title)
		}
		if p.Account.Name != "" {
			candidates = append(candidates, p.Account.Name)
		}
	}

	for _, cand := range candidates {
		cand = strings.TrimSpace(cand)
		if cand != "" {
			var user models.User
			if err := h.db.WithContext(ctx).Where("username = ? COLLATE NOCASE", cand).First(&user).Error; err == nil {
				return &user, nil
			}
		}
	}

	// 5. If only 1 user exists in the entire database, default to that user (self-hosted single user setup)
	var count int64
	if err := h.db.WithContext(ctx).Model(&models.User{}).Count(&count).Error; err == nil && count == 1 {
		var singleUser models.User
		if err := h.db.WithContext(ctx).First(&singleUser).Error; err == nil {
			return &singleUser, nil
		}
	}

	return nil, errors.New("unable to identify target OmniShelf user; configure ?token= or ?username= in the webhook URL")
}

// detectMediaType classifies the payload into Movie or TV.
func (h *webhookHandler) detectMediaType(p *rawWebhookPayload) (isMovie bool, isTV bool) {
	typ := strings.ToLower(p.ItemType)
	if typ == "" {
		typ = strings.ToLower(p.Type)
	}
	if typ == "" {
		typ = strings.ToLower(p.MediaType)
	}
	if typ == "" && p.Item != nil {
		typ = strings.ToLower(p.Item.Type)
	}
	if typ == "" && p.Metadata != nil {
		typ = strings.ToLower(p.Metadata.Type)
	}

	switch typ {
	case "movie", "film":
		return true, false
	case "episode", "tv", "show", "series", "season":
		return false, true
	}

	// Fallback inferences
	if (p.Metadata != nil && (p.Metadata.GrandparentTitle != "" || p.Metadata.ParentIndex != nil)) ||
		p.SeriesName != "" || (p.Item != nil && p.Item.SeriesName != "") ||
		p.SeasonNumber != nil || p.ParentIndexNumber != nil {
		return false, true
	}

	title := p.Title
	if title == "" {
		title = p.Name
	}
	if title == "" && p.Metadata != nil {
		title = p.Metadata.Title
	}
	if title == "" && p.Item != nil {
		title = p.Item.Name
	}

	if title != "" {
		return true, false
	}

	return false, false
}

// processMovie processes a completed Movie playback.
func (h *webhookHandler) processMovie(c *gin.Context, ctx context.Context, userID uint, p *rawWebhookPayload) {
	tmdbID := h.extractMovieTMDB(p)

	title := p.Title
	if title == "" {
		title = p.Name
	}
	if title == "" && p.Metadata != nil {
		title = p.Metadata.Title
	}
	if title == "" && p.Item != nil {
		title = p.Item.Name
	}

	if tmdbID == 0 && title != "" {
		// Fallback: search TMDB by title
		if searchRes, err := h.movieSvc.Search(ctx, title); err == nil && len(searchRes.Results) > 0 {
			tmdbID = searchRes.Results[0].ID
			title = searchRes.Results[0].Title
		}
	}

	if tmdbID == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "unable to resolve movie TMDB ID from payload")
		return
	}

	// Add movie to library (creates shared Movie cache + user TrackingItem if missing)
	res, err := h.movieSvc.AddMovie(ctx, userID, tmdbID)
	var conflict *movies.ConflictError
	if err != nil && !errors.As(err, &conflict) {
		log.Printf("webhooks: adding movie %d for user %d failed: %v", tmdbID, userID, err)
		Error(c, http.StatusInternalServerError, CodeInternal, "tracking movie failed")
		return
	}

	// Mark the movie as COMPLETED
	extID := strconv.Itoa(tmdbID)
	if err := h.db.WithContext(ctx).Model(&models.TrackingItem{}).
		Where("user_id = ? AND type = ? AND external_id = ?", userID, "MOVIE", extID).
		Updates(map[string]any{
			"status":     "COMPLETED",
			"updated_at": time.Now(),
		}).Error; err != nil {
		log.Printf("webhooks: updating movie %d status failed: %v", tmdbID, err)
		Error(c, http.StatusInternalServerError, CodeInternal, "updating movie status failed")
		return
	}

	movieTitle := title
	if res != nil && res.Movie.Title != "" {
		movieTitle = res.Movie.Title
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"mediaType": "MOVIE",
		"tmdbId":    tmdbID,
		"title":     movieTitle,
	})
}

// processEpisode processes a completed TV show episode playback.
func (h *webhookHandler) processEpisode(c *gin.Context, ctx context.Context, userID uint, p *rawWebhookPayload) {
	showTMDBID := h.extractShowTMDB(p)
	seasonNum, episodeNum := h.extractSeasonAndEpisode(p)

	seriesName := p.SeriesName
	if seriesName == "" && p.Item != nil {
		seriesName = p.Item.SeriesName
	}
	if seriesName == "" && p.Metadata != nil {
		seriesName = p.Metadata.GrandparentTitle
	}

	if showTMDBID == 0 && seriesName != "" {
		// Fallback: search TMDB for TV show by name
		if searchRes, err := h.tvSvc.Search(ctx, seriesName); err == nil && len(searchRes.Results) > 0 {
			showTMDBID = searchRes.Results[0].ID
			seriesName = searchRes.Results[0].Name
		}
	}

	if showTMDBID == 0 {
		Error(c, http.StatusBadRequest, CodeInvalidRequest, "unable to resolve TV show TMDB ID from payload")
		return
	}

	// Ensure the show and its episodes are cached and tracked in OmniShelf
	var show models.Show
	err := h.db.WithContext(ctx).Where("tmdb_id = ?", showTMDBID).First(&show).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		addRes, addErr := h.tvSvc.AddShow(ctx, userID, showTMDBID)
		if addErr != nil {
			var conflict *tv.ConflictError
			if !errors.As(addErr, &conflict) {
				log.Printf("webhooks: adding show %d for user %d failed: %v", showTMDBID, userID, addErr)
				Error(c, http.StatusInternalServerError, CodeInternal, "tracking show failed")
				return
			}
		}
		if addRes != nil {
			show = addRes.Show
		} else {
			_ = h.db.WithContext(ctx).Where("tmdb_id = ?", showTMDBID).First(&show)
		}
	} else if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "database error")
		return
	} else {
		// Show is cached; ensure user tracks it
		_, _ = h.tvSvc.AddShow(ctx, userID, showTMDBID)
	}

	// Locate the specific episode
	var episode models.Episode
	err = h.db.WithContext(ctx).
		Where("show_id = ? AND season = ? AND number = ?", show.ID, seasonNum, episodeNum).
		First(&episode).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Re-fetch show from TMDB in case a new season/episode was announced
		_, _ = h.tvSvc.AddShow(ctx, userID, showTMDBID)
		err = h.db.WithContext(ctx).
			Where("show_id = ? AND season = ? AND number = ?", show.ID, seasonNum, episodeNum).
			First(&episode).Error
	}

	if err != nil {
		log.Printf("webhooks: episode S%02dE%02d not found for show %d: %v", seasonNum, episodeNum, showTMDBID, err)
		Error(c, http.StatusNotFound, CodeNotFound, fmt.Sprintf("episode S%02dE%02d not found for show %s", seasonNum, episodeNum, show.Title))
		return
	}

	// Mark the episode as watched
	nextUp, err := h.tvSvc.MarkWatched(ctx, userID, episode.ID)
	if err != nil {
		log.Printf("webhooks: MarkWatched failed for user %d episode %d: %v", userID, episode.ID, err)
		Error(c, http.StatusInternalServerError, CodeInternal, "marking episode watched failed")
		return
	}

	nextEpNum := 0
	if nextUp != nil {
		nextEpNum = nextUp.Number
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"mediaType":   "TV",
		"showTmdbId":  showTMDBID,
		"showTitle":   show.Title,
		"season":      seasonNum,
		"episode":     episodeNum,
		"nextEpisode": nextEpNum,
	})
}

// extractMovieTMDB extracts TMDB ID for a movie from various payload shapes.
func (h *webhookHandler) extractMovieTMDB(p *rawWebhookPayload) int {
	if id := parseTMDBInt(p.TmdbID); id > 0 {
		return id
	}
	if p.ProviderTmdb != "" {
		if id, err := strconv.Atoi(p.ProviderTmdb); err == nil && id > 0 {
			return id
		}
	}
	if p.Item != nil && len(p.Item.ProviderIds) > 0 {
		for k, v := range p.Item.ProviderIds {
			if strings.EqualFold(k, "tmdb") || strings.EqualFold(k, "themoviedb") {
				if id, err := strconv.Atoi(v); err == nil && id > 0 {
					return id
				}
			}
		}
	}
	if p.Metadata != nil {
		for _, g := range p.Metadata.Guids {
			if matches := tmdbRegex.FindStringSubmatch(g.ID); len(matches) > 1 {
				if id, err := strconv.Atoi(matches[1]); err == nil && id > 0 {
					return id
				}
			}
		}
		if matches := tmdbRegex.FindStringSubmatch(p.Metadata.Guid); len(matches) > 1 {
			if id, err := strconv.Atoi(matches[1]); err == nil && id > 0 {
				return id
			}
		}
	}
	return 0
}

// extractShowTMDB extracts TMDB ID for a TV show from various payload shapes.
func (h *webhookHandler) extractShowTMDB(p *rawWebhookPayload) int {
	if id := parseTMDBInt(p.SeriesTmdbID); id > 0 {
		return id
	}
	if p.SeriesTmdb != "" {
		if id, err := strconv.Atoi(p.SeriesTmdb); err == nil && id > 0 {
			return id
		}
	}
	if p.Item != nil && len(p.Item.SeriesProviderIds) > 0 {
		for k, v := range p.Item.SeriesProviderIds {
			if strings.EqualFold(k, "tmdb") || strings.EqualFold(k, "themoviedb") {
				if id, err := strconv.Atoi(v); err == nil && id > 0 {
					return id
				}
			}
		}
	}
	if p.Metadata != nil {
		for _, g := range p.Metadata.GrandparentGuids {
			if matches := tmdbRegex.FindStringSubmatch(g.ID); len(matches) > 1 {
				if id, err := strconv.Atoi(matches[1]); err == nil && id > 0 {
					return id
				}
			}
		}
		if matches := tmdbRegex.FindStringSubmatch(p.Metadata.GrandparentGuid); len(matches) > 1 {
			if id, err := strconv.Atoi(matches[1]); err == nil && id > 0 {
				return id
			}
		}
		for _, g := range p.Metadata.Guids {
			if matches := tmdbRegex.FindStringSubmatch(g.ID); len(matches) > 1 {
				if id, err := strconv.Atoi(matches[1]); err == nil && id > 0 {
					return id
				}
			}
		}
	}
	// Fallback to top-level ProviderTmdb
	if p.ProviderTmdb != "" {
		if id, err := strconv.Atoi(p.ProviderTmdb); err == nil && id > 0 {
			return id
		}
	}
	return 0
}

// extractSeasonAndEpisode parses season and episode numbers from the payload.
func (h *webhookHandler) extractSeasonAndEpisode(p *rawWebhookPayload) (season int, episode int) {
	season = 1
	episode = 1

	if p.SeasonNumber != nil && *p.SeasonNumber >= 0 {
		season = *p.SeasonNumber
	} else if p.ParentIndexNumber != nil && *p.ParentIndexNumber >= 0 {
		season = *p.ParentIndexNumber
	} else if p.Item != nil && p.Item.ParentIndexNumber != nil && *p.Item.ParentIndexNumber >= 0 {
		season = *p.Item.ParentIndexNumber
	} else if p.Item != nil && p.Item.SeasonNumber != nil && *p.Item.SeasonNumber >= 0 {
		season = *p.Item.SeasonNumber
	} else if p.Metadata != nil && p.Metadata.ParentIndex != nil && *p.Metadata.ParentIndex >= 0 {
		season = *p.Metadata.ParentIndex
	}

	if p.EpisodeNumber != nil && *p.EpisodeNumber >= 0 {
		episode = *p.EpisodeNumber
	} else if p.IndexNumber != nil && *p.IndexNumber >= 0 {
		episode = *p.IndexNumber
	} else if p.Item != nil && p.Item.IndexNumber != nil && *p.Item.IndexNumber >= 0 {
		episode = *p.Item.IndexNumber
	} else if p.Item != nil && p.Item.EpisodeNumber != nil && *p.Item.EpisodeNumber >= 0 {
		episode = *p.Item.EpisodeNumber
	} else if p.Metadata != nil && p.Metadata.Index != nil && *p.Metadata.Index >= 0 {
		episode = *p.Metadata.Index
	}

	return season, episode
}

func parseTMDBInt(v any) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case string:
		id, _ := strconv.Atoi(strings.TrimSpace(val))
		return id
	}
	return 0
}
