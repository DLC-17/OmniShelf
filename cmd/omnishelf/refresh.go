package main

import (
	"context"
	"log"
	"os"

	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/games"
	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/images"
	"github.com/davidlc1229/omnishelf/internal/scandex"
	syncengine "github.com/davidlc1229/omnishelf/internal/sync"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
	"github.com/davidlc1229/omnishelf/internal/tv"
)

func runRefresh(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("refresh: loading configuration: %v", err)
	}

	gdb, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("refresh: opening database: %v", err)
	}

	ctx := context.Background()

	// 1. TV & Movie metadata sync (TMDB)
	tmdbClient := tmdb.New(cfg.TMDBAPIKey)
	imageStore := images.New(cfg.ImagesDir)
	tvSvc := tv.New(gdb, tmdbClient, imageStore)

	engine := syncengine.New(gdb, tmdbClient, imageStore,
		syncengine.WithReconcileWatching(tvSvc.ReconcileAllWatching),
	)

	log.Println("refresh: starting TMDB TV & movie metadata sync...")
	if err := engine.Run(ctx); err != nil {
		log.Printf("refresh: TMDB sync warning: %v", err)
	}

	// 2. Video Game metadata & artwork sync (IGDB & GOG)
	log.Println("refresh: starting video game metadata & cover artwork sync...")
	scandexClient := scandex.New(cfg.ScandexUserID, cfg.ScandexAccessToken)
	igdbClient := igdb.New(cfg.IGDBClientID, cfg.IGDBClientSecret)
	gameSvc := games.NewService(gdb, scandexClient, igdbClient, imageStore)
	if err := gameSvc.RefreshAllGames(ctx); err != nil {
		log.Printf("refresh: game metadata sync warning: %v", err)
	}

	log.Println("refresh: all media metadata successfully refreshed")
	os.Exit(0)
}
