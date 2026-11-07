# Stage B — Tool Provenance, Workspace References and Target Scope (2026-11-07)

**Status:** Domain/type implementation completed; no production cutover, no schema change, no privileged runtime action.

**Architecture:** [ADR-0064](../architecture/adr/0064-tool-provenance-and-execution-placement-hardcut.md)

**Phase A migration contract:** [Tool hard-cut assessment](tool-hardcut-phase-a-migration-contract-2026-11-07.md)

## Implemented

1. `backend/internal/catalog/domain/source_ref.go` introduces immutable, strict `SourceRef(kind,id)`. `provider` accepts only validated ProviderID, `agent` accepts only canonical AgentID syntax, `native` admits only the two reviewed host keys `workspace` and `jobs`. The source ID is *not* an authorization. `Tool.SourceRef()` projects existing persisted Provider-backed Tool identity as a provider SourceRef **without** enabling Agent/native runtime Tool construction.
2. `backend/internal/workspace/domain/reference.go` provides a stable, opaque Workspace `wrk_<32 lowercase hexadecimal>` identity. It contains no host directory, secret, owner grant, driver or false claim that a Workspace is mounted in a Sandbox.
3. `backend/internal/workspace/application/placement_query.go` adds a consumer-owned, read-only candidate port and strict identity validation. It accepts only the exact requested Workspace, canonical Sandbox ID syntax, non-empty backend-specific root, distinct targets and deterministic ordering. This does *not* select a target, check permissions or call a Sandbox driver. The present implementation has no production read store/route connected to this interface.
4. `backend/internal/profile/domain/target_scope.go` supplies a restrictive `ExecutionTarget{WorkspaceRef,SandboxID}` and immutable `TargetScope`. Empty/zero scope denies access; wildcard, malformed and duplicate targets are invalid. `Allows` requires an **exact** workspace/sandbox pair; `IsSubsetOf` can be used by a later migration to check no authority expansion. A jobs-only target may omit WorkspaceRef. Stage B deliberately does **not** accept a TargetScope through the current ToolBinding API because SQLite/API/Invocation cannot yet persist and enforce it; allowing an unsaved scope would be unsafe.
5. `backend/internal/toolsuite/infrastructure/curated/native_tools.go` reserves exactly nine immutable future native operation identities:

| Source | Operation | Reserved ToolID |
| --- | --- | --- |
| `native:workspace` | `workspace.read` | `tool_882fb9c25db58744f34da684f7f7ab5f` |
| `native:workspace` | `workspace.search` | `tool_33a65e204e96265604efddd7536eabd9` |
| `native:workspace` | `workspace.edit` | `tool_4ba8e255e6486aa5a425f8e87b348ded` |
| `native:workspace` | `workspace.shell` | `tool_1f28bd14ee01b125dcda89402b892ca9` |
| `native:jobs` | `jobs.submit` | `tool_3c766860cad00c990eea717f7189fb57` |
| `native:jobs` | `jobs.status` | `tool_5a5206f3704bb3b15f73493f91556d64` |
| `native:jobs` | `jobs.logs` | `tool_aaf99df8c4418467e4d28dc5b5986600` |
| `native:jobs` | `jobs.result` | `tool_19b4bffd11581c24979594e5bd319497` |
| `native:jobs` | `jobs.cancel` | `tool_2829fcf2f091f0566c5579b4d22bd70f` |

The constants are the first 128 bits of `SHA256("io.mender.native.tool.v1/"+operation)` encoded as an opaque ToolID. They are **reserved for a future migration, not executable, registered or exposed in MCP**. Their lack of collision with the *actual user database* has not been tested and **must be checked by the Stage E dry-run before write**. Existing per-Sandbox ToolIDs remain unchanged until the atomic cutover. Both suites' capability requirements are mapped to the existing reviewed Host registry; Agent-as-Tool ToolIDs remain Agent-specific and are not part of this native registry.

## Boundaries intentionally preserved

- Runtime still persists ProviderID for all Tool records, exposes existing Provider-based Catalog/MCP/HTTP/CLI/Wails routes and uses the same Invocations, Profiles, Agent/Sandbox facades, lease and authority gates as before this commit.
- Workspace and execution target candidate identities are **syntax and read-model values**; not proof of directory ownership, mount correctness, authorization, capability, isolation or lifecycle readiness. Stage C must verify those at preflight and acquisition, including TOCTOU and grants.
- `TargetScope` is not yet a Profile persisted property or new source of authority. Wiring it before the Profile SQLite/API/Invocation can roundtrip and enforce would silently drop restrictions, so it remains a pure, tested value object.
- No untrusted Plugin can choose reserved native Tool identities; Registry inventory is Host-owned. Browser Session remains unavailable.
- No new generic scheduler, auto-grant, auto-provision, Host fallback or ACP v2 bridge; Agent as Tool remains responsible for its own Task/Session and Agent Loop.

## Verification

- [x] Focused `go test`: Catalog Domain; curated Tool Suite Registry; Workspace Domain/Application; Profile Domain.
- [x] Full ordinary `go test ./backend/...` and backend `golangci-lint`: passed, 0 lint issues.
- [x] `go test -race ./backend/internal/catalog/... ./backend/internal/profile/... ./backend/internal/workspace/... ./backend/internal/toolsuite/...`: all relevant packages passed (including Catalog SQLite and Profile SQLite).
- [x] `go run scripts/check-backend-architecture.go`, SDK generation parity, Wails binding parity and Desktop architecture checks: passed.
- [x] Separately completed `pnpm lint && pnpm typecheck && pnpm test && pnpm build`: i18n 1639/1639; Vitest 76 files / 512 tests; Web/Desktop frontend builds passed with pre-existing large-chunk warnings.
- [x] Focused rerun of **all 10 Bootstrap tests still running at the full-suite timeout**, using `go test -race -count=1 -parallel=2 -timeout=4m ./backend/internal/bootstrap -run <10-test-regex>`: passed in 25.441s, exit 0.
- [ ] **Full `pnpm check` remains red (exit 1)**: `go test -race ./backend/...` caused `backend/internal/bootstrap` to panic with `test timed out after 10m0s`, listing 10 concurrent tests. No confirmed race report or compilation failure was observed in that log. Bounded rerun success suggests full-suite scheduling/parallel-load sensitivity, **not proof** that the canonical full gate is green. Retain evidence in `/tmp/mender-tool-hardcut-stage-b-20261107-check.log` and `/tmp/mender-tool-hardcut-stage-b-bootstrap-race-focused.log`; investigate/verify a bounded full-suite runner separately before hard-cut release.
- [ ] Disposable-database source/ID/ACL migration assessment (Stage E).
- [ ] Real Wails and MCP client end-to-end acceptance after the hard cut (Stages G–H).

## Stage C entry requirements

1. Implement a trusted, side-effect-free **Workspace/Sandbox placement selection** over these contracts. Do not infer a Sandbox from a Provider facade, raw Tool argument or matching root name.
2. Add exact authorized target facts to Invocation *before* Policy/Approval; freeze target plus verified backend/run generation; revalidate and Acquire on the same target only.
3. Keep Agent Task/Session/ACP v1 separate from the generic Workspace executor. Agent tool supply into Mender is per-invocation and governed.
4. In Stages D–F, ship new Catalog SourceRef persistence, Profile TargetScope persistence, URI/SDK/HTTP/Wails/MCP and permission checks **together**, not as a version where TargetScope is silently accepted but ignored.
5. Stage E must check all reserved ToolIDs for collision and produce a dry-run complete mapping before any production SQLite migration.
