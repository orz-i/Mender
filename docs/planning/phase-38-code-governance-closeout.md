# Phase 38 — Code-governance hard-cut closeout

- Date: 2026-10-15
- Baseline: `41a8b77` (Phase 37 accepted)
- Architecture decision: [ADR-0044](../architecture/adr/0044-hard-cut-residual-source-compatibility.md)
- Status: Source cleanup and automated gates **passed**. Live native-client restart/acceptance was not performed.
- Constraints: Mender remains the LLM/Agent Tool and Resource aggregation, governance and supply plane. No new compatibility bridge, Agent loop, data reset or production-runtime mutation.

## Removed and collapsed

| Slice | What changed | Commit |
| --- | --- | --- |
| CLI | Unregistered hidden `provider add http\|stdio` transport aliases; invalid old commands fail rather than returning success from Cobra help; `provider add mcp http\|stdio` remains canonical. Added negative CLI regression. | `e909571` |
| Profile domain | Retired `ToolSelection`, `DefaultToolBinding`, `NewProfile`, `Selections`, and domain `AddTool`/`ReplaceTools` shims. Production callers and Profile/MCP tests now construct and operate on `ToolBinding` directly; canonical grants and metadata are preserved. | `97c19b3` |
| Profile application/composition | Removed `ReplaceToolSelection` alias, optional CredentialLookup constructor arguments, unused `NewProfiles` convenience composer and forwarding `newProfiles` method. Explicit dependencies remain available, including an intentional nil lookup for tests without Credentials. | `97c19b3`, `c360014` |
| MCP endpoint policy | Deleted the MCP-only `transportpolicy` forwarding package and duplicate tests; config validation and connection both call shared `providerhttp.ValidateEndpoint`, preserving IPv6 and other endpoint validation cases at the policy owner. | `87c6878` |
| Documentation | Indexed ADR-0044 and annotated Phase 13 historical interfaces superseded by ADR-0043/0044, without rewriting the historical implementation record. | `33a3cfa` |

Across backend code and tests compared to `41a8b77`: **27 changed files, 130 inserted lines, 244 deleted lines, net -114 lines**. This is a measured review delta, not a claim that every remaining legacy-named feature is dead.

## Verification evidence

| Gate | Result |
| --- | --- |
| Targeted Profile, MCP Provider, shared HTTP policy, Bootstrap and CLI Go tests | PASS |
| `pnpm check` (plugin Protobuf generator parity, SDK, Wails bindings and architecture, Go lint/test/race, ESLint, TypeScript, i18n, Vitest, production build) | PASS — exit 0 |
| `pnpm security:check` | PASS — exit 0 |
| Go `govulncheck` | 0 reachable symbol vulnerabilities; 1 dependency-module vulnerability reported as not called by current code |
| `pnpm audit --prod --audit-level high` | PASS — no known vulnerabilities |
| `rg` of removed Go symbol names/imports; `git diff --check` | PASS — no residual Go references or whitespace errors |
| Git staging | Only owned files explicitly staged; `.agents/`, `.pi/`, and `.intent-loop/` excluded |

Vite reports a non-fatal build chunk-size warning for existing large Web/Desktop JS bundles. This is a frontend performance concern, not an excuse to widen this backend source-retirement change.

## Audited survivors — not safe to delete blindly

1. **Plugin installation one-time upgrade.** `backend/internal/plugin/installation/infrastructure/sqlite/migrations.go` still actively migrates old `provider_plugin_installations` inventories; `legacy_descriptor.go` and `legacy/` are used by that path. `localfs` retains legacy digest verification for migrated artifacts. Retiring these requires a read-only installation/artifact inventory, explicit obsolete-version admission policy, and an upgrade/rollback test of any new forward-only schema guard. This is not a runtime duplicate configuration store.
2. **MCP HTTP Header Templates.** `TransportConfig.Headers` remains a supported configuration input for connection-time header resolution. UI labels it legacy because inventory-backed credential bindings are preferred; its persistence/usage must be checked and intentionally migrated or rejected before hard-removal. Removing it without inventory can break Provider authentication.
3. **Agent/Sandbox `legacy` Tool Suite states.** These statuses express explicit adoption/ownership reconciliation, not SSH Jobs v1 execution or Workspace v1 config parsing. Preserve until their own resource contracts are proven removable.
4. **Historical SQLite migrations and fail-closed guards.** These encode real upgrade and resource-ownership guarantees. Do not mistake them for live dual-write bridges.

No live Client restart, Profile/Policy/Credential mutation, database table drop, SSH remote action, plugin installation, or sandbox mutation occurred in this phase. Native Wails rebuild and restarted-runtime smoke tests, if required, belong to deployment acceptance.

## Conclusion

The bounded source-compatibility removal is complete with full quality and security gates. A future destructive installation/header retirement is a separate, evidence-bound migration decision, not part of this code-only hard cut.

## Post-restart acceptance

The 2026-10-15 [live read-only acceptance](phase-38-post-restart-acceptance-2026-10-15.md) verified the rebuilt Wails Runtime, canonical SQLite state, connected Tunnel, retired CLI command behavior, and a successful Rocky SSH read-only Tool invocation. The Docker Workspace read call remains unavailable because its Sandbox is stopped; this is an operational caveat, not a reopened Phase 38 source-compatibility regression.
