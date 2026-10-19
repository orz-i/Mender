package oauthflow

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

// flow runs one bounded authorization lifecycle. The runner goroutine is the
// single writer of the status; Cancel and Start-timeout transitions also go
// through dismiss, which only ever counts from pending.
type flow struct {
	id          string
	reference   credentialdomain.Reference
	request     StartRequest
	createdAt   time.Time
	expiresAt   time.Time
	redirectURL string

	now func() time.Time

	ctx    context.Context
	cancel context.CancelFunc

	listener net.Listener
	server   *http.Server

	resultCh chan callbackResult

	urlReadyOnce sync.Once
	urlReady     chan struct{}

	shutdownOnce sync.Once

	mu                   sync.Mutex
	status               FlowStatus
	message              string
	credential           *CredentialView
	terminalStamp        time.Time
	hasTerminalStamp     bool
	authorizationURL     string
	expectedState        string
	token                *oauth2.Token
	resolvedClientID     string
	resolvedClientSecret string
	resolvedAuthStyle    string
	wireResource         string
	requestedScopes      []string
}

type callbackResult struct {
	code string
	iss  string
	err  error
}

func newFlow(
	ref credentialdomain.Reference,
	request StartRequest,
	options Options,
	now func() time.Time,
) (*flow, error) {
	id, err := newFlowID()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("bind loopback callback listener: %w", err)
	}
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d%s", listener.Addr().(*net.TCPAddr).Port, callbackPath)
	flow := &flow{
		id:          id,
		reference:   ref,
		request:     request,
		now:         now,
		createdAt:   now(),
		expiresAt:   now().Add(options.FlowTTL),
		redirectURL: redirectURL,
		listener:    listener,
		resultCh:    make(chan callbackResult, 1),
		urlReady:    make(chan struct{}),
		status:      FlowPending,
	}
	flow.ctx, flow.cancel = context.WithTimeout(context.Background(), options.FlowTTL)
	mux := http.NewServeMux()
	mux.Handle(callbackPath, http.HandlerFunc(flow.serveCallback))
	flow.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	// The listener is bound (and serving) before Start returns the URL.
	go func() {
		_ = flow.server.Serve(listener)
	}()
	return flow, nil
}

func (f *flow) currentStatus() FlowStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *flow) terminalAt() (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.terminalStamp, f.hasTerminalStamp
}

// view renders the safe non-secret projection.
func (f *flow) view() FlowView {
	f.mu.Lock()
	defer f.mu.Unlock()
	view := FlowView{
		FlowID:    f.id,
		Status:    f.status,
		CreatedAt: f.createdAt,
		ExpiresAt: f.expiresAt,
	}
	if f.status == FlowPending {
		view.AuthorizationURL = f.authorizationURL
	} else {
		view.Message = f.message
	}
	if f.status == FlowConnected && f.credential != nil {
		clone := *f.credential
		clone.Metadata.Scopes = append([]string(nil), clone.Metadata.Scopes...)
		view.Credential = &clone
	}
	return view
}

func (f *flow) callbackPort() int {
	return f.listener.Addr().(*net.TCPAddr).Port
}

// run performs the whole bounded authorization lifecycle:
//
//  1. probe the MCP endpoint (no Authorization header attached),
//  2. discover protected-resource (challenge first, then well-known) and
//     authorization-server metadata through the SDK's public helpers,
//  3. run the SDK AuthorizationCodeHandler (PKCE S256, state, RFC 9207 iss,
//     loopback callback wait) until tokens arrive,
//  4. persist the opaque token bundle via SaveOAuth into the secret store.
func (f *flow) run(m *Manager) {
	defer f.finish()

	client := m.options.HTTPClient

	probe, err := http.NewRequestWithContext(f.ctx, http.MethodGet, f.request.MCPEndpoint, nil)
	if err != nil {
		f.fail("OAuth discovery: " + sanitizeAssistant(err))
		return
	}
	probeResponse, err := client.Do(probe)
	if err != nil {
		if isFlowLifecycleError(err) {
			return
		}
		f.fail("OAuth discovery: the MCP endpoint is unreachable")
		return
	}

	prm, err := f.discoverProtectedResource(probeResponse, client)
	if err != nil {
		_ = probeResponse.Body.Close()
		f.fail("OAuth discovery: " + err.Error())
		return
	}

	issuer, tokenEndpoint, err := discoverAuthorizationServer(f.ctx, client, firstAuthorizationServer(prm))
	if err != nil {
		_ = probeResponse.Body.Close()
		f.fail("OAuth discovery: " + err.Error())
		return
	}

	handler, err := newSDKHandler(m, f, client, issuer)
	if err != nil {
		_ = probeResponse.Body.Close()
		f.fail("OAuth authorization: " + err.Error())
		return
	}

	// Hand the original challenge response to the SDK; it consumes it.
	if err := handler.Authorize(f.ctx, probe, probeResponse); err != nil {
		if isFlowLifecycleError(err) {
			// Either Cancel cancelled the flow or the TTL elapsed; the flow
			// cleanup below settles the terminal status.
			return
		}
		f.fail("OAuth authorization: " + sanitizeAssistant(err))
		return
	}

	token := f.takeCapturedToken()
	if token == nil {
		f.fail("OAuth authorization completed without a usable token")
		return
	}

	bundle, metadata, err := f.buildTokenBundle(m, token, issuer, tokenEndpoint)
	if err != nil {
		f.fail("OAuth authorization: " + err.Error())
		return
	}
	defer clear(bundle)
	if err := f.persistCredential(m, metadata, bundle); err != nil {
		if isFlowLifecycleError(err) {
			return
		}
		f.fail("OAuth authorization: " + sanitizeAssistant(err))
		return
	}
}

// finish maps any leftover pending state to a terminal status, releases the
// loopback listener, and unblocks Start even on early failures.
func (f *flow) finish() {
	defer f.shutdownListener()
	defer func() {
		f.urlReadyOnce.Do(func() { close(f.urlReady) })
	}()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status != FlowPending {
		return
	}
	switch {
	case errors.Is(f.ctx.Err(), context.DeadlineExceeded):
		f.dismissLocked(FlowExpired, "authorization flow expired before completion")
	default:
		f.dismissLocked(FlowFailed, "OAuth authorization failed")
	}
}

// --- terminal transitions (single writer through dismiss) ---

func (f *flow) dismissLocked(status FlowStatus, message string) {
	f.status = status
	f.message = message
	f.terminalStamp = f.now()
	f.hasTerminalStamp = true
	f.authorizationURL = ""
	f.expectedState = ""
	f.token = nil
}

func (f *flow) fail(message string) {
	f.mu.Lock()
	if f.status != FlowPending {
		f.mu.Unlock()
		return
	}
	f.dismissLocked(FlowFailed, message)
	f.mu.Unlock()
	f.cancel()
}

func (f *flow) cancelFlow() error {
	f.mu.Lock()
	if f.status != FlowPending {
		f.mu.Unlock()
		return ErrFlowNotPending
	}
	f.dismissLocked(FlowCancelled, "authorization flow was cancelled")
	f.mu.Unlock()
	f.cancel()
	f.shutdownListener()
	return nil
}

// persistCredential serializes native token rotation, local disconnect, and
// the final pending -> connected transition. Holding the flow lock through
// the bounded keychain operation prevents a cancelled flow from persisting a
// usable token after reporting cancellation to the operator.
func (f *flow) persistCredential(m *Manager, metadata credentialdomain.OAuthMetadata, bundle []byte) error {
	m.tokenMu.Lock()
	defer m.tokenMu.Unlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status != FlowPending || f.ctx.Err() != nil {
		return context.Canceled
	}
	credential, err := m.save.Execute(f.ctx, credentialapplication.SaveOAuthRequest{
		Namespace: f.request.Namespace, Name: f.request.Name,
		DisplayName: f.request.DisplayName, Metadata: metadata, Secret: bundle,
	})
	if err != nil {
		return err
	}
	view := CredentialView{Reference: credential.Reference().String(), DisplayName: credential.DisplayName()}
	if metadata := credential.OAuthMetadata(); metadata != nil {
		view.Metadata = *metadata
	}
	f.credential = &view
	f.dismissLocked(FlowConnected, "")
	return nil
}

// --- SDK handler construction and fetcher ---

func newSDKHandler(
	m *Manager,
	f *flow,
	client *http.Client,
	issuer string,
) (*auth.AuthorizationCodeHandler, error) {
	config := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL:              f.redirectURL,
		AuthorizationCodeFetcher: f.fetcher,
		ScopeFilter:              scopeFilterFor(f.request.Scopes),
		RequestRefreshToken:      true,
		AcceptUnadvertisedIss:    true,
		Client:                   client,
		NewTokenSource: func(
			_ context.Context,
			clientConfig *oauth2.Config,
			token *oauth2.Token,
		) (oauth2.TokenSource, error) {
			f.captureToken(token, clientConfig)
			return oauth2.StaticTokenSource(token), nil
		},
	}
	switch {
	case f.request.ClientIDMetadataURL != "":
		config.ClientIDMetadataDocumentConfig = &auth.ClientIDMetadataDocumentConfig{
			URL: f.request.ClientIDMetadataURL,
		}
	case f.request.ClientID != "":
		credentials := &oauthex.ClientCredentials{ClientID: f.request.ClientID, Issuer: issuer}
		if f.request.ClientSecret != "" {
			credentials.ClientSecretAuth = &oauthex.ClientSecretAuth{
				ClientSecret: f.request.ClientSecret,
			}
		}
		config.PreregisteredClient = credentials
	default:
		config.DynamicClientRegistrationConfig = &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName: "Mender", RedirectURIs: []string{f.redirectURL},
				GrantTypes:    []string{"authorization_code", "refresh_token"},
				ResponseTypes: []string{"code"}, TokenEndpointAuthMethod: "none",
				ApplicationType: "native",
			},
		}
	}
	handler, err := auth.NewAuthorizationCodeHandler(config)
	if err != nil {
		return nil, errors.New("invalid authorization handler configuration")
	}
	return handler, nil
}

// fetcher is invoked by the SDK with the ready authorization URL. It
// advertises the URL to Start, then waits for the loopback callback while
// the flow context (TTL/cancel) arms it; cancellation always fails closed.
func (f *flow) fetcher(
	flowCtx context.Context,
	args *auth.AuthorizationArgs,
) (*auth.AuthorizationResult, error) {
	parsed, err := url.Parse(args.URL)
	if err != nil || parsed.Query().Get("state") == "" {
		f.fail("the authorization request is missing its state parameter")
		return nil, errors.New("authorization request states are required")
	}
	if err := validateBrowserAuthorizationURL(args.URL); err != nil {
		f.fail("authorization endpoint: " + err.Error())
		return nil, err
	}
	query := parsed.Query()
	state := query.Get("state")
	resource := query.Get("resource")
	if resource == "" {
		resource = f.request.MCPEndpoint
	}

	f.mu.Lock()
	f.expectedState = state
	f.wireResource = resource
	f.requestedScopes = grantedScopeTokens(query.Get("scope"))
	f.authorizationURL = args.URL
	f.mu.Unlock()
	f.markURLReady()

	select {
	case result := <-f.resultCh:
		if result.err != nil {
			return nil, result.err
		}
		return &auth.AuthorizationResult{Code: result.code, State: state, Iss: result.iss}, nil
	case <-flowCtx.Done():
		return nil, flowCtx.Err()
	}
}

func (f *flow) markURLReady() {
	f.urlReadyOnce.Do(func() { close(f.urlReady) })
}

func (f *flow) deliver(result callbackResult) {
	select {
	case f.resultCh <- result:
	default:
	}
}

// --- loopback callback server ---

// serveCallback handles only the bound callback path, validates the state
// parameter by constant-time comparison, and never echoes any value back to
// the browser.
func (f *flow) serveCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != callbackPath {
		writeFlowPage(w, http.StatusNotFound, pageUnknownHTML)
		return
	}
	query := r.URL.Query()
	f.mu.Lock()
	expected := f.expectedState
	f.mu.Unlock()
	if expected == "" ||
		subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(expected)) != 1 {
		// A mismatched or replayed callback fails the whole flow closed: the
		// SDK state check never sees this attempt's data. No values echoed.
		writeFlowPage(w, http.StatusBadRequest, pageFailureHTML)
		f.fail("the authorization callback state did not match")
		return
	}
	if serverError := query.Get("error"); serverError != "" {
		writeFlowPage(w, http.StatusOK, pageFailureHTML)
		f.fail("the authorization server returned " + safeMachineToken(serverError))
		return
	}
	code := query.Get("code")
	if code == "" {
		writeFlowPage(w, http.StatusOK, pageFailureHTML)
		f.fail("the authorization callback did not carry an authorization code")
		return
	}
	writeFlowPage(w, http.StatusOK, pageSuccessHTML)
	f.deliver(callbackResult{code: code, iss: query.Get("iss")})
}

func safeMachineToken(raw string) string {
	if raw == "" || len(raw) > 64 {
		return "an error response"
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return "an error response"
		}
	}
	return "error " + raw
}

func (f *flow) shutdownListener() {
	f.shutdownOnce.Do(func() {
		_ = f.listener.Close()
		_ = f.server.Close()
	})
}

// isFlowLifecycleError reports context lifecycle errors that flow cleanup
// maps to expired or cancelled statuses instead of a failure.
func isFlowLifecycleError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// --- discovery on top of public SDK APIs ---

// discoverProtectedResource mirrors the MCP protected-resource discovery
// order: the WWW-Authenticate challenge URL first, then well-known paths on
// the endpoint and the origin. The SDK validates the metadata's `resource`
// and authorization-server URL safety for us.
func (f *flow) discoverProtectedResource(
	response *http.Response,
	client *http.Client,
) (*oauthex.ProtectedResourceMetadata, error) {
	challenges := response.Header.Values("WWW-Authenticate")
	challengeURL := ""
	parsed, err := oauthex.ParseWWWAuthenticate(challenges)
	if err != nil {
		return nil, errors.New("the MCP endpoint issued an unparsable WWW-Authenticate challenge")
	}
	for _, challenge := range parsed {
		if metadataURL := challenge.Params["resource_metadata"]; metadataURL != "" {
			challengeURL = metadataURL
			break
		}
	}
	endpoint := f.request.MCPEndpoint
	if challengeURL != "" {
		metadata, err := oauthex.GetProtectedResourceMetadata(f.ctx, challengeURL, endpoint, client)
		if err != nil || metadata == nil {
			return nil, errors.New("the MCP-provided OAuth resource metadata is invalid or unavailable")
		}
		return metadata, nil
	}
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.New("the MCP endpoint is not a valid URL")
	}
	probe := &url.URL{Scheme: endpointURL.Scheme, Host: endpointURL.Host, User: endpointURL.User}
	pathProbe := *probe
	pathProbe.Path = "/.well-known/oauth-protected-resource/" + strings.TrimLeft(endpointURL.Path, "/")
	if metadata, err := oauthex.GetProtectedResourceMetadata(f.ctx, pathProbe.String(), endpoint, client); err == nil {
		return metadata, nil
	}
	if metadata, err := oauthex.GetProtectedResourceMetadata(f.ctx, rootWellKnownPath(probe), probe.String(), client); err == nil {
		return metadata, nil
	}
	// MCP 2026-07-28 requires protected-resource metadata. There is no
	// operator-supplied issuer to fall back to, so missing metadata fails closed.
	return nil, errors.New("OAuth protected-resource metadata is unavailable")
}

func rootWellKnownPath(origin *url.URL) string {
	clone := *origin
	clone.Path = "/.well-known/oauth-protected-resource"
	return clone.String()
}

func firstAuthorizationServer(metadata *oauthex.ProtectedResourceMetadata) string {
	if len(metadata.AuthorizationServers) == 0 {
		return ""
	}
	return metadata.AuthorizationServers[0]
}

// discoverAuthorizationServer requires RFC 8414 or OIDC discovery. Guessing
// legacy endpoints would bypass OAuth server metadata and PKCE verification.
func discoverAuthorizationServer(ctx context.Context, client *http.Client, issuer string) (string, string, error) {
	if issuer == "" {
		return "", "", errors.New("no authorization server is advertised")
	}
	metadata, err := auth.GetAuthServerMetadata(ctx, issuer, client)
	if err != nil {
		return "", "", errors.New("authorization server metadata is unavailable")
	}
	if metadata == nil {
		return "", "", errors.New("authorization server metadata is required")
	}
	resolvedIssuer := metadata.Issuer
	if resolvedIssuer == "" {
		resolvedIssuer = issuer
	}
	tokenEndpoint := metadata.TokenEndpoint
	if tokenEndpoint == "" {
		tokenEndpoint = strings.TrimSuffix(issuer, "/") + "/token"
	}
	return resolvedIssuer, tokenEndpoint, nil
}

// --- token persistence ---

// tokenBundle is the opaque JSON stored in the OS secret store. It never
// reaches SQLite, logs, flows, or API responses.
type tokenBundle struct {
	TokenType      string    `json:"tokenType,omitempty"`
	AccessToken    string    `json:"accessToken"`
	RefreshToken   string    `json:"refreshToken,omitempty"`
	ExpiresAt      time.Time `json:"expiresAt,omitempty"`
	Issuer         string    `json:"issuer"`
	ClientID       string    `json:"clientId"`
	MCPEndpoint    string    `json:"mcpEndpoint,omitempty"`
	ClientSecret   string    `json:"clientSecret,omitempty"`
	TokenAuthStyle string    `json:"tokenAuthStyle,omitempty"`
	TokenEndpoint  string    `json:"tokenEndpoint,omitempty"`
	Resource       string    `json:"resource"`
	Scopes         []string  `json:"scopes,omitempty"`
	CapturedAt     time.Time `json:"capturedAt"`
}

func (f *flow) takeCapturedToken() *oauth2.Token {
	f.mu.Lock()
	defer f.mu.Unlock()
	token := f.token
	f.token = nil
	return token
}

func (f *flow) captureToken(token *oauth2.Token, cfg *oauth2.Config) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = token
	if cfg != nil {
		f.resolvedClientID = cfg.ClientID
		f.resolvedClientSecret = cfg.ClientSecret
		if cfg.Endpoint.AuthStyle == oauth2.AuthStyleInHeader {
			f.resolvedAuthStyle = "basic"
		} else {
			f.resolvedAuthStyle = "post"
		}
	}
}

// buildTokenBundle encodes the newly authorized identity into the native
// secret store payload. A new grant must never inherit the old refresh token.
func (f *flow) buildTokenBundle(
	m *Manager,
	token *oauth2.Token,
	issuer string,
	tokenEndpoint string,
) ([]byte, credentialdomain.OAuthMetadata, error) {
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, credentialdomain.OAuthMetadata{}, errors.New("token is empty")
	}
	resource := f.wireResource
	if resource == "" {
		resource = f.request.MCPEndpoint
	}
	f.mu.Lock()
	requestedScopes := append([]string(nil), f.requestedScopes...)
	clientID := f.resolvedClientID
	clientSecret := f.resolvedClientSecret
	authStyle := f.resolvedAuthStyle
	f.mu.Unlock()
	if clientID == "" {
		return nil, credentialdomain.OAuthMetadata{}, errors.New("OAuth client registration yielded no client identifier")
	}
	registrationMode := credentialdomain.OAuthRegistrationDynamic
	switch {
	case f.request.ClientID != "":
		registrationMode = credentialdomain.OAuthRegistrationPreregistered
	case f.request.ClientIDMetadataURL != "":
		registrationMode = credentialdomain.OAuthRegistrationMetadataDocument
	}
	bundle := tokenBundle{
		TokenType:      token.TokenType,
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		ExpiresAt:      token.Expiry,
		Issuer:         issuer,
		ClientID:       clientID,
		MCPEndpoint:    f.request.MCPEndpoint,
		ClientSecret:   clientSecret,
		TokenAuthStyle: authStyle,
		TokenEndpoint:  tokenEndpoint,
		Resource:       resource,
		Scopes:         grantedScopes(token, requestedScopes),
		CapturedAt:     m.clock.Now(),
	}
	//nolint:gosec // Secret-containing bundle is serialized exclusively for the native OS keyring.
	blob, err := json.Marshal(bundle)
	if err != nil {
		return nil, credentialdomain.OAuthMetadata{}, errors.New("the token bundle could not be encoded")
	}
	payload := credentialdomain.OAuthMetadata{
		Resource:         resource,
		MCPEndpoint:      f.request.MCPEndpoint,
		Issuer:           issuer,
		ClientID:         bundle.ClientID,
		RegistrationMode: registrationMode,
		TokenEndpoint:    tokenEndpoint,
		Scopes:           bundle.Scopes,
		ExpiresAt:        token.Expiry,
		Status:           credentialdomain.OAuthConnected,
	}
	return blob, payload, nil
}

func grantedScopes(token *oauth2.Token, requested []string) []string {
	if granted, ok := token.Extra("scope").(string); ok {
		if tokens := grantedScopeTokens(granted); len(tokens) > 0 {
			return tokens
		}
	}
	return append([]string(nil), requested...)
}
