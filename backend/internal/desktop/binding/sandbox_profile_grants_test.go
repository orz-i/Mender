package binding

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestDesktopProfileGrantContract(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "desktop-grant.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	svc := NewService(ctx, runtime)
	sandbox, err := svc.CreateLocalSandbox(CreateLocalSandboxInput{Name: "Grant sample", Root: t.TempDir(), Isolation: "host"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := runtime.Profiles.Create.Execute(ctx, profileapplication.CreateProfileRequest{Slug: "desktop-grant", DisplayName: "Desktop grant", Mode: profiledomain.ExposureHybrid})
	if err != nil {
		t.Fatal(err)
	}
	initially, err := svc.GetSandboxProfileGrants(sandbox.SandboxID)
	if err != nil || len(initially.Grants) != 0 {
		t.Fatalf("implicit grant %+v %v", initially, err)
	}
	created, err := svc.PutSandboxProfileGrants(sandbox.SandboxID, profile.ID().String(), []string{"file.read", "file.search"})
	if err != nil || len(created.Grants) != 1 {
		t.Fatalf("put %+v %v", created, err)
	}
	if _, err = svc.PutSandboxProfileGrants(sandbox.SandboxID, profile.ID().String(), []string{"INVALID"}); err == nil {
		t.Fatal("accepted invalid capability")
	}
	events, err := svc.GetSandboxProfileGrantEvents(sandbox.SandboxID, 100)
	if err != nil || len(events.Events) != 2 {
		t.Fatalf("events %+v %v", events, err)
	}
	revoked, err := svc.RevokeSandboxProfileGrants(sandbox.SandboxID, profile.ID().String())
	if err != nil || len(revoked.Grants) != 0 {
		t.Fatalf("revoke %+v %v", revoked, err)
	}
}
