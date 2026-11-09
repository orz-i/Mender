package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agent "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentdelegate "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	agentstore "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	auditstore "github.com/orz-i/Mender/backend/internal/audit/infrastructure/sqlite"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationrouter "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/router"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	policyapp "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	policylocal "github.com/orz-i/Mender/backend/internal/policy/infrastructure/local"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	supply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	configstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

type noStartAgentExecutor struct{}

func (noStartAgentExecutor) ExecuteConfigured(context.Context, agent.Agent, string, agentapp.RunConfig) (string, error) {
	return "", errors.New("offline MCP status fixture may not launch an ACP process")
}

func stagedAgentStatusCandidate(t *testing.T) (string, string, gov.Principal, agent.ID, agent.ID, catalog.ToolID) {
	t.Helper()
	ctx := context.Background()
	legacy := filepath.Join(t.TempDir(), "legacy.sqlite")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: legacy})
	if err != nil {
		t.Fatal(err)
	}
	owner := runtime.OwnerPrincipal()
	principal, err := gov.NewContext(gov.OriginCLI, owner, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtime.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Candidate Alpha", Backend: agent.BackendGrok, WorkspaceRoot: t.TempDir(), Isolation: "host",
	}, principal)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Candidate Beta", Backend: agent.BackendGrok, WorkspaceRoot: t.TempDir(), Isolation: "host",
	}, principal)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := runtime.AgentTools(ctx, first.ID(), principal)
	if err != nil {
		t.Fatal(err)
	}
	var statusID catalog.ToolID
	for _, entry := range tools.Tools {
		if entry.SourceKey == "agent.status" {
			statusID, err = catalog.ParseToolID(entry.ToolID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if statusID == "" {
		t.Fatal("missing Agent status canonical ToolID")
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	manifest, err := hardcut.AssessPath(ctx, legacy)
	if err != nil || manifest.Status != "ready" {
		t.Fatalf("disposable legacy Agent mapping blocked: %+v %v", manifest.Conflicts, err)
	}
	artifact, err := hardcut.StageCopy(ctx, legacy, filepath.Join(t.TempDir(), "typed.sqlite"), manifest.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	if err := hardcut.VerifyStagedCopy(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	return legacy, artifact.CandidatePath, owner, first.ID(), second.ID(), statusID
}

func TestTypedCandidateOwnedAgentStatusUsesSingleGovernedInvocationAndMCP(t *testing.T) {
	ctx := context.Background()
	legacy, path, owner, first, second, statusID := stagedAgentStatusCandidate(t)
	cat, err := NewTypedCatalog(ctx, CatalogOptions{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cat.Close() }()
	profiles, err := NewTypedProfiles(ctx, path, cat.store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = profiles.Close() }()
	policies, err := NewTypedPolicies(ctx, path, cat, profiles)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = policies.Close() }()
	profile, err := profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "candidate-agent", DisplayName: "Candidate Agent"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: profile.Slug(), ToolID: statusID, Alias: "status"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.ExecContext(ctx, `INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision)
 VALUES(?,'direct','',0) ON CONFLICT(profile_id) DO UPDATE SET mode='direct'`, profile.ID()); err != nil {
		t.Fatal(err)
	}
	agents, err := agentstore.OpenTyped(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = agents.Close() }()
	// Both Agents may be granted to one Profile. Its selected Tool is still
	// owned only by Alpha, and Beta's TaskID may never be read through it.
	for _, id := range []agent.ID{first, second} {
		if err := agents.GrantProfile(ctx, id, profile.ID().String()); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	newTask := func(id agent.ID) agent.TaskID {
		t.Helper()
		taskID, err := agent.NewTaskID()
		if err != nil {
			t.Fatal(err)
		}
		task := agent.Task{ID: taskID, AgentID: id, Status: agent.TaskQueued, Origin: "mcp",
			PrincipalKind: string(gov.PrincipalService), PrincipalRef: "profile:" + profile.ID().String(),
			ScopeKind: string(gov.ScopeProfile), ScopeRef: profile.ID().String(), Generation: 1,
			TimeoutSeconds: 600, CreatedAt: now, UpdatedAt: now}
		if err := agents.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
		return taskID
	}
	firstTask := newTask(first)
	secondTask := newTask(second)
	runtimeGovernance, err := NewRuntimeGovernance(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtimeGovernance.Close() }()
	credentials, err := NewCredentials(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = credentials.Close() }()
	audit, err := auditstore.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = audit.Close() }()
	providerConfigs, err := configstore.OpenTyped(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = providerConfigs.Close() }()
	reader, err := supply.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	service, _, _, err := composeAgentService(ctx, agents, owner, profiles, policies, runtimeGovernance, noStartAgentExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	executor, err := agentdelegate.NewOwnedExecutor(service, first)
	if err != nil {
		t.Fatal(err)
	}
	routers := invocationrouter.New()
	source, err := catalog.AgentSource(string(first))
	if err != nil {
		t.Fatal(err)
	}
	if err := routers.RegisterSource(source, executor); err != nil {
		t.Fatal(err)
	}
	_, invoke := (runtimeInvocationWiring{
		options: RuntimeOptions{DefaultTimeout: 5 * time.Second}, tools: cat.store, profileScopes: reader,
		facts:  newInvocationGovernanceFactExtractor(providerConfigs, providerplugin.NewRegistry(), routers),
		policy: policies, governance: runtimeGovernance, executors: routers,
		credentials: credentials, audit: audit, approval: policylocal.RejectApproval{},
	}).compose()
	adapter, err := mcpserver.NewTyped(ctx, profile.ID().String(), mcpserver.TypedDependencies{
		Profiles: reader, DescribeTool: cat.DescribeTool.Execute, InvokeTool: invoke.Execute,
	})
	if err != nil {
		t.Fatal(err)
	}
	transportClient, transportServer := mcp.NewInMemoryTransports()
	serverSession, err := adapter.MCPServer().Connect(ctx, transportServer, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "candidate-invocation", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, transportClient, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clientSession.Close() }()
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 1 {
		t.Fatalf("Agent MCP Direct list mismatch: %+v %v", tools, err)
	}
	name := tools.Tools[0].Name
	approved, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"taskId": firstTask}})
	if err != nil || approved.IsError {
		t.Fatalf("canonical Agent Task status MCP call rejected: %+v %v", approved, err)
	}
	body, ok := approved.StructuredContent.(map[string]any)
	if !ok || body["taskId"] != string(firstTask) || body["status"] != "queued" {
		t.Fatalf("Agent Task status did not come from existing TaskService: %#v", approved.StructuredContent)
	}
	var count int
	var provider sql.NullString
	var auditScope, kind string
	err = db.QueryRowContext(ctx, `SELECT provider_id,scope_reference,status FROM invocation_audit
 WHERE tool_id=? AND status='completed' ORDER BY finished_at DESC LIMIT 1`, statusID.String()).Scan(&provider, &auditScope, &kind)
	if err != nil || provider.Valid || auditScope != profile.ID().String() || kind != "completed" {
		t.Fatalf("Agent Tool got fake Provider or lost audit: provider=%+v scope=%s status=%s err=%v", provider, auditScope, kind, err)
	}
	// Native ToolBinding alone never grants Workspace/Sandbox authority.
	// The same canonical Invocation must reject it without a TargetScope,
	// and it must not reserve or start an execution environment.
	var nativeReadID catalog.ToolID
	knownNative, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range knownNative {
		if entry.SourceKey == "workspace.read" {
			nativeReadID = entry.ToolID
		}
	}
	if nativeReadID == "" {
		t.Fatal("pinned native Workspace Tool not found")
	}
	if _, err := profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
		ProfileSlug: profile.Slug(), ToolID: nativeReadID,
	}); err != nil {
		t.Fatal(err)
	}
	servicePrincipal, err := gov.NewPrincipal(gov.PrincipalService, "profile:"+profile.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	profileScope, err := gov.ProfileScope(profile.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginMCP, servicePrincipal, profileScope)
	if err != nil {
		t.Fatal(err)
	}
	native, err := invoke.Execute(ctx, invocationapp.InvokeToolRequest{
		ToolID: nativeReadID, Arguments: []byte(`{"path":"README.md"}`), Context: caller,
	})
	if err == nil && native.Outcome.Kind() == "complete" {
		t.Fatalf("Native ToolBinding without TargetScope gained Sandbox execution: %+v", native)
	}
	var nativeLeases int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_execution_leases").Scan(&nativeLeases); err != nil || nativeLeases != 0 {
		t.Fatalf("ungranted native execution reserved a Sandbox lease: %d %v", nativeLeases, err)
	}
	// The same Profile is granted Beta but cannot inspect its Task via Alpha's
	// owned executor. Any error is a terminal audited failure, never a fallback.
	crossed, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"taskId": secondTask}})
	if err == nil && crossed != nil && !crossed.IsError {
		t.Fatalf("Alpha Tool disclosed Beta Task: %+v", crossed)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM invocation_audit WHERE tool_id=?", statusID.String()).Scan(&count); err != nil || count < 2 {
		t.Fatalf("failed cross-Agent attempt had no terminal audit: %d %v", count, err)
	}
	// Policy denies the exact canonical ToolID, not all Provider or Agent Tools.
	rule, err := policies.CreateRule.Execute(ctx, policyapp.RuleInput{
		DisplayName: "Block candidate status", Enabled: true, Decision: policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{Actions: []gov.Action{gov.ActionToolInvoke}, ToolIDs: []string{statusID.String()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	denied, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"taskId": firstTask}})
	if err == nil && denied != nil && !denied.IsError {
		t.Fatalf("Policy-denied Agent Tool still returned result: %+v", denied)
	}
	var deniedCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM invocation_audit WHERE tool_id=? AND status='denied'", statusID.String()).Scan(&deniedCount); err != nil || deniedCount == 0 {
		t.Fatalf("Policy-denied invocation not audited: n=%d err=%v", deniedCount, err)
	}
	// Remove the Policy deny so that revocation is independently exercised;
	// otherwise a previously denied call could masquerade as a grant check.
	if err := policies.DeleteRule.Execute(ctx, rule.ID()); err != nil {
		t.Fatal(err)
	}
	// Removing the Agent grant removes the direct MCP tool from the current
	// Profile projection even if its old tools/list response is cached.
	if err := agents.RevokeProfile(ctx, first, profile.ID().String()); err != nil {
		t.Fatal(err)
	}
	revoked, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"taskId": firstTask}})
	if err == nil && revoked != nil && !revoked.IsError {
		t.Fatalf("revoked Agent MCP Tool was still executable: %+v", revoked)
	}
	current, err := reader.Project(ctx, profile.ID().String())
	if err != nil || len(current.Sources) != 0 || len(current.Direct) != 0 {
		t.Fatalf("revoked Agent still visible in current Profile: %+v %v", current, err)
	}
	direct, err := invoke.Execute(ctx, invocationapp.InvokeToolRequest{
		ToolID: statusID, Arguments: []byte(`{"taskId":"` + string(firstTask) + `"}`), Context: caller})
	if err == nil && direct.Outcome.Kind() == "complete" {
		t.Fatalf("direct Invocation bypassed revoked Agent grant: %+v", direct)
	}
	var afterRevokeCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM invocation_audit WHERE tool_id=? AND status='denied'", statusID.String()).Scan(&afterRevokeCount); err != nil || afterRevokeCount <= deniedCount {
		t.Fatalf("revoked canonical Invocation did not record a separate terminal denial: %d %v", afterRevokeCount, err)
	}
	// All work above was carried out solely on the migrated disposable copy.
	fresh, err := hardcut.AssessPath(ctx, legacy)
	if err != nil || fresh.Status != "ready" {
		t.Fatalf("candidate execution mutated v2 source: %v %+v", err, fresh.Conflicts)
	}
}
