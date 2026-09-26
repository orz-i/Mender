# Management Web UI

Mender's browser management surface is backed by the local Management API and the generated TypeScript SDK.

## Start the local control plane

From the repository root:

    pnpm run:server

The management server defaults to:

    http://127.0.0.1:8080

It intentionally rejects non-loopback listen addresses in Phase 6 because remote management authentication and authorization have not been designed yet.

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
- Policy
- Credentials
- Audit

### Providers

Register Streamable HTTP or stdio MCP Providers, synchronize their Tool catalogs, and remove Providers that are no longer referenced by Profiles.

Persistent stdio configuration does not store environment values or secrets.

### Catalog

Catalog follows progressive disclosure:

    search compact metadata
      -> select Tool
      -> load full schema
      -> invoke through Mender

Search can be restricted to one Profile.

### Profiles

Create direct, dynamic, or hybrid Profiles and manage Tool membership.

### Policy and Credentials

These pages are intentionally read-only in Phase 6.

Current runtime behavior is displayed accurately rather than inventing configuration CRUD that the backend does not yet support.

### Audit

Audit shows data-minimized terminal invocation metadata only.

It does not display or retain Tool arguments, result payloads, Credential references, or secret values.

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
