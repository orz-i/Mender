package bootstrap

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
)

// Bounded retries stay inside one Start operation: only the TCP bind is retried,
// not Profile activation, MCP construction, Tunnel startup, or any authorization.
// Each attempt is an atomic OS bind; never probe a port and use it later.
const managedMCPPortAttempts = 32

func listenManagedMCPPort(address string) (net.Listener, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		// Preserve the system's normal invalid-port error instead of rotating.
		return net.Listen("tcp", address)
	}
	if port == 0 {
		// The existing :0 contract asks the OS to allocate a free port.
		return net.Listen("tcp", address)
	}

	var occupied error
	for attempt := 0; attempt < managedMCPPortAttempts && port+attempt <= 65535; attempt++ {
		candidate := net.JoinHostPort(host, strconv.Itoa(port+attempt))
		listener, bindErr := net.Listen("tcp", candidate)
		if bindErr == nil {
			return listener, nil
		}
		if !errors.Is(bindErr, syscall.EADDRINUSE) {
			return nil, fmt.Errorf("listen on %s: %w", candidate, bindErr)
		}
		occupied = bindErr
	}
	return nil, fmt.Errorf(
		"no available MCP listener port within %d attempts from %s: %w",
		managedMCPPortAttempts, address, occupied,
	)
}
