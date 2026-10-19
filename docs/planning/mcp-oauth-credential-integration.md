# MCP Provider OAuth + Credential inventory — implementation contract

- Date: 2026-10-19; status: implemented and code-validated; live deployment and external IdP acceptance pending
- Scope: upstream HTTP MCP OAuth authorization, token lifecycle, Credential inventory and shared UI.
- References: MCP Authorization 2026-07-28; RFC 9728, 8414, 9207, 8707; OAuth 2.1 PKCE S256; Go MCP SDK v1.8.0 auth.AuthorizationCodeHandler.
- Invariants: ADR-0021 Provider-wide connection identity + Profile ToolBinding credential proof, Mender not Agent Loop.

## Design

1. Add Credential `source: "oauth"` with only safe metadata: canonical MCP resource endpoint, issuer, scopes, expiry, status (`connected|authorization_required|expired`). Persist OAuth access/refresh tokens, confidential client credentials and dynamically registered secret **only in existing native OS keyring**. Never store secret or authorization code/state/verifier in SQLite, logs/audit, Provider config, frontend storage or API responses. Generic `CredentialResolver.Resolve` must reject OAuth so it cannot be misused as a static API Key.
2. MCP HTTP plugin supports `transport.oauthCredentialRef: "credential://namespace/name"`, mutually exclusive with static and credential-backed Authorization headers. stdio rejects OAuth fields. Include OAuth ref in `RequiredCredentialRefs` and `ConfigMetadata`, preserving Profile scope, Credential deletion usage and exact Provider identity. Two OAuth accounts = two Providers, never switch identities on a shared connection.
3. Backend OAuth APIs on management REST and Wails desktop with equivalent semantics:
   - `startOAuthCredential({namespace,name,displayName,mcpEndpoint,clientId?,clientIdMetadataUrl?,scopes?})` → `{flowId,authorizationUrl}`, reusing the same OAuth reference for reauthorization and rejecting unrelated collisions. With neither client field, advertised DCR is attempted.
   - `getOAuthFlow(flowId)` → `{status:"pending"|"connected"|"failed"|"expired"|"cancelled",credential?:Credential,message?:string}`.
   - `cancelOAuthFlow(flowId)` cancels matching pending transaction.
   - `disconnectOAuthCredential(namespace,name)` clears the local token; retains the inventory Reference with authorization_required state. Remote revocation is intentionally deferred and cannot be claimed.
   - Existing list/get/delete project safe OAuth metadata; rotateNative rejects OAuth. Deletion remains usage-guarded.
4. Browser flow: bind localhost `127.0.0.1:0` before returning auth URL; in-memory bounded TTL transaction with cryptographic state + PKCE S256, loopback callback validates state/issuer, exchanges code server-side, emits safe finish page (no tokens/codes). Frontend opens system browser and polls flow; no automatic browser challenge on Tool calls.
5. Use MCP-protected-resource discovery `WWW-Authenticate: resource_metadata`, well-known path fallback, auth-server metadata (RFC8414/OIDC); send resource parameter on auth/token, validate RFC9207 issuer, scope selection + server errors; prefer Go SDK auth. Support pre-registered client (including manually hosted HTTPS client-ID metadata document) and advertised DCR fallback; do not pretend native desktop hosts a public CIMD. No insecure network metadata/redirect following, SSRF and cross-origin token protections, bounded timeouts, safe nonsecret errors.
6. MCP resource HTTP transport obtains the current OAuth token per request, refreshes expiring token and persists rotated refresh token, remains functional across app restart. Only attach Authorization to canonical MCP resource origin (no authorization on metadata/token/DCR requests, no redirect leakage). Missing credentials/invalid_grant → reauthorization_required, never launch browser from tool execution.
7. UX: existing inventory first, create/edit in dialogs, OAuth connect/status/reconnect/disconnect in Credentials; MCP Provider HTTP form offers OAuth Credential selector, no exposed token field. Preserve native/environment credentials, existing Header, stdio, Profile grants, no extra tabs/descriptive clutter.

## Parallel ownership

- OpenCode (backend): Go credential domain + SQLite migration + secret storage; OAuth flow/refresh/transport; bootstrap, desktop service & binding generation, management API and OpenAPI, Go tests. Stage cohesive commits; **do not** restart running Wails/Tunnel or mutate production DB.
- Antigravity (web): shared React Credentials & Provider management, SDK client/mapping, desktop frontend bridge, i18n and UI tests. Do not edit Go/SQLite/Go-generated Wails bindings or OpenAPI managed by backend. Coordinate names from this contract; avoid conflicting generated schema changes.
- Coordinator: reconcile interfaces, review tests/security, run targeted and full `pnpm check`, commit acceptance. No production deployment claim before owner rebuild.

## Required tests

First login discovery -> PKCE -> localhost callback -> OS keyring -> MCP bearer call; refresh rotation after restart; concurrent refresh; invalid state/issuer/resource, cancelled/timeout, unsafe redirect/metadata, invalid_grant; OAuth/static collision; delete usage guard and Profile gate; stdio/static compatibility; no raw secrets anywhere outside secure runtime storage. Fake IdP tests acceptable for automatic evidence; external IdP acceptance must be separately reported if no account.

## Implementation acceptance — 2026-10-19

### Implemented and committed

- `d272662`: architecture/credential-security contract.
- `cbd70b4`: built-in MCP HTTP `oauthCredentialRef` persistence validation, stdio rejection, static/bound Authorization-header exclusivity and Credential Ref dependency projection.
- `b0535a9`: `source=oauth`, non-secret SQLite v2 metadata, native OS Keychain-backed OAuth token storage and local disconnection, with native/environment/SSH preserving their existing paths.
- `3465c1e`, `c237868`: REST/Wails/SDK OAuth credential projections and local disconnect.
- `07d342d`: Profile-scoped non-secret OAuth Credential authorization proof; no OAuth Token exposed through ordinary `CredentialResolver.Resolve`.
- `ac96532`: OAuth login manager (SDK AuthorizationCodeHandler, PKCE S256, state, issuer, challenge/well-known discovery, bounded loopback callback, DCR), plus inventory and Provider authentication UI. The original shared staging race inadvertently put Flow Go files and frontend in this one commit; tests and follow-up changes independently reviewed and verified it. Do not repeat or reset the commit based solely on prior Agent `unknown` state.
- `26bb205`: explicit operator-only Start/Status/Cancel authorization APIs on local REST and Wails/SDK, with non-secret status projection.
- `7d163a9`: HTTP MCP bearer-token transport restricted to the exact configured endpoint, host-owned OAuth Manager, serialized token refresh and rotated refresh-token persistence, credential-aware disconnect, RFC8707 `resource` on authorization/token requests, root-audience-vs-endpoint mapping, safe metadata/SSRF/DNS and redirect gates. Static header/stdio and Profile execution proof remain unchanged.
- `10bb942`: native system-browser launch or explicit authorization link, OAuth Credential creation, pending/terminal status polling, cancellation, reauthorization, disconnect/delete management modal, required endpoint/client configuration and localized compact UI.

### API surface

- `POST /api/v1/credentials/oauth/flows` starts a local interactive OAuth browser flow. The request accepts `namespace`, `name`, `displayName`, `mcpEndpoint`, optional `clientId`, `clientIdMetadataUrl`, `scopes`; absence of registration details attempts advertised DCR, otherwise fails closed.
- `GET /api/v1/credentials/oauth/flows/{flowId}` reads only safe flow state, optionally safe Credential metadata; `DELETE` cancels pending flows.
- `POST /api/v1/credentials/{namespace}/{name}/disconnect` clears local authorization state and native token without deleting a bound Credential Reference. Native Wails methods and typed Web SDK correspond to these operations.
- Provider `transport.oauthCredentialRef` is configuration metadata; it never stores OAuth secret material. Active MCP HTTP requests resolve/refresh only for their endpoint, and OAuth Credential access is checked against Profile grants independently of token delivery.

### Verified evidence

- Go targeted `go test -race` passed for `credential/...`, `mcp/infrastructure/...`, `invocation/application`, including fake OAuth authorization server (PKCE, 401 challenge, issuer, dynamic native client registration, callback tampering and cancellation), Keychain-backed token storage through fakes, concurrent single refresh and rotated refresh token across Manager restart, `invalid_grant` requiring reauthorization, OAuth credential/method conflict, wrong resource, redirect and cross-origin/path rejection.
- `golangci-lint run ./backend/...` reported **0 issues**.
- `pnpm lint`, `pnpm typecheck`, `pnpm i18n:check` passed (1,420 localized message keys). All generated Wails bindings and OpenAPI SDK schema are committed; generated-file checks were successful.
- First full `pnpm check` reached the shared Vitest suite and encountered **one intermittent unrelated Docker 5-second polling-timer assertion**. That exact test passed in isolation, all **37** tests in the same file passed, and follow-up `pnpm test` passed **49 files / 300 tests**; `pnpm build` passed Web and Desktop. **Second complete `pnpm check` rerun passed, exit code 0**, log `/tmp/mender-oauth-full-check-rerun.log`. It includes Protocol/SDK checks, generated OpenAPI/Wails bindings, Go unit and Go Race tests, lint, typecheck, i18n (1,420 keys), all 300 Vitest cases and both production builds. Only existing non-fatal Vite chunk-size warnings remain.

### Deliberately pending / truthful production boundary

- **No production deployment** was performed: the operator's running Wails desktop, Tunnel, live SQLite, Profile grants and OAuth accounts were not restarted or altered. Owner must rebuild/reconnect after this set of commits before live product acceptance.
- No external OAuth provider/account was exercised. Local fake IdP tests verify protocol behavior without authentic real provider scopes or client registration policies. Real provider OAuth consent and reconnect/expiry acceptance remain outstanding.
- Local disconnect invalidates token/connection on next HTTP request; it does **not** currently call remote RFC 7009 revocation, even if a remote server advertises it. That optional remote revocation remains explicit follow-up, not a claimed supported feature.
- No automatic redirect to a browser from Agent Tool invocation; browser consent is initiated only by an explicit Credential-management action. No Agent-loop changes or compatibility bridges were added.
