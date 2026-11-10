package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
)

// Run explicitly with MENDER_PHASE46_REAL_AGENT_BACKEND=opencode (or grok).
// This is a REAL model-facing smoke test, never part of the default suite.
// It uses temporary resources only and never relaxes Seatbelt or imports auth.
type realACPAcceptanceRunner struct {
	agentacp.Runner
	evidence chan string
}

func (r realACPAcceptanceRunner) ExecuteConfigured(ctx context.Context, a agentdomain.Agent, prompt string, cfg agentapp.RunConfig) (string, error) {
	stage := "governed_http_supplied"
	if cfg.ToolSupply.IsZero() {
		stage = "missing_governed_tool_supply"
	}
	result, err := r.Runner.ExecuteConfigured(ctx, a, prompt, cfg)
	switch {
	case err == nil:
		stage += ":completed"
	case errors.Is(err, context.Canceled):
		stage += ":cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		stage += ":deadline"
	case strings.Contains(err.Error(), "ACP Agent does not support stable protocol"):
		stage += ":protocol_version"
	case strings.Contains(err.Error(), "does not advertise ACP v1 HTTP MCP"):
		stage += ":http_mcp_not_advertised"
	case strings.Contains(err.Error(), "ACP initialize failed"):
		stage += ":initialize_rejected"
	case strings.Contains(err.Error(), "ACP session/new failed"):
		stage += ":session_new_rejected"
	case strings.Contains(err.Error(), "ACP session/prompt failed"):
		stage += ":prompt_rejected"
	case strings.Contains(err.Error(), "ACP server closed before response"):
		stage += ":closed_before_response"
	case strings.Contains(err.Error(), "start ACP Agent"):
		stage += ":process_start_rejected"
	default:
		stage += ":other_rejected"
	}
	// A bounded, non-secret classification, not the raw ACP output or bearer.
	select {
	case r.evidence <- stage:
	default:
	}
	return result, err
}

func TestPhase46OptInRealRestrictedAgentGovernedTools(t *testing.T) {
	backend := agentdomain.Backend(os.Getenv("MENDER_PHASE46_REAL_AGENT_BACKEND"))
	if backend == "" {
		t.Skip("real model inference is opt-in only")
	}
	if backend != agentdomain.BackendOpenCode && backend != agentdomain.BackendGrok {
		t.Fatal("choose an explicitly supported backend: opencode or grok")
	}
	if runtime.GOOS != "darwin" || !processisolation.New().Info().RestrictedHostAvailable {
		t.Skip("macOS restricted_host process acceptance only")
	}
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "disposable-governed-acceptance.db")
	if err := os.WriteFile(filepath.Join(root, "review.txt"), []byte("phase46 disposable review marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	factory, err := agentacp.NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := factory.Availability(backend); err != nil {
		t.Fatalf("real Agent CLI unavailable: %v", err)
	}
	receipt := make(chan string, 1)
	runner := realACPAcceptanceRunner{Runner: agentacp.Runner{
		NewCommand:            factory.Command,
		NewCommandInWorkspace: factory.CommandInWorkspace,
		NewCommandWithConfig:  factory.CommandWithConfig,
	}, evidence: receipt}
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: db, AgentExecutor: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	sandbox, err := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "Phase46 disposable restricted root", Root: root, Isolation: sandboxdomain.IsolationRestrictedHost,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := r.SandboxWorkspaceTools(ctx, sandbox.ID(), owner)
	if err != nil || suite.Status != "ready" {
		t.Fatalf("restricted Workspace tools unprepared: %v", err)
	}
	ids := map[string]catalogdomain.ToolID{}
	for _, tool := range suite.Tools {
		id, e := catalogdomain.ParseToolID(tool.ToolID)
		if e != nil {
			t.Fatal(e)
		}
		ids[tool.SourceKey] = id
	}
	profile, err := createMCPProfileFixture(t, r, ctx, profileapp.CreateProfileRequest{
		Slug: "real-agent-acceptance", DisplayName: "Real Agent disposable scope",
	}, "direct")

	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"workspace.read", "workspace.edit"} {
		profile, err = addMCPProfileToolFixture(t, r, ctx, profileapp.AddToolRequest{
			ProfileSlug: profile.Slug(), ToolID: ids[key],
		}, true)

		if err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Sandboxes.SetProfileGrants(ctx, sandbox.ID(), profile.ID().String(),
		[]sandboxdomain.Capability{sandboxdomain.CapabilityRead, sandboxdomain.CapabilityEdit}, owner); err != nil {
		t.Fatal(err)
	}
	// The v3 Runtime does not infer Workspace authority from an Agent cwd or
	// the Sandbox root. The live acceptance must configure the complete
	// trusted owner-attested Workspace → Placement → Profile grant → exact
	// Tool TargetScope chain just like a real user. No implicit grants.
	shared, err := r.Workspaces.RegisterLocal(ctx, owner, "ACP isolated project", root)
	if err != nil {
		t.Fatalf("real ACP Workspace registration: %v", err)
	}
	if err = r.Workspaces.LinkLocal(ctx, owner, shared.ID, sandbox.ID().String()); err != nil {
		t.Fatalf("real ACP exact Sandbox placement: %v", err)
	}
	if err = r.Workspaces.Grant(ctx, owner, shared.ID, profile.ID().String()); err != nil {
		t.Fatalf("real ACP Workspace Profile grant: %v", err)
	}
	for _, key := range []string{"workspace.read", "workspace.edit"} {
		if err = r.Workspaces.PutTarget(ctx, owner, workspaceapp.TargetRecord{
			ProfileID: profile.ID().String(), ToolID: ids[key], WorkspaceID: shared.ID.String(), SandboxID: sandbox.ID().String(),
		}); err != nil {
			t.Fatalf("real ACP %s TargetScope: %v", key, err)
		}
	}
	confirmed, err := r.Workspaces.Targets(ctx, owner, profile.ID().String())
	if err != nil || len(confirmed) != 2 {
		t.Fatalf("real ACP Tool TargetScope proof: %v %d", err, len(confirmed))
	}

	agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Real restricted ACP acceptance", Backend: backend, WorkspaceRoot: root, Isolation: "restricted_host",
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Agents.Grant(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	// This is a real Agent as Tool acceptance: invoke the canonical AgentID-owned
	// agent.delegate Tool, rather than bypassing the Catalog/Invocation path
	// with a direct Agent Task service submission.
	agentIDs := agentOwnedToolIDs(t, r, ctx, agent.ID(), owner)
	delegateID := agentIDs["agent.delegate"]
	if delegateID == "" {
		t.Fatal("Agent-owned delegate Tool missing")
	}
	profile, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: profile.Slug(), ToolID: delegateID})
	if err != nil {
		t.Fatalf("explicit Agent delegate Profile binding: %v", err)
	}

	if _, err := r.ConfigureAgentToolPlacement(ctx, agent.ID(), "", sandbox.ID().String(),
		[]string{ids["workspace.read"].String(), ids["workspace.edit"].String()}, owner); err != nil {
		t.Fatal(err)
	}
	prompt := "Use only Mender MCP tools mender_read and mender_edit. Read review.txt. " +
		"Then create controlled-edit.txt containing exactly 'phase46 governed real edit' using mender_edit. " +
		"Do not use native shell, native file edits, plugins, or any other file or network operation. " +
		"State briefly whether the two Mender tool calls succeeded."
	if backend == agentdomain.BackendOpenCode {
		// OpenCode v2 exposes ACP-provided MCP tools through its Code Mode.
		prompt = "Use the execute Code Mode tool to call only the mender_task_tools namespace. " +
			"Invoke mender_read for review.txt, then mender_edit to create controlled-edit.txt " +
			"containing exactly phase46 governed real edit. Do not use fetch or other tools. " +
			"Confirm only after both calls succeed."
	}
	arguments, err := json.Marshal(struct {
		Prompt string `json:"prompt"`
	}{Prompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	delegated, err := r.CallConfiguredTool(ctx, delegateID, arguments, taskServicePrincipal(t, profile.ID().String()))
	if err != nil {
		t.Fatalf("real canonical agent.delegate Invocation failed: %v", err)
	}
	if delegated.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("real Agent as Tool admission outcome %s", delegated.Outcome.Kind())
	}
	var delegationReceipt struct {
		TaskID string `json:"taskId"`
		Status string `json:"status"`
	}
	if err = json.Unmarshal(delegated.Outcome.Content().Bytes(), &delegationReceipt); err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.ParseTaskID(delegationReceipt.TaskID)
	if err != nil {
		t.Fatalf("real canonical Agent Tool failed to produce a TaskID: %v", err)
	}
	// Explicit short operator acceptance bound, not the 10-minute product deadline.
	var finished agentdomain.Task
	limit := time.Now().Add(50 * time.Second)
	for time.Now().Before(limit) {
		finished, err = r.Agents.Task(ctx, taskID, owner)
		if err != nil {
			t.Fatal(err)
		}
		if finished.Status.Terminal() {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !finished.Status.Terminal() {
		_ = r.Agents.Cancel(ctx, taskID, owner)
		t.Fatal("real Agent acceptance exceeded 50-second smoke bound; cancel requested")
	}
	stage := "no_runner_receipt"
	select {
	case stage = <-receipt:
	default:
	}
	audit, err := r.ListAudit.Execute(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[catalogdomain.ToolID]int{}
	for _, ev := range audit {
		if ev.Status() == auditdomain.StatusCompleted && ev.OutcomeKind() == invocationdomain.OutcomeKindComplete {
			calls[ev.ToolID()]++
		}
	}
	t.Logf("real_backend=%s, restricted=true, task_state=%s, stage=%s, successful_delegate=%d, successful_read=%d, successful_edit=%d",
		backend, finished.Status, stage, calls[delegateID], calls[ids["workspace.read"]], calls[ids["workspace.edit"]])
	if finished.Status != agentdomain.TaskCompleted {
		t.Fatalf("real restricted Agent did not finish (diagnostic category=%q)", finished.Diagnostic)
	}
	if calls[delegateID] != 1 {
		t.Fatalf("canonical agent.delegate Invocation missing from completed audit (count=%d)", calls[delegateID])
	}
	if calls[ids["workspace.read"]] == 0 || calls[ids["workspace.edit"]] == 0 {
		t.Fatal("Agent did not invoke both governed Mender tools (Agent text is not proof)")
	}
	content, err := os.ReadFile(filepath.Join(root, "controlled-edit.txt"))
	if err != nil || string(content) != "phase46 governed real edit" {
		t.Fatalf("canonical edited file does not match disposable expected content: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "native-bypass.txt")); !os.IsNotExist(err) {
		t.Fatal("unexpected native write in protected root")
	}
	if err := r.DisableAgentToolPlacement(ctx, agent.ID(), "", owner); err != nil {
		t.Fatal(err)
	}
	if placements, err := r.ListAgentToolPlacements(ctx, agent.ID(), owner); err != nil || len(placements) != 0 {
		t.Fatal("owner disable failed after real Agent Task")
	}
}
