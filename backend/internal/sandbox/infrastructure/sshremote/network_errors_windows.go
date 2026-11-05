package sshremote

import (
	"errors"
	"syscall"
)

// Winsock's WSAECONNREFUSED differs from the portable syscall sentinel.
const winsockConnectionRefused syscall.Errno = 10061

func connectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, winsockConnectionRefused)
}
