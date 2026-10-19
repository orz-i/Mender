// Package oauthflow drives interactive MCP OAuth 2.1 authorization-code
// flows with PKCE S256 on top of the official MCP Go SDK.
//
// Start binds a 127.0.0.1 loopback callback listener first, runs
// protected-resource and authorization-server metadata discovery through the
// SDK, then hands the authorization URL to the caller for the user's system
// browser. Nothing in this package launches a browser, and neither Provider
// activation nor Tool execution may call Start — the OAuth connection always
// requires an explicit interactive authorization.
//
// Tokens never surface through this package's results, SQLite, or logs:
// after a successful code exchange the opaque token bundle is stored in the
// existing native secret store through the credential application use cases,
// and flows expose only non-secret metadata.
package oauthflow

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

// OAuthFlowManager is the interface the Desktop and HTTP layers will adopt in
// a later integration stage.
type OAuthFlowManager interface {
	Start(ctx context.Context, request StartRequest) (StartResult, error)
	Status(ctx context.Context, flowID string) (FlowView, error)
	Cancel(ctx context.Context, flowID string) error
	Close() error
}

var _ OAuthFlowManager = (*Manager)(nil)

var (
	ErrFlowNotFound   = errors.New("OAuth flow: not found")
	ErrFlowNotPending = errors.New("OAuth flow: not pending")
	ErrTooManyFlows   = errors.New("OAuth flow: too many pending authorization flows")
)

// StartRequest describes one interactive authorization against one MCP
// endpoint. The client secret (when present) stays in flow memory only; it
// is never persisted, logged, or surfaced.
type StartRequest struct {
	Namespace   string
	Name        string
	DisplayName string

	// MCPEndpoint is the protected-resource URL and the canonical `resource`
	// identifier sent to the authorization server.
	MCPEndpoint string

	// An explicit client ID uses pre-registration. If both client ID and
	// ClientIDMetadataURL are absent, the SDK tries advertised dynamic client
	// registration; when unsupported, authorization fails closed.
	ClientID     string
	ClientSecret string

	// ClientIDMetadataURL is the SEP-991 client-id metadata document HTTPS
	// URL as an alternative to a pre-registered client ID.
	ClientIDMetadataURL string

	// Scopes limits the authorization request to this set after discovery.
	Scopes []string
}

func (r StartRequest) validate() error {
	if r.Namespace == "" || r.Name == "" {
		return errors.New("namespace and name are required")
	}
	ref, err := credentialdomain.NewReference(r.Namespace, r.Name)
	if err != nil {
		return err
	}
	if ref.Namespace() == "ssh" {
		return errors.New("OAuth credentials cannot use the ssh namespace")
	}
	if err := checkProtectedResourceURL(r.MCPEndpoint); err != nil {
		return err
	}
	if r.ClientID != "" && r.ClientIDMetadataURL != "" {
		return errors.New("pre-registered client ID and client-id metadata are mutually exclusive")
	}
	if len(r.ClientID) > 2048 {
		return errors.New("OAuth client ID is limited to 2048 characters")
	}
	if r.ClientSecret != "" && r.ClientID == "" {
		return errors.New("a client secret requires a pre-registered client ID")
	}
	if len(r.Scopes) > 64 {
		return errors.New("too many OAuth scopes")
	}
	for _, scope := range r.Scopes {
		if scope == "" || len(scope) > 256 || containsWhitespace(scope) {
			return errors.New("invalid OAuth scope")
		}
	}
	return nil
}

func containsWhitespace(value string) bool {
	for _, r := range value {
		switch r {
		case ' ', '\t', '\r', '\n':
			return true
		}
	}
	return false
}

// StartResult is returned once the loopback callback listener is bound and
// the authorization URL is ready for the system browser.
type StartResult struct {
	FlowID           string `json:"flowId"`
	AuthorizationURL string `json:"authorizationUrl"`
}

// FlowStatus is the lifecycle of one authorization flow.
type FlowStatus string

const (
	FlowPending   FlowStatus = "pending"
	FlowConnected FlowStatus = "connected"
	FlowFailed    FlowStatus = "failed"
	FlowExpired   FlowStatus = "expired"
	FlowCancelled FlowStatus = "cancelled"
)

// CredentialView is the non-secret result of a connected flow; tokens are
// held only by the native secret store.
type CredentialView struct {
	Reference   string                         `json:"reference"`
	DisplayName string                         `json:"displayName"`
	Metadata    credentialdomain.OAuthMetadata `json:"metadata"`
}

// FlowView is the safe projection of an authorization flow.
type FlowView struct {
	FlowID           string          `json:"flowId"`
	Status           FlowStatus      `json:"status"`
	CreatedAt        time.Time       `json:"createdAt,omitempty"`
	ExpiresAt        time.Time       `json:"expiresAt,omitempty"`
	AuthorizationURL string          `json:"authorizationUrl,omitempty"`
	Message          string          `json:"message,omitempty"`
	Credential       *CredentialView `json:"credential,omitempty"`
}

// Options tunes the manager; zero values fall back to bounded defaults.
type Options struct {
	// StartTimeout bounds metadata discovery before the URL is handed out.
	StartTimeout time.Duration
	// FlowTTL bounds a pending authorization flow end to end, after which
	// the flow fails closed.
	FlowTTL time.Duration
	// MaxPendingFlows bounds concurrent authorization flows.
	MaxPendingFlows int
	// ResponseByteLimit bounds every OAuth HTTP response body.
	ResponseByteLimit int64
	// HTTPClient overrides the fail-closed client; used by tests.
	HTTPClient *http.Client
}

func (o *Options) fill() {
	if o == nil {
		return
	}
	if o.StartTimeout <= 0 {
		o.StartTimeout = defaultStartTimeout
	}
	if o.FlowTTL <= 0 {
		o.FlowTTL = defaultFlowTTL
	}
	if o.MaxPendingFlows <= 0 {
		o.MaxPendingFlows = defaultMaxPendingFlows
	}
	if o.ResponseByteLimit <= 0 {
		o.ResponseByteLimit = defaultResponseBytes
	}
	if o.HTTPClient == nil {
		o.HTTPClient = secureHTTPClient(defaultRequestTimeout, o.ResponseByteLimit)
	}
}

// Manager keeps authorization flows in memory with bounded lifetimes and
// persists successful tokens through the existing credential use cases.
type Manager struct {
	repository credentialapplication.Repository
	secrets    credentialapplication.SecretStore
	clock      credentialapplication.Clock
	save       credentialapplication.SaveOAuth
	options    Options

	mu    sync.Mutex
	flows map[string]*flow

	// tokenMu serializes refresh, token rotation and explicit disconnect.
	// It is deliberately separate from the in-memory flow registry lock.
	tokenMu sync.Mutex
}

func NewManager(
	repository credentialapplication.Repository,
	secrets credentialapplication.SecretStore,
	clock credentialapplication.Clock,
	options Options,
) *Manager {
	options.fill()
	if clock == nil {
		clock = systemClock{}
	}
	return &Manager{
		repository: repository,
		secrets:    secrets,
		clock:      clock,
		save:       credentialapplication.NewSaveOAuth(repository, secrets, clock),
		options:    options,
		flows:      make(map[string]*flow),
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Start validates the request, binds the loopback callback listener, runs
// discovery and client registration through the MCP SDK, and returns the
// authorization URL for the system browser. It never opens a browser.
func (m *Manager) Start(ctx context.Context, request StartRequest) (StartResult, error) {
	if err := request.validate(); err != nil {
		return StartResult{}, fmt.Errorf(
			"OAuth connection requires interactive authorization: %w", err,
		)
	}
	ref, err := credentialdomain.NewReference(request.Namespace, request.Name)
	if err != nil {
		return StartResult{}, fmt.Errorf(
			"OAuth connection requires interactive authorization: %w", err,
		)
	}

	flow, err := m.startFlow(ref, request)
	if err != nil {
		return StartResult{}, fmt.Errorf(
			"OAuth connection requires interactive authorization: %w", err,
		)
	}
	go flow.run(m)

	select {
	case <-flow.urlReady:
		view := flow.view()
		if view.Status != FlowPending || view.AuthorizationURL == "" {
			return StartResult{}, fmt.Errorf(
				"OAuth connection requires interactive authorization: %s", view.Message,
			)
		}
		return StartResult{FlowID: flow.id, AuthorizationURL: view.AuthorizationURL}, nil

	case <-time.After(m.options.StartTimeout):
		flow.fail("timed out waiting for the authorization URL")
		return StartResult{}, errors.New(
			"OAuth connection requires interactive authorization: metadata discovery did not complete in time",
		)

	case <-ctx.Done():
		flow.fail("authorization flow was interrupted")
		return StartResult{}, fmt.Errorf(
			"OAuth connection requires interactive authorization: %w", ctx.Err(),
		)
	}
}

// startFlow binds the callback listener and registers the flow. The
// registry insert happens before run starts so Status is always consistent.
func (m *Manager) startFlow(ref credentialdomain.Reference, request StartRequest) (*flow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	m.sweepLocked(now)
	pending := 0
	for _, item := range m.flows {
		if item.currentStatus() == FlowPending {
			pending++
		}
	}
	if pending >= m.options.MaxPendingFlows {
		return nil, ErrTooManyFlows
	}
	flow, err := newFlow(ref, request, m.options, m.clock.Now)
	if err != nil {
		return nil, err
	}
	m.flows[flow.id] = flow
	return flow, nil
}

// Status reports the safe, non-secret view of one flow.
func (m *Manager) Status(_ context.Context, flowID string) (FlowView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(m.clock.Now())
	flow, ok := m.flows[flowID]
	if !ok {
		return FlowView{}, ErrFlowNotFound
	}
	return flow.view(), nil
}

// Cancel cancels a pending flow; terminal flows cannot be cancelled.
func (m *Manager) Cancel(_ context.Context, flowID string) error {
	m.mu.Lock()
	flow, ok := m.flows[flowID]
	m.mu.Unlock()
	if !ok {
		return ErrFlowNotFound
	}
	return flow.cancelFlow()
}

// Close cancels every flow and releases loopback listeners.
func (m *Manager) Close() error {
	m.mu.Lock()
	flows := make([]*flow, 0, len(m.flows))
	for _, item := range m.flows {
		flows = append(flows, item)
	}
	m.mu.Unlock()
	for _, flow := range flows {
		flow.cancel()
		flow.shutdownListener()
	}
	return nil
}

// sweepLocked evicts terminal records older than the retention window so the
// registry stays bounded.
func (m *Manager) sweepLocked(now time.Time) {
	for id, flow := range m.flows {
		if flow.currentStatus() == FlowPending {
			continue
		}
		if terminal, ok := flow.terminalAt(); ok && now.Sub(terminal) > terminalRetention {
			delete(m.flows, id)
		}
	}
}

// checkProtectedResourceURL applies the OAuth endpoint transport policy to
// the MCP endpoint named in StartRequest.
func checkProtectedResourceURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New(
			"MCP endpoint must be an absolute https (or loopback http) URL",
		)
	}
	if err := checkRequestURL(u); err != nil {
		return fmt.Errorf("MCP endpoint: %w", err)
	}
	return nil
}
