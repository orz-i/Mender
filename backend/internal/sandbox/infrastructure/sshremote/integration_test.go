package sshremote

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type fixtureCredentials struct{ secret []byte }

func (c fixtureCredentials) Resolve(context.Context, credentialdomain.Reference) ([]byte, error) {
	return append([]byte(nil), c.secret...), nil
}

func sshFixture(t *testing.T) (Template, fixtureCredentials) {
	t.Helper()
	private, hostKey := testKeys(t)
	return sshFixtureFromKey(t, private, hostKey)
}

func sshFixtureFromKey(t *testing.T, private []byte, hostKey string) (Template, fixtureCredentials) {
	t.Helper()
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKey))
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := ssh.ParsePrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello ssh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sshConfig := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if string(key.Marshal()) != string(hostSigner.PublicKey().Marshal()) {
			return nil, errors.New("unknown login key")
		}
		return nil, nil
	}}
	sshConfig.AddHostKey(hostSigner)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			raw, e := listener.Accept()
			if e != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				conn, chans, requests, e := ssh.NewServerConn(raw, sshConfig)
				if e != nil {
					_ = raw.Close()
					return
				}
				defer func() { _ = conn.Close() }()
				go ssh.DiscardRequests(requests)
				for incoming := range chans {
					if incoming.ChannelType() != "session" {
						_ = incoming.Reject(ssh.UnknownChannelType, "only session")
						continue
					}
					ch, reqs, e := incoming.Accept()
					if e != nil {
						continue
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer func() { _ = ch.Close() }()
						for req := range reqs {
							switch req.Type {
							case "subsystem":
								var payload struct{ Name string }
								if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
									_ = req.Reply(false, nil)
									return
								}
								if payload.Name != "sftp" {
									_ = req.Reply(false, nil)
									continue
								}
								_ = req.Reply(true, nil)
								server, e := sftp.NewServer(ch)
								if e == nil {
									_ = server.Serve()
									_ = server.Close()
								}
								return
							case "exec":
								var payload struct{ Command string }
								if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
									_ = req.Reply(false, nil)
									return
								}
								_ = req.Reply(true, nil)
								// #nosec G204 -- command is supplied only by this test's ephemeral SSH client; fixture is local and test-owned.
								cmd := exec.Command("sh", "-c", payload.Command)
								cmd.Dir = root
								cmd.Stdout = ch
								cmd.Stderr = ch.Stderr()
								status := uint32(0)
								if e := cmd.Run(); e != nil {
									status = 1
								}
								_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
								return
							default:
								_ = req.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	t.Cleanup(func() { _ = listener.Close() }) // do not wait on slow SSH failure
	_ = pub
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		t.Fatal("invalid ephemeral SSH listener port")
	}
	ref, _ := credentialdomain.ParseReference("credential://ssh/fixture")
	// #nosec G115 -- the listener port was checked to be within 1..65535.
	return Template{Host: host, Port: uint16(port), User: "agent", Root: root, HostPublicKey: hostKey, CredentialRef: ref}, fixtureCredentials{private}
}
func TestSSHRemoteRSA2048UsesSHA2ForClientAndPinnedHost(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "generated-fixture")
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(block)
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	hostLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	template, credentials := sshFixtureFromKey(t, keyPEM, hostLine)
	if err := credentialdomain.ValidateSSHPrivateKey(credentials.secret); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	connection, err := (Dialer{Credentials: credentials}).Connect(ctx, template)
	if err != nil {
		t.Fatalf("RSA-2048 pinned SSH connection failed: %v", err)
	}
	defer func() { _ = connection.Close() }()
	info, err := connection.SFTP.Stat(filepath.Join(template.Root, "hello.txt"))
	if err != nil || info.IsDir() {
		t.Fatalf("RSA-2048 pinned SFTP failed: info=%v, err=%v", info, err)
	}
}

func jsonArg(t *testing.T, content any) invocationdomain.JSONValue {
	t.Helper()
	data, e := json.Marshal(content)
	if e != nil {
		t.Fatal(e)
	}
	value, e := invocationdomain.NewJSONValue(data)
	if e != nil {
		t.Fatal(e)
	}
	return value
}
func execute(t *testing.T, s *Session, cap sandboxdomain.Capability, arg any) map[string]any {
	t.Helper()
	outcome, err := s.ExecuteWorkspace(context.Background(), cap, jsonArg(t, arg))
	if err != nil {
		t.Fatalf("%s: %v", cap, err)
	}
	var result map[string]any
	if e := json.Unmarshal(outcome.Content().Bytes(), &result); e != nil {
		t.Fatalf("%s decode %v", cap, e)
	}
	return result
}
func TestSSHRemoteWorkspaceOverPinnedSSHAndSFTP(t *testing.T) {
	template, creds := sshFixture(t)
	dial := Dialer{Credentials: creds}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	connection, err := dial.Connect(ctx, template)
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{conn: connection, root: template.Root}
	defer func() { _ = s.Close() }()
	read := execute(t, s, sandboxdomain.CapabilityRead, map[string]any{"path": "hello.txt"})
	if read["content"] != "hello ssh" {
		t.Fatalf("read: %+v", read)
	}
	search := execute(t, s, sandboxdomain.CapabilitySearch, map[string]any{"mode": "content", "query": "ssh"})
	matches, ok := search["matches"].([]any)
	if !ok || len(matches) != 1 {
		t.Fatalf("search: %+v", search)
	}
	edited := execute(t, s, sandboxdomain.CapabilityEdit, map[string]any{"path": "hello.txt", "operation": "replace",
		"edits": []map[string]string{{"oldText": "ssh", "newText": "secure"}}})
	if edited["replacements"] != float64(1) {
		t.Fatalf("edit: %+v", edited)
	}
	after := execute(t, s, sandboxdomain.CapabilityRead, map[string]any{"path": "hello.txt"})
	if after["content"] != "hello secure" {
		t.Fatalf("replace result: %+v", after)
	}
	created := execute(t, s, sandboxdomain.CapabilityEdit, map[string]any{"path": "added.txt", "operation": "create", "content": "created"})
	if created["operation"] != "create" {
		t.Fatalf("create: %+v", created)
	}
	// Repeated exclusive create must never overwrite the existing target.
	duplicate, err := s.ExecuteWorkspace(ctx, sandboxdomain.CapabilityEdit, jsonArg(t, map[string]any{
		"path": "added.txt", "operation": "create", "content": "must-not-overwrite",
	}))
	if err != nil || duplicate.Kind() != invocationdomain.OutcomeKindFailed {
		t.Fatalf("exclusive SFTP create did not reject existing file: %v %+v", err, duplicate)
	}
	preserved := execute(t, s, sandboxdomain.CapabilityRead, map[string]any{"path": "added.txt"})
	if preserved["content"] != "created" {
		t.Fatalf("exclusive SFTP create overwrote original bytes: %+v", preserved)
	}
	result := execute(t, s, sandboxdomain.CapabilityShell, map[string]any{"command": "pwd && cat hello.txt"})
	if result["exitCode"] != float64(0) || !strings.Contains(fmt.Sprint(result["stdout"]), "hello secure") {
		t.Fatalf("shell: %+v", result)
	}
	// Trusted remote server may support symbolic links; structured tools
	// must refuse them, not silently follow outside the configured root.
	if err := os.Symlink("hello.txt", filepath.Join(template.Root, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	symlinkRead, err := s.ExecuteWorkspace(ctx, sandboxdomain.CapabilityRead, jsonArg(t,
		map[string]any{"path": "linked.txt"}))
	if err != nil || symlinkRead.Kind() != invocationdomain.OutcomeKindFailed {
		t.Fatalf("structured SFTP read followed a symlink: %v %+v", err, symlinkRead)
	}
	deleted := execute(t, s, sandboxdomain.CapabilityEdit, map[string]any{"path": "added.txt", "operation": "delete"})
	if deleted["operation"] != "delete" {
		t.Fatalf("delete: %+v", deleted)
	}
	badHost := template
	_, different, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(different.Public())
	if err != nil {
		t.Fatal(err)
	}
	badHost.HostPublicKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	if conn, err := dial.Connect(ctx, badHost); err == nil || conn != nil {
		t.Fatal("SSH handshake accepted wrong pinned host public key")
	}
}

var _ = time.Second
