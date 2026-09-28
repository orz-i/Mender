# Phase 17 Plan — Provider Plugin Lifecycle & Runtime Resilience

Status: In Progress

## Objective

Make the installable Provider plugin platform resilient enough that a third-party plugin can crash, hang, be disabled, or become unavailable without taking Mender's control plane or persistent governance metadata with it.

ADR-0028 defines the architecture.

Target lifecycle:

~~~text
install + verify
    -> persist verified descriptor
    -> configure + inspect
    -> persist normalized host metadata
    -> open/reuse Provider session
    -> crash/hang
    -> fail current operation
    -> later request detects stale instance
    -> restart package process
    -> reopen Provider session
    -> continue under the same ProviderID
~~~

## Baseline

Phase 16 already provides native out-of-process plugins, exact InstallationID pinning, immutable package digests, executable checksums, protocol negotiation, AutoMTLS transport, reduced inherited environment, shared package processes with Provider sessions, canonical Tool discovery/execution, scoped Credential handoff, generic management UI, and a public Go SDK/example.

The post-Phase-16 review identified six gaps:

1. Manager reuses a config-identical Instance even after its external process exits.
2. Plugin catalog listing can execute every enabled plugin and fail globally on one broken installation.
3. ValidateConfig/InspectConfig/Open/ListTools lack host-owned maximum deadlines.
4. Credential usage checks execute plugin InspectConfig instead of persisted host metadata.
5. connection_summary is discarded by the internal projection.
6. exact installation pinning has no ProviderID-preserving rebind path.

## Scope

Phase 17 includes:

- optional Instance reuse/health capability;
- request-driven recovery after external process exit;
- stale-session close semantics that tolerate a dead process;
- bounded host deadlines for external control-plane RPCs;
- verified descriptor snapshot persistence;
- side-effect-free Provider plugin discovery;
- item-scoped degraded behavior for legacy/unavailable installations;
- persisted normalized Provider metadata:
  - Credential References;
  - connection summary;
- offline Credential usage/deletion checks;
- safe management connection-summary projection;
- explicit same-PluginID installation rebind;
- ProviderID/SourceKey/ToolID preservation tests;
- forward-only migrations and downgrade fail-closed coverage;
- lifecycle/race/security regression coverage.

## Non-goals

Phase 17 does not include automatic replay of a failed Tool invocation, background restart loops, periodic health polling, automatic update discovery, automatic latest retargeting, OCI registries, Sigstore/Cosign, revocation feeds, native OS sandboxing, WASM Component execution, arbitrary plugin frontend code, or conversion of built-in plugins to subprocesses.

## Work packages

### 17.1 Architecture and planning

Deliver ADR-0028, this plan, and the roadmap entry.

Commit:

~~~text
docs: plan provider plugin resilience phase
~~~

### 17.2 Health-aware Provider instance reuse

Introduce an optional internal capability:

~~~go
type ReusableInstance interface {
    Reusable() bool
}
~~~

Rules:

- instances without the capability keep existing reuse behavior;
- external instances return false when closed, missing a process, or the process exited;
- Manager replaces a same-revision but non-reusable instance;
- stale external Close does not require a dead process to answer;
- the failed invocation is never replayed;
- a later normal Runtime call reopens the process/session.

Tests cover healthy reuse, same-revision replacement, crash then later recovery, route replacement, and race behavior.

Commit:

~~~text
fix(provider): recover stale external plugin instances
~~~

### 17.3 Host-owned external RPC deadlines

Bound ValidateConfig, InspectConfig, Open, and ListTools. Keep existing startup/GetDescriptor/Close limits and governed Invocation timeout for ExecuteTool.

The earlier caller deadline wins.

Tests include a helper plugin that stays alive but stops replying to each control-plane operation.

Commit:

~~~text
fix(provider): bound external plugin control-plane RPCs
~~~

### 17.4 Verified descriptor snapshots

Persist the verified runtime descriptor with installation metadata:

- ProtocolVersion;
- ConfigVersion;
- DisplayName;
- Description;
- ConfigSchema;
- UISchema;
- Capabilities.

Fresh installation verifies then persists the snapshot. Normal plugin catalog listing reads the snapshot and does not launch a process.

Legacy installation rows may be lazily backfilled; failure is scoped to that installation rather than the full plugin catalog.

Tests cover fresh persistence, no process start during listing, isolation of a broken installation, v1-to-v2 migration, and future-schema fail closed.

Commit:

~~~text
feat(provider): persist verified plugin descriptors
~~~

### 17.5 Persist normalized Provider config metadata

Extend generic Provider configuration persistence with metadata tied to the exact config revision:

~~~text
CredentialRefs
ConnectionSummary
~~~

Runtime create/update paths become:

~~~text
ValidateConfig
-> InspectConfig
-> normalize host metadata
-> persist config + metadata
~~~

Requirements:

- no secret persistence;
- refs canonicalized/de-duplicated/sorted;
- connection summary UTF-8 and bounded;
- Credential usage checking becomes storage-only;
- compensation restores config and metadata consistently.

Tests prove Credential deletion is still blocked while an installation is disabled/unavailable and connection summary round-trips.

Commit:

~~~text
feat(provider): persist normalized plugin config metadata
~~~

### 17.6 Management projection

Expose persisted connection summary and safe availability information without executing third-party code.

Administrative status and runtime health remain distinct. No background health polling is added.

Commit:

~~~text
feat(app): surface provider plugin runtime metadata
~~~

### 17.7 Explicit installation rebind

Add an explicit Runtime operation with:

~~~text
ProviderID
Target InstallationID
Target config JSON
~~~

Validation:

- Provider currently uses an external installation;
- target exists/enabled/verified;
- target PluginID equals current PluginID;
- target config validates;
- replacement instance opens;
- Tool synchronization succeeds.

Successful rebind preserves ProviderID, namespace, Profile/Policy references, and SourceKey-stable ToolIDs.

Expose through generic CLI/OpenAPI/Wails/Web/Desktop surfaces without PluginID-specific branches.

Commits:

~~~text
feat(provider): rebind providers to plugin installations
feat(adapters): expose provider plugin rebind
feat(app): manage provider plugin upgrades
~~~

### 17.8 End-to-end hardening

Prove:

- crash -> failed operation -> later recovery;
- hung management RPC bounded;
- plugin list remains usable with one unavailable installation;
- Credential usage is offline;
- disable tears down sessions/process;
- re-enable stays lazy;
- rebind preserves ProviderID and stable ToolIDs;
- failed rebind preserves old pin/route;
- Runtime shutdown reaps restarted processes.

Commit:

~~~text
test(provider): harden resilient plugin lifecycle
~~~

### 17.9 Documentation and verification

Update development, CLI/Web/Desktop, migration, and roadmap docs.

Quality gates:

~~~bash
pnpm provider-plugin:proto:check
pnpm provider-plugin:sdk:lint
pnpm provider-plugin:sdk:test
pnpm provider-plugin:sdk:test:race
pnpm provider-plugin:example:lint
pnpm provider-plugin:example:test
pnpm provider-plugin:example:test:race
pnpm provider-plugin:example:build
pnpm check
pnpm security:check
~~~

Commit:

~~~text
docs: complete provider plugin resilience phase
~~~

## Exit criteria

Phase 17 is complete when:

1. a crashed external process does not require manual Sync before a later governed call can reopen it;
2. the failed invocation is never replayed automatically;
3. control-plane plugin RPCs have host-owned maximum durations;
4. plugin catalog listing is side-effect free for verified installations;
5. one broken external installation cannot hide healthy plugins;
6. Credential deletion/reference governance works without executing plugin code;
7. normalized metadata contains no secrets and is revision-consistent;
8. management can display external connection summaries without plugin activation;
9. a Provider can explicitly move to another installation of the same PluginID without changing ProviderID;
10. SourceKey-stable Tools preserve ToolID across successful rebind;
11. upgrades remain explicit and never follow latest automatically;
12. migrations remain forward-only/fail-closed;
13. repository, race, build, and security checks are green.

## Progress

- [x] Architecture decision recorded in ADR-0028.
- [x] Phase plan created.
- [ ] Health-aware Provider instance recovery.
- [ ] Host-owned control-plane RPC deadlines.
- [ ] Verified descriptor snapshot persistence.
- [ ] Offline normalized config metadata/Credential governance.
- [ ] Management runtime metadata projection.
- [ ] Explicit installation rebind.
- [ ] End-to-end lifecycle hardening.
- [ ] Documentation/verification closeout.
