package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSourceLoaderLoadsURL(t *testing.T) {
	t.Parallel()

	const body = "openapi: 3.2.1\ninfo:\n  title: Demo\n  version: 1\n"
	var accept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	loaded, err := newSourceLoader().Load(t.Context(), SourceConfig{
		Kind: SourceURL,
		URL:  server.URL + "/openapi.yaml",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(loaded.data) != body {
		t.Fatalf("Load() data = %q", loaded.data)
	}
	if loaded.location == nil || loaded.location.String() != server.URL+"/openapi.yaml" {
		t.Fatalf("Load() location = %v", loaded.location)
	}
	if !strings.Contains(accept, "application/json") {
		t.Fatalf("Accept = %q", accept)
	}
}

func TestSourceLoaderDoesNotFollowRedirect(t *testing.T) {
	t.Parallel()

	var targetCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/source", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/target", http.StatusFound)
	})
	mux.HandleFunc("/target", func(w http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		_, _ = w.Write([]byte("openapi: 3.2.1"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	_, err := newSourceLoader().Load(t.Context(), SourceConfig{
		Kind: SourceURL,
		URL:  server.URL + "/source",
	})
	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("Load() error = %v, want redirect status error", err)
	}
	if got := targetCalls.Load(); got != 0 {
		t.Fatalf("redirect target calls = %d, want 0", got)
	}
}

func TestSourceLoaderLoadsFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "openapi.yaml")
	const body = "openapi: 3.2.1\ninfo:\n  title: Demo\n  version: 1\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := newSourceLoader().Load(t.Context(), SourceConfig{
		Kind: SourceFile,
		Path: path,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(loaded.data) != body {
		t.Fatalf("Load() data = %q", loaded.data)
	}
	if loaded.location == nil || loaded.location.Scheme != "file" {
		t.Fatalf("Load() location = %v", loaded.location)
	}
}

func TestSourceLoaderRejectsLargeFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "large.yaml")
	file, err := os.Create(path) //nolint:gosec // test file under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxDescriptionBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = newSourceLoader().Load(t.Context(), SourceConfig{
		Kind: SourceFile,
		Path: path,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Load() error = %v, want size error", err)
	}
}

func TestSourceLoaderHonorsHTTPTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte("openapi: 3.2.1"))
	}))
	t.Cleanup(server.Close)

	loader := sourceLoader{
		httpClient: &http.Client{
			Timeout: 10 * time.Millisecond,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	_, err := loader.Load(context.Background(), SourceConfig{
		Kind: SourceURL,
		URL:  server.URL,
	})
	if err == nil {
		t.Fatal("Load() error = nil, want timeout")
	}
}
