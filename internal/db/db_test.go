package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := Open(t.TempDir())
	require.NoError(t, err, "Open should succeed on a temp dir")
	t.Cleanup(func() {
		// Close the pool so TempDir cleanup can delete the db file on Windows.
		sqlDB, err := gdb.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	return gdb
}

func TestOpenPragmas(t *testing.T) {
	gdb := openTestDB(t)

	var journalMode string
	require.NoError(t, gdb.Raw("PRAGMA journal_mode").Scan(&journalMode).Error)
	assert.Equal(t, "wal", journalMode, "journal_mode must be WAL")

	var busyTimeout int
	require.NoError(t, gdb.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error)
	assert.Equal(t, 5000, busyTimeout, "busy_timeout must be 5000ms")

	var synchronous int
	require.NoError(t, gdb.Raw("PRAGMA synchronous").Scan(&synchronous).Error)
	assert.Equal(t, 1, synchronous, "synchronous must be NORMAL (1)")
}

func TestOpenConnectionPool(t *testing.T) {
	gdb := openTestDB(t)

	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	assert.Equal(t, 10, sqlDB.Stats().MaxOpenConnections, "MaxOpenConns must be 10")
}

func TestMigrateCreatesAllTables(t *testing.T) {
	gdb := openTestDB(t)

	tables := []string{
		"users",
		"invite_codes",
		"tracking_items",
		"shows",
		"episodes",
		"episode_watches",
		"books",
		"import_jobs",
		"sync_logs",
	}
	for _, table := range tables {
		assert.Truef(t, gdb.Migrator().HasTable(table), "table %q must exist after migrate", table)
	}
}

func TestIdxUserMediaRejectsDuplicate(t *testing.T) {
	gdb := openTestDB(t)

	item := models.TrackingItem{UserID: 1, Type: "TV", ExternalID: "1399", Title: "Game of Thrones"}
	require.NoError(t, gdb.Create(&item).Error)

	dup := models.TrackingItem{UserID: 1, Type: "TV", ExternalID: "1399", Title: "Duplicate"}
	assert.Error(t, gdb.Create(&dup).Error, "idx_user_media must reject duplicate (user,type,externalID)")

	// Differing on any component of the index is allowed.
	otherUser := models.TrackingItem{UserID: 2, Type: "TV", ExternalID: "1399", Title: "Same show, other user"}
	assert.NoError(t, gdb.Create(&otherUser).Error)
	otherType := models.TrackingItem{UserID: 1, Type: "BOOK", ExternalID: "1399", Title: "Same ID, other type"}
	assert.NoError(t, gdb.Create(&otherType).Error)
}

func TestIdxShowEpRejectsDuplicate(t *testing.T) {
	gdb := openTestDB(t)

	air := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	ep := models.Episode{ShowID: 1, Season: 1, Number: 1, Title: "Pilot", AirDate: &air}
	require.NoError(t, gdb.Create(&ep).Error)

	dup := models.Episode{ShowID: 1, Season: 1, Number: 1, Title: "Pilot again"}
	assert.Error(t, gdb.Create(&dup).Error, "idx_show_ep must reject duplicate (show,season,number)")

	next := models.Episode{ShowID: 1, Season: 1, Number: 2, Title: "Second"}
	assert.NoError(t, gdb.Create(&next).Error)
}

func TestIdxUserEpRejectsDuplicate(t *testing.T) {
	gdb := openTestDB(t)

	watch := models.EpisodeWatch{UserID: 1, EpisodeID: 1, WatchedAt: time.Now()}
	require.NoError(t, gdb.Create(&watch).Error)

	dup := models.EpisodeWatch{UserID: 1, EpisodeID: 1, WatchedAt: time.Now()}
	assert.Error(t, gdb.Create(&dup).Error, "idx_user_ep must reject duplicate (user,episode)")

	otherUser := models.EpisodeWatch{UserID: 2, EpisodeID: 1, WatchedAt: time.Now()}
	assert.NoError(t, gdb.Create(&otherUser).Error)
}
