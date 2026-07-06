// Package omnishelf (repo root) exists solely to host the go:embed directive
// for the built frontend: embed paths cannot reference parent directories, so
// embedding ui/dist from cmd/ or internal/ is impossible. Build prerequisite:
// `cd ui && npm run build` must have produced ui/dist before `go build`.
package omnishelf

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
)

// The all: prefix includes dotfiles, so the embed compiles even when ui/dist
// holds only the committed .gitkeep (a real build is still required to serve
// the SPA at runtime).
//
//go:embed all:ui/dist
var distFS embed.FS

// UIAssets returns the embedded SPA as an http.FileSystem with SPA fallback:
// requests for paths that do not exist in the build output are served
// index.html so client-side routing works on hard refresh/deep links.
// API and image routes are registered on the router before this fallback,
// so only unknown non-/api paths reach it.
func UIAssets() (http.FileSystem, error) {
	sub, err := fs.Sub(distFS, "ui/dist")
	if err != nil {
		return nil, fmt.Errorf("locating embedded ui/dist: %w", err)
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, fmt.Errorf("embedded ui/dist has no index.html — run `cd ui && npm run build` before building the binary: %w", err)
	}
	return spaFS{http.FS(sub)}, nil
}

// spaFS wraps an http.FileSystem, falling back to index.html for missing paths.
type spaFS struct {
	inner http.FileSystem
}

func (s spaFS) Open(name string) (http.File, error) {
	f, err := s.inner.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return s.inner.Open("/index.html")
	}
	return f, err
}
