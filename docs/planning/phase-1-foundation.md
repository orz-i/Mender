# Phase 1 Plan — Foundation Scaffold

## Objective

Create the smallest repository skeleton that enforces the accepted architecture before product features are implemented.

This phase should produce boring, reliable infrastructure. It should not implement Catalog, Invocation, MCP aggregation, or management UI features.

## Work packages

### 1. Repository workspace

Create:

```text
apps/
backend/
packages/
contracts/
skills/
docs/
scripts/
```

Add:

- `go.work` when module layout requires it.
- root `package.json`.
- `pnpm-workspace.yaml`.
- root ignore/editor configuration.

Decision rule: do not add Nx/Turborepo during this phase.

### 2. Go foundation

Create the backend module and minimal entry points:

```text
backend/
  cmd/
    mender/
    menderd/
  internal/
    bootstrap/
```

The commands may initially do no more than identify themselves and prove composition/build boundaries.

Add:

- repository-pinned golangci-lint configuration.
- formatting/import rules.
- unit test baseline.
- race-test command for CI where applicable.

No MCP SDK should be imported into domain packages because no domain implementation exists yet.

### 3. TypeScript foundation

Create:

- React 19 + Vite web host.
- shared application package.
- shared UI package.
- SDK/transport boundary package.
- shared TypeScript/ESLint configuration.

Required TypeScript safety baseline:

```json
{
  "strict": true,
  "noUncheckedIndexedAccess": true,
  "exactOptionalPropertyTypes": true,
  "noImplicitOverride": true,
  "useUnknownInCatchVariables": true,
  "noFallthroughCasesInSwitch": true,
  "noImplicitReturns": true,
  "verbatimModuleSyntax": true
}
```

The first UI should be a shell/health screen only.

### 4. Repository command facade

Prefer a cross-platform task runner suitable for macOS, Linux, and Windows.

Expected developer commands:

```text
check
lint
test
typecheck
build
dev
```

Individual ecosystem commands remain directly runnable.

### 5. CI baseline

CI should independently report at least:

Go:

- formatting.
- golangci-lint.
- tests.

TypeScript:

- lint.
- typecheck.
- tests.
- build.

Do not hide multiple failing quality gates behind one opaque script if CI can report them separately.

### 6. Architecture enforcement

Investigate lightweight enforcement for:

- forbidden inward-to-outward Go imports.
- frontend host packages importing shared application packages, not the reverse.
- infrastructure SDK dependencies remaining at adapters.

Prefer simple static checks over introducing a large architecture framework.

## Commit segmentation

Phase 1 should be split into reviewable commits. A suggested sequence is:

1. `build: initialize monorepo workspaces`
2. `build(go): add backend foundation and lint policy`
3. `build(web): add strict React TypeScript foundation`
4. `ci: add baseline quality gates`
5. `docs: document local development commands`

Feature/domain implementation should begin only after these gates pass.

## Exit checklist

- [ ] Workspace structure matches ADR-0001.
- [ ] Go commands build.
- [ ] golangci-lint passes.
- [ ] Go tests pass.
- [ ] pnpm workspace installs reproducibly.
- [ ] TypeScript strict typecheck passes.
- [ ] ESLint passes.
- [ ] Frontend tests pass.
- [ ] Vite build passes.
- [ ] CI executes the same quality gates.
- [ ] No product feature logic has been introduced.
