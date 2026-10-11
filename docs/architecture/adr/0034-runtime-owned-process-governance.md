# ADR-0034: Govern every Runtime-owned native process through one host-owned process boundary

- Status: Accepted
- Date: 2026-10-06

## Context

ADR-0033 introduced explicit `host` / `restricted_host` process isolation and routed `workspace.shell` plus external Provider plugin package processes through a Mender-owned launcher. The later governance audit found one remaining Runtime-owned native child outside that contract: OpenAI Secure MCP Tunnel's `tunnel-client`.

Phase 21 currently persists an administrator-provided Tunnel executable and starts it directly with `exec.Command`. The process inherits the complete host environment plus the resolved Tunnel runtime API key. That creates a process-authority exception beside the otherwise centralized Runtime process boundary.

The exception matters even though the management plane is loopback-only and trusted to the Runtime owner's OS account. A Tunnel record can currently select arbitrary native code, that native code receives secret material, and Phase 23 isolation settings do not constrain it. A future process class could repeat the same mistake if process governance remains feature-specific.

Mender needs one rule for local children: Runtime-owned native execution is host infrastructure and must be launched through a host-owned process contract. Tool arguments, Provider protocol messages, Tunnel configuration, and other persisted resource state must not supply sandbox policy or select arbitrary executable authority.

This ADR supersedes ADR-0031 only where ADR-0031 made the Tunnel executable a persisted configurable field and launched `tunnel-client` directly. ADR-0031's Tunnel bounded context, Credential Reference, Profile binding, loopback-only MCP endpoint, and lifecycle coupling remain active. This ADR extends ADR-0033's process-isolation boundary to the Tunnel process class.

## Decision

### One Runtime process boundary

Every Runtime-owned native child belongs to an explicit process class and must enter the same host-owned launcher boundary:

~~~text
Runtime-owned native process
        |
        +--> workspace.shell
        |
        +--> external Provider plugin
        |
        +--> Tunnel client
        |
        v
host-owned ProcessLauncher
        |
        +--> host
        |
        +--> restricted_host
~~~

The process class determines host-owned policy inputs such as isolation setting, permitted filesystem root, root writability, executable resolution, working directory, and bounded environment. Untrusted Tool/Provider/Tunnel data may supply only operation-specific arguments defined by that class.

### Runtime isolation gains TunnelClient

`RuntimeIsolation` becomes:

~~~text
WorkspaceShell   = host | restricted_host
ExternalProvider = host | restricted_host
TunnelClient     = host | restricted_host
~~~

Fresh defaults:

- WorkspaceShell: `restricted_host` when the backend is available, otherwise `host`;
- ExternalProvider: `host`;
- TunnelClient: `restricted_host` when the backend is available, otherwise `host`.

The Tunnel default follows WorkspaceShell rather than ExternalProvider. `tunnel-client` is a Mender-selected infrastructure executable with a narrow purpose and does not require arbitrary host filesystem integration as part of its Provider contract. Network access remains available in both Phase 23 restricted-host backends.

Persisted `restricted_host` never silently degrades to `host` if backend support later disappears. Updating Runtime isolation to `restricted_host` also fails closed when the backend is unavailable.

### Tunnel executable selection is host-owned

The Tunnel domain no longer stores an executable path/name.

OpenAI Tunnel startup resolves the fixed logical executable `tunnel-client` through a host-owned resolver at the execution boundary. Resolution must produce an absolute executable path before the process-isolation launcher is called.

Tunnel API/SDK/Wails/Web/Desktop contracts remove the `executable` field in the same hard cut. Existing persisted Tunnel rows are migrated forward by dropping the old executable value; there is no runtime fallback parser, compatibility field, or per-Tunnel override.

Automatic download, installation, package management, signing, or update of `tunnel-client` remains out of scope. Operators install the executable on the Runtime host through normal system administration.

### Tunnel child environment is bounded

Tunnel startup must not clone `os.Environ()`.

The child receives only the host-owned non-secret process environment required by the execution contract plus the resolved `CONTROL_PLANE_API_KEY`. The secret is resolved immediately before launch, kept out of persistent Tunnel state and management projections, and cleared from the parent-owned byte buffer after the runner returns.

The Tunnel process cannot request additional host environment variables through Tunnel configuration.

### Tunnel restricted-host root

The host canonicalizes the resolved `tunnel-client` executable and uses its containing directory as the Tunnel process permitted root and working directory. The root is read-only.

The existing platform backend still permits normal system/runtime reads and network access. The Tunnel process does not receive arbitrary writable host roots. Failure to resolve/canonicalize the executable or construct the selected isolation environment prevents launch.

### Lifecycle ownership is unchanged

The managed MCP server remains loopback-only. Starting a Profile server still:

1. activates required Providers through `provider.activate` governance;
2. starts the loopback MCP server;
3. resolves the bound Tunnel runtime Credential;
4. launches the governed Tunnel child;
5. rolls the server back if Tunnel launch fails.

Stop, restart, server exit, and Runtime close still stop the Tunnel child.

This phase does not add a new Policy action for Tunnel startup. Tunnel startup remains part of the local Runtime-owner control operation that starts a managed Profile server. Remote end-user Principal propagation remains separate follow-up work.

## Persistence

Runtime governance schema advances forward-only to add `tunnel_client` to the singleton isolation settings row.

The Tunnel schema advances forward-only by rebuilding the Tunnel table without the `executable` column while preserving:

- TunnelID;
- display name;
- provider;
- external Tunnel ID;
- Runtime Credential Reference.

No old executable value is copied into another field.

## Management contracts

Management HTTP, OpenAPI/generated TypeScript SDK, Wails bindings, Web, and Desktop move directly to:

~~~text
RuntimeIsolation {
  workspaceShell
  externalProvider
  tunnelClient
}

Tunnel {
  tunnelId
  displayName
  provider
  externalTunnelId
  runtimeCredentialRef
}
~~~

The Runtime page exposes Tunnel client isolation beside Workspace shell and External Providers.

Tunnel creation no longer exposes an executable input.

## Security properties

Phase 24 establishes:

1. no production Tunnel path constructs its own unmanaged native child;
2. Tunnel configuration cannot select arbitrary executable authority;
3. Tunnel child filesystem authority is explicit and follows Runtime isolation settings;
4. Tunnel child environment is bounded instead of inheriting the complete Runtime environment;
5. the Tunnel API key remains execution-only secret material;
6. `restricted_host` remains explicit and fail closed;
7. loopback MCP request provenance controls remain unchanged.

It does not claim VM/container isolation, CPU/memory accounting, or network egress filtering.

## Hard cut

Phase 24 removes rather than deprecates:

- `Tunnel.Executable`;
- `CreateOpenAITunnelRequest.Executable`;
- management/OpenAPI/SDK/Wails/UI `executable` Tunnel fields;
- direct `exec.Command` construction inside Tunnel infrastructure;
- full host environment inheritance for `tunnel-client`.

One-way SQLite migration preserves Tunnel identity/configuration except the removed executable override.

## Alternatives considered

### Reuse ExternalProvider isolation for Tunnel

Rejected. External Provider packages and Tunnel infrastructure have different trust/default requirements. Conflating them would make administrators weaken or strengthen unrelated process classes together.

### Keep executable override as an advanced field

Rejected. The field is itself the audited native-code escape hatch. Marking it advanced does not change its authority.

### Keep direct exec but sanitize the environment

Rejected. Environment minimization alone does not close the process-isolation exception and would leave future process classes without one enforceable launch rule.

### Always force restricted_host

Rejected. The backend is not available on every supported Runtime. Mender keeps `host` explicit rather than making Tunnel unavailable on unsupported platforms.

### Add tunnel.start Policy action now

Deferred. The immediate gap is native process authority. Expanding the governance action model would mix local control-plane authorization with process-environment hardening and remote Principal work.

## Consequences

Positive:

- Runtime process governance becomes structurally complete for current native child classes;
- Tunnel secrets no longer reach an arbitrary per-resource executable or unrelated host environment entries;
- process isolation configuration explains all current Runtime-owned local children;
- future native process features have a clear architectural rule to follow.

Costs:

- custom per-Tunnel executable overrides break intentionally;
- Runtime governance and Tunnel SQLite schemas advance;
- HTTP/OpenAPI/SDK/Wails/Web/Desktop contracts change together;
- `tunnel-client` must be discoverable by the Runtime host at launch time;
- restricted Tunnel execution may expose platform/backend incompatibilities that host mode previously hid.

## macOS Seatbelt TLS trust correction (October 2026)

A live Wails development Runtime with `TunnelClient=restricted_host` reproduced an outbound TLS failure at `https://api.openai.com`: `x509: OSStatus -26276`. The tunnel child remained `running` with `/healthz` and `/readyz` at 200, but its control-plane health stayed `degraded/backoff` (`network_error`) and had zero successful polls. Mender's loopback MCP `initialize` succeeded independently. Native Go HTTPS probes with the same verified destination passed outside Seatbelt, failed with the original `(deny default)` profile, and passed with **only** `(allow mach-lookup (global-name "com.apple.trustd.agent"))` added. Granting unrelated `com.apple.trustd` or `com.apple.securityd` alone did not resolve it.

The macOS `restricted_host` launcher must allow lookup of exactly `com.apple.trustd.agent` for operating-system certificate trust evaluation. It must **not** use unrestricted `(allow mach-lookup)`, disable TLS certificate verification, give a child host-mode fallback, broaden HOME/keychain file permissions, or pass extra environment secrets. The shared Seatbelt profile already permits outbound networking; the targeted trust-agent lookup restores the ability to verify HTTPS peers while retaining the existing filesystem and process boundaries. Tests should assert the precise exception and preserve read-only/host-home isolation. Deployment requires rebuilding/restarting the desktop Runtime and restarting the managed MCP Server/Tunnel child.

## Follow-up

Separate phases should address the remaining audit findings: canonical Outcome byte budgeting/Unknown Governance Facts, then verified remote Principal propagation and interactive approval.
