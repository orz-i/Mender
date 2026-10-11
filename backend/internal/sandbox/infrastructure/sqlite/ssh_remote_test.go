package sqlite

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"strings"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
	"golang.org/x/crypto/ssh"
)

func TestSSHRemoteV11ImmutableTemplateCredentialProtectionAndPassiveLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "ssh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(private.Public())
	if err != nil {
		t.Fatal(err)
	}
	principal, err := gov.NewPrincipal(gov.PrincipalLocalUser, "os:test")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginCLI, principal, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := credentialdomain.ParseReference("credential://ssh/build")
	id := sandboxdomain.ID("sbx_000000000000000000000000000000aa")
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{
		ID: id, Name: "SSH remote fixture", Backend: sandboxdomain.BackendSSHRemote,
		Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote), Isolation: sandboxdomain.IsolationRemote,
		OwnerKind: string(principal.Kind()), OwnerRef: principal.Reference(),
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "ssh_remote:"+id.String())
	if err != nil {
		t.Fatal(err)
	}
	tpl := sshremote.Template{Host: "host.example.com", Port: 22, User: "agent", Root: "/home/agent/work",
		CredentialRef: ref, HostPublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))}
	if err := store.CreateReadySSH(ctx, item, run, tpl); err != nil {
		t.Fatal(err)
	}
	again, err := store.GetSSHRemoteTemplate(ctx, id)
	if err != nil || again != tpl {
		t.Fatalf("template %+v, %v", again, err)
	}
	refs, err := store.SSHCredentialBindings(ctx, ref)
	if err != nil || len(refs) != 1 || refs[0] != id {
		t.Fatalf("active credential binding %+v %v", refs, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE sandbox_ssh_remote_templates SET host='evil' WHERE sandbox_id=?`, id.String()); err == nil {
		t.Fatal("immutable host binding was overwritten")
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE sandboxes SET backend_plugin_id='io.mender.sandbox.docker' WHERE id=?`, id.String()); err == nil {
		t.Fatal("SSH plugin identity changed without authorization")
	}
	// In the complete Runtime, the Provider config table is composed before
	// Sandbox lifecycle. Provide only its queried shape in this isolated store.
	if _, err := store.db.ExecContext(ctx, `CREATE TABLE provider_configs (
		plugin_id TEXT, plugin_installation_id TEXT, config_version INTEGER, config_json TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	for _, action := range []gov.Action{gov.ActionSandboxStop, gov.ActionSandboxStart, gov.ActionSandboxDestroy} {
		previous, err := store.GetRun(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		next, err := store.ChangeSSHRemoteLifecycle(ctx, previous, action, caller)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if next.SandboxID != id {
			t.Fatal("lifecycle changed ownership")
		}
	}
	refs, err = store.SSHCredentialBindings(ctx, ref)
	if err != nil || len(refs) != 0 {
		t.Fatalf("destroyed resource blocks credential forever: %+v %v", refs, err)
	}
	if _, err := store.GetSSHRemoteTemplate(ctx, id); err != nil {
		t.Fatal("tombstone lost immutable SSH config")
	}
}
