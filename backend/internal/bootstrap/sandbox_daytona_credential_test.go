package bootstrap

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdaytona "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
)

func TestDaytonaSandboxRetainsCredentialUsageUntilDestroyed(t *testing.T) {
	t.Setenv(DaytonaExperimentalEnvironment, "")
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "daytona-credentials.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	if runtime.Sandboxes.DaytonaConfigured() {
		t.Fatal("Daytona remote adapter enabled without operator opt-in")
	}
	id := sandboxdomain.ID("sbx_abcdef0123456789abcdef0123456789")
	owner := runtime.OwnerPrincipal()
	resource, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: "Daytona pending", Backend: sandboxdomain.BackendDaytona, Isolation: sandboxdomain.IsolationRemote, OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference()})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := credentialdomain.ParseReference("credential://daytona/api-key")
	if err != nil {
		t.Fatal(err)
	}
	template := sandboxdaytona.Template{CredentialRef: ref, ImageDigest: "docker.io/library/python@sha256:" + strings.Repeat("a", 64), Target: sandboxdaytona.TargetUS, CPU: 1, MemoryGiB: 2, DiskGiB: 3, TTLMinutes: 30}
	if err := runtime.Sandboxes.daytonaTemplates.CreateDefinedSandbox(ctx, resource, template); err != nil {
		t.Fatal(err)
	}
	checked := runtimeCredentialUsageChecker{profiles: runtime.Profiles, tunnels: runtime.Tunnels, configs: runtime.providerConfigs, sandboxes: runtime.Sandboxes}
	usages, err := checked.Usages(ctx, ref)
	if err != nil || len(usages) != 1 || usages[0].Kind != "sandbox" || !strings.Contains(usages[0].Description, id.String()) {
		t.Fatalf("live Daytona credential usage missing: %+v %v", usages, err)
	}
	unrelated, err := credentialdomain.ParseReference("credential://daytona/other")
	if err != nil {
		t.Fatal(err)
	}
	other, err := checked.Usages(ctx, unrelated)
	if err != nil || len(other) != 0 {
		t.Fatalf("unrelated key retained %+v %v", other, err)
	}
}
