package bootstrap

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Regression from first real Rocky smoke: root SSH Jobs over a distant
// transport created a persistent manifest, but 30s generic Provider deadline
// cancelled before the private script/systemd unit could be created.
func TestSSHJobsImplicitTimeoutIsExactPinnedAndDoesNotLeak(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "jobs-timeout.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	caller, err := governancedomain.NewContext(governancedomain.OriginInternal, runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	template, secret, _ := startInProcessSSHForSandbox(t)
	runtime.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	template.User = "root" // local test-owned SSH fixture; no real root host or systemd
	remote, err := runtime.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "Verified root SSH Jobs", Template: template}, caller)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := runtime.EnsureSandboxJobTools(ctx, remote.ID(), caller)
	if err != nil || jobs.Status != "ready" || len(jobs.Tools) != 5 {
		t.Fatalf("reviewed Jobs suite %+v %v", jobs, err)
	}
	for _, j := range jobs.Tools {
		id, err := catalogdomain.ParseToolID(j.ToolID)
		if err != nil {
			t.Fatal(err)
		}
		if !runtime.isSSHJobsTool(ctx, id) || runtime.isSSHWorkspaceTool(ctx, id) {
			t.Fatalf("Jobs tool not classified with exact backend: %s", j.SourceKey)
		}
		request := invocationapplication.InvokeToolRequest{ToolID: id, Context: caller}
		if got := runtime.withSSHWorkspaceExecutionBudget(ctx, request).Timeout; got != sshJobsInvocationTimeout {
			t.Fatalf("%s implicit timeout %v, expected %v", j.SourceKey, got, sshJobsInvocationTimeout)
		}
		request.Timeout = 5 * time.Second
		if got := runtime.withSSHWorkspaceExecutionBudget(ctx, request).Timeout; got != request.Timeout {
			t.Fatalf("%s overwrote explicit shorter Tool deadline", j.SourceKey)
		}
	}
	unrelated := catalogdomain.ToolID("tool_00000000000000000000000000000009")
	if runtime.isSSHJobsTool(ctx, unrelated) || runtime.withSSHWorkspaceExecutionBudget(ctx, invocationapplication.InvokeToolRequest{ToolID: unrelated}).Timeout != 0 {
		t.Fatal("unknown or arbitrary Tool gained extended SSH Jobs timeout")
	}
}

func TestSSHWorkspaceBudgetAndTypedToolFailureAreScoped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	template, secret, _ := startInProcessSSHForSandbox(t)
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "ssh-workspace-budget.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	r.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	caller, err := governancedomain.NewContext(
		governancedomain.OriginInternal, r.OwnerPrincipal(), governancedomain.CatalogScope(),
	)
	if err != nil {
		t.Fatal(err)
	}

	remote, err := r.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{
		Name: "SSH Tool diagnostic fixture", Template: template,
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	remoteSuite, err := r.EnsureSandboxWorkspaceTools(ctx, remote.ID(), caller)
	if err != nil || remoteSuite.Status != "ready" || len(remoteSuite.Tools) != 4 {
		t.Fatalf("SSH Tool Suite state %+v / %v", remoteSuite, err)
	}
	var readID catalogdomain.ToolID
	for _, tool := range remoteSuite.Tools {
		if tool.SourceKey == "workspace.read" {
			readID, err = catalogdomain.ParseToolID(tool.ToolID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if readID == "" || !r.isSSHWorkspaceTool(ctx, readID) {
		t.Fatal("SSH Workspace Tool binding not recognized")
	}
	base := invocationapplication.InvokeToolRequest{ToolID: readID, Context: caller}
	if got := r.withSSHWorkspaceExecutionBudget(ctx, base).Timeout; got != sshWorkspaceInvocationTimeout {
		t.Fatalf("SSH implicit Tool budget %v, want %v", got, sshWorkspaceInvocationTimeout)
	}
	explicit := base
	explicit.Timeout = 8 * time.Second
	if got := r.withSSHWorkspaceExecutionBudget(ctx, explicit).Timeout; got != explicit.Timeout {
		t.Fatal("SSH Tool default overrode tighter explicit timeout")
	}
	// Compare a local Workspace suite to prove the timeout does not extend to
	// unrelated local or Docker Tool Providers.
	local, err := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "local fixture", Root: t.TempDir(), Isolation: sandboxdomain.IsolationHost,
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	localSuite, err := r.EnsureSandboxWorkspaceTools(ctx, local.ID(), caller)
	if err != nil || localSuite.Status != "ready" {
		t.Fatalf("local Tool Suite state %+v / %v", localSuite, err)
	}
	localID, err := catalogdomain.ParseToolID(localSuite.Tools[0].ToolID)
	if err != nil {
		t.Fatal(err)
	}
	if r.isSSHWorkspaceTool(ctx, localID) ||
		r.withSSHWorkspaceExecutionBudget(ctx, invocationapplication.InvokeToolRequest{ToolID: localID, Context: caller}).Timeout != 0 {
		t.Fatal("local Workspace was assigned SSH extended timeout")
	}
	// Cause an SSH failure before a network connection is attempted, then
	// verify the Tool-level result and audit expose only a fixed diagnostic.
	r.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: []byte("synthetic-invalid-test-key")}
	resp, err := r.CallConfiguredTool(ctx, readID, []byte(`{"path":"greeting.txt"}`), caller)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		resp.Outcome.FailureCode() != invocationdomain.FailureProviderError ||
		resp.Outcome.Message() != "ssh_remote_failure:credential_invalid" {
		t.Fatalf("SSH Tool outcome kind=%s code=%s message=%q", resp.Outcome.Kind(), resp.Outcome.FailureCode(), resp.Outcome.Message())
	}
	records, err := r.ListAudit.Execute(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		if record.ToolID().String() == readID.String() {
			found = true
			if record.ErrorSummary() != "ssh_remote_failure:credential_invalid" ||
				strings.Contains(record.ErrorSummary(), "synthetic-invalid-test-key") {
				t.Fatalf("unsafe SSH Tool audit diagnostic %q", record.ErrorSummary())
			}
		}
	}
	if !found {
		t.Fatal("SSH Tool failure did not leave a corresponding audit record")
	}
}
