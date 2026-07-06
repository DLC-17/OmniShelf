// Package config loads OmniShelf configuration from environment variables
// (project_spec.md §1.3). Configuration comes from env vars only.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

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
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("OMNISHELF_JWT_SECRET is not set: refusing to start (a stable secret is required so sessions survive restarts)")
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
