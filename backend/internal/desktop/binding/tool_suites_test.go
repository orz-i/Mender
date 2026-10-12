package binding

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestSandboxToolSuitesDesktopBindingsMatchReviewedCapabilities(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "suite.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	service := NewService(ctx, runtime)
	definitions, err := service.ListToolSuites()
	if err != nil || len(definitions.Suites) != 3 {
		t.Fatalf("suite descriptors: %+v %v", definitions, err)
	}
	browser := definitions.Suites[2]
	if browser.Status != "unavailable" || len(browser.ToolSourceKeys) != 0 {
		t.Fatal("Browser falsely executable")
	}
	owner, err := gov.NewContext(gov.OriginDesktop, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := runtime.Sandboxes.CreateLocalSandbox(ctx, bootstrap.CreateLocalSandboxRequest{Name: "Desktop Suite", Root: t.TempDir(), Isolation: sandboxdomain.IsolationHost}, owner)
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.GetSandboxTools(item.ID().String())
	if err != nil || before.Status != "missing" {
		t.Fatalf("GET suite: %+v %v", before, err)
	}
	after, err := service.EnsureSandboxTools(item.ID().String())
	if err != nil || after.Status != "ready" || len(after.Tools) != 4 || after.ProviderID == "" {
		t.Fatalf("ensure suite: %+v %v", after, err)
	}
	if err = service.RemoveSandboxTools(item.ID().String()); err != nil {
		t.Fatal(err)
	}
}
