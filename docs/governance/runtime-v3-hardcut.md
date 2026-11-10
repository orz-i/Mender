# Runtime v3 hard-cut and acceptance

**Architecture**: [ADR-0064](../architecture/adr/0064-tool-provenance-and-execution-placement-hardcut.md). **Status**: v3 selected as the only supported runtime schema; release acceptance requires direct code and real-runtime checks. There are no incremental legacy migration gates or authorized compatibility pathways.

## Decision (2026-11-09)

The operator explicitly chose **destructive legacy-data retirement** rather than migration of v2 ToolIDs, Agent Tasks, SSH Job records, or MCP exports. The previous migration conflict report (13 ownership/recovery blockers and 24 MCP export diffs) no longer governs v3 startup because no old records are being imported. Historical values are not used as live v3 principals, grants or tools. This is a deliberate data reset, not an assertion that those old facts were safely migrated.

The default desktop and CLI SQLite files were separately backed up using SQLite's consistent backup API while no Mender process was detected, then atomically replaced with the pinned empty v3 image. Original backups remain separate and are **not runtime-readable**; restoring one directly into v3 must fail, not activate legacy facades. SSH known-host pins were left intact to avoid weakening host verification. No unrelated Project files or remote resources were removed.

## Mandatory runtime invariants

1. Tool SourceRef is always typed (`provider`, `agent`, `native`). No Agent/Sandbox-generated Provider facades and no Provider-keyed Workspace Target CRUD routes.
2. Native Workspace and Jobs tools have one host-reviewed canonical definition per operation and use independent Workspace/Sandbox grants, TargetScope, Policy, approval, budget, generation and lease checks.
3. Agent as Tool uses one AgentID-owned source; ACP v1 Task/Session state remains Agent-owned. Startup reconciliation may fence *expired* tasks as `unknown` but must not replay prompts, synthesize completion, or downgrade permissions.
4. An external Provider continues through its real Provider plugin, activation and credentials. Tool discovery and schema inspection must not activate side effects.
5. No v2 compatibility reads, fallback executors or endpoint aliases; any v2 file is rejected before creating providers, agents or sandbox processes.
6. Prior historical plans/test fixtures do not define current behavior. Only tests of supported v3 behavior and fail-closed rejection of truly old schema are release gates.

## Acceptance evidence to collect

- Full Go test and Race, architecture/lint, SDK and Wails generation parity, `pnpm check`, `pnpm security:check`, Web/Desktop builds and localization.
- One disposable v3 database, official menderd and Wails startup, SourceRef discovery, external Provider invoke, Native Workspace read/edit/shell via real sandbox lease, SSH Job operations only when verified, and Agent delegate/status/result/cancel/resume with a real supported backend.
- Negative cases: Profile revocation, Task ownership, ambiguous Workspace selection, stale Sandbox generation, source/ToolID collision, false backend capability, old schema startup, and unknown Task nonreplay.
- No original user DB, restored v2 fixture or production remote Sandbox may be used as an implicit integration-test backend.

**Release rule:** tests that intentionally describe retired v2 execution/UI contracts must be deleted, not adapted to synthesize facades; security properties that remain applicable must be asserted through v3 tests. Working-tree cleanup and checks are separate from actual Wails production deployment.

## Verified acceptance results (current governance branch)

- **Quality gate**: `pnpm check` completed with exit code 0, including backend architecture, plugin suites, Go unit/integration tests, `go test -race ./backend/...`, golangci-lint, OpenAPI SDK generation, Wails binding/architecture checks, ESLint, TypeScript strict checking, i18n (1607/1607), 75 Vitest files with 505 passing tests, and Web/Desktop production frontend builds. Existing Vite large-chunk notices are non-fatal.
- **Security gate**: `pnpm security:check` completed with exit code 0, with no known callable Go vulnerabilities and no high-severity production JavaScript dependency advisories.
- **CLI v3 smoke**: against a disposable newly initialized database, `mender tool search workspace --source native:workspace` returned exactly the four canonical Workspace operations; Catalog contained five `native:jobs` operations, zero Agent/Sandbox instances and no old facade or Provider-keyed Target tables; SQLite `integrity_check` returned `ok`. The temporary database was deleted after inspection.
- **Boundary checks**: Agent as Tool is AgentID-owned, External Provider sources remain configurable, native Workspace/Jobs use independent target grants and leases. Rebuilding the official Runtime no longer calls legacy bootstrap `NewCatalog/NewProfiles/NewSandboxes` or a reverse Provider-v2 startup gate.
- **Outstanding acceptance, not simulated**: native Wails interactive use with actual supported Agent and SSH/Sandbox backends must be validated on the released binary. The earlier agent/task data has been deliberately retired, not migrated. The repository still includes some unit-test-only legacy per-context SQLite migration constructors and fixtures; these are not called by the official v3 Runtime, but should not be described as fully deleted source.

The above automated outcomes are tied to the worktree version before the documentation-only acceptance record, not a claim that any production installation has been restarted or deployed.
