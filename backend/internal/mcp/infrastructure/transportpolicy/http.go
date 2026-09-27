// Package transportpolicy contains security policy shared by MCP transport
// configuration and connection adapters.
package transportpolicy

import "github.com/orz-i/Mender/backend/internal/providerhttp"

// ValidateHTTPEndpoint preserves the MCP adapter API while delegating the
// baseline endpoint rule to the Provider-generic HTTP policy.
func ValidateHTTPEndpoint(endpoint string) error {
	return providerhttp.ValidateEndpoint(endpoint)
}
