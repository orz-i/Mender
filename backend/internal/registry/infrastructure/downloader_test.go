package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDownloaderSuccess(t *testing.T) {
	t.Parallel()

	content := []byte("mender-package-binary-archive-test-content")
	hasher := sha256.Sum256(content)
	contentSHA256 := hex.EncodeToString(hasher[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(content)
	}))
	defer server.Close()

	downloader := NewDownloader(server.Client())
	tempDir := t.TempDir()

	path, err := downloader.DownloadArchive(context.Background(), server.URL+"/pkg.tar.gz", "sha256:"+contentSHA256, tempDir)
	if err != nil {
		t.Fatalf("DownloadArchive() error = %v", err)
	}
	defer func() { _ = os.Remove(path) }()

	downloaded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(downloaded) != string(content) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloaderSHA256Mismatch(t *testing.T) {
	t.Parallel()

	content := []byte("corrupt-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer server.Close()

	downloader := NewDownloader(server.Client())
	tempDir := t.TempDir()

	wrongSHA256 := strings.Repeat("0", 64)
	_, err := downloader.DownloadArchive(context.Background(), server.URL+"/pkg.tar.gz", wrongSHA256, tempDir)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected sha256 mismatch error, got: %v", err)
	}
}

func TestDownloaderCutoff256MB(t *testing.T) {
	t.Parallel()

	downloader := &Downloader{
		client:     http.DefaultClient,
		maxArchive: 1024, // Low cutoff for test
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send 2048 bytes
		_, _ = w.Write(make([]byte, 2048))
	}))
	defer server.Close()

	downloader.client = server.Client()
	tempDir := t.TempDir()

	dummySHA := strings.Repeat("a", 64)
	_, err := downloader.DownloadArchive(context.Background(), server.URL+"/large.tar.gz", dummySHA, tempDir)
	if err == nil || (!strings.Contains(err.Error(), "exceeds maximum allowed size") && !strings.Contains(err.Error(), "exceeded cutoff")) {
		t.Fatalf("expected cutoff error, got: %v", err)
	}
}
