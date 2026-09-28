# Phase 16 Plan — External Provider Plugin Host

Status: Complete

## Objective

Turn the existing Provider Plugin Runtime into an installable Provider plugin platform.

A user must be able to install a new Provider type from outside the Mender repository, configure it through generic management surfaces, synchronize its Tools into the canonical Catalog, and invoke those Tools through the existing governed Invocation pipeline without adding the PluginID to Mender source code.

ADR-0027 defines the architecture and trust boundary.

## Architectural acceptance test

The phase is successful when an independently built example plugin can complete this flow:

~~~text
example-provider repository
  -> build/package
  -> mender plugin install <local package>
  -> management/CLI discovers plugin descriptor
  -> create Provider with generic schema form/config
  -> sync canonical Tools
  -> bind Tool to Profile
  -> governed InvokeTool
  -> scoped Credential handoff
  -> canonical Outcome
  -> uninstall blocked while Provider references installation
~~~

Mender core must not require a PluginID-specific branch for the example plugin.

No changes to canonical Catalog Provider/Tool domain types, Policy match semantics, Profile ToolBinding identity, or Audit record identity are expected.

## Current baseline

Phase 13 already provides:

- stable namespaced PluginID;
- generic opaque Provider configuration persistence;
- plugin-owned ValidateConfig/InspectConfig/Open;
- ProviderID-scoped instance reuse/reload/close;
- generic ToolSource and ToolExecutor ports;
- Credential Reference inspection;
- generic Runtime register/update/sync/ensure/delete operations;
- management plugin discovery.

Phases 14 and 15 proved that OpenAPI and Model Tools can join the Runtime without changing Catalog/Invocation orchestration.

Remaining built-in assumptions:

- Runtime statically registers three Go plugins;
- Registry identity is only PluginID;
- there is no package/installation model;
- there is no public host/plugin protocol;
- Provider config cannot pin an external installation;
- Web/Desktop configuration is hard-coded for first-party PluginIDs;
- external code cannot safely receive the in-process CredentialResolver interface.

## External baseline

Reviewed on 2026-09-27.

### Executable plugin protocol

Terraform uses separate Provider executables and a versioned protobuf/gRPC protocol.

HashiCorp go-plugin provides mature subprocess handshake/lifecycle patterns and can support non-Go gRPC implementations. Mender may reuse that process transport while keeping its own protobuf service contract authoritative.

References:

- https://developer.hashicorp.com/terraform/plugin/terraform-plugin-protocol
- https://github.com/hashicorp/go-plugin/blob/main/docs/internals.md
- https://github.com/hashicorp/go-plugin/blob/main/docs/guide-plugin-write-non-go.md

### Package/version distribution

Terraform Provider distribution distinguishes package versions from runtime protocol versions and verifies platform package checksums/signatures.

Reference:

- https://developer.hashicorp.com/terraform/internals/provider-registry-protocol

### Declarative configuration

JSON Schema Draft 2020-12 remains the current published JSON Schema specification.

Reference:

- https://json-schema.org/specification

### Future capability-oriented runtime

WASI 0.3 and the WebAssembly Component Model/WIT are suitable candidates for a later capability-oriented plugin runtime.

References:

- https://wasi.dev/
- https://component-model.bytecodealliance.org/design/wit.html

## Scope

Phase 16 includes:

- Provider Plugin Protocol v1;
- protobuf/gRPC contract generation;
- local external-plugin package manifest v1;
- package/installation identity and persistence;
- managed local install/list/remove/enable/disable lifecycle;
- exact package digest verification;
- native plugin process supervisor;
- protocol compatibility negotiation;
- ExternalPlugin adapter implementing existing internal Provider plugin ports;
- external Tool discovery mapping;
- external Tool execution mapping;
- bounded progress propagation;
- host-owned scoped Credential resolution and ephemeral secret handoff;
- Provider configuration installation pinning;
- built-in/external plugin resolution;
- API/CLI/Wails plugin installation management;
- expanded ProviderPlugin descriptor metadata;
- JSON-Schema-driven generic Provider configuration UI;
- first-party renderer fallback/override rules;
- standalone Go SDK;
- standalone example external Provider;
- lifecycle, crash, timeout, malformed protocol, credential-scope, and compatibility hardening tests.

## Non-goals

Phase 16 does not include:

- public plugin marketplace;
- automatic remote plugin discovery;
- automatic plugin update;
- OCI registry installation;
- Sigstore/Cosign verification;
- revocation feeds;
- enterprise allow/deny registry policy;
- OS-level native executable sandboxing;
- WASM plugin execution;
- arbitrary plugin-provided React/HTML/CSS/JavaScript;
- remote Mender management access;
- bypass execution paths outside InvokeTool;
- changing canonical Provider/Tool identity rules;
- migrating trusted built-in plugins into subprocesses.

These may be separate later phases.

## Terminology

### Plugin

Stable Provider implementation identity:

~~~text
dev.example.github
~~~

### Package

One released version of an external PluginID:

~~~text
PluginID        dev.example.github
PackageVersion  1.4.2
~~~

### Installation

One locally managed immutable package instance identified by Mender.

An installation records package source, digest, manifest, compatibility, and status.

### Provider

One user configuration of a plugin implementation, retaining the existing canonical ProviderID and namespace.

### Protocol version

Version of the Mender host/plugin RPC compatibility contract.

### Config version

Version of the plugin-owned Provider configuration document.

## Version model

Keep these separate:

~~~text
Plugin ID             dev.example.github
Package version       1.4.2
Protocol version      1
Config version        3
Manifest version      1
~~~

Rules:

- PackageVersion uses valid SemVer in v1.
- Protocol negotiation is explicit.
- ConfigVersion belongs to the Provider plugin.
- ManifestVersion belongs to package metadata.
- existing built-in plugins do not need package installations.
- external Provider configurations pin InstallationID.

## Package layout

Preferred backend layout:

~~~text
backend/internal/providerplugin/
  plugin.go
  registry.go
  manager.go

backend/internal/providerplugin/protocol/
  v1/
    provider_plugin.proto
    generated...

backend/internal/providerplugin/manifest/
  manifest.go
  manifest_test.go

backend/internal/providerplugin/installation/
  domain/
  application/
  infrastructure/sqlite/

backend/internal/providerplugin/external/
  plugin.go
  instance.go
  process.go
  transport.go
  mapper.go
  credentials.go
  *_test.go
~~~

Exact folder names may be simplified while preserving Clean Architecture dependency direction.

Public SDK/protocol packaging should not import backend internal packages.

Candidate public layout:

~~~text
contracts/providerplugin/v1/provider_plugin.proto
sdk/providerplugin/go/...
examples/provider-plugin-example/...
~~~

If keeping generated Go code inside the backend initially reduces repository complexity, the public SDK extraction can happen after the protocol stabilizes within Phase 16, but before closeout.

## Manifest v1

Conceptual:

~~~json
{
  "manifestVersion": 1,
  "pluginId": "dev.example.echo",
  "version": "0.1.0",
  "protocolVersions": [1],
  "displayName": "Example Echo",
  "description": "Example external Mender Provider",
  "entrypoints": {
    "darwin/arm64": "bin/mender-provider-example",
    "linux/amd64": "bin/mender-provider-example"
  }
}
~~~

Validation:

- exact supported manifestVersion;
- canonical providerplugin PluginID;
- valid SemVer;
- non-empty unique supported protocol versions;
- bounded display name/description;
- normalized relative entrypoint paths only;
- no absolute paths;
- no parent-directory escape;
- selected OS/arch must exist;
- package file count/size bounded;
- symlink policy explicit and fail-closed;
- selected executable must remain inside managed package root.

The package itself is treated as immutable after installation.

## Installation persistence

Add a separate SQLite bounded context/store.

Conceptual schema:

~~~text
provider_plugin_installations
  installation_id    PRIMARY KEY
  plugin_id          NOT NULL
  package_version    NOT NULL
  source_kind        NOT NULL
  source_reference   NOT NULL
  digest             NOT NULL
  manifest_json      NOT NULL
  status             NOT NULL
  installed_at       NOT NULL
~~~

Initial status values:

~~~text
enabled
disabled
invalid
~~~

A failed launch does not automatically mutate persistent status to invalid; operational health and administrative enablement remain separate.

Installation identity is generated by Mender and is not derived only from PluginID/version.

Uniqueness should prevent accidentally importing the same immutable package repeatedly while still permitting side-by-side versions.

## Managed plugin directory

Mender owns a platform-appropriate managed plugin root.

The persistence layer stores logical installation metadata, not arbitrary executable paths supplied at invocation time.

Installation:

1. read package from user-selected local path;
2. validate bounded package/manifest;
3. select platform entrypoint;
4. calculate package/content digest;
5. copy into a staging directory under managed root;
6. fsync/close as practical;
7. atomically move to immutable installation directory;
8. persist installation metadata;
9. perform descriptor/protocol probe without Provider activation;
10. surface installed plugin.

If persistence fails after copy, compensate by removing the staged/installed directory.

If copy fails, persistence must not occur.

Uninstall removes persistence and files only after reference checks and active process shutdown.

## Provider configuration pinning

Migrate generic Provider config schema forward.

Add nullable:

~~~text
plugin_installation_id
~~~

Rules:

- built-in Provider: null;
- external Provider: non-null;
- referenced installation PluginID must match config PluginID;
- disabled/missing installation fails activation/sync;
- Provider update cannot silently change installation;
- plugin upgrade/retarget is a separate explicit operation;
- uninstall is rejected while referenced.

Existing rows migrate unchanged with null installation ID.

## Registry/resolver evolution

The current providerplugin.Registry is built-in-only.

Introduce a resolution boundary with semantics such as:

~~~text
ResolveBuiltIn(pluginID)
ResolveInstallation(installationID)
Descriptors()
~~~

Avoid forcing persistent installation responsibilities into the current simple in-memory Registry.

A likely split:

~~~text
BuiltInRegistry
InstallationCatalog
PluginResolver
~~~

The Runtime depends on a resolver interface rather than directly assuming every plugin lives in the built-in map.

Do not make Provider Instance Manager understand package stores or process launch details.

## Protocol v1

The protobuf protocol carries transport DTOs only.

Minimum service shape:

~~~text
service ProviderPlugin {
  rpc GetDescriptor(GetDescriptorRequest) returns (GetDescriptorResponse);
  rpc ValidateConfig(ValidateConfigRequest) returns (ValidateConfigResponse);
  rpc InspectConfig(InspectConfigRequest) returns (InspectConfigResponse);
  rpc Open(OpenRequest) returns (OpenResponse);
  rpc ListTools(ListToolsRequest) returns (ListToolsResponse);
  rpc ExecuteTool(ExecuteToolRequest) returns (stream ExecuteToolEvent);
  rpc Close(CloseRequest) returns (CloseResponse);
}
~~~

A session/token returned by Open may identify one configured Provider instance inside one plugin process.

Alternative factoring is acceptable if:

- multiple Provider instances can be represented safely;
- Provider instance close is deterministic;
- one plugin process failure is detected;
- no host internal Go interface leaks into protobuf.

### Descriptor DTO

Contains:

~~~text
plugin_id
package_version
display_name
description
protocol_version
config_version
config_schema_json
ui_schema_json
capabilities
~~~

All strings/documents bounded.

Config/UI schemas are parsed and validated by Mender before management exposure.

### ValidateConfig

Input:

- ConfigVersion;
- bounded config JSON.

Output:

- valid/invalid;
- bounded structured diagnostics.

No Credentials.

No host callbacks.

No Provider activation.

### InspectConfig

Input:

- ConfigVersion;
- config JSON.

Output includes bounded metadata:

~~~text
credential_refs
connection_summary
~~~

The host validates canonical Credential References.

The host must not trust arbitrary plugin-generated metadata as authorization.

For v1, Credential refs returned by an external plugin must be present as exact canonical reference strings in its persisted config. This prevents an external plugin from using InspectConfig as an inventory-probing request channel.

A later schema annotation model may make this stronger/more declarative.

### Open

Open receives:

- host-generated Provider instance/session identity;
- ConfigVersion;
- config JSON;
- no resolved secret by default.

The plugin returns an opaque session handle if needed.

Open may initialize local state but must not create a parallel execution policy boundary.

Where discovery requires Provider authentication, the protocol carries explicitly scoped discovery credentials supplied by the host rather than a resolver callback.

### ListTools

Returns bounded Tool descriptors:

- SourceKey;
- name;
- title;
- description;
- input schema JSON;
- optional output schema JSON;
- execution Credential refs/metadata where supported.

The adapter maps these into canonical Tool candidates.

Limits:

- maximum tools per Provider;
- maximum schema/description bytes;
- duplicate SourceKey rejected;
- invalid JSON Schema rejected before Catalog sync;
- no plugin ToolID accepted.

### ExecuteTool

Host supplies:

- session;
- SourceKey/plugin tool identity;
- canonical invocation correlation ID safe for plugin logging;
- arguments JSON;
- only operation-required resolved Credential values.

Execution events are streamed:

~~~text
progress
result
input_required
failure
~~~

Exact representation must map without ambiguity into existing invocationdomain.Outcome variants.

Only one terminal event is accepted.

Progress/event count and message sizes are bounded.

Cancellation/deadline propagates to the RPC and plugin process operation.

## Credential handoff

The external plugin never receives a general CredentialResolver RPC.

### Provider configuration

InspectConfig returns canonical references.

Mender uses them for:

- Credential deletion usage checks;
- management display;
- sync/discovery credential planning.

### Invocation

The ExternalPlugin wrapper still receives the existing scoped CredentialResolver from InvokeTool.

It resolves only the references required for the selected plugin Tool.

Therefore:

~~~text
Profile ToolBinding CredentialRefs
        |
        v
allowlist CredentialResolver
        |
ExternalPlugin wrapper
        |
resolve required refs
        |
ephemeral ExecuteTool RPC credentials
~~~

If a required Provider credential is outside Profile scope, resolution fails before the plugin receives that secret or executes its upstream operation.

Secret bytes:

- are never persisted;
- are not sent during Descriptor/ValidateConfig;
- are not placed in process argv/environment;
- are redacted from plugin diagnostics;
- are zeroed/minimized in host memory where practical.

## Process host

Implement one Runtime-owned native plugin supervisor.

Responsibilities:

- exact executable launch;
- no shell;
- managed working directory;
- minimal environment;
- handshake/protocol negotiation;
- optional authenticated local transport where supported;
- process health observation;
- startup timeout;
- graceful close then kill fallback;
- stderr/log bounding;
- process cleanup during Runtime close;
- package digest verification before each fresh launch or equivalent immutable-file guarantee;
- one clear failure classification for crash/protocol mismatch/start timeout.

The process supervisor is infrastructure and must not import Catalog/Policy/Profile domain logic.

## Process reuse model

Start with one process per installed plugin package per Mender Runtime, hosting multiple Provider sessions when the protocol implementation supports it.

Reasons:

- lower repeated startup cost;
- clear package-version isolation;
- one crash affects Providers of that plugin installation but not unrelated plugins;
- avoids one process per invocation.

If the SDK/protocol cannot safely support multiple sessions initially, one process per Provider is acceptable as an implementation stepping stone, but the public protocol must not bake ProviderID into process identity.

Process ownership and Provider instance ownership remain distinct.

## Descriptor/schema management contract

Expand the current ProviderPlugin management DTO with optional fields:

~~~text
sourceKind
installationId
packageVersion
protocolVersion
configSchema
uiSchema
capabilities
enabled
~~~

Built-ins project sensible defaults:

~~~text
sourceKind = builtin
installationId absent
packageVersion = current descriptor version
~~~

External installations project:

~~~text
sourceKind = external
installationId = ...
packageVersion = manifest package version
~~~

Do not expose package filesystem paths unless explicitly required for local diagnostics.

## JSON Schema form

Generic Provider creation/editing must no longer return false/unsupported merely because PluginID is unknown to first-party code.

Preferred frontend structure:

~~~text
ProviderEditor
  |
  +-- known first-party renderer? -> dedicated form
  |
  +-- otherwise -> GenericProviderConfigForm
                      |
                      +-- descriptor.configSchema
                      +-- safe uiSchema hints
                      +-- Credential inventory widget
~~~

Requirements:

- Mender renders controls;
- no arbitrary HTML/JS from plugin;
- nested objects/arrays supported to a bounded depth;
- strings/numbers/booleans/enums supported;
- required fields surfaced;
- defaults applied explicitly;
- additionalProperties behavior respected;
- secret/Credential annotations never ask users to type secret values into Provider config;
- unsupported schema features produce an explicit management error rather than silently ignoring validation.

The backend plugin remains final semantic validator.

## First-party renderer policy

Existing MCP/OpenAPI/Model Tools dedicated forms may remain.

The mapping is host-owned:

~~~text
known PluginID -> optional first-party renderer
otherwise      -> generic schema renderer
~~~

First-party renderers must serialize to the same plugin config contract and cannot bypass backend validation.

Tests must include an unknown external PluginID to prove the generic fallback.

## CLI

Target top-level workflow:

~~~text
mender plugin list
mender plugin install <path>
mender plugin inspect <installation-or-plugin>
mender plugin enable <installation>
mender plugin disable <installation>
mender plugin remove <installation>
~~~

Provider workflow remains:

~~~text
mender provider plugins
mender provider add ...
mender provider sync ...
~~~

Provider plugin selection for an external plugin must identify the exact installation when multiple compatible versions exist.

Machine-readable JSON is required for installation commands.

No secret material in CLI output.

## Management API

Add installation resources separate from configured Providers.

Conceptual:

~~~text
GET    /provider-plugin-installations
POST   /provider-plugin-installations
GET    /provider-plugin-installations/{installationId}
POST   /provider-plugin-installations/{installationId}/enable
POST   /provider-plugin-installations/{installationId}/disable
DELETE /provider-plugin-installations/{installationId}
~~~

The exact local package upload/import shape must fit the loopback management security model.

For Web running outside the desktop file picker context, arbitrary server filesystem paths are not accepted casually.

Desktop may use a native file chooser/binding and invoke a host-local installation operation.

The browser management API may initially accept a multipart package upload to loopback if bounded/upload hardening is implemented, or external installation may remain CLI/Desktop-only in the first adapter commit. Final Phase 16 exit requires a coherent supported Web/Desktop management story.

## Desktop

Wails remains a shell.

Desktop-specific value:

- native package file/directory chooser;
- reveal installed plugin diagnostics where safe;
- install operation passes selected local path only through the desktop binding.

Provider management itself continues using shared React components.

## Example plugin

Add a standalone example proving the public contract.

Preferred v1 example:

~~~text
dev.mender.example.echo
~~~

It should:

- expose descriptor/config schema;
- accept a small config such as prefix/title;
- expose one deterministic Tool;
- execute without network;
- optionally support one Credential-ref mode to prove scoped handoff;
- stream one progress event;
- return canonical JSON;
- implement Close cleanly.

The example must not import backend/internal packages.

It is the architectural regression test that custom Provider types no longer require Mender source changes.

## SDK

Provide a small Go SDK after protocol primitives stabilize.

Responsibilities:

- handshake/server startup;
- protocol version advertisement;
- descriptor registration;
- config validation helpers;
- Tool descriptor/result mapping helpers;
- bounded diagnostics;
- cancellation;
- zero/minimize credential values where practical.

The SDK does not expose Mender domain packages.

Non-Go plugin authors can implement the protobuf contract directly; additional language SDKs are deferred.

## Error taxonomy

Introduce stable operational classes/messages for:

~~~text
plugin_install_invalid
plugin_install_conflict
plugin_installation_not_found
plugin_disabled
plugin_digest_mismatch
plugin_protocol_incompatible
plugin_start_failed
plugin_handshake_failed
plugin_crashed
plugin_rpc_failed
plugin_descriptor_invalid
plugin_config_invalid
plugin_tool_descriptor_invalid
~~~

Map into existing public error envelope conventions.

Do not invent new canonical Invocation result kinds solely for process failures.

## Observability

Safe operational metadata may include:

- InstallationID;
- PluginID;
- package version;
- protocol version;
- process/session state;
- bounded exit code/signal classification;
- startup duration;
- RPC operation class.

Do not include:

- config secrets;
- Credential values;
- full arguments/results in operational process logs;
- arbitrary unbounded plugin stderr.

Audit remains invocation-centric and data-minimized.

## Migration

### Provider config schema

Forward-only migration adds nullable installation identity.

Existing rows remain built-in and retain:

- ProviderID;
- PluginID;
- ConfigVersion;
- config JSON;
- ToolIDs;
- Profile bindings;
- Policy references.

### Management clients

Add optional descriptor fields so generated SDK/Web/Desktop clients can roll forward with built-in defaults.

### Provider UI

Refactor incrementally:

1. extract first-party config codecs/renderers from monolithic Providers page;
2. introduce plugin descriptor lookup;
3. add generic config state;
4. add generic renderer;
5. prove external example;
6. retain existing first-party behavior/tests.

Avoid one large rewrite of all management flows.

## Security tests

At minimum cover:

- manifest path traversal;
- absolute entrypoint rejection;
- symlink escape;
- oversized package/manifest/schema/config;
- unsupported manifest version;
- malformed SemVer;
- PluginID mismatch manifest vs runtime descriptor;
- package version mismatch;
- unsupported protocol;
- duplicate installation;
- digest mutation after install;
- executable missing/non-executable;
- startup timeout;
- invalid handshake;
- process immediate exit;
- process crash during ListTools;
- process crash during ExecuteTool;
- bounded stderr;
- cancellation/deadline;
- duplicate SourceKeys;
- invalid Tool schemas;
- multiple terminal execution events;
- no terminal execution event;
- plugin attempts to reference undeclared Credential;
- Profile Credential outside scope;
- secret never appears in surfaced plugin failure;
- disable blocks new activation;
- uninstall in-use rejected;
- Runtime shutdown reaps processes.

## Test strategy

### Manifest

Pure unit tests for all validation/path/platform cases.

### Installation store

SQLite integration tests:

- migration;
- create/get/list/update status/delete;
- future schema fail-closed;
- immutable identity fields;
- duplicate/conflict rules.

### Installer

Filesystem tests under temporary managed roots:

- atomic staging;
- compensation;
- digest;
- immutable path;
- removal;
- in-use guard.

### Protocol

In-process fake gRPC contract tests plus generated compatibility fixtures where useful.

### Process host

Use tiny test plugin executables/subprocess helpers:

- healthy handshake;
- timeout;
- crash;
- malformed descriptor;
- protocol mismatch;
- cleanup.

Avoid tests that leak child processes.

### External adapter

Test as a normal providerplugin.Plugin/Instance:

- ValidateConfig;
- InspectConfig;
- Open;
- ToolSource mapping;
- ToolExecutor mapping;
- progress;
- Credential scope;
- Close.

### Runtime

End-to-end:

~~~text
install example
  -> resolve installation
  -> register external Provider
  -> sync
  -> invoke
  -> reload
  -> close
  -> delete Provider
  -> uninstall
~~~

Also prove two versions/installations do not silently retarget Providers.

### Frontend/Desktop

- plugin install/list/remove management;
- unknown plugin generic Provider creation;
- generic schema validation;
- Credential selector annotation;
- edit/view;
- first-party forms unchanged;
- unsupported schema feature error;
- installation version display.

## Work packages and commit segmentation

### 1. Architecture decision and phase plan

Files:

- docs/architecture/adr/0027-external-provider-plugin-host.md;
- ADR index;
- this phase plan;
- roadmap.

Commit intent:

~~~text
docs(architecture): define external provider plugin host
~~~

### 2. Manifest and protocol primitives

Add:

- manifest v1 parser/validator;
- protocol version primitives;
- protobuf contract v1;
- generated Go stubs;
- focused compatibility/limit tests.

No process launch yet.

Commit intent:

~~~text
feat(provider): define external plugin protocol
~~~

### 3. Plugin installation persistence

Add:

- InstallationID/domain model;
- SQLite schema/store;
- local managed-root abstraction;
- forward-only migration tests.

Commit intent:

~~~text
feat(provider): persist plugin installations
~~~

### 4. Local package installer

Add:

- package validation/copy;
- digest;
- atomic staging;
- enable/disable/remove;
- reference guards.

No Provider activation yet.

Commit intent:

~~~text
feat(provider): install local plugin packages
~~~

### 5. Native process host

Add:

- direct subprocess launch;
- handshake/protocol negotiation;
- gRPC lifecycle;
- deadlines/log limits/cleanup;
- process-host tests.

Commit intent:

~~~text
feat(provider): host external plugin processes
~~~

### 6. External Provider adapter

Implement current internal ports over protocol v1:

- descriptor;
- config validation/inspection;
- open/session;
- ToolSource;
- ToolExecutor;
- progress;
- Credential handoff;
- close.

Commit intent:

~~~text
feat(provider): adapt external plugins to runtime
~~~

### 7. Runtime resolver and Provider pinning

Add:

- provider config nullable InstallationID migration;
- built-in/external resolver;
- exact installation resolution;
- disabled/missing/digest mismatch fail-closed;
- Runtime end-to-end lifecycle tests.

Commit intent:

~~~text
feat(runtime): resolve installed provider plugins
~~~

### 8. CLI and management contracts

Add:

- plugin installation API/domain operations;
- CLI plugin commands;
- OpenAPI/SDK contracts;
- Wails bindings;
- generated checks.

Commit intent:

~~~text
feat(adapters): manage provider plugin installations
~~~

### 9. Generic Provider configuration UI

Refactor Provider UI:

- descriptor capabilities/schema;
- renderer registry;
- generic schema form;
- safe Credential widget;
- unknown plugin fallback;
- installation version/status.

Commit intent:

~~~text
feat(app): configure external provider plugins
~~~

### 10. Go SDK and example plugin

Add:

- public Go SDK;
- standalone example plugin;
- package fixture;
- integration tests proving no Mender PluginID branch.

Commit intent:

~~~text
feat(sdk): add provider plugin sdk and example
~~~

### 11. Hardening and compatibility

Add:

- crash/timeout/malformed protocol cases;
- path/symlink/package hardening;
- race tests;
- secret leak regression tests;
- multi-version/pinning tests;
- shutdown process leak checks.

Commit intent:

~~~text
test(provider): harden external plugin lifecycle
~~~

### 12. Closeout

Update:

- product/development docs;
- CLI docs;
- upgrade notes;
- roadmap/phase status;
- verification evidence;
- deferred OCI/signing/WASM plan.

Commit intent:

~~~text
docs: complete external provider plugin phase
~~~

## Implementation and verification evidence

Phase 16 shipped in segmented commits:

- `3cd866b` — architecture decision and phase plan;
- `e15f5dc` — versioned Provider Plugin Protocol and manifest primitives;
- `0ae64ec` — plugin installation persistence;
- `64e8a0d` — managed local package installer;
- `dda34c2` — native subprocess host;
- `63a7e13` — external Provider adapter;
- `56999f8` — exact installation resolution and Provider pinning;
- `b5b2ac1` — CLI/OpenAPI/Wails installation management contracts;
- `0b0a981` — schema-driven unknown external Provider configuration;
- `d90c433` / `587cb35` — public Go SDK and standalone example cleanup;
- `e8aaa78` — process/protocol/package/Credential/multi-version hardening;
- `a7090a9` — Web/Desktop installation management UI and Desktop native directory install;
- `b2ac072` — Provider SDK/example transport dependency updated to patched `grpc-go v1.83.2` after `GO-2026-6443` was detected by the final security gate.

Architectural acceptance is covered by `TestRuntimeInstalledExternalProviderEndToEnd`, which builds the standalone public example module with `GOWORK=off`, packages and installs it, resolves its external descriptor/schema, registers an exact pinned Provider, synchronizes its Tool, invokes it through the shared governed Invocation pipeline, verifies disable/enable behavior, blocks in-use removal, and verifies Runtime shutdown reaps the package process.

Additional hardening covers package path/symlink/size limits, digest mutation, failed-install compensation, invalid handshake/protocol/descriptor/schema/Tool metadata, startup and execution deadlines, subprocess crashes during discovery/execution, bounded gRPC messages/logs, malformed terminal-event streams, Credential scope and secret redaction, future SQLite schema fail-closed behavior, and side-by-side package versions pinned by exact InstallationID.

Management security intentionally differs by host: CLI and Desktop can install a local package; Desktop uses a native directory picker. Browser management can list/enable/disable/remove installed packages and configure external Providers, but has no endpoint that accepts an arbitrary server filesystem path.

Final verification on 2026-09-28:

- `pnpm check` — passed, including protocol/schema generation drift checks, SDK/example lint/test/race/build, Wails bindings/architecture, backend lint/test/race, ESLint, strict TypeScript, 43 Vitest tests, and Web/Desktop production builds;
- `pnpm security:check` — passed; `govulncheck` reported zero reachable vulnerabilities in backend, Desktop, public Provider SDK, and example plugin, and `pnpm audit --prod --audit-level high` reported no known vulnerabilities.

## Exit criteria

Phase 16 is complete when:

- [x] ADR-0027 is accepted and implementation matches it.
- [x] an external package has manifest/package/protocol/config versions represented separately.
- [x] local packages install into a Mender-managed root with digest verification.
- [x] external Provider configurations pin an exact InstallationID.
- [x] existing built-in Provider configs migrate without identity changes.
- [x] unsupported/missing/disabled/digest-mismatched installations fail closed.
- [x] external plugins execute in subprocesses rather than the Mender process.
- [x] plugin crashes do not crash Mender.
- [x] native plugins launch without a shell and with bounded/minimal host process state.
- [x] public protobuf protocol is versioned and compatibility-tested.
- [x] external plugins cannot receive a general CredentialResolver capability.
- [x] Profile Credential scope is enforced before execution secrets cross RPC.
- [x] generic Tool discovery maps into canonical Catalog without accepting plugin ToolIDs.
- [x] external Tool execution uses the shared InvokeTool pipeline.
- [x] Web/Desktop can configure an unknown external PluginID through Mender-owned declarative UI.
- [x] external plugins cannot inject arbitrary frontend code.
- [x] first-party MCP/OpenAPI/Model Tools management behavior remains covered.
- [x] CLI/API/Wails manage plugin installations with machine-readable contracts.
- [x] uninstall is blocked while an installation is referenced.
- [x] side-by-side plugin versions do not silently retarget existing Providers.
- [x] a public Go SDK and standalone example external Provider work without importing backend internals.
- [x] targeted process/race/security tests pass without child-process leaks.
- [x] pnpm check passes.
- [x] pnpm security:check passes.

## Deferred follow-up

After Phase 16, evaluate a separate distribution/trust phase for:

- OCI artifact installation;
- Sigstore/Cosign signatures;
- registry origin policy;
- revocation;
- update discovery and explicit upgrade UX;
- enterprise plugin allowlists.

A separate runtime-hardening phase may evaluate a WASM Component/WIT Provider runtime for plugins that can operate through capability-oriented host imports.
