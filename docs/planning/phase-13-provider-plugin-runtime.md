# Phase 13 Plan — Provider Plugin Runtime and MCP Unification

Status: Complete

## Objective

Turn the existing protocol-independent Tool ports into a first-class Provider Plugin Runtime.

Phase 13 will:

- represent upstream MCP as one built-in Provider plugin;
- make stdio and Streamable HTTP plugin transport variants rather than Provider types;
- replace MCP-specific Runtime/provider configuration orchestration with generic Provider plugin orchestration;
- introduce explicit Provider instance lifecycle ownership;
- preserve every canonical ProviderID/ToolID/Profile/Policy/Credential relationship during migration;
- prepare a stable extension boundary for future OpenAPI and model-hosted Tool Providers without implementing those integrations in this phase.

The primary architecture decision is ADR-0024.

## Why now

Mender has completed the governance foundation through Phase 12.

The current canonical boundaries are already suitable for multiple Provider implementations:

~~~text
Catalog ToolSource
  DiscoverTools

Invocation ToolExecutor
  Execute
~~~

MCP `Connection` implements both ports and keeps MCP SDK types in infrastructure.

The remaining specialization is composition/configuration rather than Domain behavior. Extending the current pattern directly would create one Runtime/config/API/UI branch per new Provider integration.

Phase 13 removes that specialization before OpenAPI and LLM-hosted Tool work begins.

## Baseline

Current upstream MCP flow:

~~~text
Catalog Provider
    |
mcp_provider_configs
    |
Transport switch
    +-- HTTP -> ConnectHTTP
    +-- stdio -> ConnectCommand
                  |
              Connection
             /          \
    DiscoverTools      Execute
         |                |
       Catalog         Invocation
~~~

Current management contracts expose transport as Provider kind:

~~~text
POST /providers/http
POST /providers/stdio
~~~

Runtime owns MCP-specific methods such as:

~~~text
RegisterMCPProvider
ConnectConfiguredMCPProvider
SyncConfiguredMCPProvider
ActivateConfiguredMCPProvider
UpdateMCPProvider
DeleteMCPProvider
~~~

The executor router is generic, but active instance lifetime is not. Repeated direct Tool calls can activate replacement MCP connections while old connections remain in the Runtime closer list until shutdown.

## Target architecture

~~~text
Canonical Provider
      |
      | ProviderID
      v
Provider Configuration
  PluginID
  ConfigVersion
  Config
      |
      v
Plugin Registry
      |
      v
Provider Instance Manager
      |
      +-----------------------------+
      |                             |
      v                             v
 ToolSource                    ToolExecutor
      |                             |
      v                             v
   Catalog                      Invocation

Built-in plugins:

io.mender.provider.mcp
        |
        +-- stdio
        +-- streamable-http
~~~

Future plugins can implement the same boundary without changes to Catalog/Invocation governance.

## Ubiquitous language

### Provider

The existing canonical Catalog entity.

Provider owns stable Mender identity, namespace, display metadata, and timestamps.

It does not own protocol/transport configuration.

### PluginID

Stable identifier for the Provider integration implementation.

Initial value:

~~~text
io.mender.provider.mcp
~~~

PluginID is immutable for one Provider identity.

### ProviderConfig

Persisted infrastructure configuration binding one ProviderID to one PluginID and plugin-owned config version/data.

ProviderConfig never contains resolved secret bytes.

### Plugin

A statically registered implementation capable of validating configuration and opening Provider instances.

### Instance

One active runtime instance for a Provider configuration.

It supplies Tool discovery, Tool execution, and lifecycle close semantics.

### Instance Manager

Runtime component that owns active Provider instances, Provider-scoped activation synchronization, replacement, and shutdown.

### Transport

A plugin-specific connection mechanism.

For the MCP plugin:

~~~text
stdio
streamable-http
~~~

Transport is not a canonical Provider type.

## Scope

Phase 13 includes:

- ADR and planning documentation;
- Provider plugin identifiers and descriptors;
- built-in plugin registry;
- generic Provider configuration model/store;
- forward-only migration from existing MCP provider configuration;
- Provider Instance Manager;
- MCP plugin config with stdio/Streamable HTTP variants;
- migration of existing MCP connection/executor/discovery code behind the plugin boundary;
- generic Runtime Provider lifecycle operations;
- Credential reference enumeration/usage checks through the generic configuration boundary;
- CLI contract migration;
- management HTTP/OpenAPI migration;
- Wails binding migration;
- shared Web/Desktop Provider workflow migration;
- regression tests for existing MCP behavior;
- race/lifecycle tests for instance activation/replacement;
- documentation and upgrade notes.

## Non-goals

Phase 13 does not:

- implement OpenAPI-to-Tool Provider behavior;
- implement OpenAI/Anthropic/Gemini hosted Tool bridges;
- expose third-party dynamically loaded plugins;
- load Go `.so` files;
- define a marketplace or registry service;
- allow plugin-supplied JavaScript/HTML in management UI;
- redesign canonical Tool schemas or Invocation outcomes;
- change global Policy semantics;
- change Profile ToolBinding semantics;
- add account switching/session pools inside one Provider;
- make downstream MCP serving a Provider plugin;
- add remote Mender management authentication.

## Architectural invariants

The implementation must preserve:

1. Catalog and Invocation packages do not import Provider plugin implementations.
2. MCP SDK types remain inside MCP infrastructure/plugin packages.
3. Tool synchronization still uses canonical `DiscoveredTool`.
4. Tool execution still enters shared `InvokeTool`.
5. Policy and Profile governance remain plugin-agnostic.
6. Credential resolution remains capability-based and scoped by Invocation.
7. Audit does not gain raw plugin config, arguments, secrets, or protocol frames.
8. ProviderID and ToolID identity rules do not change.
9. Provider Plugin metadata is not trusted as an enforcement guarantee.
10. an unknown/unavailable PluginID fails closed.

## Provider plugin contract

Initial conceptual contract:

~~~go
type PluginID string

type Descriptor struct {
    ID            PluginID
    Version       string
    DisplayName   string
    Description   string
    ConfigVersion int
}

type Plugin interface {
    Descriptor() Descriptor
    ValidateConfig(context.Context, json.RawMessage) error
    Open(context.Context, OpenRequest) (Instance, error)
}

type Instance interface {
    catalogapplication.ToolSource
    invocationapplication.ToolExecutor
    io.Closer
}
~~~

The concrete implementation may separate discovery/executor accessors if doing so improves ownership or testability.

The contract must not expose MCP SDK types.

## Plugin registry

The registry is an infrastructure/composition component.

Required behavior:

- register built-in plugins during Runtime construction;
- reject duplicate PluginID;
- resolve PluginID deterministically;
- return a stable unknown-plugin error;
- optionally expose safe descriptors to management;
- never perform network/process work during descriptor listing.

Initial registry contains only the MCP plugin.

Tests cover duplicate registration and unknown lookup.

## MCP plugin

### Identity

~~~text
io.mender.provider.mcp
~~~

### Transport config

Conceptual JSON shape:

~~~json
{
  "transport": {
    "kind": "streamable-http",
    "endpoint": "https://example.test/mcp",
    "headers": {}
  }
}
~~~

or:

~~~json
{
  "transport": {
    "kind": "stdio",
    "executable": "npx",
    "args": ["-y", "@example/mcp-server"],
    "directory": ""
  }
}
~~~

Credential placement remains structured rather than embedded plaintext.

Exact persisted JSON field names should be stable and documented before generated management contracts expose them.

### Connection reuse

The existing MCP `Connection` remains the protocol anti-corruption object:

~~~text
MCP ClientSession
    |
Connection
    +-- DiscoverTools
    +-- Execute
    +-- Close
~~~

The MCP plugin opens the correct transport and returns this instance.

No Catalog/Invocation code switches on MCP transport.

## Provider Instance Manager

### Requirements

The manager owns active instance lifetime.

At minimum:

~~~text
Ensure(ctx, providerID)
Reload(ctx, providerID)
Close(providerID)
CloseAll()
~~~

Exact API may be narrower if Runtime orchestration can express these semantics cleanly.

### Ensure

Ensure:

1. loads current Provider config;
2. resolves PluginID;
3. reuses the active instance when it corresponds to the current config revision;
4. otherwise opens a replacement through the plugin;
5. installs the executor route;
6. closes any superseded instance.

Concurrent Ensure calls for the same Provider must not open N equivalent instances.

### Provider-scoped synchronization

Use ProviderID-scoped synchronization or equivalent singleflight behavior.

Do not preserve the current global activation mutex as the long-term mechanism.

A slow Provider must not block unrelated Provider activation.

### Reload

Updating Provider config invalidates the old instance.

Preferred transition:

~~~text
validate config
open replacement where appropriate
atomic executor/instance swap
close old instance
sync Catalog as requested
~~~

Where an upstream cannot be opened before persistence, failure behavior must remain explicit and recoverable.

### Delete

Deletion must close active execution state as part of Provider removal.

Usage checks for Profile/Policy references still run before destructive removal.

### Shutdown

Runtime closes all active instances exactly once.

Tests include stdio-process-compatible close behavior through fakes rather than leaking child processes.

## Generic Provider configuration

### Persistence target

Introduce a generic schema separate from canonical Catalog tables.

Conceptually:

~~~text
provider_configs
  provider_id       PRIMARY KEY
  plugin_id         NOT NULL
  config_version    NOT NULL
  config_json       NOT NULL
  created_at        ...
  updated_at        ...
~~~

The exact timestamp requirement may be reduced if Catalog Provider timestamps already provide all required management semantics.

### Migration

Existing MCP schema version 3 contains:

- transport;
- endpoint;
- headers;
- credential headers;
- executable;
- args;
- directory;
- credential environment.

Forward migration maps each row to:

~~~text
plugin_id = io.mender.provider.mcp
config_version = <MCP plugin config version>
config_json = normalized MCP config
~~~

Mapping:

~~~text
http  -> transport.kind = streamable-http
stdio -> transport.kind = stdio
~~~

No Provider row is recreated.

No Tool snapshot is rewritten.

No ProviderID/ToolID/Profile/Policy target changes.

### Compatibility

The new store must either import/migrate legacy data transactionally or provide a deterministic one-time compatibility migration.

Downgrade behavior follows ADR-0014: a binary that does not support the newer schema must fail closed rather than silently reinterpret it.

## Credential bindings

Provider authentication remains governed by ADR-0021.

Phase 13 must make Credential usage generic.

Required properties:

- plugin config stores Credential References only;
- secret values are resolved only at open/execution boundaries;
- Runtime can enumerate Provider-referenced Credential References for deletion checks;
- Profile-scoped execution still proves access to every Provider-required reference before upstream execution;
- legacy MCP environment header templates remain explicitly legacy until separately migrated.

Two acceptable implementation strategies:

1. normalized Provider credential-binding persistence owned by Runtime; or
2. a plugin config inspection contract that returns referenced Credential metadata without opening the plugin.

Selection should favor simple migration and preserve adapter-specific placement semantics.

## Generic Runtime lifecycle

Target behavior replaces MCP-specific naming:

~~~text
RegisterConfiguredProvider
UpdateConfiguredProvider
SyncConfiguredProvider
EnsureConfiguredProvider
DeleteConfiguredProvider
~~~

Naming should remain concise; exact exported names may drop “Configured” when unambiguous.

### Sync

~~~text
ProviderID
  -> load ProviderConfig
  -> plugin registry
  -> instance manager Ensure/Reload
  -> Catalog.SyncProviderTools(instance)
~~~

### Call

~~~text
ToolID
  -> canonical Tool
  -> ProviderID
  -> instance manager Ensure
  -> shared InvokeTool
  -> executor router
  -> active instance
~~~

No MCP-specific activation appears in the generic Tool call path.

### MCP server startup

Profile-scoped downstream MCP serving resolves ProviderIDs from selected Tools and calls generic Provider Ensure.

Downstream server implementation remains MCP-specific because it is an exposure adapter, not an upstream Provider plugin.

## Management API

### Target contract

Prefer:

~~~text
GET  /provider-plugins
GET  /providers
POST /providers
PUT  /providers/{providerId}
POST /providers/{providerId}/sync
DELETE /providers/{providerId}
~~~

The generic creation body carries:

~~~text
namespace
displayName
description
pluginId
config
credential bindings / plugin-owned auth config as selected design requires
~~~

### Compatibility strategy

Because current SDK/Web/Desktop code is generated from `contracts/api/openapi.yaml`, migration should be staged.

Recommended order:

1. backend generic runtime/store;
2. add generic API while retaining existing HTTP/stdio endpoints as compatibility shims;
3. migrate SDK/frontend/Wails to generic API;
4. remove compatibility endpoints after all first-party clients use the generic contract.

If the compatibility window is completed inside Phase 13, the final contract contains no transport-specific create endpoints.

## CLI

Current:

~~~text
mender provider add http
mender provider add stdio
~~~

Target user experience may keep ergonomic MCP aliases temporarily, but the conceptual structure becomes:

~~~text
mender provider add mcp
~~~

with one transport selection, or:

~~~text
mender provider add mcp http
mender provider add mcp stdio
~~~

The CLI must not make HTTP/stdio top-level Provider implementation identities.

Machine-readable JSON should expose `pluginId` and a safe connection/config projection.

Secret values remain absent.

## Web/Desktop UX

Provider creation becomes plugin-first.

Recommended flow:

### Step 1 — Provider

- namespace;
- display name;
- description;
- Provider plugin.

### Step 2 — Connection

For MCP:

- Local command;
- Remote endpoint.

The rest of the fields depend on the selected transport.

### Step 3 — Authentication

Reuse inventory-backed Credential selectors.

Connection metadata and authentication stay separated, preserving the existing UI rule established during Provider refinement.

Provider rows should show:

- Provider name/namespace;
- plugin display name;
- concise connection summary;
- synchronized Tool count;
- status/actions.

Transport remains visible as secondary MCP connection detail, not the Provider category.

## Plugin management metadata

Phase 13 needs enough descriptor metadata for first-party management surfaces:

~~~text
PluginID
display name
description
plugin version
config version
capability flags needed by Mender UI
~~~

Do not design a general arbitrary UI-schema ecosystem unless required by MCP migration.

MCP can use a dedicated first-party form while the descriptor contract stabilizes.

## Security review

This phase touches process execution, filesystem paths, network access, credentials, and Provider trust.

### Local MCP

Preserve:

- no shell invocation;
- explicit executable/argv;
- minimal ambient environment;
- Credential-backed environment entries only;
- bounded/validated working directory semantics already enforced by current adapter behavior.

### Remote MCP

Preserve:

- non-loopback HTTPS requirement;
- no URL userinfo credentials;
- same-origin credential header redirect protection;
- bounded header validation;
- credential values resolved only at connection boundary.

### Plugin config

- config size should be bounded before persistence;
- unknown fields should fail or be explicitly normalized by the plugin;
- plugin validators must not perform arbitrary activation side effects;
- management projections must redact/omit secret material;
- unknown PluginID/config version fails closed.

### External plugins

Out of scope. No in-process third-party code loading is introduced.

## Observability and errors

Stable operational error classes should distinguish at least:

~~~text
plugin_not_found
plugin_config_invalid
provider_activation_failed
provider_instance_unavailable
provider_sync_failed
~~~

Existing Invocation failure classifications remain canonical unless a Domain-level distinction is required.

Provider/plugin-specific diagnostics must be bounded before reaching Audit or management responses.

## OpenAPI plugin readiness criteria

Phase 13 is successful only if the next OpenAPI Provider can be added without modifying:

- Catalog Domain Tool/Provider types;
- Catalog synchronization semantics;
- Invocation orchestration/policy stages;
- Profile ToolBinding Domain;
- Policy Domain;
- Audit Domain.

Expected OpenAPI work should mostly consist of:

~~~text
OpenAPI plugin config
OpenAPI discovery mapper
HTTP executor
credential placement
plugin registration
management form/schema
tests
~~~

This is an architectural acceptance test, even though OpenAPI implementation is deferred.

## Model-hosted Tool readiness

Model-provider hosted Tools are not assumed to be ordinary HTTP function calls.

A later Provider may execute:

~~~text
Mender Invocation
  -> model-tool plugin
  -> model provider API
  -> hosted tool / provider-side agentic loop
  -> normalized canonical Outcome
~~~

Phase 13 must not constrain every plugin to MCP-like transport semantics.

Therefore only the MCP plugin owns the `transport` concept.

## Test strategy

### Plugin registry

- duplicate registration rejected;
- unknown PluginID rejected;
- descriptor lookup has no activation side effects.

### Provider config store

- create/get/list/update/delete;
- unknown config version rejected;
- existing MCP v3 migration;
- HTTP mapping to streamable-http;
- stdio mapping preserved;
- Credential References preserved;
- newer schema fails closed.

### Instance manager

- Ensure reuses active instance;
- concurrent Ensure opens one instance per Provider/config revision;
- different Providers can activate independently;
- Reload swaps and closes old instance;
- failed replacement does not silently install a broken executor;
- Delete closes instance;
- CloseAll closes each active instance once;
- race test coverage.

### MCP plugin

- stdio discovery/invocation parity;
- Streamable HTTP discovery/invocation parity;
- authentication behavior unchanged;
- input-required mapping unchanged;
- Tool error mapping unchanged;
- close behavior unchanged.

### Runtime

- generic sync for MCP;
- generic Tool activation/call;
- Profile MCP server startup through generic Ensure;
- Provider update invalidates/reloads instance;
- Provider deletion usage checks unchanged.

### Adapters

- CLI;
- management HTTP;
- generated OpenAPI SDK;
- Wails bindings;
- React Web/Desktop.

### Repository validation

At phase close:

~~~text
pnpm check
pnpm security:check
~~~

Targeted Go race tests should run during the instance-manager work package before full repository validation.

## Work packages and commit segmentation

### 1. Architecture decision

Files:

- `docs/architecture/adr/0024-provider-plugin-runtime.md`
- ADR index.

Commit intent:

~~~text
docs(architecture): define provider plugin runtime
~~~

### 2. Phase plan

Files:

- this plan;
- roadmap;
- related product/development docs where terminology must change.

Commit intent:

~~~text
docs: plan provider plugin runtime phase
~~~

The first two documentation changes may be combined when reviewed as one architecture gate.

### 3. Plugin primitives and registry

Add:

- PluginID validation;
- Descriptor;
- Plugin/Instance ports;
- built-in registry;
- focused tests.

No persistence or management changes yet.

Commit intent:

~~~text
feat(provider): add plugin registry primitives
~~~

### 4. Provider Instance Manager

Add explicit active-instance ownership and Provider-scoped synchronization.

Replace repeated connection accumulation in generic call/start paths where possible without config migration.

Commit intent:

~~~text
refactor(runtime): manage provider instance lifecycle
~~~

### 5. MCP built-in plugin

Move/wrap MCP connection opening behind `io.mender.provider.mcp`.

Represent stdio and Streamable HTTP as transport variants.

Keep existing config store as a temporary adapter if needed to isolate the commit.

Commit intent:

~~~text
refactor(mcp): unify upstream transports behind provider plugin
~~~

### 6. Generic Provider config persistence

Introduce generic Provider config schema/store and migrate existing MCP rows.

Update Credential usage lookup.

Commit intent:

~~~text
feat(provider): persist plugin configurations
~~~

### 7. Generic Runtime orchestration

Replace MCP-specific register/update/sync/activate/delete logic with plugin-driven operations.

MCP-specific compatibility wrappers may remain only at adapter boundaries.

Commit intent:

~~~text
refactor(runtime): generalize provider orchestration
~~~

### 8. Management and CLI contracts

Add/migrate generic management endpoints and CLI Provider plugin semantics.

Regenerate SDK contract.

Commit intent:

~~~text
feat(adapters): expose provider plugin configuration
~~~

### 9. Web/Desktop Provider UX

Migrate first-party UI to plugin-first Provider creation/edit/view.

Regenerate/check Wails bindings.

Commit intent:

~~~text
feat(app): manage provider plugins
~~~

### 10. End-to-end compatibility and lifecycle coverage

Cover:

- existing database migration;
- MCP HTTP/stdio;
- direct Tool calls;
- Profiles/downstream MCP;
- Provider update/delete;
- Credential scope;
- instance reuse/close;
- Web/Desktop integration.

Commit intent:

~~~text
test(runtime): cover provider plugin lifecycle end to end
~~~

### 11. Closeout

Update:

- product vision terminology;
- development docs;
- upgrade notes;
- phase status;
- verification evidence.

Commit intent:

~~~text
docs: complete provider plugin runtime phase
~~~

## Exit criteria

Phase 13 is complete when:

- [x] Provider plugin identity/configuration is persisted generically.
- [x] `io.mender.provider.mcp` is registered as a built-in plugin.
- [x] MCP stdio and Streamable HTTP are transport variants of that plugin.
- [x] Catalog Provider Domain contains no plugin/transport SDK coupling.
- [x] Runtime generic Tool call path contains no MCP-specific activation.
- [x] Provider Instance Manager reuses and closes instances deterministically.
- [x] repeated direct Tool calls do not accumulate stale MCP connections/processes.
- [x] Provider-scoped activation does not globally serialize unrelated Providers.
- [x] existing MCP Provider configs migrate without ProviderID/ToolID changes.
- [x] Credential Reference usage checks work through the generic Provider boundary.
- [x] Profile Credential scope still blocks unauthorized shared Provider execution.
- [x] management API/SDK expose plugin-oriented Provider configuration.
- [x] Wails/Web/Desktop use the same plugin-oriented contract.
- [x] normal UX no longer presents HTTP/stdio as top-level Provider kinds.
- [x] legacy compatibility behavior, if retained, is explicitly documented.
- [x] targeted lifecycle race tests pass.
- [x] `pnpm check` passes.
- [x] `pnpm security:check` passes.

## Closeout

> Historical snapshot, dated 2026-09-27. The MCP dual store and HTTP/Wails compatibility paths described below were hard-retired by ADR-0043; hidden CLI transport aliases and Profile membership adapters were hard-retired by ADR-0044 on 2026-10-15. Do not use the former interfaces for new integrations.

Phase 13 completed on 2026-09-27.

The implementation now has a real Provider Plugin Runtime rather than a collection of MCP-specific Runtime branches:

- `providerplugin.Plugin` defines descriptor, configuration validation/inspection, and instance opening; the Registry resolves stable namespaced Plugin IDs.
- `providerplugin.Manager` owns ProviderID-scoped activation, revision-based reuse, replacement, executor routing, and deterministic close semantics without one global Provider activation lock.
- `io.mender.provider.mcp` is the first built-in plugin. Its stdio and Streamable HTTP paths are discriminated transport variants of one plugin configuration and both open the existing MCP anti-corruption `Connection`.
- generic `provider_configs` persistence is the Runtime source of truth. Existing MCP rows are imported once without recreating canonical Providers or Tools; the legacy MCP schema is retained temporarily as a version-4 compatibility/sentinel store for first-party compatibility paths and downgrade fail-closed behavior.
- Credential deletion/usage inspection now asks the registered plugin for bounded `ConfigMetadata` rather than understanding MCP authentication fields in the Credential bounded context.
- Runtime registration/update/sync/ensure/delete is plugin-generic. Direct Tool calls and Profile Provider activation no longer contain MCP-specific activation logic.
- HTTP/OpenAPI SDK and Wails expose Provider plugin discovery plus generic Provider create/update configuration. Legacy `/providers/http` and `/providers/stdio` creation remain compatibility shims.
- Web/Desktop Provider management is plugin-first; MCP local/remote choices appear only after selecting MCP and are stored as stdio/Streamable HTTP config variants.
- CLI normal discovery is plugin-first through `provider plugins` and `provider add mcp http|stdio`; the Phase-5 `provider add http|stdio` paths remain hidden compatibility aliases.
- Runtime end-to-end coverage proves repeated configured Tool calls reuse one plugin instance, configuration revision changes replace/close the old instance, and Provider deletion closes the current instance.

Validation evidence:

~~~text
pnpm sdk:check                         PASS
pnpm desktop:bindings:check            PASS
pnpm desktop:architecture:check        PASS
pnpm go:lint                           PASS
pnpm go:test                           PASS
pnpm go:test:race                      PASS
pnpm lint                              PASS
pnpm typecheck                         PASS
pnpm test                              PASS (5 files / 33 tests)
pnpm build                             PASS (Web + Desktop; existing chunk-size warnings only)
pnpm security:go                       PASS (no vulnerabilities: backend + desktop)
pnpm security:js                       PASS (no known production dependency vulnerabilities)
~~~

The aggregate `pnpm check` and `pnpm security:check` invocations exceeded the local bridge's single-call execution window, so their repository-defined constituent commands were run separately. Every constituent command in both scripts passed.

The next architectural validation should be a non-MCP Provider plugin, preferably OpenAPI-to-Tool, without changes to Catalog Domain, Invocation governance, Profile ToolBinding, Policy Domain, or Audit Domain.

## Deferred follow-up

After Phase 13, preferred next work is an OpenAPI Provider plugin used as the first non-MCP validation of the extension boundary.

A later separate architecture phase should cover external third-party plugin packaging/isolation before Mender accepts executable Provider code that is not built into the application.
