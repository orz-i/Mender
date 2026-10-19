package oauthflow

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Defaults keep OAuth networking and flow lifetimes bounded without explicit
// configuration from the caller.
const (
	defaultRequestTimeout  = 15 * time.Second
	defaultStartTimeout    = 15 * time.Second
	defaultFlowTTL         = 10 * time.Minute
	defaultMaxPendingFlows = 8
	defaultResponseBytes   = int64(512 << 10)
	terminalRetention      = 5 * time.Minute
	callbackPath           = "/callback"
)

var (
	errRedirectNotFollowed = errors.New(
		"redirects are not followed during OAuth metadata and token requests",
	)
	errBodyTooLarge = errors.New("OAuth HTTP response exceeded the allowed size")
	errSchemeDenied = errors.New(
		"OAuth networking allows only https outside loopback and http on loopback",
	)
	errHostDenied = errors.New("OAuth networking forbids sprayed non-public hosts")

	// oauthJSONSecretPattern scrubs credential-bearing JSON keys; x/oauth2
	// surfaces raw token endpoint response bodies in some failure messages.
	oauthJSONSecretPattern = regexp.MustCompile(
		`(?i)("(?:access_token|refresh_token|id_token|authorization_code|code|state)"\s*:\s*)("[^"]*"|[^,}\s]+)`,
	)
	oauthParamSecretPattern = regexp.MustCompile(
		`(?i)((?:access|refresh)_token|id_token|authorization_code|code|state)=[^&\s"']+`,
	)
)

// secureHTTPClient returns an http.Client for OAuth discovery and token
// exchange that fails closed: no proxy, no redirect following, https (or
// loopback-only http), link-local hosts denied, bounded request timeouts and
// bounded response bodies.
func secureHTTPClient(timeout time.Duration, maxBytes int64) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{
		Timeout: timeout,
		Transport: &secureTransport{
			base:     transport,
			maxBytes: maxBytes,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirectNotFollowed
		},
	}
}

type secureTransport struct {
	base     http.RoundTripper
	maxBytes int64
}

func (t *secureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := checkRequestURL(request.URL); err != nil {
		return nil, fmt.Errorf("blocked %s request: %w", request.URL.Scheme, err)
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.Body != nil {
		response.Body = &boundedBody{reader: response.Body, remaining: t.maxBytes}
	}
	return response, nil
}

// boundedBody fails any response larger than the configured limit so
// discovery and token endpoints cannot pin memory or stall decoders.
type boundedBody struct {
	reader    io.ReadCloser
	remaining int64
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, errBodyTooLarge
	}
	n, err := b.reader.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func (b *boundedBody) Close() error {
	return b.reader.Close()
}

func checkRequestURL(u *url.URL) error {
	if u == nil || u.Hostname() == "" {
		return fmt.Errorf("%w: requests target absolute host URLs", errHostDenied)
	}
	switch u.Scheme {
	case "https":
		if isLinkLocalHost(u.Hostname()) {
			return fmt.Errorf("%w: link-local targets are never contacted", errHostDenied)
		}
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return errSchemeDenied
		}
	default:
		return errSchemeDenied
	}
	return nil
}

// validateBrowserAuthorizationURL applies the same policy to the
// authorization redirect handed to the system browser: the URL surfaced to
// the user must be https, or loopback-only http.
func validateBrowserAuthorizationURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New(
			"authorization endpoint must be an absolute https (or loopback http) URL",
		)
	}
	return checkRequestURL(u)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLinkLocalHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast())
}

// sanitizeAssistant projects a failure into user-visible flow state without
// ever echoing token, code, or state values.
func sanitizeAssistant(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if strings.Contains(message, "token exchange failed") {
		return "authorization server rejected the authorization code"
	}
	message = oauthJSONSecretPattern.ReplaceAllString(message, `${1}"[removed]"`)
	message = oauthParamSecretPattern.ReplaceAllString(message, `${1}=[removed]`)
	message = strings.TrimSpace(message)
	if message == "" {
		message = "OAuth authorization failed"
	}
	return truncateMessage(message, 512)
}

func truncateMessage(message string, limit int) string {
	runes := []rune(message)
	if len(runes) <= limit {
		return message
	}
	return string(runes[:limit]) + "…"
}

// scopeFilterFor narrows server-advertised scopes to the caller's request:
// server metadata can never widen what the client asks for, and an empty
// request keeps discovery as-is.
func scopeFilterFor(requested []string) func(discovered []string) []string {
	wanted := append([]string(nil), requested...)
	return func(discovered []string) []string {
		if len(wanted) == 0 {
			return append([]string(nil), discovered...)
		}
		if len(discovered) == 0 {
			return append([]string(nil), wanted...)
		}
		advertised := make(map[string]struct{}, len(discovered))
		for _, scope := range discovered {
			advertised[scope] = struct{}{}
		}
		filtered := make([]string, 0, len(wanted))
		for _, scope := range wanted {
			if _, ok := advertised[scope]; ok {
				filtered = append(filtered, scope)
			}
		}
		return filtered
	}
}

// grantedScopeTokens keeps only well-formed OAuth scope tokens from a granted
// scope string; malformed input is dropped instead of turned into state.
func grantedScopeTokens(raw string) []string {
	values := make([]string, 0, 8)
	for _, token := range strings.Fields(raw) {
		if token == "" || len(token) > 256 || strings.ContainsAny(token, "\r\n\t ") {
			continue
		}
		values = append(values, token)
		if len(values) == 64 {
			break
		}
	}
	return values
}

// Browser-facing callback pages never echo codes, states, or tokens.
const (
	pageSuccessHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Mender</title></head><body><p>Authorization for Mender completed. You can close this window.</p></body></html>`
	pageFailureHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Mender</title></head><body><p>Authorization for Mender could not be completed. Use the Connect action again.</p></body></html>`
	pageUnknownHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Mender</title></head><body><p>Unknown request.</p></body></html>`
)

func writeFlowPage(w http.ResponseWriter, status int, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(page))
}

func newFlowID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate OAuth flow id: %w", err)
	}
	return "mfo_" + hex.EncodeToString(raw), nil
}
