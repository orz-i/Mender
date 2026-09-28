# ADR-0028: Make Provider plugin resilience and governance host-owned

- Status: Accepted
- Date: 2026-09-28

## Context

ADR-0024 made Provider plugins the Runtime boundary and ADR-0027 added installable out-of-process Provider plugins. Phase 16 proved that an independently built plugin can be installed, configured, synchronized, invoked, disabled, re-enabled, and removed without adding its PluginID to Mender source code.

Operational review found a second-order problem: several host control-plane operations still depend on third-party plugin processes being alive and responsive.

Today:

- Provider instance reuse checks configuration identity but not external process/session liveness.
- A crashed process can leave a Provider routed to a stale Instance until a manual reload or lifecycle event replaces it.
- Listing selectable external plugins resolves runtime descriptors by starting plugin processes; one broken enabled installation can fail the complete plugin catalog.
- Credential deletion usage checks call plugin InspectConfig, coupling Mender-owned referential integrity to third-party availability.
- ValidateConfig, InspectConfig, Open, and ListTools inherit caller deadlines but have no host-owned upper bound.
- External Providers pin exact InstallationID, but no explicit ProviderID-preserving operation moves a Provider to another installation of the same PluginID.
- Protocol metadata such as connection_summary is validated but not retained by the internal metadata projection.

Crash containment is therefore present, but lifecycle recovery and control-plane isolation are incomplete.

## Decision

Mender will make Provider plugin resilience, verified descriptor metadata, normalized configuration metadata, bounded RPC deadlines, and installation rebinding host-owned Runtime concerns.

### Health-aware instance reuse

Provider instance reuse is no longer determined only by PluginID, InstallationID, ConfigVersion, and config revision.

The generic Provider Instance Manager supports an optional host-facing reuse capability. Plugins that do not implement it keep existing behavior. External instances implement it by checking local closed state and process liveness without issuing an RPC health call.

If persisted configuration matches but the active instance is not reusable, Ensure opens a replacement, swaps executor routing, and closes the stale instance.

A stale external instance whose process already exited closes idempotently without requiring an RPC to the dead process.

Recovery is request-driven. Mender does not automatically replay the invocation that observed the crash.

### Host-owned RPC upper bounds

External plugin control-plane RPCs receive host-owned maximum durations even when callers provide no deadline:

- GetDescriptor
- ValidateConfig
- InspectConfig
- Open
- ListTools
- Close

The effective deadline is the earlier of caller deadline and host maximum. ExecuteTool continues to use the governed Invocation deadline.

### Verified descriptor snapshot

Successful external installation already launches and verifies the managed executable, protocol negotiation, runtime descriptor identity, and descriptor/schema bounds. Phase 17 persists that verified descriptor snapshot with the installation.

The snapshot contains management-safe fields required to render/select a plugin without starting it:

- ProtocolVersion
- ConfigVersion
- DisplayName
- Description
- ConfigSchema
- UISchema
- Capabilities

Plugin catalog listing reads the snapshot. Normal plugin discovery must not execute third-party code after verification.

Legacy Phase 16 installation rows without snapshots may be backfilled once, but one failed backfill must remain installation-scoped rather than hiding unrelated plugins.

### Persist normalized configuration metadata

After plugin configuration passes validation and inspection, Mender persists bounded host-owned metadata tied to that exact config revision:

- CredentialRefs
- ConnectionSummary

Rules:

1. Credential References are validated, de-duplicated, sorted, and never contain resolved secret bytes.
2. External refs retain Protocol v1's requirement that returned refs occur as exact strings in persisted config.
3. Metadata is revision-consistent with Provider config.
4. Credential usage/deletion checks read persisted metadata and do not execute plugin code.
5. Connection summary is untrusted display metadata only, bounded by the host.
6. Built-in and external plugins use the same normalized metadata representation.

### Administrative status and runtime health are distinct

Installation administrative status remains enabled, disabled, or invalid. Runtime process/session health is separate.

A transient process crash does not rewrite administrative state. An enabled installation may be stopped, healthy, or degraded at runtime.

Phase 17 does not add periodic health polling.

### Explicit installation rebind

Exact InstallationID pinning remains mandatory. Moving an existing external Provider to another installation is an explicit operation preserving canonical Provider identity.

A rebind requires:

- current Provider is external;
- target installation exists, is enabled, and verified;
- target PluginID equals the current PluginID;
- target config validates under the target ConfigVersion;
- replacement instance can open and synchronize before the old pin is retired.

A successful rebind preserves ProviderID, namespace, stable ToolIDs for unchanged SourceKeys, Profile ToolBindings, Policy Provider selectors, and Audit identity.

Mender never follows a newer installation automatically.

## Security invariants

1. Every Tool invocation still enters the shared governed Invocation pipeline.
2. Recovery never retries the invocation that may already have started side effects.
3. Secret values remain ephemeral and are never persisted in descriptor/config metadata.
4. Credential reference governance does not require executing third-party code after normalized metadata exists.
5. Hung control-plane RPCs are bounded by host deadlines.
6. Dead external sessions are not reusable solely because config is unchanged.
7. Normal plugin catalog listing is side-effect free after verified descriptor persistence.
8. Rebind never changes PluginID or ProviderID implicitly and never selects latest automatically.
9. Package digest and executable checksum verification remain mandatory before launch.
10. Native external plugins remain trusted local executables, not OS-sandboxed code.

## Alternatives considered

### Require manual Sync after every crash

Rejected because request-driven replacement can recover a later call safely without replaying the failed call.

### Add an always-on restart loop

Rejected for this phase because it executes third-party code in the background, adds backoff/restart-storm policy, and complicates shutdown. Request-driven recovery is sufficient.

### Re-run InspectConfig for Credential usage checks

Rejected because Mender-owned referential integrity would remain dependent on third-party process availability.

### Use manifest metadata instead of a verified runtime descriptor snapshot

Rejected because the runtime descriptor is the verified protocol/config/UI contract and installation already validates it.

### Automatically retarget to the newest installation

Rejected because it breaks exact package pinning and reproducibility.

## Consequences

Positive:

- later governed calls can recover after an external process crash;
- one broken plugin no longer takes down the complete management plugin catalog;
- Credential inventory governance remains available while plugins are disabled/unavailable;
- control-plane hangs are bounded;
- connection summaries can be displayed without plugin activation;
- exact pinning gains an identity-preserving upgrade path.

Trade-offs:

- installation and Provider config stores gain forward-only migrations;
- normalized metadata must stay revision-consistent;
- legacy installations need snapshot backfill behavior;
- rebind requires compensation/ordering tests;
- health-aware replacement adds lifecycle branches.

## Follow-up

Phase 17 is implemented in segmented commits:

1. architecture and plan;
2. health-aware stale-instance recovery;
3. host-owned control-plane RPC deadlines;
4. verified descriptor snapshot persistence and side-effect-free plugin discovery;
5. normalized Provider config metadata and offline Credential governance;
6. management connection-summary/runtime projection;
7. explicit same-PluginID installation rebind;
8. end-to-end lifecycle/migration/rebind hardening;
9. documentation and repository/security verification.
