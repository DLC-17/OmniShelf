// Package models defines the GORM data models for OmniShelf.
package models

import "time"

// User is an account on the instance.
type User struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"unique;not null"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    time.Time
}

// InviteCode is a single-use registration code.
type InviteCode struct {
	ID        uint   `gorm:"primaryKey"`
	Code      string `gorm:"unique;not null"`
	IsUsed    bool   `gorm:"default:false"`
	CreatedAt time.Time
}

// TrackingItem is the user ↔ media link.
type TrackingItem struct {
	ID         uint   `gorm:"primaryKey"`
	UserID     uint   `gorm:"not null;index:idx_user_media,unique"`
	Type       string `gorm:"type:varchar(10);not null;index:idx_user_media,unique"` // "TV" | "BOOK"
	ExternalID string `gorm:"not null;index:idx_user_media,unique"`                  // TMDB ID | ISBN-13 | barcode
	Title      string `gorm:"not null"`
	Status     string `gorm:"default:'WATCHING'"` // WATCHING, READING, PLAYING, COMPLETED, PLAN_TO, STOPPED
	Progress   int    `gorm:"default:0"`          // page number (books); unused for TV
	Rating     int    `gorm:"default:0"`          // user's 1–5 self-rating; 0 = unrated
	UpdatedAt  time.Time
}

// Show is the shared TMDB metadata cache (one row per show, all users).
type Show struct {
	ID           uint   `gorm:"primaryKey"`
	TMDBID       int    `gorm:"unique;not null"`
	Title        string `gorm:"not null"`
	PosterPath   string // relative path under images dir
	Status       string // TMDB status: Returning Series, Ended, ...
	LastSyncedAt time.Time
}

// Episode is one episode of a Show.
type Episode struct {
	ID      uint `gorm:"primaryKey"`
	ShowID  uint `gorm:"not null;index;uniqueIndex:idx_show_ep"`
	Season  int  `gorm:"not null;uniqueIndex:idx_show_ep"`
	Number  int  `gorm:"not null;uniqueIndex:idx_show_ep"`
	Title   string
	AirDate *time.Time // nil = unannounced
}

// EpisodeWatch is per-user seen state.
type EpisodeWatch struct {
	ID        uint `gorm:"primaryKey"`
	UserID    uint `gorm:"not null;uniqueIndex:idx_user_ep"`
	EpisodeID uint `gorm:"not null;uniqueIndex:idx_user_ep"`
	WatchedAt time.Time
}

// Book is the shared OpenLibrary metadata cache.
type Book struct {
	ID          uint   `gorm:"primaryKey"`
	ISBN13      string `gorm:"unique;not null"`
	Title       string `gorm:"not null"`
	Authors     string // comma-joined
	CoverPath   string
	PageCount   int
	Description string // OpenLibrary work summary; may be empty
}

// Movie is the shared TMDB movie metadata cache (one row per movie, all
// users). Unlike Show it has no seasons or episodes.
type Movie struct {
	ID           uint   `gorm:"primaryKey"`
	TMDBID       int    `gorm:"unique;not null"`
	Title        string `gorm:"not null"`
	PosterPath   string // relative path under images dir
	Overview     string
	ReleaseDate  string // "YYYY-MM-DD" as returned by TMDB
	LastSyncedAt time.Time
}

// Game is the shared IGDB metadata cache (one row per game, all users). The
// IGDB game id is the canonical identity; Barcode is an optional alternate
// lookup that a ScanDex barcode scan fills in and that is empty ("") for games
// added by name search or GOG import.
//
// Uniqueness is enforced by two PARTIAL unique indexes created in db.Open
// (idx_games_igdb_id WHERE igdb_id <> 0, idx_games_barcode WHERE barcode <> '').
// The struct tags stay plain on purpose: a full unique index would fail
// AutoMigrate on pre-IGDB-keying databases, whose rows can share igdb_id = 0.
// See db.migrateGameIdentity for the backfill gap.
type Game struct {
	ID          uint   `gorm:"primaryKey"`
	IGDBID      int    // canonical identity; unique among rows where igdb_id <> 0
	Barcode     string // optional scanned UPC/EAN; "" for name-search / GOG games
	Title       string `gorm:"not null"`
	Platform    string
	CoverPath   string
	Description string // IGDB summary; may be empty
	ReleaseDate string // "YYYY-MM-DD" from IGDB first_release_date; "" when unknown
}

// Tag is a source-derived keyword/genre, shared across every media item that
// carries it (one row per normalized slug). Tags are NEVER user-created: they
// come only from upstream sources (TMDB keywords, IGDB genres/keywords,
// OpenLibrary subjects). Name is the human-readable label; Slug is the
// normalized, unique key used for dedupe and lookup.
type Tag struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"not null"`
	Slug string `gorm:"unique;not null"`
}

// MediaTag links a Tag to one shared metadata cache row (Show/Movie/Game/Book).
// It is media-type-agnostic: MediaType mirrors TrackingItem.Type ("TV",
// "MOVIE", "GAME", "BOOK") and MediaID is the primary key of the corresponding
// cache row. This is the join surface future per-type tag filters (#13) and
// search (#14) query against; the composite (media_type, media_id) index makes
// "tags for this item" cheap, and the unique index guards against dupes.
type MediaTag struct {
	ID        uint   `gorm:"primaryKey"`
	TagID     uint   `gorm:"not null;index;uniqueIndex:idx_media_tag"`
	MediaType string `gorm:"type:varchar(10);not null;uniqueIndex:idx_media_tag;index:idx_media_lookup"`
	MediaID   uint   `gorm:"not null;uniqueIndex:idx_media_tag;index:idx_media_lookup"`
}

// ShowAlias remembers that an imported (normalized) series title resolved to a
// TMDB id, so future imports of the same title skip the TMDB search entirely.
type ShowAlias struct {
	ID        uint   `gorm:"primaryKey"`
	NormTitle string `gorm:"unique;not null"` // normalized imported title
	TMDBID    int    `gorm:"not null;index"`
	CreatedAt time.Time
}

// RejectedRec records a Discover suggestion the user dismissed, so it is not
// suggested again. Keyed by (user, type, external id).
type RejectedRec struct {
	ID         uint   `gorm:"primaryKey"`
	UserID     uint   `gorm:"not null;index:idx_user_rec,unique"`
	Type       string `gorm:"type:varchar(10);not null;index:idx_user_rec,unique"` // "TV" | "BOOK"
	ExternalID string `gorm:"not null;index:idx_user_rec,unique"`
	CreatedAt  time.Time
}

// BookNote is one timestamped journal entry a user attaches to a book they
// track. Notes are per-user (never shared metadata): they are scoped by UserID
// and reference the user's book TrackingItem via ItemID. A book can carry many
// notes; deleting the tracking item leaves its notes orphaned only if untrack
// does not prune them (handlers do). This model is deliberately source-agnostic
// so imported Goodreads reviews (#2) can be inserted as ordinary entries.
type BookNote struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null;index:idx_note_user_item"`
	ItemID    uint   `gorm:"not null;index:idx_note_user_item"` // the book's TrackingItem.ID
	Body      string `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ImportJob tracks a TV Time CSV import.
type ImportJob struct {
	ID         uint   `gorm:"primaryKey"`
	UserID     uint   `gorm:"not null;index"`
	Status     string `gorm:"default:'PENDING'"` // PENDING, RUNNING, DONE, FAILED
	Processed  int
	Total      int
	Unresolved string // JSON array of unmatched titles
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SyncLog records one nightly TMDB sync run.
type SyncLog struct {
	ID        uint `gorm:"primaryKey"`
	RanAt     time.Time
	ShowCount int
	Errors    string // JSON array of per-show failures
}

// All returns every model for AutoMigrate, in dependency order.
func All() []any {
	return []any{
		&User{},
		&InviteCode{},
		&TrackingItem{},
		&Show{},
		&Episode{},
		&EpisodeWatch{},
		&Book{},
		&Game{},
		&Movie{},
		&ImportJob{},
		&SyncLog{},
		&RejectedRec{},
		&ShowAlias{},
		&Tag{},
		&MediaTag{},
		&BookNote{},
	}
}
