package sshremote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func testTrustStore(t *testing.T) *HostTrustStore {
	t.Helper()
	dir := t.TempDir()
	return &HostTrustStore{
		ManagedFile: filepath.Join(dir, "mender", "ssh_known_hosts"),
		UserFile:    filepath.Join(dir, ".ssh", "known_hosts"),
	}
}

func TestHostIdentityFirstUseTrustAndPinnedReuse(t *testing.T) {
	template, _ := sshFixture(t)
	trust := testTrustStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	first, err := trust.Inspect(ctx, template.Host, template.Port)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != HostUntrusted || first.Fingerprint == "" || !strings.HasPrefix(first.Fingerprint, "SHA256:") ||
		first.Algorithm != ssh.KeyAlgoED25519 || first.HostPublicKey != template.HostPublicKey {
		t.Fatalf("new SSH host identity = %+v", first)
	}
	if _, err := os.Stat(trust.ManagedFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("host identity probe wrote trust before operator confirmation")
	}

	if err := trust.RememberPinned(template.Host, template.Port, first.HostPublicKey); err != nil {
		t.Fatal(err)
	}
	after, err := trust.Inspect(ctx, template.Host, template.Port)
	if err != nil || after.Status != HostTrusted || after.Fingerprint != first.Fingerprint {
		t.Fatalf("trusted identity=%+v, err=%v", after, err)
	}
	if err := trust.RememberPinned(template.Host, template.Port, first.HostPublicKey); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(trust.ManagedFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(content), "ssh-ed25519") != 1 {
		t.Fatal("repeated provisioning created duplicate known_hosts pins")
	}
	info, err := os.Stat(trust.ManagedFile)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("Mender trust file permissions = %v, err %v", info, err)
	}
}

func TestHostIdentityChangedPinnedKeyBlocksWithoutAutoTrust(t *testing.T) {
	template, _ := sshFixture(t)
	trust := testTrustStore(t)
	_, otherHost := testKeys(t)
	if err := trust.RememberPinned(template.Host, template.Port, otherHost); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(trust.ManagedFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	observed, err := trust.Inspect(ctx, template.Host, template.Port)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != HostChanged {
		t.Fatalf("changed pinned host key must be blocked, status=%s", observed.Status)
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(template.HostPublicKey))
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.VerifyPin(template.Host, template.Port, key); err == nil {
		t.Fatal("changed host key was accepted for connection")
	}
	after, err := os.ReadFile(trust.ManagedFile)
	if err != nil || string(after) != string(previous) {
		t.Fatal("changed key inspection modified trusted host identity")
	}
}

func TestHostIdentityHonorsOpenSSHHashedKnownHosts(t *testing.T) {
	template, _ := sshFixture(t)
	trust := testTrustStore(t)
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(template.HostPublicKey))
	if err != nil {
		t.Fatal(err)
	}
	// Hash the exact known_hosts address, including a non-default SSH port.
	endpoint := knownhosts.Normalize(fmt.Sprintf("%s:%d", template.Host, template.Port))
	hashed := knownhosts.HashHostname(endpoint)
	line := hashed + " " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + "\n"
	if err := os.MkdirAll(filepath.Dir(trust.UserFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trust.UserFile, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result, err := trust.Inspect(ctx, template.Host, template.Port)
	if err != nil || result.Status != HostTrusted {
		t.Fatalf("hashed known_hosts trust not recognized: %+v err=%v", result, err)
	}
}

func TestHostTrustManagedSymlinkRejected(t *testing.T) {
	template, _ := sshFixture(t)
	trust := testTrustStore(t)
	safe := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(safe, []byte("do not modify"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(trust.ManagedFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(safe, trust.ManagedFile); err != nil {
		t.Fatal(err)
	}
	if err := trust.RememberPinned(template.Host, template.Port, template.HostPublicKey); err == nil {
		t.Fatal("symlink managed trust file accepted")
	}
	// #nosec G304 -- test-only file created inside t.TempDir, verifies symlink denial.
	unchanged, err := os.ReadFile(safe)
	if err != nil || string(unchanged) != "do not modify" {
		t.Fatal("managed trust attempted to write through symlink")
	}
}
