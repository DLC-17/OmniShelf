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
	ExternalID string `gorm:"not null;index:idx_user_media,unique"`                  // TMDB ID | ISBN-13
	Title      string `gorm:"not null"`
	Status     string `gorm:"default:'WATCHING'"` // WATCHING, READING, COMPLETED, PLAN_TO
	Progress   int    `gorm:"default:0"`          // page number (books); unused for TV
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
	ID        uint   `gorm:"primaryKey"`
	ISBN13    string `gorm:"unique;not null"`
	Title     string `gorm:"not null"`
	Authors   string // comma-joined
	CoverPath string
	PageCount int
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
		&ImportJob{},
		&SyncLog{},
	}
}
