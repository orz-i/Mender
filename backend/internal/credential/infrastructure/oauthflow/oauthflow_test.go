package oauthflow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHappyPathConnectsCredentialWithoutLeaking(t *testing.T) {
	fake := newFakeOAuthServer(t)
	repository := newMemoryRepository()
	secrets := newMemorySecrets()
	manager := newTestManager(repository, secrets, Options{})
	defer manager.Close()

	started, err := manager.Start(context.Background(), baseStartRequest(fake.endpoint))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if started.FlowID == "" || started.AuthorizationURL == "" {
		t.Fatalf("Start result incomplete: %+v", started)
	}

	authorization, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatalf("authorization URL parse: %v", err)
	}
	if authorization.Host != strings.TrimPrefix(fake.issuerURL, "http://") {
		t.Fatalf("authorization server host mismatch: %s", authorization.Host)
	}
	query := authorization.Query()
	if query.Get("response_type") != "code" ||
		query.Get("client_id") != testClientID ||
		query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") == "" ||
		query.Get("state") == "" ||
		query.Get("resource") != fake.endpoint ||
		query.Get("scope") != testRequestedScopes {
		t.Fatalf("authorization request parameters wrong: %v", query)
	}
	redirect, err := url.Parse(query.Get("redirect_uri"))
	if err != nil ||
		redirect.Scheme != "http" ||
		redirect.Hostname() != "127.0.0.1" ||
		redirect.Path != callbackPath {
		t.Fatalf("redirect URI is not the bound loopback callback: %v", query.Get("redirect_uri"))
	}
	port, errPort := strconv.Atoi(redirect.Port())
	if errPort != nil {
		t.Fatalf("callback port parse: %v", errPort)
	}

	// Simulate the user browser completing the grant.
	callback, page := browserVisit(t, started.AuthorizationURL)
	callbackURL, err := url.Parse(callback)
	boundURL, err2 := url.Parse(query.Get("redirect_uri"))
	if err != nil || err2 != nil ||
		callbackURL.Host != boundURL.Host ||
		callbackURL.Path != boundURL.Path {
		t.Fatalf("authorization redirect escaped the bound callback: %s", callback)
	}
	for _, banned := range []string{testAccessToken, testRefreshToken, "code-", query.Get("state")} {
		if strings.Contains(page, banned) {
			t.Fatalf("callback page leaked %q: %s", banned, page)
		}
	}
	if !strings.Contains(page, "completed") {
		t.Fatalf("callback page is not a completion notice: %s", page)
	}

	view := waitForFlowStatus(t, manager, started.FlowID, FlowConnected)
	if view.Credential == nil {
		t.Fatalf("connected flow without credential: %+v", view)
	}
	if view.Credential.Reference != "credential://oauth/work" ||
		view.Credential.DisplayName != "Work MCP" {
		t.Fatalf("credential view wrong: %+v", view.Credential)
	}
	metadata := view.Credential.Metadata
	if metadata.Resource != fake.endpoint ||
		metadata.Issuer != fake.issuerURL ||
		metadata.ClientID != testClientID ||
		metadata.TokenEndpoint != fake.tokenEndpoint() ||
		metadata.Status != "connected" ||
		metadata.ExpiresAt.IsZero() {
		t.Fatalf("OAuth metadata wrong: %s", metadataJSON(t, metadata))
	}
	for _, banned := range []string{testAccessToken, testRefreshToken} {
		if strings.Contains(metadataJSON(t, metadata), banned) {
			t.Fatalf("metadata leaked %q", banned)
		}
	}
	if len(metadata.Scopes) == 0 || !contains(metadata.Scopes, testRequestedScopes) {
		t.Fatalf("granted scopes wrong: %v", metadata.Scopes)
	}

	credential, err := repository.Get(context.Background(), mustReference("oauth", "work"))
	if err != nil {
		t.Fatalf("credential missing from inventory: %v", err)
	}
	if credential.Source() != "oauth" {
		t.Fatalf("credential source wrong: %s", credential.Source())
	}

	stored, err := secrets.Get(context.Background(), credential.Reference())
	if err != nil {
		t.Fatalf("token bundle missing from secret store: %v", err)
	}
	bundle := string(stored)
	for _, wantedSub := range []string{testAccessToken, testRefreshToken, fake.issuerURL, fake.endpoint} {
		if !strings.Contains(bundle, wantedSub) {
			t.Fatalf("token bundle misses %q", wantedSub)
		}
	}

	// PKCE S256 rounding trip: the fake token endpoint only accepts a
	// verifier matching the S256 challenge of the authorization request.
	fake.authMu.Lock()
	exchange := fake.tokenRequests
	fake.authMu.Unlock()
	if len(exchange) == 0 {
		t.Fatal("fake authorization server saw no token exchange")
	}
	if exchange[0].Get("code_verifier") == "" || exchange[0].Get("code") == "" {
		t.Fatalf("token exchange missing PKCE data: %v", exchange[0])
	}
	if exchange[0].Get("resource") != fake.endpoint {
		t.Fatalf("token exchange skipped the resource parameter: %v", exchange[0])
	}
	if clientID := exchange[0].Get("client_id"); clientID != "" && clientID != testClientID {
		t.Fatalf("token exchange client id wrong: %v", exchange[0])
	}

	flowJSON, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{testAccessToken, testRefreshToken} {
		if strings.Contains(string(flowJSON), banned) {
			t.Fatalf("flow view leaked %q: %s", banned, flowJSON)
		}
	}

	waitForListenerClosed(t, port)
}

func TestBadStateFailsTheFlowClosed(t *testing.T) {
	fake := newFakeOAuthServer(t)
	repository := newMemoryRepository()
	secrets := newMemorySecrets()
	manager := newTestManager(repository, secrets, Options{})
	defer manager.Close()

	started, err := manager.Start(context.Background(), baseStartRequest(fake.endpoint))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	view := manager.mustStatus(t, started.FlowID)
	if view.Status != FlowPending || view.AuthorizationURL == "" {
		t.Fatalf("flow should be pending with URL: %+v", view)
	}

	// A hostile local page replays the callback with a forged state.
	forged := forgedCallbackURL(t, started.AuthorizationURL)
	page := fetchPage(t, forged)
	if strings.Contains(page, "forged-by-attacker") || strings.Contains(page, "can close this window") {
		t.Fatalf("forged callback must not echo state or complete: %s", page)
	}

	terminal := waitForFlowStatus(t, manager, started.FlowID, FlowFailed)
	if terminal.Message != "the authorization callback state did not match" {
		t.Fatalf("unexpected failure message: %q", terminal.Message)
	}
	if terminal.Credential != nil || terminal.AuthorizationURL != "" {
		t.Fatalf("failed flow kept credential or URL: %+v", terminal)
	}
	if secrets.storageLength() != 0 || repository.length() != 0 {
		t.Fatal("a failed flow must not persist tokens or credentials")
	}
	waitForListenerClosed(t, manager.mustPort(t, started.FlowID))
}

func TestCancelClosesFlowAndListener(t *testing.T) {
	fake := newFakeOAuthServer(t)
	repository := newMemoryRepository()
	secrets := newMemorySecrets()
	manager := newTestManager(repository, secrets, Options{})
	defer manager.Close()

	started, err := manager.Start(context.Background(), baseStartRequest(fake.endpoint))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	port := manager.mustPort(t, started.FlowID)

	if err := manager.Cancel(context.Background(), started.FlowID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	view := waitForFlowStatus(t, manager, started.FlowID, FlowCancelled)
	if view.AuthorizationURL != "" || view.Credential != nil {
		t.Fatalf("cancelled flow kept URL or credential: %+v", view)
	}
	if err := manager.Cancel(context.Background(), started.FlowID); !errors.Is(err, ErrFlowNotPending) {
		t.Fatalf("double cancel accepted: %v", err)
	}
	if secrets.storageLength() != 0 || repository.length() != 0 {
		t.Fatal("cancelled flow persisted tokens")
	}
	waitForListenerClosed(t, port)

	if _, err := manager.Status(context.Background(), "mfo_unknown"); !errors.Is(err, ErrFlowNotFound) {
		t.Fatalf("unknown flow status: %v", err)
	}
	if err := manager.Cancel(context.Background(), "mfo_unknown"); !errors.Is(err, ErrFlowNotFound) {
		t.Fatalf("unknown flow cancel: %v", err)
	}
}

func TestFlowExpiry(t *testing.T) {
	fake := newFakeOAuthServer(t)
	repository := newMemoryRepository()
	secrets := newMemorySecrets()
	manager := newTestManager(repository, secrets, Options{FlowTTL: 400 * time.Millisecond})
	defer manager.Close()

	started, err := manager.Start(context.Background(), baseStartRequest(fake.endpoint))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	port := manager.mustPort(t, started.FlowID)

	view := waitForFlowStatus(t, manager, started.FlowID, FlowExpired)
	if view.Message != "authorization flow expired before completion" {
		t.Fatalf("expiry message wrong: %q", view.Message)
	}
	if secrets.storageLength() != 0 || repository.length() != 0 {
		t.Fatal("expired flow persisted tokens")
	}
	waitForListenerClosed(t, port)
}

func TestStartValidationIsFailClosed(t *testing.T) {
	fake := newFakeOAuthServer(t)
	repository := newMemoryRepository()
	secrets := newMemorySecrets()
	manager := newTestManager(repository, secrets, Options{})
	defer manager.Close()

	tests := []struct {
		name     string
		request  StartRequest
		contains string
	}{
		{
			name:     "ssh namespace",
			request:  StartRequest{Namespace: "ssh", Name: "keys", DisplayName: "S", MCPEndpoint: fake.endpoint, ClientID: "c"},
			contains: "ssh",
		},
		{
			name: "insecure remote endpoint",
			request: StartRequest{
				Namespace: "oauth", Name: "work", DisplayName: "W",
				MCPEndpoint: "http://insecure.example.test/mcp", ClientID: "c",
			},
			contains: "loopback",
		},
		{
			name: "endpoint is not a URL",
			request: StartRequest{
				Namespace: "oauth", Name: "work", DisplayName: "W",
				MCPEndpoint: "not a url", ClientID: "c",
			},
			contains: "absolute",
		},
		{
			name: "both client modes",
			request: StartRequest{
				Namespace: "oauth", Name: "work", DisplayName: "W",
				MCPEndpoint:         fake.endpoint,
				ClientID:            "c",
				ClientIDMetadataURL: "https://client.example.test/mcp-client.json",
			},
			contains: "mutually exclusive",
		},
		{
			name:     "missing client registration",
			request:  StartRequest{Namespace: "oauth", Name: "work", DisplayName: "W", MCPEndpoint: fake.endpoint},
			contains: "client ID",
		},
		{
			name: "secret without client id",
			request: StartRequest{
				Namespace: "oauth", Name: "work", DisplayName: "W",
				MCPEndpoint: fake.endpoint, ClientSecret: "secret",
			},
			contains: "pre-registered",
		},
		{
			name: "empty namespace",
			request: StartRequest{
				Name: "work", DisplayName: "W", MCPEndpoint: fake.endpoint, ClientID: "c",
			},
			contains: "namespace",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := manager.Start(context.Background(), test.request)
			if err == nil {
				t.Fatal("Start accepted an invalid request")
			}
			if !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error %q does not mention %q", err, test.contains)
			}
		})
	}
	if repository.length() != 0 || secrets.storageLength() != 0 {
		t.Fatal("invalid requests persisted state")
	}
}

func TestInsecureMetadataEndpointsFailClosed(t *testing.T) {
	scheme := func(t *testing.T) (fake *fakeOAuthServer, endpoint string) {
		t.Helper()
		return nil, ""
	}
	tests := []struct {
		name   string
		poison func(fake *fakeOAuthServer)
	}{
		{
			name: "plainhttp token endpoint",
			poison: func(fake *fakeOAuthServer) {
				fake.tokenEndpointURL = "http://downgraded.example.test/token"
			},
		},
		{
			name: "plainhttp authorization endpoint",
			poison: func(fake *fakeOAuthServer) {
				fake.authorizeEndpointURL = "http://downgraded.example.test/authorize"
			},
		},
	}
	_ = scheme
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeOAuthServer(t)
			test.poison(fake)
			repository := newMemoryRepository()
			secrets := newMemorySecrets()
			manager := newTestManager(repository, secrets, Options{})
			defer manager.Close()

			_, err := manager.Start(context.Background(), baseStartRequest(fake.endpoint))
			if err == nil {
				t.Fatal("Start accepted metadata with an insecure endpoint")
			}
			if !strings.Contains(err.Error(), "authorization server metadata") {
				t.Fatalf("error %q does not mention authorization server metadata", err)
			}
			if repository.length() != 0 || secrets.storageLength() != 0 {
				t.Fatal("downgraded flow persisted tokens")
			}
		})
	}
}

func TestInvalidGrantSanitizesTokenValues(t *testing.T) {
	fake := newFakeOAuthServer(t)
	fake.tokenEndpointRejectsGrant = true
	repository := newMemoryRepository()
	secrets := newMemorySecrets()
	manager := newTestManager(repository, secrets, Options{})
	defer manager.Close()

	started, err := manager.Start(context.Background(), baseStartRequest(fake.endpoint))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, _ = browserVisit(t, started.AuthorizationURL)
	terminal := waitForFlowStatus(t, manager, started.FlowID, FlowFailed)
	if !strings.Contains(terminal.Message, "authorization server rejected the authorization code") {
		t.Fatalf("exchange failure message is not sanitized: %q", terminal.Message)
	}
	flowJSON, err := json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{testAccessToken, testRefreshToken, "already used"} {
		if strings.Contains(string(flowJSON), banned) {
			t.Fatalf("failure view leaked %q: %s", banned, flowJSON)
		}
	}
	if secrets.storageLength() != 0 || repository.length() != 0 {
		t.Fatal("failed flow persisted tokens")
	}
}

func TestSecureHTTPClientPolicy(t *testing.T) {
	// Redirect refusal: discovery and token requests must never follow
	// redirect responses.
	overlayURL := ""
	overlay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		w.Header().Set("Location", overlayURL+"/elsewhere")
		w.WriteHeader(http.StatusFound)
	}))
	defer overlay.Close()
	overlayURL = overlay.URL

	client := secureHTTPClient(2*time.Second, 1<<10)
	_ = overlayURL
	resp, err := client.Get(overlay.URL + "/start")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("secure client followed a redirect")
	}
	if !errors.Is(err, errRedirectNotFollowed) {
		t.Fatalf("redirect rejection: %v", err)
	}

	// Response bodies are bounded.
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 1<<11))
	}))
	defer big.Close()
	bounded, err := client.Get(big.URL)
	if err != nil {
		t.Fatalf("bounded fetch failed: %v", err)
	}
	defer bounded.Body.Close()
	if _, err := io.ReadAll(bounded.Body); !errors.Is(err, errBodyTooLarge) {
		t.Fatalf("large body not rejected: %v", err)
	}

	// URL policy: plain http only on loopback; link-local never contacted.
	if err := checkProtectedResourceURL("http://insecure.example.test/mcp"); err == nil {
		t.Fatal("non-loopback http endpoint accepted")
	}
	if err := checkProtectedResourceURL("https://discovery.example.test/mcp"); err != nil {
		t.Fatalf("https endpoint rejected: %v", err)
	}
	if err := checkProtectedResourceURL("http://127.0.0.1:8931/mcp"); err != nil {
		t.Fatalf("loopback http endpoint rejected: %v", err)
	}
	metadata, _ := url.Parse("https://169.254.169.254/protected-resource")
	if err := checkRequestURL(metadata); err == nil {
		t.Fatal("link-local host accepted")
	}
}

// --- small internal test helpers ---

func (m *Manager) mustStatus(t *testing.T, flowID string) FlowView {
	t.Helper()
	view, err := m.Status(context.Background(), flowID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	return view
}

func (m *Manager) mustPort(t *testing.T, flowID string) int {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	flow, ok := m.flows[flowID]
	if !ok {
		t.Fatalf("flow %s missing from registry", flowID)
	}
	return flow.callbackPort()
}

func (s *memorySecrets) storageLength() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.values)
}

func (r *memoryRepository) length() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

// forgedCallbackURL follows the authorization redirect and tampers with the
// state parameter the way a hostile local page could.
func forgedCallbackURL(t *testing.T, authorizationURL string) string {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(authorizationURL)
	if err != nil {
		t.Fatalf("authorization URL fetch failed: %v", err)
	}
	defer resp.Body.Close()
	location := resp.Header.Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("redirect parse: %v", err)
	}
	query := parsed.Query()
	query.Set("state", "forged-by-attacker")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func fetchPage(t *testing.T, address string) string {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(address)
	if err != nil {
		t.Fatalf("fetch %s: %v", address, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", address, err)
	}
	return string(body)
}

func metadataJSON(t *testing.T, metadata any) string {
	t.Helper()
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	return string(raw)
}
