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
