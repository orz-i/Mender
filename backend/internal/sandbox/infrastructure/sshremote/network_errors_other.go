//go:build !windows

package sshremote

import (
	"errors"
	"syscall"
)

func connectionRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }
