package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	jobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
)

func TestSSHJobsFacadeRecoversAfterInterruptedCreationWithoutNewIdentity(t *testing.T) {
	ctx := context.Background()
	template, secret, _ := startInProcessSSHForSandbox(t)
	template.User = "root"
	filename := filepath.Join(t.TempDir(), "partial-jobs-facade.db")
	first, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filename})
	if err != nil {
		t.Fatal(err)
	}
	first.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	owner, err := gov.NewContext(gov.OriginInternal, first.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	resource, err := first.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{Name: "Interrupted Job Suite", Template: template}, owner)
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(jobsplugin.Config{SandboxID: resource.ID().String()})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a process dying between Provider persist and Tool synchronization.
	provider, err := first.registerConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{
		Namespace: jobFacadeNamespace(resource.ID()), DisplayName: "Interrupted Job Suite",
	}, jobsplugin.ID, config, owner, true)
	if err != nil {
		t.Fatal(err)
	}
	incomplete, err := first.SandboxJobTools(ctx, resource.ID(), owner)
	if err != nil || incomplete.Status != "incomplete" || incomplete.ProviderID != provider.ID().String() || len(incomplete.Tools) != 0 {
		t.Fatalf("unclaimed provider not recoverable: %+v %v", incomplete, err)
	}
	if err = first.RemoveSandboxJobTools(ctx, resource.ID(), owner); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("partial facade allowed owner teardown instead of repair: %v", err)
	}
	before, err := first.Catalog.ListProviders.Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filename})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	reopenedOwner, _ := gov.NewContext(gov.OriginInternal, second.OwnerPrincipal(), gov.CatalogScope())
	repaired, err := second.EnsureSandboxJobTools(ctx, resource.ID(), reopenedOwner)
	if err != nil || repaired.Status != "ready" || repaired.ProviderID != provider.ID().String() || len(repaired.Tools) != 5 {
		t.Fatalf("resume did not complete exact facade: %+v %v", repaired, err)
	}
	after, err := second.Catalog.ListProviders.Execute(ctx)
	if err != nil || len(after) != len(before) {
		t.Fatalf("repair created duplicate Provider: %d -> %d, err=%v", len(before), len(after), err)
	}
	check, err := second.EnsureSandboxJobTools(ctx, resource.ID(), reopenedOwner)
	if err != nil || check.ProviderID != repaired.ProviderID {
		t.Fatalf("repair not idempotent %+v %v", check, err)
	}
	for i, tool := range repaired.Tools {
		if check.Tools[i].ToolID != tool.ToolID {
			t.Fatalf("ToolID changed across ensure: %+v vs %+v", repaired.Tools, check.Tools)
		}
	}
	grantOwner, err := gov.NewContext(gov.OriginCLI, second.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	grant, err := second.Sandboxes.ListProfileGrants(ctx, resource.ID(), grantOwner)
	if err != nil || len(grant) != 0 {
		t.Fatalf("repair silently authorized Profiles: %+v %v", grant, err)
	}
}
