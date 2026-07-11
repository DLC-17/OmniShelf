// Package config loads OmniShelf configuration from environment variables.
// Configuration comes from env vars only.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// minJWTSecretLen is the minimum accepted OMNISHELF_JWT_SECRET length; a
// short HMAC key makes forged session tokens brute-forceable offline.
const minJWTSecretLen = 32

// jwtSecretPlaceholder is the .env.example value, rejected so a deployment
// cannot accidentally go live with the published default.
const jwtSecretPlaceholder = "change-me-to-a-long-random-string"

// Config holds all runtime configuration for OmniShelf.
type Config struct {
	// Port is the HTTP listen port (OMNISHELF_PORT, default 8080).
	Port string
	// DataDir is the SQLite database directory (OMNISHELF_DATA_DIR, default /data).
	DataDir string
	// ImagesDir is the cached image root (OMNISHELF_IMAGES_DIR, default /images).
	ImagesDir string
	// JWTSecret is the HMAC signing key (OMNISHELF_JWT_SECRET, required).
	JWTSecret string
	// TMDBAPIKey is the TMDB v3 API key (TMDB_API_KEY, required for the TV module).
	TMDBAPIKey string
	// ContactEmail is injected into the OpenLibrary User-Agent
	// (OMNISHELF_CONTACT_EMAIL, required for the book module).
	ContactEmail string
	// ScandexUserID and ScandexAccessToken authenticate ScanDex game barcode
	// lookups (SCANDEX_USER_ID, SCANDEX_ACCESS_TOKEN). Optional: when unset the
	// games module returns a clear "not configured" error instead of failing
	// startup.
	ScandexUserID      string
	ScandexAccessToken string
	// IGDBClientID and IGDBClientSecret authenticate IGDB game-detail lookups
	// via Twitch OAuth (IGDB_CLIENT_ID, IGDB_CLIENT_SECRET). Optional: when
	// unset games keep their ScanDex title/platform but gain no cover or
	// summary.
	IGDBClientID     string
	IGDBClientSecret string
	// DiscogsToken authenticates Discogs album barcode lookups
	// (OMNISHELF_DISCOGS_TOKEN). Optional: when unset the music module returns a
	// clear "not configured" error for scans instead of failing startup;
	// MusicBrainz name search (which needs no key) still works.
	DiscogsToken string
	// GoogleVisionCredentials is the path to a Google Cloud service-account
	// JSON file for Vision OCR (GOOGLE_APPLICATION_CREDENTIALS). Optional: when
	// unset the cards module returns a clear "not configured" error for scans
	// instead of failing startup.
	GoogleVisionCredentials string
	// PokemonTCGAPIKey authenticates api.pokemontcg.io card lookups
	// (POKEMONTCG_API_KEY). Optional: the API works keyless at lower rate
	// limits.
	PokemonTCGAPIKey string
}

// Load reads configuration from the environment and validates it.
// It fails fast (returns an error) when OMNISHELF_JWT_SECRET is unset (E15)
// or when the data/images directories are not writable (E14).
func Load() (*Config, error) {
	cfg := &Config{
		Port:         getenvDefault("OMNISHELF_PORT", "8080"),
		DataDir:      getenvDefault("OMNISHELF_DATA_DIR", "/data"),
		ImagesDir:    getenvDefault("OMNISHELF_IMAGES_DIR", "/images"),
		JWTSecret:    os.Getenv("OMNISHELF_JWT_SECRET"),
		TMDBAPIKey:   os.Getenv("TMDB_API_KEY"),
		ContactEmail: os.Getenv("OMNISHELF_CONTACT_EMAIL"),

		ScandexUserID:      os.Getenv("SCANDEX_USER_ID"),
		ScandexAccessToken: os.Getenv("SCANDEX_ACCESS_TOKEN"),

		IGDBClientID:     os.Getenv("IGDB_CLIENT_ID"),
		IGDBClientSecret: os.Getenv("IGDB_CLIENT_SECRET"),

		DiscogsToken: os.Getenv("OMNISHELF_DISCOGS_TOKEN"),

		GoogleVisionCredentials: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		PokemonTCGAPIKey:        os.Getenv("POKEMONTCG_API_KEY"),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("OMNISHELF_JWT_SECRET is not set: refusing to start (a stable secret is required so sessions survive restarts)")
	}
	if len(cfg.JWTSecret) < minJWTSecretLen || cfg.JWTSecret == jwtSecretPlaceholder {
		return nil, fmt.Errorf("OMNISHELF_JWT_SECRET is too weak: set a random value of at least %d characters (generate one with: openssl rand -hex 32)", minJWTSecretLen)
	}

	for _, dir := range []struct{ name, path string }{
		{"OMNISHELF_DATA_DIR", cfg.DataDir},
		{"OMNISHELF_IMAGES_DIR", cfg.ImagesDir},
	} {
		if err := ensureWritableDir(dir.path); err != nil {
			return nil, fmt.Errorf("%s (%s) is not a writable directory: %w", dir.name, dir.path, err)
		}
	}

	return cfg, nil
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ensureWritableDir verifies path exists as a directory and is writable by
// creating and removing a probe file. It does not create missing directories:
// on the target deployment these are mounted volumes, and a missing mount
// must surface as a startup failure, not be papered over.
func ensureWritableDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory")
	}
	probe := filepath.Join(path, ".omnishelf-write-check")
	f, err := os.Create(probe)
	if err != nil {
		return fmt.Errorf("write probe failed: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing write probe: %w", err)
	}
	if err := os.Remove(probe); err != nil {
		return fmt.Errorf("removing write probe: %w", err)
	}
	return nil
}
