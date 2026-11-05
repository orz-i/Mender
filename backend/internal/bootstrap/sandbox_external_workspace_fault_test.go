package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

// This builds the checked-in v1 example with a temporary Go overlay. Only this
// acceptance fixture gains fault paths; SDK/proto/example sources and declared
// capabilities remain unchanged. Calls still use the real managed process,
// authenticated gRPC dialer, Sandbox lease and canonical Invocation pipeline.
func TestExternalWorkspaceV1ProcessFaultsPreserveAuditAndLease(t *testing.T) {
	pkg := exampleSandboxPluginPackage(t, externalWorkspaceFaultOverlay(t))
	for _, test := range []struct {
		name          string
		path          string
		failureSource invocationdomain.FailureSource
		failureCode   invocationdomain.FailureCode
		auditClass    auditdomain.ErrorClass
		retainedLease bool
		processExited bool
	}{
		{"process exit", "fixture:exit", invocationdomain.FailureSourceProvider, invocationdomain.FailureProviderError, auditdomain.ErrorExecution, true, true},
		{"malformed JSON", "fixture:json", invocationdomain.FailureSourceProvider, invocationdomain.FailureProviderError, auditdomain.ErrorExecution, false, false},
		{"schema mismatch", "fixture:schema", invocationdomain.FailureSourceResultGuard, invocationdomain.FailureOutputSchemaMismatch, auditdomain.ErrorResultGuard, false, false},
		{"result budget precedes schema mismatch", "fixture:large-schema", invocationdomain.FailureSourceResultGuard, invocationdomain.FailureResultTooLarge, auditdomain.ErrorResultGuard, false, false},
		{"valid padded JSON retains original size budget", "fixture:padding", invocationdomain.FailureSourceResultGuard, invocationdomain.FailureResultTooLarge, auditdomain.ErrorResultGuard, false, false},
		{"oversized response", "fixture:oversized", invocationdomain.FailureSourceProvider, invocationdomain.FailureProviderError, auditdomain.ErrorExecution, false, false},
		{"cleanup overrides schema mismatch", "fixture:schema-close-error", invocationdomain.FailureSourceProvider, invocationdomain.FailureProviderError, auditdomain.ErrorExecution, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, sandboxID, installationID, toolID := externalWorkspaceFaultRuntime(t, pkg)
			ctx := context.Background()
			arguments, err := json.Marshal(map[string]string{"path": test.path})
			if err != nil {
				t.Fatal(err)
			}
			response, err := r.CallConfiguredTool(ctx, toolID, arguments, ownerContext(t, r))
			if err != nil || response.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
				response.Outcome.FailureSource() != test.failureSource || response.Outcome.FailureCode() != test.failureCode {
				t.Fatalf("native fault classification = %+v, %v; want %s/%s", response.Outcome, err, test.failureSource, test.failureCode)
			}
			record, err := r.audit.Get(ctx, response.InvocationID)
			if err != nil || record.Status() != auditdomain.StatusFailed || record.ErrorClass() != test.auditClass ||
				record.OutcomeKind() != invocationdomain.OutcomeKindFailed || record.FailureSource() != test.failureSource || record.FailureCode() != test.failureCode {
				t.Fatalf("native fault audit = %+v, %v", record, err)
			}
			wantLeases := 0
			if test.retainedLease {
				wantLeases = 1
			}
			leases, err := r.Sandboxes.store.ActiveLeaseCount(ctx, sandboxID)
			if err != nil || leases != wantLeases {
				t.Fatalf("active leases = %d, %v; want %d", leases, err, wantLeases)
			}
			if running := r.externalProcesses.Running(installationID); running == test.processExited {
				t.Fatalf("native process running=%v; exited=%v", running, test.processExited)
			}
			// A clean Close releases the durable lease and permits a fresh read.
			// Failed cleanup fences the next acquisition before any RPC/restart.
			again, againErr := r.CallConfiguredTool(ctx, toolID, []byte(`{"path":"README.txt"}`), ownerContext(t, r))
			if test.retainedLease {
				if againErr == nil && again.Outcome.Kind() == invocationdomain.OutcomeKindComplete {
					t.Fatal("uncertain cleanup lease allowed another native operation")
				}
				leases, err = r.Sandboxes.store.ActiveLeaseCount(ctx, sandboxID)
				if err != nil || leases != 1 || (test.processExited && r.externalProcesses.Running(installationID)) {
					t.Fatalf("retained lease was released or process replayed: leases=%d err=%v", leases, err)
				}
			} else if againErr != nil || again.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
				t.Fatalf("clean fault session prevented a fresh read: %+v %v", again.Outcome, againErr)
			}
		})
	}
	t.Run("HTML encoding expansion retains original size budget", func(t *testing.T) {
		r, sandboxID, _, toolID := externalWorkspaceFaultRuntime(t, pkg)
		ctx := context.Background()
		response, err := r.CallConfiguredTool(ctx, toolID, []byte(`{"path":"fixture:html"}`), ownerContext(t, r))
		if err != nil || response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf("typed JSON expansion changed accepted v1 result: %+v %v", response.Outcome, err)
		}
		if len(response.Outcome.Content().Bytes()) <= 1<<20 {
			t.Fatal("fixture did not exercise typed JSON expansion beyond Runtime result budget")
		}
		var result struct{ Content string }
		if err := json.Unmarshal(response.Outcome.Content().Bytes(), &result); err != nil || result.Content != strings.Repeat("<", 256*1024) {
			t.Fatalf("HTML content changed during typed mapping: bytes=%d err=%v", len(result.Content), err)
		}
		record, err := r.audit.Get(ctx, response.InvocationID)
		if err != nil || record.Status() != auditdomain.StatusCompleted || record.ErrorClass() != auditdomain.ErrorNone || record.OutcomeKind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf("HTML accepted-result audit = %+v %v", record, err)
		}
		leases, err := r.Sandboxes.store.ActiveLeaseCount(ctx, sandboxID)
		if err != nil || leases != 0 {
			t.Fatalf("HTML result did not close its lease: %d %v", leases, err)
		}
	})
}

func externalWorkspaceFaultRuntime(t *testing.T, pkg string) (*Runtime, sandboxdomain.ID, platformplugin.InstallationID, catalogdomain.ToolID) {
	t.Helper()
	ctx := context.Background()
	folder := t.TempDir()
	managed := filepath.Join(folder, "managed")
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(folder, "mender.db"), ProviderPluginRoot: managed})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		makeManagedPluginTempTreeWritable(managed)
	})
	installed, err := r.InstallSandboxPlugin(ctx, pkg)
	if err != nil {
		t.Fatal(err)
	}
	sandbox, err := r.Sandboxes.CreateExternalSandbox(ctx, fixtureSandboxRequest(installed.ID(), `{}`), ownerContext(t, r))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(workspaceplugin.Config{SandboxID: sandbox.ID().String()})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapp.RegisterProviderRequest{
		Namespace: "external-fault-fixture", DisplayName: "External fault fixture",
	}, workspaceplugin.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfiguredProviderTest(t, r, ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	tools, err := r.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.SourceKey() == "workspace.read" {
			return r, sandbox.ID(), installed.ID(), tool.ID()
		}
	}
	t.Fatal("canonical workspace.read Tool was not discovered")
	return nil, "", "", ""
}

func externalWorkspaceFaultOverlay(t *testing.T) string {
	t.Helper()
	source := filepath.Join(bootstrapRepositoryRoot(t), "examples", "sandbox-plugin-example", "cmd", "mender-sandbox-example", "main.go")
	raw, err := os.ReadFile(source) // #nosec G304 -- fixed checked-in native acceptance fixture.
	if err != nil {
		t.Fatal(err)
	}
	readMarker := `if arg.Path != "README.txt" {`
	closeMarker := `delete(s.sessions, req.GetSessionId())`
	if strings.Count(string(raw), readMarker) != 1 || strings.Count(string(raw), closeMarker) != 1 {
		t.Fatal("native example changed; review the fault overlay anchors")
	}
	readFaults := `switch arg.Path {
	case "fixture:exit":
		os.Exit(17)
	case "fixture:json":
		return &sandboxv1.ExecuteWorkspaceResponse{ContentJson: []byte("not-json")}, nil
	case "fixture:schema":
		return &sandboxv1.ExecuteWorkspaceResponse{ContentJson: []byte("{}")}, nil
	case "fixture:large-schema":
		return &sandboxv1.ExecuteWorkspaceResponse{ContentJson: []byte("{\"content\":\"" + strings.Repeat("x", 1024*1024+1) + "\"}")}, nil
	case "fixture:padding":
		return &sandboxv1.ExecuteWorkspaceResponse{ContentJson: []byte("{" + strings.Repeat(" ", 1024*1024+1) + "\"path\":\"README.txt\",\"content\":\"fixture\",\"startLine\":1,\"endLine\":1,\"truncated\":false}")}, nil
	case "fixture:html":
		return &sandboxv1.ExecuteWorkspaceResponse{ContentJson: []byte("{\"path\":\"README.txt\",\"content\":\"" + strings.Repeat("<", 256*1024) + "\",\"startLine\":1,\"endLine\":1,\"truncated\":false}")}, nil
	case "fixture:oversized":
		return &sandboxv1.ExecuteWorkspaceResponse{ContentJson: []byte(strings.Repeat("x", 2*1024*1024+1))}, nil
	case "fixture:schema-close-error":
		s.mu.Lock()
		s.sessions[req.GetSessionId()] = "fixture:close-error"
		s.mu.Unlock()
		return &sandboxv1.ExecuteWorkspaceResponse{ContentJson: []byte("{}")}, nil
	}
	`
	closeFault := `if s.sessions[req.GetSessionId()] == "fixture:close-error" {
		s.mu.Unlock()
		return nil, fmt.Errorf("fixture session cleanup failed")
	}
	`
	patched := strings.Replace(string(raw), readMarker, readFaults+readMarker, 1)
	patched = strings.Replace(patched, closeMarker, closeFault+closeMarker, 1)
	folder := t.TempDir()
	replacement := filepath.Join(folder, "main.go")
	if err := os.WriteFile(replacement, []byte(patched), 0o600); err != nil { // #nosec G703 -- fixed basename under a test-owned TempDir.
		t.Fatal(err)
	}
	overlay, err := json.Marshal(struct{ Replace map[string]string }{Replace: map[string]string{source: replacement}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, "overlay.json")
	if err := os.WriteFile(path, overlay, 0o600); err != nil { // #nosec G703 -- fixed basename under a test-owned TempDir.
		t.Fatal(err)
	}
	return path
}
