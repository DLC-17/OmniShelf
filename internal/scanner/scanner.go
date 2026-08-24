package scanner

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/models"
)

type Scanner struct {
	db       *gorm.DB
	scanDirs []string
}

func New(db *gorm.DB, cfg *config.Config) *Scanner {
	return &Scanner{
		db:       db,
		scanDirs: cfg.ScanDirs,
	}
}

func (s *Scanner) Schedule(c *cron.Cron) error {
	if len(s.scanDirs) == 0 {
		return nil
	}
	_, err := c.AddFunc("0 3 * * *", func() {
		s.Run(context.Background())
	})
	return err
}

func (s *Scanner) Run(ctx context.Context) {
	log.Println("scanner: starting NAS file scan...")
	for _, dir := range s.scanDirs {
		s.scanDirectory(ctx, dir)
	}
	log.Println("scanner: file scan complete")
}

func (s *Scanner) scanDirectory(ctx context.Context, root string) {
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("scanner: skip %s: %v", path, err)
			return nil // skip
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".epub", ".mobi", ".pdf":
			s.matchBook(ctx, path, info)
		case ".mp4", ".mkv", ".avi":
			s.matchVideo(ctx, path, info)
		}
		return nil
	})

	if err != nil {
		log.Printf("scanner: directory %s error: %v", root, err)
	}
}

func (s *Scanner) matchBook(ctx context.Context, path string, info os.FileInfo) {
	filename := info.Name()
	title := strings.TrimSuffix(filename, filepath.Ext(filename))
	// Basic heuristic: match against Book.Title
	var book models.Book
	if err := s.db.WithContext(ctx).Where("title LIKE ?", "%"+title+"%").First(&book).Error; err == nil {
		s.upsertMapping(ctx, "BOOK", book.ID, path, info.Size())
	}
}

func (s *Scanner) matchVideo(ctx context.Context, path string, info os.FileInfo) {
	filename := info.Name()
	title := strings.TrimSuffix(filename, filepath.Ext(filename))
	// Basic heuristic: match against Movie.Title or Show.Title
	var movie models.Movie
	if err := s.db.WithContext(ctx).Where("title LIKE ?", "%"+title+"%").First(&movie).Error; err == nil {
		s.upsertMapping(ctx, "MOVIE", movie.ID, path, info.Size())
		return
	}
	var show models.Show
	if err := s.db.WithContext(ctx).Where("title LIKE ?", "%"+title+"%").First(&show).Error; err == nil {
		s.upsertMapping(ctx, "TV", show.ID, path, info.Size())
	}
}

func (s *Scanner) upsertMapping(ctx context.Context, mediaType string, mediaID uint, path string, size int64) {
	mapping := models.LocalFileMapping{
		MediaType: mediaType,
		MediaID:   mediaID,
		FilePath:  path,
		FileSize:  size,
		ScannedAt: time.Now(),
	}
	// Use an upsert so rescanning the same file updates the existing row
	// instead of creating duplicates. The unique index is
	// (media_type, media_id, file_path).
	if err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "media_type"}, {Name: "media_id"}, {Name: "file_path"}},
			DoUpdates: clause.AssignmentColumns([]string{"file_size", "scanned_at"}),
		}).
		Create(&mapping).Error; err != nil {
		log.Printf("scanner: upsert mapping %s/%d %s: %v", mediaType, mediaID, path, err)
	}
}
