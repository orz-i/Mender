# ADR-0019: Make Profile ToolBinding the per-Profile governance unit

- Status: Accepted
- Date: 2026-09-26

## Context

Mender now treats Profile scope as an authoritative Invocation security boundary.

The Profile Domain still models one selected Tool as ToolSelection with only ToolID and Direct. That is sufficient for membership and MCP exposure, but it cannot express Profile-specific aliases, presentation metadata, stricter execution policy, or execution constraints.

Those properties cannot belong to the canonical Catalog Tool because the same Tool may participate in multiple Profiles with different behavior. They also cannot live only in MCP, CLI, or UI adapters because execution governance must remain adapter-independent.

Mender does not yet have a configurable Policy store, interactive Approval implementation, or Credential inventory/store. Phase 10 must therefore avoid persisting governance settings that cannot be validated or enforced.

## Decision

Profile will replace the internal ToolSelection concept with a first-class ToolBinding.

A ToolBinding is the canonical relationship between one Profile and one canonical Tool.

Initial state:

~~~text
ToolBinding
  ToolID
  Direct
  Alias?
  TitleOverride?
  DescriptionOverride?
  PolicyConstraint
  ConcurrencyConstraint
~~~

All fields belong to the Profile bounded context. ToolBinding does not copy Tool schemas, Provider configuration, credentials, or arbitrary policy JSON.

### Stable identity

ToolID remains the canonical Tool identity.

Alias and presentation overrides never replace ToolID in persistence, Invocation, Audit, or internal references.

Normal management UX continues to select Tools from the Catalog and must not require users to type ToolID or a new opaque binding identifier.

ToolBinding has no separate BindingID in Phase 10. The pair ProfileID + ToolID remains the relationship key.

### Exposure and presentation

Direct keeps its existing meaning and remains normalized by Profile exposure mode:

- direct Profile: every binding is direct;
- dynamic Profile: no binding is direct;
- hybrid Profile: each binding chooses direct exposure.

Alias is an optional Profile-local machine/model-facing Tool name override.

It does not replace Provider namespace isolation. Direct MCP export remains conceptually:

~~~text
provider namespace + "__" + (binding alias or canonical Tool name)
~~~

TitleOverride and DescriptionOverride are optional Profile-specific presentation values. When absent, adapters use canonical Catalog metadata.

Overrides affect Profile projections only. They do not mutate Catalog, synchronization results, provenance, schemas, tags, or source keys.

Phase 10 does not make Catalog lexical ranking override-aware. Profile projections may return overridden metadata for a Tool selected by canonical discovery.

### Restrictive policy constraint

A ToolBinding cannot grant execution that global Policy denied.

Initial PolicyConstraint values:

~~~text
inherit
require_approval
deny
~~~

There is intentionally no binding-level allow.

Invocation combines the global Policy decision with the binding constraint using most-restrictive-wins ordering:

~~~text
allow < require_approval < deny
~~~

Examples:

- global allow + inherit -> allow;
- global allow + require_approval -> require_approval;
- global allow + deny -> deny;
- global require_approval + inherit -> require_approval;
- global require_approval + deny -> deny;
- global deny + any binding constraint -> deny.

A Policy evaluation error remains an internal failure and cannot be masked by ToolBinding.

Binding-generated decisions use stable reason codes such as profile.binding.require_approval and profile.binding.deny.

The existing ApprovalGate remains authoritative. Because the current local gate fails closed, require_approval is enforceable before interactive approval UX exists.

### Binding-local concurrency constraint

Initial ConcurrencyConstraint values:

~~~text
inherit
serial
~~~

serial means executions of the same ProfileID + ToolID binding do not overlap.

It does not claim Provider-wide or resource-wide serialization. It does not constrain Catalog-scope calls or another Profile's binding.

The Runtime ExecutionGuard remains the baseline constraint. Binding-local serialization is additive and cannot relax the baseline guard.

### Invocation boundary

ADR-0017's Profile membership boundary evolves from a boolean membership check into binding-governance resolution.

Conceptually:

~~~text
Profile scope + ToolID
        |
        v
resolve Profile ToolBinding
        |
        +--> absent: rejected/tool_outside_scope
        |
        v
normalized scoped governance
        |
        +--> restrictive policy constraint
        +--> binding concurrency constraint
~~~

Invocation does not import Profile persistence or Profile repository types directly.

Bootstrap/application composition adapts the Profile-owned ToolBinding into an Invocation-owned scoped-governance projection.

Presentation fields do not enter Invocation because they are projection concerns, not execution semantics.

### Persistence

The existing profile_tools row becomes the persisted ToolBinding row.

Profile SQLite schema version 2 adds columns for alias, title override, description override, policy constraint, and concurrency constraint.

Existing rows migrate to behavior-preserving defaults:

~~~text
alias = ""
title_override = ""
description_override = ""
policy_constraint = "inherit"
concurrency_constraint = "inherit"
~~~

Profile storage retains ADR-0014 forward-only migration and future-version fail-closed behavior.

### External contracts

Management/OpenAPI/Wails/CLI Profile projections extend the existing Tool membership object rather than create a second governance resource.

New request fields are optional so existing clients that send only ToolID and Direct retain current behavior. Responses expose normalized ToolBinding configuration.

Generated SDK and Wails bindings remain generated artifacts.

### Management UX

Tool membership selection and ToolBinding configuration are separate user tasks.

The Catalog-backed Choose tools dialog remains the membership workflow and never asks for ToolID.

Per-binding configuration opens from an already-selected Tool.

To preserve the product rule that one surface should carry one category of information:

- presentation/exposure configuration is separate from execution governance;
- long descriptions are truncated in lists and edited/viewed in focused surfaces;
- raw stable IDs remain secondary metadata.

Phase 10 does not put all binding fields into the Tool picker.

### Credential scope is deferred

ToolBinding is the intended future home of Credential scope, but Phase 10 does not persist Credential references/scopes.

Credential currently has an opaque reference parser and unavailable resolver, but no persistent inventory a user can browse or an application boundary that can validate Profile-to-Credential references.

Adding Credential scope now would create dead configuration and likely reintroduce manual opaque-reference entry.

A later Credential governance phase may extend ToolBinding once inventory, validation, selector UX, and scoped resolver enforcement exist.

## Alternatives considered

### Put aliases/policy/concurrency on Catalog Tool

This would make Profile-specific behavior global and allow one use case to mutate another Profile's governance.

### Keep ToolSelection and store governance in adapters

This would duplicate semantics across MCP, CLI, Web, and Desktop and make execution constraints bypassable.

### Add a separate BindingID

The relationship already has the stable composite identity ProfileID + ToolID. A new opaque ID adds complexity without an independent lifecycle.

### Allow binding-level allow

This could weaken a global deny/approval decision and create a privilege-escalation path.

### Add Provider-wide concurrency per binding

One binding cannot reliably serialize against Catalog-scope or other Profile calls unless every invocation participates in the same Provider-wide rule. Phase 10 therefore uses binding-local serialization.

### Persist Credential references now

Without Credential inventory and validation this would be an opaque string field with incomplete UX and enforcement.

### Copy canonical Tool schemas into ToolBinding

Schemas remain Catalog-owned. Duplicating them would introduce synchronization and provenance drift.

## Consequences

### Positive

- Profile becomes the concrete Profile × Tool governance unit described by the architecture.
- Presentation customization no longer contaminates canonical Catalog metadata.
- Profile configuration can tighten execution without overriding global Policy.
- require_approval and deny are immediately enforceable.
- Binding-local serialization reuses the Phase 9 execution stage without pretending to solve global resource locking.
- Existing Profile rows and clients retain prior behavior through defaults.
- Future Credential scope has a clear extension point without dead configuration now.
- Management UX stays human-readable and Catalog-backed.

### Negative / trade-offs

- Replacing ToolSelection touches Profile Domain, persistence, API projections, Desktop bindings, CLI JSON, and UI state.
- Profile SQLite requires a version 2 migration.
- Profile-scoped MCP projection gains collision validation for effective names.
- Catalog search ranking remains canonical rather than override-aware.
- Binding-local serial does not serialize Catalog scope or another Profile.
- require_approval currently fails closed because no interactive Approval adapter exists.

## Follow-up

Phase 10 implements ToolBinding, projection overrides, restrictive binding policy, and binding-local concurrency across all adapters.

Credential inventory/scope, editable global Policy rules, interactive approval, resource-key-specific execution guards, and override-aware search ranking remain separate planning items.
