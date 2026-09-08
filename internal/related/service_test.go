package related_test

import (
	"context"
	"testing"

	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
	"github.com/davidlc1229/omnishelf/internal/related"
)

func TestRelatedService(t *testing.T) {
	tmpDir := t.TempDir()
	gdb, err := db.Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	svc := related.NewService(gdb)
	ctx := context.Background()

	// Seed media
	movie1 := models.Movie{TMDBID: 101, Title: "The Fellowship of the Ring", PosterPath: "movie/lotr1.jpg"}
	movie2 := models.Movie{TMDBID: 102, Title: "The Two Towers", PosterPath: "movie/lotr2.jpg"}
	book1 := models.Book{ISBN13: "9780261103573", Title: "The Fellowship of the Ring", CoverPath: "book/lotr1.jpg"}

	gdb.Create(&movie1)
	gdb.Create(&movie2)
	gdb.Create(&book1)

	// Link movie1 and movie2 as sequel
	if err := svc.AddRelation(ctx, "MOVIE", movie1.ID, "MOVIE", movie2.ID, "sequel"); err != nil {
		t.Fatalf("failed to add relation: %v", err)
	}

	// Link movie1 and book1 as adaptation
	if err := svc.AddRelation(ctx, "MOVIE", movie1.ID, "BOOK", book1.ID, "adaptation"); err != nil {
		t.Fatalf("failed to add relation: %v", err)
	}

	// Test GetRelatedForMedia
	items, err := svc.GetRelatedForMedia(ctx, 1, "MOVIE", "101")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 related items, got %d", len(items))
	}

	graph, err := svc.GetFranchiseGraph(ctx, 1, "MOVIE", "101")
	if err != nil {
		t.Fatalf("unexpected error getting graph: %v", err)
	}

	if len(graph.Nodes) != 3 {
		t.Fatalf("expected 3 graph nodes (root + 2 related), got %d", len(graph.Nodes))
	}
}

func TestGameRelationIsolationAndLookup(t *testing.T) {
	tmpDir := t.TempDir()
	gdb, err := db.Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	svc := related.NewService(gdb)
	ctx := context.Background()

	// Seed Lego Batman Collection
	lb1 := models.Game{IGDBID: 1001, Title: "Lego Batman: The Videogame", ReleaseDate: "2008-09-23"}
	lb2 := models.Game{IGDBID: 1002, Title: "Lego Batman 2: DC Super Heroes", ReleaseDate: "2012-06-19"}
	lb3 := models.Game{IGDBID: 1003, Title: "Lego Batman 3: Beyond Gotham", ReleaseDate: "2014-11-11"}

	// Seed Spider-Man Insomniac series
	sm1 := models.Game{IGDBID: 2001, Title: "Marvel's Spider-Man", ReleaseDate: "2018-09-07"}
	sm2 := models.Game{IGDBID: 2002, Title: "Marvel's Spider-Man 2", ReleaseDate: "2023-10-20"}

	// Seed Standalone Spider-Man game
	smWos := models.Game{IGDBID: 3001, Title: "Spider-Man: Web of Shadows", ReleaseDate: "2008-10-21"}

	gdb.Create(&lb1)
	gdb.Create(&lb2)
	gdb.Create(&lb3)
	gdb.Create(&sm1)
	gdb.Create(&sm2)
	gdb.Create(&smWos)

	// Link Lego Batman series (lb1 <-> lb2 <-> lb3)
	_ = svc.AddRelation(ctx, "GAME", lb1.ID, "GAME", lb2.ID, "sequel")
	_ = svc.AddRelation(ctx, "GAME", lb2.ID, "GAME", lb3.ID, "sequel")

	// Link Insomniac Spider-Man series (sm1 <-> sm2)
	_ = svc.AddRelation(ctx, "GAME", sm1.ID, "GAME", sm2.ID, "sequel")

	// 1. Verify Lego Batman 1 has Lego Batman 2
	lb1Related, err := svc.GetRelatedForMedia(ctx, 1, "GAME", "1001")
	if err != nil {
		t.Fatalf("failed to get related for lb1: %v", err)
	}
	if len(lb1Related) != 1 || lb1Related[0].Title != "Lego Batman 2: DC Super Heroes" {
		t.Fatalf("unexpected related for lb1: %+v", lb1Related)
	}

	// 2. Verify Insomniac Spider-Man 1 only relates to Spider-Man 2, NOT Web of Shadows
	sm1Related, err := svc.GetRelatedForMedia(ctx, 1, "GAME", "2001")
	if err != nil {
		t.Fatalf("failed to get related for sm1: %v", err)
	}
	if len(sm1Related) != 1 || sm1Related[0].Title != "Marvel's Spider-Man 2" {
		t.Fatalf("expected sm1 to only relate to Marvel's Spider-Man 2, got: %+v", sm1Related)
	}

	// 3. Verify standalone Web of Shadows has 0 related games
	wosRelated, err := svc.GetRelatedForMedia(ctx, 1, "GAME", "3001")
	if err != nil {
		t.Fatalf("failed to get related for wos: %v", err)
	}
	if len(wosRelated) != 0 {
		t.Fatalf("expected standalone Web of Shadows to have 0 related games, got: %+v", wosRelated)
	}
}

func TestRecommendations(t *testing.T) {
	tmpDir := t.TempDir()
	gdb, err := db.Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	svc := related.NewService(gdb)
	ctx := context.Background()

	// Seed TV shows
	show1 := models.Show{TMDBID: 1399, Title: "Game of Thrones", PosterPath: "/got.jpg"}
	show2 := models.Show{TMDBID: 94997, Title: "House of the Dragon", PosterPath: "/hotd.jpg"}
	show3 := models.Show{TMDBID: 100088, Title: "The Last of Us", PosterPath: "/tlou.jpg"}

	gdb.Create(&show1)
	gdb.Create(&show2)
	gdb.Create(&show3)

	// Create a tag and link show1 and show2
	tag := models.Tag{Name: "fantasy"}
	gdb.Create(&tag)
	gdb.Create(&models.MediaTag{MediaType: "TV", MediaID: show1.ID, TagID: tag.ID})
	gdb.Create(&models.MediaTag{MediaType: "TV", MediaID: show2.ID, TagID: tag.ID})

	// User 1 tracks show2 as WATCHING
	gdb.Create(&models.TrackingItem{
		UserID:     1,
		Type:       "TV",
		ExternalID: "94997",
		Title:      "House of the Dragon",
		Status:     "WATCHING",
		Rating:     5,
	})

	// Get recommendations for show1
	recs, err := svc.GetRecommendationsForMedia(ctx, 1, "TV", "1399", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error getting recommendations: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation from tag similarity, got %d", len(recs))
	}

	if recs[0].Title != "House of the Dragon" {
		t.Fatalf("expected recommendation 'House of the Dragon', got %s", recs[0].Title)
	}

	if !recs[0].IsTracked || recs[0].UserStatus != "WATCHING" || recs[0].UserRating != 5 {
		t.Fatalf("expected tracking overlay to be set: %+v", recs[0])
	}

	// Verify recommendation was persisted in StoredRecommendation
	var stored []models.StoredRecommendation
	gdb.Where("user_id = 1 AND source_type = 'TV' AND source_id = '1399'").Find(&stored)
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored recommendation in db, got %d", len(stored))
	}

	// Test DismissRecommendation
	if err := svc.DismissRecommendation(ctx, 1, "TV", "94997"); err != nil {
		t.Fatalf("unexpected error dismissing recommendation: %v", err)
	}

	var rejected []models.RejectedRec
	gdb.Where("user_id = 1 AND type = 'TV' AND external_id = '94997'").Find(&rejected)
	if len(rejected) != 1 {
		t.Fatalf("expected 1 rejected rec, got %d", len(rejected))
	}
}
