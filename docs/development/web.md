# Management Web UI

Mender's browser management surface is backed by the local Management API and the generated TypeScript SDK.

## Start the local control plane

From the repository root:

    pnpm run:server

The management server defaults to:

    http://127.0.0.1:8080

It intentionally rejects non-loopback listen addresses because remote management authentication and authorization have not been designed yet.

The HTTP adapter also validates local request provenance: Host must resolve to localhost/loopback, browser Origin (when present) must be localhost/loopback, and explicit cross-site Fetch Metadata is rejected. Mender does not add permissive CORS headers. These checks harden the local control plane against hostile web origins and DNS-rebinding-style paths; they are not a substitute for a future authenticated remote-management design.

Use a specific database when needed:

    go run ./backend/cmd/menderd --db ./mender.db

## Start the Web development host

In another terminal:

    pnpm dev:web

Vite proxies /api to http://127.0.0.1:8080, so product UI code uses relative management API paths.

## Management areas

The shared React application provides:

- Overview
- Providers
- Catalog
- Profiles
- Servers
- Policy
- Credentials
- Audit

### Providers

The Providers page is list-first. Registration no longer occupies permanent page space: `Add provider` opens a focused Dialog for HTTP or stdio configuration. Provider rows expose View, Edit, Sync, and Delete actions.

Provider ID, namespace, and transport kind remain stable during editing. Editable metadata/connection fields are persisted through the shared Runtime and the Provider is re-synchronized after save.

Remote HTTP Providers support a separate inventory-backed `Authentication` workflow plus legacy header templates for compatibility. Provider connection fields and authentication are intentionally edited in separate Dialogs.

Normal authentication selects Credentials from inventory. HTTP bindings store header name, optional scheme, and Credential Reference; stdio bindings store environment name and Credential Reference. Resolved values are injected only when connecting/launching the Provider, and shared authenticated MCP connections re-authorize required References before every Tool call.

Legacy header templates and import of common `mcpServers` / `mcp` JSON shapes remain supported. For example:

```text
Authorization: Bearer ${ANYSEARCH_API_KEY}
X-Anysearch-Client: mcp/1.0.0
```

Legacy sensitive header templates must use an environment-variable reference; Mender persists only the template and resolves the value from the Mender process environment when connecting or reconnecting to the Provider. Plaintext Authorization/API-key/token-like values are rejected. This path is explicitly labeled legacy because it is not represented by Profile Credential scope. Resolved secret material is not stored or returned by the management surface. Cross-origin redirects are rejected when custom headers are configured.

Persistent stdio configuration stores only Credential References for secret environment bindings. Local stdio Provider processes receive only a small non-secret ambient environment allow-list plus explicitly configured Credential-backed entries, rather than Mender's complete process environment.

Non-loopback HTTP Provider endpoints must use HTTPS, and Provider URLs cannot contain URL userinfo credentials.

### Catalog

Catalog now opens with a compact browse of synchronized Tools, so successful Provider synchronization is visible without guessing a search term. Provider rows also expose their current synchronized Tool count.

Catalog still follows progressive disclosure:

    browse/search compact metadata
      -> result list
      -> Tool overview
           -> full description
           -> schemas
           -> invocation/result

The Tool overlay is intentionally split into mutually exclusive views so long descriptions, schemas, and invocation controls never compete in one screen. Browse/search never eagerly loads Tool schemas. Catalog filtering is source-oriented: both browse and lexical search can be restricted to one Provider. Profile membership is an exposure/governance concern and is managed only from the Profiles workflow, not used as the Catalog page's primary filter.

Tool calls return the canonical Invocation outcome body even when the outcome is `rejected` or `failed`; those governed outcomes are not reclassified as arbitrary HTTP transport errors. Malformed HTTP envelopes still use 4xx responses and internal server failures remain 5xx.

### Profiles

Profiles is also list-first. `Create profile` opens a Dialog; Tool membership management opens from each Profile row in a separate Manage Dialog.

Normal Profile governance never asks the user to type or copy a ToolID. `Choose tools` opens a Catalog-backed picker with Tool search, Provider filtering, multi-select, and human-readable Tool/Provider metadata. Existing membership rows also display Tool names and Providers rather than stable IDs.

Saving picker selection uses one atomic binding-replacement operation. The application validates every selected Tool before the Profile repository transaction replaces bindings, so a multi-select update cannot intentionally leave a partially saved Profile. Hybrid Profiles expose the Direct export choice; direct/dynamic modes normalize that flag automatically. Existing alias/presentation/policy/concurrency fields are preserved when membership or Direct state changes.

Each selected Tool exposes separate focused `Presentation`, `Governance`, and `Credentials` actions rather than expanding the picker into a dense form. Presentation edits alias/title/description plus hybrid Direct export; Governance edits restrictive policy (`inherit`, `require approval`, `deny`) and binding-local concurrency (`inherit`, `serial`); Credentials multi-selects existing inventory items. Long descriptions stay truncated in the selected-Tool row. The dialogs never overlap the Manage dialog, and normal management asks for neither ToolID nor a raw Credential Reference.

### Servers

Servers completes the management flow by exposing a persisted Profile as a running downstream MCP endpoint.

The page lists runtime-owned Streamable HTTP servers and supports Start, Restart, Stop, endpoint copy, and client-config copy.

Start selects a Profile and loopback bind address. The endpoint is exposed at `/mcp`, for example:

```text
http://127.0.0.1:8765/mcp
```

Managed servers are runtime-only. They stop when Mender exits and are not auto-started after restart. Profile changes require Restart before an already-running server sees the new Profile snapshot.

Desktop-managed/management-managed servers are loopback-only and reject non-local Host/Origin and explicit cross-site browser requests. Remote downstream serving is intentionally unavailable until authentication/authorization is designed.

stdio remains client-launched through:

```bash
mender serve mcp --profile default --stdio
```

### Policy

Policy is now list-first management over the persistent Runtime-wide PolicySet.

The page separates the main information classes:

- summary cards for Default decision, enabled Rule count, and Approval behavior;
- a Rule table for compact decision/state/target summaries;
- a focused Default decision Dialog;
- one stepped Rule Dialog that shows only one category at a time: Decision, Scope/Targets, or Governance Facts.

Rule ordering is not an authorization control. There is no priority field. The default applies only when no enabled Rule matches; matching Rules combine by `deny > require_approval > allow`.

Profile, Provider, and Tool targets use current inventory selectors. Normal Policy management never asks for RuleID, ProfileID, ProviderID, or ToolID text input. Missing historical Tool targets remain visibly unresolved rather than being retargeted by name.

Governance Fact matching distinguishes `any`, `known`, and `unknown`. Effects/resource matchers apply only to known reviewed Facts; selecting unknown removes those predicates and creates an explicit unknown-fact rule. Resource matching is limited to exact/prefix. `require_approval` clearly reports that the current Approval adapter fails closed until an interactive approval workflow exists.

### Credentials

Credentials is list-first management. Users can create native-keyring or environment-backed Credentials, list non-secret metadata, rotate native secrets, and delete unused Credentials. Native create/rotate requests carry the secret only for that local request; responses never echo it. Environment Credentials persist only the variable name.

Credential deletion returns a conflict while a Profile ToolBinding or Provider authentication binding references the Credential. Normal workflows derive `credential://namespace/name` from namespace/name and use inventory selectors elsewhere rather than asking users to paste raw References.

### Audit

Audit shows data-minimized terminal invocation metadata. Each row can open a read-only detail Dialog containing invocation identity, execution origin/scope, canonical outcome classification, timing, policy/approval outcome, error class, and an optional bounded/redacted failure summary.

It does not display or retain Tool arguments, Governance Facts, full result payloads, Credential references, progress payloads, or raw MCP protocol traffic. Historical Audit rows keep empty governance/outcome fields for metadata that was not retained when they were created.

## API contract

The canonical management contract is:

    contracts/api/openapi.yaml

Generated TypeScript types live at:

    packages/sdk/src/generated/schema.ts

The generator is pinned in the SDK script:

    pnpm --filter @mender/sdk generate

pnpm check verifies that regenerating the contract produces no diff.

## Frontend transport boundary

packages/sdk is the only browser package allowed to perform HTTP requests.

The product UI in `packages/app` receives a typed `MenderClient` from its host. The browser host constructs the HTTP implementation; the Wails desktop host constructs a binding-backed implementation without duplicating product pages.

## Production build

Build the current browser bundle with:

    pnpm build

API serving and browser asset distribution remain separate from desktop packaging. The Wails desktop shell owns its Runtime directly and embeds its Vite production assets; it does not start the management HTTP server.
