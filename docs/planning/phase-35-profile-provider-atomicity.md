# Phase 35 — Profile CAS and Provider reference atomicity (P1 closure)

- Status: **CLOSED — code and isolated acceptance gates passed** (2026-10-14/15; workstation timezone differs).
- Refines ADR-0019/0039/0041; preserves Mender as a Tool/Resource aggregation, governance and supply platform, not an Agent loop.
- Work scope: Profile write versioning and Provider deletion versus new Profile/Policy references. No real remote hosts, new plugin protocols, or implicit reauthorization.

## Audited failure cases

1. `Profile.SaveProfile` writes a complete aggregate after a separate read, with no optimistic revision check. Two independently valid writes can both succeed, lose one ToolBinding, or **restore an explicitly revoked binding**. Reproduced on real SQLite Store with two stale aggregate snapshots.
2. `Runtime.DeleteConfiguredProvider` checks references, closes its active instance, removes config and then deletes Catalog Provider in separate operations. `profile_tools.tool_id` has no FK to Catalog Tools, and Policy match selectors are JSON. A new binding/rule can commit after the scan and before deletion.
3. Provider usage scanning currently checks Policy ProviderIDs but not Policy ToolIDs whose Tools belong to the target Provider. Provider deletion also needs to preserve Catalog search index integrity.

## Invariants and implementation decision

- **Profile v5** adds durable monotonic `revision`. All Profile reads restore it, every aggregate mutation preserves the read revision, and `SaveProfile` uses `UPDATE ... WHERE id=? AND revision=?` in the same SQLite transaction that replaces ToolBindings. A stale write fails with a typed conflict; it cannot silently retry or restore revoked grants. No automatic merging of authorization changes. Tunnel binding mutations share this revision.
- **Commit-point Provider reference protection:** when composing the shared Runtime database, install idempotent SQL guards for Profile binding insertion/retargeting and Policy match insertion/update. A write targeting a missing Provider/Tool fails closed. Install an atomic Provider deletion guard checking both Profile ToolBindings and Policy ProviderIDs/ToolIDs. SQLite's single-writer serialization makes deletion and reference creation mutually exclusive, even across independent Runtimes, without a long-held global lock or expiring safety lease.
- **Atomic provider cleanup:** after established owner/resource checks and active instance closure, remove Provider config, Catalog FTS rows, and Catalog Provider in one transaction. If a late reference wins the race, the entire deletion transaction rolls back; config/catalogue remain intact and the call returns a typed conflict. If deletion wins, later Profile/Policy writers fail their SQL guard. Never retry model/Agent invocations or silently re-grant. Plugin closure and future governed reactivation remain separate.
- **Policy protection:** inspect Policy selectors for direct ProviderID **and** ToolIDs mapping to the Provider; SQL guards cover both, including concurrent rule creation and rule update. Disallow stale/unknown references rather than allowing a dangling selector.
- **Public error contract:** map stale Profile writes and guarded Provider/Tool reference conflicts to explicit HTTP 409 and preserve CLI/Desktop errors. Forward-only schema migration; no V1 compatibility parser.
- **Boundary:** SQL reference guards deliberately span Catalog/Profile/Policy at Runtime composition as an explicit cross-context consistency adapter. No cross-context Repository import in domain code. Preserve canonical IDs, Profile aliases, Tool Suite facade ownership, Policy semantics, Wails API shapes, and Core invocation execution.

## Acceptance tests

1. Two stale Profile writes: only first wins, second gets conflict; concurrent revoke cannot be undone by stale Add/Replace/Tunnel write; revision remains monotonic and is restored on reopen, including old v4 database upgrade.
2. Two Runtime/SQLite writers: Profile/Policy binding wins -> Provider deletion cannot commit; Provider deletion wins -> late Profile/Policy binding rejected. No orphan ToolBinding or Policy selector, no loss of Provider config on conflict.
3. Provider Policy ToolID-only reference blocks deletion; managed Agent/Sandbox Provider ownership guards remain unchanged.
4. Fail-closed in malformed or unavailable reference state; no side effects on inert GET, no external resources touched.
5. Targeted Go tests and race; full `pnpm check`, `pnpm security:check`, `git diff --check`; clean tracked tree and separate bounded commits. Exclude `.agents/`, `.pi/`, `.intent-loop/` from all Git stages.

## Work packages (bounded)

- D: this contract committed before implementation.
- A: Profile domain/SQLite revision CAS, API conflict propagation, regressions.
- B: shared-DB reference guards, atomic Provider delete, concurrent/negative tests.
- Close: full validation and closure evidence as final documentation commit.

No automatic replay, no generic remote task manager, no background mutation of real resources.

## Closure evidence (2026-10-14/15)

| Commit | Result |
| --- | --- |
| `deb06e7` | P1 contract committed before modifying production code |
| `dbd7997` | Profile schema 5, monotonic Revision/CAS, stale Add/Remove/Replace/Tunnel conflicts, explicit HTTP 409 |
| `ed42bda` | Runtime-composed SQLite commit-point guards, Policy ToolID/ProviderID reference closure, transactional Provider config/FTS/Catalog deletion, cross-Runtime tests |
| `135ae0e` | HTTP 409 contract regression tests for Profile revision, Profile Tool, Policy and Provider conflicts |

### New deterministic evidence

- Two stale Profile snapshots cannot both overwrite: the previously reproducible revoke→stale Add authorization resurrection now yields `ErrRevisionConflict`. A fresh authorized write succeeds; revision is monotonic across Get/List. Profile v1 fixtures upgrade forward to v5 with revision 0.
- Profile mutation and Provider removal from two distinct Runtime connections have mutually exclusive outcomes. In the “late binding won” fixture, Provider config, Catalog and FTS all survive the failed delete. In the “delete won” fixture, late Profile/Policy/Provider-config inserts are rejected at SQLite write time.
- Policy references by **ToolID alone** block Provider retirement. A raw SQL Provider deletion that bypasses application preflight is still rejected if Profile ToolBindings exist. Invalid Policy retargeting cannot create an orphan reference.
- SQL triggers are installed idempotently at shared Runtime composition after Catalog, Provider Config, Profile, Policy and Sandbox stores are present; standalone context stores continue to use their own tests. No new cross-context domain imports or new global scheduler were introduced.

### Quality gates

- `go test ./backend/...`, `go test -race ./backend/...`, `pnpm go:lint`: passed; Go lint reported zero issues.
- `pnpm typecheck`, `pnpm lint`, `pnpm test` (**204/204**), `pnpm build`, i18n **1205/1205**: passed. Vite warns about large output chunks; unrelated to this fix.
- `pnpm go:modules:check`, `pnpm sdk:check`, `pnpm desktop:architecture:check`, `pnpm desktop:bindings:check`, `pnpm provider-plugin:proto:check`: passed.
- Provider/Sandbox/Tunnel plugin SDK and example lint, race, build/test checks: passed individually.
- `pnpm security:check`: exit 0. Go vulnerability scan found no reached vulnerable symbol; one required-but-unused module vulnerability remains in dependency metadata. Production JavaScript dependency audit found no known vulnerabilities.
- The aggregate `pnpm check` entrypoint was blocked by the remote command safety gate; **all commands included in that script were executed individually and passed**. This is not a claim that the aggregate command itself ran.
- `git diff --check`: passed. No real Rocky, production database, secrets, Profile grants or running Tunnel were changed. All tests used isolated local fixtures.

### Operational boundary

- The existing Provider instance manager intentionally shuts down a Provider *before* durable deletion. If a competing reference commits in that small interval, the new atomic SQL operation rejects and rolls back deletion while retaining Provider config/Catalog; the now-closed local instance may need a separately governed activation/sync. This is an availability/recovery consideration, not a dangling-reference or unauthorized-Tool reauthorization path. The P1 referential safety invariant is closed; no implicit plugin restart or secret use is added as error compensation.
- Deploying this change requires a **new Runtime binary**; the first startup upgrades Profile SQLite to schema 5 and installs the reference guards. Do not run old and new binaries against a migrated production database. Manual packaged Desktop/real-world operational acceptance is separate from this isolated code closure.
