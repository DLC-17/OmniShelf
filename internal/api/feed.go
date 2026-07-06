package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Feed pagination bounds (spec §2.7: cursor-paginated).
const (
	feedDefaultLimit = 20
	feedMaxLimit     = 100
)

// Source ranks break ties between entries sharing an identical timestamp so
// the ordering is total and cursor pagination has no duplicates or gaps.
// Episode watches sort before tracking-item events at the same instant.
const (
	feedRankWatch = iota
	feedRankItem
	// feedRankTimeOnly marks a cursor built from a bare RFC3339 timestamp
	// (no tiebreaker): strictly-before semantics on both sources.
	feedRankTimeOnly = -1
)

const feedCursorSep = "|"

type feedHandler struct{ db *gorm.DB }

// RegisterFeedRoutes attaches GET /feed to the JWT-protected /api group.
// The feed is a read-only, cross-user view (hard rule 7) derived entirely
// from existing timestamps — EpisodeWatch.WatchedAt and
// TrackingItem.UpdatedAt — with no separate activity table (spec §2.7).
func RegisterFeedRoutes(grp *gin.RouterGroup, gdb *gorm.DB) {
	h := &feedHandler{db: gdb}
	grp.GET("/feed", h.list)
}

type feedUserDTO struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

type feedMediaDTO struct {
	Type  string `json:"type"` // "TV" | "BOOK"
	Title string `json:"title"`
	ID    string `json:"id"` // TMDB ID (TV) or ISBN-13 (BOOK), as string
}

type feedEntry struct {
	User      feedUserDTO  `json:"user"`
	Action    string       `json:"action"`
	Media     feedMediaDTO `json:"media"`
	Timestamp time.Time    `json:"timestamp"`

	// Unexported sort/cursor keys — never serialized.
	rank  int
	rowID uint
}

// cursor renders the entry's position as an opaque before-cursor:
// "<RFC3339Nano>|<watch|item>|<rowID>".
func (e feedEntry) cursor() string {
	kind := "watch"
	if e.rank == feedRankItem {
		kind = "item"
	}
	return e.Timestamp.Format(time.RFC3339Nano) + feedCursorSep + kind +
		feedCursorSep + strconv.FormatUint(uint64(e.rowID), 10)
}

// feedLess orders the feed: newest first, then source rank, then row ID
// descending — a total order, so pages never overlap or skip entries.
func feedLess(a, b feedEntry) bool {
	if !a.Timestamp.Equal(b.Timestamp) {
		return a.Timestamp.After(b.Timestamp)
	}
	if a.rank != b.rank {
		return a.rank < b.rank
	}
	return a.rowID > b.rowID
}

type feedCursor struct {
	ts   time.Time
	rank int // feedRankTimeOnly when the client sent a bare timestamp
	id   uint
}

// parseFeedCursor accepts either a bare RFC3339 timestamp (spec §2.7 —
// entries strictly before it) or the composite cursor this endpoint emits.
func parseFeedCursor(raw string) (*feedCursor, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, feedCursorSep)
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("parsing before timestamp: %w", err)
	}
	if len(parts) == 1 {
		return &feedCursor{ts: ts, rank: feedRankTimeOnly}, nil
	}
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed cursor %q", raw)
	}
	var rank int
	switch parts[1] {
	case "watch":
		rank = feedRankWatch
	case "item":
		rank = feedRankItem
	default:
		return nil, fmt.Errorf("unknown cursor kind %q", parts[1])
	}
	id, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing cursor id: %w", err)
	}
	return &feedCursor{ts: ts, rank: rank, id: uint(id)}, nil
}

// list handles GET /api/feed?limit=&before=.
func (h *feedHandler) list(c *gin.Context) {
	limit := feedDefaultLimit
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			Error(c, http.StatusBadRequest, CodeInvalidRequest, "limit must be a positive integer")
			return
		}
		limit = n
		if limit > feedMaxLimit {
			limit = feedMaxLimit
		}
	}
	cur, err := parseFeedCursor(c.Query("before"))
	if err != nil {
		Error(c, http.StatusBadRequest, CodeInvalidRequest,
			"before must be an RFC3339 timestamp or a cursor returned by this endpoint")
		return
	}

	ctx := c.Request.Context()
	watches, err := h.watchEntries(ctx, cur, limit)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading activity feed")
		return
	}
	items, err := h.itemEntries(ctx, cur, limit)
	if err != nil {
		Error(c, http.StatusInternalServerError, CodeInternal, "loading activity feed")
		return
	}

	// Each source query already returns its own top `limit` entries after
	// the cursor, so the merged top `limit` is globally correct.
	entries := make([]feedEntry, 0, len(watches)+len(items))
	entries = append(entries, watches...)
	entries = append(entries, items...)
	sort.Slice(entries, func(i, j int) bool { return feedLess(entries[i], entries[j]) })
	if len(entries) > limit {
		entries = entries[:limit]
	}

	var next any // null when this is (or may be) the last page
	if len(entries) == limit {
		next = entries[len(entries)-1].cursor()
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries, "nextBefore": next})
}

// feedWatchRow is the join projection for episode-watch activity.
type feedWatchRow struct {
	ID        uint
	WatchedAt time.Time
	UserID    uint
	Username  string
	Season    int
	Number    int
	ShowTitle string
	TMDBID    int `gorm:"column:tmdb_id"`
}

func (h *feedHandler) watchEntries(ctx context.Context, cur *feedCursor, limit int) ([]feedEntry, error) {
	q := h.db.WithContext(ctx).Table("episode_watches").
		Select("episode_watches.id, episode_watches.watched_at, episode_watches.user_id, " +
			"users.username, episodes.season, episodes.number, " +
			"shows.title AS show_title, shows.tmdb_id").
		Joins("JOIN episodes ON episodes.id = episode_watches.episode_id").
		Joins("JOIN shows ON shows.id = episodes.show_id").
		Joins("JOIN users ON users.id = episode_watches.user_id").
		Order("episode_watches.watched_at DESC, episode_watches.id DESC").
		Limit(limit)
	if cur != nil {
		if cur.rank == feedRankWatch {
			// Same-timestamp watches after the cursor row still qualify.
			q = q.Where("episode_watches.watched_at < ? OR (episode_watches.watched_at = ? AND episode_watches.id < ?)",
				cur.ts, cur.ts, cur.id)
		} else {
			// Item or bare-timestamp cursor: watches rank before items at
			// an equal timestamp, so only strictly older watches remain.
			q = q.Where("episode_watches.watched_at < ?", cur.ts)
		}
	}
	var rows []feedWatchRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("querying episode watches: %w", err)
	}
	out := make([]feedEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, feedEntry{
			User:      feedUserDTO{ID: r.UserID, Username: r.Username},
			Action:    fmt.Sprintf("watched S%02dE%02d of %s", r.Season, r.Number, r.ShowTitle),
			Media:     feedMediaDTO{Type: "TV", Title: r.ShowTitle, ID: strconv.Itoa(r.TMDBID)},
			Timestamp: r.WatchedAt,
			rank:      feedRankWatch,
			rowID:     r.ID,
		})
	}
	return out, nil
}

// feedItemRow is the join projection for tracking-item activity.
type feedItemRow struct {
	ID         uint
	UpdatedAt  time.Time
	UserID     uint
	Username   string
	Type       string
	ExternalID string
	Title      string
	Status     string
}

func (h *feedHandler) itemEntries(ctx context.Context, cur *feedCursor, limit int) ([]feedEntry, error) {
	q := h.db.WithContext(ctx).Table("tracking_items").
		Select("tracking_items.id, tracking_items.updated_at, tracking_items.user_id, " +
			"users.username, tracking_items.type, tracking_items.external_id, " +
			"tracking_items.title, tracking_items.status").
		Joins("JOIN users ON users.id = tracking_items.user_id").
		Order("tracking_items.updated_at DESC, tracking_items.id DESC").
		Limit(limit)
	if cur != nil {
		switch cur.rank {
		case feedRankWatch:
			// Items rank after watches at an equal timestamp, so every
			// same-timestamp item still qualifies.
			q = q.Where("tracking_items.updated_at <= ?", cur.ts)
		case feedRankItem:
			q = q.Where("tracking_items.updated_at < ? OR (tracking_items.updated_at = ? AND tracking_items.id < ?)",
				cur.ts, cur.ts, cur.id)
		default: // bare timestamp: strictly before
			q = q.Where("tracking_items.updated_at < ?", cur.ts)
		}
	}
	var rows []feedItemRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("querying tracking items: %w", err)
	}
	out := make([]feedEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, feedEntry{
			User:      feedUserDTO{ID: r.UserID, Username: r.Username},
			Action:    feedItemAction(r.Type, r.Status, r.Title),
			Media:     feedMediaDTO{Type: r.Type, Title: r.Title, ID: r.ExternalID},
			Timestamp: r.UpdatedAt,
			rank:      feedRankItem,
			rowID:     r.ID,
		})
	}
	return out, nil
}

// feedItemAction phrases a tracking-item event from its type and status
// (spec §2.7: items added, statuses changed, books finished).
func feedItemAction(mediaType, status, title string) string {
	switch mediaType {
	case "TV":
		switch status {
		case "WATCHING":
			return fmt.Sprintf("is watching %s", title)
		case "COMPLETED":
			return fmt.Sprintf("finished watching %s", title)
		case "PLAN_TO":
			return fmt.Sprintf("plans to watch %s", title)
		}
	case "BOOK":
		switch status {
		case "READING":
			return fmt.Sprintf("is reading %s", title)
		case "COMPLETED":
			return fmt.Sprintf("finished book %s", title)
		case "PLAN_TO":
			return fmt.Sprintf("plans to read %s", title)
		}
	}
	return fmt.Sprintf("updated %s", title)
}
