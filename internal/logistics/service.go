package logistics

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/davidlc1229/omnishelf/internal/models"
)

// Service provides high-level location management and label generation.
type Service struct {
	db *gorm.DB
}

// NewService instantiates a new logistics service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// ListLocations retrieves and aggregates all physical locations used by a user.
func (s *Service) ListLocations(ctx context.Context, userID uint, filter LocationFilter) ([]LocationSummary, error) {
	var items []models.TrackingItem
	q := s.db.WithContext(ctx).Where("user_id = ? AND location <> ''", userID)

	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}

	if err := q.Order("location COLLATE NOCASE, title COLLATE NOCASE").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("listing items with location for user %d: %w", userID, err)
	}

	// Group items by location
	grouped := make(map[string][]models.TrackingItem)
	for _, item := range items {
		loc := NormalizeLocation(item.Location)
		if loc != "" {
			grouped[loc] = append(grouped[loc], item)
		}
	}

	summaries := make([]LocationSummary, 0, len(grouped))
	for loc, locItems := range grouped {
		cType := DetectContainerType(loc)
		if filter.ContainerType != "" && cType != filter.ContainerType {
			continue
		}

		itemTypes := make(map[string]int)
		previews := make([]ContainerItem, 0, len(locItems))

		for _, it := range locItems {
			itemTypes[it.Type]++
			previews = append(previews, ContainerItem{
				ID:          it.ID,
				Type:        it.Type,
				Title:       it.Title,
				ExternalID:  it.ExternalID,
				Status:      it.Status,
				SubLocation: it.Location,
				UpdatedAt:   it.UpdatedAt,
			})
		}

		var binderDetails *BinderDetails
		if cType == ContainerBinder {
			bName, pos, ok := ParseBinderLocation(loc)
			if ok && pos != nil {
				binderDetails = &BinderDetails{
					BinderName: bName,
					Page:       pos.Page,
					Slot:       pos.Slot,
				}
			}
		}

		summaries = append(summaries, LocationSummary{
			Location:      loc,
			ContainerType: cType,
			ItemCount:     len(locItems),
			ItemTypes:     itemTypes,
			FilterURL:     FormatFilterURL("", loc),
			BinderDetails: binderDetails,
			Items:         previews,
		})
	}

	// Sort alphabetically by location name
	sort.Slice(summaries, func(i, j int) bool {
		return strings.ToLower(summaries[i].Location) < strings.ToLower(summaries[j].Location)
	})

	return summaries, nil
}

// GetLocationDetails fetches full details for a specific location belonging to a user.
func (s *Service) GetLocationDetails(ctx context.Context, userID uint, location string) (*LocationSummary, error) {
	loc := NormalizeLocation(location)
	if loc == "" {
		return nil, ErrEmptyLocation
	}

	var items []models.TrackingItem
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND location = ?", userID, loc).
		Order("title COLLATE NOCASE, id").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("loading items for location %q (user %d): %w", loc, userID, err)
	}

	if len(items) == 0 {
		return nil, ErrLocationNotFound
	}

	cType := DetectContainerType(loc)
	itemTypes := make(map[string]int)
	containerItems := make([]ContainerItem, 0, len(items))

	for _, it := range items {
		itemTypes[it.Type]++
		containerItems = append(containerItems, ContainerItem{
			ID:          it.ID,
			Type:        it.Type,
			Title:       it.Title,
			ExternalID:  it.ExternalID,
			Status:      it.Status,
			SubLocation: it.Location,
			UpdatedAt:   it.UpdatedAt,
		})
	}

	var binderDetails *BinderDetails
	if cType == ContainerBinder {
		bName, pos, ok := ParseBinderLocation(loc)
		if ok && pos != nil {
			binderDetails = &BinderDetails{
				BinderName: bName,
				Page:       pos.Page,
				Slot:       pos.Slot,
			}
		}
	}

	return &LocationSummary{
		Location:      loc,
		ContainerType: cType,
		ItemCount:     len(items),
		ItemTypes:     itemTypes,
		FilterURL:     FormatFilterURL("", loc),
		BinderDetails: binderDetails,
		Items:         containerItems,
	}, nil
}

// GenerateLabelsForUser generates printable SVG labels from user's locations or custom label overrides.
func (s *Service) GenerateLabelsForUser(ctx context.Context, userID uint, opts LabelOptions, locationNames []string, customLabels []ShelfLabel) (string, []ShelfLabel, error) {
	var finalLabels []ShelfLabel

	if len(customLabels) > 0 {
		for _, cl := range customLabels {
			if cl.Location == "" {
				continue
			}
			if cl.ContainerType == "" {
				cl.ContainerType = DetectContainerType(cl.Location)
			}
			if cl.FilterURL == "" {
				cl.FilterURL = FormatFilterURL(opts.BaseURL, cl.Location)
			}
			if cl.Barcode == "" {
				cl.Barcode = GenerateBarcodeData(cl.Location)
			}
			finalLabels = append(finalLabels, cl)
		}
	} else if len(locationNames) > 0 {
		// Specific locations requested
		for _, name := range locationNames {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			summary, err := s.GetLocationDetails(ctx, userID, name)
			if err != nil {
				// If not found in DB, still generate a clean label with 0 items
				cType := DetectContainerType(name)
				finalLabels = append(finalLabels, ShelfLabel{
					Location:      name,
					Title:         name,
					ContainerType: cType,
					Barcode:       GenerateBarcodeData(name),
					FilterURL:     FormatFilterURL(opts.BaseURL, name),
					ItemCount:     0,
				})
				continue
			}

			previews := make([]string, 0, len(summary.Items))
			for _, it := range summary.Items {
				previews = append(previews, it.Title)
			}

			finalLabels = append(finalLabels, ShelfLabel{
				Location:      summary.Location,
				Title:         summary.Location,
				ContainerType: summary.ContainerType,
				Barcode:       GenerateBarcodeData(summary.Location),
				FilterURL:     FormatFilterURL(opts.BaseURL, summary.Location),
				ItemCount:     summary.ItemCount,
				ItemTypes:     summary.ItemTypes,
				ItemPreviews:  previews,
			})
		}
	} else {
		// All user locations
		summaries, err := s.ListLocations(ctx, userID, LocationFilter{})
		if err != nil {
			return "", nil, fmt.Errorf("listing locations: %w", err)
		}

		for _, sum := range summaries {
			previews := make([]string, 0, len(sum.Items))
			for _, it := range sum.Items {
				previews = append(previews, it.Title)
			}

			finalLabels = append(finalLabels, ShelfLabel{
				Location:      sum.Location,
				Title:         sum.Location,
				ContainerType: sum.ContainerType,
				Barcode:       GenerateBarcodeData(sum.Location),
				FilterURL:     FormatFilterURL(opts.BaseURL, sum.Location),
				ItemCount:     sum.ItemCount,
				ItemTypes:     sum.ItemTypes,
				ItemPreviews:  previews,
			})
		}
	}

	if len(finalLabels) == 0 {
		// Fallback sample label if user has no locations yet
		finalLabels = append(finalLabels, ShelfLabel{
			Location:      "Shelf A",
			Title:         "Shelf A",
			ContainerType: ContainerShelf,
			Barcode:       GenerateBarcodeData("Shelf A"),
			FilterURL:     FormatFilterURL(opts.BaseURL, "Shelf A"),
			ItemCount:     0,
		})
	}

	format := opts.Format
	if format == "" {
		format = FormatAvery5160
	}

	svg, err := GenerateLabels(format, finalLabels, opts)
	if err != nil {
		return "", nil, err
	}

	return svg, finalLabels, nil
}
