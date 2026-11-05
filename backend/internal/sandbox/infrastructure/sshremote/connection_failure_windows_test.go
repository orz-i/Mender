package sshremote

import (
	"net"
	"os"
	"syscall"
	"testing"
)

func TestWinsockConnectionRefusalUsesSanitizedFailureCode(t *testing.T) {
	t.Parallel()
	err := &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connectex", Err: syscall.Errno(10061)}}
	expectConnectionFailure(t, networkFailure(err), FailureNetworkRefused)
}
