// Command omnishelf runs the OmniShelf server (default) or one of its
// administrative subcommands (`invite`).
package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"

	omnishelf "github.com/davidlc1229/omnishelf"
	"github.com/davidlc1229/omnishelf/internal/api"
	"github.com/davidlc1229/omnishelf/internal/books"
	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/games"
	"github.com/davidlc1229/omnishelf/internal/igdb"
	"github.com/davidlc1229/omnishelf/internal/images"
	"github.com/davidlc1229/omnishelf/internal/importer"
	"github.com/davidlc1229/omnishelf/internal/movies"
	"github.com/davidlc1229/omnishelf/internal/openlibrary"
	"github.com/davidlc1229/omnishelf/internal/scandex"
	syncengine "github.com/davidlc1229/omnishelf/internal/sync"
	"github.com/davidlc1229/omnishelf/internal/tmdb"
	"github.com/davidlc1229/omnishelf/internal/tv"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "invite":
			runInvite(os.Args[2:])
			return
		default:
			log.Printf("unknown subcommand %q (available: invite)", os.Args[1])
			os.Exit(1)
		}
	}

	if err := runServer(); err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
}

func runServer() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	gdb, err := db.Open(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	assets, err := omnishelf.UIAssets()
	if err != nil {
		return fmt.Errorf("preparing embedded UI: %w", err)
	}

	// Imports left RUNNING by an unclean shutdown can never finish; mark
	// them FAILED before serving so their owners see it.
	if err := importer.MarkInterrupted(gdb); err != nil {
		return fmt.Errorf("marking interrupted imports: %w", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// Shared external clients and image cache.
	tmdbClient := tmdb.New(cfg.TMDBAPIKey)
	olClient := openlibrary.New(cfg.ContactEmail)
	scandexClient := scandex.New(cfg.ScandexUserID, cfg.ScandexAccessToken)
	igdbClient := igdb.New(cfg.IGDBClientID, cfg.IGDBClientSecret)
	imageStore := images.New(cfg.ImagesDir)

	// Unauthenticated liveness probe (Docker HEALTHCHECK / TrueNAS): reachable
	// without a JWT, so it is attached to the bare engine, not the group below.
	api.RegisterHealthRoutes(router, gdb, cfg.ImagesDir)

	// Auth endpoints plus the JWT-protected /api group that later domains
	// (tv, books, feed, imports) extend.
	protected := api.RegisterRoutes(router, gdb, cfg)

	tvSvc := tv.New(gdb, tmdbClient, imageStore)
	api.RegisterTVRoutes(protected, tvSvc)

	movieSvc := movies.New(gdb, tmdbClient, imageStore)
	api.RegisterMovieRoutes(protected, movieSvc)

	bookSvc := books.NewService(gdb, olClient, imageStore)
	api.RegisterBookRoutes(protected, bookSvc)
	api.RegisterLibraryRoutes(protected, bookSvc)

	gameSvc := games.NewService(gdb, scandexClient, igdbClient, imageStore)
	api.RegisterGameRoutes(protected, gameSvc)

	imp := importer.New(importer.Config{DB: gdb, TMDB: tmdbClient, Images: imageStore})
	api.RegisterImportRoutes(protected, imp)

	api.RegisterFeedRoutes(protected, gdb)
	api.RegisterUserRoutes(protected, gdb)

	// Nightly TMDB sync at 03:00.
	engine := syncengine.New(gdb, tmdbClient, imageStore)
	scheduler := cron.New()
	if err := engine.Schedule(scheduler); err != nil {
		return fmt.Errorf("scheduling nightly sync: %w", err)
	}
	scheduler.Start()

	// Cached artwork straight off the images volume.
	router.StaticFS("/images", gin.Dir(cfg.ImagesDir, false))

	// API routes are registered by later tasks; an unknown /api path must be a
	// JSON 404 in the standard envelope, never the SPA fallback HTML.
	router.NoRoute(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
			c.JSON(404, gin.H{"error": "not_found", "message": "unknown API route"})
			return
		}
		c.FileFromFS(c.Request.URL.Path, assets)
	})

	addr := net.JoinHostPort("0.0.0.0", cfg.Port)
	log.Printf("omnishelf listening on %s (data=%s images=%s)", addr, cfg.DataDir, cfg.ImagesDir)
	if err := router.Run(addr); err != nil {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}
