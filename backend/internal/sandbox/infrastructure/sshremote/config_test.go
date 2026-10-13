package sshremote

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"golang.org/x/crypto/ssh"
)

func testKeys(t *testing.T) ([]byte, string) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "test")
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(private.Public())
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
}
func TestSSHKeyAndConfigValidation(t *testing.T) {
	t.Parallel()
	secret, hostKey := testKeys(t)
	if err := credentialdomain.ValidateSSHPrivateKey(secret); err != nil {
		t.Fatal(err)
	}
	ref, _ := credentialdomain.ParseReference("credential://ssh/work")
	template := Template{Host: "example.com", Port: 22, User: "agent", Root: "/home/agent/work", HostPublicKey: hostKey, CredentialRef: ref}
	if err := template.Validate(); err != nil {
		t.Fatal(err)
	}
	variations := []func(*Template){
		func(t *Template) { t.Host = "https://evil.example:22" },
		func(t *Template) { t.Port = 0 },
		func(t *Template) { t.User = "root" },
		func(t *Template) { t.Root = "/" },
		func(t *Template) { t.Root = "/workspace/../etc" },
		func(t *Template) { t.HostPublicKey = "" },
		func(t *Template) { t.HostPublicKey += "\n" + hostKey },
		func(t *Template) { t.CredentialRef = "credential://other/key" },
	}
	for _, change := range variations {
		bad := template
		change(&bad)
		if err := bad.Validate(); err == nil {
			t.Fatalf("accepted invalid SSH config: %+v", bad)
		}
	}
	for _, raw := range [][]byte{nil, []byte("definitely-not-a-key"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----")} {
		if err := credentialdomain.ValidateSSHPrivateKey(raw); err == nil {
			t.Fatal("invalid SSH private key accepted")
		}
	}
}

func TestPinnedHostKeySupportsStrongRSAAndECDSAWithNoSHA1(t *testing.T) {
	rsa2048, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsa1024, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		key     any
		allowed bool
	}{
		{"RSA 2048", rsa2048, true},
		{"RSA 1024", rsa1024, false},
		{"ECDSA", ecdsaKey, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signer, err := ssh.NewSignerFromKey(tc.key)
			if err != nil {
				t.Fatal(err)
			}
			line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
			parsed, err := parseHostKey(line)
			if (err == nil) != tc.allowed {
				t.Fatalf("parseHostKey() = %v, expected allowed=%t", err, tc.allowed)
			}
			if !tc.allowed {
				return
			}
			algos := pinnedHostKeyAlgorithms(parsed)
			if len(algos) == 0 {
				t.Fatal("no pinned host key algorithms")
			}
			for _, algo := range algos {
				if algo == ssh.KeyAlgoRSA || algo == ssh.InsecureKeyAlgoDSA {
					t.Fatal("weak SSH host signature algorithm enabled")
				}
			}
			if parsed.Type() == ssh.KeyAlgoRSA && (len(algos) != 2 || algos[0] != ssh.KeyAlgoRSASHA512 || algos[1] != ssh.KeyAlgoRSASHA256) {
				t.Fatal("RSA host key must use RSA-SHA2 signatures")
			}
			if _, err := parseHostKey(line + "\n" + line); err == nil {
				t.Fatal("multiple pinned host keys accepted")
			}
			if _, err := parseHostKey("command=echo " + line); err == nil {
				t.Fatal("authorized_keys options accepted")
			}
		})
	}
}

type localInfo struct {
	name string
	mode fs.FileMode
}

func (i localInfo) Name() string       { return i.name }
func (i localInfo) Size() int64        { return 0 }
func (i localInfo) Mode() fs.FileMode  { return i.mode }
func (i localInfo) ModTime() time.Time { return time.Time{} }
func (i localInfo) IsDir() bool        { return i.mode.IsDir() }
func (i localInfo) Sys() any           { return nil }

type fakeLstat map[string]fs.FileMode

func (f fakeLstat) Lstat(p string) (fs.FileInfo, error) {
	mode, ok := f[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	return localInfo{name: path.Base(p), mode: mode}, nil
}
func TestSSHRootAndRelativePathsRejectTraversals(t *testing.T) {
	t.Parallel()
	state := fakeLstat{"/home": fs.ModeDir | 0700, "/home/agent": fs.ModeDir | 0700, "/home/agent/work": fs.ModeDir | 0700}
	if err := ValidateRoot(state, "/home/agent/work"); err != nil {
		t.Fatal(err)
	}
	state["/home/agent"] = fs.ModeSymlink | 0777
	if err := ValidateRoot(state, "/home/agent/work"); err == nil {
		t.Fatal("symlink accepted")
	}
	for _, p := range []string{"../etc/passwd", "/etc/passwd", "..", "foo/../../etc", "hello\x00world"} {
		if _, err := relative(p, false); err == nil {
			t.Fatalf("unsafe relative path %q accepted", p)
		}
	}
	for _, p := range []string{"file.txt", "./nested/file.txt", "a/b"} {
		if _, err := relative(p, false); err != nil {
			t.Fatal(err)
		}
	}
}

type noCredential struct{}

func (noCredential) Resolve(context.Context, credentialdomain.Reference) ([]byte, error) {
	return nil, errors.New("unavailable")
}
func TestSSHConnectionFailsClosedWithoutCredential(t *testing.T) {
	t.Parallel()
	_, hostKey := testKeys(t)
	ref, _ := credentialdomain.ParseReference("credential://ssh/work")
	c, err := (Dialer{Credentials: noCredential{}}).Connect(context.Background(), Template{Host: "127.0.0.1", Port: 22, User: "agent", Root: "/home/agent", HostPublicKey: hostKey, CredentialRef: ref})
	if err == nil || c != nil {
		t.Fatal("SSH dial succeeded without key")
	}
}
