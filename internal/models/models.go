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
	Status     string `gorm:"default:'WATCHING'"` // WATCHING, READING, PLAYING, LISTENING, COMPLETED, PLAN_TO, STOPPED
	Progress   int    `gorm:"default:0"`          // page number (books); unused for TV
	Rating     int    `gorm:"default:0"`          // user's 1–5 self-rating; 0 = unrated
	// Ownership is a comma-joined list of physical formats the user owns
	// (music only, e.g. "Vinyl,CD"); empty for media types without ownership.
	Ownership string
	UpdatedAt time.Time
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

// Game is the shared ScanDex/IGDB metadata cache (one row per barcode, all
// users). ScanDex supplies title, platform and the IGDB id; cover art is not
// part of its payload, so CoverPath is usually empty.
type Game struct {
	ID          uint   `gorm:"primaryKey"`
	Barcode     string `gorm:"unique;not null"` // scanned UPC/EAN
	Title       string `gorm:"not null"`
	Platform    string
	CoverPath   string
	IGDBID      int
	Description string // IGDB summary; may be empty
}

// Album is the shared Discogs/MusicBrainz metadata cache (one row per album,
// all users). An album is added either from a Discogs barcode scan or a
// MusicBrainz name search, so ExternalID is a source-prefixed key —
// "discogs:<id>" or "mb:<mbid>" — reused verbatim as the MUSIC TrackingItem's
// ExternalID. Albums are grouped by Artist in the library; no separate Artist
// table is needed. Barcode/DiscogsID are set only for scanned albums;
// MusicBrainzID only for search-added ones.
type Album struct {
	ID            uint   `gorm:"primaryKey"`
	ExternalID    string `gorm:"unique;not null"` // "discogs:<id>" | "mb:<mbid>"
	Artist        string `gorm:"not null;index"`
	Title         string `gorm:"not null"`
	Year          int
	CoverPath     string // relative path under images dir; "" = no cover
	Barcode       string // scanned UPC/EAN (Discogs only); may be empty
	DiscogsID     int
	MusicBrainzID string
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
		&Album{},
		&ImportJob{},
		&SyncLog{},
		&RejectedRec{},
		&ShowAlias{},
	}
}
