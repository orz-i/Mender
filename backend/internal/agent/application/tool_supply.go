package application

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ToolDelivery identifies a way an external Agent can consume canonical
// Mender tools. The Agent application does not speak ACP or run an Agent Loop;
// protocol adapters map this capability to their own negotiated transport.
// An unsupported Agent MUST reject supply rather than falling back to host.
type ToolDelivery string

const ToolDeliveryMCPHTTP ToolDelivery = "mcp_http"

// ToolSupply is a transient process-local connection, not a persisted Grant,
// Provider, ToolID, credential inventory or model-controllable Tool input.
// It is allocated by the owning Runtime for one active Task and expired on
// cancellation/termination. The agent protocol adapter may use it ONLY if
// the Agent's negotiated capabilities support its kind.
type ToolSupply struct {
	Delivery    ToolDelivery
	Endpoint    string
	BearerToken string // never log, persist, emit to upstream MCP, or return in Task results
}

func (s ToolSupply) IsZero() bool { return s.Delivery == "" && s.Endpoint == "" && s.BearerToken == "" }
func (s ToolSupply) Validate() error {
	if s.IsZero() {
		return nil
	}
	if s.Delivery != ToolDeliveryMCPHTTP {
		return errors.New("unsupported task tool delivery")
	}
	u, err := url.Parse(s.Endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" ||
		u.Path != "/mcp" || u.Hostname() != "127.0.0.1" {
		return errors.New("task tool HTTP delivery must use loopback MCP")
	}
	_, port, err := net.SplitHostPort(u.Host)
	number, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || number < 1 || number > 65535 {
		return errors.New("invalid task tool loopback port")
	}
	if len(s.BearerToken) != 64 {
		return errors.New("invalid task tool capability")
	}
	for _, r := range s.BearerToken {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return errors.New("invalid task tool capability")
		}
	}
	return nil
}
