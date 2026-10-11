// Package tunnelplugin provides a standalone SDK for an external Tunnel Transport.
// Implement tunnelv1.TunnelTransportServiceServer and call Serve. It does not
// import Mender backend internals.
package tunnelplugin

import (
	"github.com/orz-i/Mender/sdk/tunnelplugin/go/protocol"
	tunnelv1 "github.com/orz-i/Mender/sdk/tunnelplugin/go/protocol/v1"
	"github.com/orz-i/Mender/sdk/tunnelplugin/go/transport"
)

// Serve starts the versioned authenticated local gRPC plugin transport.
// Tunnel identity, managed MCP endpoint, Credential resolution and lifecycle are host-owned.
func Serve(service tunnelv1.TunnelTransportServiceServer) error {
	return transport.Serve(protocol.SupportedVersions(), service)
}
