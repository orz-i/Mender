package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// MaxDownloadSizeBytes is the hard cutoff (256MB) for streamed archive packages.
	MaxDownloadSizeBytes = int64(256 * 1024 * 1024)

	// MaxMetadataSizeBytes is the cutoff (10MB) for catalog index metadata envelopes.
	MaxMetadataSizeBytes = int64(10 * 1024 * 1024)

	defaultHTTPTimeout = 60 * time.Second
)

// HTTPClient represents the HTTP client interface for downloading artifacts.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Downloader manages streaming HTTP downloads with size caps and SHA-256 verification.
type Downloader struct {
	client     HTTPClient
	maxArchive int64
}

// NewDownloader creates a new Downloader. If client is nil, http.DefaultClient with timeout is used.
func NewDownloader(client HTTPClient) *Downloader {
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Downloader{
		client:     client,
		maxArchive: MaxDownloadSizeBytes,
	}
}

// DownloadArchive streams a remote package archive into a temporary file while verifying its SHA-256 and size cutoff.
func (d *Downloader) DownloadArchive(
	ctx context.Context,
	downloadURL string,
	expectedSHA256 string,
	targetDir string,
) (string, error) {
	parsedURL, err := url.ParseRequestURI(downloadURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return "", fmt.Errorf("downloader: invalid download url %q", downloadURL)
	}

	cleanExpected := strings.ToLower(strings.TrimSpace(expectedSHA256))
	cleanExpected = strings.TrimPrefix(cleanExpected, "sha256:")
	if len(cleanExpected) != 64 {
		return "", fmt.Errorf("downloader: invalid expected sha256 checksum %q", expectedSHA256)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("downloader: create request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloader: execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloader: unexpected http status %d (%s)", resp.StatusCode, resp.Status)
	}

	if resp.ContentLength > d.maxArchive {
		return "", fmt.Errorf("downloader: content length %d exceeds maximum allowed size %d", resp.ContentLength, d.maxArchive)
	}

	// Create temp file for writing
	tempFile, err := os.CreateTemp(targetDir, "mender-download-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("downloader: create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	success := false
	defer func() {
		if !success {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
		}
	}()

	hasher := sha256.New()
	// LimitReader bounds the read to maxArchive + 1 to detect overflows even when Content-Length is missing or chunked
	limitedReader := io.LimitReader(resp.Body, d.maxArchive+1)
	writer := io.MultiWriter(tempFile, hasher)

	copied, err := io.Copy(writer, limitedReader)
	if err != nil {
		return "", fmt.Errorf("downloader: stream copy: %w", err)
	}
	if copied > d.maxArchive {
		return "", fmt.Errorf("downloader: downloaded bytes exceeded cutoff of %d bytes", d.maxArchive)
	}
	if err := tempFile.Sync(); err != nil {
		return "", fmt.Errorf("downloader: sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("downloader: close temp file: %w", err)
	}

	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if actualSHA256 != cleanExpected {
		return "", fmt.Errorf("downloader: sha256 mismatch (got %s, want %s)", actualSHA256, cleanExpected)
	}

	success = true
	return tempPath, nil
}

// DownloadMetadata fetches raw metadata bytes (e.g. signed envelope JSON) bounded by 10MB cutoff.
func (d *Downloader) DownloadMetadata(ctx context.Context, endpointURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return nil, fmt.Errorf("downloader: create metadata request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloader: fetch metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloader: metadata http status %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxMetadataSizeBytes {
		return nil, fmt.Errorf("downloader: metadata content length %d exceeds max %d", resp.ContentLength, MaxMetadataSizeBytes)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxMetadataSizeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("downloader: read metadata: %w", err)
	}
	if int64(len(data)) > MaxMetadataSizeBytes {
		return nil, fmt.Errorf("downloader: metadata exceeded max size of %d bytes", MaxMetadataSizeBytes)
	}

	return data, nil
}
