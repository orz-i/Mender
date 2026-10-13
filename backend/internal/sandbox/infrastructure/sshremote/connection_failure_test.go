package sshremote

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/pkg/sftp"
)

func expectConnectionFailure(t *testing.T, err error, want ConnectionFailureCode) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a connection failure")
	}
	got, ok := FailureCodeOf(err)
	if !ok || got != want {
		t.Fatalf("failure code %q (present=%t), want %q", got, ok, want)
	}
	if !errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
		t.Fatal("connection failure no longer preserves backend unavailable semantics")
	}
	if gotMessage := err.Error(); !strings.HasSuffix(gotMessage, "ssh_remote_failure:"+string(want)) {
		t.Fatalf("unexpected sanitized diagnostic for code %q", want)
	}
	if strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "/private/") {
		t.Fatal("unsafe diagnostic in returned connection error")
	}
}

func TestSSHFailureToolDiagnosticOnlyEmitsAllowlistedStage(t *testing.T) {
	for _, code := range []ConnectionFailureCode{
		FailureCredentialUnavailable, FailureNetworkTimeout,
		FailureHostKeyMismatch, FailureAuthenticationFailed,
		FailureSFTPUnavailable, FailureWorkspaceMissing,
	} {
		err := newConnectionFailure(code)
		var diagnostic interface{ DiagnosticSummary() string }
		if !errors.As(err, &diagnostic) {
			t.Fatal("typed SSH failure lacks Tool diagnostic interface")
		}
		if got := diagnostic.DiagnosticSummary(); got != "ssh_remote_failure:"+string(code) {
			t.Fatalf("SSH Tool summary = %q for code %q", got, code)
		}
	}
	var nilFailure *ConnectionFailure
	if got := nilFailure.DiagnosticSummary(); got != "" {
		t.Fatal("nil SSH failure exposed diagnostic")
	}
	if got := (&ConnectionFailure{Code: "untrusted/PRIVATE-KEY"}).DiagnosticSummary(); got != "" {
		t.Fatal("arbitrary SSH error value escaped allowlist")
	}
}

func TestConnectionFailureCodesDoNotLeakRemoteErrors(t *testing.T) {
	t.Parallel()
	if _, ok := FailureCodeOf(errors.New("not a connection failure")); ok {
		t.Fatal("arbitrary error was classified as typed SSH failure")
	}
	expectConnectionFailure(t, newConnectionFailure(FailureCredentialUnavailable), FailureCredentialUnavailable)
	expectConnectionFailure(t, networkFailure(&net.OpError{
		Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED,
		Addr: &net.TCPAddr{IP: net.IPv4(192, 0, 2, 42), Port: 1234},
	}), FailureNetworkRefused)
	expectConnectionFailure(t, networkFailure(&net.DNSError{Err: "sensitive DNS response", IsTimeout: true}), FailureNetworkTimeout)
	expectConnectionFailure(t, networkFailure(&net.DNSError{Err: "sensitive remote name"}), FailureNetworkUnreachable)
	expectConnectionFailure(t, handshakeFailure(errors.New("sensitive remote host key"), true), FailureHostKeyMismatch)
	expectConnectionFailure(t, handshakeFailure(errors.New("ssh: unable to authenticate, sensitive server payload"), false), FailureAuthenticationFailed)
	expectConnectionFailure(t, handshakeFailure(errors.New("sensitive handshake algorithms"), false), FailureSSHHandshakeFailed)
	expectConnectionFailure(t, workspaceFailure(os.ErrNotExist), FailureWorkspaceMissing)
	expectConnectionFailure(t, workspaceFailure(os.ErrPermission), FailureWorkspacePermission)
	expectConnectionFailure(t, workspaceFailure(&sftp.StatusError{Code: 2}), FailureWorkspaceMissing)
	expectConnectionFailure(t, workspaceFailure(&sftp.StatusError{Code: 3}), FailureWorkspacePermission)
	expectConnectionFailure(t, workspaceFailure(errors.New("sensitive /private/root directory")), FailureWorkspaceInvalid)
}

func TestConnectionRefusalIsReportedBeforeHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	template, credentials := sshFixture(t)
	// Use a test-owned closed listener port, never a real remote address.
	template.Port = uint16(addr.Port) // #nosec G115 -- OS-selected port is 1..65535
	_, err = (Dialer{Credentials: credentials}).Connect(context.Background(), template)
	expectConnectionFailure(t, err, FailureNetworkRefused)
}
