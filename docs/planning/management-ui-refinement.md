# Management UI Refinement

Status: Complete

## Objective

Reduce information overload in the management application by giving each top-level page one primary content responsibility.

The page itself should present the resource collection or primary workflow. Creation, inspection, editing, confirmation, and secondary details should move into focused overlays such as Dialogs when they do not require a dedicated route.

This work refines presentation and interaction structure without moving domain rules into React.

## Page responsibility rule

For top-level management routes:

1. One page = one primary resource or task.
2. The primary collection/list/search surface stays visible.
3. Creation forms do not permanently occupy the page.
4. View/Edit actions open focused Dialogs unless the workflow is complex enough to justify a dedicated route.
5. Destructive actions require explicit confirmation.
6. Read-only backend capabilities remain read-only; UI refinement must not invent unsupported CRUD.
7. Host-neutral `MenderClient` remains the frontend boundary for both Web and Desktop.

## Providers baseline

The Providers page establishes the interaction pattern for later page refinement.

### Primary page content

The page contains:

- page title/description,
- one `Add provider` primary action,
- one Provider list/table,
- empty/loading/error states.

The always-visible Remote MCP and Local MCP registration cards are removed.

### Add Provider

`Add provider` opens one Dialog.

The Dialog selects transport:

- Remote MCP / Streamable HTTP,
- Local MCP / stdio.

Only fields for the selected transport are shown.

Creation continues to synchronize the Provider immediately using existing semantics.

### View Provider

Each row exposes a View action.

View is read-only and shows:

- Provider ID,
- namespace,
- display name,
- description,
- transport,
- connection configuration,
- created/updated timestamps.

No secret/environment values are introduced.

### Edit Provider

Each row exposes an Edit action.

Provider identity is intentionally stable:

- Provider ID is immutable,
- namespace is immutable,
- transport kind is immutable.

Editable fields are:

- display name,
- description,
- HTTP endpoint for HTTP Providers,
- executable/argv/working directory for stdio Providers.

Saving an edit persists metadata/configuration and re-synchronizes the Provider Catalog snapshot. If synchronization fails, the updated configuration remains visible so the user can correct it and retry Sync, matching existing registration behavior.

A real Runtime/Application update path is required. The UI must not fake local editing.

### Row actions

Provider rows expose:

- View,
- Edit,
- Sync,
- Delete.

Actions should remain visually secondary to the Provider identity/list content.

## Backend contract

Add an explicit Provider update use case and adapter contract.

The update path must:

- preserve ProviderID, namespace, and created timestamp,
- update `updatedAt`,
- validate Domain metadata invariants,
- validate MCP transport configuration,
- avoid persisting environment/secret fields,
- keep HTTP transport hardening rules,
- reuse the existing Provider synchronization path,
- remain available through both HTTP management and Wails bindings.

## Follow-up pages

After Providers establishes the pattern, review:

- Profiles: list first; create/edit/membership management should not all compete on one page.
- Catalog: search/results remain primary; Tool detail/invocation can move into focused detail/action surfaces.
- Audit: table remains primary; record detail can use a read-only Dialog if needed.
- Policy/Credentials: retain read-only status semantics until editable application models exist.
- Overview: remain a summary/navigation surface, not a second copy of each domain page.

Each page should be changed independently with tests rather than through one broad visual rewrite.

## Closeout

Completed on 2026-09-26.

Applied the page-responsibility rule to the management surfaces that were carrying multiple competing workflows:

- Providers: list-first; Add/View/Edit moved to Dialogs; Sync/Delete remain row actions.
- Profiles: list-first; Create and Tool membership management moved to Dialogs.
- Catalog: search/results remain the page responsibility; Tool schemas and invocation moved to a Tool Dialog while preserving progressive disclosure.
- Audit: already a single invocation-history table, so no structural split was required.
- Policy/Credentials: already single read-only status surfaces and remain intentionally non-editable.
- Overview: remains the intentional cross-domain summary/navigation surface rather than duplicating domain editing workflows.

Provider editing required a real end-to-end update contract. Provider ID, namespace, and transport remain immutable; display metadata and transport-specific connection fields can be updated through both HTTP and Wails adapters, followed by Catalog synchronization.

Final validation: `pnpm check` passed with OpenAPI and Wails binding regeneration, architecture checks, Go lint/tests/race, ESLint, strict TypeScript, 4 Vitest files / 15 tests, and Web/Desktop production builds.

## Verification

Providers refinement must cover:

- add Dialog for HTTP and stdio,
- list-only initial page structure,
- View Dialog,
- Edit Dialog and persisted update,
- Sync/Delete regression,
- HTTP SDK update contract,
- Desktop Wails update contract,
- shared product UI with no host-specific imports,
- existing full `pnpm check`.
