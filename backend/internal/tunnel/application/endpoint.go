package application

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ValidateManagedMCPEndpoint ensures a plugin never selects a non-local public
// listener or receives a Credential from a host-selected arbitrary URL.
func ValidateManagedMCPEndpoint(raw string) error {
	if len(raw) == 0 || len(raw) > 2048 || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\t") {
		return fmt.Errorf("managed MCP tunnel endpoint invalid")
	}
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Scheme != "http" || endpoint.User != nil ||
		endpoint.Hostname() == "" || endpoint.Path != "/mcp" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return fmt.Errorf("tunnel transport requires host-selected loopback HTTP MCP URL")
	}
	ip := net.ParseIP(endpoint.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("tunnel transport endpoint must use literal loopback IP")
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("tunnel transport endpoint port invalid")
	}
	return nil
}
