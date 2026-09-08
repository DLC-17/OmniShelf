// Package db owns the single SQLite connection for OmniShelf.
// WAL pragmas and pool settings are configured here and nowhere else.
package db

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// Open opens (creating if needed) {dataDir}/omnishelf.db with WAL journaling,
// busy_timeout=5000, synchronous=NORMAL, a 10-open/5-idle connection pool,
// and runs AutoMigrate for all models. It returns the sole *gorm.DB the
// application must use.
func Open(dataDir string) (*gorm.DB, error) {
	dsn := filepath.Join(dataDir, "omnishelf.db") +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)"

	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("access underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)

	if err := gdb.AutoMigrate(models.All()...); err != nil {
		return nil, fmt.Errorf("auto-migrate models: %w", err)
	}

	if err := migrateGameIdentity(gdb); err != nil {
		return nil, fmt.Errorf("migrating game identity: %w", err)
	}

	if err := migrateStatsIndexes(gdb); err != nil {
		return nil, fmt.Errorf("migrating stats indexes: %w", err)
	}

	if err := migrateAdminBootstrap(gdb); err != nil {
		return nil, fmt.Errorf("migrating admin bootstrap: %w", err)
	}

	return gdb, nil
}

// migrateGameIdentity enforces the Game identity model: the IGDB game id is the
// canonical key and the barcode an optional alternate lookup. Both uniques are
// PARTIAL so the change is safe on databases created before IGDB keying:
//
//   - igdb_id is unique only among rows that actually carry one (igdb_id <> 0),
//     so pre-existing rows ScanDex could not map to IGDB (igdb_id = 0) never
//     collide with each other.
//   - barcode is unique only among rows that carry one (barcode <> ''), so games
//     added by name search or GOG import (no barcode) never collide.
//
// BACKFILL GAP: rows written before this migration keep igdb_id = 0 whenever
// ScanDex returned no IGDB id; they are not retro-resolved here. Re-scanning
// such a game records its IGDB id and backfills the row. An index create that
// fails because existing data already violates it (e.g. two rows sharing a real
// igdb_id) is logged and skipped rather than aborting startup, so an upgrade
// never crashes on legacy data.
func migrateGameIdentity(gdb *gorm.DB) error {
	stmts := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_games_igdb_id ON games(igdb_id) WHERE igdb_id <> 0`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_games_barcode ON games(barcode) WHERE barcode <> ''`,
		`DELETE FROM media_relations WHERE source_type = 'GAME' AND target_type = 'GAME'`,
	}
	for _, s := range stmts {
		if err := gdb.Exec(s).Error; err != nil {
			log.Printf("db: skipping game identity migration: %v", err)
		}
	}
	return nil
}

// migrateStatsIndexes ensures indexes supporting stats and annual wrapped aggregations exist.
func migrateStatsIndexes(gdb *gorm.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_episode_watches_user_watched ON episode_watches(user_id, watched_at)`,
		`CREATE INDEX IF NOT EXISTS idx_tracking_items_user_type_status_updated ON tracking_items(user_id, type, status, updated_at)`,
	}
	for _, s := range stmts {
		if err := gdb.Exec(s).Error; err != nil {
			log.Printf("db: skipping stats index migration: %v", err)
		}
	}
	return nil
}

// migrateAdminBootstrap ensures at least one administrator exists. If no user has
// is_admin = true (e.g. after schema migration on an existing database), the lowest-ID
// user is promoted. Safe and idempotent on every startup.
func migrateAdminBootstrap(gdb *gorm.DB) error {
	var adminCount int64
	if err := gdb.Model(&models.User{}).Where("is_admin = ?", true).Count(&adminCount).Error; err != nil {
		return fmt.Errorf("counting admins: %w", err)
	}
	if adminCount > 0 {
		return nil
	}

	result := gdb.Model(&models.User{}).
		Where("id = (SELECT MIN(id) FROM users)").
		Update("is_admin", true)
	if result.Error != nil {
		return fmt.Errorf("promoting first user to admin: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		log.Print("db: promoted lowest-ID user to administrator (no admin existed)")
	}
	return nil
}

