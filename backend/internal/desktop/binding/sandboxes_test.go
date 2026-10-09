package binding

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestDesktopSandboxManagementFollowsRuntimeGovernance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "desktop-sandboxes.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	service := NewService(ctx, runtime)
	initially, err := service.ListSandboxes()
	if err != nil || len(initially.Sandboxes) != 0 {
		t.Fatalf("initial %+v %v", initially, err)
	}
	root := t.TempDir()
	created, err := service.CreateLocalSandbox(CreateLocalSandboxInput{Name: "Desktop", Root: root, Isolation: "host"})
	if err != nil {
		t.Fatal(err)
	}
	if created.SandboxID == "" || created.Backend != "local_workspace" || created.State != "ready" {
		t.Fatalf("created %+v", created)
	}
	fetched, err := service.GetSandbox(created.SandboxID)
	if err != nil || fetched.SandboxID != created.SandboxID {
		t.Fatalf("detail %+v %v", fetched, err)
	}
	list, err := service.ListSandboxes()
	if err != nil || len(list.Sandboxes) != 1 {
		t.Fatalf("list %+v %v", list, err)
	}
	if _, err := service.GetSandbox("sbx_invalid"); err == nil {
		t.Fatal("accepted malformed SandboxID")
	}
	if _, err := runtime.Policy.CreateRule.Execute(ctx, policyapplication.RuleInput{DisplayName: "Deny new Sandbox", Enabled: true, Decision: policydomain.DecisionDeny, Match: policydomain.MatchSpec{Actions: []governancedomain.Action{governancedomain.ActionSandboxProvision}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateLocalSandbox(CreateLocalSandboxInput{Name: "Denied", Root: filepath.Join(t.TempDir(), "nonexistent"), Isolation: "host"}); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("sandbox.provision policy bypass: %v", err)
	}
	unchanged, err := service.ListSandboxes()
	if err != nil || len(unchanged.Sandboxes) != 1 {
		t.Fatalf("denied create persisted: %+v %v", unchanged, err)
	}
	stopped, err := service.StopSandbox(created.SandboxID)
	if err != nil || stopped.State != "stopped" {
		t.Fatalf("desktop stop %+v %v", stopped, err)
	}
	started, err := service.StartSandbox(created.SandboxID)
	if err != nil || started.State != "ready" {
		t.Fatalf("desktop start %+v %v", started, err)
	}
	destroyed, err := service.DestroySandbox(created.SandboxID)
	if err != nil || destroyed.State != "destroyed" {
		t.Fatalf("desktop destroy %+v %v", destroyed, err)
	}
	if destroyed.ActiveLeases != 0 {
		t.Fatalf("destroyed Sandbox active leases %+v", destroyed)
	}
	if _, err := service.StartSandbox(created.SandboxID); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("restart tombstone: %v", err)
	}
}

func TestDesktopDockerBackendUnavailableWithoutExplicitSocket(t *testing.T) {
	t.Setenv(bootstrap.DockerSocketEnvironment, "")
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "docker-desktop-no-endpoint.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	service := NewService(ctx, runtime)
	backends, err := service.GetSandboxBackends()
	if err != nil || backends.DockerAvailable {
		t.Fatalf("backend status %+v %v", backends, err)
	}
	_, err = service.CreateDockerSandbox(CreateDockerSandboxInput{Name: "No Engine", ImageDigest: "docker.io/library/alpine@sha256:" + strings.Repeat("a", 64), UserID: 10001, GroupID: 10001, CPUmilli: 1000, MemoryMiB: 512, WorkspaceMiB: 64, PIDs: 64})
	if !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatalf("created Docker Sandbox without socket: %v", err)
	}
	list, err := service.ListSandboxes()
	if err != nil || len(list.Sandboxes) != 0 {
		t.Fatalf("unexpected resources %+v %v", list, err)
	}
}
