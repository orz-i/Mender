package binding

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestDesktopDaytonaCredentialGateBeforeRemoteSideEffects(t *testing.T) {
	t.Setenv(bootstrap.DaytonaExperimentalEnvironment, "enabled")
	t.Setenv("DAYTONA_JWT_TOKEN", "")
	t.Setenv("DAYTONA_ORGANIZATION_ID", "")
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "daytona-desktop.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	desktop := NewService(ctx, runtime)
	ready, err := desktop.GetSandboxBackends()
	if err != nil || !ready.DaytonaConfigured {
		t.Fatalf("Daytona SDK adapter missing %+v %v", ready, err)
	}
	// #nosec G101 -- a test-only Credential Reference, not plaintext key material.
	_, err = desktop.CreateDaytonaSandbox(CreateDaytonaSandboxInput{
		Name: "Denied Daytona", CredentialRef: "credential://daytona/api-key", ImageDigest: "docker.io/library/python@sha256:" + strings.Repeat("a", 64),
		Target: "us", CPU: 1, MemoryGiB: 2, DiskGiB: 3, TTLMinutes: 30,
	})
	if !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("missing Mender credential admitted remote provision: %v", err)
	}
	all, err := desktop.ListSandboxes()
	if err != nil || len(all.Sandboxes) != 0 {
		t.Fatalf("denied provision persisted %+v %v", all, err)
	}
}

// Desktop and Web must both hide the unverified remote adapter unless the
// owner explicitly permits an experimental deployment on that Runtime.
func TestDesktopDaytonaExperimentalDisabledByDefault(t *testing.T) {
	t.Setenv(bootstrap.DaytonaExperimentalEnvironment, "")
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "daytona-disabled-desktop.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	service := NewService(ctx, runtime)
	backends, err := service.GetSandboxBackends()
	if err != nil || backends.DaytonaConfigured {
		t.Fatalf("remote enabled without owner opt-in: %+v %v", backends, err)
	}
	// #nosec G101 -- this is a test-only opaque Credential Reference.
	_, err = service.CreateDaytonaSandbox(CreateDaytonaSandboxInput{Name: "Disabled",
		CredentialRef: "credential://daytona/api-key", ImageDigest: "docker.io/library/python@sha256:" + strings.Repeat("a", 64),
		Target: "us", CPU: 1, MemoryGiB: 2, DiskGiB: 3, TTLMinutes: 30})
	if !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("remote Desktop accepted create with no opt-in: %v", err)
	}
	inventory, err := service.ListSandboxes()
	if err != nil || len(inventory.Sandboxes) != 0 {
		t.Fatalf("quarantined Daytona persisted a remote create: %+v %v", inventory, err)
	}
}
