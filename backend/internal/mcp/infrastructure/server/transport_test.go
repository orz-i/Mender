package server

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestStreamableHTTPHandlerNegotiatesLatestStatelessProtocol(t *testing.T) {
	t.Parallel()

	fixture := newServerFixture(t)
	profile := fixture.profile(t, profiledomain.ExposureDynamic, []profiledomain.ToolBindingSpec{
		{ToolID: fixture.tools[0].ID()},
	})
	adapter := fixture.adapter(t, profile)

	httpServer := httptest.NewServer(adapter.StreamableHTTPHandler())
	t.Cleanup(httpServer.Close)

	client := mcp.NewClient(
		&mcp.Implementation{Name: "mender-http-test", Version: "1.0.0"},
		nil,
	)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL,
		HTTPClient:           httpServer.Client(),
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	initialize := session.InitializeResult()
	if initialize == nil {
		t.Fatal("InitializeResult() = nil")
	}
	if got, want := initialize.ProtocolVersion, "2026-07-28"; got != want {
		t.Fatalf("ProtocolVersion = %q, want %q", got, want)
	}

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(result.Tools) != 3 {
		t.Fatalf("ListTools() count = %d, want 3", len(result.Tools))
	}
}
