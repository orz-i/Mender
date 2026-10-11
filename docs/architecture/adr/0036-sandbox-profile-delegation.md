# ADR-0036: Explicit Profile Service Principal → Sandbox delegation

Status: Accepted
Date: 2026-10-11

## Context

A managed Profile MCP adapter executes its tools under the trusted service identity `service/profile:<immutable ProfileID>` with `OriginMCP` and `ProfileScope(ProfileID)`. A Desktop/CLI Sandbox is owned by `local_user/os:<uid>`. The Sandbox Router and Admission both enforce direct owner equality, so remote Profile MCP tools are discoverable but every Workspace operation fails at Sandbox acquisition. Policy allow does not grant resource ownership.

## Decision

Grant independent `read`, `search`, `edit`, and `shell` capabilities by the immutable pair `SandboxID + ProfileID`. No wildcard, implicit grant or automatic migration. Only the actual local Runtime owner via CLI/Desktop/loopback management can grant, replace or revoke the capability set. Profile IDs must refer to existing Profiles; display names and slugs are never authority.

A delegated acquisition requires the exact trusted tuple: `OriginMCP`, `PrincipalService`, reference `profile:<ProfileID>`, `ScopeProfile` and scope reference equal to that same ProfileID. Neither model arguments nor HTTP headers can set this tuple. The existing ToolInvocation Policy and separate `sandbox.acquire` Policy/Approval must both allow the operation. The grants authorize only acquisition, not Sandbox provision/start/stop/destroy, ownership change or unbounded host execution.

Store capability rows and immutable grant/revoke audit in Sandbox SQLite v8. Check the matching grant before backend access and repeat the exact authorization test in the same atomic SQLite statement that reserves the durable execution lease. This prevents concurrent revocation and another Runtime process from authorizing a new lease after revocation. Existing leases finish under their original authority; revocation does not force-kill them. Lifecycle and cross-owner protections remain unchanged. Destroyed Sandboxes cannot acquire or receive new grants.

Offer owner-only typed list/replace/revoke operations via HTTP/OpenAPI SDK and Wails, with a separate Sandbox **Access** view. The view first previews existing access; focused dialogs select a Profile from inventory and explicit capabilities, defaulting to read/search. Confirm high-authority edit/shell operations and revoke without `window.confirm`. Never expose grant management as an MCP Tool.

## Security implications

Every remote caller with access to a Profile endpoint can exercise that Profile's explicitly granted capabilities, subject to ToolBinding and Policy. `host` is not isolated; `shell` may run as the local OS user. Grant management must explain this. Never impersonate the owner, relax Sandboxes to accept arbitrary services, disable Policy, use compatibility bridges, or automatically authorize the user's existing Mender Profile.

## Acceptance

Default deny; owner-only changes; typed input validation; exact Profile identity/origin/scope; capability-by-capability enforcement; concurrent revoke/acquire fencing; Policy still authoritative; immutable audit; successful temporary-database Profile MCP call only after grant; Web/Desktop/CLI or management adapter parity; full lint/tests/race and Wails build. No real user Sandbox or Credential is modified during tests.
