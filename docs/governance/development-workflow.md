# Development Workflow and Change Governance

## Purpose

This document defines how Mender turns decisions into implementation while preserving reviewable history.

## Change classes

### Class A — Architecture change

Examples:

- Changes dependency direction.
- Introduces a new bounded context.
- Changes canonical Tool or Invocation semantics.
- Changes persistence ownership.
- Changes the supported runtime model.
- Replaces a major framework or protocol strategy.
- Conflicts with an accepted ADR.

Requirement: an ADR must be proposed and accepted before or with implementation.

### Class B — Cross-boundary feature

Examples:

- Adds a new adapter.
- Adds a new provider type.
- Extends policy or credential behavior.
- Adds an API contract spanning backend and frontend.

Requirement: planning note or issue with explicit affected contexts and contracts. An ADR is required only if architectural constraints change.

### Class C — Local implementation change

Examples:

- Refactor inside an established boundary.
- Bug fix with no contract change.
- Test coverage.
- UI behavior within an existing feature.

Requirement: normal review and quality gates.

## Planning rule

Planning artifacts should describe:

- Problem.
- Scope.
- Non-goals.
- Affected bounded contexts.
- Contracts affected.
- Security implications.
- Test strategy.
- Exit criteria.

Plans should avoid prescribing low-level implementation prematurely unless that detail is itself a reviewed decision.

## Commit segmentation

Repository history is part of the design record.

Each commit should have one clear purpose and leave the repository in an understandable state.

Preferred ordering for architecture-bearing work:

```text
docs/ADR
    ->
workspace/build foundation
    ->
domain/application behavior
    ->
infrastructure adapters
    ->
UI/integration
    ->
follow-up documentation
```

Do not combine an architectural decision, broad formatting churn, dependency upgrades, and feature implementation in one commit.

## Branch and pull request discipline

- Keep branches focused on one planned outcome.
- Rebase/synchronize before final merge according to repository hosting policy.
- Avoid unrelated cleanup while implementing a feature; submit it separately.
- Explain deviations from the planned commit sequence in the pull request when necessary.

## Dependency changes

A new production dependency must answer:

1. What capability does it provide?
2. Why is the standard library/current dependency set insufficient?
3. Which architectural layer owns it?
4. Does it introduce runtime, licensing, security, or platform constraints?
5. How is it version-pinned and updated?

Protocol/framework dependencies belong at adapters or infrastructure boundaries unless an ADR explicitly establishes otherwise.

## Schema and contract changes

Changes to JSON Schema, OpenAPI, CLI machine output, or other externally consumed contracts must consider backward compatibility.

Breaking changes require explicit release/migration planning once public releases begin.

## Security-sensitive changes

Changes involving credentials, process execution, filesystem/network access, provider trust, authorization, approval, audit, or remote exposure require an explicit security review section in the planning/PR description.

## Documentation synchronization

Update documentation in the same change when:

- public behavior changes;
- a command or configuration changes;
- an architectural boundary changes;
- a new invariant is introduced.

Historical ADRs are not rewritten to pretend the old decision never existed.
