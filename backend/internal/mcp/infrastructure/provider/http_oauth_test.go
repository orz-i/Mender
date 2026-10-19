package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

type fixtureOAuthTokens struct {
	ref      credentialdomain.Reference
	resource string
	calls    atomic.Int32
}

func (f *fixtureOAuthTokens) AccessToken(_ context.Context, ref credentialdomain.Reference, resource string) (string, error) {
	f.calls.Add(1)
	if ref != f.ref || resource != f.resource {
		return "", errOAuthFixture
	}
	return "fixture-private-access-token", nil
}

type fixtureOAuthErr string

func (f fixtureOAuthErr) Error() string { return string(f) }

const errOAuthFixture fixtureOAuthErr = "unexpected OAuth token request"

func TestOAuthHTTPConnectionAuthenticatesOnlyConfiguredMCPResource(t *testing.T) {
	var accepted atomic.Int32
	server := mcp.NewServer(&mcp.Implementation{Name: "oauth-fixture", Version: "1.0"}, nil)
	addEchoTool(server)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-private-access-token" {
			http.Error(w, "missing OAuth authorization", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Test") != "header" {
			http.Error(w, "missing custom header", http.StatusBadRequest)
			return
		}
		accepted.Add(1)
		mcpHandler.ServeHTTP(w, r)
	}))
	defer local.Close()
	ref, _ := credentialdomain.NewReference("oauth", "work")
	source := &fixtureOAuthTokens{ref: ref, resource: local.URL + "/mcp"}
	connection, err := ConnectHTTP(context.Background(), HTTPConfig{
		Endpoint: local.URL + "/mcp", Headers: map[string]string{"X-Test": "header"},
		OAuthCredentialRef: ref, OAuthTokens: source,
		RequiredCredentials: []credentialdomain.Reference{ref},
	})
	if err != nil {
		t.Fatalf("ConnectHTTP: %v", err)
	}
	defer func() { _ = connection.Close() }()
	if connection.oauthCredentialRef != ref {
		t.Fatal("connected MCP session forgot OAuth Credential proof")
	}
	result, err := connection.session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "echo", Arguments: json.RawMessage(`{"value":"transport"}`),
	})
	if err != nil || result == nil || result.StructuredContent == nil {
		t.Fatalf("OAuth upstream Tool call failed: %+v %v", result, err)
	}
	if source.calls.Load() < 2 || accepted.Load() < 2 {
		t.Fatal("MCP OAuth header missing on initialization or Tool call")
	}
}

func TestOAuthHTTPTransportRejectsOtherPathsOriginsAndRedirects(t *testing.T) {
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Store(true)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	var attempted atomic.Int32
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempted.Add(1)
		http.Redirect(w, r, target.URL+"/anything", http.StatusTemporaryRedirect)
	}))
	defer sourceServer.Close()
	ref, _ := credentialdomain.NewReference("oauth", "work")
	tokens := &fixtureOAuthTokens{ref: ref, resource: sourceServer.URL + "/mcp"}
	client, err := httpClientWithOAuth(nil, sourceServer.URL+"/mcp", ref, tokens)
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{sourceServer.URL + "/other", target.URL + "/mcp"} {
		response, err := client.Get(link)
		if response != nil {
			_ = response.Body.Close()
		}
		if err == nil {
			t.Fatalf("OAuth transport accepted off-resource URL %s", link)
		}
	}
	if attempted.Load() != 0 || tokens.calls.Load() != 0 || leaked.Load() {
		t.Fatal("OAuth token was looked up or leaked before resource scope check")
	}
	response, err := client.Get(sourceServer.URL + "/mcp")
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("OAuth transport allowed resource redirect: %v", err)
	}
	if leaked.Load() {
		t.Fatal("OAuth bearer token followed a cross-origin redirect")
	}
}

func TestOAuthTransportWithoutSourceFailsClosed(t *testing.T) {
	ref, _ := credentialdomain.NewReference("oauth", "work")
	if _, err := httpClientWithOAuth(nil, "https://mcp.example.test/mcp", ref, nil); err == nil {
		t.Fatal("MCP OAuth silently downgraded to an unauthenticated client")
	}
}
