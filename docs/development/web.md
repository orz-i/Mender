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

The Providers page is list-first. `Add provider` is a two-stage plugin-first workflow: the first Dialog lists available Provider plugins with an explicit Add action for each entry and contains no configuration fields; after selecting one plugin, a second focused Dialog shows only that plugin's configuration. A create handoff from Plugin Marketplace already carries an exact plugin identity, so it bypasses the chooser and opens the same focused configuration Dialog directly. Built-in plugins currently include `io.mender.provider.mcp`, `io.mender.provider.model-tools`, `io.mender.provider.openapi`, and `io.mender.provider.workspace`. Enabled external installations appear alongside built-ins with their exact package version/Installation ID; unknown external PluginIDs use Mender's schema-driven generic form and remain pinned to that exact installation. MCP offers `Local command` (stdio) and `Remote endpoint` (Streamable HTTP); HTTP/stdio are MCP transport choices rather than top-level Provider kinds. OpenAPI offers an explicit description source (HTTPS/loopback URL or absolute local file), optional API server override, operation include/exclude SourceKey filters, and a separate Authentication workflow. Model Tools offers an engine selector (OpenAI / Anthropic / Gemini), a model identifier, the normalized v1 `web_search` capability, and a separate Model API Credential selector. Workspace uses the generic schema renderer for one absolute host-local root and exposes `read`, `search`, `edit`, and `shell`; the browser does not turn that root into a client-side filesystem grant. Provider rows expose View, Edit, Sync, and Delete actions; an external Provider additionally exposes Rebind when another enabled installation of the same PluginID is available.

Provider ID, namespace, and Plugin ID remain stable during editing. Plugin-owned configuration can change without changing canonical Provider identity. Saving persists metadata/configuration and re-synchronizes the Provider; for OpenAPI this reloads the description and preserves ToolID when the operation SourceKey is unchanged, Model Tools keeps stable `web_search` identity across engine/model configuration refreshes, and Workspace keeps the four stable SourceKeys while the configured root changes explicitly.

The management contract exposes `GET /api/v1/provider-plugins`, installation list/status/remove resources, generic `POST /api/v1/providers`, and `POST /api/v1/providers/{providerId}/rebind`, while the historical `/providers/http` and `/providers/stdio` create endpoints remain temporary MCP compatibility shims. External Provider creation carries optional `installationId`; Provider responses preserve that exact pin and may expose the persisted plugin connection summary. Rebind accepts only target `installationId` plus plugin-owned `config`; PluginID and ProviderID cannot be changed by the request. Browser management deliberately has no endpoint that accepts an arbitrary server filesystem path for plugin installation: local package installation is CLI- or Desktop-hosted. Provider responses expose `pluginId`, installation/plugin/config versions, connection summary, and plugin-owned `config`; the old MCP transport-specific projection remains for compatibility while Web/Desktop use the plugin-oriented contract. Neither OpenAPI nor Model Tools requires protocol/vendor-specific management endpoints.

Provider connection/source fields and authentication are intentionally edited in separate Dialogs. Normal authentication selects Credentials from inventory. MCP HTTP bindings store header name, optional scheme, and Credential Reference; MCP stdio bindings store environment name and Credential Reference. OpenAPI bindings store Security Scheme name → Credential Reference. Model Tools stores one model API Credential Reference and resolves it only during governed execution; Provider sync performs no model API request and resolves no API key. Profile-scoped calls must authorize the required Credential Reference before any model-provider network access. Model Tools v1 verifies that provider-hosted web search actually executed and returns only normalized answer/source/query data, excluding provider reasoning/thought/internal state.

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
