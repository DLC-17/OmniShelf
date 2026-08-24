// Package models defines the GORM data models for OmniShelf.
package models

import "time"

// User is an account on the instance.
type User struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"unique;not null"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    time.Time
	Theme        string   `gorm:"type:text;default:'dark-espresso'" json:"theme"`
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
	Location   string `gorm:"default:''"`         // physical shelf location tag: 'Shelf A', 'Binder #1', etc.
	UpdatedAt  time.Time
}

// Show is the shared TMDB metadata cache (one row per show, all users).
type Show struct {
	ID           uint   `gorm:"primaryKey"`
	TMDBID       int    `gorm:"unique;not null"`
	Title        string `gorm:"not null"`
	PosterPath   string // relative path under images dir
	Overview     string // TMDB synopsis; may be empty (backfilled by metadata refresh)
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
	Runtime int        // episode duration in minutes; 0 when unknown
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
	ID            uint   `gorm:"primaryKey"`
	ISBN13        string `gorm:"unique;not null"`
	Title         string `gorm:"not null"`
	Authors       string // comma-joined
	CoverPath     string
	PageCount     int
	Description   string // OpenLibrary work summary; may be empty
	PublishedDate string // "YYYY-MM-DD" or "YYYY" from OpenLibrary; "" when unknown
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
	Runtime      int    // duration in minutes; 0 when unknown
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
	GOGID       string // GOG slug/ID for DRM-free library sync; "" when not from GOG
	Barcode     string // optional scanned UPC/EAN; "" for name-search / GOG games
	Title       string `gorm:"not null"`
	Platform    string
	CoverPath   string
	Description string // IGDB summary; may be empty
	ReleaseDate string // "YYYY-MM-DD" from IGDB first_release_date; "" when unknown
	Developer   string // primary developer studio; "" when unknown
	Publisher   string // primary publisher; "" when unknown
	Runtime     int    // completion time in minutes; 0 when unknown
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

// OwnershipFormat records that a tracked item is owned in a particular format,
// e.g. a game owned Physical and/or on GOG. It is a GENERIC, media-type-agnostic
// store (like MediaTag): MediaType mirrors TrackingItem.Type ("GAME", and later
// "MUSIC") and ItemID is the TrackingItem primary key — ownership is a per-user
// fact about a tracked copy, NOT shared metadata, so it keys off the tracking
// item, not the shared cache row.
//
// Ownership is a multi-select over a FIXED option set per media type (games:
// Physical, GOG), so one item may have several rows. The composite unique index
// (media_type, item_id, format) makes each (item, format) pair appear at most
// once; the (media_type, item_id) lookup index makes "formats for this item"
// cheap for the library DTO batch read. Allowed values are validated in the
// ownership service, not the schema.
type OwnershipFormat struct {
	ID        uint   `gorm:"primaryKey"`
	MediaType string `gorm:"type:varchar(10);not null;uniqueIndex:idx_ownership_format;index:idx_ownership_lookup"`
	ItemID    uint   `gorm:"not null;uniqueIndex:idx_ownership_format;index:idx_ownership_lookup"`
	Format    string `gorm:"not null;uniqueIndex:idx_ownership_format"`
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
	ReleaseDate   string // "YYYY-MM-DD" from Discogs/MusicBrainz; "" when unknown
}

// Card is the shared trading-card metadata cache (one row per card printing,
// all users), fed by the photo-scan flow in internal/cards. ExternalID is a
// source-prefixed key — "ygo:<SetCode>" for Yu-Gi-Oh! (YGOPRODeck, keyed by
// the printed set code) or "ptcg:<card id>" for Pokémon (api.pokemontcg.io) —
// reused verbatim as the CARD TrackingItem's ExternalID.
type Card struct {
	ID         uint   `gorm:"primaryKey"`
	ExternalID string `gorm:"unique;not null"` // "ygo:<SetCode>" | "ptcg:<card id>"
	Game       string `gorm:"not null"`        // "YUGIOH" | "POKEMON"
	Name       string `gorm:"not null"`
	CardType   string // e.g. "Normal Monster", "Pokémon — Stage 2"
	Race       string // YGO race/attribute; "" for Pokémon
	Artist     string // illustrator credit (Pokémon); "" when the source has none
	SetCode    string
	SetName    string
	Price      float64 // market/tcgplayer price at scan time
	CoverPath  string  // cached artwork, relative /images path
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

// Universal emotional sentiment tags for diary logs across all media types.
const (
	SentimentMindBlown        = "MIND_BLOWN"
	SentimentCozy             = "COZY"
	SentimentTearJerker       = "TEAR_JERKER"
	SentimentInstantClassic   = "INSTANT_CLASSIC"
	SentimentThoughtProvoking = "THOUGHT_PROVOKING"
)

// UniversalSentiments lists the standard sentiment tags.
var UniversalSentiments = []string{
	SentimentMindBlown,
	SentimentCozy,
	SentimentTearJerker,
	SentimentInstantClassic,
	SentimentThoughtProvoking,
}

// Note is a media-type-agnostic journal entry or emotional reaction a user
// attaches to any tracked item. It supersedes the book-only BookNote model
// (which is preserved for backward compatibility and existing data). Notes are
// per-user and scoped by UserID + ItemID (the TrackingItem). MediaType mirrors
// TrackingItem.Type so queries can scope to a single media without joining.
// The optional Sentiment field captures an immediate emotional reaction
// (e.g. "MIND_BLOWN", "COZY", "TEAR_JERKER", "INSTANT_CLASSIC", "THOUGHT_PROVOKING")
// logged after marking media complete or during a re-watch / re-read.
type Note struct {
	ID          uint       `gorm:"primaryKey"`
	UserID      uint       `gorm:"not null;index:idx_note_user_item_type"`
	ItemID      uint       `gorm:"not null;index:idx_note_user_item_type"` // TrackingItem.ID
	MediaType   string     `gorm:"type:varchar(10);not null;index:idx_note_user_item_type"`
	Body        string     `gorm:"not null"`
	Sentiment   string     // optional emotional tag: "MIND_BLOWN", "COZY", "TEAR_JERKER", etc.
	IsRewatch   bool       `gorm:"default:false"` // re-watch, re-read, re-play, or re-listen completion log
	CompletedAt *time.Time // timestamp when the completion / re-watch occurred
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UserCollection is a user-created cross-media collection (e.g. "Cozy Sci-Fi",
// "Halloween Watchlist"). Unlike source-derived Tags, collections are entirely
// user-owned and can span any media type. A collection can optionally be shared
// externally via a ShareToken.
type UserCollection struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null;index:idx_collection_user"`
	Name      string `gorm:"not null"`
	Slug      string `gorm:"not null;uniqueIndex:idx_collection_user_slug"` // URL-safe identifier
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CollectionItem links a TrackingItem to a UserCollection. One item can belong
// to many collections; one collection can hold many items across media types.
type CollectionItem struct {
	ID           uint `gorm:"primaryKey"`
	CollectionID uint `gorm:"not null;uniqueIndex:idx_coll_item;index"`
	ItemID       uint `gorm:"not null;uniqueIndex:idx_coll_item"` // TrackingItem.ID
	AddedAt      time.Time
}

// CardPriceHistory records periodic market price snapshots for a trading card,
// enabling a historical price delta line graph on card detail pages. Snapshots
// are taken by a scheduled job that re-queries pricing APIs.
type CardPriceHistory struct {
	ID         uint    `gorm:"primaryKey"`
	CardID     uint    `gorm:"not null;index:idx_card_price_date"`
	Price      float64 `gorm:"not null"`
	SnapshotAt time.Time `gorm:"not null;index:idx_card_price_date"`
}

// MediaRelation links two media cache rows as related entries within the same
// franchise/series or across adaptations (e.g. a movie adaptation linked to its
// source book, or games within the same series). Both sides reference shared
// cache rows by type + primary key. RelationType describes the nature of the
// link ("sequel", "prequel", "adaptation", "same_series", "similar").
type MediaRelation struct {
	ID            uint   `gorm:"primaryKey"`
	SourceType    string `gorm:"type:varchar(10);not null;uniqueIndex:idx_relation"`
	SourceID      uint   `gorm:"not null;uniqueIndex:idx_relation"`
	TargetType    string `gorm:"type:varchar(10);not null;uniqueIndex:idx_relation"`
	TargetID      uint   `gorm:"not null;uniqueIndex:idx_relation"`
	RelationType  string `gorm:"not null"` // "sequel", "prequel", "adaptation", "same_series", "similar"
}

// Badge is a permanent achievement awarded for franchise completion or
// milestones (e.g. completing all games in a series). Badges are defined by
// the system, not user-created.
type Badge struct {
	ID          uint   `gorm:"primaryKey"`
	Slug        string `gorm:"unique;not null"` // e.g. "zelda-completionist"
	Name        string `gorm:"not null"`        // e.g. "Zelda Completionist"
	Description string
	IconPath    string // relative path under images dir; "" = default icon
}

// UserBadge records that a user earned a Badge, with the timestamp of when
// the badge criteria were met.
type UserBadge struct {
	ID       uint `gorm:"primaryKey"`
	UserID   uint `gorm:"not null;uniqueIndex:idx_user_badge"`
	BadgeID  uint `gorm:"not null;uniqueIndex:idx_user_badge"`
	EarnedAt time.Time
}

// ShareToken is a crypto-random token that grants read-only public access to a
// specific UserCollection without authentication. Tokens can be revoked by
// deletion. The token is long enough (32 bytes, hex-encoded = 64 chars) to
// resist enumeration.
type ShareToken struct {
	ID           uint   `gorm:"primaryKey"`
	UserID       uint   `gorm:"not null;index"`
	CollectionID uint   `gorm:"not null;index"`
	Token        string `gorm:"unique;not null"` // 64-char hex string
	CreatedAt    time.Time
}


// LocalFileMapping links a file detected on the local NAS filesystem to a
// shared metadata cache row. The scanner periodically traverses configured
// directories and matches filenames/paths to known titles. This lets users
// see which media they physically hold on their drives.
type LocalFileMapping struct {
	ID        uint   `gorm:"primaryKey"`
	MediaType string `gorm:"type:varchar(10);not null;uniqueIndex:idx_local_file"`
	MediaID   uint   `gorm:"not null;uniqueIndex:idx_local_file"`
	FilePath  string `gorm:"not null;uniqueIndex:idx_local_file"` // absolute path on the NAS
	FileSize  int64  // bytes; 0 = unknown
	ScannedAt time.Time
}

// UserGOGAccount stores the authenticated GOG OAuth2 session and sync state for a user.
type UserGOGAccount struct {
	ID           uint       `gorm:"primaryKey"`
	UserID       uint       `gorm:"uniqueIndex;not null"`
	GOGUsername  string
	AccessToken  string     `gorm:"not null"`
	RefreshToken string
	ExpiresAt    *time.Time
	LastSyncedAt *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
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
		&BookNote{},
		&Note{},
		&Game{},
		&Movie{},
		&Album{},
		&Card{},
		&CardPriceHistory{},
		&ImportJob{},
		&SyncLog{},
		&RejectedRec{},
		&ShowAlias{},
		&Tag{},
		&MediaTag{},
		&OwnershipFormat{},
		&UserCollection{},
		&CollectionItem{},
		&MediaRelation{},
		&Badge{},
		&UserBadge{},
		&ShareToken{},
		&LocalFileMapping{},
		&UserGOGAccount{},
	}
}
