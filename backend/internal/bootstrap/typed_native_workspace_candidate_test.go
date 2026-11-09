package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	auditstore "github.com/orz-i/Mender/backend/internal/audit/infrastructure/sqlite"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationrouter "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/router"
	mcpmanaged "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/managed"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	policylocal "github.com/orz-i/Mender/backend/internal/policy/infrastructure/local"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	supply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	configstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
	localbackend "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/localworkspace"
	sandboxregistry "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/pluginregistry"
	sandboxrouter "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/router"
	sandboxstore "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	"github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/typedlease"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	tunnelapp "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
	"github.com/orz-i/Mender/backend/internal/workspace/infrastructure/sandboxbridge"
)

func TestTypedCandidateNativeWorkspaceReadsThroughSandboxLeaseAndMCP(t *testing.T) {
	ctx := context.Background()
	legacy := filepath.Join(t.TempDir(), "legacy.sqlite")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("Mender native candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	existing, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: legacy})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := gov.NewContext(gov.OriginCLI, existing.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	sb, err := existing.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "Local candidate workspace", Root: root, Isolation: sandbox.IsolationHost,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := existing.EnsureSandboxWorkspaceTools(ctx, sb.ID(), owner); err != nil {
		t.Fatal(err)
	}
	if err := existing.Close(); err != nil {
		t.Fatal(err)
	}
	assessment, err := hardcut.AssessPath(ctx, legacy)
	if err != nil || assessment.Status != "ready" {
		t.Fatalf("local candidate assessment blocked: %+v %v", assessment.Conflicts, err)
	}
	stage, err := hardcut.StageCopy(ctx, legacy, filepath.Join(t.TempDir(), "candidate.sqlite"), assessment.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	if err := hardcut.VerifyStagedCopy(ctx, stage); err != nil {
		t.Fatal(err)
	}
	// Test modifies only the disposable candidate after verifying its Stage E
	// provenance. Those writes are NOT a reviewed production migration.
	db, err := hardcut.OpenTypedContextStore(ctx, stage.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var wsRef string
	if err := db.QueryRowContext(ctx, "SELECT workspace_id FROM workspace_placements WHERE sandbox_id=?", sb.ID().String()).Scan(&wsRef); err != nil {
		t.Fatal(err)
	}
	cat, err := NewTypedCatalog(ctx, CatalogOptions{DatabasePath: stage.CandidatePath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cat.Close() }()
	profiles, err := NewTypedProfiles(ctx, stage.CandidatePath, cat.store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = profiles.Close() }()
	policies, err := NewTypedPolicies(ctx, stage.CandidatePath, cat, profiles)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = policies.Close() }()
	profile, err := profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "workspace-candidate", DisplayName: "Workspace candidate"})
	if err != nil {
		t.Fatal(err)
	}
	var readID catalog.ToolID
	tools, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range tools {
		if entry.SourceKey == "workspace.read" {
			readID = entry.ToolID
		}
	}
	if readID == "" {
		t.Fatal("canonical Workspace read Tool absent")
	}
	if _, err := profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: profile.Slug(), ToolID: readID}); err != nil {
		t.Fatal(err)
	}
	// The Stage E schema mandates distinct Sandbox + Workspace grants before
	// a TargetScope row can exist. This is a disposable owner-reviewed test
	// fixture, never a public/automatic grant path.
	for _, entry := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO mcp_delivery_configs(profile_id,mode,tunnel_id,revision) VALUES(?,'direct','',0)`, []any{profile.ID().String()}},
		{`INSERT INTO mcp_delivery_direct_tools(profile_id,tool_id) VALUES(?,?)`, []any{profile.ID().String(), readID.String()}},
		{`INSERT INTO sandbox_profile_grants(sandbox_id,profile_id,capability,granted_at) VALUES(?,?,'file.read','test')`, []any{sb.ID().String(), profile.ID().String()}},
		{`INSERT INTO workspace_profile_grants(workspace_id,profile_id,granted_at) VALUES(?,?,'test')`, []any{wsRef, profile.ID().String()}},
		{`INSERT INTO profile_tool_target_scopes(profile_id,tool_id,workspace_id,sandbox_id) VALUES(?,?,?,?)`, []any{profile.ID().String(), readID.String(), wsRef, sb.ID().String()}},
	} {
		if _, err := db.ExecContext(ctx, entry.query, entry.args...); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := sandboxbridge.NewTypedInventory(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := workspaceapp.NewWorkspaceSelection(inventory)
	if err != nil {
		t.Fatal(err)
	}
	placements, err := workspaceapp.NewPlacementQuery(inventory)
	if err != nil {
		t.Fatal(err)
	}
	sandboxSQL, err := sandboxstore.OpenTyped(ctx, stage.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sandboxSQL.Close() }()
	governanceRuntime, err := NewRuntimeGovernance(ctx, stage.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = governanceRuntime.Close() }()
	backend, err := localbackend.New(sandboxWorkspaceShellLauncher{governance: governanceRuntime})
	if err != nil {
		t.Fatal(err)
	}
	registry := sandboxregistry.NewRegistry()
	descriptor, err := sandboxregistry.BuiltinDescriptor(backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(backend, descriptor); err != nil {
		t.Fatal(err)
	}
	admission := sandboxapp.NewAdmit(policies.Evaluate, policylocal.RejectApproval{}, sandboxSQL, sandboxidentity.Generator{}, systemClock{}, sandboxSQL)
	leaseStore, err := typedlease.New(ctx, db, invocationapp.ApprovedAttemptVerifier{})
	if err != nil {
		t.Fatal(err)
	}
	router, err := sandboxrouter.New(sandboxSQL, sandboxSQL, admission, leaseStore, registry)
	if err != nil {
		t.Fatal(err)
	}
	guarded, err := sandboxbridge.NewGuardedRouter(sandboxbridge.GuardedDependencies{
		Placements: placements, Router: router, Backends: registry, Grants: sandboxSQL,
		Workspaces: inventory, Bindings: inventory, Roots: sandboxbridge.LocalRootAttestor{},
		Admissions: invocationapp.ApprovedAttemptVerifier{},
	})
	if err != nil {
		t.Fatal(err)
	}
	native := workspaceplugin.NativeWorkspaceTools{Selection: selection, Router: guarded, RequiredIsolation: sandbox.IsolationHost}
	invRouter := invocationrouter.New()
	source, err := catalog.NativeSource("workspace")
	if err != nil {
		t.Fatal(err)
	}
	if err := invRouter.RegisterSource(source, native); err != nil {
		t.Fatal(err)
	}
	credentials, err := NewCredentials(ctx, stage.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = credentials.Close() }()
	audit, err := auditstore.Open(ctx, stage.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = audit.Close() }()
	config, err := configstore.OpenTyped(ctx, stage.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = config.Close() }()
	reader, err := supply.Open(ctx, stage.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	_, invoker := (runtimeInvocationWiring{options: RuntimeOptions{DefaultTimeout: 8 * time.Second},
		tools: cat.store, profileScopes: reader,
		facts:  newInvocationGovernanceFactExtractor(config, providerplugin.NewRegistry(), invRouter),
		policy: policies, governance: governanceRuntime, executors: invRouter,
		credentials: credentials, audit: audit, approval: policylocal.RejectApproval{},
	}).compose()
	adapter, err := mcpserver.NewTyped(ctx, profile.ID().String(), mcpserver.TypedDependencies{
		Profiles: reader, DescribeTool: cat.DescribeTool.Execute, InvokeTool: invoker.Execute,
	})
	if err != nil {
		t.Fatal(err)
	}
	clientSide, serverSide := mcp.NewInMemoryTransports()
	serverSession, err := adapter.MCPServer().Connect(ctx, serverSide, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "typed-native-candidate", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientSide, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clientSession.Close() }()
	exports, err := clientSession.ListTools(ctx, nil)
	if err != nil || len(exports.Tools) != 1 {
		t.Fatalf("Native MCP direct export unavailable: %+v %v", exports, err)
	}
	call := &mcp.CallToolParams{Name: exports.Tools[0].Name, Arguments: map[string]any{"workspaceId": wsRef, "path": "README.md"}}
	outcome, err := clientSession.CallTool(ctx, call)
	if err != nil || outcome.IsError {
		t.Fatalf("Native Workspace MCP call failed: %+v %v", outcome, err)
	}
	body, ok := outcome.StructuredContent.(map[string]any)
	if !ok || body["content"] != "Mender native candidate" {
		t.Fatalf("actual Sandbox file read disagreed: %+v", outcome.StructuredContent)
	}
	var leaseCount, admissionCount, invocations int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_execution_leases").Scan(&leaseCount); err != nil || leaseCount != 0 {
		t.Fatalf("Sandbox lease leaked: %d %v", leaseCount, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_admissions WHERE sandbox_id=? AND action='sandbox.acquire' AND status='admitted'", sb.ID().String()).Scan(&admissionCount); err != nil || admissionCount != 1 {
		t.Fatalf("Sandbox admission audit absent: %d %v", admissionCount, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM invocation_audit WHERE tool_id=? AND provider_id IS NULL AND status='completed'", readID.String()).Scan(&invocations); err != nil || invocations != 1 {
		t.Fatalf("canonical Native Invocation audit absent: %d %v", invocations, err)
	}
	// A second granted Workspace must make implicit selection ambiguous,
	// even if its placement is not currently runnable. No list-order or host
	// fallback may silently choose the first eligible Workspace.
	otherWorkspace := "wrk_88888888888888888888888888888888"
	if _, err := db.ExecContext(ctx, `INSERT INTO workspaces
  (workspace_id,owner_kind,owner_ref,display_name,verified_root,created_at)
  VALUES(?,?,?,?,?,?)`, otherWorkspace, string(owner.Principal().Kind()),
		owner.Principal().Reference(), "Ambiguity proof", "", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO workspace_profile_grants(workspace_id,profile_id,granted_at)
  VALUES(?,?,'test')`, otherWorkspace, profile.ID().String()); err != nil {
		t.Fatal(err)
	}
	ambiguous, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: exports.Tools[0].Name, Arguments: map[string]any{"path": "README.md"},
	})
	if err == nil && ambiguous != nil && !ambiguous.IsError {
		t.Fatalf("implicit Workspace selection silently chose a target: %+v", ambiguous)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_admissions WHERE sandbox_id=? AND action='sandbox.acquire' AND status='admitted'", sb.ID().String()).Scan(&admissionCount); err != nil || admissionCount != 1 {
		t.Fatalf("ambiguous target was routed into Sandbox admission: %d %v", admissionCount, err)
	}
	// An explicit valid WorkspaceRef may still select the exact approved
	// placement; ambiguity never creates a global lockout or a new grant.
	explicit, err := clientSession.CallTool(ctx, call)
	if err != nil || explicit.IsError {
		t.Fatalf("explicit authorized Workspace refused after ambiguity: %+v %v", explicit, err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM workspace_profile_grants WHERE workspace_id=? AND profile_id=?", otherWorkspace, profile.ID().String()); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_execution_leases").Scan(&leaseCount); err != nil || leaseCount != 0 {
		t.Fatalf("explicit Workspace execution leaked a lease: %d %v", leaseCount, err)
	}
	// The SAME canonical v3 InvokeTool and Typed MCP projection can be
	// served through the existing managed MCP listener/refresh manager;
	// Native tools have no Provider activation and this fixture forbids all
	// unreviewed external Provider/Tunnel activation.
	delivery, err := newTypedMCPDelivery(ctx, stage.CandidatePath, profiles.GetByID, tunnelapp.GetTunnel{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = delivery.Close() }()
	load := func(ctx context.Context, p profiledomain.Profile) (mcpmanaged.Snapshot, error) {
		config, err := delivery.Get.Execute(ctx, p.ID())
		return mcpmanaged.Snapshot{Profile: p, Config: config}, err
	}
	manager, err := mcpmanaged.New(mcpmanaged.Dependencies{
		LoadBySlug: func(ctx context.Context, slug profiledomain.Slug) (mcpmanaged.Snapshot, error) {
			p, err := profiles.Get.Execute(ctx, slug)
			if err != nil {
				return mcpmanaged.Snapshot{}, err
			}
			return load(ctx, p)
		},
		LoadByID: func(ctx context.Context, id profiledomain.ProfileID) (mcpmanaged.Snapshot, error) {
			p, err := profiles.GetByID.Execute(ctx, id)
			if err != nil {
				return mcpmanaged.Snapshot{}, err
			}
			return load(ctx, p)
		},
		Revisions: func(ctx context.Context, id profiledomain.ProfileID) (mcpmanaged.Versions, error) {
			p, err := profiles.GetRevision.Execute(ctx, id)
			if err != nil {
				return mcpmanaged.Versions{}, err
			}
			d, err := delivery.GetRevision.Execute(ctx, id)
			return mcpmanaged.Versions{Profile: p, Delivery: d}, err
		},
		BuildHTTP: func(ctx context.Context, snapshot mcpmanaged.Snapshot, control gov.Context) (http.Handler, error) {
			if control != owner {
				return nil, errors.New("unexpected managed MCP control principal")
			}
			selected, err := reader.Project(ctx, snapshot.ID().String())
			if err != nil {
				return nil, err
			}
			for _, src := range selected.Sources {
				if src.SourceKind != "native" {
					return nil, errors.New("test-only native managed MCP cannot activate a Provider or Agent")
				}
			}
			adapter, err := mcpserver.NewTyped(ctx, snapshot.ID().String(), mcpserver.TypedDependencies{
				Profiles: reader, DescribeTool: cat.DescribeTool.Execute, InvokeTool: invoker.Execute,
			})
			if err != nil {
				return nil, err
			}
			return adapter.StreamableHTTPHandler(), nil
		},
		DescribeTool:    cat.DescribeTool.Execute,
		CatalogRevision: cat.store.CatalogRevision,
		StartTunnel: func(context.Context, tunneldomain.TunnelID, string) (tunnelapp.Process, tunneldomain.Tunnel, error) {
			return nil, tunneldomain.Tunnel{}, errors.New("unexpected managed MCP Tunnel activation")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Close() }()
	instance, err := manager.Start(ctx, profile.Slug(), "127.0.0.1:0", owner)
	if err != nil {
		t.Fatalf("managed typed MCP listener unavailable: %v", err)
	}
	managedClient := mcp.NewClient(&mcp.Implementation{Name: "native-managed-candidate", Version: "1"}, nil)
	httpCtx, httpCancel := context.WithTimeout(ctx, 8*time.Second)
	defer httpCancel()
	managedSession, err := managedClient.Connect(httpCtx, &mcp.StreamableClientTransport{
		Endpoint: instance.Endpoint, HTTPClient: &http.Client{Timeout: 6 * time.Second}, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("managed typed MCP transport: %v", err)
	}
	defer func() { _ = managedSession.Close() }()
	managedExports, err := managedSession.ListTools(httpCtx, nil)
	if err != nil || len(managedExports.Tools) != 1 {
		t.Fatalf("managed typed MCP tools: %+v %v", managedExports, err)
	}
	managedCall := &mcp.CallToolParams{Name: managedExports.Tools[0].Name, Arguments: call.Arguments}
	managedResult, err := managedSession.CallTool(httpCtx, managedCall)
	if err != nil || managedResult.IsError {
		t.Fatalf("managed Native execution failed: %+v %v", managedResult, err)
	}
	managedBody, valid := managedResult.StructuredContent.(map[string]any)
	if !valid || managedBody["content"] != "Mender native candidate" {
		t.Fatalf("managed MCP bypassed native Sandbox read: %+v", managedResult.StructuredContent)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_execution_leases").Scan(&leaseCount); err != nil || leaseCount != 0 {
		t.Fatalf("managed MCP leaked Sandbox lease: %d %v", leaseCount, err)
	}
	// The old listed MCP tool is cached in the client, but authority must be
	// reloaded on every invocation and a revoked Workspace grant closes it.
	if _, err := db.ExecContext(ctx, "DELETE FROM workspace_profile_grants WHERE workspace_id=? AND profile_id=?", wsRef, profile.ID().String()); err != nil {
		t.Fatal(err)
	}
	revoked, err := clientSession.CallTool(ctx, call)
	if err == nil && revoked != nil && !revoked.IsError {
		t.Fatalf("Workspace grant revocation bypassed: %+v", revoked)
	}
	managedRevoked, err := managedSession.CallTool(httpCtx, managedCall)
	if err == nil && managedRevoked != nil && !managedRevoked.IsError {
		t.Fatalf("managed MCP listener exposed a revoked Workspace: %+v", managedRevoked)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_execution_leases").Scan(&leaseCount); err != nil || leaseCount != 0 {
		t.Fatalf("Sandbox lease created after revoke: %d %v", leaseCount, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_admissions WHERE sandbox_id=? AND action='sandbox.acquire' AND status='admitted'", sb.ID().String()).Scan(&admissionCount); err != nil || admissionCount != 3 {
		t.Fatalf("revoked call reacquired Sandbox: %d %v", admissionCount, err)
	}
	// No migrations or candidate swaps occurred on the real app database.
	again, err := hardcut.AssessPath(ctx, legacy)
	if err != nil || again.Status != "ready" {
		t.Fatalf("legacy source touched: %v %+v", err, again.Conflicts)
	}
}
