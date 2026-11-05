//go:build !windows

package managed

import (
	"errors"
	"syscall"
)

func managedMCPAddressInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
