package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshjobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
)

func TestSSHJobsManagedSuiteOnlyAfterExplicitOwnerPrepare(t *testing.T) {
	ctx := context.Background()
	database := filepath.Join(t.TempDir(), "jobs-suite.sqlite")
	tpl, secret, _ := startInProcessSSHForSandbox(t)
	tpl.User = "root" // Local fixture only; no actual root login or systemd service.
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database})
	if err != nil {
		t.Fatal(err)
	}
	r.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, err := gov.NewContext(gov.OriginInternal, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	item, err := r.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "Suite SSH", Template: tpl}, owner)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := r.Catalog.ListProviders.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := r.SandboxJobTools(ctx, item.ID(), owner)
	if err != nil || before.Status != "missing" || !before.Available || len(before.Tools) != 0 {
		t.Fatalf("not inert %+v %v", before, err)
	}
	afterGet, err := r.Catalog.ListProviders.Execute(ctx)
	if err != nil || len(previous) != len(afterGet) {
		t.Fatal("GET created Provider")
	}
	raw, _ := json.Marshal(sshjobsplugin.Config{SandboxID: item.ID().String()})
	_, err = r.RegisterConfiguredProvider(ctx, catalogapp.RegisterProviderRequest{Namespace: "forge-job-provider", DisplayName: "Forged Jobs"}, sshjobsplugin.ID, raw, owner)
	if !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("generic creation bypassed host suite: %v", err)
	}
	first, err := r.EnsureSandboxJobTools(ctx, item.ID(), owner)
	if err != nil || first.Status != "ready" || len(first.Tools) != 5 || first.ProviderID == "" {
		t.Fatalf("prepare %+v %v", first, err)
	}
	second, err := r.EnsureSandboxJobTools(ctx, item.ID(), owner)
	if err != nil || second.ProviderID != first.ProviderID {
		t.Fatalf("second prepare changed binding %+v %v", second, err)
	}
	for i, t1 := range first.Tools {
		if t1.ToolID == "" || t1.ToolID != second.Tools[i].ToolID {
			t.Fatal("unstable ToolID")
		}
	}
	providerID, err := catalogdomain.ParseProviderID(first.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.DeleteConfiguredProvider(ctx, providerID); !errors.Is(err, ErrProviderInUse) {
		t.Fatalf("generic deletion escaped job suite ownership: %v", err)
	}
	grantOwner, err := gov.NewContext(gov.OriginCLI, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	grants, err := r.Sandboxes.ListProfileGrants(ctx, item.ID(), grantOwner)
	if err != nil || len(grants) != 0 {
		t.Fatalf("suite implicit grant %+v %v", grants, err)
	}
	profiles, err := r.Profiles.List.Execute(ctx)
	if err != nil || len(profiles) != 0 {
		t.Fatal("suite implicitly exposed Profile")
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: database})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.Close() }()
	readOwner, err := gov.NewContext(gov.OriginInternal, next.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	restored, err := next.SandboxJobTools(ctx, item.ID(), readOwner)
	if err != nil || restored.ProviderID != first.ProviderID || restored.Status != "ready" {
		t.Fatalf("restart lost owned facade %+v %v", restored, err)
	}
	for i, t1 := range first.Tools {
		if t1.ToolID != restored.Tools[i].ToolID {
			t.Fatal("ToolID changed after restart")
		}
	}
}
