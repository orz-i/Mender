package binding

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	wsapp "github.com/orz-i/Mender/backend/internal/workspace/application"
)

func TestWailsWorkspaceRegistryAndExactProfileExecutionTargets(t *testing.T) {
	ctx := context.Background()
	runtime, e := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "wails-v3.sqlite")})
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = runtime.Close() }()
	service := NewService(ctx, runtime)
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	sandbox, e := service.CreateLocalSandbox(CreateLocalSandboxInput{Name: "Wails Project", Root: root, Isolation: "host"})
	if e != nil {
		t.Fatal(e)
	}
	registered, e := service.CreateWorkspace("Wails Project", root)
	if e != nil {
		t.Fatal(e)
	}
	listed, e := service.ListWorkspaces()
	if e != nil || len(listed.Workspaces) != 1 {
		t.Fatalf("Wails workspace source: %+v %v", listed, e)
	}
	if e = service.LinkWorkspaceSandbox(registered.WorkspaceID, sandbox.SandboxID); e != nil {
		t.Fatal(e)
	}
	places, e := service.ListWorkspacePlacements(registered.WorkspaceID)
	if e != nil || len(places.Placements) != 1 {
		t.Fatalf("verified placement missing: %+v %v", places, e)
	}
	profile, e := runtime.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "desktop-workspace", DisplayName: "Wails Workspace"})
	if e != nil {
		t.Fatal(e)
	}
	if e = service.GrantWorkspaceProfile(registered.WorkspaceID, profile.ID().String()); e != nil {
		t.Fatal(e)
	}
	grants, e := service.ListWorkspaceProfileGrants(registered.WorkspaceID)
	if e != nil || len(grants.ProfileIDs) != 1 {
		t.Fatalf("Wails workspace grant: %+v %v", grants, e)
	}
	if e = service.RevokeWorkspaceProfile(registered.WorkspaceID, profile.ID().String()); e != nil {
		t.Fatal(e)
	}
	if e = service.DeleteWorkspace(registered.WorkspaceID); !errors.Is(e, wsapp.ErrRegistryConflict) {
		t.Fatalf("Wails deleted a still-attached Workspace: %v", e)
	}
	if e = service.UnlinkWorkspaceSandbox(registered.WorkspaceID, sandbox.SandboxID); e != nil {
		t.Fatal(e)
	}
	if e = service.DeleteWorkspace(registered.WorkspaceID); e != nil {
		t.Fatal(e)
	}
}
