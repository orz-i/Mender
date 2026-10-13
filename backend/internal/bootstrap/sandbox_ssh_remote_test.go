package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
	"golang.org/x/crypto/ssh"
)

// An SSH credential must not be deleted while any active SSH Sandbox uses
// its reference; reading usage does not need access to the raw keyring secret.
func TestSSHHostIdentityInspectionRequiresLocalOperatorBeforeNetwork(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "ssh-inspection.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	service, err := gov.NewPrincipal(gov.PrincipalService, "profile:untrusted")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		principal gov.Principal
		origin    gov.Origin
	}{
		{service, gov.OriginMCP},
		{service, gov.OriginInternal},
		{r.OwnerPrincipal(), gov.OriginMCP},
	} {
		caller, err := gov.NewContext(input.origin, input.principal, gov.CatalogScope())
		if err != nil {
			t.Fatal(err)
		}
		// No remote socket/key resolution is permitted for untrusted callers.
		if _, err := r.Sandboxes.InspectSSHHostKey(ctx, "198.51.100.9", 22, caller); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
			t.Fatalf("untrusted SSH inspection admitted for %s/%s: %v", input.principal.Kind(), input.origin, err)
		}
	}
}

func TestSSHRemoteCreateRequiresLocalOperatorBeforeKeyResolution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "no-ssh-access.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(key.Public())
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := credentialdomain.ParseReference("credential://ssh/unavailable")
	request := CreateSSHRemoteSandboxRequest{Name: "Restricted remote", Template: sshremote.Template{
		Host: "127.0.0.1", Port: 22, User: "agent", Root: "/home/agent/work",
		CredentialRef: ref, HostPublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))),
	}}
	service, err := gov.NewPrincipal(gov.PrincipalService, "profile:untrusted")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		kind   gov.Principal
		origin gov.Origin
	}{
		{service, gov.OriginMCP}, {service, gov.OriginInternal}, {r.OwnerPrincipal(), gov.OriginMCP},
	} {
		caller, err := gov.NewContext(tc.origin, tc.kind, gov.CatalogScope())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Sandboxes.CreateSSHRemoteSandbox(ctx, request, caller); !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
			t.Fatalf("untrusted SSH host connection admitted for %s/%s: %v",
				tc.kind.Kind(), tc.origin, err)
		}
	}
	items, err := r.Sandboxes.List(ctx)
	if err != nil || len(items) != 0 {
		t.Fatalf("denied SSH attempts created resources: %+v %v", items, err)
	}
}

// An SSH credential must not be deleted while any active SSH Sandbox uses
// its reference; reading usage does not need access to the raw keyring secret.
func TestSSHRemoteSandboxCredentialUsageCannotDisappearUntilDestroyed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "ssh-inventory.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := r.OwnerPrincipal()
	caller, err := gov.NewContext(gov.OriginInternal, owner, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := credentialdomain.ParseReference("credential://ssh/fixture")
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(key.Public())
	if err != nil {
		t.Fatal(err)
	}
	id := sandboxdomain.ID("sbx_00000000000000000000000000000abc")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID: id, Name: "SSH credential reference fixture", Backend: sandboxdomain.BackendSSHRemote,
		Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote), Isolation: sandboxdomain.IsolationRemote,
		OwnerKind: string(owner.Kind()), OwnerRef: owner.Reference(),
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "ssh_remote:"+id.String())
	if err != nil {
		t.Fatal(err)
	}
	template := sshremote.Template{
		Host: "host.example.com", Port: 22, User: "agent", Root: "/home/agent/work", CredentialRef: ref,
		HostPublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))),
	}
	if err := r.Sandboxes.store.CreateReadySSH(ctx, item, run, template); err != nil {
		t.Fatal(err)
	}
	usage := runtimeCredentialUsageChecker{
		profiles: r.Profiles, tunnels: r.Tunnels, sandboxes: r.Sandboxes, configs: r.providerConfigs,
	}
	active, err := usage.Usages(ctx, ref)
	if err != nil || len(active) != 1 || active[0].Kind != "sandbox" {
		t.Fatalf("SSH reference not protected from deletion: %+v %v", active, err)
	}
	if _, err := r.Sandboxes.ChangeLifecycle(ctx, id, gov.ActionSandboxStop, caller); err != nil {
		t.Fatal(err)
	}
	stopped, err := usage.Usages(ctx, ref)
	if err != nil || len(stopped) != 1 {
		t.Fatalf("stopped SSH binding was unprotected: %+v %v", stopped, err)
	}
	if _, err := r.Sandboxes.ChangeLifecycle(ctx, id, gov.ActionSandboxDestroy, caller); err != nil {
		t.Fatal(err)
	}
	removed, err := usage.Usages(ctx, ref)
	if err != nil || len(removed) != 0 {
		t.Fatalf("SSH tombstone prevents future credential deletion: %+v %v", removed, err)
	}
}
