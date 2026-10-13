package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

// Real in-memory MCP client -> Profile ToolBinding -> Invocation Policy/Audit
// -> host-owned jobs facade -> exact pinned SSH backend -> local SFTP server.
// The SSH fixture does not have a systemd manager or require real root.
func TestMCPSSHJobsSeparateGrantsAndReadOnlyRecoveryWithUncertainLease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	template, secret, _ := startInProcessSSHForSandbox(t)
	template.User = "root"
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "jobs-mcp.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	r.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, err := gov.NewContext(gov.OriginDesktop, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := r.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "MCP Jobs", Template: template}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := r.EnsureSandboxJobTools(ctx, item.ID(), owner)
	if err != nil || suite.Status != "ready" || len(suite.Tools) != 5 {
		t.Fatalf("Jobs facade not ready %+v %v", suite, err)
	}

	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "mcp-jobs", DisplayName: "MCP Jobs", Mode: profiledomain.ExposureDirect})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range suite.Tools {
		profile, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
			ProfileSlug: profile.Slug(), ToolID: catalogdomain.ToolID(tool.ToolID), Direct: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	adapter, err := r.NewMCPServer(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	cTransport, sTransport := mcp.NewInMemoryTransports()
	serverSession, err := adapter.MCPServer().Connect(ctx, sTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "jobs-scope-check", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, cTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, tool := range list.Tools {
		for _, name := range []string{"submit", "status", "logs", "result", "cancel"} {
			if strings.HasSuffix(tool.Name, "__"+name) {
				names[name] = tool.Name
			}
		}
	}
	if len(names) != 5 {
		t.Fatalf("MCP jobs tools not exposed by explicit bindings %+v", list.Tools)
	}
	invoke := func(name string, args map[string]any) bool {
		t.Helper()
		result, e := session.CallTool(ctx, &mcp.CallToolParams{Name: names[name], Arguments: args})
		if e != nil {
			t.Fatalf("MCP transport %v", e)
		}
		if result.IsError {
			t.Logf("MCP %s error content=%+v structured=%+v", name, result.Content, result.StructuredContent)
			return false
		}
		if val, ok := result.StructuredContent.(map[string]any); ok {
			if val["kind"] == "failed" || val["kind"] == "rejected" {
				t.Logf("MCP %s outcome=%+v", name, val)
				return false
			}
		}
		return true
	}
	req := map[string]any{"idempotencyKey": strings.Repeat("e", 32), "command": "echo mcp-fake-server-no-systemd"}
	task := map[string]any{"taskId": "sjob_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if invoke("submit", req) || invoke("status", task) || invoke("cancel", task) {
		t.Fatal("Profile ToolBindings alone bypassed missing SSH Job grants")
	}
	// An owner-only fixture submit reserves a durable remote TaskID without
	// executing code (this local SSH fixture has no systemd exec subsystem).
	prior, priorErr := r.Sandboxes.SubmitSSHJob(ctx, item.ID(), owner, sshremote.JobRequest{IdempotencyKey: strings.Repeat("d", 32), Command: "echo test-owned pending record"})
	if !errors.Is(priorErr, sshremote.ErrJobUnknown) || prior.TaskID == "" {
		t.Fatalf("no task record for observe test %+v %v", prior, priorErr)
	}
	if err = r.Sandboxes.SetProfileGrants(ctx, item.ID(), profile.ID().String(), []sandboxdomain.Capability{
		sandboxdomain.CapabilityJobObserve}, owner); err != nil {
		t.Fatal(err)
	}
	if !invoke("status", task) || !invoke("result", task) || !invoke("logs", map[string]any{
		"taskId": prior.TaskID, "stream": "stdout", "cursor": 0, "limit": 1024}) {
		t.Fatal("explicit JobObserve grant cannot query remote TaskID")
	}
	if invoke("submit", req) || invoke("cancel", task) {
		t.Fatal("JobObserve elevated to submit/cancel")
	}
	if err = r.Sandboxes.SetProfileGrants(ctx, item.ID(), profile.ID().String(), []sandboxdomain.Capability{
		sandboxdomain.CapabilityJobObserve, sandboxdomain.CapabilityJobSubmit}, owner); err != nil {
		t.Fatal(err)
	}
	if !invoke("submit", req) {
		t.Fatal("JobSubmit grant rejected fake SSH unknown receipt")
	}
	if invoke("cancel", task) {
		t.Fatal("JobSubmit elevated to cancel")
	}
	// A stale uncertain Workspace execution lease does NOT gate observation,
	// which is needed precisely to reconcile the preceding uncertain submit.
	run, err := r.Sandboxes.GetRun(ctx, item.ID())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := r.Sandboxes.store.ReserveExecution(ctx, item.ID(), run.Generation, owner, sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	if !invoke("status", task) {
		t.Fatal("old short lease blocked read-only job observation")
	}
	if invoke("submit", map[string]any{"idempotencyKey": strings.Repeat("f", 32), "command": "echo refused"}) {
		t.Fatal("exclusive uncertain lease did not block new job mutation")
	}
	if err = r.Sandboxes.store.ReleaseExecution(ctx, item.ID(), lease); err != nil {
		t.Fatal(err)
	}
}
