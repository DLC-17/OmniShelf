package images

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeJPEG is a tiny stand-in payload; content only matters as bytes.
var fakeJPEG = []byte("\xff\xd8\xff\xe0fake-jpeg-bytes\xff\xd9")

func assertNoTempFiles(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			assert.False(t, strings.HasSuffix(d.Name(), ".tmp"),
				"leftover temp file: %s", path)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestFetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	root := t.TempDir()
	store := New(root)
	rel, err := store.Fetch(context.Background(), srv.Client(), srv.URL+"/poster.jpg", "tv", "1399")
	require.NoError(t, err)
	assert.Equal(t, "tv/1399.jpg", rel, "must return the relative path")

	data, err := os.ReadFile(filepath.Join(root, "tv", "1399.jpg"))
	require.NoError(t, err)
	assert.Equal(t, fakeJPEG, data)
	assertNoTempFiles(t, root)
}

func TestFetchCreatesKindSubdir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	root := t.TempDir()
	rel, err := New(root).Fetch(context.Background(), srv.Client(), srv.URL, "book", "9780140328721")
	require.NoError(t, err)
	assert.Equal(t, "book/9780140328721.jpg", rel)
	assert.FileExists(t, filepath.Join(root, "book", "9780140328721.jpg"))
}

func TestFetchOverwritesExistingFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeJPEG)
	}))
	defer srv.Close()

	root := t.TempDir()
	store := New(root)
	// Pre-existing stale artwork must be replaced (nightly artwork retry).
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tv"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tv", "1399.jpg"), []byte("stale"), 0o644))

	rel, err := store.Fetch(context.Background(), srv.Client(), srv.URL, "tv", "1399")
	require.NoError(t, err)
	assert.Equal(t, "tv/1399.jpg", rel)
	data, err := os.ReadFile(filepath.Join(root, "tv", "1399.jpg"))
	require.NoError(t, err)
	assert.Equal(t, fakeJPEG, data)
	assertNoTempFiles(t, root)
}

func TestFetchRejectsNonImageContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>not an image</html>"))
	}))
	defer srv.Close()

	root := t.TempDir()
	_, err := New(root).Fetch(context.Background(), srv.Client(), srv.URL, "tv", "1399")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content-type")
	assert.NoFileExists(t, filepath.Join(root, "tv", "1399.jpg"))
	assertNoTempFiles(t, root)
}

func TestFetchMidStreamFailureLeavesNoTempFile(t *testing.T) {
	// E13: server advertises a large image then kills the connection
	// mid-body. Fetch must fail, and neither the destination file nor any
	// .tmp file may remain.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		w.Write(fakeJPEG) // far less than the advertised length
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Abort the connection so the client sees an unexpected EOF.
		hj, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, _, err := hj.Hijack()
		require.NoError(t, err)
		conn.Close()
	}))
	defer srv.Close()

	root := t.TempDir()
	client := &http.Client{Timeout: 5 * time.Second}
	_, err := New(root).Fetch(context.Background(), client, srv.URL, "tv", "1399")
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(root, "tv", "1399.jpg"))
	assertNoTempFiles(t, root)
}

func TestFetchNon200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	root := t.TempDir()
	_, err := New(root).Fetch(context.Background(), srv.Client(), srv.URL, "book", "123")
	require.Error(t, err)
	assertNoTempFiles(t, root)
}
