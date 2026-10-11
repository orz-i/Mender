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
| `sandbox.backend` | Sandbox provisioning/session and backend file/process operations | Trusted Local Workspace/Docker/opt-in Daytona; installable native external backend through Sandbox Plugin Protocol v1 and governed CLI |
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

Both `provider.tools` and `sandbox.backend` have independently versioned,
installable native runtimes. The generic Plugin Core verifies both packages
and supervises their native processes, but Provider/Sandbox management and
execution protocols remain separate. The CLI dispatches installation by
`extensions[0].kind`; Tunnel is still a proposed extension contract. Web and
Desktop plugin-marketplace integration for Sandbox/Tunnel remains Phase 27 E.

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

## Sandbox exact extension binding (SQLite v9)

Each persisted Sandbox now contains an immutable `BackendBinding`:

~~~text
PluginID         io.mender.sandbox.docker
ExtensionID      main
InstallationID   (empty for trusted built-in)
~~~

The historical `backend=docker` classification is still used to validate
Docker-specific isolation/templates. It is **not** a plugin resolver or
executable authority. The Execution Router checks the stored PluginID,
ExtensionID, optional exact InstallationID, backend kind and verified capability
before opening a backend. `sandbox.acquire` admission/Owner/Principal,
run generation and durable lease fencing remain in Mender.

SQLite v8 upgrades to v9 in one forward-only transaction, preserving SandboxIDs,
run generations/states/external IDs, Docker/Daytona templates, Profile grants,
audit and leases. A `.mender-sandbox-v8-backup-*.db` SQLite snapshot is
created beside the runtime database before the hard cut. Do not delete it until
checking the upgraded runtime and resource inventory. If an unknown backend
kind cannot be bound, startup fails without modifying the v8 inventory.
A pre-v9 client cannot write Sandbox records after upgrade: the v9 schema
rejects missing extension pins and overwriting existing pins.

Trusted built-in Sandbox binding continues to cover Local Workspace, Docker
Engine (only when explicitly configured), and Daytona (only when operator
experimental opt-in is enabled). The new `external` backend classification
requires an exact installed and verified `sandbox.backend/main` package, with
a non-empty immutable InstallationID. Its capabilities are checked in the
host-governed Router; unknown or missing packages never fall back to the host.
A descriptor is never evidence of cloud reachability or OS-level confinement.

## Native external Sandbox extensions (SQLite v10)

Phase 27 C3 adds a separate, generated `contracts/sandboxplugin/v1/`
gRPC contract and standalone `sdk/sandboxplugin/go/` SDK. The checked-in
`examples/sandbox-plugin-example/` is a deterministic local-only fixture; it
does not host a real cloud Sandbox or touch the host filesystem.

New external resources use the existing `SandboxID`, `BackendBinding` and
Sandbox Router. `sandbox_external_configs` stores bounded immutable
non-secret JSON and config version; `sandbox_external_operations` stores
the operation's stable idempotency key and observed completion/uncertainty.
The host validates the extension's JSON Schema and rejects recognized
credential-bearing fields/values (including escaped or nested keys). This
filter cannot identify arbitrary secrets; a later credential-backed protocol
must use a narrowly scoped, host-mediated credential handoff.

A state transition is governed before remote execution, fenced by the persisted
Run generation and durable lease, and audited separately from Policy admission.
Timed-out, crashed, or ambiguous remote operations stay visible as
`unknown`/`orphaned` until an explicitly authorized `sandbox.reconcile`.
No automatic operation replay, local shell fallback, retargeting, or silent
remote resource deletion is permitted.

**Install and exercise the local-only example:**

~~~sh
cd examples/sandbox-plugin-example
mkdir -p bin
GOWORK=off go build -o bin/mender-sandbox-example ./cmd/mender-sandbox-example

# From the repository root, using the same Mender CLI database:
mender plugin install /absolute/path/to/examples/sandbox-plugin-example
mender plugin list
mender sandbox add external --name example --installation pinst_<exact-id> --config-json '{}'
mender sandbox list
mender sandbox reconcile sbx_<id>             # only when in unknown/orphaned state
mender sandbox stop sbx_<id>
mender sandbox start sbx_<id>
mender sandbox destroy sbx_<id> --confirm
~~~

Example package `file.read` returns a sample `README.txt` response
conforming to the canonical Workspace Tool output schema. Other capabilities
are not advertised and cannot be silently routed to Host. The fixture keeps
remote state in memory and may lose it when restarted; this intentionally
demonstrates fail-closed recovery, not production persistence.

**Forward-only upgrades:** v8→v9 still backs up to
`.mender-sandbox-v8-backup-*.db`; v9→v10 creates a WAL-consistent
`.mender-sandbox-v9-backup-*.db` before adding external resource config,
operation journals and stricter extension bindings. Existing SandboxID,
Run generation, durable leases, Profile grants, Docker/Daytona templates and
audit are preserved. Retain migration backups until the upgraded Runtime and
its resource inventory have been reviewed.

**Package lifecycle safety:** a plugin installation referenced by any external
Sandbox, including an already destroyed historical resource, cannot be disabled
or removed. Atomic SQLite pin and installation status/delete guards also
reject a creation racing with administrative disable; management preflight
alone is not trusted for that invariant.

**Product scope:** the governed native install/create/operate path is currently
available from the CLI and backend Runtime. Generic Web/Desktop plugin-market
and resource creation flows are Phase 27 E work, not part of C3.

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
