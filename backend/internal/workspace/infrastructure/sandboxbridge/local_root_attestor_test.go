package sandboxbridge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

func TestLocalRootAttestorRequiresExactCanonicalRoot(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := workspace.ParseRef("wrk_11111111111111111111111111111111")
	item, err := sandbox.NewSandbox(sandbox.Spec{
		ID: "sbx_11111111111111111111111111111111", Name: "Local",
		Backend: sandbox.BackendLocalWorkspace, Binding: sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace),
		Isolation: sandbox.IsolationRestrictedHost, LocalRoot: root,
		OwnerKind: "local_user", OwnerRef: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	placement := sandboxapp.Placement{Sandbox: item, Required: sandbox.CapabilityRead}
	check := LocalRootAttestor{}
	candidate := workspaceapp.PlacementCandidate{Workspace: ref, SandboxID: item.ID().String(), TargetRoot: root}
	if yes, err := check.AttestRoot(context.Background(), candidate, placement); err != nil || !yes {
		t.Fatalf("actual root not recognized: %v %v", yes, err)
	}
	candidate.TargetRoot = t.TempDir()
	if yes, err := check.AttestRoot(context.Background(), candidate, placement); err != nil || yes {
		t.Fatalf("unrelated directory unexpectedly matched: %v %v", yes, err)
	}
	candidate.TargetRoot = root
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := check.AttestRoot(ctx, candidate, placement); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled root lookup: %v", err)
	}
}

func TestLocalRootAttestorDeniesSymlinkMissingOrNonLocalRoot(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := workspace.ParseRef("wrk_11111111111111111111111111111111")
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path    string
		backend sandbox.BackendKind
		binding sandbox.BackendBinding
		isol    sandbox.IsolationClass
	}{
		{link, sandbox.BackendLocalWorkspace, sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace), sandbox.IsolationHost},
		{filepath.Join(root, "missing"), sandbox.BackendLocalWorkspace, sandbox.BuiltinBinding(sandbox.BackendLocalWorkspace), sandbox.IsolationHost},
		{"/workspace", sandbox.BackendSSHRemote, sandbox.BuiltinBinding(sandbox.BackendSSHRemote), sandbox.IsolationRemote},
	} {
		spec := sandbox.Spec{
			ID: "sbx_11111111111111111111111111111111", Name: "Test",
			Backend: tc.backend, Binding: tc.binding, Isolation: tc.isol,
			OwnerKind: "local_user", OwnerRef: "test",
		}
		if tc.backend == sandbox.BackendLocalWorkspace {
			spec.LocalRoot = tc.path
		}
		item, err := sandbox.NewSandbox(spec)
		if err != nil {
			t.Fatal(err)
		}
		candidate := workspaceapp.PlacementCandidate{Workspace: ref, SandboxID: item.ID().String(), TargetRoot: tc.path}
		if yes, err := (LocalRootAttestor{}).AttestRoot(context.Background(), candidate, sandboxapp.Placement{Sandbox: item}); err != nil || yes {
			t.Fatalf("accepted unverified root %+v: %v %v", tc, yes, err)
		}
	}
}
