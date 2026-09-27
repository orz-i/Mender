package providerhttp

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateEndpoint applies Mender's baseline policy for explicitly configured
// Provider HTTP(S) endpoints.
func ValidateEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("endpoint must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("endpoint must be absolute")
	}
	if parsed.User != nil {
		return fmt.Errorf("endpoint must not contain URL userinfo")
	}
	if parsed.Scheme == "http" && !isLoopbackHostname(parsed.Hostname()) {
		return fmt.Errorf("non-loopback endpoint must use https")
	}
	return nil
}

func isLoopbackHostname(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
