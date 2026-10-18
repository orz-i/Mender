package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Durable Jobs are intentionally not treated as Workspace tools: stopping
// the remote Sandbox disables new submissions but does not invalidate the
// read-only recovery surface for durable task status, logs and results.
func TestResolveMCPToolSourcesRetainsStoppedSSHJobsRecovery(t *testing.T) {
	ctx := context.Background()
	template, secret, _ := startInProcessSSHForSandbox(t)
	template.User = "root" // Test SSH fixture only, no external/root host.

	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "jobs-mcp-source.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	r.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, err := governancedomain.NewContext(
		governancedomain.OriginInternal, r.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := r.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{
		Name: "Recovery Sandbox", Template: template,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := r.EnsureSandboxJobTools(ctx, item.ID(), owner)
	if err != nil || suite.Status != "ready" {
		t.Fatalf("prepare SSH Jobs: %+v / %v", suite, err)
	}
	id, err := catalogdomain.ParseProviderID(suite.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := r.resolveMCPToolSources(ctx, []catalogdomain.ProviderID{id})
	if err != nil || before[id].Unavailable || before[id].Kind != "sandbox" {
		t.Fatalf("ready Jobs source: %+v / %v", before, err)
	}
	stopped, err := r.Sandboxes.ChangeLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStop, owner)
	if err != nil || stopped.State != sandboxdomain.StateStopped {
		t.Fatalf("stop SSH fixture: %+v / %v", stopped, err)
	}
	after, err := r.resolveMCPToolSources(ctx, []catalogdomain.ProviderID{id})
	if err != nil || after[id].Unavailable || after[id].ResourceID != item.ID().String() {
		t.Fatalf("stopped Jobs recovery source was incorrectly hidden: %+v / %v", after, err)
	}
}
