//go:build !windows

package bootstrap

import (
	"errors"
	"syscall"
)

func managedMCPAddressInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
