# ADR-0044 — Hard-cut residual source compatibility and Profile binding adapters

- Status: Accepted for Phase 38 bounded code governance (2026-10-15)
- Parents: ADR-0019, ADR-0039, ADR-0042, ADR-0043.
- Scope: source/API cleanup without modifying deployed state, SQLite schema, secrets, Profile grants or SSH remote jobs.

## Audited evidence

- The canonical Profile aggregate already stores `[]ToolBinding`, normalizes by `ExposureMode`, and exposes `Bindings`, `NewProfileWithBindings`, `AddBinding`, `ReplaceBindings`. However `ToolSelection`, `NewProfile`, `DefaultToolBinding`, `Selections`, `AddTool`, and `ReplaceTools` reconstruct or flatten that canonical state. Production CreateProfile, provider reference checks and CLI list/show still call this compatibility surface. Many tests use it rather than exercise canonical bindings.
- `ReplaceToolSelection` is only a type alias for `ToolBindingInput`. The Profile application constructors still accept omitted CredentialLookup through variadic arguments despite Runtime providing an explicit lookup. `NewProfiles` is an unused convenience constructor distinct from the Runtime's `NewProfilesWithTunnelLookup`.
- `provider add http` and `provider add stdio` are hidden registrations of the same MCP subcommands already available at `provider add mcp http|stdio`. The canonical Plugin-based route is in use and documented in ADR-0043.
- HTTP Header Templates are an **active** MCP plugin configuration path, not a proven-dead shim. Likewise `legacy` statuses in Sandbox/Agent suites describe explicit adoption/diagnostic states, not necessarily protocol-v1 readers; database migrations are needed for real upgrade contracts. Name matching alone is not enough to justify deletion.

## Decisions

1. **One Profile domain representation.** Hard-delete the obsolete membership types/constructors/accessors/mutators and move all production and test users to `ToolBindingSpec` / `NewToolBinding`, `NewProfileWithBindings`, `Bindings`, `AddBinding`, and `ReplaceBindings`. Do not alter persisted binding format or drop alias, Policy, concurrency, credential or Tunnel metadata. Preserve normalization, revision and ToolID ordering.
2. **One application input contract.** Retire the `ReplaceToolSelection` type alias. Make dependencies explicit on `NewAddTool` and `NewReplaceTools`; remove the unused `NewProfiles` constructor when no caller remains. Keep application use-case names `AddTool` / `ReplaceTools` because these describe supported user actions; they are not domain compatibility APIs.
3. **One CLI entrypoint.** Remove hidden `provider add http|stdio` registrations. Keep `provider add mcp http|stdio` and their implementation functions. A legacy invocation must fail instead of redirecting.
4. **Do not convert active features into dead code by declaration.** Defer removal of MCP HTTP Header Templates and Sandbox/Agent adoption statuses until authoritative configuration inventory plus an explicit data/operational retirement boundary is established. Preserve legitimate forward-only SQLite migrations and security fail-closed checks; do not reintroduce compatibility bridges.

## Verification / acceptance

- Confirm zero references to retired names across production and tests, aside from historical docs and negative regression assertions. Add a deterministic CLI test confirming the old commands are rejected while canonical commands remain installed.
- Existing Profile tests must directly cover mode normalization, duplicate IDs, immutable mutation, binding metadata and revision with the canonical API; MCP exposure, CLI and Provider reference guard behavior must remain unchanged.
- Run targeted Go tests after each bounded change, whole `go test ./backend/...`, Go lint/race and frontend checks if affected, repository `pnpm check` (subject to external tool availability), and `git diff --check`. Document any unavailable gate; do not silently weaken it.
- Stage only owned files; never add `.agents/`, `.pi/`, `.intent-loop/`. Commit architecture decision first, then independently verifiable source slices. Leave canonical user data and live processes untouched.

## Non-goals

No Agent loop, new adapters, replacement framework, wide Provider/Sandbox/Tunnel redesign, database destructive migration, hidden alias to compensate for deleted APIs or synthetic future-version switch.
