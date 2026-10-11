package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

// Full in-memory MCP transport invokes the real Runtime/Workspace/lease chain,
// not the local user's Sandbox management API or a fake sandbox executor.
func TestMCPProfileSandboxDelegationReadAndRevoke(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "temp.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "proof.txt"), []byte("delegated read works"), 0600); err != nil {
		t.Fatal(err)
	}
	owner, err := gov.NewContext(gov.OriginDesktop, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{Name: "Temporary Delegation", Root: root, Isolation: sandboxdomain.IsolationHost}, owner)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(workspaceplugin.Config{SandboxID: sandbox.ID().String()})
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{Namespace: "delegation", DisplayName: "Delegation"}, workspaceplugin.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = syncConfiguredProviderTest(t, r, ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	tools, err := r.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	var readID, editID string
	for _, tool := range tools {
		if tool.SourceKey() == "workspace.read" {
			readID = tool.ID().String()
		}
		if tool.SourceKey() == "workspace.edit" {
			editID = tool.ID().String()
		}
	}
	if readID == "" || editID == "" {
		t.Fatal("missing Workspace Tools")
	}
	makeProfile := func(slug string) profiledomain.Profile {
		p, e := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: slug, DisplayName: slug, Mode: profiledomain.ExposureDirect})
		if e != nil {
			t.Fatal(e)
		}
		for _, id := range []string{readID, editID} {
			p, e = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: p.Slug(), ToolID: catalogdomain.ToolID(id), Direct: true})
			if e != nil {
				t.Fatal(e)
			}
		}
		return p
	}
	profile := makeProfile("delegate-a")
	other := makeProfile("delegate-b")
	openClient := func(p profiledomain.Profile) (*mcp.ClientSession, string, string) {
		t.Helper()
		adapter, e := r.NewMCPServer(ctx, p)
		if e != nil {
			t.Fatal(e)
		}
		cTransport, sTransport := mcp.NewInMemoryTransports()
		serverSession, e := adapter.MCPServer().Connect(ctx, sTransport, nil)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = serverSession.Close() })
		client := mcp.NewClient(&mcp.Implementation{Name: "delegation-test", Version: "1.0"}, nil)
		session, e := client.Connect(ctx, cTransport, nil)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = session.Close() })
		list, e := session.ListTools(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		var read, edit string
		for _, tool := range list.Tools {
			if strings.HasSuffix(tool.Name, "__read") {
				read = tool.Name
			}
			if strings.HasSuffix(tool.Name, "__edit") {
				edit = tool.Name
			}
		}
		if read == "" || edit == "" {
			t.Fatalf("missing listed tools %+v", list.Tools)
		}
		return session, read, edit
	}
	client, read, edit := openClient(profile)
	otherClient, otherRead, _ := openClient(other)
	call := func(session *mcp.ClientSession, name string, args string) bool {
		t.Helper()
		resp, e := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: json.RawMessage(args)})
		if e != nil {
			t.Fatalf("MCP call transport %v", e)
		}
		if resp.IsError {
			return false
		}
		if val, ok := resp.StructuredContent.(map[string]any); ok {
			if val["kind"] == "failed" || val["kind"] == "rejected" {
				return false
			}
		}
		return true
	}
	if call(client, read, `{"path":"proof.txt"}`) {
		t.Fatal("MCP read without delegation succeeded")
	}
	if err := r.Sandboxes.SetProfileGrants(ctx, sandbox.ID(), profile.ID().String(), []sandboxdomain.Capability{sandboxdomain.CapabilityRead}, owner); err != nil {
		t.Fatal(err)
	}
	if !call(client, read, `{"path":"proof.txt"}`) {
		t.Fatal("explicitly granted MCP read failed")
	}
	if call(client, edit, `{"path":"proof.txt","content":"not allowed"}`) {
		t.Fatal("edit without grant succeeded")
	}
	if call(otherClient, otherRead, `{"path":"proof.txt"}`) {
		t.Fatal("other Profile used grant")
	}
	if err := r.Sandboxes.SetProfileGrants(ctx, sandbox.ID(), profile.ID().String(), nil, owner); err != nil {
		t.Fatal(err)
	}
	if call(client, read, `{"path":"proof.txt"}`) {
		t.Fatal("MCP read after revocation succeeded")
	}
	// #nosec G304 -- root is an isolated temporary test fixture.
	data, err := os.ReadFile(filepath.Join(root, "proof.txt"))
	if err != nil || string(data) != "delegated read works" {
		t.Fatalf("host file changed: %s %v", data, err)
	}
}
