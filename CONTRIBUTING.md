# Contributing to Mender

Mender is currently architecture-first. Contributions should preserve the accepted domain boundaries and make architectural intent explicit.

## Before changing code

Read:

- `docs/product/vision.md`
- `docs/architecture/principles.md`
- `docs/architecture/adr/README.md`
- `docs/planning/roadmap.md`
- `docs/governance/development-workflow.md`

If a proposed change conflicts with an accepted ADR or repository-level architecture principle, write a new ADR before implementation.

## Development principles

- Keep dependencies pointing inward.
- Keep protocol, persistence, framework, and UI types out of domain packages.
- Define interfaces near consumers.
- Prefer small explicit abstractions over speculative generalization.
- Keep bounded-context ownership explicit.
- Do not bypass the shared invocation pipeline.
- Do not place secrets in schemas, prompts, logs, fixtures, or audit payloads.
- Do not introduce new build systems, frameworks, or runtime dependencies without a concrete need.

## Commit policy

Use Conventional Commit style:

```text
<type>(optional-scope): <summary>
```

Common types:

- `docs`: documentation only.
- `feat`: user-visible capability.
- `fix`: bug fix.
- `refactor`: behavior-preserving implementation change.
- `test`: test-only change.
- `build`: build/dependency/workspace change.
- `ci`: CI change.
- `chore`: repository maintenance.

Commits should be reviewable and single-purpose. Architectural decisions, repository scaffolding, and feature implementation should normally be separate commits.

Do not use a later cleanup commit to hide an architectural decision that should have been visible earlier in history.

## Pull requests

A pull request should:

1. Explain the problem and scope.
2. Link relevant ADRs or planning items.
3. State architectural impact.
4. Include tests appropriate to the changed layer.
5. Pass all quality gates.
6. Avoid unrelated refactors.
7. Update documentation when behavior or architecture changes.

Use the repository pull request template.

## Quality gates

Once the implementation scaffold is in place, changes are expected to pass:

### Go

```bash
gofmt
go test ./...
golangci-lint run ./...
```

Race tests will be required for relevant concurrent/runtime changes.

### TypeScript

```bash
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

`vite build` is not a substitute for TypeScript type checking.

## Testing by layer

- Domain: fast unit tests; no database, network, filesystem, or protocol runtime.
- Application: use fakes for ports and test use-case behavior.
- Infrastructure: integration/contract tests for SQLite, MCP, secret stores, and external boundaries.
- End-to-end: prove shared workflows through CLI/MCP/UI entry points when those surfaces exist.

## Generated code

Generated files must identify their generator where practical and must not be edited manually.

Contract generation must be reproducible from source files committed in the repository.

## Dependencies

New production dependencies should have a clear owner and purpose.

Prefer standard library or already-adopted dependencies when they meet the requirement without compromising correctness or maintainability.
