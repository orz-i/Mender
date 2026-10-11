// Package sandboxplugin provides a standalone SDK for an external Sandbox Backend.
// Implement sandboxv1.SandboxPluginServiceServer and call Serve. It does not
// import Mender backend internals.
package sandboxplugin

import (
	"github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol"
	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
	"github.com/orz-i/Mender/sdk/sandboxplugin/go/transport"
)

// Serve starts the versioned authenticated local gRPC plugin transport.
// Sandbox identity, admission, policy, lease and audit are host-owned.
func Serve(service sandboxv1.SandboxPluginServiceServer) error {
	return transport.Serve(protocol.SupportedVersions(), service)
}
