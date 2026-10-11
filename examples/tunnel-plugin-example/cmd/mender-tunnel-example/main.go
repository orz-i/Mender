// A no-cloud, no-public-listener, deterministic Tunnel Transport fixture.
// Only the public SDK is imported; native transport trust is still required.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tunnelplugin "github.com/orz-i/Mender/sdk/tunnelplugin/go"
	tunnelv1 "github.com/orz-i/Mender/sdk/tunnelplugin/go/protocol/v1"
)

const maxSessions = 16

type session struct {
	endpoint string
	phase    string
}
type service struct {
	tunnelv1.UnimplementedTunnelTransportServiceServer
	mu       sync.Mutex
	sessions map[string]session
}

func newService() *service { return &service{sessions: map[string]session{}} }
func (*service) GetDescriptor(context.Context, *tunnelv1.GetDescriptorRequest) (*tunnelv1.GetDescriptorResponse, error) {
	return &tunnelv1.GetDescriptorResponse{Transport: &tunnelv1.TransportDescriptor{
		PluginId: "dev.mender.example.tunnel", PackageVersion: "0.1.0", ProtocolVersion: 1, ConfigVersion: 1,
		DisplayName: "Example Tunnel", Description: "Local-only mock transport, never a real public OpenAI tunnel",
		ConfigSchemaJson: []byte(`{"type":"object","additionalProperties":false,"required":["externalTunnelId"],"properties":{"externalTunnelId":{"type":"string","minLength":1,"maxLength":256}}}`),
		Capabilities:     []string{"loopback-mcp-proxy"}, ReachabilitySemantics: "loopback fixture only; no public reachability",
	}}, nil
}
func validateConfig(raw []byte) error {
	if len(raw) == 0 || len(raw) > 4096 {
		return fmt.Errorf("invalid bounded Tunnel config")
	}
	var c struct {
		ExternalTunnelID string `json:"externalTunnelId"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("invalid Tunnel config")
	}
	if c.ExternalTunnelID == "" || len(c.ExternalTunnelID) > 256 {
		return fmt.Errorf("invalid external tunnel id")
	}
	return nil
}
func (*service) ValidateConfig(_ context.Context, r *tunnelv1.ValidateConfigRequest) (*tunnelv1.ValidateConfigResponse, error) {
	if r.GetConfigVersion() != 1 {
		return &tunnelv1.ValidateConfigResponse{Diagnostic: "unsupported config version"}, nil
	}
	if err := validateConfig(r.GetConfigJson()); err != nil {
		return &tunnelv1.ValidateConfigResponse{Diagnostic: err.Error()}, nil
	}
	return &tunnelv1.ValidateConfigResponse{Valid: true}, nil
}
func (*service) InspectConfig(_ context.Context, r *tunnelv1.InspectConfigRequest) (*tunnelv1.InspectConfigResponse, error) {
	if r.GetConfigVersion() != 1 {
		return nil, fmt.Errorf("unsupported config version")
	}
	if err := validateConfig(r.GetConfigJson()); err != nil {
		return nil, err
	}
	return &tunnelv1.InspectConfigResponse{ConnectionSummary: "Local loopback fixture; does not expose public connectivity"}, nil
}
func validateLoopbackEndpoint(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.Path != "/mcp" || u.Hostname() == "" {
		return fmt.Errorf("tunnel endpoint must be an exact managed loopback HTTP MCP URL")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("tunnel endpoint must be literal loopback")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("tunnel endpoint port invalid")
	}
	if strings.ContainsAny(value, "\r\n\t") {
		return fmt.Errorf("tunnel endpoint contains invalid bytes")
	}
	return nil
}
func (s *service) Connect(ctx context.Context, r *tunnelv1.ConnectRequest) (*tunnelv1.ConnectResponse, error) {
	if r.GetTunnelId() == "" || r.GetExternalTunnelId() == "" || len(r.GetEphemeralCredential()) == 0 {
		return nil, fmt.Errorf("missing exact tunnel identity or scoped Credential")
	}
	if err := validateLoopbackEndpoint(r.GetMcpEndpoint()); err != nil {
		return nil, err
	}
	if r.GetExternalTunnelId() == "force-fail" {
		return nil, fmt.Errorf("fixture declined Tunnel Transport connection")
	}
	// The fixture establishes readiness only from an actual local MCP HTTP
	// response. It does not claim any downstream public Tunnel registration.
	client := http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.GetMcpEndpoint(), nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("managed MCP endpoint unreachable: %w", err)
	}
	_ = response.Body.Close()
	// A live MCP server may legitimately reject an unauthenticated GET with
	// 400/405; it must at least respond. A 502/503/504 is not healthy.
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return nil, fmt.Errorf("managed MCP endpoint redirected unexpectedly")
	}
	if response.StatusCode >= 500 {
		return nil, fmt.Errorf("managed MCP endpoint unhealthy")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) >= maxSessions {
		return nil, fmt.Errorf("example tunnel session capacity exceeded")
	}
	id := "tunnel:" + r.GetTunnelId()
	if _, present := s.sessions[id]; present {
		return nil, fmt.Errorf("duplicate tunnel session")
	}
	phase := "ready"
	if r.GetExternalTunnelId() == "degraded" {
		phase = "degraded"
	}
	s.sessions[id] = session{endpoint: r.GetMcpEndpoint(), phase: phase}
	return &tunnelv1.ConnectResponse{SessionId: id, Phase: phase, Diagnostic: "managed loopback endpoint responded; no public reachability is asserted"}, nil
}
func (s *service) Status(_ context.Context, r *tunnelv1.StatusRequest) (*tunnelv1.StatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.sessions[r.GetSessionId()]
	if !ok {
		return &tunnelv1.StatusResponse{Phase: "failed", Diagnostic: "session is unavailable"}, nil
	}
	return &tunnelv1.StatusResponse{Phase: active.phase, Diagnostic: "local fixture health only"}, nil
}
func (s *service) Disconnect(_ context.Context, r *tunnelv1.DisconnectRequest) (*tunnelv1.DisconnectResponse, error) {
	s.mu.Lock()
	delete(s.sessions, r.GetSessionId())
	s.mu.Unlock()
	return &tunnelv1.DisconnectResponse{}, nil
}
func main() {
	if err := tunnelplugin.Serve(newService()); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
