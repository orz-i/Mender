# ADR-0027: Host installable Provider plugins out of process behind a versioned Mender protocol

- Status: Accepted
- Date: 2026-09-27

## Context

ADR-0024 established Provider Plugin Runtime as Mender's internal upstream extension boundary.

Phases 14 and 15 validated that boundary with two materially different built-in Provider plugins:

- OpenAPI operations mapped into canonical Tools;
- model-provider-hosted tools executed through bounded agentic loops.

The current runtime is nevertheless still a **built-in plugin runtime** rather than an installable plugin platform.

Today:

- `bootstrap.NewRuntime` statically registers MCP, OpenAPI, and Model Tools Go implementations;
- `providerplugin.Registry` stores in-process `Plugin` values keyed only by PluginID;
- Provider configuration persists opaque plugin JSON, but does not pin an external package installation or digest;
- Web/Desktop Provider forms branch on the three known first-party PluginIDs;
- `providerplugin.OpenRequest` can carry an in-process `CredentialResolver`, which must not be exposed wholesale to third-party code;
- no package manifest, installation lifecycle, protocol compatibility, trust decision, or external process ownership exists.

The next requirement is to let users install a new Provider type without recompiling Mender.

That extension mechanism must preserve existing architectural invariants:

- Catalog Provider/Tool models remain protocol-independent;
- all execution still enters the shared Invocation pipeline;
- Policy/Profile/Credential scope remains host-owned;
- third-party code cannot inject arbitrary JavaScript/HTML into management surfaces;
- plugin crashes and dependency graphs must not become host-process crashes or ABI constraints;
- unknown/unavailable/incompatible plugins fail closed;
- existing built-in Providers continue to work without ProviderID/ToolID changes.

ADR-0024 deliberately deferred this decision and explicitly rejected Go native `.so` plugins as the cross-platform extension mechanism.

## External baseline

Reviewed on 2026-09-27.

Relevant current practice:

- Terraform Provider plugins are separate executables and communicate with Terraform through a versioned protobuf/gRPC plugin protocol.
- HashiCorp `go-plugin` provides subprocess lifecycle, handshake/protocol-version negotiation, gRPC transport, health checking, bounded plugin log buffering, and optional per-launch mTLS. Its magic-cookie handshake is an intent/UX mechanism, not a security sandbox.
- Terraform's Provider Registry separates Provider package version from runtime protocol version and distributes platform-specific packages with checksums/signatures.
- JSON Schema's current published specification remains Draft 2020-12 and is suitable for declarative Provider configuration validation/documentation.
- WASI 0.3, published in 2026, and the WebAssembly Component Model/WIT provide a credible later capability-oriented runtime, but introducing both native and WASM runtimes in the first installable-plugin phase would multiply compatibility and lifecycle work.

References:

- https://developer.hashicorp.com/terraform/plugin/terraform-plugin-protocol
- https://developer.hashicorp.com/terraform/internals/provider-registry-protocol
- https://github.com/hashicorp/go-plugin/blob/main/docs/internals.md
- https://github.com/hashicorp/go-plugin/blob/main/docs/guide-plugin-write-non-go.md
- https://json-schema.org/specification
- https://wasi.dev/
- https://component-model.bytecodealliance.org/design/wit.html

## Decision

Mender will support installable Provider plugins through an **out-of-process native plugin host**.

The internal Provider Plugin ports remain authoritative:

~~~text
providerplugin.Plugin
providerplugin.Instance
~~~

A new external adapter implements those ports while talking to one plugin executable through a Mender-owned, versioned protobuf/gRPC protocol.

Conceptually:

~~~text
Catalog / Invocation
        ^
        |
providerplugin.Plugin / Instance
        ^
        |
 ExternalPlugin adapter
        |
 Mender Provider Plugin Protocol v1
        |
 native plugin executable
~~~

The external protocol is an adapter boundary. Catalog, Invocation, Profile, Policy, Credential, and Audit domains do not depend on protobuf, gRPC, `go-plugin`, package manifests, or third-party SDK types.

### Native subprocess is the v1 runtime

Phase 16 uses native executables as the first external runtime.

Reasons:

- Provider integrations commonly need mature HTTP/TLS/client libraries and platform features;
- subprocesses isolate crashes and language/dependency graphs from the Mender process;
- protobuf/gRPC permits non-Go plugin implementations;
- the implementation can reuse established subprocess/protocol negotiation patterns;
- it avoids Go `plugin` ABI/platform restrictions;
- it leaves room for a later WASM Component runtime behind the same host abstraction.

A native subprocess is **not** treated as a security sandbox.

An installed native plugin is code the user/admin has explicitly chosen to trust on the local OS account. Mender minimizes its ambient environment and controls Mender-owned capabilities, but v1 does not claim to prevent the executable from using OS-level filesystem/network/process authority available to that account.

Platform-specific sandboxing may be added later without changing the canonical Provider model.

### Public protocol versus process transport

Mender owns the public **Provider Plugin Protocol**.

The protocol is defined in versioned protobuf files under a repository-owned contract package.

The v1 service covers, at minimum:

~~~text
GetDescriptor
ValidateConfig
InspectConfig
Open / Configure
ListTools
ExecuteTool
Close
~~~

Exact RPC factoring may combine `Open` and `Configure` if the implementation can preserve the same lifecycle semantics.

The public compatibility contract is the Mender protobuf protocol, not Go interfaces.

A process-transport library such as HashiCorp `go-plugin` may be used to own process startup/handshake/gRPC lifecycle, but Mender does not expose that library's Go API as the Provider SDK contract.

### Protocol version model

Four identities/versions are distinct:

~~~text
PluginID                dev.example.github
PackageVersion          1.4.2
ProtocolVersion         1
ConfigVersion           3
~~~

They must never be conflated.

- **PluginID** is the stable namespaced Provider implementation identity.
- **PackageVersion** identifies a released plugin package and follows SemVer for v1 packages.
- **ProtocolVersion** identifies host/plugin RPC compatibility.
- **ConfigVersion** identifies the plugin-owned persisted Provider configuration shape.

A plugin package may advertise multiple supported protocol versions.

Host/plugin negotiation selects one mutually supported protocol version before any Provider is activated.

Unsupported protocol versions fail closed with an explicit compatibility error.

### Plugin manifest

Every installed external plugin has bounded declarative package metadata.

Conceptual v1 manifest:

~~~json
{
  "manifestVersion": 1,
  "pluginId": "dev.example.github",
  "version": "1.4.2",
  "protocolVersions": [1],
  "displayName": "GitHub",
  "description": "Example Provider plugin",
  "entrypoints": {
    "darwin/arm64": "bin/mender-provider-github"
  }
}
~~~

Manifest validation is host-owned and side-effect free.

The manifest does not replace runtime `GetDescriptor`. Mender verifies that runtime identity/version/protocol metadata agrees with the installed manifest before registering the plugin.

A mismatch fails closed.

### Installation identity and exact package pinning

External package installation is separate from PluginID.

Mender persists an installation record containing at least:

~~~text
InstallationID
PluginID
PackageVersion
SourceKind
SourceReference
Digest
Manifest
Status
InstalledAt
~~~

An external Provider configuration pins the exact installation identity it uses.

Existing built-in Provider configurations retain their current PluginID configuration and do not require synthetic package rows.

Conceptually:

~~~text
provider_configs
  provider_id
  plugin_id
  plugin_installation_id NULLABLE
  config_version
  revision
  config_json
~~~

Rules:

- `plugin_installation_id = NULL` means a built-in plugin resolved from the built-in registry;
- external Providers require a valid installation ID whose manifest PluginID matches `plugin_id`;
- replacing/upgrading an installation never silently changes which executable an existing Provider uses;
- uninstall is blocked while any Provider references the installation;
- package digest is verified before launch.

This preserves existing Provider/Tool identities while allowing side-by-side external package versions.

### Initial installation source

Phase 16 v1 supports explicit local installation from a package/directory selected by the user/admin.

The install operation copies the validated package into a Mender-managed plugin directory and records its digest.

Remote registries, OCI distribution, automatic update, marketplace discovery, revocation feeds, and background update checks are deferred.

The installation model intentionally keeps `SourceKind` extensible so an OCI-backed source can be added later without changing Provider configuration identity.

### Runtime resolution

The current built-in-only `Registry` evolves into a resolver that can return either:

~~~text
BuiltInPlugin
ExternalInstalledPlugin
~~~

The Provider Instance Manager remains the owner of ProviderID-scoped active instances.

It continues to own:

- revision-based reuse;
- reload/replacement;
- executor routing;
- deterministic close;
- Provider-scoped synchronization.

The manager must not branch on concrete plugin type.

External process ownership lives below the plugin adapter.

A plugin process crash invalidates the affected external plugin session/instance and surfaces a bounded activation/execution failure. It must not terminate Mender or silently route to a different installation.

### Process lifecycle

Mender launches external plugin executables directly, never through a shell.

Minimum v1 controls:

- exact managed executable path;
- minimal environment;
- plugin-specific managed working directory;
- startup/handshake timeout;
- protocol negotiation timeout;
- per-RPC deadlines derived from host operations;
- bounded protobuf message sizes;
- bounded stderr/log capture;
- process-tree termination on host shutdown where supported;
- deterministic Close/kill fallback;
- package digest verification before launch;
- no secret material in argv, environment inherited from Mender, handshake output, or normal logs.

Process handshake data is not treated as an authentication/security boundary by itself.

If the selected transport library supports per-launch authenticated local RPC such as AutoMTLS, Mender enables it where compatible, but this protects the host/plugin RPC channel from unrelated local peers; it does not sandbox the plugin executable.

### Descriptor and configuration schema

The Provider plugin descriptor expands from display metadata into a bounded management contract.

Conceptually:

~~~text
PluginID
PackageVersion
ProtocolVersion
ConfigVersion
DisplayName
Description
ConfigSchema
UISchemaHints
Capabilities
~~~

`ConfigSchema` uses JSON Schema Draft 2020-12.

Mender may support a small reviewed set of `x-mender-*` annotations for UI widgets such as Credential inventory selection.

Unknown extension annotations are ignored for presentation and never become security policy.

`UISchemaHints` is declarative metadata only.

External plugins cannot provide executable JavaScript, React components, HTML, CSS, or arbitrary frontend bundles.

### Generic management form

Unknown external plugin types must be manageable without modifying Mender frontend source.

The management flow becomes descriptor-driven:

~~~text
Plugin Descriptor
   |
Config JSON Schema
   |
Mender-owned generic form
   |
plugin config JSON
~~~

First-party built-in plugins may retain dedicated Mender-owned renderers where they materially improve UX.

A renderer is an optional host override keyed by known PluginID. Absence of an override must fall back to the generic form rather than make the plugin unusable.

The generic form is the architectural acceptance path for external plugins.

### Configuration validation

Configuration passes two validation layers:

1. Mender validates the generic envelope, size limits, JSON validity, and supported JSON Schema contract.
2. The plugin validates plugin-specific semantics through `ValidateConfig`.

Plugin validation diagnostics are untrusted bounded data.

Validation must not activate a Provider, resolve Credentials, execute Tools, or perform arbitrary host capability calls.

The native executable itself is already trusted OS code after installation, so Mender cannot technically prevent all side effects inside the process. The protocol contract nevertheless defines `ValidateConfig` as side-effect-free, and Mender does not supply credentials or host capabilities during validation.

### Credential boundary

External plugins never receive Mender's in-process `CredentialResolver` capability.

The host remains the Credential policy enforcement point.

`InspectConfig` returns bounded metadata describing canonical Credential References used by the configuration.

Before secret material can cross the external RPC boundary:

1. Mender verifies the persisted Provider configuration and selected installation;
2. the external adapter determines the bounded Credential References required for the operation;
3. the existing Invocation-scoped `CredentialResolver` resolves those references;
4. Profile scope therefore still rejects an unapproved reference with the existing `ErrCredentialOutsideScope` behavior;
5. only the resolved values needed for that operation are placed in an ephemeral RPC request;
6. the plugin cannot enumerate Credential inventory or request arbitrary references through a host resolver API.

Provider synchronization/discovery may use Provider-declared discovery credentials under the existing privileged management/catalog operation semantics.

Execution credentials are resolved for every governed Tool invocation, not cached as a general external-plugin secret store.

The external adapter minimizes the lifetime of resolved bytes and never persists them.

The protocol may add per-Tool required Credential metadata so the host sends only the subset needed for the selected Tool. Until that metadata is available, an external plugin may require all Provider-declared execution Credential References; this is safe but potentially conservative.

### Tool discovery

The external protocol carries protocol DTOs, not Catalog domain types.

The adapter maps each plugin Tool descriptor into the existing canonical ToolSource contract.

The same invariants remain required:

- stable SourceKey;
- bounded name/title/description;
- valid self-contained input/output JSON Schemas;
- no Credential secret in Tool metadata;
- deterministic discovery result;
- duplicate SourceKeys rejected;
- Catalog owns ToolID generation/persistence.

A plugin cannot supply a canonical ToolID.

### Tool execution

An external Provider Tool invocation remains:

~~~text
caller
  -> InvokeTool
  -> Profile scope
  -> argument validation
  -> Policy / approval
  -> execution guards
  -> scoped CredentialResolver
  -> ExternalPlugin Instance
  -> plugin RPC
  -> canonical Outcome
  -> ResultGuard
  -> Audit
~~~

No external plugin endpoint bypasses `InvokeTool`.

The protocol supports bounded progress events so existing ProgressReporter semantics can be preserved without exposing host internals.

Expected plugin failures map into existing canonical Invocation failure outcomes/classes where possible.

Plugin/protocol lifecycle failures remain operational errors at the Provider adapter boundary and are bounded before management/Audit presentation.

### Trust and permissions

Installation is an explicit trust decision.

Mender displays at least:

- PluginID;
- package version;
- source;
- digest;
- declared capabilities/permissions metadata;
- signature status when signature support exists.

Phase 16 does not claim OS-level enforcement of native plugin filesystem/network/process declarations.

The only strongly enforced external-plugin capabilities in v1 are Mender-owned capabilities that must cross the RPC boundary, especially Credential material and governed Tool invocation.

A future WASM runtime may provide stronger capability enforcement through host imports.

### Built-in plugins remain supported

MCP, OpenAPI, and Model Tools stay statically linked in Phase 16.

They continue implementing the current in-process `providerplugin.Plugin` port.

They are not forced through subprocess RPC merely for architectural symmetry.

This keeps trusted first-party startup fast and avoids migrating working Provider configurations solely to prove the external ABI.

### WASM is a future runtime, not a v1 requirement

The host architecture must allow another implementation such as:

~~~text
PluginRuntime
  +-- BuiltIn
  +-- NativeProcess
  +-- WasmComponent
~~~

A later ADR/phase may adopt the WebAssembly Component Model/WIT and WASI capabilities for a lower-trust plugin class.

Phase 16 does not ship a WASM runtime.

### Package distribution beyond local install

The installation record and manifest are designed so later distribution can use content-addressed sources such as OCI artifacts and signatures such as Sigstore/Cosign.

Phase 16 does not require a public registry or marketplace.

Automatic plugin download/update is deliberately deferred until origin trust, signing, revocation, proxy, and enterprise policy are specified.

## Security invariants

External Provider plugins add a new executable trust boundary. The following are mandatory:

1. existing Invocation governance remains the only Tool execution path;
2. plugins cannot receive a general Mender service locator or CredentialResolver;
3. secret values never enter persisted plugin config, manifests, Catalog, normal Audit, or management projections;
4. Profile-scoped Credential enforcement happens before execution secret values are sent to the plugin;
5. plugin identity/version/protocol must match the installed manifest;
6. executable digest is checked before launch;
7. incompatible/unknown/missing installations fail closed;
8. plugin stdout handshake/protocol data and stderr diagnostics are bounded;
9. plugin processes launch without a shell and with minimal inherited environment;
10. plugin configuration/UI metadata is treated as untrusted bounded data;
11. external plugins cannot inject executable frontend code;
12. uninstall is blocked while Providers reference an installation;
13. upgrading one plugin package never silently retargets existing Providers;
14. a plugin process crash cannot terminate Mender;
15. native subprocess isolation is never represented to users as an OS sandbox.

## Alternatives considered

### Dynamically load Go `.so` plugins

This would map directly to the current Go interface and avoid RPC DTOs.

Rejected because it expands third-party code into the host process, couples build toolchain/dependencies, has platform limitations, and provides weak crash/resource isolation.

### Require every custom Provider to expose MCP

This would let Mender reuse the MCP Provider without adding a plugin ABI.

MCP remains a strong interoperability option, but requiring it would prevent Provider-specific configuration/discovery/execution semantics and Mender-owned declarative management metadata. It would also force integrations whose natural execution is not MCP to build a protocol server merely to become a Provider.

Rejected as the only extensibility mechanism. Custom MCP servers remain fully supported.

### Load arbitrary frontend bundles from plugins

This gives plugin authors complete UX freedom.

Rejected because it moves third-party executable code into the Web/Desktop trust boundary, complicates CSP/supply-chain review, and makes management compatibility depend on frontend APIs.

Mender-owned schema-driven forms are selected instead.

### Pass a host CredentialResolver over reverse RPC

This most closely resembles the current in-process Plugin API.

Rejected because it would let a compromised plugin probe arbitrary Credential References at runtime and turns a narrow secret handoff into an open capability.

The host resolves a bounded approved set and sends only operation-required values.

### Make external plugins a separate Provider domain type

This would avoid modifying the current registry/config resolution model.

Rejected because external versus built-in is a deployment/runtime concern, not a canonical Provider category.

### Ship native process and WASM runtimes together

This could establish the final multi-runtime model immediately.

Deferred because it doubles SDK, packaging, compatibility, filesystem/network capability, and lifecycle work before the external contract itself has production evidence.

The public protocol/runtime abstraction is designed not to block a later WASM Component implementation.

### Automatically update plugins to latest versions

This reduces manual maintenance.

Rejected for v1 because Provider behavior and config compatibility must not change without an explicit installation/upgrade operation, and supply-chain/revocation policy needs a separate review.

## Consequences

### Positive

- users can add new Provider types without recompiling Mender;
- plugin crashes/dependencies are isolated from the host process;
- non-Go plugins can implement the protobuf/gRPC protocol;
- Catalog/Invocation/Profile/Policy/Audit remain unchanged by plugin type;
- existing built-in Providers retain their current fast in-process implementation;
- Provider configurations can pin exact external package installations;
- generic JSON-Schema-driven configuration removes the need for frontend changes for every new plugin;
- Credential scope remains host-owned and enforceable;
- protocol, package, and config versions can evolve independently;
- a future WASM runtime can be introduced behind the same architectural boundary.

### Negative / trade-offs

- process/RPC startup and serialization add latency and implementation complexity;
- external plugin packages require installation persistence and lifecycle management;
- protobuf contracts become a compatibility surface that must evolve conservatively;
- generic schema-driven forms will be less polished than dedicated first-party forms;
- native subprocesses remain fully trusted local executables at the OS level;
- cross-platform process-tree cleanup and managed paths require platform-specific tests;
- exact installation pinning makes upgrades explicit and may require a migration UX.

## Follow-up

Phase 16 implements this ADR in segmented work packages:

1. versioned Provider Plugin Protocol and manifest primitives;
2. installation persistence and managed local-package install/remove;
3. native process host/supervisor;
4. ExternalPlugin adapter and Credential handoff;
5. Provider config installation pinning and resolver integration;
6. management API/CLI/Wails contracts;
7. descriptor/schema-driven Web/Desktop generic Provider form;
8. public Go SDK and example external Provider;
9. lifecycle/security/compatibility hardening and closeout.

A later phase may add OCI-backed distribution, signature/revocation policy, automatic update checks, and/or a WASM Component runtime.
