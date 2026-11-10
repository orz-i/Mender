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
- Tools: services, Catalog and Workspace targets
- Profiles: selection, governance and delivery
- Plugin Marketplace, Skills, Sandboxes, Agents, Tunnels and Runtime
- Policy
- Credentials
- Audit

### Tool services

The service list lives at `/tools`. Choose an available plugin, then fill in its
focused configuration dialog. A Marketplace create handoff can select the
plugin and exact Installation ID in advance. Configurable built-ins include
MCP (stdio or Streamable HTTP), OpenAPI and Model Tools. Enabled external
installations show their exact package versions and use the shared
schema-driven configuration form. Create the Workspace Tool Suite from its
owning Sandbox, then manage additional targets from the service page. See
[v3 tool ownership and Workspace execution](../architecture/context.md).

Service actions include View, Edit, Sync and Delete as applicable. An external
Provider can be rebound to another enabled installation of the same PluginID.
Catalog drilldown and Workspace targets use `/tools/:providerId` and
`/tools/:providerId/workspaces`. The route parameter is the canonical
ProviderID.

ProviderID, namespace and PluginID remain stable during editing. Saving
persists metadata/configuration and re-synchronizes the Provider. OpenAPI
operations retain ToolIDs when their SourceKeys stay the same, and Model Tools
retains its `web_search` identity across engine/model changes.

Provider management uses one typed Plugin contract: `GET /api/v1/provider-plugins`, installation inventory/enable/disable/delete, `POST /api/v1/providers` and `PUT /api/v1/providers/{providerId}` with a required plugin-owned `config`, `DELETE /api/v1/providers/{providerId}`, and `POST /api/v1/providers/{providerId}/rebind`. The old `/providers/http`, `/providers/stdio` and flat MCP-specific PUT variants have been removed (ADR-0043). MCP HTTP/stdio is selected by `config.transport.kind` inside the built-in MCP Provider Plugin, not through separate persistence or endpoints. External creation carries optional `installationId`; Provider responses retain that exact pin and can include a bounded connection summary. Rebind accepts target installation identity plus plugin-owned config and never changes ProviderID or PluginID. Web management has no arbitrary server-filesystem plugin installation endpoint; local package installation belongs to CLI/Desktop. No second store or implicit Provider grants are created by these operations.

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

Catalog opens from a service with a compact browse of its synchronized Tools.
Service rows show the synchronized Tool count.

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

Profiles has dedicated creation, detail and selection editing routes.

Normal Profile governance never asks the user to type or copy a ToolID. `Choose tools` opens a Catalog-backed picker with Tool search, Provider filtering, multi-select, and human-readable Tool/Provider metadata. Existing membership rows also display Tool names and Providers rather than stable IDs.

Saving picker selection uses one atomic binding replacement. The application
validates every selected Tool before the Profile repository writes membership.
Retained bindings preserve their presentation, policy, concurrency and
credential constraints.

Each selected Tool has focused `Presentation`, `Governance` and
`Credentials` actions. Presentation edits alias/title/description, Governance
edits restrictive policy and binding-local concurrency, and Credentials
selects existing inventory entries.

### MCP delivery

Open a Profile's MCP delivery page at `/profiles/:slug/deliveries/mcp` to
configure mode, optional Tunnel and the hybrid direct Tool subset. Configuration
has its own revision and also checks the Profile selection revision on save.
Direct mode derives its projection from all current members. Dynamic mode
uses discovery. Hybrid exposes the configured direct subset alongside
discovery. Saving configuration is separate from starting a server.

Start, Restart and Stop control the managed Streamable HTTP instance for the
Profile. The page offers endpoint and client-config copy. The loopback
endpoint is exposed at `/mcp`, for example:

```text
http://127.0.0.1:8765/mcp
```

Managed servers stop when Mender exits and require an explicit start after
restart. Requests refresh the projection when Profile selection, MCP delivery
configuration or Catalog revisions change. This refresh preserves the active
listener and Tunnel. stdio uses a startup snapshot for its client process.

Managed listeners bind to loopback and reject non-local Host/Origin and
explicit cross-site browser requests. External reachability uses a configured
Tunnel subject to the Runtime's lifecycle and admission checks.

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

Create forms accept human-typed namespace/name values and derive the canonical lowercase-hyphen identity for preview and submission (for example, `OpenAI` + `Tunnel API Key` becomes `credential://openai/tunnel-api-key`). Each canonical part must match `^[a-z][a-z0-9-]{0,62}$` and environment variable names must match `^[A-Za-z_][A-Za-z0-9_]*$`. Invalid identifiers cannot be submitted, and the form explains the constraints inline. Creation failures are displayed **inside** the create dialog rather than behind it; known reference conflicts and unavailable native keyring errors receive safe, localized guidance. Secret bytes and untrusted backend error strings are never included in that guidance. No plaintext fallback is permitted.

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

`packages/sdk` is the browser HTTP boundary. The Web host gives
`packages/app` a `ManagementClient` implemented by the SDK. Desktop supplies
the same contract through Wails and separately supplies local owner
capabilities. Native Agent approvals and Tool placement are therefore
available only through the host that supplies those capabilities.

## Production build

Build the current browser bundle with:

    pnpm build

API serving and browser asset distribution remain separate from desktop packaging. The Wails desktop shell owns its Runtime directly and embeds its Vite production assets; it does not start the management HTTP server.
