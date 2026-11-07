package server_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	"github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sourcecatalog"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	server "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	migration "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	supply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
	_ "modernc.org/sqlite"
)

const typedProfileID = "prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type trustedCandidateOwner struct{}

func (trustedCandidateOwner) VerifySourceOwner(context.Context, catalog.SourceRef) error { return nil }

func TestTypedMCPProjectsRealOfflineCandidateAndRevocation(t *testing.T) {
	ctx := context.Background()
	legacy := filepath.Join(t.TempDir(), "legacy.sqlite")
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: legacy})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := governance.NewContext(governance.OriginCLI, runtime.OwnerPrincipal(), governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	agent, err := runtime.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Candidate Agent", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := runtime.AgentTools(ctx, agent.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	var delegateID string
	for _, tool := range tools.Tools {
		if tool.SourceKey == "agent.delegate" {
			delegateID = tool.ToolID
		}
	}
	if delegateID == "" {
		t.Fatal("candidate Agent has no delegate Tool")
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := migration.AssessPath(ctx, legacy)
	if err != nil || report.Status != "ready" {
		t.Fatalf("offline assessment: %+v %v", report.Conflicts, err)
	}
	candidate, err := migration.StageCopy(ctx, legacy, filepath.Join(t.TempDir(), "typed.sqlite"), report.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", candidate.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO profiles(id,slug,display_name,revision) VALUES(?,'candidate','Candidate',0)`, []any{typedProfileID}},
		{`INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision) VALUES(?,'direct','',0)`, []any{typedProfileID}},
		{`INSERT INTO profile_tools(profile_id,tool_id,alias,title_override,description_override,policy_constraint,concurrency_constraint,credential_refs_json) VALUES(?,?,'delegate','','','inherit','inherit','[]')`, []any{typedProfileID, delegateID}},
		{`INSERT INTO agent_grants(agent_id,profile_id,created_at) VALUES(?,?,'now')`, []any{string(agent.ID()), typedProfileID}},
	} {
		if _, err := db.ExecContext(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := supply.Open(ctx, candidate.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	cat, err := sourcecatalog.NewRepository(ctx, db, trustedCandidateOwner{})
	if err != nil {
		t.Fatal(err)
	}
	var called int
	invoke := func(_ context.Context, request invocation.InvokeToolRequest) (invocation.InvokeToolResponse, error) {
		called++
		if request.ToolID.String() != delegateID || request.Context.Scope().Reference() != typedProfileID {
			t.Fatalf("cross-Profile or ToolID execution: %+v", request)
		}
		payload, _ := invocationdomain.NewJSONValue([]byte(`{"taskId":"atask_fixture","status":"queued"}`))
		out, err := invocationdomain.Complete(payload)
		return invocation.InvokeToolResponse{Outcome: out}, err
	}
	adapter, err := server.NewTyped(ctx, typedProfileID, server.TypedDependencies{
		Profiles: reader, DescribeTool: cat.GetTool, InvokeTool: invoke,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := connectTypedCandidate(t, adapter)
	names := listToolNames(t, client)
	if len(names) != 1 || names[0] == "" {
		t.Fatalf("candidate export did not use Agent SourceRef: %v", names)
	}
	toolCall := &mcp.CallToolParams{Name: names[0], Arguments: map[string]any{"prompt": "verify only"}}
	if result, err := client.CallTool(ctx, toolCall); err != nil || result.IsError || called != 1 {
		t.Fatalf("verified candidate call=%+v err=%v invocations=%d", result, err, called)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM agent_grants WHERE agent_id=? AND profile_id=?", string(agent.ID()), typedProfileID); err != nil {
		t.Fatal(err)
	}
	if result, err := client.CallTool(ctx, toolCall); err == nil && (result == nil || !result.IsError) {
		t.Fatalf("revoked Agent grant still invoked: %+v", result)
	}
	if called != 1 {
		t.Fatalf("revocation did not fence Invocation: calls=%d", called)
	}
	// The candidate remains independent; the legacy source DB is never
	// opened by a future Schema adapter or replaced by this fixture.
	if _, err := supply.Open(ctx, legacy); err == nil {
		t.Fatal("typed supply opened the legacy Provider-v2 source")
	}
}

func connectTypedCandidate(t *testing.T, adapter *server.Adapter) *mcp.ClientSession {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := adapter.MCPServer().Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "typed-candidate-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func listToolNames(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names
}
