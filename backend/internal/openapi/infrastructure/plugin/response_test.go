package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestExecuteHTTPRequestMapsJSONSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"ok\":true}"))
	}))
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := executeHTTPRequest(t.Context(), newExecutionHTTPClient(), request)
	if err != nil {
		t.Fatalf("executeHTTPRequest() error = %v", err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("outcome kind = %q", outcome.Kind())
	}
	if string(outcome.Content().Bytes()) != "{\"ok\":true}" {
		t.Fatalf("outcome content = %s", outcome.Content().Bytes())
	}
}

func TestExecuteHTTPRequestMapsEmptySuccessToNull(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := executeHTTPRequest(t.Context(), newExecutionHTTPClient(), request)
	if err != nil {
		t.Fatal(err)
	}
	if string(outcome.Content().Bytes()) != "null" {
		t.Fatalf("outcome content = %s", outcome.Content().Bytes())
	}
}

func TestExecuteHTTPRequestMapsNon2xxToToolFailureWithoutBody(t *testing.T) {
	t.Parallel()

	const sensitiveBody = "upstream-secret-body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("{\"message\":\"upstream-secret-body\"}"))
	}))
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := executeHTTPRequest(t.Context(), newExecutionHTTPClient(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		outcome.FailureSource() != invocationdomain.FailureSourceTool ||
		outcome.FailureCode() != invocationdomain.FailureToolError {
		t.Fatalf(
			"outcome = kind=%q source=%q code=%q",
			outcome.Kind(),
			outcome.FailureSource(),
			outcome.FailureCode(),
		)
	}
	if strings.Contains(outcome.Message(), sensitiveBody) {
		t.Fatalf("failure message leaked upstream body: %q", outcome.Message())
	}
	if !strings.Contains(outcome.Message(), "400 Bad Request") {
		t.Fatalf("failure message = %q", outcome.Message())
	}
}

func TestExecuteHTTPRequestRejectsNonJSONSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executeHTTPRequest(t.Context(), newExecutionHTTPClient(), request)
	if err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Fatalf("executeHTTPRequest() error = %v", err)
	}
}

func TestExecutionHTTPClientDoesNotFollowRedirect(t *testing.T) {
	t.Parallel()

	targetCalls := 0
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

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL+"/start",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := executeHTTPRequest(context.Background(), newExecutionHTTPClient(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindFailed {
		t.Fatalf("redirect outcome kind = %q", outcome.Kind())
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", targetCalls)
	}
}
