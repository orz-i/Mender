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
| `tunnel.transport` | Managed downstream MCP Profile reachability | Trusted OpenAI built-in; external native Tunnel Transport Protocol v1 and CLI/Runtime support |

A Sandbox does not automatically create Tools and a Tunnel does not participate
in Catalog discovery. Credentials, Policy, Admission, leases, budgets, Audit,
resource identity and lifecycle orchestration always belong to Mender.

## Reviewed integration offerings (Phase 34 C)

Integration offerings are **configuration recipes**, not native Plugin
Installations or live Providers. The host exposes an inert reviewed catalogue
through `GET /api/v1/integrations` and the matching SDK/Wails APIs. In the
Marketplace, offerings have **Available** status (not connected/healthy), a
separate Remote MCP Integration type, and a **Connect** action.

The initial AnySearch offering pins the built-in MCP driver,
`https://api.anysearch.com/mcp`, Streamable HTTP and a reviewed
`X-Anysearch-Client` header. The focused connection flow asks for a name,
an optional existing Credential Reference (or anonymous use), and hides the
default namespace under Advanced. The endpoint and protocol are read-only in
this preset. Custom MCP endpoints remain available through the Advanced
Provider workflow. A connection uses canonical Tool sync and existing
Credential/Policy/Approval/Audit governance; it does **not** grant a Profile.

Ordinary Marketplace **Discover** shows MCP, OpenAPI and Model Tools as
first-class, directly configurable Provider offerings, alongside healthy
Sandbox/Agent/Tunnel drivers and reviewed integrations. An **Installed plugins**
view manages native installation status, versions and removal separately. The
Workspace, Agent Delegation and SSH Jobs Provider adapters are internal
resource-owned Tool Suites, not independently selectable Market/Provider items.
The complete technical Registry remains available for existing-resource
resolution; no catalogue read contacts AnySearch or requests its API key.

A signed-off offering is not evidence that the service is reachable or that
an external account/API key works; operator-side live verification is separate.
For ownership and trust boundaries, see
[ADR-0039](../architecture/adr/0039-offerings-resource-tool-suites.md)
and [Phase 34 C](../planning/phase-34-c-integration-offerings.md).

## Governed Tool Suites and resource ownership (Phase 34 D)

A **Tool Suite** defines a reviewed set of operations and their required
resource capabilities. It is **not** a native Plugin Package, an Agent Skill,
or a Profile authorization grant. `GET /api/v1/tool-suites` and matching
CLI/SDK/Wails reads return inert metadata. Suite binding and repair belong
exclusively to the owning Agent/Sandbox details, not Marketplace cards.

| Suite | Resource | Required capabilities | Availability |
| --- | --- | --- | --- |
| `agent.delegation` | Agent | `agent.task` | Built-in; managed from each Agent's Tools view |
| `sandbox.workspace` | Sandbox | `file.read`, `file.search`, `file.edit`, `process.shell` | Built-in, only for an exact ready backend supporting **all four** operations |
| `sandbox.browser` | Sandbox | `browser.session` | Unavailable until a verified managed Browser Session driver is implemented |

An Agent or Sandbox can have resources configured without any Profile exposure.
For Sandboxes, open the resource **Tools** view and select **Prepare Workspace
Tools** (or explicitly adopt a unique existing Provider). Mender preserves
existing ProviderID/ToolIDs/SourceKeys and Profile aliases. Conflicting existing
Providers are reported, never auto-merged or renamed. New managed suite
bindings are unique at the SQLite boundary and cannot be removed/reconfigured
through generic Provider CRUD. Only an owner-authorized action may retire an
unreferenced suite.

The Sandbox APIs are:

- `GET /api/v1/sandboxes/{sandboxId}/tools` — inert binding/availability snapshot
- `POST /api/v1/sandboxes/{sandboxId}/tools` — explicitly ensure and sync governed Workspace tools
- `DELETE /api/v1/sandboxes/{sandboxId}/tools` — explicitly retire an unused suite
- `GET /api/v1/tool-suites` — reviewed definitions, including unavailable Browser

CLI equivalents:

~~~sh
mender sandbox suites
mender sandbox tools <sandbox-id>
mender sandbox tools <sandbox-id> --prepare
mender sandbox tools <sandbox-id> --remove --confirm
~~~

Provider Activation, ToolBinding, Policy, Sandbox grants/admission/lease,
Runtime Budget, result guard and Audit remain mandatory during real execution.
Preparing a suite does not start a Sandbox or grant a Profile. A Browser-only
environment **does not** receive file or Shell tools via host fallback.
`BrowserSession` is a separate consumer-side port with **no registered
implementation** and no runnable Browser Tools at present; third-party browser
Remote MCP services can instead be connected as normal reviewed/custom MCP
integrations, with their own transport/security lifecycle.

See [ADR-0039](../architecture/adr/0039-offerings-resource-tool-suites.md)
and [Phase 34 D](../planning/phase-34-d-tool-suite-governance.md).

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

`provider.tools`, `sandbox.backend` and `tunnel.transport` each have
independently versioned, installable native runtimes. The generic Plugin Core
verifies all three package kinds and supervises their native processes, while
Provider, Sandbox and Tunnel keep separate management/execution contracts.
The CLI dispatches installation by `extensions[0].kind`. Phase 27 E now
provides one host-owned Plugin Catalogue, type-aware Web/Desktop creation
routes, generated OpenAPI/SDK/Wails bindings and a trusted desktop-local
package chooser. Web does **not** expose an arbitrary native package install
endpoint.

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

**Product scope:** the governed native install/create/operate path is supported
by CLI/Runtime. Phase 27 E adds unified Web/Desktop plugin discovery and
exact-installation Sandbox creation/configuration/review. This does not claim
cloud provider availability, unrestricted shell execution or an OS-isolated
native plugin.

## Tunnel Transport extensions (SQLite v3)

`io.mender.tunnel.openai/main` is the trusted OpenAI transport. Existing
Tunnel IDs, provider-facing ExternalTunnelIDs, Profile tunnel links and
Credential references are preserved by a one-time **v2→v3 hard cut**.
A WAL-consistent `.mender-tunnel-pre-v3-*.db` snapshot is created before
upgrading the Tunnel database. The new Tunnel `Binding` is immutable and
retains an empty InstallationID only for the shipped OpenAI built-in.

External Tunnels require the exact verified, enabled `tunnel.transport/main`
InstallationID. Mender checks package status, full SHA-256 tree integrity and
typed descriptor before resolving the runtime Credential Reference. Connection
parameters always contain a host-selected `http://127.0.0.1:<port>/mcp`
(or equivalent literal IPv6 loopback) endpoint. Neither a Profile nor a
native plugin may choose a public MCP listener.

Independent `contracts/tunnelplugin/v1/` and `sdk/tunnelplugin/go/`
provide GetDescriptor, Validate/InspectConfig, Connect, Status and Disconnect.
A native extension can report `starting`, `ready`, `degraded` or `failed`;
these are **transport-reported** states, not proof of OpenAI/control-plane
registration or public Internet accessibility. OpenAI `tunnel-client`
process startup alone is `starting`, never an assumed `ready`.

A managed Profile MCP HTTP server must exist before Tunnel Connect. On
connection failure the listener is shut down. On stop/restart the Tunnel
session is boundedly disconnected; unexpected process termination is observed
as `exited`. Resource deletion is blocked while any Profile still references
the Tunnel or a managed MCP server is actively exposing it. The exact
installation cannot be disabled/removed as long as any Tunnel inventory row
references it, even after Profile unbind. SQLite also prevents racing package
disable/create/delete from silently stranding a Tunnel.

**Headless local fixture example:**

~~~sh
cd examples/tunnel-plugin-example
mkdir -p bin
GOWORK=off go build -o bin/mender-tunnel-example ./cmd/mender-tunnel-example

# Use the same Mender CLI database throughout:
mender plugin install /absolute/path/to/examples/tunnel-plugin-example
mender plugin list
mender tunnel add external --name example --installation pinst_<exact-id> --external-id test --credential-ref credential://example/tunnel
mender tunnel list
~~~

The Credential Reference must exist in Mender's inventory. The example uses
a local loopback MCP reachability probe only and does **not** connect to a
cloud relay. Attach the TunnelID to a Profile through the existing managed
Profile Tunnel binding flow, then start that Profile's managed MCP Server.
Once the Profile is unbound and its MCP server stopped, `mender tunnel delete`
can remove the Tunnel resource. CLI/Runtime and the Phase 27 E Web/Desktop
focused Tunnel plugin selection/creation flow are implemented. This is not
verification of publicly reachable OpenAI or third-party relay connectivity.

## Unified Plugin Catalogue and Marketplace (Phase 27 E)

The host-owned `GET /api/v1/plugins` catalogue is **one source of truth** for
three typed extension kinds. Rows represent built-ins or verified exact native
installations and contain safe descriptive identity, source, status,
capabilities, optional config JSON Schema, InstalledAt/digest and usage count.
No native plugin is started, no Credential is resolved, and no remote Sandbox
or Tunnel is provisioned during catalogue reads.

Generic package status/delete operations use:
`POST /api/v1/plugin-installations/{installationId}/enable`,
`POST /api/v1/plugin-installations/{installationId}/disable`, and
`DELETE /api/v1/plugin-installations/{installationId}`. A referenced package
cannot be disabled or deleted; the underlying SQLite guards continue to
enforce that constraint for Sandbox/Tunnel even across Runtime processes.

Marketplace cards show a compact preview and stable cursor-based pagination.
Type/search/source/status controls and install/refresh share the toolbar. The
card detail separates **overview**, **capabilities**, and **technical details**.
The Add action pins one software identity and opens only the matching
owner-domain resource flow:

- **Provider:** existing generic Provider create dialog with exact PluginID
  and InstallationID; Provider Tools still resolve through their Provider
  descriptor, not a Sandbox/Tunnel RPC.
- **Sandbox:** built-in Local/Docker/Daytona focused guided create or a
  separate external `/sandboxes/new/external` config/review page. Schema
  validation occurs in both UI and the host; a configuration cannot retarget
  to a different installed driver. Unknown states require manual
  `/sandboxes/{sandboxId}/reconcile`.
- **Tunnel:** choose OpenAI built-in or a verified external transport, then a
  focused Credential Reference selector. Credentials are resolved by Mender
  only when starting a managed MCP server; the creation form never asks for
  the raw key or arbitrary public listen address.

The local desktop app may install native package folders only after the user
selects a directory through the OS file chooser. HTTP/Web has no native package
upload/path-install endpoint. In-app confirmation dialogs replace
`window.confirm` for plugin and Tunnel deletion, including WKWebView.

**Sample HTTP usage (already-installed extensions):**

~~~text
GET    /api/v1/plugins
POST   /api/v1/sandboxes/external
POST   /api/v1/tunnels/external
POST   /api/v1/sandboxes/{sandboxId}/reconcile
~~~

Both external creation endpoints select a fixed InstallationID validated by
their own bounded contexts. The UI and metadata do not prove provider cloud
reachability, charge limits or native executable confinement.

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
go test -race ./backend/internal/bootstrap ./backend/internal/management/httpapi ./backend/internal/tunnel/...
pnpm check
pnpm security:check
~~~
