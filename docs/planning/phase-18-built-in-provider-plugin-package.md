# Phase 18 Plan — Built-in Provider Plugin Package

Status: Complete

## Goal

Consolidate Mender's three statically linked first-party Provider plugins into one explicit **Built-in Provider Plugin package**:

- `io.mender.provider.mcp`;
- `io.mender.provider.openapi`;
- `io.mender.provider.model-tools`.

The change is organizational and compositional. It must not change Provider identity, persisted configuration, Tool identity, Credential governance, execution behavior, or the external Provider plugin protocol.

## Context

Phases 13–15 introduced MCP, OpenAPI, and Model Tools as independent built-in Provider plugins. Phase 16 added installable external plugins and Phase 17 hardened their lifecycle.

The runtime behavior is already plugin-oriented, but built-in composition is still spread across the bootstrap root:

~~~go
providerPlugins := providerplugin.NewRegistry()
providerPlugins.Register(mcpplugin.New())
providerPlugins.Register(openapiplugin.New())
providerPlugins.Register(modeltoolsplugin.New())
~~~

This leaves the bootstrap package responsible for knowing every first-party plugin implementation and makes the phrase "built-in plugins" a management projection rather than an explicit composition package.

## Decision

Add a composition-only package:

~~~text
backend/internal/providerplugin/builtin/
~~~

It owns the statically linked built-in plugin set and registration order.

Conceptually:

~~~go
package builtin

func Plugins() []providerplugin.Plugin
func Register(*providerplugin.Registry) error
~~~

The package may import:

- `backend/internal/mcp/infrastructure/plugin`;
- `backend/internal/openapi/infrastructure/plugin`;
- `backend/internal/modeltools/infrastructure/plugin`;
- the generic `backend/internal/providerplugin` contract.

The generic `providerplugin` package itself must remain implementation-agnostic. It must not import MCP, OpenAPI, Model Tools, bootstrap, or external plugin packages.

## Dependency direction

~~~text
MCP plugin ───────────┐
OpenAPI plugin ───────┼──> providerplugin/builtin ──> providerplugin contracts
Model Tools plugin ───┘                 │
                                        ▼
                                  bootstrap Runtime

external plugins ───────────────────────> providerplugin contracts
~~~

The built-in package is a composition adapter, not a new domain/runtime abstraction.

## Package responsibilities

### Built-in package owns

- the exact set of statically linked first-party Provider plugins;
- deterministic plugin construction order;
- registration of that set into a supplied Registry;
- tests asserting stable IDs and descriptor validity;
- registration error context.

### Built-in package does not own

- Provider configuration persistence;
- active Provider Instance lifecycle;
- external installation resolution;
- plugin-specific configuration decoding;
- MCP/OpenAPI/model API protocol logic;
- management DTOs;
- frontend renderer selection.

Those responsibilities remain where they are today.

## Compatibility invariants

This refactor must preserve:

1. Plugin IDs:
   - `io.mender.provider.mcp`;
   - `io.mender.provider.openapi`;
   - `io.mender.provider.model-tools`.
2. Plugin descriptor versions and ConfigVersion values.
3. Existing `provider_configs` rows and legacy MCP migration behavior.
4. ProviderID, ToolID, SourceKey, Profile and Policy references.
5. External Provider installation semantics and exact InstallationID pinning.
6. Built-in management projection:
   - `sourceKind = builtin`;
   - no InstallationID;
   - enabled by construction.
7. Existing first-party Web/Desktop renderers.
8. No built-in conversion to subprocesses.

No SQLite, OpenAPI, protobuf, Wails, or SDK contract migration is expected.

## Implementation stages

### Stage A — Package seam

Create `backend/internal/providerplugin/builtin` with:

- `Plugins()`;
- `Register()`;
- focused unit tests.

At this stage Runtime remains unchanged. This proves the package independently before changing the composition root.

### Stage B — Runtime adoption

Replace direct MCP/OpenAPI/Model Tools imports and three registrations in `bootstrap.NewRuntime` with the built-in package registration entrypoint.

Add/adjust bootstrap tests so Runtime plugin discovery still exposes exactly the three expected built-in IDs.

### Stage C — Documentation and verification

Update development documentation to distinguish:

- the generic Provider Plugin Runtime;
- the built-in composition package;
- the external installable package format.

Run repository gates and record completion evidence.

## Checks

### Package-level

- built-in package returns exactly three plugins;
- IDs are stable and unique;
- descriptors validate;
- order is deterministic;
- `Register(nil)` fails clearly;
- registering into a fresh Registry exposes all three descriptors;
- duplicate registration fails rather than silently replacing a plugin.

### Runtime-level

- `NewRuntime` exposes MCP, OpenAPI, and Model Tools through `ProviderPluginDescriptors()`;
- each continues to project as `sourceKind=builtin`;
- external Resolver behavior remains unchanged.

### Repository gates

At minimum:

~~~bash
go test ./backend/internal/providerplugin/...
go test ./backend/internal/bootstrap/...
golangci-lint run ./backend/...
pnpm typecheck
pnpm test
pnpm build
~~~

Final completion should run the repository's broader `pnpm check` when environment cost permits.

## Non-goals

- moving plugin implementation source trees under `providerplugin/builtin`;
- merging the three plugins into one PluginID;
- inventing one shared config schema across unrelated plugins;
- changing first-party UI forms to generic schema rendering;
- changing external plugin package layout;
- converting built-ins into external/subprocess plugins;
- adding plugin enable/disable state for statically linked built-ins.

## Commit discipline

This phase is document-first and uses segmented commits:

1. planning/architecture document;
2. built-in package seam + package tests;
3. Runtime adoption + integration tests;
4. documentation completion/evidence.

Skill/workflow directories such as `.agents/`, `.pi/`, and `.intent-loop/` are excluded from all commits.

## Completion evidence

The implementation landed in segmented commits:

- `9db74c6` — document the Phase 18 architecture and migration boundary;
- `581f919` — add the composition-only `providerplugin/builtin` package and package tests;
- `df8199d` — make Runtime consume the built-in package instead of importing the three implementations directly;
- `774b09c` — repair an unrelated pre-existing Providers UI test wait condition exposed by the full repository gate.

The built-in package now constructs MCP, OpenAPI, and Model Tools in one deterministic first-party set. Runtime owns a generic Registry and performs one `providerbuiltin.Register(providerPlugins)` call. The generic `providerplugin` core remains free of implementation imports.

Verification completed:

~~~text
go test ./backend/internal/providerplugin/... ./backend/internal/bootstrap/...
golangci-lint run ./backend/...                         PASS
go test ./backend/...                                  PASS
go test -race ./backend/...                            PASS
Provider plugin public SDK lint/test/race              PASS
Provider plugin example lint/test/race/build           PASS
Go module isolation                                    PASS
OpenAPI TypeScript SDK regeneration check              PASS
Wails binding generation check                         PASS
Desktop architecture check                             PASS
ESLint                                                 PASS
workspace TypeScript                                   PASS
pnpm test                                              8 files / 56 tests PASS
pnpm build                                             Web/Desktop PASS
~~~

`pnpm check` was also attempted. Its first network-dependent step, `provider-plugin:proto:check`, reached `buf generate` and failed because the Buf remote plugin service reported `the server hosted at that remote is unavailable`. No generated protocol file changed. All remaining `pnpm check` stages were then executed separately and passed as listed above.

## Exit criteria

- [x] Built-in plugin composition has one explicit package.
- [x] Bootstrap no longer imports MCP/OpenAPI/Model Tools plugin implementations individually.
- [x] Generic `providerplugin` core remains implementation-agnostic.
- [x] Runtime still exposes the same three built-in PluginIDs.
- [x] Persisted Provider and Tool identities remain unchanged.
- [x] External plugin behavior remains unchanged.
- [x] Focused Go tests pass.
- [x] Repository lint/typecheck/test/build gates pass.
- [x] Development documentation describes the new package boundary.
