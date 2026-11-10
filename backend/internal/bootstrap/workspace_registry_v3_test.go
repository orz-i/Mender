package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
)

func TestV3WorkspaceRegistryFullAdmissionAndRevocationDoesNotResurrect(t *testing.T) {
	ctx := context.Background()
	r, db, _ := freshOfficialV3(t)
	owner, e := gov.NewContext(gov.OriginCLI, r.OwnerPrincipal(), gov.CatalogScope())
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "README.md"), []byte("registry end-to-end\n"), 0600); e != nil {
		t.Fatal(e)
	}
	sb, e := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{Name: "Scope test", Root: root, Isolation: sandbox.IsolationHost}, owner)
	if e != nil {
		t.Fatal(e)
	}
	w, e := r.Workspaces.RegisterLocal(ctx, owner, "Test Workspace", root)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Workspaces.LinkLocal(ctx, owner, w.ID, sb.ID().String()); e != nil {
		t.Fatal(e)
	}
	profile, e := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "registry-v3", DisplayName: "Workspace Test"})
	if e != nil {
		t.Fatal(e)
	}
	tool := nativeID(t, "workspace.read")
	if _, e = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: profile.Slug(), ToolID: tool}); e != nil {
		t.Fatal(e)
	}
	target := workspaceapp.TargetRecord{ProfileID: profile.ID().String(), ToolID: tool, WorkspaceID: w.ID.String(), SandboxID: sb.ID().String()}
	if e = r.Workspaces.PutTarget(ctx, owner, target); !errors.Is(e, workspaceapp.ErrRegistryConflict) {
		t.Fatalf("target without grants admitted: %v", e)
	}
	if e = r.Sandboxes.SetProfileGrants(ctx, sb.ID(), profile.ID().String(), []sandbox.Capability{sandbox.CapabilityRead}, owner); e != nil {
		t.Fatal(e)
	}
	if e = r.Workspaces.Grant(ctx, owner, w.ID, profile.ID().String()); e != nil {
		t.Fatal(e)
	}
	if e = r.Workspaces.PutTarget(ctx, owner, target); e != nil {
		t.Fatal(e)
	}
	scoped, e := r.Workspaces.Targets(ctx, owner, profile.ID().String())
	if e != nil || len(scoped) != 1 || scoped[0].ToolID != tool {
		t.Fatalf("target not persisted: %+v %v", scoped, e)
	}
	if e = r.Workspaces.Revoke(ctx, owner, w.ID, profile.ID().String()); e != nil {
		t.Fatal(e)
	}
	if e = r.Workspaces.Grant(ctx, owner, w.ID, profile.ID().String()); e != nil {
		t.Fatal(e)
	}
	scoped, e = r.Workspaces.Targets(ctx, owner, profile.ID().String())
	if e != nil || len(scoped) != 0 {
		t.Fatalf("revoked target revived: %+v %v", scoped, e)
	}
	if e = r.Workspaces.PutTarget(ctx, owner, target); e != nil {
		t.Fatal(e)
	}
	if e = r.Sandboxes.SetProfileGrants(ctx, sb.ID(), profile.ID().String(), nil, owner); e != nil {
		t.Fatal(e)
	}
	if e = r.Sandboxes.SetProfileGrants(ctx, sb.ID(), profile.ID().String(), []sandbox.Capability{sandbox.CapabilityRead}, owner); e != nil {
		t.Fatal(e)
	}
	scoped, e = r.Workspaces.Targets(ctx, owner, profile.ID().String())
	if e != nil || len(scoped) != 0 {
		t.Fatalf("Sandbox revocation resurrected target: %+v %v", scoped, e)
	}
	// A placement detach also permanently fences targets from future reattach.
	if e = r.Workspaces.PutTarget(ctx, owner, target); e != nil {
		t.Fatal(e)
	}
	if e = r.Workspaces.Unlink(ctx, owner, w.ID, sb.ID().String()); e != nil {
		t.Fatal(e)
	}
	if e = r.Workspaces.LinkLocal(ctx, owner, w.ID, sb.ID().String()); e != nil {
		t.Fatal(e)
	}
	scoped, e = r.Workspaces.Targets(ctx, owner, profile.ID().String())
	if e != nil || len(scoped) != 0 {
		t.Fatalf("detached placement resurrected target: %+v %v", scoped, e)
	}
	if e = r.Workspaces.Unlink(ctx, owner, w.ID, sb.ID().String()); e != nil {
		t.Fatal(e)
	}
	if e = r.Workspaces.Delete(ctx, owner, w.ID); !errors.Is(e, workspaceapp.ErrRegistryConflict) {
		t.Fatalf("Workspace with Profile grant retired: %v", e)
	}
	if e = r.Workspaces.Revoke(ctx, owner, w.ID, profile.ID().String()); e != nil {
		t.Fatal(e)
	}
	if e = r.Workspaces.Delete(ctx, owner, w.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Workspaces.Get(ctx, owner, w.ID); !errors.Is(e, workspaceapp.ErrRegistryMissing) {
		t.Fatalf("deleted workspace still visible: %v", e)
	}
	var left int
	if e = db.QueryRowContext(ctx, `SELECT count(*) FROM profile_tool_target_scopes`).Scan(&left); e != nil || left != 0 {
		t.Fatalf("stale target rows %d %v", left, e)
	}
}

func TestV3WorkspaceRegistryCannotConfuseOwnerOrRemoteRoot(t *testing.T) {
	ctx := context.Background()
	r, _, _ := freshOfficialV3(t)
	owner, e := gov.NewContext(gov.OriginCLI, r.OwnerPrincipal(), gov.CatalogScope())
	if e != nil {
		t.Fatal(e)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	sb, e := r.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{Name: "Other root", Root: root, Isolation: sandbox.IsolationHost}, owner)
	if e != nil {
		t.Fatal(e)
	}
	outside, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	w, e := r.Workspaces.RegisterLocal(ctx, owner, "Elsewhere", outside)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Workspaces.LinkLocal(ctx, owner, w.ID, sb.ID().String()); !errors.Is(e, workspaceapp.ErrRegistryConflict) {
		t.Fatalf("wrong sandbox root linked: %v", e)
	}
	if _, e = r.Workspaces.RegisterLocal(ctx, owner, "Duplicate", outside); !errors.Is(e, workspaceapp.ErrRegistryConflict) {
		t.Fatalf("duplicate root registered: %v", e)
	}
}

// Force compile-time canonical ToolID identity and prevent accidental selector
// substitution from raw caller input.
var _ catalog.ToolID = "tool_11111111111111111111111111111111"
