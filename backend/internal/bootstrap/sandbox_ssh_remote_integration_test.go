package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// A complete, test-owned SSH/SFTP endpoint exercises admission, pinned
// credential handoff, SQLite v11, the Registry/Router lease and local lifecycle.
// It must not query SSH agents, contact user hosts, or write the macOS Keychain.
type sshFixtureCredentialResolver struct{ secret []byte }

func (r sshFixtureCredentialResolver) Resolve(_ context.Context, ref credentialdomain.Reference) ([]byte, error) {
	if ref.String() != "credential://ssh/fixture" {
		return nil, errors.New("unknown fixture key")
	}
	return append([]byte(nil), r.secret...), nil
}

func startInProcessSSHForSandbox(t *testing.T) (sshremote.Template, []byte, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	_, loginKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := ssh.MarshalPrivateKey(loginKey, "mender-fixture")
	if err != nil {
		t.Fatal(err)
	}
	secret := pem.EncodeToMemory(encoded)
	loginPublic, err := ssh.NewPublicKey(loginKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatal(err)
	}
	sshConfig := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if string(key.Marshal()) != string(loginPublic.Marshal()) {
			return nil, errors.New("login rejected")
		}
		return nil, nil
	}}
	sshConfig.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "greeting.txt"), []byte("hello from ssh fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				remote, channels, requests, e := ssh.NewServerConn(conn, sshConfig)
				if e != nil {
					return
				}
				defer func() { _ = remote.Close() }()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					if incoming.ChannelType() != "session" {
						_ = incoming.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					channel, channelRequests, e := incoming.Accept()
					if e != nil {
						continue
					}
					go func() {
						defer func() { _ = channel.Close() }()
						for request := range channelRequests {
							if request.Type != "subsystem" {
								_ = request.Reply(false, nil)
								continue
							}
							var payload struct{ Name string }
							if ssh.Unmarshal(request.Payload, &payload) != nil || payload.Name != "sftp" {
								_ = request.Reply(false, nil)
								return
							}
							_ = request.Reply(true, nil)
							server, e := sftp.NewServer(channel)
							if e != nil {
								return
							}
							_ = server.Serve()
							_ = server.Close()
							return
						}
					}()
				}
			}()
		}
	}()
	_ = ctx
	host, rawPort, _ := net.SplitHostPort(listener.Addr().String())
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		t.Fatal("invalid SSH fixture port")
	}
	ref, _ := credentialdomain.ParseReference("credential://ssh/fixture")
	slashedRoot := filepath.ToSlash(root)
	if !strings.HasPrefix(slashedRoot, "/") {
		slashedRoot = "/" + slashedRoot
	}
	// #nosec G115 -- ephemeral listener port was verified to be 1..65535.
	tpl := sshremote.Template{Host: host, Port: uint16(port), User: "agent", Root: slashedRoot, CredentialRef: ref,
		HostPublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostSigner.PublicKey())))}
	return tpl, secret, root
}

func TestRuntimeSSHRemoteAdmissionRouterWorkspaceAndLocalLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	tpl, secret, workspace := startInProcessSSHForSandbox(t)
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "ssh-endtoend.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	r.Sandboxes.sshRemote.Dial.Credentials = sshFixtureCredentialResolver{secret: secret}
	caller, err := governancedomain.NewContext(governancedomain.OriginInternal, r.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity, err := r.Sandboxes.InspectSSHHostKey(ctx, tpl.Host, tpl.Port, caller)
	if err != nil || firstIdentity.Status != sshremote.HostUntrusted || firstIdentity.HostPublicKey != tpl.HostPublicKey {
		t.Fatalf("initial SSH identity should require first-use confirmation: %+v %v", firstIdentity, err)
	}
	item, err := r.Sandboxes.CreateSSHRemoteSandbox(ctx, CreateSSHRemoteSandboxRequest{
		Name: "Pinned test SSH host", Template: tpl,
	}, caller)
	if err != nil {
		t.Fatalf("create verified SSH resource: %v", err)
	}
	secondIdentity, err := r.Sandboxes.InspectSSHHostKey(ctx, tpl.Host, tpl.Port, caller)
	if err != nil || secondIdentity.Status != sshremote.HostTrusted || secondIdentity.Fingerprint != firstIdentity.Fingerprint {
		t.Fatalf("admitted pinned SSH creation should persist Mender-managed trust: %+v %v", secondIdentity, err)
	}
	if item.Backend() != sandboxdomain.BackendSSHRemote || item.Isolation() != sandboxdomain.IsolationRemote {
		t.Fatalf("SSH resource false identity/isolation: %+v", item.Spec())
	}
	tplStored, err := r.Sandboxes.store.GetSSHRemoteTemplate(ctx, item.ID())
	if err != nil || tplStored != tpl {
		t.Fatalf("persisted nonsecret SSH binding: %+v %v", tplStored, err)
	}
	entries, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if entry.PluginID == "io.mender.sandbox.ssh-remote" {
			count++
			if !entry.Enabled || entry.Status != "enabled" || entry.UsageCount != 1 {
				t.Fatalf("verified SSH resource not reflected in Marketplace: %+v", entry)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one SSH plugin, got %d", count)
	}
	placement, err := r.Sandboxes.router.Resolve(ctx, item.ID(), sandboxdomain.CapabilityRead)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := r.Sandboxes.router.Acquire(ctx, caller, placement)
	if err != nil {
		t.Fatalf("admitted Router failed to acquire SSH: %v", err)
	}
	input, err := invocationdomain.NewJSONValue([]byte(`{"path":"greeting.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	workspaceSession, ok := lease.Session.(interface {
		ExecuteWorkspace(context.Context, sandboxdomain.Capability, invocationdomain.JSONValue) (invocationdomain.Outcome, error)
	})
	if !ok {
		t.Fatal("SSH acquired session lacks Workspace interface")
	}
	outcome, err := workspaceSession.ExecuteWorkspace(ctx, sandboxdomain.CapabilityRead, input)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(outcome.Content().Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Content != "hello from ssh fixture" {
		t.Fatalf("SSH Router SFTP content mismatch: %q", result.Content)
	}
	if _, err := r.Sandboxes.ChangeLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStop, caller); !errors.Is(err, sandboxdomain.ErrConflict) {
		t.Fatalf("SSH stop bypassed durable execution lease: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("SSH session close failed: %v", err)
	}
	if _, err := r.Sandboxes.ChangeLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStop, caller); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Sandboxes.ChangeLifecycle(ctx, item.ID(), governancedomain.ActionSandboxStart, caller); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Sandboxes.ChangeLifecycle(ctx, item.ID(), governancedomain.ActionSandboxDestroy, caller); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- workspace is the canonical test-owned temporary directory.
	data, err := os.ReadFile(filepath.Join(workspace, "greeting.txt"))
	if err != nil || string(data) != "hello from ssh fixture\n" {
		t.Fatalf("local SSH tombstone deleted/modified remote fixture files: %q %v", data, err)
	}
}
