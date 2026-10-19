# MCP Provider OAuth + Credential inventory — implementation contract

- Date: 2026-10-19; status: implementation
- Scope: upstream HTTP MCP OAuth authorization, token lifecycle, Credential inventory and shared UI.
- References: MCP Authorization 2026-07-28; RFC 9728, 8414, 9207, 8707; OAuth 2.1 PKCE S256; Go MCP SDK v1.8.0 auth.AuthorizationCodeHandler.
- Invariants: ADR-0021 Provider-wide connection identity + Profile ToolBinding credential proof, Mender not Agent Loop.

## Design

1. Add Credential `source: "oauth"` with only safe metadata: canonical MCP resource endpoint, issuer, scopes, expiry, status (`connected|authorization_required|expired`). Persist OAuth access/refresh tokens, confidential client credentials and dynamically registered secret **only in existing native OS keyring**. Never store secret or authorization code/state/verifier in SQLite, logs/audit, Provider config, frontend storage or API responses. Generic `CredentialResolver.Resolve` must reject OAuth so it cannot be misused as a static API Key.
2. MCP HTTP plugin supports `transport.oauthCredentialRef: "credential://namespace/name"`, mutually exclusive with static and credential-backed Authorization headers. stdio rejects OAuth fields. Include OAuth ref in `RequiredCredentialRefs` and `ConfigMetadata`, preserving Profile scope, Credential deletion usage and exact Provider identity. Two OAuth accounts = two Providers, never switch identities on a shared connection.
3. Backend OAuth APIs on management REST and Wails desktop with equivalent semantics:
   - `startOAuthCredential({namespace,name,displayName,mcpEndpoint,clientId?,clientSecretRef?,scopes?})` → `{flowId,authorizationUrl}`, reusing same OAuth reference on reauth, rejecting unrelated collisions.
   - `getOAuthFlow(flowId)` → `{status:"pending"|"connected"|"failed"|"expired"|"cancelled",credential?:Credential,error?:string}`.
   - `cancelOAuthFlow(flowId)` cancels matching pending transaction.
   - `disconnectOAuthCredential(namespace,name)` clears local token, best-effort remote revocation if advertised; retains inventory ref with authorization_required state.
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
