package related

import (
	"context"
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// SyncTMDBMovieRelations fetches collection parts and recommendations for a movie
// and populates models.MediaRelation rows.
func (s *Service) SyncTMDBMovieRelations(ctx context.Context, client *tmdb.Client, movieID uint, tmdbID int) error {
	remote, err := client.GetMovie(ctx, tmdbID)
	if err != nil {
		return fmt.Errorf("fetch movie %d: %w", tmdbID, err)
	}

	if remote.BelongsToCollection != nil && remote.BelongsToCollection.ID > 0 {
		col, colErr := client.GetCollection(ctx, remote.BelongsToCollection.ID)
		if colErr == nil && col != nil {
			for _, part := range col.Parts {
				if part.ID == tmdbID {
					continue
				}

				// Find or create local movie row for related part
				var partRow models.Movie
				err := s.db.WithContext(ctx).Where("tmdb_id = ?", part.ID).First(&partRow).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					partRow = models.Movie{
						TMDBID:      part.ID,
						Title:       part.Title,
						PosterPath:  part.PosterPath,
						Overview:    part.Overview,
						ReleaseDate: part.ReleaseDate,
					}
					if cErr := s.db.WithContext(ctx).Create(&partRow).Error; cErr != nil {
						continue
					}
				}

				relType := "same_series"
				if part.ReleaseDate != "" && remote.ReleaseDate != "" {
					if part.ReleaseDate > remote.ReleaseDate {
						relType = "sequel"
					} else {
						relType = "prequel"
					}
				}

				rel := models.MediaRelation{
					SourceType:   "MOVIE",
					SourceID:     movieID,
					TargetType:   "MOVIE",
					TargetID:     partRow.ID,
					RelationType: relType,
				}
				_ = s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rel).Error
			}
		}
	}
	return nil
}

// SyncIGDBGameRelations links games within the same franchise or collection.
func (s *Service) SyncIGDBGameRelations(ctx context.Context, client *igdb.Client, gameID uint, igdbID int) error {
	if client == nil || !client.Configured() {
		return nil
	}

	remote, err := client.GetGame(ctx, igdbID)
	if err != nil || remote == nil {
		return err
	}

	if len(remote.CollectionGameIDs) > 0 {
		var siblingGames []models.Game
		if err := s.db.WithContext(ctx).Where("igdb_id IN ? AND id <> ?", remote.CollectionGameIDs, gameID).Find(&siblingGames).Error; err == nil {
			for _, sib := range siblingGames {
				relType := "same_series"
				if sib.ReleaseDate != "" && remote.ReleaseDate != "" {
					if sib.ReleaseDate > remote.ReleaseDate {
						relType = "sequel"
					} else {
						relType = "prequel"
					}
				}
				rel1 := models.MediaRelation{
					SourceType:   "GAME",
					SourceID:     gameID,
					TargetType:   "GAME",
					TargetID:     sib.ID,
					RelationType: relType,
				}
				_ = s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rel1).Error

				invRel := relType
				if relType == "sequel" {
					invRel = "prequel"
				} else if relType == "prequel" {
					invRel = "sequel"
				}
				rel2 := models.MediaRelation{
					SourceType:   "GAME",
					SourceID:     sib.ID,
					TargetType:   "GAME",
					TargetID:     gameID,
					RelationType: invRel,
				}
				_ = s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rel2).Error
			}
		}
	}

	if len(remote.RemakeIDs) > 0 {
		var remakes []models.Game
		if err := s.db.WithContext(ctx).Where("igdb_id IN ? AND id <> ?", remote.RemakeIDs, gameID).Find(&remakes).Error; err == nil {
			for _, rk := range remakes {
				rel := models.MediaRelation{
					SourceType:   "GAME",
					SourceID:     gameID,
					TargetType:   "GAME",
					TargetID:     rk.ID,
					RelationType: "adaptation",
				}
				_ = s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rel).Error
			}
		}
	}
	return nil
}

// SweepAndSyncAllRelations queries all tracked movies and games to auto-populate franchise relations.
func (s *Service) SweepAndSyncAllRelations(ctx context.Context, tmdbClient *tmdb.Client, igdbClient *igdb.Client) error {
	// Clean legacy game relations so only verified exact collections remain
	_ = s.db.WithContext(ctx).Where("source_type = ? AND target_type = ?", "GAME", "GAME").Delete(&models.MediaRelation{}).Error

	var movies []models.Movie
	if err := s.db.WithContext(ctx).Find(&movies).Error; err == nil {
		for _, m := range movies {
			if tmdbClient != nil && m.TMDBID > 0 {
				if err := s.SyncTMDBMovieRelations(ctx, tmdbClient, m.ID, m.TMDBID); err != nil {
					log.Printf("sync relations: movie %d: %v", m.TMDBID, err)
				}
			}
		}
	}

	var games []models.Game
	if err := s.db.WithContext(ctx).Find(&games).Error; err == nil {
		for _, g := range games {
			if igdbClient != nil && g.IGDBID > 0 {
				if err := s.SyncIGDBGameRelations(ctx, igdbClient, g.ID, g.IGDBID); err != nil {
					log.Printf("sync relations: game %d: %v", g.IGDBID, err)
				}
			}
		}
	}
	return nil
}
