package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEngineHTTPClientPostsJSONWithoutRedirectOrAmbientProxy(t *testing.T) {
	t.Parallel()

	var targetCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/target", http.StatusFound)
	})
	mux.HandleFunc("/target", func(w http.ResponseWriter, _ *http.Request) {
		targetCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"ok\":true}"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := newEngineHTTPClient()
	transport, ok := client.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T", client.client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("engine HTTP client inherited ambient proxy function")
	}

	var response map[string]any
	err := client.postJSON(
		t.Context(),
		server.URL+"/start",
		nil,
		map[string]any{"query": "demo"},
		&response,
	)
	if err == nil || !strings.Contains(err.Error(), "302 Found") {
		t.Fatalf("postJSON() error = %v, want redirect status", err)
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", targetCalls)
	}
}

func TestEngineHTTPClientHonorsContextAndResponseBound(t *testing.T) {
	t.Parallel()

	t.Run("context", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte("{}"))
		}))
		t.Cleanup(server.Close)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		err := newEngineHTTPClient().postJSON(
			ctx,
			server.URL,
			nil,
			map[string]any{"x": true},
			&map[string]any{},
		)
		if err == nil {
			t.Fatal("postJSON() error = nil, want context timeout")
		}
	})

	t.Run("size", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(strings.Repeat("x", maxEngineResponseBytes+1)))
		}))
		t.Cleanup(server.Close)

		err := newEngineHTTPClient().postJSON(
			t.Context(),
			server.URL,
			nil,
			map[string]any{"x": true},
			&map[string]any{},
		)
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("postJSON() error = %v, want size error", err)
		}
	})
}

func TestEngineHTTPClientDoesNotReturnProviderBodyOnError(t *testing.T) {
	t.Parallel()

	const sensitive = "sensitive-provider-diagnostic"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("{\"error\":\"sensitive-provider-diagnostic\"}"))
	}))
	t.Cleanup(server.Close)

	err := newEngineHTTPClient().postJSON(
		t.Context(),
		server.URL,
		nil,
		map[string]any{"x": true},
		&map[string]any{},
	)
	if err == nil {
		t.Fatal("postJSON() error = nil")
	}
	if strings.Contains(err.Error(), sensitive) {
		t.Fatalf("provider error leaked body: %v", err)
	}
	if !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Fatalf("postJSON() error = %v", err)
	}
}
