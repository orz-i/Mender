# Mender typed plugin packages

Mender is a governed Tool and Resource runtime, not an Agent. A plugin package
is software, an extension implements a typed host contract, and a configured
Provider/Sandbox/Tunnel is an independently governed resource.

The accepted architecture is [ADR-0037](../architecture/adr/0037-multi-kind-plugin-platform.md).
Work is tracked in [Phase 27](../planning/phase-27-multi-kind-plugin-platform.md).

## Extension kinds

| Kind | Ownership | Available external implementation |
| --- | --- | --- |
| `provider.tools` | Canonical Tool discovery/invocation, Provider lifecycle | Supported with Provider Plugin Protocol v1 |
| `sandbox.backend` | Sandbox provisioning/session and backend file/process operations | Typed metadata defined; external driver protocol under development |
| `tunnel.transport` | Managed downstream MCP Profile reachability | Typed metadata defined; external transport protocol under development |

A Sandbox does not automatically create Tools and a Tunnel does not participate
in Catalog discovery. Credentials, Policy, Admission, leases, budgets, Audit,
resource identity and lifecycle orchestration always belong to Mender.

## Package manifest v2

New packages must carry `mender-plugin.json`:

~~~json
{
  "manifestVersion": 2,
  "pluginId": "dev.example.echo",
  "version": "1.0.0",
  "displayName": "Example Echo",
  "description": "Example tool plugin",
  "entrypoints": {"darwin/arm64": "bin/echo"},
  "extensions": [
    {"id": "main", "kind": "provider.tools", "protocolVersion": 1}
  ]
}
~~~

The platform validates namespaced PluginID, SemVer, manifest/file limits,
safe platform-specific relative executable paths, typed extension kind and
protocol version. Multi-extension native packages are not yet supported:
exactly one `extensions` item is required. Configuration Schema,
Credentials and capabilities are validated by the host and the respective
extension contract. Manifest claims never confer authority.

Only `provider.tools` has a supported external runtime implementation at this
milestone. CLI/Desktop Provider-plugin installation rejects wrong-kind packages
before launching the executable. The generic installation store is shared;
additional driver installation/verification becomes available only after those
typed protocols exist.

## Forward-only hard cut for already installed Provider packages

Existing immutable external Provider packages are **not rewritten or rehashed**.
The new `plugin_installations` inventory retains each InstallationID,
PluginID, version, original SHA-256 tree digest, exact package location and
administrative status. Legacy v1 manifest and verified descriptor metadata are
converted once to typed metadata within a transaction. The original artifact
location `mender-provider.json` is recorded as an integrity-verification path;
the runtime verifies the entire original byte tree with its original v1 digest
domain and resolves the executable from normalized, pinned v2 metadata.
It never parses the v1 manifest on normal launch or catalogue reads.

Before an old inventory is migrated, the database store writes a SQLite-consistent
`.mender-phase27-backup-*.db` snapshot adjacent to the configured database.
An incompatible row aborts the migration, leaving the old inventory intact.
New installations use `mender-plugin.json` and the v2 digest domain. Unknown
future schema versions fail closed. There is no permanent dual-read installation
inventory. Do not delete the pre-upgrade backup until verifying normal Runtime
startup and resource references on the upgraded database.

PluginID and installation identity are separate from ProviderID, SandboxID and
TunnelID; side-by-side package versions never automatically rebind resources.

## Native code trust

A native plugin runs as OS-account-trusted code; gRPC, local mTLS, SHA-256 and
process supervision do not prevent that code from accessing the host with its
OS permissions. No native plugin is an OS sandbox without a separately verified
isolation mechanism. Docker socket, host mounts and remote resource lifecycles
must not be delegated to untrusted code based only on a manifest claim.
High-authority Sandbox backends remain built-in until a host-enforced boundary
is designed.

## Test gates

~~~bash
go test -race ./backend/internal/plugin/...
go test ./backend/internal/providerplugin/...
pnpm check
pnpm security:check
~~~
