# Phase 42 follow-up — Duplicate Agent workspace registration feedback

Date: 2026-10-17. Status: implementation pending. Refines [ADR-0049](../architecture/adr/0049-agent-owned-local-workspace-paths.md).

## Reproduction and cause

In the live OpenCode Agent, `workspaceRoot=/Users/mini/Coding/Mender`. The owner already added alias `gaoge` at `/Users/mini/Coding/Gaoge`. In the new screenshot the owner attempts to add alias `mender` with `/Users/mini/Coding/Mender`, which duplicates the **implicit default workspace**. The Agent service correctly raises `ErrConflict` on this directory; however, Wails returns an ordinary JavaScript Error whose English message has no Chinese translation, so shared `errorMessage` renders only the catch-all **发生意外错误**. Existing alias and existing nondefault directory conflicts are similarly not distinguished by the UI or repository.

## Contract

1. An Agent-local directory may have **only one authoritative binding per Agent** (the implicit default or exactly one named workspace). Aliases are unique. A duplicate default root, registered directory, or registered alias does **not** add another record or grant. No Profile rights are widened by a conflict.
2. UI anticipates known duplicates from its current owner-visible Agent and workspace list: normalized key comparison and comparable local directory paths (trim/trailing slash). It displays specific inline, translated explanations and disables Confirm. In particular, adding `mender` at the Agent's default directory clearly explains **it is already the default workspace**.
3. Canonical path authorization remains server-side (including symlinks), so the backend must classify races and canonical aliases deterministically using typed conflict reasons, preserving `errors.Is(err, ErrConflict)` for governance/HTTP 409. Map these reasons consistently to a translated focused-dialog message for Wails and to stable HTTP error codes for API consumers; never rely solely on stale UI state.
4. Keep existing Agent, Provider/ToolID, Profile, task lifecycle and SQLite v9 schema, with no new migration, source alias inventory or compatibility bridge. No automatic replacement, overwrite, repair, Sandbox creation or deletion. The owner can use the default workspace directly, or remove the existing named alias before changing its target.
5. Document first, code and regression tests second, full `pnpm check` third. Use only isolated test databases and synthetic UI fixtures, then verify the current Wails frontend without modifying the owner's workspaces. Exclude all skill directories from git commits.

## Acceptance

- Duplicate default path: specific Chinese explanation before submit and rejected with typed 409 from backend.
- Duplicate workspace key but new path, duplicate registered path but new key, and canonical equivalent symlink/trailing slash: rejected, preserving the original record and Profile grants.
- Stale dialog racing with another owner write: typed backend result surfaced as localized message (not Unexpected error). Unrelated errors remain visible and are not incorrectly labeled as duplicates.
- Nonduplicate alias and directory still create one binding normally. No Agent Task, Provider, Tool or Sandbox is created in tests.
