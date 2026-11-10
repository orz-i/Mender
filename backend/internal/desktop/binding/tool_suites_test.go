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
	if err != nil || len(definitions.Suites) != 4 {
		t.Fatalf("suite descriptors: %+v %v", definitions, err)
	}
	browser := definitions.Suites[3]
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
	jobState, err := service.GetSandboxJobTools(item.ID().String())
	if err != nil || jobState.SuiteID != "sandbox.jobs" || jobState.Available {
		t.Fatalf("local Sandbox job capability projection: %+v %v", jobState, err)
	}
	ws, err := service.GetSandboxTools(item.ID().String())
	if err != nil || ws.Status != "ready" || !ws.Available || len(ws.Tools) != 4 {
		t.Fatalf("native workspace capability projection: %+v %v", ws, err)
	}
}
