# Phase 36 — P2 architecture governance closure

- Status: **CODE CLOSED — all constituent quality gates verified** (2026-10-14/15; Mac mini local timezone). Packaged/live deployment acceptance remains separate.
- Architecture: [ADR-0042](../architecture/adr/0042-p2-contracts-isolation-complexity.md)
- Operate on `main` with document-first, bounded commits. Preserve `.agents/`, `.pi/`, `.intent-loop/`.

## Outcomes and checks

- **A · cross-context invariants:** Agent schema v5 upgrade, immutable claimed facade config version, exact Claim version, two-Runtime regression, persistence contract consistency.
- **B · native plugin authority:** first-installation external Provider restricted-host where verified, existing explicit host retained, UI warning/confirmation and i18n, isolation negative tests.
- **C · maintainability:** isolate pure Provider form/config projection and one Core Runtime wiring responsibility without altering identity or contracts. Unit and integration parity checks.
- **Close:** full repo Go/race/lint + UI + SDK/generation + security gates, separate commits, no production mutations. Document incomplete external packaged/host acceptance honestly.

## Constraints

No new compatibility bridge or speculative plugin framework. No mixing Tool Suite, Agent, Sandbox and Plugin ownership. Do not modify the production App/SQLite/Tunnel/SSH process. A confirmation is not an effective sandbox boundary: host mode remains high authority. No mass upgrade of persisted Runtime isolation choices.
## Implementation and closure evidence

| Commit | Outcome |
| --- | --- |
| `ee7b692` | Document-first ADR-0042 and Phase 36 scope |
| `875c5f1` | Agent schema v5, exact managed facade config-version check and immutable SQL guard, independent connection and v4→v5 fixture tests |
| `ed3b308` | Fresh external native Provider defaults to verified `restricted_host`, persisted `host` preserved, UI warning/explicit Host escalation confirmation, localized tests |
| `b1c4ae5` | Extract pure Provider form/config transforms (about 492 lines) and bounded invocation wiring; preserve SDK/Wails/CLI and canonical execution semantics |
| `0afadea` | Replace dynamic SQL test construction with fixed SQL statements to satisfy G202 without relaxing lint rules |

### Scoped outcomes

- **Cross-context integrity:** Agent facade Claim refuses the wrong ConfigVersion; an already claimed Agent Provider's `plugin_id`, `plugin_installation_id`, `config_json` and `config_version` cannot be changed through SQLite, even from an independent database connection. Safe configuration-revision metadata updates still work. The existing trigger is upgraded transactionally from Agent v4 to v5 and reinstalled idempotently after a restart.
- **Native plugin authority:** fresh `DefaultIsolation(true)` uses `restricted_host` for external Provider processes, Workspace shell and Tunnel; `DefaultIsolation(false)` remains explicit host. Upgrading or reopening a database preserves the operator's previous `host` choice without process restart or mass migration. The platform launcher refuses unsupported restricted-host execution; the UI isolation viewport shows a warning when native Providers run as the OS user and requires explicit acknowledgement before switching them from restricted to host. Two additional localized messages bring i18n coverage to **1207/1207**.
- **Maintainability:** `providers-page.tsx` went from about 2867 to 2400 lines, with ~492 lines of pure MCP/OpenAPI/model-tools/generic configuration mapping in `provider-form-model.ts`, covered by new deterministic tests. `bootstrap/runtime.go` delegates Invocation + Provider activation wiring to `runtime_invocation.go`. ProviderID/ToolID, execution order, public DTOs and Auth/Credential boundaries are unchanged. This is a bounded extraction, **not** a claim that every large file has been completely refactored.
- **Documentation:** Product Vision now describes Mender's Tool/Resource aggregation, governance and supply positioning, rather than an Agent loop; the curated Marketplace is distinguished from a public registry. Architecture context and Principle 5 identify the narrowly documented shared-DB transactional-reference exception. ADR index includes 0041 and 0042; historical ADR-0033 initial-host default is expressly superseded for new settings only.

### Gates and reproducibility

- Go: `go test ./backend/...` **pass**, `go test -race ./backend/...` **pass**, `pnpm go:lint` **0 issues** after the fixed-query test change.
- UI: `pnpm lint`, `pnpm typecheck`, `pnpm test` **35 files / 210 tests passed**, `pnpm build` Web/Desktop **pass**, `pnpm i18n:check` **1207/1207**. Vite large JS chunk warnings remain nonblocking.
- Plugin contract/SDK: `provider-plugin:proto:check`, Provider/Sandbox/Tunnel SDK/example lint, tests, race and example builds, `go:modules:check`, `sdk:check`, `desktop:bindings:check`, `desktop:architecture:check` all passed during the initial aggregate invocation before its Go-lint failure. No contracts changed thereafter.
- `pnpm security:check` passed using the fallback Go vulnerability database after the primary DB was unavailable. No vulnerable code symbols reached; one required but unused Go module remains flagged in metadata. Production JavaScript dependency audit reports no known vulnerabilities.
- The first aggregate `pnpm check` failed at G202 caused by a **test-only** dynamic SQL string. That test was corrected using fixed statements, then Go lint, all backend Go tests/race and the full frontend/build gates passed individually. A second aggregate command lost its remote tool execution result, so **do not claim `pnpm check` exited 0 in aggregate**; its constituent gates have independent passing evidence.
- `git diff --check` passed. None of the tests changed production SQLite, running Wails/Tunnel, real Rocky SSH, Provider/Agent/Policy state, secret stores or live Plugins.

### Rollout and limitations

- Rebuild/restart the Runtime before claiming operational adoption. The Agent schema upgrades to **5** only on the new Runtime's first startup; old binaries that cannot read that schema must not be co-hosted on the upgraded DB.
- Existing `external_provider=host` persists after upgrade by design. Administrators may opt into `restricted_host` after plugin compatibility review. Existing host-mode processes will not be terminated merely because a new fresh default exists.
- `restricted_host` constrains selected filesystem paths, not outbound network traffic or kernel/container isolation. The host override is still available for explicitly trusted plugins; successful SDK unit tests cannot establish safety for every third-party executable.
- Packaged Wails Desktop, third-party native plugin compatibility and production migration read-only smoke were not run in this change. The implementation P2 contracts are closed by isolated evidence; operator deployment remains a separate manual acceptance.
