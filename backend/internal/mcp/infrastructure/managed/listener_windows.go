package managed

import (
	"errors"
	"syscall"
)

func managedMCPAddressInUse(err error) bool {
	// WinSock WSAEADDRINUSE is 10048. syscall.EADDRINUSE is an invented
	// compatibility value on Windows, not the error returned by net.Listen.
	const wsaAddressInUse = syscall.Errno(10048)
	return errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, wsaAddressInUse)
}
