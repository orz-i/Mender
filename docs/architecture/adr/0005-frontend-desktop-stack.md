# ADR-0005: Standardize the frontend stack and keep Wails as a shell

- Status: Accepted
- Date: 2026-09-25

## Context

Mender needs a management UI for catalog, profiles, policies, credentials, and invocation history. The same product UI should be reusable in browser and desktop distributions.

The desktop client must not become the architecture boundary for backend behavior because CLI and daemon deployments need the same core without WebView dependencies.

## Decision

The frontend stack is standardized on:

- React 19.
- TypeScript.
- React Router.
- Tailwind CSS 4.
- shadcn/ui.
- Motion.
- Vite.
- pnpm.

TypeScript will use strict mode plus additional safety flags, including `noUncheckedIndexedAccess` and `exactOptionalPropertyTypes`, unless superseded by an ADR.

Frontend code will be organized around product domains/features rather than a repository-wide split into generic `components`, `hooks`, `services`, and `utils`.

Reusable application UI will live in workspace packages so the browser and desktop hosts can share it.

Wails v3 is the selected optional desktop shell.

Wails-specific bindings, lifecycle code, and native integrations remain outside the Go domain/application core. The backend must be buildable and testable without Wails.

The browser host may use HTTP/API adapters while the desktop host may use Wails bindings. Both adapt into the same frontend application concepts.

## Alternatives considered

### Separate desktop and web applications

This gives each host complete freedom but would duplicate substantial UI and domain-facing behavior.

### Make the Wails frontend the only management UI

This would make remote/server deployments unnecessarily dependent on a desktop distribution.

### Electron or Tauri as the initial desktop shell

Both are viable, but Wails aligns with the Go backend and is the selected project constraint. The shell boundary keeps future replacement possible.

## Consequences

### Positive

- Browser and desktop clients share most UI code.
- Strict TypeScript catches contract and state errors early.
- Wails does not contaminate backend domain boundaries.
- The selected stack is compatible with a Vite-based monorepo workflow.

### Negative / trade-offs

- Host adapters must be abstracted explicitly.
- Wails v3 lifecycle/version changes may require shell-level maintenance.
- Shared UI packages need discipline to avoid depending directly on one host.

## Follow-up

The scaffold phase will establish shared TypeScript configuration and workspace package boundaries before feature UI is implemented.
