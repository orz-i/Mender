# Phase 11 Plan — Credential Inventory and Scoped Provider Authentication

Status: Complete

## Objective

Make Credential a real governance capability instead of a fail-closed placeholder.

Phase 11 introduces a browsable Credential inventory, safe secret backends, ToolBinding Credential scope, and Credential-backed MCP Provider authentication without allowing secret values into SQLite, Profile, Provider config, Audit, Policy, or generated response models.

The phase preserves the shared Invocation pipeline and keeps secret resolution at the execution/infrastructure boundary.

## Architectural baseline

Phase 11 follows ADR-0007, ADR-0012, ADR-0017, ADR-0019, ADR-0020, and ADR-0021.

## Scope

Phase 11 includes:

1. Credential Domain metadata and application use cases;
2. Credential SQLite metadata inventory;
3. native OS keyring SecretStore;
4. environment-backed Credential source;
5. production inventory-aware CredentialResolver;
6. list/create/get/rotate/delete management contracts;
7. in-use deletion checks;
8. ToolBinding CredentialRefs allowlist;
9. Profile SQLite migration for CredentialRefs;
10. Profile application validation against Credential inventory;
11. Invocation allowlist-wrapped resolver for Profile scope;
12. Provider credential-backed HTTP headers;
13. Provider credential-backed stdio environment injection;
14. per-call authorization checks for shared authenticated MCP connections;
15. CLI/OpenAPI/Wails/Web/Desktop Credential management;
16. Credential selector in Profile ToolBinding;
17. Provider authentication configuration using inventory selection;
18. backward compatibility for legacy environment header templates;
19. end-to-end tests and security verification.

## Non-goals

Phase 11 does not include:

- remote secret synchronization;
- cloud secret-manager integrations;
- per-user authenticated remote principals;
- per-call account switching for one Provider;
- automatic migration of existing environment templates into inventory Credentials;
- secret values in SQLite;
- secret values in Profile or Provider responses;
- secret reveal/copy API;
- policy rules based on secret content;
- Credential values in Audit or runtime events;
- encrypted-file fallback when native keyring is unavailable;
- a separate opaque CredentialID.

## Ubiquitous language

### Credential Reference

Canonical identity:

~~~text
credential://namespace/name
~~~

Normal UI derives this from namespace/name fields.

### Credential Source

~~~text
native
environment
~~~

native stores secret bytes in OS credential storage.

environment stores only an environment-variable name and resolves the current process environment at use time.

### Credential Inventory

Persistent non-secret metadata that can be listed and selected.

### SecretStore

Infrastructure capability for native secret bytes.

~~~text
Put(reference, secret)
Get(reference)
Delete(reference)
~~~

### CredentialResolver

Execution/infrastructure capability that resolves one Reference into fresh secret bytes.

### CredentialRefs

ToolBinding allowlist of inventory Credentials available to Profile-scoped execution.

Empty means no inventory-backed Credential access.

### Provider Credential Binding

Provider-owned non-secret declaration of how one Credential Reference becomes connection authentication.

## Credential Domain

Initial metadata:

~~~text
Credential
  Reference
  DisplayName
  Source
  EnvironmentVariable
  CreatedAt
  UpdatedAt
~~~

Invariants:

- Reference parses under existing credential:// grammar.
- DisplayName is non-empty and bounded.
- native source has no EnvironmentVariable metadata.
- environment source requires a valid environment variable name.
- secret bytes never enter Credential Domain state.

## Credential Application

Ports:

~~~text
Repository
SecretStore
EnvironmentLookup
UsageChecker
Clock
~~~

Use cases:

~~~text
CreateNative
CreateEnvironment
ListCredentials
GetCredential
RotateNativeSecret
DeleteCredential
ResolveCredential
~~~

CreateNative:

1. validate metadata;
2. reject existing Reference;
3. write secret to native SecretStore;
4. persist metadata;
5. compensate SecretStore if metadata persistence fails.

CreateEnvironment persists metadata only.

RotateNativeSecret refuses environment-backed Credentials.

DeleteCredential:

1. resolve metadata;
2. UsageChecker must report no Profile/Provider references;
3. native source deletes secret first;
4. delete metadata.

## Persistence

Credential SQLite schema version 1 owns:

~~~text
credential_schema_migrations
credentials
~~~

credentials stores:

- reference;
- display_name;
- source;
- environment_variable;
- created_at;
- updated_at.

Schema tests explicitly forbid columns containing secret/token/password/value material.

Future versions fail closed.

## Native keyring

The native SecretStore uses github.com/zalando/go-keyring.

Service name:

~~~text
Mender
~~~

Account:

~~~text
credential://namespace/name
~~~

No plaintext fallback exists.

The dependency remains isolated in Credential infrastructure.

## Production resolver

ResolveCredential loads metadata and selects source.

native:

- retrieve from SecretStore;
- copy into a fresh byte slice;
- return safe classified errors.

environment:

- lookup configured variable name;
- missing variable fails closed;
- return fresh bytes.

No resolver path logs the value.

## ToolBinding Credential scope

ToolBinding gains CredentialRefs.

Domain owns a bounded unique list of canonical Reference strings.

Profile Application validates each Reference through a CredentialLookup port before Save/Replace.

Normal UI edits the list through a Credential selector.

Profile SQLite advances one version and stores the allowlist as non-secret JSON/reference rows.

Existing bindings migrate to an empty list.

## Invocation scoped resolver

ScopedBindingGovernance gains allowed Credential References.

Catalog scope:

~~~text
full Runtime resolver
~~~

Profile scope:

~~~text
allowlist resolver(full Runtime resolver, CredentialRefs)
~~~

A disallowed Reference fails without invoking the underlying resolver.

The allowlist wrapper is passed only to ToolExecutor.

PolicyRequest, Audit, and lifecycle events remain free of CredentialRefs.

## Provider authentication

### HTTP

Provider config adds structured Credential headers:

~~~text
HeaderName
Scheme
CredentialRef
~~~

Examples:

~~~text
Authorization | Bearer | credential://github/work
X-API-Key    |        | credential://search/prod
~~~

Static non-secret headers remain supported.

Sensitive plaintext headers remain rejected.

Legacy environment-template headers remain compatibility-only.

### stdio

Provider config adds explicit Credential environment bindings:

~~~text
EnvironmentName -> CredentialRef
~~~

At connection activation Runtime resolves each Reference with the full resolver and injects the secret only into the child process environment.

No secret environment values are persisted.

### Required references

An MCP Connection records the set of Provider-required Credential References.

Before upstream CallTool it resolves every required Reference through the invocation-supplied resolver.

This authorizes use of the already-authenticated shared connection.

A Profile lacking one required Reference fails before CallTool.

## Failure model

Stable Credential failures should distinguish at least:

~~~text
credential_not_found
credential_outside_scope
credential_source_unavailable
credential_environment_missing
credential_native_store_failed
~~~

No error text includes secret values.

Credential access failure is an expected runtime/provider failure rather than an internal panic.

## Management contracts

### Credentials

New operations:

~~~text
GET    /credentials
POST   /credentials/native
POST   /credentials/environment
POST   /credentials/{namespace}/{name}/rotate
DELETE /credentials/{namespace}/{name}
~~~

No GET endpoint returns a secret.

Native create/rotate requests contain a secret field marked write-only in OpenAPI.

### Provider config

HTTP Provider contracts gain structured credentialHeaders.

stdio Provider contracts gain credentialEnvironment.

Provider responses expose References and formatting metadata, never resolved values.

### ToolBinding

ProfileTool input/response gains CredentialRefs.

Old ToolID/direct-only clients remain compatible and receive an empty normalized list.

## CLI

Credential commands:

~~~text
mender credential list
mender credential add native
mender credential add environment
mender credential rotate
mender credential remove
~~~

Native secret input must not be accepted as a normal command-line flag because shell history/process inspection can expose it.

Environment Credential creation accepts the environment-variable name as non-secret metadata.

Provider auth and ToolBinding advanced CLI commands may accept Credential References, but normal Web/Desktop flow uses inventory selectors.

## Web/Desktop UX

### Credentials

Credentials becomes list-first.

Create Credential is a focused Dialog with source-specific fields.

Rows show display name, derived Reference, source, environment variable name for environment source, and actions.

Native secret value is never shown after submission.

Native rows support Rotate secret.

Delete explains in-use conflicts.

### Profile ToolBinding

Selected Tool actions become:

- Presentation;
- Governance;
- Credentials.

Credentials Dialog lists inventory items with multi-select.

No raw Credential Reference input.

### Provider

Provider connection metadata and Authentication remain separate information categories.

Authentication Dialog:

- HTTP: static non-secret headers plus Credential header bindings;
- stdio: Credential environment bindings.

Credential choices come from inventory.

Legacy environment-template auth is displayed as Legacy and is not presented as inventory-scoped.

## Security invariants

1. SQLite stores no secret values.
2. Native keyring failure never falls back to plaintext.
3. Environment source stores only variable names.
4. API responses never echo native secret values.
5. CLI does not require secret values in command-line flags.
6. Profile CredentialRefs are inventory-validated and unique.
7. Profile empty CredentialRefs means no inventory-backed Credential access.
8. Catalog scope remains explicit privileged access.
9. Invocation does not resolve or log secret bytes.
10. Profile resolver denies outside-scope References before touching the underlying resolver.
11. Provider config stores References, never resolved values.
12. Shared MCP connections verify required References before Tool calls.
13. stdio secret environment injection remains explicit and minimal.
14. Audit/events/Policy contain neither CredentialRefs nor values by default.
15. Credential deletion fails while referenced.
16. Normal UI never asks users to type a complete Credential Reference.
17. Legacy env-template auth is clearly treated as compatibility-only.

## Test strategy

### Credential Domain/Application

Cover metadata invariants, native/environment creation, duplicate Reference, compensation, rotation, deletion, in-use conflicts, and no secret echo.

### Credential SQLite

Cover metadata round-trip, future-version fail closed, malformed state fail closed, and forbidden secret-bearing columns.

### Native keyring adapter

Use the library test/mock facility or an adapter-owned fake backend.

Never depend on a developer machine's actual keychain in unit tests.

### Profile

Cover CredentialRefs defaults, normalization, duplicate rejection, inventory validation, persistence migration, and replacement rollback.

### Invocation

Cover Catalog full resolver, Profile allowlist resolver, outside-scope denial without underlying Resolve call, and executor-only resolution.

### Provider

Cover HTTP Credential header resolution, scheme handling, stdio Credential environment injection, missing Credential failure, and no config secret persistence.

### MCP executor

Cover required Reference authorization before CallTool.

### Adapters/UI

Cover write-only secret requests, list responses without secret fields, selector workflows, Provider authentication selection, ToolBinding Credential selection, and legacy request compatibility.

### End to end

Cover:

- environment Credential -> HTTP Provider auth -> Profile allowlist -> successful MCP call;
- same Provider + Profile without allowed Reference -> no upstream Tool call;
- Catalog scope -> successful privileged call;
- native Credential resolver using fake SecretStore;
- delete in-use Credential -> conflict;
- CredentialRefs never appear in Audit.

## Work packages and commit segmentation

### 1. Architecture decisions

Commit:

~~~text
docs: define credential governance architecture
~~~

Contents: ADR-0020, ADR-0021, ADR index.

### 2. Phase plan/docs

Commit:

~~~text
docs: plan credential governance phase
~~~

Contents: this plan, Roadmap Phase 11, architecture/product/development wording.

### 3. Credential Domain and inventory application

Commit:

~~~text
feat(credential): add inventory domain and use cases
~~~

### 4. Credential SQLite metadata store

Commit:

~~~text
feat(credential): persist credential inventory metadata
~~~

### 5. Native/environment secret resolution

Commit:

~~~text
feat(credential): add native and environment secret sources
~~~

Includes the reviewed direct keyring dependency.

### 6. ToolBinding Credential scope

Commit:

~~~text
feat(profile): add credential scope to tool bindings
~~~

### 7. Invocation scoped Credential resolver

Commit:

~~~text
feat(invocation): enforce profile credential scope
~~~

### 8. Provider Credential bindings

Commit:

~~~text
feat(mcp): bind provider authentication to credentials
~~~

### 9. Adapter contracts

Commit:

~~~text
feat(adapters): expose credential inventory and bindings
~~~

### 10. Management UX

Commit:

~~~text
feat(app): manage credentials and credential scope
~~~

### 11. End-to-end coverage

Commit:

~~~text
test(runtime): cover credential governance end to end
~~~

### 12. Closeout

Commit:

~~~text
docs: complete credential governance phase
~~~

## Exit criteria

- [x] Credential inventory is a real bounded-context model.
- [x] Credential SQLite contains metadata only.
- [x] native secrets use OS keyring with no plaintext fallback.
- [x] environment Credentials resolve without persisting values.
- [x] create/list/get/rotate/delete contracts never reveal secret values.
- [x] in-use Credential deletion fails safely.
- [x] ToolBinding CredentialRefs are inventory-selected and persisted.
- [x] old Profile rows migrate to empty CredentialRefs without behavior change.
- [x] Profile scope receives an allowlist CredentialResolver.
- [x] Catalog scope retains explicit full resolver access.
- [x] Provider HTTP auth can use Credential-backed headers.
- [x] Provider stdio auth can use Credential-backed environment entries.
- [x] Provider config persists References only.
- [x] shared MCP connections verify required References before Tool calls.
- [x] a Profile lacking Provider-required CredentialRefs cannot use the authenticated connection.
- [x] legacy environment header templates remain backward compatible and marked legacy.
- [x] OpenAPI/Wails contracts synchronize.
- [x] normal Web/Desktop Credential management never requires raw Reference entry.
- [x] native secret CLI input is not exposed as a normal flag.
- [x] Audit/Policy/events remain free of secret values and CredentialRefs.
- [x] Go race tests pass.
- [x] pnpm check passes.
- [x] pnpm security:check passes.

## Closeout

Phase 11 completed on 2026-09-27.

Implementation commits:

~~~text
82c3779 docs: define credential governance architecture
18d6ab3 docs: plan credential governance phase
7c9646f feat(credential): add inventory domain and use cases
485c7cc feat(credential): persist credential inventory metadata
cd98585 feat(credential): add native and environment secret sources
d19f2e7 feat(profile): add credential scope to tool bindings
4aecad5 feat(invocation): enforce profile credential scope
c80415e feat(mcp): bind provider authentication to credentials
99942e3 feat(adapters): expose credential inventory and bindings
3a1735c feat(app): manage credentials and credential scope
64460ef fix(credential): satisfy static security gates
fb9b2fc test(runtime): cover credential governance end to end
~~~

Verification evidence:

- `pnpm check` passed after the end-to-end Credential governance coverage and static-security cleanup. OpenAPI SDK regeneration, Wails binding regeneration, desktop architecture checks, golangci-lint, all Go tests, all Go race tests, ESLint, strict TypeScript, 5 Vitest files / 30 tests, and Web/Desktop production builds are green.
- `pnpm security:check` passed. Pinned `govulncheck v1.8.0` found no vulnerabilities in backend or desktop, and production pnpm audit reported no known vulnerabilities at high severity.
- Credential SQLite persists non-secret metadata only. Native values are stored through the OS keyring adapter with no plaintext fallback; environment-backed Credentials persist only the variable name and resolve the process environment at use time.
- The local management/OpenAPI/Wails contracts support Credential list/get/create/rotate/delete. Native secret fields are request-only/write-only and never appear in Credential or Provider response models.
- ToolBinding CredentialRefs are inventory-validated and persisted as non-secret references. Existing Profile rows migrate with an empty allowlist, preserving pre-Phase-11 behavior.
- Invocation passes the full Runtime CredentialResolver only for explicit Catalog scope. Profile scope receives an allowlist resolver; an outside-scope Reference fails before the underlying secret resolver and before the upstream Tool call.
- MCP Provider HTTP headers and stdio environment entries can bind to Credential References. Provider persistence stores only References and formatting metadata. Shared authenticated MCP connections re-authorize every required Reference through the Invocation-supplied resolver before each Tool call.
- The real Runtime integration proves environment Credential -> authenticated HTTP Provider -> allowed Profile success, a Profile without the required Reference fails before upstream execution, and Catalog scope remains explicitly privileged.
- Credential deletion is blocked while referenced by Profile/Provider configuration. Audit schema inspection confirms no credential/secret columns, and Policy/runtime event contracts remain secret-free.
- Web/Desktop now expose a list-first Credential inventory, focused native-secret rotation, Profile Credential selectors, and a separate Provider Authentication dialog. Normal management never requires a raw Credential Reference input, and Provider Connection/Authentication dialogs do not overlap.
- CLI native secret creation/rotation accepts secret bytes only through stdin with `--secret-stdin`; there is no normal `--secret` flag. `profile add-tool --credential-ref` remains an advanced metadata-only CLI path, while normal Web/Desktop management uses inventory selectors.
- Legacy environment-based HTTP header templates remain executable for backward compatibility but are explicitly labeled a compatibility path; they are not represented as inventory-backed Profile Credential scope.

Phase 11 deliberately does not add per-call account switching for a shared Provider. Different authentication identities should continue to use distinct Provider configurations until a session-pool design is planned.

## Deferred follow-up

- automatic migration/removal of legacy environment header templates;
- Provider session pools keyed by Credential identity;
- cloud secret-manager backends;
- remote authenticated principals;
- editable global Policy rules over Credential metadata;
- secret expiry/rotation reminders;
- organization-managed secret stores.
