package logistics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
)

func newTestLogisticsService(t *testing.T) (*Service, uint) {
	t.Helper()
	gdb, err := db.Open(t.TempDir())
	require.NoError(t, err)

	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	userID := uint(1)

	// Seed tracking items with locations
	items := []models.TrackingItem{
		{
			UserID:     userID,
			Type:       "BOOK",
			ExternalID: "9780441172719",
			Title:      "Dune",
			Location:   "Shelf A",
			Status:     "READING",
			UpdatedAt:  time.Now(),
		},
		{
			UserID:     userID,
			Type:       "BOOK",
			ExternalID: "9780441569595",
			Title:      "Neuromancer",
			Location:   "Shelf A",
			Status:     "COMPLETED",
			UpdatedAt:  time.Now(),
		},
		{
			UserID:     userID,
			Type:       "GAME",
			ExternalID: "119133",
			Title:      "Elden Ring",
			Location:   "Shelf A",
			Status:     "PLAYING",
			UpdatedAt:  time.Now(),
		},
		{
			UserID:     userID,
			Type:       "CARD",
			ExternalID: "ptcg:base1-4",
			Title:      "Charizard",
			Location:   "Binder 1 (P1/S1)",
			Status:     "OWNED",
			UpdatedAt:  time.Now(),
		},
		{
			UserID:     userID,
			Type:       "CARD",
			ExternalID: "ygo:LOB-001",
			Title:      "Blue-Eyes White Dragon",
			Location:   "Binder 1 (P1/S2)",
			Status:     "OWNED",
			UpdatedAt:  time.Now(),
		},
		{
			UserID:     userID,
			Type:       "MOVIE",
			ExternalID: "603",
			Title:      "The Matrix",
			Location:   "Living Room Cabinet",
			Status:     "COMPLETED",
			UpdatedAt:  time.Now(),
		},
		{
			UserID:     userID,
			Type:       "GAME",
			ExternalID: "1020",
			Title:      "Grand Theft Auto V",
			Location:   "", // unassigned location
			Status:     "COMPLETED",
			UpdatedAt:  time.Now(),
		},
	}

	for i := range items {
		require.NoError(t, gdb.Create(&items[i]).Error)
	}

	return NewService(gdb), userID
}

func TestListLocations(t *testing.T) {
	svc, userID := newTestLogisticsService(t)
	ctx := context.Background()

	// List all locations
	locations, err := svc.ListLocations(ctx, userID, LocationFilter{})
	require.NoError(t, err)
	require.Len(t, locations, 4)

	// Shelf A should have 3 items (2 books, 1 game)
	var shelfA *LocationSummary
	for i := range locations {
		if locations[i].Location == "Shelf A" {
			shelfA = &locations[i]
			break
		}
	}
	require.NotNil(t, shelfA)
	assert.Equal(t, ContainerShelf, shelfA.ContainerType)
	assert.Equal(t, 3, shelfA.ItemCount)
	assert.Equal(t, 2, shelfA.ItemTypes["BOOK"])
	assert.Equal(t, 1, shelfA.ItemTypes["GAME"])
	assert.Equal(t, "/library?location=Shelf+A", shelfA.FilterURL)
	assert.Len(t, shelfA.Items, 3)

	// Filter by Type = CARD
	cardLocations, err := svc.ListLocations(ctx, userID, LocationFilter{Type: "CARD"})
	require.NoError(t, err)
	require.Len(t, cardLocations, 2) // Binder 1 (P1/S1) and Binder 1 (P1/S2)

	// Filter by ContainerType = ContainerCabinet
	cabLocations, err := svc.ListLocations(ctx, userID, LocationFilter{ContainerType: ContainerCabinet})
	require.NoError(t, err)
	require.Len(t, cabLocations, 1)
	assert.Equal(t, "Living Room Cabinet", cabLocations[0].Location)
}

func TestGetLocationDetails(t *testing.T) {
	svc, userID := newTestLogisticsService(t)
	ctx := context.Background()

	details, err := svc.GetLocationDetails(ctx, userID, "Shelf A")
	require.NoError(t, err)
	assert.Equal(t, "Shelf A", details.Location)
	assert.Equal(t, 3, details.ItemCount)
	assert.Len(t, details.Items, 3)

	// Non-existent location
	_, err = svc.GetLocationDetails(ctx, userID, "NonExistent Shelf")
	assert.True(t, errors.Is(err, ErrLocationNotFound))

	// Empty location
	_, err = svc.GetLocationDetails(ctx, userID, "")
	assert.True(t, errors.Is(err, ErrEmptyLocation))
}

func TestGenerateLabelsForUser(t *testing.T) {
	svc, userID := newTestLogisticsService(t)
	ctx := context.Background()

	opts := LabelOptions{
		Format:  FormatAvery5160,
		BaseURL: "https://omnishelf.local",
	}

	// 1. Generate for all user locations
	svg, labels, err := svc.GenerateLabelsForUser(ctx, userID, opts, nil, nil)
	require.NoError(t, err)
	require.Len(t, labels, 4)
	require.Contains(t, svg, "<svg")
	require.Contains(t, svg, "Shelf A")

	// 2. Generate for specific location
	opts.Format = FormatThermal4x6
	svg4x6, labels4x6, err := svc.GenerateLabelsForUser(ctx, userID, opts, []string{"Shelf A"}, nil)
	require.NoError(t, err)
	require.Len(t, labels4x6, 1)
	require.Contains(t, svg4x6, `width="4in"`)
	require.Contains(t, svg4x6, "Shelf A")
	require.Contains(t, svg4x6, "BOOK: 2")

	// 3. Custom label overrides
	opts.Format = FormatThermal2x1
	custom := []ShelfLabel{
		{
			Location:      "Custom Bin 9",
			Title:         "Retro Consoles",
			ContainerType: ContainerBox,
		},
	}
	svg2x1, labels2x1, err := svc.GenerateLabelsForUser(ctx, userID, opts, nil, custom)
	require.NoError(t, err)
	require.Len(t, labels2x1, 1)
	require.Contains(t, svg2x1, `width="2in"`)
	require.Contains(t, svg2x1, "Retro Consoles")
}
