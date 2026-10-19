package oauthflow

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
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

// --- credential application fakes ---

type memoryRepository struct {
	mu    sync.Mutex
	items map[credentialdomain.Reference]credentialdomain.Credential
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{items: make(map[credentialdomain.Reference]credentialdomain.Credential)}
}

func (r *memoryRepository) Create(_ context.Context, credential credentialdomain.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[credential.Reference()]; ok {
		return credentialapplication.ErrReferenceConflict
	}
	r.items[credential.Reference()] = credential
	return nil
}

func (r *memoryRepository) Save(_ context.Context, credential credentialdomain.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[credential.Reference()]; !ok {
		return credentialapplication.ErrCredentialNotFound
	}
	r.items[credential.Reference()] = credential
	return nil
}

func (r *memoryRepository) Get(
	_ context.Context,
	ref credentialdomain.Reference,
) (credentialdomain.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	credential, ok := r.items[ref]
	if !ok {
		return credentialdomain.Credential{}, credentialapplication.ErrCredentialNotFound
	}
	return credential, nil
}

func (r *memoryRepository) List(context.Context) ([]credentialdomain.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]credentialdomain.Credential, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, nil
}

func (r *memoryRepository) Delete(_ context.Context, ref credentialdomain.Reference) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[ref]; !ok {
		return credentialapplication.ErrCredentialNotFound
	}
	delete(r.items, ref)
	return nil
}

type memorySecrets struct {
	mu       sync.Mutex
	values   map[credentialdomain.Reference][]byte
	putCalls int
}

func newMemorySecrets() *memorySecrets {
	return &memorySecrets{values: make(map[credentialdomain.Reference][]byte)}
}

func (s *memorySecrets) Put(
	_ context.Context,
	ref credentialdomain.Reference,
	value []byte,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putCalls++
	s.values[ref] = append([]byte(nil), value...)
	return nil
}

func (s *memorySecrets) Get(
	_ context.Context,
	ref credentialdomain.Reference,
) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[ref]
	if !ok {
		return nil, credentialapplication.ErrSecretNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *memorySecrets) Delete(_ context.Context, ref credentialdomain.Reference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[ref]; !ok {
		return credentialapplication.ErrSecretNotFound
	}
	delete(s.values, ref)
	return nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

// --- fake MCP protected resource plus authorization server ---

const (
	testClientID        = "desktop"
	testAccessToken     = "at-secret-123"
	testRefreshToken    = "rt-secret-456"
	testRequestedScopes = "mcp:tools"
)

type fakeOAuthServer struct {
	resource        *httptest.Server
	issuer          *httptest.Server
	endpoint        string
	issuerURL       string
	scopesSupported []string

	// tokenEndpointRejectsGrant makes the token endpoint answer 400 with a
	// body containing credential-shaped values: it exercised sanitization.
	tokenEndpointRejectsGrant bool

	// Poisoned metadata knobs for security tests; set before Start.
	tokenEndpointURL       string
	authorizeEndpointURL   string
	prmResourceMetadataURL string

	prmMu                   sync.Mutex
	prmAuthorizationServers []string

	authMu             sync.Mutex
	authorizeRequests  []url.Values
	tokenRequests      []url.Values
	authorizationCodes map[string]string
	redirectURIs       map[string]bool
	codeCounter        int
}

func newFakeOAuthServer(t *testing.T) *fakeOAuthServer {
	t.Helper()
	fake := &fakeOAuthServer{
		scopesSupported:    []string{testRequestedScopes, "mcp:extra"},
		authorizationCodes: make(map[string]string),
		redirectURIs:       make(map[string]bool),
	}
	fake.resource = httptest.NewServer(http.HandlerFunc(fake.serveResource))
	fake.issuer = httptest.NewServer(http.HandlerFunc(fake.serveIssuer))
	fake.issuerURL = fake.issuer.URL
	fake.endpoint = fake.resource.URL + "/mcp"
	t.Cleanup(fake.Close)
	return fake
}

func (f *fakeOAuthServer) Close() {
	f.resource.Close()
	f.issuer.Close()
}

func (f *fakeOAuthServer) metadataURL() string {
	if f.prmResourceMetadataURL != "" {
		return f.prmResourceMetadataURL
	}
	return f.issuerURL + "/oauth/protected-resource"
}

func (f *fakeOAuthServer) tokenEndpoint() string {
	if f.tokenEndpointURL != "" {
		return f.tokenEndpointURL
	}
	return f.issuerURL + "/token"
}

func (f *fakeOAuthServer) authorizeEndpoint() string {
	if f.authorizeEndpointURL != "" {
		return f.authorizeEndpointURL
	}
	return f.issuerURL + "/authorize"
}

func (f *fakeOAuthServer) serveResource(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/mcp" {
		w.Header().Set(
			"WWW-Authenticate",
			fmt.Sprintf(`Bearer resource_metadata=%q, resource="scope"`, f.metadataURL()),
		)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
}

func (f *fakeOAuthServer) serveIssuer(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/oauth/protected-resource", "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp":
		writeJSON(w, http.StatusOK, map[string]any{
			"resource":              f.endpoint,
			"authorization_servers": f.authorizationServers(),
			"scopes_supported":      f.scopesSupported,
		})
	case "/.well-known/oauth-authorization-server":
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                f.issuerURL,
			"authorization_endpoint":                f.authorizeEndpoint(),
			"token_endpoint":                        f.tokenEndpoint(),
			"token_endpoint_auth_methods_supported": []string{"none"},
			"authorization_response_iss_parameter_supported": true,
			"code_challenge_methods_supported":               []string{"S256"},
			"scopes_supported":                               f.scopesSupported,
		})
	case "/authorize":
		f.serveAuthorize(w, r)
	case "/token":
		f.serveToken(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
	}
}

func (f *fakeOAuthServer) authorizationServers() []string {
	f.prmMu.Lock()
	defer f.prmMu.Unlock()
	if len(f.prmAuthorizationServers) > 0 {
		return append([]string(nil), f.prmAuthorizationServers...)
	}
	return []string{f.issuerURL}
}

func (f *fakeOAuthServer) serveAuthorize(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	f.authMu.Lock()
	f.authorizeRequests = append(f.authorizeRequests, query.Clone())
	f.authMu.Unlock()

	if badAuthorisation(query, f) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	redirect, err := url.Parse(query.Get("redirect_uri"))
	if err != nil ||
		redirect.Scheme != "http" ||
		!isLoopbackHost(redirect.Hostname()) ||
		redirect.Path != callbackPath {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	f.authMu.Lock()
	f.codeCounter++
	code := fmt.Sprintf("code-%d", f.codeCounter)
	f.authorizationCodes[code] = query.Get("code_challenge")
	f.redirectURIs[code] = true
	f.authMu.Unlock()

	target := fmt.Sprintf(
		"%s?code=%s&state=%s&iss=%s",
		query.Get("redirect_uri"),
		url.QueryEscape(code),
		url.QueryEscape(query.Get("state")),
		url.QueryEscape(f.issuerURL),
	)
	w.Header().Set("Location", target)
	w.WriteHeader(http.StatusFound)
}

func badAuthorisation(query url.Values, fake *fakeOAuthServer) bool {
	if query.Get("response_type") != "code" ||
		query.Get("client_id") != testClientID ||
		query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") == "" ||
		query.Get("state") == "" ||
		query.Get("resource") != fake.endpoint {
		return true
	}
	requested := strings.Fields(query.Get("scope"))
	if len(requested) == 0 {
		return false
	}
	for _, scope := range requested {
		if !contains(fake.scopesSupported, scope) {
			return true
		}
	}
	return false
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (f *fakeOAuthServer) serveToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	f.authMu.Lock()
	f.tokenRequests = append(f.tokenRequests, r.PostForm.Clone())
	code := r.PostForm.Get("code")
	challenge, ok := f.authorizationCodes[code]
	f.authMu.Unlock()

	if !ok ||
		r.PostForm.Get("grant_type") != "authorization_code" ||
		r.PostForm.Get("resource") != f.endpoint {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
		return
	}
	if presented := r.PostForm.Get("client_id"); presented != "" && presented != testClientID {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_client"})
		return
	}
	verifier := r.PostForm.Get("code_verifier")
	sum := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
		return
	}

	f.authMu.Lock()
	delete(f.authorizationCodes, code)
	f.authMu.Unlock()

	if state := r.PostForm.Get("redirect_uri"); state != "" {
		// x/oauth2 may or may not echo redirect_uri; never assert its presence.
		_ = state
	}

	if f.tokenEndpointRejectsGrant {
		// Error bodies can incidentally echo credential values; the flow must
		// never project them into status messages.
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":             "invalid_grant",
			"error_description": "the code was already used",
			"access_token":      testAccessToken,
			"refresh_token":     testRefreshToken,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  testAccessToken,
		"refresh_token": testRefreshToken,
		"expires_in":    90,
		"token_type":    "Bearer",
		"scope":         strings.Join(f.scopesSupported, " "),
	})
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]any) {
	blob, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "encoding failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(blob)
}

// --- helpers shared by flow tests ---

// browserVisit follows one authorization URL exactly like a user browser:
// it fetches the URL and then the redirect the server hands back.
func browserVisit(t *testing.T, authorizationURL string) (string, string) {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	first, err := client.Get(authorizationURL)
	if err != nil {
		t.Fatalf("authorization URL fetch failed: %v", err)
	}
	defer first.Body.Close()
	location := first.Header.Get("Location")
	target := authorizationURL
	if first.StatusCode == http.StatusFound && location != "" {
		target = location
	}
	second, err := client.Get(target)
	if err != nil {
		t.Fatalf("callback fetch to %s failed: %v", target, err)
	}
	defer second.Body.Close()
	var body strings.Builder
	readBuffer := make([]byte, 4096)
	for {
		n, readErr := second.Body.Read(readBuffer)
		body.Write(readBuffer[:n])
		if readErr != nil {
			break
		}
	}
	return target, body.String()
}

func waitForFlowStatus(
	t *testing.T,
	manager *Manager,
	flowID string,
	wanted FlowStatus,
) FlowView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last FlowView
	var lastErr error
	for time.Now().Before(deadline) {
		last, lastErr = manager.Status(context.Background(), flowID)
		if lastErr == nil && last.Status == wanted {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("flow %s: status is %v (view %+v, err %v), want %s", flowID, last.Status, last, lastErr, wanted)
	return FlowView{}
}

func waitForListenerClosed(t *testing.T, port int) {
	t.Helper()
	address := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("loopback listener %s is still accepting connections", address)
}

func newTestManager(repo *memoryRepository, secrets *memorySecrets, options Options) *Manager {
	return NewManager(repo, secrets, fixedClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}, options)
}

func mustReference(namespace, name string) credentialdomain.Reference {
	ref, err := credentialdomain.NewReference(namespace, name)
	if err != nil {
		panic(fmt.Sprintf("invalid reference %s/%s: %v", namespace, name, err))
	}
	return ref
}

func baseStartRequest(endpoint string) StartRequest {
	return StartRequest{
		Namespace:   "oauth",
		Name:        "work",
		DisplayName: "Work MCP",
		MCPEndpoint: endpoint,
		ClientID:    testClientID,
		Scopes:      []string{testRequestedScopes},
	}
}
