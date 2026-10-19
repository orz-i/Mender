package domain

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

type OAuthStatus string

// OAuthRegistrationMode records how the authorization server accepted the
// public client. It is non-secret and allows a later browser reauthorization
// to repeat dynamic registration instead of reusing a redirect-bound client ID.
type OAuthRegistrationMode string

const (
	OAuthRegistrationPreregistered    OAuthRegistrationMode = "pre_registered"
	OAuthRegistrationMetadataDocument OAuthRegistrationMode = "client_id_metadata"
	OAuthRegistrationDynamic          OAuthRegistrationMode = "dynamic"
)

const (
	OAuthConnected             OAuthStatus = "connected"
	OAuthAuthorizationRequired OAuthStatus = "authorization_required"
	OAuthExpired               OAuthStatus = "expired"
)

// OAuthMetadata contains only non-secret connection metadata. Tokens, client
// secrets, authorization codes and PKCE verifiers must never be stored here.
type OAuthMetadata struct {
	Resource string `json:"resource"`
	// MCPEndpoint is the single configured HTTP MCP URL that receives tokens.
	// Resource may be its origin when RFC9728 metadata is hosted at the root.
	MCPEndpoint      string                `json:"mcpEndpoint,omitempty"`
	Issuer           string                `json:"issuer"`
	ClientID         string                `json:"clientId"`
	RegistrationMode OAuthRegistrationMode `json:"registrationMode,omitempty"`
	TokenEndpoint    string                `json:"tokenEndpoint"`
	Scopes           []string              `json:"scopes,omitempty"`
	ExpiresAt        time.Time             `json:"expiresAt,omitempty"`
	Status           OAuthStatus           `json:"status"`
}

func (m OAuthMetadata) Validate() error {
	if err := validateOAuthRemoteURL(m.Resource, true); err != nil {
		return fmt.Errorf("OAuth protected resource: %w", err)
	}
	if m.MCPEndpoint != "" {
		if err := validateOAuthRemoteURL(m.MCPEndpoint, true); err != nil {
			return fmt.Errorf("OAuth MCP endpoint: %w", err)
		}
	}
	if err := validateOAuthRemoteURL(m.Issuer, false); err != nil {
		return fmt.Errorf("OAuth issuer: %w", err)
	}
	if err := validateOAuthRemoteURL(m.TokenEndpoint, false); err != nil {
		return fmt.Errorf("OAuth token endpoint: %w", err)
	}
	if m.ClientID == "" || len(m.ClientID) > 2048 {
		return fmt.Errorf("OAuth client ID must be 1–2048 characters")
	}
	switch m.RegistrationMode {
	case "", OAuthRegistrationPreregistered, OAuthRegistrationMetadataDocument, OAuthRegistrationDynamic:
	default:
		return fmt.Errorf("invalid OAuth client registration mode")
	}
	switch m.Status {
	case OAuthConnected, OAuthAuthorizationRequired, OAuthExpired:
	default:
		return fmt.Errorf("invalid OAuth connection status")
	}
	if len(m.Scopes) > 64 {
		return fmt.Errorf("too many OAuth scopes")
	}
	for _, scope := range m.Scopes {
		if scope == "" || len(scope) > 256 || strings.ContainsAny(scope, "\r\n\t ") {
			return fmt.Errorf("invalid OAuth scope")
		}
	}
	return nil
}

func validateOAuthRemoteURL(raw string, allowPath bool) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("must be an absolute URL without userinfo or fragment")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return fmt.Errorf("must use https outside loopback")
		}
	}
	if !allowPath && u.RawQuery != "" {
		return fmt.Errorf("must not contain query parameters")
	}
	return nil
}
