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
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
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
	return sshFixtureWithSFTP(t, private, hostKey, true)
}

func sshFixtureWithSFTP(t *testing.T, private []byte, hostKey string, enableSFTP bool) (Template, fixtureCredentials) {
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
								if payload.Name != "sftp" || !enableSFTP {
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
								command := strings.ReplaceAll(payload.Command, remoteFixturePath(root), filepath.ToSlash(root))
								// #nosec G204 -- command is supplied only by this test's ephemeral SSH client; fixture is local and test-owned.
								cmd := exec.Command("sh", "-c", command)
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
	return Template{Host: host, Port: uint16(port), User: "agent", Root: remoteFixturePath(root), HostPublicKey: hostKey, CredentialRef: ref}, fixtureCredentials{private}
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
	info, err := connection.SFTP.Stat(path.Join(template.Root, "hello.txt"))
	if err != nil || info.IsDir() {
		t.Fatalf("RSA-2048 pinned SFTP failed: info=%v, err=%v", info, err)
	}
}

func TestSSHRemoteRootLoginUsesPinnedKeyAndSFTP(t *testing.T) {
	// A test-owned SSH server accepts a login named "root"; the fixture runs
	// under the test process account and does not contact a real root host.
	template, credentials := sshFixture(t)
	template.User = "root"
	if err := template.Validate(); err != nil {
		t.Fatalf("root SSH login should pass template validation: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	connection, err := (Dialer{Credentials: credentials}).Connect(ctx, template)
	if err != nil {
		t.Fatalf("root SSH login with explicit pinned host key failed: %v", err)
	}
	defer func() { _ = connection.Close() }()
	info, err := connection.SFTP.Stat(path.Join(template.Root, "hello.txt"))
	if err != nil || info.IsDir() {
		t.Fatalf("root SSH session SFTP verification failed: info=%v, err=%v", info, err)
	}
}

func TestSSHRemoteProvisioningReportsActualFailureStage(t *testing.T) {
	t.Run("mismatched pinned host key", func(t *testing.T) {
		template, credentials := sshFixture(t)
		_, unrelatedHostKey := testKeys(t)
		template.HostPublicKey = unrelatedHostKey
		_, err := (Dialer{Credentials: credentials}).Connect(context.Background(), template)
		expectConnectionFailure(t, err, FailureHostKeyMismatch)
	})

	t.Run("rejected public key authentication", func(t *testing.T) {
		template, credentials := sshFixture(t)
		unrelatedLoginKey, _ := testKeys(t)
		credentials.secret = unrelatedLoginKey
		_, err := (Dialer{Credentials: credentials}).Connect(context.Background(), template)
		expectConnectionFailure(t, err, FailureAuthenticationFailed)
	})

	t.Run("SFTP subsystem not enabled", func(t *testing.T) {
		private, hostKey := testKeys(t)
		template, credentials := sshFixtureWithSFTP(t, private, hostKey, false)
		_, err := (Dialer{Credentials: credentials}).Connect(context.Background(), template)
		expectConnectionFailure(t, err, FailureSFTPUnavailable)
	})

	t.Run("remote workspace does not exist", func(t *testing.T) {
		template, credentials := sshFixture(t)
		template.Root = path.Join(template.Root, "missing-folder")
		_, err := (Dialer{Credentials: credentials}).Connect(context.Background(), template)
		expectConnectionFailure(t, err, FailureWorkspaceMissing)
	})

	t.Run("remote workspace is not a directory", func(t *testing.T) {
		template, credentials := sshFixture(t)
		template.Root = path.Join(template.Root, "hello.txt")
		_, err := (Dialer{Credentials: credentials}).Connect(context.Background(), template)
		expectConnectionFailure(t, err, FailureWorkspaceInvalid)
	})

	t.Run("remote workspace contains symbolic link", func(t *testing.T) {
		template, credentials := sshFixture(t)
		symlink := filepath.Join(localFixturePath(template.Root), "redirect")
		if err := os.Symlink(localFixturePath(template.Root), symlink); err != nil {
			t.Fatal(err)
		}
		template.Root = remoteFixturePath(symlink)
		_, err := (Dialer{Credentials: credentials}).Connect(context.Background(), template)
		expectConnectionFailure(t, err, FailureWorkspaceInvalid)
	})
}

func execute(t *testing.T, s *Session, cap sandboxdomain.Capability, arg any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(arg)
	if err != nil {
		t.Fatal(err)
	}
	var result any
	switch cap {
	case sandboxdomain.CapabilityRead:
		var in workspace.ReadRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		result, err = s.Read(context.Background(), workspaceapp.NormalizeRead(in))
	case sandboxdomain.CapabilitySearch:
		var in workspace.SearchRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		result, err = s.Search(context.Background(), workspaceapp.NormalizeSearch(in))
	case sandboxdomain.CapabilityEdit:
		var in workspace.EditRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		result, err = s.Edit(context.Background(), workspaceapp.NormalizeEdit(in))
	case sandboxdomain.CapabilityShell:
		var in workspace.ShellRequest
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		result, err = s.Shell(context.Background(), workspaceapp.NormalizeShell(in))
	default:
		t.Fatalf("unsupported fixture capability %q", cap)
	}
	if err != nil {
		t.Fatalf("%s: %v", cap, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("%s decode %v", cap, err)
	}
	return output
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
	t.Run("typed read and search fields", func(t *testing.T) {
		literal, err := s.Read(ctx, workspaceapp.NormalizeRead(workspace.ReadRequest{Path: "./hello.txt"}))
		if err != nil || literal.Path != "./hello.txt" || literal.StartLine != 1 || literal.EndLine != 1 {
			t.Fatalf("SSH literal path or line range changed: %+v %v", literal, err)
		}
		beyond, err := s.Read(ctx, workspaceapp.NormalizeRead(workspace.ReadRequest{Path: "hello.txt", Offset: 5}))
		if err != nil || beyond != (workspace.ReadResult{Path: "hello.txt", StartLine: 2, EndLine: 1}) {
			t.Fatalf("SSH out-of-range line fields changed: %+v %v", beyond, err)
		}
		for name, data := range map[string]string{"empty.txt": "", "empty-line.txt": "\n"} {
			if err := os.WriteFile(filepath.Join(localFixturePath(template.Root), name), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			empty, err := s.Read(ctx, workspaceapp.NormalizeRead(workspace.ReadRequest{Path: name}))
			if err != nil || empty != (workspace.ReadResult{Path: name, StartLine: 1}) {
				t.Fatalf("SSH empty file line fields changed: %+v %v", empty, err)
			}
		}
		empty, err := s.Search(ctx, workspaceapp.NormalizeSearch(workspace.SearchRequest{Mode: "content", Query: "^$", Regex: true}))
		if err != nil || len(empty.Matches) != 2 {
			t.Fatalf("SSH empty-line matches changed: %+v %v", empty, err)
		}
		for _, match := range empty.Matches {
			if match.Line != 1 || match.Text != nil {
				t.Fatalf("SSH empty snippet presence changed: %+v", match)
			}
		}
	})
	t.Run("pre-cancelled typed operations", func(t *testing.T) {
		cancelled, stop := context.WithCancel(context.Background())
		stop()
		operations := map[string]func() error{
			"read": func() error {
				_, err := s.Read(cancelled, workspaceapp.NormalizeRead(workspace.ReadRequest{Path: "hello.txt"}))
				return err
			},
			"search": func() error {
				_, err := s.Search(cancelled, workspaceapp.NormalizeSearch(workspace.SearchRequest{Mode: "files", Pattern: "*"}))
				return err
			},
			"edit": func() error {
				_, err := s.Edit(cancelled, workspaceapp.NormalizeEdit(workspace.EditRequest{Path: "must-not-exist", Operation: "create"}))
				return err
			},
			"shell": func() error {
				_, err := s.Shell(cancelled, workspaceapp.NormalizeShell(workspace.ShellRequest{Command: "must-not-run"}))
				return err
			},
		}
		for name, operation := range operations {
			if err := operation(); !errors.Is(err, context.Canceled) {
				t.Fatalf("%s lost pre-execution cancellation: %v", name, err)
			}
		}
		if s.uncertain.Load() {
			t.Fatal("pre-execution cancellation marked an untouched transport uncertain")
		}
		if _, err := s.conn.SFTP.Lstat(path.Join(template.Root, "must-not-exist")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cancelled edit reached SFTP: %v", err)
		}
	})
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
	duplicate, err := s.Edit(ctx, workspaceapp.NormalizeEdit(workspace.EditRequest{
		Path: "added.txt", Operation: "create", Content: "must-not-overwrite",
	}))
	var toolError *workspace.ToolError
	if !errors.As(err, &toolError) {
		t.Fatalf("exclusive SFTP create did not reject existing file: %v %+v", err, duplicate)
	}
	preserved := execute(t, s, sandboxdomain.CapabilityRead, map[string]any{"path": "added.txt"})
	if preserved["content"] != "created" {
		t.Fatalf("exclusive SFTP create overwrote original bytes: %+v", preserved)
	}
	t.Run("remote POSIX shell", func(t *testing.T) {
		if _, err := exec.LookPath("sh"); err != nil {
			t.Skip("local SSH server fixture has no POSIX sh; Windows clients can still use a remote Linux shell")
		}
		result := execute(t, s, sandboxdomain.CapabilityShell, map[string]any{"command": "pwd && cat hello.txt"})
		if result["exitCode"] != float64(0) || !strings.Contains(fmt.Sprint(result["stdout"]), "hello secure") {
			t.Fatalf("shell: %+v", result)
		}
	})
	// Trusted remote server may support symbolic links; structured tools
	// must refuse them, not silently follow outside the configured root.
	if err := os.Symlink("hello.txt", filepath.Join(localFixturePath(template.Root), "linked.txt")); err != nil {
		t.Fatal(err)
	}
	symlinkRead, err := s.Read(ctx, workspaceapp.NormalizeRead(workspace.ReadRequest{Path: "linked.txt"}))
	if !errors.As(err, &toolError) {
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
