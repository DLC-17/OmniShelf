// Package db owns the single SQLite connection for OmniShelf.
// WAL pragmas and pool settings are configured here and nowhere else.
package db

import (
	"fmt"
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

	return gdb, nil
}
