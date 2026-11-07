# Tool / Workspace / Sandbox hard cut — Stage C: governed placement, 2026-11-07

**Status:** Scoped Stage C implementation and tests completed; **not wired into the production Tool / Invocation path**. No user DB upgrade, Profile grant mutation, ToolID migration, remote provisioning, or live Agent execution. Stage D–F must publish a single atomic hard-cut release, not two running execution paths.

**Inputs:** [ADR-0064](../architecture/adr/0064-tool-provenance-and-execution-placement-hardcut.md), [Stage B](tool-hardcut-stage-b-2026-11-07.md), [migration contract](tool-hardcut-phase-a-migration-contract-2026-11-07.md).

## 1. Implementation and ownership

| Layer | Stage C code | Responsibility |
| --- | --- | --- |
| Workspace application | `context_selection.go` | Read-only approved Workspace choice: explicit identity → approved default → unique eligible; none, revoked default, untrusted identity or ambiguity fail closed |
| Workspace application | `placement_selection.go` | Read-only candidate evaluation; pinned target never falls back to another Sandbox; unique candidate required when no target is pinned |
| Workspace infrastructure | `sandboxbridge/guarded_placement.go` | Trusted native ToolID/capability matching, exact Profile target-scope and Workspace-grant checks, sandbox read-only Resolve/Backend capability/isolation/ready checks; freeze run/spec/root and exact Policy facts |
| Workspace infrastructure | `sandboxbridge/local_root_attestor.go` | Local-only root attestation using actual filesystem observations; rejects noncanonical/symlink/absent roots and all remote/container guesses |
| Workspace infrastructure | `sandboxbridge/bridge.go` | Reuse original typed Workspace lease wrapper so SSH/external argument/result constraints and cleanup authority are unchanged |
| Sandbox application/infrastructure | existing `ExecutionRouter.Resolve/Acquire` | Read-only placement discovery followed by `sandbox.acquire` Policy, grants, exact backend pin, generation CAS, durable lease and cleanup |

Only Mender's registered **native** workspace ToolIDs from Stage B are eligible for the guarded route. A Provider ToolID, Agent ToolID, fake ToolID or unsupported operation never resolves through a Sandbox. Remote MCP/OpenAPI/Model Tools remain Provider executions. Agent as Tool remains in the separate Agent Task/Session/ACP v1 service, not the Workspace router.

The current `Runtime` uses its already-supported Provider-backed Tool facades. **It does not construct or invoke `GuardedRouter`**. This avoids publishing a new route while Profile TargetScope persistence/permission guards and Catalog SourceRef migration are incomplete.

## 2. Policy, authority, timing, and anti-TOCTOU

The Stage C flow is:

`Trusted Profile/Owner context + registered native ToolID → authorized Workspace selection → known placement candidates → exact Profile TargetScope + Workspace authority + Sandbox Profile grant (or actual local owner) → matching capability, versioned backend, required isolation and verified root → unique/pinned choice → frozen Sandbox spec/run generation + known workspace_id/sandbox_id/effect facts → canonical Invocation Policy/Approval/Budget/ToolBinding → Stage C verified Invocation receipt → repeat current Workspace/Binding/grants/root/capability/backend/run checks → existing Sandbox Router.Acquire → exact lease → typed operation/release → Invocation terminal guard/audit`.

Key invariants:

1. **Identity is not authority.** A well-formed `wrk_` or `sbx_` ID alone grants nothing. Workspace Inventory, Profile ToolBinding/TargetScope and Sandbox grants are independently checked and refreshed.
2. **No implicit default selection.** An explicit WorkspaceRef must be visible; a host-selected default must still be granted; only one eligible Workspace or Sandbox may be auto-selected. Ambiguity fails closed.
3. **No downgrade, fallback or side-effecting preflight.** A pinned Sandbox that is stopped, missing, denied, unsupported or different isolation fails. Read-only `Resolve`, registered `ResolveBackend` and local root observations do not launch a process, mount a directory, reserve a lease or resolve a Credential.
4. **Exact facts and real admission.** The snapshot emits known `workspace_id`, `sandbox_id` and filesystem/process effect facts. At the later Invocation cutover, the trusted argument extractor **must retain normalized `workspace_path` facts** (and any existing argument-related restrictions) and merge them with the selected target facts before Policy; contradictory/duplicate identities fail closed. Using only the new target facts would silently bypass path-specific Policy. Acquire rejects mismatches, duplicates, unknown facts or wrong effects; **matching facts alone are not approval**. A required `InvocationAdmissionVerifier` must confirm an already admitted, valid InvocationID for the same Tool, caller and facts. It is a consuming proof of the *existing* Invocation pipeline, not an independent Policy engine.
5. **Review and execution must be identical.** Acquire reads Workspace ownership and authorized Binding TargetScope again, verifies exact candidate/root, Backend pin/isolation/capability and Sandbox run/generation, then passes the original placement to the existing Router, which independently revalidates status/grants under the durable lease.
6. **Lease integrity.** A lease from a different Sandbox/generation is rejected and closed. Typed Workspace operations continue releasing the original lease once; cleanup failures remain authoritative.
7. **Root evidence is backend-specific.** Only Local Workspace is attested in this stage; Docker/SSH/Daytona/External need explicit trusted target-side proof. Two similar paths are not proof of an equivalent workspace or mount.
8. **Agent as Tool is not a Sandbox.** Agent workspace grants and Task/Session/native permission/resume remain owned by the Agent context. Stage C does not launch ACP processes or reinterpret Agent Task execution as Workspace shell commands.

### Deliberate nonproduction requirements

Stage C only defines the interfaces and a tested adapter; it does **not** create persistent Workspace inventory/placement, Profile TargetScope store, or an InvocationAdmissionVerifier implementation. Production wiring must wait for:

- Stage D/E: Catalog SourceRef, exact native Tool registration, Workspace identity/placement persistence, deterministic migration of old facade ToolIDs and original target-limited Profile scopes.
- Stage F: a canonical Invocation-owned receipt/proof flow with the existing preflight/Policy/Approval/budget rather than a fabricated caller token, plus SDK/Wails/MCP/CLI contract parity.
- A **single** durable commit-point enforcement for Profile TargetScope/Workspace-grant changes concurrent with execution admission. Stage C's double-check is necessary but **not an atomic substitute** for this forthcoming persistence guard; existing Sandbox grant reservation already has its own atomic fence.
- Verified root attestors for additional backends. Nonlocal Workspace placement remains unavailable until an actual driver/source proves root ownership and its sandbox grants.

No compatibility bridge, new generic execution engine, speculative browser session, Agent Loop, ACP v2 adapter or Host fallback is introduced.

## 3. Focused acceptance tests

- `context_selection_test.go`: explicit/default/sole Workspaces, revoked defaults, missing/malformed/duplicate/ambiguous list and canceled/unauthenticated caller; no names-based inference.
- `placement_selection_test.go`: single eligible target, ambiguous target, denied pinned target with another eligible, unregistered pinned ID, invalid operation and inspection errors.
- `guarded_placement_test.go`: **real Sandbox Router** with fixture Sandbox store, owner/Profile grants, Backend Resolver and lifecycle. Tests frozen facts, exact lease and release, fake ToolIDs, ACL revocation, run generation changes, failed backend availability, required isolation, pinned-target no fallback, missing Invocation proof and mismatched admitted facts.
- Simulated revoke inside the Router's `sandbox.acquire` admission window verifies that the lease Store's last-word grant check prevents execution after concurrent revocation.
- `local_root_attestor_test.go`: actual temporary local root, different/missing/symlink targets, remote backend fail-closed, cancellation.
- Existing `sandboxbridge/bridge_test.go` and Workspace plugin tests prove legacy typed request/session semantics are unchanged.

## 4. Verification and release gate

Stage C implementation commit: `aa1a001` (`refactor(runtime): add guarded workspace execution placement`).

- **Passed:** `go test -count=1 ./backend/internal/workspace/... ./backend/internal/sandbox/application ./backend/internal/sandbox/infrastructure/router`.
- **Passed:** `go test -race -count=1 ./backend/internal/workspace/... ./backend/internal/sandbox/application ./backend/internal/sandbox/infrastructure/router ./backend/internal/catalog/domain ./backend/internal/profile/domain ./backend/internal/toolsuite/infrastructure/curated`.
- **Passed:** `go run scripts/check-backend-architecture.go` and `golangci-lint run ./backend/...` (0 issues).
- **Passed:** `pnpm lint && pnpm typecheck && pnpm test && pnpm build`; i18n 1639/1639, Vitest 76/76 files and 512/512 tests, Web/Desktop frontend builds passed. Only existing Vite large-bundle warnings remain.
- **Evidence retained:** bounded full-backend Race test `go test -race -count=1 -parallel=2 -timeout=15m ./backend/...` in `/tmp/mender-stage-c-20261107-race.log` with exit receipt `/tmp/mender-stage-c-20261107-race.exit`. Treat its result separately from canonical `pnpm check`.
- **Historical gate:** [Stage B](tool-hardcut-stage-b-2026-11-07.md) documented canonical `pnpm check` exit 1 from the default 10-minute Bootstrap full-parallel Race timeout. Focused bounded rerun passed; this **does not** make the canonical full gate green.

Stage C remains nonproduction-wired. No test in this phase is evidence of actual Profile TargetScope persistence, full client contract parity, Docker/SSH root attestation, live Agent execution, production database migration or Wails-native GUI acceptance.

**Next Stage D risk:** remove the mandatory Provider ancestry from the canonical Catalog atomically with native registration, resource provenance and Profile-source projections; never expose Stage C GuardedRouter to arbitrary callers without the real Invocation-issued admission verifier and TargetScope persistence.