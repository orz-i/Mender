# Mender Tool ownership hard cut — Stage E offline assessment and candidate migration (2026-11-07)

**Status:** Read-only dry-run, consistency backup, and **restricted offline candidate migration** implemented and tested. **NOT production cutover; not all nonempty databases can be migrated.** Stage F must finish cross-context authorization/Tool delivery/Provider-runtime rewrite before any new Runtime starts on a migrated database.

**References:** [ADR-0064](../architecture/adr/0064-tool-provenance-and-execution-placement-hardcut.md), [Phase A mapping and safety contract](tool-hardcut-phase-a-migration-contract-2026-11-07.md), [Stage D typed Catalog](tool-hardcut-stage-d-2026-11-07.md).

## 1. Delivered code and operator CLI

| Implementation | Behavior |
| --- | --- |
| `backend/internal/migration/toolhardcut/assess.go`, `fingerprint.go` | Open existing SQLite **read-only**, assess in a consistent transaction, require known schema versions and core legacy tables, run SQLite checks; SHA-256 fingerprint **all SQLite schema and table rows** including grants, audit, Agent sessions and FTS shadow tables, never export raw sensitive values |
| `report.go`, `inventory.go`, `bindings.go`, `attest.go` | Deterministic JSON report with sorted old→new IDs, verified resource ownership, Profile TargetScope diffs, Policy/Source references, MCP delivery changes, Agent Task and SSH recovery evidence, zero-authority-expansion blocking codes |
| `backup.go` | Online SQLite Backup API (including WAL), destination `O_EXCL`/0600, checkpoint independent backup to one-file rollback artifact, verify semantic fingerprint and manifest; never copy live `.sqlite` bytes with naive `cp` |
| `stage_copy.go`, `target_schema.go`, `workspace_conversion.go` | Require exact reviewed digest and report `status=ready`, recheck inside destination transaction, rebuild typed Catalog/FTS, preserve exact Agent ToolIDs, retire pinned internal facades, register 9 fixed native Tools, copy audit-only legacy ID mappings; create verified local Workspace placements, SQL native/Provider/Agent ownership and Profile/Policy reference guards. Commit **only a new candidate file**. Failures roll back and remove incomplete candidate, preserving `.preupgrade` backup |
| `backend/cmd/mender-tool-migrate` | `assess` and `stage-copy` only. There is intentionally **no** `apply`, `restore-live`, `swap` or background deployment command |

Run from the repository root; commands shown for **an expressly chosen, isolated/test DB**:

```bash
go run ./backend/cmd/mender-tool-migrate assess --db /path/to/test.sqlite --format json

go run ./backend/cmd/mender-tool-migrate stage-copy \
  --db /path/to/test.sqlite \
  --destination /path/to/candidate.sqlite \
  --manifest 'sha256:<exact-report-manifestDigest>'
```

The `assess` command outputs JSON and returns 0 only for `ready`; `blocked` or `requires_review` returns exit code 3 with full assessment JSON. Invalid options return 2, read/backup errors return 1. `stage-copy` returns a candidate file plus `.preupgrade` backup and JSON evidence **only if** the assessed digest exactly matches and every automated safety condition is proven. Any unresolved decision fails closed. No CLI command writes the supplied source DB.

## 2. Migration shape and confirmed invariants

- **External Provider Tool:** original ProviderID, ToolID and Tool schemas remain the exact external source; no fake sandbox or privilege expansion.
- **Agent as Tool:** one pinned Agent facade moves to `SourceRef(agent, AgentID)`. All five `delegate/status/result/cancel/resume` ToolIDs remain unchanged where the original 1:1 mapping is complete; Agent Task/Session, grant, approval and historical evidence stays with the Agent. If Agent MCP Profile exposure changes its source namespace, **even with unchanged ToolID**, `requires_review` blocks automatic staging.
- **Local Sandbox Workspace Tools:** a verified native `local_workspace` Sandbox with canonical, already existing host root and valid owner can be staged, preserving precise default/secondary logical Workspace refs and Sandbox bindings. Four per-Sandbox ToolIDs map to four pinned global `native:workspace` operations, recorded only as audit-only old→new IDs. No Profile grants are fabricated; no new environment is chosen automatically.
- **Native SSH Jobs:** new `native:jobs` Tool definitions are installed as fixed reviewed metadata, but a populated legacy SSH Jobs facade is **blocked** until Stage F can prove backend/job ownership, status/recovery/cancel and permission equivalence. Uncertain Job result is never replayed. **Installing a native Tool record is not enabling its execution.**
- **Profile ToolBinding:** distinct legacy ToolIDs merging into one native ID cannot silently union permissions or change aliases/approval/concurrency/credentials/Direct delivery. Conflicting restrictions generate `BINDING_CONSTRAINT_COLLISION`. Even one otherwise safe native Profile remap emits a `MCPExportDiff` and is `requires_review` until the authoritative Stage F external name/TargetScope contract and owner review are available.
- **Policy:** references to removed internal Provider IDs or remapped ToolIDs in `policy_rules.match_json` block instead of dropping deny/approval conditions. No policy is rewritten without semantic proof.
- **Workspace:** local root attestation is read-only and verifies canonical absolute directory, owner and accepted lifecycle state; no Docker/SSH/remote content equivalence inferred from path spelling. Existing Agent workspace aliases/root grants require independent approval and currently block automatic migration.
- **Task/lease:** pending or `unknown` Agent Tasks, active/uncertain Sandbox leases or SSH recovery obligations block migration, without auto-completion, resume, cancel or retry.
- **Audit and schema:** typed tools use `(source_kind,source_id,source_key)` and fixed ToolIDs. Old Resource Provider rows are retired only when pinned, and historical ToolIDs persist in `hardcut_legacy_tool_identities` for **read-only interpretation**, never for runtime Invoke fallback.

### Future-SQL guards installed in the **candidate only**

- Agent and real Provider Tool ownership and deletion restrictions, native ToolID/Schema immutable contract, Profile Tool / Policy reference checks.
- Workspace ownership/root and placement cannot be retargeted in place; any Profile scope referencing a placement blocks deleting it.
- New `profile_tool_target_scopes` represent explicit native Tool/workspace/sandbox pairs; no default wildcard or automatically generated grants.
- Stage E does **not** claim these tables are already connected to the real Wails/MCP/API runtime.

## 3. Actual disposable database acceptance evidence

- [x] Empty legacy `bootstrap.NewRuntime` DB: read-only repeatability, 7 expected schema versions, unchanged DB bytes, `ready` 0/0 and 9 future native operations.
- [x] Consistent SQLite Online Backup, including **committed WAL records with the source connection still open**; distinct destination, 0600, no overwrite, single-file checkpointed restore, actual source fingerprint unchanged.
- [x] Empty DB offline upgrade: typed Catalog v3, 9 native tools/FTS, legacy facade tables removed, untrusted native SQL insert rejected, **old v2 Runtime explicitly refuses future schema**.
- [x] Agent facade: 5 original Agent ToolIDs and AgentID preserved, internal Provider removed; SQL guard rejects deleting an Agent still owning canonical tools.
- [x] Local Sandbox facade: 4 old ToolIDs mapped into native workspace tools, exact verified root/Workspace→Sandbox binding preserved; source unchanged.
- [x] Two local Sandbox ToolBindings for the same Profile with differing presentations: `blocked` rather than union or a broader ToolID.
- [x] One local Sandbox Profile ToolBinding: `requires_review` even if target scope is exact, and `stage-copy` refuses premature export change.
- [x] Profile-direct Agent Tool: `requires_review` due source namespace change **even though ToolID stays the same**.
- [x] SQL Policy matching old Agent facade: `POLICY_SEMANTIC_DRIFT`, transaction not attempted.
- [x] Unknown Agent Task: `TASK_STATE_UNCERTAIN`; no replay.
- [x] Missing local Workspace root: `RUNTIME_PLACEMENT_UNVERIFIED` and denied staging.
- [x] External changes to a different SQLite Context (Agent grant history) invalidate the reviewed manifest even if Catalog IDs are unchanged.
- [x] Injected unknown legacy SQL trigger stops upgrade before it can commit; source and `.preupgrade` backup remain, candidate removed.
- [x] SQLite future Workspace authority/root and placement UPDATE SQL blocked by candidate triggers.
- [x] CLI `assess` / `stage-copy` return intentional failure codes, emit structured JSON, do not touch source, and reject wrong digest.
- [x] Full ordinary Go regression: `go test -count=1 -parallel=2 -timeout=4m ./backend/...` **exit 0** (receipt `/tmp/mender-stage-e-go-normal-20261107.exit`), plus a subsequent passing focused rerun after the WAL backup fixture was added.
- [x] Focused Race Detector: `go test -race -count=1 ./backend/internal/migration/toolhardcut ./backend/cmd/mender-tool-migrate` — both packages passed, including real Bootstrap/SQLite fixtures.
- [x] `golangci-lint run ./backend/...` — **0 issues**. `go run scripts/check-backend-architecture.go` — passed with no dependency exceptions.
- [x] `pnpm desktop:bindings:check && pnpm sdk:check && pnpm lint && pnpm typecheck && pnpm test && pnpm build` — Wails/SDK generation checks and TypeScript passed, i18n **1639/1639**, Vitest **76 files / 512 tests**, Web/Desktop builds passed with existing Vite large-chunk notices.
- [x] `git diff --check` and edited Markdown local-link/fence checks passed before submission.
- [ ] Canonical full `pnpm check` was **not rerun**: the historical full Bootstrap Race timeout remains open from stages B/C. Focused Stage E Race success and full ordinary Go success **do not** count as a canonical green run; Wails GUI was not manually tested because production runtime has not changed.

## 4. What remains a hard blocker for Stage F (not silently shipped in E)

**This Stage E candidate is safe only for proven subsets** (empty DB, unexposed Agent tools, unexposed verified local Sandbox Workspace facades). Typical populated user Profiles or nonlocal Sandbox/SSH Jobs receive `blocked` or `requires_review` and cannot be automatically cut over.

1. **Reviewed Profile/Policy/Direct/Hybrid migration:** preserve exact binding aliases, policy/approval, concurrency, credential allowlists, original authorized Workspace/Sandbox targets, MCP direct name/export and Profile revision. New permissions must be a subset of the old ones. Requires owner-reviewed `MCPExportDiff`, exact ToolID remap and runtime commit-point checks; no blanket approved flag.
2. **Agent Workspaces and active Task/Session:** migrate roots/aliases only after owner, root/permission and Task recovery proof; no ACP process is assumed to run inside Sandbox. Unknown Tasks never auto-retry. ACP v1 remains baseline.
3. **SSH/remote Jobs and Docker/remote Workspace:** require trusted target-side attestation and JobID→Sandbox/Backend evidence. Absent evidence is a P0 blocker.
4. **Atomic runtime cutover:** Stage F must replace ProviderID-only Catalog/Invocation/Policy/Audit/MCP/UI/SDK/Wails consumers **in one releasable unit** and remove their legacy execution paths. Never run `sourcecatalog` alongside the old Provider facade runtime.
5. **Operational release gate:** quiesce all writers and Task submissions, re-assess latest snapshot, independent DB rollback/backup recovery rehearsal, complete canonical Go Race/full frontend/Wails native acceptance, explicit operator authorization; only then consider a controlled upgrade. This phase **never swaps a candidate file into a real user path**.

A successful offline candidate is **not a claim that Mender has been upgraded**. It demonstrates deterministic preflight, safe limited conversion and failure rollback; production migration remains blocked until F/H complete.
