package provider

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

// oauthResourceRoundTripper attaches the OAuth bearer only to the configured
// MCP resource's exact origin and path. Discovery, token and redirect requests
// are never allowed to inherit the token.
type oauthResourceRoundTripper struct {
	base      http.RoundTripper
	origin    string
	path      string
	resource  string
	reference credentialdomain.Reference
	source    OAuthTokenSource
}

func (t oauthResourceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil || canonicalOrigin(req.URL) != t.origin || req.URL.EscapedPath() != t.path {
		return nil, fmt.Errorf("MCP OAuth request escaped the configured resource")
	}
	token, err := t.source.AccessToken(req.Context(), t.reference, t.resource)
	if err != nil {
		return nil, fmt.Errorf("MCP OAuth authorization unavailable: %w", err)
	}
	if token == "" || strings.ContainsAny(token, "\r\n\x00") {
		return nil, fmt.Errorf("MCP OAuth bearer token is invalid")
	}
	outgoing := req.Clone(req.Context())
	outgoing.Header = req.Header.Clone()
	outgoing.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(outgoing)
}

func httpClientWithOAuth(
	client *http.Client,
	endpoint string,
	reference credentialdomain.Reference,
	source OAuthTokenSource,
) (*http.Client, error) {
	if source == nil {
		return nil, fmt.Errorf("OAuth token source is unavailable")
	}
	if _, err := credentialdomain.ParseReference(reference.String()); err != nil {
		return nil, fmt.Errorf("OAuth Credential Reference is invalid: %w", err)
	}
	endpointURL, err := url.Parse(endpoint)
	if err != nil || endpointURL.Host == "" {
		return nil, fmt.Errorf("OAuth protected resource URL is invalid")
	}
	path := endpointURL.EscapedPath()
	if path == "" {
		path = "/"
	}
	var result http.Client
	if client != nil {
		result = *client
	}
	base := result.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	result.Transport = oauthResourceRoundTripper{
		base: base, origin: canonicalOrigin(endpointURL), path: path,
		reference: reference, resource: endpoint, source: source,
	}
	// Even same-origin redirects are refused so a path switch cannot leak
	// the bearer into an untrusted endpoint.
	result.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("MCP OAuth resource redirects are forbidden")
	}
	return &result, nil
}
