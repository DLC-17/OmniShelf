// Package images implements the server-side artwork cache writer.
//
// Remote posters/covers are downloaded to {root}/{kind}/{externalID}.jpg via
// a temp file in the same directory followed by os.Rename, so a failed
// download never leaves a partial file behind. Only the
// relative path is returned for storage in the database.
package images

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// safePathComponent reports whether s can be used as a single path element:
// non-empty, no separators, and not a dot-directory reference.
func safePathComponent(s string) bool {
	return s != "" && s != "." && s != ".." &&
		!strings.ContainsAny(s, `/\`) && !strings.ContainsRune(s, 0)
}

// Store writes cached artwork under a root directory.
type Store struct {
	rootDir string
}

// New returns a Store rooted at rootDir. The directory itself is created
// lazily per kind on first Fetch.
func New(rootDir string) *Store {
	return &Store{rootDir: rootDir}
}

// Fetch downloads url to {root}/{kind}/{externalID}.jpg and returns the
// relative path "{kind}/{externalID}.jpg". The body is streamed to a temp
// file in the destination directory and renamed into place on success; any
// failure removes the temp file and returns an error, leaving no partial
// files (E13). Responses without an image content-type are rejected.
func (s *Store) Fetch(ctx context.Context, httpClient *http.Client, url, kind, externalID string) (string, error) {
	// kind and externalID become filesystem path components; reject anything
	// that could escape the root even if a caller forgets to sanitize its
	// upstream-supplied ID (defense in depth against path traversal).
	if !safePathComponent(kind) || !safePathComponent(externalID) {
		return "", fmt.Errorf("images: unsafe path component kind=%q externalID=%q", kind, externalID)
	}
	relPath := filepath.ToSlash(filepath.Join(kind, externalID+".jpg"))
	dir := filepath.Join(s.rootDir, kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("images: create dir %s: %w", dir, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("images: build request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("images: download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("images: download %s: unexpected status %d", url, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		return "", fmt.Errorf("images: download %s: non-image content-type %q", url, ct)
	}

	// Temp file in the destination directory so the final rename is atomic
	// (same filesystem).
	tmp, err := os.CreateTemp(dir, externalID+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("images: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		cleanup()
		return "", fmt.Errorf("images: write %s: %w", relPath, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("images: close temp file: %w", err)
	}

	dest := filepath.Join(dir, externalID+".jpg")
	// Windows os.Rename fails if the destination exists; remove any stale
	// cached copy first so re-fetches (nightly artwork retry) succeed.
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		os.Remove(tmpName)
		return "", fmt.Errorf("images: replace %s: %w", relPath, err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("images: rename into place: %w", err)
	}
	return relPath, nil
}
