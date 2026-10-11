# Mender Sandbox Plugin Example

A deterministic external Sandbox backend implementing the public
`github.com/orz-i/Mender/sdk/sandboxplugin/go` v1 protocol. It does not
import Mender backend internals, call Docker, connect to cloud services, or
execute host shell commands.

Build from this directory:

```sh
mkdir -p bin
GOWORK=off go build -o bin/mender-sandbox-example ./cmd/mender-sandbox-example
```

The package's `mender-plugin.json` advertises a `sandbox.backend` extension
with only the `file.read` capability. The fake Provision can return an
uncertain result (`{"uncertainProvision":true}`) while retaining a
reconcilable remote-ready state. `README.txt` returns an illustrative JSON
response. The service is an acceptance fixture, **not a security sandbox**:
native plugin binaries run under the local account's OS authority.

Do not assume a hosted Sandbox or automatic network isolation. The Mender
runtime must admit lifecycle operations, provision/inspect and acquire sessions
under its own Policy, Principal, immutable binding, durable lease and audit.
