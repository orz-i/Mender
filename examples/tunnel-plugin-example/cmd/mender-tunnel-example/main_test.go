package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tunnelv1 "github.com/orz-i/Mender/sdk/tunnelplugin/go/protocol/v1"
)

func TestTunnelTransportFixtureLifecycle(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()
	service := newService()
	ctx := context.Background()
	descriptor, err := service.GetDescriptor(ctx, &tunnelv1.GetDescriptorRequest{})
	if err != nil || descriptor.GetTransport().GetProtocolVersion() != 1 || descriptor.GetTransport().GetReachabilitySemantics() == "" {
		t.Fatalf("descriptor unavailable: %+v %v", descriptor, err)
	}
	valid, err := service.ValidateConfig(ctx, &tunnelv1.ValidateConfigRequest{ConfigVersion: 1, ConfigJson: []byte(`{"externalTunnelId":"example"}`)})
	if err != nil || !valid.GetValid() {
		t.Fatalf("valid config rejected %+v %v", valid, err)
	}
	if result, err := service.ValidateConfig(ctx, &tunnelv1.ValidateConfigRequest{ConfigVersion: 1, ConfigJson: []byte(`{}`)}); err != nil || result.GetValid() {
		t.Fatalf("unsafe config accepted %+v %v", result, err)
	}
	request := &tunnelv1.ConnectRequest{TunnelId: "tun_0123456789abcdef0123456789abcdef", ExternalTunnelId: "test",
		McpEndpoint: server.URL + "/mcp", EphemeralCredential: []byte("fixture-credential-not-for-logs")}
	connected, err := service.Connect(ctx, request)
	if err != nil || connected.GetPhase() != "ready" || connected.GetSessionId() == "" {
		t.Fatalf("connect failed %+v %v", connected, err)
	}
	if strings.Contains(connected.GetDiagnostic(), "fixture-credential") {
		t.Fatal("Connect response leaked secret")
	}
	current, err := service.Status(ctx, &tunnelv1.StatusRequest{SessionId: connected.GetSessionId()})
	if err != nil || current.GetPhase() != "ready" {
		t.Fatalf("health %+v %v", current, err)
	}
	if _, err := service.Connect(ctx, request); err == nil {
		t.Fatal("duplicate session started")
	}
	if _, err := service.Disconnect(ctx, &tunnelv1.DisconnectRequest{SessionId: connected.GetSessionId()}); err != nil {
		t.Fatal(err)
	}
	after, err := service.Status(ctx, &tunnelv1.StatusRequest{SessionId: connected.GetSessionId()})
	if err != nil || after.GetPhase() != "failed" {
		t.Fatalf("stopped session still ready: %+v %v", after, err)
	}
}

func TestRejectNonLoopbackMCPAndRedirect(t *testing.T) {
	t.Parallel()
	for _, target := range []string{
		"https://127.0.0.1:8080/mcp", "http://example.com:8080/mcp",
		"http://0.0.0.0:8080/mcp", "http://127.0.0.1:8080/not-mcp",
		"http://127.0.0.1:8080/mcp?token=foo", "http://user:pass@127.0.0.1:8080/mcp",
		"file:///tmp/data", "http://192.168.1.10:8080/mcp",
	} {
		if err := validateLoopbackEndpoint(target); err == nil {
			t.Errorf("unsafe endpoint accepted %q", target)
		}
	}
	server := httptest.NewServer(http.RedirectHandler("http://example.com/leak", http.StatusFound))
	defer server.Close()
	svc := newService()
	_, err := svc.Connect(context.Background(), &tunnelv1.ConnectRequest{
		TunnelId: "tun_0123456789abcdef0123456789abcdef", ExternalTunnelId: "test",
		McpEndpoint: server.URL + "/mcp", EphemeralCredential: []byte("fixture-secret"),
	})
	if err == nil {
		t.Fatal("MCP endpoint redirect was treated as ready")
	}
}
