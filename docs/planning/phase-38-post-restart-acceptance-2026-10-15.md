# Phase 38 — Post-restart live read-only acceptance (2026-10-15)

- Source baseline: `f1514cf` (Phase 38 source cleanup closeout), ADR-0044.
- Acceptance: **PASSED for restart, canonical configuration, Profile/Tool identity, discovery, CLI retirement, and one live SSH-backed read-only execution**.
- Exception: Docker-backed local Workspace tool invocation remains **unavailable** while its selected Sandbox run is `stopped`. This is a pre-existing lifecycle state, not evidence of a Phase 38 regression; no sandbox start was requested or performed.
- Scope: Read-only SQLite and process inspection, local HTTP health probes, read-only Tool discovery and search, and CLI command-surface checks. Live read/search invocations generated expected invocation-audit rows, but no Provider/Profile/Policy/Credential/Sandbox settings or remote files were changed.

## Restart and runtime identity

1. `main` was at Phase 38 closeout `f1514cf` when inspected; tracked files clean. Existing untracked `.agents/`, `.intent-loop/`, `.pi/` were left untouched.
2. Active Wails desktop development binary `apps/desktop/bin/mender-desktop.dev.app/Contents/MacOS/mender-desktop` was built and started at **2026-10-15 19:01:22** in the Mac's displayed local time, after the code-governance commits. Process remained running throughout checks.
3. Vite desktop frontend HTTP root returned 200. The embedded Wails UI was not visually inspected by automation, so this is process/frontend availability rather than pixel-level UI acceptance.
4. Active Runtime MCP loopback endpoint returned **405 + Allow: POST** on a GET probe, consistent with a POST-only MCP handler; attached tunnel-client health endpoint returned **HTTP 200**. The connected Mender tool discovery and Tool description calls succeeded from ChatGPT.
5. macOS unified logs contain WebKit/AppKit sandbox and entitlement warnings during startup. No corresponding demonstrated Mender control-plane outage was observed. These system warnings were not treated as proof of Runtime failure.

## Persistent state (read-only SQLite connection)

- Database opened using SQLite URI `mode=ro` and `PRAGMA query_only=ON`; `PRAGMA quick_check` returned `ok`.
- Counts: **6 canonical provider_configs**, **25 Tools**, **1 Profile**, **21 profile_tools ToolBindings**, **6 Sandboxes**, **1 Tunnel**.
- Obsolete `mcp_provider_configs`, `mcp_config_schema_migrations`, `provider_config_imports`, and `provider_plugin_installations` tables are absent.
- Provider Config schema is v4; Profile schema v5; Sandbox schema v18; Agent schema v5. No unsupported future schema version was observed.
- Zero orphaned `profile_tools` ToolIDs, `provider_configs` ProviderIDs, or catalog Tool ProviderIDs.
- Both the Docker Workspace `read` Tool and Rocky SSH Workspace `rocky_read` Tool IDs returned by ChatGPT are still present under their exact canonical Provider IDs in SQLite. Their respective ToolBindings remain in the Profile.

## Live tool and CLI behavior

| Check | Evidence | Result |
| --- | --- | --- |
| Mender `mender_search_tools` | Workspace/Rocky Tool IDs and metadata returned through currently connected Tunnel | PASS |
| Mender `mender_describe_tool` | Canonical `read`, `rocky_read`, `rocky_search` schemas returned | PASS |
| Rocky SSH read-only `rocky_search` | `mode=files`, bounded patterns; returned a well-formed empty match set, `truncated=false` | PASS — invocation audit `completed/complete` |
| Local `read` (`README.md`) | Provider returned `provider_error`; matching invocation audit `failed / provider_error` | ENVIRONMENT-LIMITED |
| CLI retired `provider add http` | Returns `unknown command "http"` with nonzero exit | PASS |
| CLI `provider add mcp --help` | Lists canonical `http` and `stdio` subcommands | PASS |

### Docker limitation triage

- Docker Engine was responsive (`client=29.8.0`, `server=29.8.0`); unrelated existing Docker containers were running.
- The selected canonical Workspace Provider binds to a Docker Sandbox with persisted run state `stopped` (generation 1). Read/execute availability was **not** assumed merely because a Tool is visible in the Profile catalog.
- The same local `read` Tool had a `provider_error` recorded on **2026-10-13**, before Phase 38, and had earlier successful calls on **2026-10-11**. This supports treating the observed failure as pre-existing / operational until a running Docker Sandbox is accepted separately.
- A Sandbox lifecycle transition would be a separate host-owned action; do not automatically start, stop, recreate, provision or delete it during this acceptance.

## Acceptance outcome

**Phase 38 post-restart read-only acceptance is closed with one isolated operational caveat.** Canonical source/API retirement is observable in the live build; Profile and Provider identities survive; Mender Tunnel discovery works; the ready SSH Workspace read-only search successfully crosses the governed invocation path; no migration or schema corruption was found.

**Not claimed:** successful invocation of the stopped Docker Sandbox, a pixel-level desktop UI run-through, or validation of every configured Provider. If the Docker Sandbox is intentionally started, its local read/edit/shell/search tools can be re-verified separately without reopening Phase 38.
