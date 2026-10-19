package oauthflow

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

type memoryOAuthRepo struct {
	mu    sync.Mutex
	items map[credentialdomain.Reference]credentialdomain.Credential
}

func (r *memoryOAuthRepo) Create(_ context.Context, c credentialdomain.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[c.Reference()]; exists {
		return credentialapplication.ErrReferenceConflict
	}
	r.items[c.Reference()] = c
	return nil
}
func (r *memoryOAuthRepo) Save(_ context.Context, c credentialdomain.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[c.Reference()]; !exists {
		return credentialapplication.ErrCredentialNotFound
	}
	r.items[c.Reference()] = c
	return nil
}
func (r *memoryOAuthRepo) Get(_ context.Context, ref credentialdomain.Reference) (credentialdomain.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.items[ref]
	if !ok {
		return credentialdomain.Credential{}, credentialapplication.ErrCredentialNotFound
	}
	return c, nil
}
func (r *memoryOAuthRepo) List(context.Context) ([]credentialdomain.Credential, error) {
	panic("unused List")
}
func (r *memoryOAuthRepo) Delete(context.Context, credentialdomain.Reference) error {
	panic("unused Delete")
}

type memoryOAuthSecrets struct {
	mu     sync.Mutex
	values map[credentialdomain.Reference][]byte
}

func (s *memoryOAuthSecrets) Put(_ context.Context, ref credentialdomain.Reference, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[ref] = append([]byte(nil), value...)
	return nil
}
func (s *memoryOAuthSecrets) Get(_ context.Context, ref credentialdomain.Reference) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	val, ok := s.values[ref]
	if !ok {
		return nil, credentialapplication.ErrSecretNotFound
	}
	return append([]byte(nil), val...), nil
}
func (s *memoryOAuthSecrets) Delete(_ context.Context, ref credentialdomain.Reference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, ref)
	return nil
}

type testOAuthAS struct {
	url               string
	server            *httptest.Server
	mu                sync.Mutex
	verifierChallenge string
	gotResource       string
	tokenRequests     int
	advertiseDCR      bool
	rootMetadata      bool
}

func newTestOAuthAS(t *testing.T) *testOAuthAS {
	t.Helper()
	f := &testOAuthAS{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			if !f.rootMetadata {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer resource_metadata=%q", f.url+"/prm"))
			}
			http.Error(w, "authorization required", http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource":
			if !f.rootMetadata {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": f.url, "authorization_servers": []string{f.url}, "scopes_supported": []string{"read"}})
		case "/prm":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": f.url + "/mcp", "authorization_servers": []string{f.url}, "scopes_supported": []string{"read"}})
		case "/.well-known/oauth-authorization-server":
			w.Header().Set("Content-Type", "application/json")
			metadata := map[string]any{
				"issuer": f.url, "authorization_endpoint": f.url + "/authorize", "token_endpoint": f.url + "/token",
				"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"},
				"code_challenge_methods_supported": []string{"S256"}, "scopes_supported": []string{"read", "offline_access"},
				"token_endpoint_auth_methods_supported":          []string{"none"},
				"authorization_response_iss_parameter_supported": true,
			}
			if f.advertiseDCR {
				metadata["registration_endpoint"] = f.url + "/register"
			}
			_ = json.NewEncoder(w).Encode(metadata)
		case "/register":
			if !f.advertiseDCR {
				http.NotFound(w, r)
				return
			}
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request["application_type"] != "native" {
				http.Error(w, "DCR requires native application", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"client_id": "mender-dcr-client", "token_endpoint_auth_method": "none",
				"redirect_uris": request["redirect_uris"], "application_type": "native",
			})
		case "/authorize":
			q := r.URL.Query()
			audience := f.url + "/mcp"
			if f.rootMetadata {
				audience = f.url
			}
			if q.Get("code_challenge_method") != "S256" || q.Get("state") == "" || q.Get("resource") != audience {
				http.Error(w, "bad authorization parameters", http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.verifierChallenge = q.Get("code_challenge")
			f.gotResource = q.Get("resource")
			f.mu.Unlock()
			redirect, err := url.Parse(q.Get("redirect_uri"))
			if err != nil {
				http.Error(w, "bad redirect", 400)
				return
			}
			v := redirect.Query()
			v.Set("code", "fake-valid-code")
			v.Set("state", q.Get("state"))
			v.Set("iss", f.url)
			redirect.RawQuery = v.Encode()
			//nolint:gosec // Fixture IdP redirects to the loopback URI explicitly supplied by our test client.
			http.Redirect(w, r, redirect.String(), http.StatusFound)
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", 400)
				return
			}
			f.mu.Lock()
			challenge := f.verifierChallenge
			f.tokenRequests++
			f.mu.Unlock()
			digest := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
			computed := base64.RawURLEncoding.EncodeToString(digest[:])
			audience := f.url + "/mcp"
			if f.rootMetadata {
				audience = f.url
			}
			if r.PostForm.Get("code") != "fake-valid-code" || r.PostForm.Get("resource") != audience || computed != challenge {
				http.Error(w, "invalid token request", 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-access-token", "refresh_token": "fixture-refresh-token", "token_type": "Bearer", "expires_in": 3600, "scope": "read"})
		default:
			http.NotFound(w, r)
		}
	}))
	f.url = f.server.URL
	t.Cleanup(f.server.Close)
	return f
}

func newLoopbackTestManager(t *testing.T) (*Manager, *memoryOAuthRepo, *memoryOAuthSecrets) {
	t.Helper()
	repo := &memoryOAuthRepo{items: map[credentialdomain.Reference]credentialdomain.Credential{}}
	secrets := &memoryOAuthSecrets{values: map[credentialdomain.Reference][]byte{}}
	manager := NewManager(repo, secrets, nil, Options{StartTimeout: 5 * time.Second, FlowTTL: 20 * time.Second})
	t.Cleanup(func() { _ = manager.Close() })
	return manager, repo, secrets
}

func awaitTerminalFlow(t *testing.T, m *Manager, id string) FlowView {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		view, err := m.Status(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status != FlowPending {
			return view
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("OAuth flow did not reach a terminal state")
	return FlowView{}
}

func TestOAuthFlowAuthorizationCodePKCEAndSecretIsolation(t *testing.T) {
	fixture := newTestOAuthAS(t)
	manager, repo, secrets := newLoopbackTestManager(t)
	start, err := manager.Start(context.Background(), StartRequest{
		Namespace: "oauth", Name: "work", DisplayName: "OAuth Work", MCPEndpoint: fixture.url + "/mcp", ClientID: "public-desktop", Scopes: []string{"read"},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if start.FlowID == "" || !strings.HasPrefix(start.AuthorizationURL, fixture.url+"/authorize?") {
		t.Fatalf("invalid authorization output %+v", start)
	}
	res, err := http.Get(start.AuthorizationURL)
	if err != nil {
		t.Fatalf("browser callback: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authorization completion status: %d", res.StatusCode)
	}
	final := awaitTerminalFlow(t, manager, start.FlowID)
	if final.Status != FlowConnected || final.Credential == nil {
		t.Fatalf("OAuth not connected: %+v", final)
	}
	ref, _ := credentialdomain.NewReference("oauth", "work")
	credential, err := repo.Get(context.Background(), ref)
	if err != nil || credential.OAuthMetadata() == nil || credential.OAuthMetadata().Issuer != fixture.url {
		t.Fatalf("OAuth metadata not saved: %+v %v", credential.OAuthMetadata(), err)
	}
	stored, err := secrets.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), "fixture-refresh-token") {
		t.Fatal("refresh token not stored in SecretStore")
	}
	viewJSON, err := json.Marshal(final)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(viewJSON), "fixture-access-token") || strings.Contains(string(viewJSON), "fixture-refresh-token") {
		t.Fatal("OAuth flow result exposed bearer/refresh token")
	}
	fixture.mu.Lock()
	calls := fixture.tokenRequests
	fixture.mu.Unlock()
	if calls != 1 {
		t.Fatalf("token exchanges=%d, want 1", calls)
	}
}

func TestOAuthFlowRejectsInvalidStateWithoutSecret(t *testing.T) {
	fixture := newTestOAuthAS(t)
	manager, _, secrets := newLoopbackTestManager(t)
	result, err := manager.Start(context.Background(), StartRequest{
		Namespace: "oauth", Name: "badstate", DisplayName: "Bad state", MCPEndpoint: fixture.url + "/mcp", ClientID: "public-desktop",
	})
	if err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(result.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	callback := authorize.Query().Get("redirect_uri")
	bad, err := url.Parse(callback)
	if err != nil {
		t.Fatal(err)
	}
	q := bad.Query()
	q.Set("state", "wrong")
	q.Set("code", "fake-valid-code")
	bad.RawQuery = q.Encode()
	response, err := http.Get(bad.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("mismatched state accepted, status %d", response.StatusCode)
	}
	view := awaitTerminalFlow(t, manager, result.FlowID)
	if view.Status == FlowConnected || view.Credential != nil {
		t.Fatalf("invalid state authorized: %+v", view)
	}
	ref, _ := credentialdomain.NewReference("oauth", "badstate")
	if _, err := secrets.Get(context.Background(), ref); err == nil {
		t.Fatal("invalid state stored OAuth tokens")
	}
}

func TestOAuthFlowCancellationClosesLoopbackAndDoesNotPersistTokens(t *testing.T) {
	fixture := newTestOAuthAS(t)
	manager, _, _ := newLoopbackTestManager(t)
	result, err := manager.Start(context.Background(), StartRequest{
		Namespace: "oauth", Name: "cancel", DisplayName: "Cancelled", MCPEndpoint: fixture.url + "/mcp", ClientID: "public-desktop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Cancel(context.Background(), result.FlowID); err != nil {
		t.Fatal(err)
	}
	view, err := manager.Status(context.Background(), result.FlowID)
	if err != nil || view.Status != FlowCancelled {
		t.Fatalf("cancellation state=%+v %v", view, err)
	}
	if err := manager.Cancel(context.Background(), result.FlowID); err == nil {
		t.Fatal("cancel allowed on completed flow")
	}
}

// A remote MCP server that advertises RFC 7591 registration can connect
// without the user knowing a client ID in advance.
func TestOAuthDynamicClientRegistrationWithNativeRedirect(t *testing.T) {
	fixture := newTestOAuthAS(t)
	fixture.advertiseDCR = true
	manager, repo, secrets := newLoopbackTestManager(t)
	start, err := manager.Start(context.Background(), StartRequest{
		Namespace: "oauth", Name: "dcr", DisplayName: "DCR account", MCPEndpoint: fixture.url + "/mcp",
	})
	if err != nil {
		t.Fatalf("DCR registration failed: %v", err)
	}
	parsed, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Query().Get("client_id"); got != "mender-dcr-client" {
		t.Fatalf("Dynamic client registration returned client ID %q", got)
	}
	response, err := http.Get(start.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("browser status %d", response.StatusCode)
	}
	view := awaitTerminalFlow(t, manager, start.FlowID)
	if view.Status != FlowConnected || view.Credential == nil {
		t.Fatalf("DCR connection status = %+v", view)
	}
	ref, _ := credentialdomain.NewReference("oauth", "dcr")
	credential, err := repo.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if credential.OAuthMetadata().ClientID != "mender-dcr-client" {
		t.Fatal("DCR client identity was not persisted")
	}
	secret, err := secrets.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	var bundle tokenBundle
	if err := json.Unmarshal(secret, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.ClientID != "mender-dcr-client" || bundle.AccessToken != "fixture-access-token" {
		t.Fatal("DCR tokens and client credentials not stored together in native store")
	}
}

func TestOAuthDynamicRegistrationWithoutServerSupportFailsClosed(t *testing.T) {
	fixture := newTestOAuthAS(t)
	manager, repo, secrets := newLoopbackTestManager(t)
	_, err := manager.Start(context.Background(), StartRequest{
		Namespace: "oauth", Name: "dcr", DisplayName: "DCR account", MCPEndpoint: fixture.url + "/mcp",
	})
	if err == nil {
		t.Fatal("missing advertised registration mechanism was silently accepted")
	}
	ref, _ := credentialdomain.NewReference("oauth", "dcr")
	if _, err := repo.Get(context.Background(), ref); err == nil {
		t.Fatal("unsupported DCR created a credential")
	}
	if _, err := secrets.Get(context.Background(), ref); err == nil {
		t.Fatal("unsupported DCR stored tokens")
	}
}

// Root-level protected-resource metadata may legitimately identify the origin
// as token audience even though the chosen MCP HTTP endpoint has /mcp path.
func TestOAuthRootResourceAudienceIsBoundToExactMCPEndpoint(t *testing.T) {
	fixture := newTestOAuthAS(t)
	fixture.rootMetadata = true
	manager, repo, _ := newLoopbackTestManager(t)
	endpoint := fixture.url + "/mcp"
	start, err := manager.Start(context.Background(), StartRequest{
		Namespace: "oauth", Name: "root", DisplayName: "Root metadata", MCPEndpoint: endpoint, ClientID: "public-client",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(start.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	result := awaitTerminalFlow(t, manager, start.FlowID)
	if result.Status != FlowConnected {
		t.Fatalf("root metadata OAuth flow failed: %+v", result)
	}
	ref, _ := credentialdomain.NewReference("oauth", "root")
	credential, err := repo.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	metadata := credential.OAuthMetadata()
	if metadata == nil || metadata.Resource != fixture.url || metadata.MCPEndpoint != endpoint {
		t.Fatalf("audience/transport endpoint were conflated: %+v", metadata)
	}
	if token, err := manager.AccessToken(context.Background(), ref, endpoint); err != nil || token != "fixture-access-token" {
		t.Fatalf("bound HTTP endpoint cannot access its token: %q %v", token, err)
	}
	if _, err := manager.AccessToken(context.Background(), ref, fixture.url+"/different-mcp"); err == nil {
		t.Fatal("origin-scoped OAuth audience allowed another configured path")
	}
}
