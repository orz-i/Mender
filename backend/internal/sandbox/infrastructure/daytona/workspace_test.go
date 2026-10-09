package daytona

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func execWorkspace(t *testing.T, s *session, cap sandboxdomain.Capability, args string) map[string]any {
	t.Helper()
	input, err := invocationdomain.NewJSONValue([]byte(args))
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ExecuteWorkspace(context.Background(), cap, input)
	if err != nil || result.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("%s: outcome %s: %v", cap, result.Kind(), err)
	}
	var output map[string]any
	if err := json.Unmarshal(result.Content().Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	return output
}
func TestDaytonaWorkspaceReadSearchEditShellShareOneRemoteFS(t *testing.T) {
	remote := &fakeRemote{root: "/home/remote", files: map[string][]byte{}}
	client := &fakeRemoteClient{remote: remote}
	s := &session{client: client, remote: remote, root: remote.root, template: validTemplate()}
	created := execWorkspace(t, s, sandboxdomain.CapabilityEdit, `{"path":"draft.txt","operation":"create","content":"hello remote\n"}`)
	if created["operation"] != "create" {
		t.Fatalf("create %+v", created)
	}
	read := execWorkspace(t, s, sandboxdomain.CapabilityRead, `{"path":"draft.txt"}`)
	if read["content"] != "hello remote" {
		t.Fatalf("read %+v", read)
	}
	search := execWorkspace(t, s, sandboxdomain.CapabilitySearch, `{"mode":"content","query":"remote"}`)
	if len(search["matches"].([]any)) != 1 {
		t.Fatalf("search %+v", search)
	}
	renamed := execWorkspace(t, s, sandboxdomain.CapabilityEdit, `{"path":"draft.txt","operation":"replace","edits":[{"oldText":"hello","newText":"goodbye"}]}`)
	if renamed["replacements"] != float64(1) {
		t.Fatalf("replace %+v", renamed)
	}
	updated := execWorkspace(t, s, sandboxdomain.CapabilityRead, `{"path":"draft.txt"}`)
	if updated["content"] != "goodbye remote" {
		t.Fatalf("not same remote file %+v", updated)
	}
	shell := execWorkspace(t, s, sandboxdomain.CapabilityShell, `{"command":"pwd"}`)
	if shell["stdout"] != "approved remote execution" {
		t.Fatalf("remote shell %+v", shell)
	}
	deleted := execWorkspace(t, s, sandboxdomain.CapabilityEdit, `{"path":"draft.txt","operation":"delete"}`)
	if deleted["operation"] != "delete" {
		t.Fatalf("delete %+v", deleted)
	}
	if _, ok := remote.files["/home/remote/draft.txt"]; ok {
		t.Fatal("remote delete did not modify same filesystem")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if client.closeCalls != 1 {
		t.Fatal("SDK client not closed")
	}
}
func TestDaytonaWorkspaceRejectsTraversalUnknownAndOversized(t *testing.T) {
	remote := &fakeRemote{root: "/home/remote", files: map[string][]byte{"/home/remote/big.txt": []byte(strings.Repeat("x", maxReadBytes+1))}}
	s := &session{client: &fakeRemoteClient{remote: remote}, remote: remote, root: remote.root, template: validTemplate()}
	for _, bad := range []string{`{"path":"../etc/passwd"}`, `{"path":"/etc/passwd"}`, `{"path":"unsafe; touch x"}`, `{"path":"sub//file"}`, `{"path":"bad\\path"}`} {
		value, err := invocationdomain.NewJSONValue([]byte(bad))
		if err != nil {
			t.Fatal(err)
		}
		outcome, runErr := s.ExecuteWorkspace(context.Background(), sandboxdomain.CapabilityRead, value)
		if runErr == nil && outcome.Kind() == invocationdomain.OutcomeKindComplete {
			t.Fatalf("unsafe path accepted %s", bad)
		}
	}
	oversized, err := invocationdomain.NewJSONValue([]byte(`{"path":"big.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	outcome, execErr := s.ExecuteWorkspace(context.Background(), sandboxdomain.CapabilityRead, oversized)
	if execErr == nil && outcome.Kind() == invocationdomain.OutcomeKindComplete {
		t.Fatal("oversized content accepted")
	}
	if _, err := s.ExecuteWorkspace(context.Background(), "unknown", oversized); !errors.Is(err, sandboxdomain.ErrCapabilityUnavailable) {
		t.Fatalf("unknown capability %v", err)
	}
	if remote.execCalls == 0 {
		t.Fatal("file scope checks were skipped")
	}
}
func TestDaytonaSDKVendorTransportDenySSRF(t *testing.T) {
	calls := 0
	transport := vendorTransport{base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	for _, url := range []string{"http://app.daytona.io/api", "https://127.0.0.1/internal", "https://app.daytona.io.evil.test", "https://localhost", "https://app.daytona.io:8443/api"} {
		request, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := transport.RoundTrip(request)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err == nil {
			t.Fatalf("SSRF accepted %s", url)
		}
	}
	good, err := http.NewRequest(http.MethodGet, "https://proxy.app.daytona.io/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(good)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil || calls != 1 {
		t.Fatalf("allowed vendor endpoint rejected: %v calls=%d", err, calls)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
