# ADR-0024: Make Provider plugins the runtime boundary and treat MCP transport as plugin configuration

- Status: Accepted
- Date: 2026-09-27

## Context

Mender's canonical Catalog and Invocation boundaries are already protocol-independent.

Catalog synchronization depends on `ToolSource.DiscoverTools`, Invocation depends on `ToolExecutor.Execute`, and the current MCP `Connection` implements both ports while containing MCP SDK types inside the MCP infrastructure package.

The remaining Provider lifecycle is still specialized around MCP:

- Runtime owns an MCP-specific configuration store.
- Bootstrap exposes `RegisterMCPProvider`, `SyncConfiguredMCPProvider`, and `ActivateConfiguredMCPProvider`.
- Management contracts create separate HTTP and stdio Provider shapes.
- CLI, Wails, and React treat HTTP versus stdio as the Provider kind.
- Credential usage checks know the MCP configuration schema directly.
- active Provider execution is represented indirectly by the executor router plus a Runtime-wide closer list rather than one explicit Provider instance lifecycle.

This specialization blocks the planned Provider extensions. OpenAPI operations, model-provider hosted tools, future CLI/script adapters, and possible third-party Provider packages should not require Catalog, Invocation, Profile, Policy, or Audit to learn new protocols.

It also conflates two concepts:

1. **Provider plugin** — the integration implementation and execution semantics.
2. **Transport/configuration** — one plugin's connection mechanism.

For MCP specifically, stdio and Streamable HTTP both produce the same MCP ClientSession and the same canonical discovery/execution behavior. They are connection variants of one MCP Provider plugin, not separate Provider types.

Mender also needs a stable lifecycle boundary before more Provider implementations are added. The current generic executor router supports replacement, but Runtime can create a fresh MCP connection on repeated activation and retain old connections until Runtime shutdown. Local stdio activation can therefore retain superseded child processes. A plugin runtime must own activation, replacement, and shutdown explicitly.

## Decision

Mender will introduce a **Provider Plugin Runtime** as the infrastructure/application composition boundary between canonical Provider identity and protocol-specific discovery/execution.

### Canonical Provider remains protocol-independent

Catalog Domain Provider remains limited to stable Mender identity and user-owned metadata.

It does not gain:

- transport fields;
- MCP SDK types;
- OpenAPI types;
- model-provider API types;
- executable or endpoint fields;
- secret values;
- arbitrary plugin-specific domain state.

A Tool continues to belong to exactly one canonical Provider and uses `SourceKey` as the stable identity supplied by that Provider plugin.

### Provider plugin identity

Every configured Provider references one immutable plugin identity.

Initial built-in identities are namespaced strings. The first implementation is:

~~~text
io.mender.provider.mcp
~~~

Future built-in candidates include:

~~~text
io.mender.provider.openapi
io.mender.provider.model-tools
~~~

Plugin identity is not inferred from transport, endpoint, executable, display name, or upstream metadata.

Changing a Provider to a different plugin requires registering a different Provider identity. This preserves Tool provenance and Policy/Profile references.

### Provider Plugin

A Provider Plugin describes and opens configured Provider instances.

Conceptually:

~~~go
type Plugin interface {
    Descriptor() Descriptor
    ValidateConfig(context.Context, json.RawMessage) error
    Open(context.Context, OpenRequest) (Instance, error)
}
~~~

Exact Go types may evolve during implementation, but the architectural responsibilities are fixed:

- identify the plugin;
- describe plugin/config capabilities needed by management;
- validate plugin-owned configuration;
- open a runtime instance;
- keep protocol SDK types inside the plugin implementation.

Plugins do not evaluate Mender Policy, bypass Profile scope, write Audit records, or resolve arbitrary credentials outside the capability supplied by Runtime.

### Provider Instance

An opened Provider instance supplies the canonical ports needed by Mender:

~~~text
ToolSource
ToolExecutor
Close
~~~

A plugin may use one object for all three capabilities or compose them internally.

MCP's existing `Connection` is migrated to the first built-in Provider instance with minimal protocol logic changes.

### MCP is one plugin

Local and remote upstream MCP Providers use the same plugin identity.

MCP plugin configuration contains a discriminated transport variant:

~~~text
transport.kind = stdio
transport.kind = streamable-http
~~~

The stdio variant owns executable, argv, working directory, and explicit credential-backed environment placement.

The Streamable HTTP variant owns endpoint, non-secret/static header configuration, and credential-backed header placement.

Transport selection is plugin configuration, not canonical Provider identity.

Management UX may present “Local command” and “Remote endpoint” as setup choices, but those choices must map to one MCP plugin.

### Generic Provider configuration persistence

Runtime configuration persistence becomes Provider-generic.

The canonical persisted record contains at minimum:

~~~text
ProviderID
PluginID
ConfigVersion
ConfigJSON
CreatedAt / UpdatedAt where needed
~~~

Plugin configuration remains adapter/infrastructure data.

Mender performs forward-only migration from the existing `mcp_provider_configs` schema. Migration must preserve:

- ProviderID;
- namespace;
- ToolID and Tool SourceKey;
- Profile ToolBindings;
- Policy Provider/Tool references;
- Credential References;
- Audit history.

Existing HTTP rows migrate to MCP `streamable-http`; existing stdio rows migrate to MCP `stdio`.

### Credential references remain platform-governed metadata

Plugin config must never become a plaintext secret store.

Mender continues to own Credential inventory identity and secret resolution.

Provider plugins may declare credential placement requirements such as:

- HTTP header name/scheme;
- child process environment name;
- future API/provider-specific authentication slots.

Persisted Provider configuration stores only Credential References and non-secret placement metadata.

Credential deletion/usage checks must not require secret resolution.

The implementation may keep credential bindings in a normalized platform-owned table or expose a plugin config inspection contract. The selected representation must let Runtime enumerate referenced Credential References without executing the plugin or parsing untrusted secret values.

### Provider Instance Manager

Runtime introduces an explicit Provider Instance Manager responsible for active instances.

It owns:

~~~text
ProviderID -> active instance
ProviderID -> config revision/fingerprint
ProviderID-scoped activation synchronization
instance close/replacement
Runtime shutdown
~~~

Required semantics:

1. `Ensure(providerID)` reuses a compatible active instance.
2. concurrent Ensure for the same Provider opens at most one replacement instance.
3. activation of Provider A does not serialize unrelated Provider B behind one global connection lock.
4. configuration reload opens/validates the replacement before atomically swapping where practical.
5. a replaced instance is closed promptly.
6. deleting a Provider closes its active instance.
7. Runtime shutdown closes all active instances.
8. executor routing never points at an instance that Runtime has intentionally superseded.

The existing executor router may remain an internal mechanism, but it is no longer the lifecycle owner.

### Generic Runtime orchestration

MCP-specific orchestration names are replaced by Provider-generic behavior.

Conceptually:

~~~text
RegisterProvider
UpdateProvider
SyncConfiguredProvider
EnsureProvider
DeleteProvider
~~~

These use PluginID to resolve a plugin from the registry.

Catalog synchronization still receives a `ToolSource`.

Invocation still resolves a `ToolExecutor` by canonical ProviderID.

Catalog, Invocation, Profile, Policy, Credential Domain, and Audit do not switch on plugin identifiers.

### Plugin Registry

Runtime owns a registry of available Provider plugins.

The first phase uses statically linked built-in plugins.

The registry must reject:

- duplicate PluginID registration;
- unknown PluginID in persisted configuration;
- incompatible/unsupported config versions.

A dynamic third-party plugin loading mechanism is explicitly not part of this phase.

### Management contracts

The long-term management contract is plugin-oriented rather than transport-oriented.

Conceptually:

~~~text
GET  /provider-plugins
POST /providers
PUT  /providers/{providerId}
POST /providers/{providerId}/sync
DELETE /providers/{providerId}
~~~

Provider creation supplies canonical metadata plus PluginID and plugin-owned configuration.

Legacy `/providers/http` and `/providers/stdio` compatibility may be retained temporarily during migration, but new frontend/runtime code must not extend this pattern for additional Provider types.

### Plugin configuration UI

Mender owns the management UI trust boundary.

Built-in plugins may receive dedicated Mender-owned forms.

A future generic plugin form may be driven by bounded declarative metadata such as JSON Schema and safe UI hints.

Provider plugins do not inject arbitrary JavaScript/HTML into Web/Desktop management surfaces.

### Built-in versus external plugins

Provider Plugin Architecture does not imply Go `.so` loading.

Phase 13 only requires built-in statically linked plugins.

If external third-party Provider plugins are introduced later, they require a separate ADR covering:

- process/WASM isolation;
- signed package/manifest identity;
- versioned host protocol;
- permissions/capabilities;
- filesystem/network/process authority;
- update and revocation semantics;
- crash/resource isolation.

Go's native `plugin` package is not adopted as the cross-platform extension mechanism.

## Security invariants

Provider plugin work must preserve all existing execution governance.

Specifically:

1. every Tool invocation still enters the shared Invocation pipeline;
2. plugins never evaluate or bypass global Policy or Profile ToolBinding governance;
3. plugins receive only the scoped CredentialResolver capability supplied by Invocation;
4. secret bytes never enter Catalog metadata, Tool schema/description, Policy inputs, normal Audit payloads, or management responses;
5. Provider configuration contains no resolved secret values;
6. local process execution remains shell-free and receives a minimal ambient environment;
7. remote endpoints remain subject to transport/redirect/credential-origin hardening;
8. plugin discovery data is untrusted metadata and does not become an enforcement guarantee;
9. an unavailable/unknown plugin fails closed rather than silently substituting another implementation.

## Alternatives considered

### Keep HTTP and stdio as first-class Provider types

This matches the current UI and API but makes transport a product/domain classification. Adding OpenAPI or model-hosted tools would expand transport/protocol branching across Runtime and management adapters.

Rejected because local and remote MCP already share one protocol instance and canonical execution semantics.

### Add one new Runtime path for each Provider integration

For example, add `RegisterOpenAPIProvider`, `RegisterOpenAIProvider`, and parallel config stores.

This minimizes initial abstraction work but duplicates lifecycle, Credential, synchronization, and management behavior and eventually forces every adapter to understand every integration.

Rejected because it contradicts the canonical Tool and clean architecture decisions.

### Make plugin-specific configuration fields part of Catalog Provider Domain

This would simplify some queries but turns the canonical Provider into a union of MCP/OpenAPI/model-specific state.

Rejected because it couples Catalog evolution to integration protocols and configuration schemas.

### Load Go plugins dynamically in-process

This would permit third-party binaries with low RPC overhead.

Rejected for this phase because it broadens the trust boundary into the host process, has cross-platform/build compatibility constraints, and provides weak crash/resource isolation for an application that launches untrusted Provider integrations.

### Let plugins execute outside Invocation

This can simplify hosted-tool or HTTP adapters.

Rejected because it would bypass Policy, Profile governance, execution guards, Credential scoping, result guards, lifecycle events, and Audit.

## Consequences

### Positive

- MCP local/remote configuration becomes one coherent Provider plugin.
- OpenAPI and model-tool integrations can be added without changing canonical Tool/Invocation architecture.
- Provider activation/reload/shutdown gains an explicit lifecycle owner.
- repeated Tool calls can reuse Provider instances rather than accumulate stale MCP connections/processes.
- management contracts stop expanding one endpoint/type per transport.
- Credential and Policy governance remain centralized.
- plugin configuration can evolve independently with explicit versions.
- built-in plugin architecture can stabilize before exposing third-party execution.

### Negative / trade-offs

- Provider configuration requires a migration from the MCP-specific schema.
- CLI/OpenAPI/Wails/frontend contracts require coordinated migration.
- plugin configuration becomes partially opaque to generic Runtime code, requiring explicit validation/projection boundaries.
- active instance replacement introduces concurrency/lifecycle behavior that requires focused race tests.
- compatibility endpoints/types may exist temporarily.
- external plugin distribution remains deferred.

## Follow-up

Phase 13 implements this decision in segmented commits:

1. document the Provider Plugin Runtime and MCP unification plan;
2. introduce plugin identity/config/registry primitives;
3. add Provider Instance Manager and regression coverage for reuse/replacement/close;
4. migrate MCP Connection into the built-in MCP plugin without changing canonical Tool behavior;
5. migrate MCP config persistence to generic Provider plugin configuration with compatibility coverage;
6. generalize Runtime registration/sync/activation/deletion;
7. migrate CLI/OpenAPI/Wails/Web/Desktop contracts and UX;
8. add end-to-end MCP stdio/Streamable HTTP parity tests;
9. use an OpenAPI Provider plugin as the next architectural validation, in a subsequent phase or separately reviewed work package.
