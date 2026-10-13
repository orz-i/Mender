package sshremote

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/pkg/sftp"
)

// ConnectionFailureCode is a stable, allowlisted diagnostic for provisioning.
// No hostname, username, key, remote path, or remote diagnostic may appear in
// the error. It is safe for Wails and the management HTTP API to return.
type ConnectionFailureCode string

const (
	FailureCredentialUnavailable ConnectionFailureCode = "credential_unavailable" // #nosec G101 -- stable failure code, not credential material
	FailureCredentialInvalid     ConnectionFailureCode = "credential_invalid"     // #nosec G101 -- stable failure code, not credential material
	FailureNetworkTimeout        ConnectionFailureCode = "network_timeout"
	FailureNetworkRefused        ConnectionFailureCode = "network_refused"
	FailureNetworkUnreachable    ConnectionFailureCode = "network_unreachable"
	FailureHostKeyMismatch       ConnectionFailureCode = "host_key_mismatch"
	FailureHostTrustUnavailable  ConnectionFailureCode = "host_trust_unavailable"
	FailureAuthenticationFailed  ConnectionFailureCode = "authentication_failed"
	FailureSSHHandshakeFailed    ConnectionFailureCode = "ssh_handshake_failed"
	FailureSFTPUnavailable       ConnectionFailureCode = "sftp_unavailable"
	FailureWorkspaceMissing      ConnectionFailureCode = "workspace_missing"
	FailureWorkspacePermission   ConnectionFailureCode = "workspace_permission_denied"
	FailureWorkspaceInvalid      ConnectionFailureCode = "workspace_invalid"
)

// ConnectionFailure preserves unavailable semantics for existing callers
// while exposing only an actionable, bounded reason code.
type ConnectionFailure struct {
	Code ConnectionFailureCode
}

func (e *ConnectionFailure) Error() string {
	return "ssh_remote_failure:" + string(e.Code)
}

func (*ConnectionFailure) Unwrap() error {
	return sandboxdomain.ErrBackendUnavailable
}

func newConnectionFailure(code ConnectionFailureCode) error {
	return &ConnectionFailure{Code: code}
}

// FailureCodeOf inspects wrapped errors without relying on human messages.
func FailureCodeOf(err error) (ConnectionFailureCode, bool) {
	var diagnostic *ConnectionFailure
	if errors.As(err, &diagnostic) {
		return diagnostic.Code, true
	}
	return "", false
}

func networkFailure(err error) error {
	var networkErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded),
		errors.As(err, &networkErr) && networkErr.Timeout():
		return newConnectionFailure(FailureNetworkTimeout)
	case errors.Is(err, syscall.ECONNREFUSED):
		return newConnectionFailure(FailureNetworkRefused)
	default:
		return newConnectionFailure(FailureNetworkUnreachable)
	}
}

// The x/crypto/ssh package currently reports failed public-key authentication
// with this internal marker. It is used strictly to categorize the failure;
// library diagnostics (which may contain remote data) never leave this package.
func handshakeFailure(err error, hostKeyMismatch bool) error {
	switch {
	case hostKeyMismatch:
		return newConnectionFailure(FailureHostKeyMismatch)
	case strings.Contains(err.Error(), "ssh: unable to authenticate"):
		return newConnectionFailure(FailureAuthenticationFailed)
	default:
		return newConnectionFailure(FailureSSHHandshakeFailed)
	}
}

func workspaceFailure(err error) error {
	var status *sftp.StatusError
	if errors.As(err, &status) {
		switch status.FxCode() {
		case sftp.ErrSSHFxNoSuchFile:
			return newConnectionFailure(FailureWorkspaceMissing)
		case sftp.ErrSSHFxPermissionDenied:
			return newConnectionFailure(FailureWorkspacePermission)
		}
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		return newConnectionFailure(FailureWorkspaceMissing)
	case errors.Is(err, os.ErrPermission):
		return newConnectionFailure(FailureWorkspacePermission)
	default:
		return newConnectionFailure(FailureWorkspaceInvalid)
	}
}
