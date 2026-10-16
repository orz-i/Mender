package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestWorkspaceMultiTargetProfileMCPAuthorization(t *testing.T) {
	ctx := context.Background()
	r := newSuiteRuntime(t)
	owner := agentOwnerContext(t, r)
	source := createSuiteSandbox(t, r, "MCP Source")
	target := createSuiteSandbox(t, r, "MCP Target")
	for _, item := range []struct {
		root    string
		content string
	}{
		{source.Spec().LocalRoot, "source workspace"}, {target.Spec().LocalRoot, "target workspace"},
	} {
		if err := os.WriteFile(filepath.Join(item.root, "proof.txt"), []byte(item.content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	suite, err := r.EnsureSandboxWorkspaceTools(ctx, source.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.BindWorkspaceToolTarget(ctx, suite.ProviderID, "project-b", target.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	var readID catalogdomain.ToolID
	for _, tool := range suite.Tools {
		if tool.SourceKey == "workspace.read" {
			readID = catalogdomain.ToolID(tool.ToolID)
		}
	}
	if readID == "" {
		t.Fatal("read tool missing")
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "shared-tools", DisplayName: "Shared tools", Mode: profiledomain.ExposureDirect})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: profile.Slug(), ToolID: readID, Direct: true})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := r.NewMCPServer(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := adapter.MCPServer().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "multi-target-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	available, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	for _, tool := range available.Tools {
		if strings.HasSuffix(tool.Name, "__read") {
			name = tool.Name
			schema, _ := json.Marshal(tool.InputSchema)
			if !strings.Contains(string(schema), `"workspace"`) {
				t.Fatalf("Tool schema missing target selector: %s", schema)
			}
		}
	}
	if name == "" {
		t.Fatal("read tool absent from Profile MCP")
	}
	read := func(args string) (bool, string) {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: json.RawMessage(args)})
		if err != nil {
			// A preflight-denied selection may be reported as a Tool-call
			// transport error instead of an outcome envelope. It must never
			// produce data or execute the denied target.
			return false, err.Error()
		}
		encoded, _ := json.Marshal(result)
		if result.IsError {
			return false, string(encoded)
		}
		if value, ok := result.StructuredContent.(map[string]any); ok {
			if value["kind"] == "failed" || value["kind"] == "rejected" {
				return false, string(encoded)
			}
		}
		return true, string(encoded)
	}
	const pinned = `{"path":"proof.txt"}`
	const selected = `{"workspace":"project-b","path":"proof.txt"}`
	if ok, _ := read(selected); ok {
		t.Fatal("profile accessed additional sandbox without grant")
	}
	if err := r.Sandboxes.SetProfileGrants(ctx, source.ID(), profile.ID().String(), []sandboxdomain.Capability{sandboxdomain.CapabilityRead}, owner); err != nil {
		t.Fatal(err)
	}
	if ok, _ := read(pinned); !ok {
		t.Fatal("default read denied after grant")
	}
	if ok, _ := read(selected); ok {
		t.Fatal("default Sandbox grant leaked to alternate target")
	}
	if err := r.Sandboxes.SetProfileGrants(ctx, target.ID(), profile.ID().String(), []sandboxdomain.Capability{sandboxdomain.CapabilityRead}, owner); err != nil {
		t.Fatal(err)
	}
	if ok, output := read(selected); !ok || !strings.Contains(output, "target workspace") {
		t.Fatalf("authorized read routed to wrong Sandbox: %s", output)
	}
	// The ToolID and alias are shared, but the Sandbox grant remains bound
	// to the exact Profile. A second Profile cannot borrow the first grant.
	other, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "other-shared-tools", DisplayName: "Other profile", Mode: profiledomain.ExposureDirect,
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
		ProfileSlug: other.Slug(), ToolID: readID, Direct: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherAdapter, err := r.NewMCPServer(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	otherClientTransport, otherServerTransport := mcp.NewInMemoryTransports()
	otherServer, err := otherAdapter.MCPServer().Connect(ctx, otherServerTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = otherServer.Close() }()
	otherClient := mcp.NewClient(&mcp.Implementation{Name: "other-workspace-profile", Version: "1"}, nil)
	otherSession, err := otherClient.Connect(ctx, otherClientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = otherSession.Close() }()
	otherTools, err := otherSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherName := ""
	for _, tool := range otherTools.Tools {
		if strings.HasSuffix(tool.Name, "__read") {
			otherName = tool.Name
		}
	}
	if otherName == "" {
		t.Fatal("other Profile was not bound to the same read Tool")
	}
	response, err := otherSession.CallTool(ctx, &mcp.CallToolParams{
		Name: otherName, Arguments: json.RawMessage(selected),
	})
	if err == nil && response != nil && !response.IsError {
		if content, ok := response.StructuredContent.(map[string]any); !ok ||
			(content["kind"] != "rejected" && content["kind"] != "failed") {
			t.Fatal("a second Profile read the first Profile's selected workspace")
		}
	}
	if err := r.Sandboxes.SetProfileGrants(ctx, target.ID(), profile.ID().String(), nil, owner); err != nil {
		t.Fatal(err)
	}
	if ok, _ := read(selected); ok {
		t.Fatal("revoked target Sandbox still readable")
	}
	if err := r.UnbindWorkspaceToolTarget(ctx, suite.ProviderID, "project-b", owner); err != nil {
		t.Fatal(err)
	}
	if ok, _ := read(selected); ok {
		t.Fatal("unbound workspace alias still executable")
	}
}
