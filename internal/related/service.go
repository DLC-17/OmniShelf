package related

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
)

// RelatedItemDTO is the API payload representing one related or universe item.
type RelatedItemDTO struct {
	ID           uint   `json:"id"`
	Type         string `json:"type"` // "MOVIE", "TV", "BOOK", "GAME", "MUSIC"
	ExternalID   string `json:"externalId"`
	Title        string `json:"title"`
	ArtworkPath  string `json:"artworkPath"`
	RelationType string `json:"relationType"` // "sequel", "prequel", "adaptation", "same_series", "similar"
	Franchise    string `json:"franchise,omitempty"`
	Year         int    `json:"year,omitempty"`
	Overview     string `json:"overview,omitempty"`
	IsTracked    bool   `json:"isTracked"`
	UserStatus   string `json:"userStatus,omitempty"`
	UserRating   int    `json:"userRating,omitempty"`
}

// RecommendedItemDTO represents a recommendation that heavily aligns with a source media item.
type RecommendedItemDTO struct {
	ID          uint    `json:"id"`
	Type        string  `json:"type"` // "MOVIE", "TV", "BOOK", "GAME", "MUSIC"
	ExternalID  string  `json:"externalId"`
	Title       string  `json:"title"`
	ArtworkPath string  `json:"artworkPath"`
	Year        int     `json:"year,omitempty"`
	Overview    string  `json:"overview,omitempty"`
	IsTracked   bool    `json:"isTracked"`
	UserStatus  string  `json:"userStatus,omitempty"`
	UserRating  int     `json:"userRating,omitempty"`
	MatchReason string  `json:"matchReason,omitempty"`
	Score       float64 `json:"score,omitempty"`
}

// GraphNode is a node in the interactive franchise universe graph.
type GraphNode struct {
	ID          string `json:"id"` // e.g. "MOVIE:12"
	Type        string `json:"type"`
	Title       string `json:"title"`
	ArtworkPath string `json:"artworkPath"`
	Year        int    `json:"year,omitempty"`
	IsTracked   bool   `json:"isTracked"`
	UserStatus  string `json:"userStatus,omitempty"`
}

// GraphEdge is an edge connecting two nodes in the franchise universe graph.
type GraphEdge struct {
	Source       string `json:"source"` // "MOVIE:12"
	Target       string `json:"target"` // "MOVIE:15"
	RelationType string `json:"relationType"`
}

// FranchiseGraphDTO contains nodes and edges for visualization.
type FranchiseGraphDTO struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Service manages cross-media relations and universe graphs.
type Service struct {
	db *gorm.DB
}

// NewService creates a new Related media service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// GetRelatedForMedia returns relations and recommendations for a given media item.
func (s *Service) GetRelatedForMedia(ctx context.Context, userID uint, mediaType string, externalID string) ([]RelatedItemDTO, error) {
	mediaType = strings.ToUpper(mediaType)
	var sourceID uint

	switch mediaType {
	case "MOVIE":
		var m models.Movie
		if tmdbID, err := strconv.Atoi(externalID); err == nil {
			if err := s.db.WithContext(ctx).Where("tmdb_id = ?", tmdbID).First(&m).Error; err == nil {
				sourceID = m.ID
			}
		}
	case "TV":
		var sh models.Show
		if tmdbID, err := strconv.Atoi(externalID); err == nil {
			if err := s.db.WithContext(ctx).Where("tmdb_id = ?", tmdbID).First(&sh).Error; err == nil {
				sourceID = sh.ID
			}
		}
	case "BOOK":
		var b models.Book
		if err := s.db.WithContext(ctx).Where("isbn13 = ?", externalID).First(&b).Error; err == nil {
			sourceID = b.ID
		}
	case "GAME":
		var g models.Game
		if idNum, err := strconv.Atoi(externalID); err == nil {
			if err := s.db.WithContext(ctx).Where("igdb_id = ?", idNum).First(&g).Error; err == nil {
				sourceID = g.ID
			} else if err := s.db.WithContext(ctx).Where("id = ?", idNum).First(&g).Error; err == nil {
				sourceID = g.ID
			}
		}
		if sourceID == 0 {
			if err := s.db.WithContext(ctx).Where("barcode = ? OR gog_id = ?", externalID, externalID).First(&g).Error; err == nil {
				sourceID = g.ID
			}
		}
	}

	var relations []models.MediaRelation
	if sourceID > 0 {
		s.db.WithContext(ctx).
			Where("(source_type = ? AND source_id = ?) OR (target_type = ? AND target_id = ?)", mediaType, sourceID, mediaType, sourceID).
			Find(&relations)
	}

	// Fetch user's tracked items to overlay ownership status
	var trackedItems []models.TrackingItem
	s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&trackedItems)
	trackedMap := make(map[string]models.TrackingItem)
	for _, it := range trackedItems {
		trackedMap[fmt.Sprintf("%s:%s", it.Type, it.ExternalID)] = it
	}

	results := make([]RelatedItemDTO, 0)
	seen := make(map[string]bool)

	for _, rel := range relations {
		tType := rel.TargetType
		tID := rel.TargetID
		relType := rel.RelationType

		if rel.SourceType == mediaType && rel.SourceID == sourceID {
			tType = rel.TargetType
			tID = rel.TargetID
		} else {
			tType = rel.SourceType
			tID = rel.SourceID
			if relType == "sequel" {
				relType = "prequel"
			} else if relType == "prequel" {
				relType = "sequel"
			}
		}

		key := fmt.Sprintf("%s:%d", tType, tID)
		if seen[key] {
			continue
		}
		seen[key] = true

		dto, ok := s.hydrateItem(ctx, tType, tID, relType, trackedMap)
		if ok {
			results = append(results, dto)
		}
	}

	return results, nil
}

// GetFranchiseGraph returns an interconnected node-edge graph for the media item's franchise.
func (s *Service) GetFranchiseGraph(ctx context.Context, userID uint, mediaType string, externalID string) (*FranchiseGraphDTO, error) {
	related, err := s.GetRelatedForMedia(ctx, userID, mediaType, externalID)
	if err != nil {
		return nil, err
	}

	nodes := make([]GraphNode, 0)
	edges := make([]GraphEdge, 0)
	nodeMap := make(map[string]bool)

	// Add root node
	rootKey := fmt.Sprintf("%s:%s", strings.ToUpper(mediaType), externalID)
	nodes = append(nodes, GraphNode{
		ID:    rootKey,
		Type:  strings.ToUpper(mediaType),
		Title: "Current Item",
	})
	nodeMap[rootKey] = true

	for _, it := range related {
		nodeKey := fmt.Sprintf("%s:%s", it.Type, it.ExternalID)
		if !nodeMap[nodeKey] {
			nodeMap[nodeKey] = true
			nodes = append(nodes, GraphNode{
				ID:          nodeKey,
				Type:        it.Type,
				Title:       it.Title,
				ArtworkPath: it.ArtworkPath,
				Year:        it.Year,
				IsTracked:   it.IsTracked,
				UserStatus:  it.UserStatus,
			})
		}
		edges = append(edges, GraphEdge{
			Source:       rootKey,
			Target:       nodeKey,
			RelationType: it.RelationType,
		})
	}

	return &FranchiseGraphDTO{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// AddRelation allows manual or sync-based linking of two media items.
func (s *Service) AddRelation(ctx context.Context, srcType string, srcID uint, tgtType string, tgtID uint, relType string) error {
	rel := models.MediaRelation{
		SourceType:   strings.ToUpper(srcType),
		SourceID:     srcID,
		TargetType:   strings.ToUpper(tgtType),
		TargetID:     tgtID,
		RelationType: relType,
	}
	return s.db.WithContext(ctx).Create(&rel).Error
}

func (s *Service) hydrateItem(ctx context.Context, mType string, id uint, relType string, tracked map[string]models.TrackingItem) (RelatedItemDTO, bool) {
	dto := RelatedItemDTO{
		ID:           id,
		Type:         mType,
		RelationType: relType,
	}

	switch mType {
	case "MOVIE":
		var m models.Movie
		if err := s.db.WithContext(ctx).First(&m, id).Error; err != nil {
			return dto, false
		}
		dto.Title = m.Title
		dto.ExternalID = strconv.Itoa(m.TMDBID)
		dto.ArtworkPath = m.PosterPath
		dto.Overview = m.Overview
		if len(m.ReleaseDate) >= 4 {
			dto.Year, _ = strconv.Atoi(m.ReleaseDate[:4])
		}
	case "TV":
		var sh models.Show
		if err := s.db.WithContext(ctx).First(&sh, id).Error; err != nil {
			return dto, false
		}
		dto.Title = sh.Title
		dto.ExternalID = strconv.Itoa(sh.TMDBID)
		dto.ArtworkPath = sh.PosterPath
		dto.Overview = sh.Overview
	case "BOOK":
		var b models.Book
		if err := s.db.WithContext(ctx).First(&b, id).Error; err != nil {
			return dto, false
		}
		dto.Title = b.Title
		dto.ExternalID = b.ISBN13
		dto.ArtworkPath = b.CoverPath
		dto.Overview = b.Description
		if len(b.PublishedDate) >= 4 {
			dto.Year, _ = strconv.Atoi(b.PublishedDate[:4])
		}
	case "GAME":
		var g models.Game
		if err := s.db.WithContext(ctx).First(&g, id).Error; err != nil {
			return dto, false
		}
		dto.Title = g.Title
		dto.ExternalID = strconv.Itoa(g.IGDBID)
		dto.ArtworkPath = g.CoverPath
		dto.Overview = g.Description
		if len(g.ReleaseDate) >= 4 {
			dto.Year, _ = strconv.Atoi(g.ReleaseDate[:4])
		}
	case "MUSIC":
		var a models.Album
		if err := s.db.WithContext(ctx).First(&a, id).Error; err != nil {
			return dto, false
		}
		dto.Title = a.Title
		dto.ExternalID = a.ExternalID
		dto.ArtworkPath = a.CoverPath
		dto.Year = a.Year
	}

	trackedKey := fmt.Sprintf("%s:%s", dto.Type, dto.ExternalID)
	if tr, ok := tracked[trackedKey]; ok {
		dto.IsTracked = true
		dto.UserStatus = tr.Status
		dto.UserRating = tr.Rating
	}

	return dto, true
}

func (s *Service) findTagSimilarMedia(ctx context.Context, mType string, mediaID uint, tracked map[string]models.TrackingItem, seen map[string]bool) []RelatedItemDTO {
	out := make([]RelatedItemDTO, 0)
	if mediaID == 0 {
		return out
	}

	var tagIDs []uint
	s.db.WithContext(ctx).Model(&models.MediaTag{}).
		Where("media_type = ? AND media_id = ?", mType, mediaID).
		Pluck("tag_id", &tagIDs)

	if len(tagIDs) == 0 {
		return out
	}

	type matchRow struct {
		MediaType string
		MediaID   uint
		Count     int
	}

	var matches []matchRow
	s.db.WithContext(ctx).Model(&models.MediaTag{}).
		Select("media_type, media_id, count(*) as count").
		Where("tag_id IN ? AND NOT (media_type = ? AND media_id = ?)", tagIDs, mType, mediaID).
		Group("media_type, media_id").
		Order("count DESC").
		Limit(8).
		Scan(&matches)

	for _, m := range matches {
		key := fmt.Sprintf("%s:%d", m.MediaType, m.MediaID)
		if seen[key] {
			continue
		}
		seen[key] = true
		dto, ok := s.hydrateItem(ctx, m.MediaType, m.MediaID, "same_universe", tracked)
		if ok {
			out = append(out, dto)
		}
	}
	return out
}

// GetRecommendationsForMedia returns items heavily aligned with the given media item.
func (s *Service) GetRecommendationsForMedia(
	ctx context.Context,
	userID uint,
	mediaType string,
	externalID string,
	tmdbClient *tmdb.Client,
	igdbClient *igdb.Client,
) ([]RecommendedItemDTO, error) {
	mediaType = strings.ToUpper(mediaType)

	// Fetch user's tracked items to overlay ownership status
	var trackedItems []models.TrackingItem
	s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&trackedItems)
	trackedMap := make(map[string]models.TrackingItem)
	for _, it := range trackedItems {
		trackedMap[fmt.Sprintf("%s:%s", it.Type, it.ExternalID)] = it
	}

	// Fetch user's rejected recommendations
	var rejectedRows []models.RejectedRec
	s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rejectedRows)
	rejectedMap := make(map[string]bool)
	for _, r := range rejectedRows {
		rejectedMap[fmt.Sprintf("%s:%s", r.Type, r.ExternalID)] = true
	}

	// 1. Check if stored recommendations exist for this user and source media
	var storedRecs []models.StoredRecommendation
	s.db.WithContext(ctx).
		Where("user_id = ? AND source_type = ? AND source_id = ?", userID, mediaType, externalID).
		Find(&storedRecs)

	if len(storedRecs) > 0 {
		consumedCount := 0
		for _, sr := range storedRecs {
			key := fmt.Sprintf("%s:%s", sr.TargetType, sr.TargetID)
			if _, isTracked := trackedMap[key]; isTracked {
				consumedCount++
			} else if rejectedMap[key] {
				consumedCount++
			}
		}

		// If fewer than 3 have been consumed (marked tracked or not interested), serve stored recommendations
		if consumedCount < 3 {
			storedResults := make([]RecommendedItemDTO, 0, len(storedRecs))
			for _, sr := range storedRecs {
				key := fmt.Sprintf("%s:%s", sr.TargetType, sr.TargetID)
				if rejectedMap[key] {
					continue // exclude dismissed from display
				}
				dto := RecommendedItemDTO{
					Type:        sr.TargetType,
					ExternalID:  sr.TargetID,
					Title:       sr.Title,
					ArtworkPath: sr.ArtworkPath,
					Year:        sr.Year,
					Overview:    sr.Overview,
					MatchReason: sr.MatchReason,
					Score:       sr.Score,
				}
				artwork := sr.ArtworkPath
				if sr.TargetType == "TV" {
					var local models.Show
					if tmdbID, err := strconv.Atoi(sr.TargetID); err == nil {
						if err := s.db.WithContext(ctx).Where("tmdb_id = ?", tmdbID).First(&local).Error; err == nil {
							dto.ID = local.ID
							if artwork == "" && local.PosterPath != "" {
								artwork = local.PosterPath
							}
						}
						if artwork == "" && tmdbClient != nil && tmdbID > 0 {
							if detail, err := tmdbClient.GetShow(ctx, tmdbID); err == nil && detail != nil && detail.PosterPath != "" {
								artwork = detail.PosterPath
								_ = s.db.WithContext(ctx).Model(&models.StoredRecommendation{}).
									Where("id = ?", sr.ID).Update("artwork_path", artwork).Error
							}
						}
					}
				} else if sr.TargetType == "MOVIE" {
					var local models.Movie
					if tmdbID, err := strconv.Atoi(sr.TargetID); err == nil {
						if err := s.db.WithContext(ctx).Where("tmdb_id = ?", tmdbID).First(&local).Error; err == nil {
							dto.ID = local.ID
							if artwork == "" && local.PosterPath != "" {
								artwork = local.PosterPath
							}
						}
						if artwork == "" && tmdbClient != nil && tmdbID > 0 {
							if detail, err := tmdbClient.GetMovie(ctx, tmdbID); err == nil && detail != nil && detail.PosterPath != "" {
								artwork = detail.PosterPath
								_ = s.db.WithContext(ctx).Model(&models.StoredRecommendation{}).
									Where("id = ?", sr.ID).Update("artwork_path", artwork).Error
							}
						}
					}
				}
				dto.ArtworkPath = artwork
				if tr, ok := trackedMap[key]; ok {
					dto.IsTracked = true
					dto.UserStatus = tr.Status
					dto.UserRating = tr.Rating
				}
				storedResults = append(storedResults, dto)
			}
			if len(storedResults) > 6 {
				storedResults = storedResults[:6]
			}
			return storedResults, nil
		}
	}

	results := make([]RecommendedItemDTO, 0)
	seen := make(map[string]bool)

	switch mediaType {
	case "TV":
		tmdbID, _ := strconv.Atoi(externalID)
		if tmdbID > 0 && tmdbClient != nil {
			type candidate struct {
				id           int
				name         string
				overview     string
				firstAirDate string
				posterPath   string
				inRecs       bool
				inSims       bool
			}
			candidates := make(map[int]*candidate)

			if recs, err := tmdbClient.Recommendations(ctx, tmdbID); err == nil && recs != nil {
				for _, r := range recs.Results {
					if r.ID == tmdbID {
						continue
					}
					candidates[r.ID] = &candidate{
						id:           r.ID,
						name:         r.Name,
						overview:     r.Overview,
						firstAirDate: r.FirstAirDate,
						posterPath:   r.PosterPath,
						inRecs:       true,
					}
				}
			}

			if sims, err := tmdbClient.SimilarTV(ctx, tmdbID); err == nil && sims != nil {
				for _, sItem := range sims.Results {
					if sItem.ID == tmdbID {
						continue
					}
					if c, exists := candidates[sItem.ID]; exists {
						c.inSims = true
					} else {
						candidates[sItem.ID] = &candidate{
							id:           sItem.ID,
							name:         sItem.Name,
							overview:     sItem.Overview,
							firstAirDate: sItem.FirstAirDate,
							posterPath:   sItem.PosterPath,
							inSims:       true,
						}
					}
				}
			}

			for _, c := range candidates {
				key := fmt.Sprintf("TV:%d", c.id)
				if seen[key] || rejectedMap[key] {
					continue
				}
				seen[key] = true

				var year int
				if len(c.firstAirDate) >= 4 {
					year, _ = strconv.Atoi(c.firstAirDate[:4])
				}

				score := 86.0
				reason := "Similar Genre & Style"
				if c.inRecs && c.inSims {
					score = 98.0
					reason = "Top Recommendation • High Alignment"
				} else if c.inRecs {
					score = 94.0
					reason = "Recommended • Thematic Match"
				}

				dto := RecommendedItemDTO{
					Type:        "TV",
					ExternalID:  strconv.Itoa(c.id),
					Title:       c.name,
					ArtworkPath: c.posterPath,
					Year:        year,
					Overview:    c.overview,
					MatchReason: reason,
					Score:       score,
				}

				// Check local show cache
				var local models.Show
				if err := s.db.WithContext(ctx).Where("tmdb_id = ?", c.id).First(&local).Error; err == nil {
					dto.ID = local.ID
					if dto.ArtworkPath == "" && local.PosterPath != "" {
						dto.ArtworkPath = local.PosterPath
					}
				}
				if dto.ArtworkPath == "" && tmdbClient != nil && c.id > 0 {
					if detail, err := tmdbClient.GetShow(ctx, c.id); err == nil && detail != nil {
						dto.ArtworkPath = detail.PosterPath
					}
				}

				if tr, ok := trackedMap[key]; ok {
					dto.IsTracked = true
					dto.UserStatus = tr.Status
					dto.UserRating = tr.Rating
				}

				results = append(results, dto)
			}
		}

		// Also find tag-similar local TV shows if any
		var src models.Show
		if tmdbID, _ := strconv.Atoi(externalID); tmdbID > 0 {
			_ = s.db.WithContext(ctx).Where("tmdb_id = ?", tmdbID).First(&src).Error
		}
		if src.ID > 0 {
			tagItems := s.findTagSimilarMedia(ctx, "TV", src.ID, trackedMap, seen)
			for _, ti := range tagItems {
				results = append(results, RecommendedItemDTO{
					ID:          ti.ID,
					Type:        ti.Type,
					ExternalID:  ti.ExternalID,
					Title:       ti.Title,
					ArtworkPath: ti.ArtworkPath,
					Year:        ti.Year,
					Overview:    ti.Overview,
					IsTracked:   ti.IsTracked,
					UserStatus:  ti.UserStatus,
					UserRating:  ti.UserRating,
					MatchReason: "Shared Universe & Tags",
					Score:       90.0,
				})
			}
		}

	case "MOVIE":
		tmdbID, _ := strconv.Atoi(externalID)
		if tmdbID > 0 && tmdbClient != nil {
			type candidate struct {
				id          int
				title       string
				overview    string
				releaseDate string
				posterPath  string
				inRecs      bool
				inSims      bool
			}
			candidates := make(map[int]*candidate)

			if recs, err := tmdbClient.MovieRecommendations(ctx, tmdbID); err == nil && recs != nil {
				for _, r := range recs.Results {
					if r.ID == tmdbID {
						continue
					}
					candidates[r.ID] = &candidate{
						id:          r.ID,
						title:       r.Title,
						overview:    r.Overview,
						releaseDate: r.ReleaseDate,
						posterPath:  r.PosterPath,
						inRecs:      true,
					}
				}
			}

			if sims, err := tmdbClient.SimilarMovies(ctx, tmdbID); err == nil && sims != nil {
				for _, sItem := range sims.Results {
					if sItem.ID == tmdbID {
						continue
					}
					if c, exists := candidates[sItem.ID]; exists {
						c.inSims = true
					} else {
						candidates[sItem.ID] = &candidate{
							id:          sItem.ID,
							title:       sItem.Title,
							overview:    sItem.Overview,
							releaseDate: sItem.ReleaseDate,
							posterPath:  sItem.PosterPath,
							inSims:      true,
						}
					}
				}
			}

			for _, c := range candidates {
				key := fmt.Sprintf("MOVIE:%d", c.id)
				if seen[key] || rejectedMap[key] {
					continue
				}
				seen[key] = true

				var year int
				if len(c.releaseDate) >= 4 {
					year, _ = strconv.Atoi(c.releaseDate[:4])
				}

				score := 86.0
				reason := "Similar Style"
				if c.inRecs && c.inSims {
					score = 98.0
					reason = "Top Recommendation • High Alignment"
				} else if c.inRecs {
					score = 94.0
					reason = "Recommended • Thematic Match"
				}

				dto := RecommendedItemDTO{
					Type:        "MOVIE",
					ExternalID:  strconv.Itoa(c.id),
					Title:       c.title,
					ArtworkPath: c.posterPath,
					Year:        year,
					Overview:    c.overview,
					MatchReason: reason,
					Score:       score,
				}

				var local models.Movie
				if err := s.db.WithContext(ctx).Where("tmdb_id = ?", c.id).First(&local).Error; err == nil {
					dto.ID = local.ID
					if dto.ArtworkPath == "" && local.PosterPath != "" {
						dto.ArtworkPath = local.PosterPath
					}
				}
				if dto.ArtworkPath == "" && tmdbClient != nil && c.id > 0 {
					if detail, err := tmdbClient.GetMovie(ctx, c.id); err == nil && detail != nil {
						dto.ArtworkPath = detail.PosterPath
					}
				}

				if tr, ok := trackedMap[key]; ok {
					dto.IsTracked = true
					dto.UserStatus = tr.Status
					dto.UserRating = tr.Rating
				}

				results = append(results, dto)
			}
		}

		var src models.Movie
		if tmdbID, _ := strconv.Atoi(externalID); tmdbID > 0 {
			_ = s.db.WithContext(ctx).Where("tmdb_id = ?", tmdbID).First(&src).Error
		}
		if src.ID > 0 {
			tagItems := s.findTagSimilarMedia(ctx, "MOVIE", src.ID, trackedMap, seen)
			for _, ti := range tagItems {
				results = append(results, RecommendedItemDTO{
					ID:          ti.ID,
					Type:        ti.Type,
					ExternalID:  ti.ExternalID,
					Title:       ti.Title,
					ArtworkPath: ti.ArtworkPath,
					Year:        ti.Year,
					Overview:    ti.Overview,
					IsTracked:   ti.IsTracked,
					UserStatus:  ti.UserStatus,
					UserRating:  ti.UserRating,
					MatchReason: "Shared Universe & Tags",
					Score:       90.0,
				})
			}
		}

	case "GAME":
		igdbID, _ := strconv.Atoi(externalID)
		if igdbID > 0 && igdbClient != nil && igdbClient.Configured() {
			simMap, err := igdbClient.SimilarGames(ctx, []int{igdbID})
			if err == nil {
				if simList, ok := simMap[igdbID]; ok {
					for _, g := range simList {
						if g.ID == igdbID {
							continue
						}
						key := fmt.Sprintf("GAME:%d", g.ID)
						if seen[key] || rejectedMap[key] {
							continue
						}
						seen[key] = true

						art := ""
						if g.CoverImageID != "" {
							art = fmt.Sprintf("https://images.igdb.com/igdb/image/upload/t_cover_big/%s.jpg", g.CoverImageID)
						}

						dto := RecommendedItemDTO{
							Type:        "GAME",
							ExternalID:  strconv.Itoa(g.ID),
							Title:       g.Name,
							ArtworkPath: art,
							Year:        g.Year,
							Overview:    g.Summary,
							MatchReason: "Similar Gameplay & Setting",
							Score:       92.0,
						}

						var local models.Game
						if err := s.db.WithContext(ctx).Where("igdb_id = ?", g.ID).First(&local).Error; err == nil {
							dto.ID = local.ID
							if local.CoverPath != "" {
								dto.ArtworkPath = local.CoverPath
							}
						}

						if tr, ok := trackedMap[key]; ok {
							dto.IsTracked = true
							dto.UserStatus = tr.Status
							dto.UserRating = tr.Rating
						}

						results = append(results, dto)
					}
				}
			}
		}

		var src models.Game
		if igdbID, _ := strconv.Atoi(externalID); igdbID > 0 {
			_ = s.db.WithContext(ctx).Where("igdb_id = ?", igdbID).First(&src).Error
		}
		if src.ID > 0 {
			tagItems := s.findTagSimilarMedia(ctx, "GAME", src.ID, trackedMap, seen)
			for _, ti := range tagItems {
				results = append(results, RecommendedItemDTO{
					ID:          ti.ID,
					Type:        ti.Type,
					ExternalID:  ti.ExternalID,
					Title:       ti.Title,
					ArtworkPath: ti.ArtworkPath,
					Year:        ti.Year,
					Overview:    ti.Overview,
					IsTracked:   ti.IsTracked,
					UserStatus:  ti.UserStatus,
					UserRating:  ti.UserRating,
					MatchReason: "Shared Universe & Tags",
					Score:       90.0,
				})
			}
		}

	case "BOOK":
		var src models.Book
		_ = s.db.WithContext(ctx).Where("isbn13 = ?", externalID).First(&src).Error
		if src.ID > 0 {
			tagItems := s.findTagSimilarMedia(ctx, "BOOK", src.ID, trackedMap, seen)
			for _, ti := range tagItems {
				results = append(results, RecommendedItemDTO{
					ID:          ti.ID,
					Type:        ti.Type,
					ExternalID:  ti.ExternalID,
					Title:       ti.Title,
					ArtworkPath: ti.ArtworkPath,
					Year:        ti.Year,
					Overview:    ti.Overview,
					IsTracked:   ti.IsTracked,
					UserStatus:  ti.UserStatus,
					UserRating:  ti.UserRating,
					MatchReason: "Similar Author & Genre",
					Score:       90.0,
				})
			}
		}
	}

	// Sort results by Score descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > 6 {
		results = results[:6]
	}

	// Persist newly fetched recommendations
	if len(results) > 0 {
		_ = s.db.WithContext(ctx).
			Where("user_id = ? AND source_type = ? AND source_id = ?", userID, mediaType, externalID).
			Delete(&models.StoredRecommendation{}).Error

		for _, item := range results {
			sr := models.StoredRecommendation{
				UserID:      userID,
				SourceType:  mediaType,
				SourceID:    externalID,
				TargetType:  item.Type,
				TargetID:    item.ExternalID,
				Title:       item.Title,
				ArtworkPath: item.ArtworkPath,
				Year:        item.Year,
				Overview:    item.Overview,
				Score:       item.Score,
				MatchReason: item.MatchReason,
				CreatedAt:   time.Now(),
			}
			_ = s.db.WithContext(ctx).Create(&sr).Error
		}
	}

	return results, nil
}

// DismissRecommendation records that the user is not interested in a recommendation.
func (s *Service) DismissRecommendation(ctx context.Context, userID uint, mediaType string, externalID string) error {
	rec := models.RejectedRec{
		UserID:     userID,
		Type:       strings.ToUpper(mediaType),
		ExternalID: externalID,
		CreatedAt:  time.Now(),
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error
}
