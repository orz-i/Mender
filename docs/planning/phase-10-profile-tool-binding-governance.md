# Phase 10 Plan — Profile ToolBinding Governance

Status: Complete

## Objective

Upgrade Profile from a Tool-membership/exposure list into the concrete per-Profile Tool governance model.

Phase 10 replaces internal ToolSelection with ToolBinding and makes one Profile-to-Tool relationship carry enforceable Profile-specific behavior:

- direct/dynamic exposure state;
- optional machine/model alias;
- optional title/description presentation overrides;
- a restrictive execution policy constraint;
- a binding-local concurrency constraint.

The phase builds on Phase 9's governed Invocation runtime.

It does not introduce a second execution path, mutate canonical Catalog Tool metadata, or add configuration that cannot yet be validated and enforced.

## Architectural baseline

Phase 10 is governed by ADR-0003, ADR-0007, ADR-0008, ADR-0010, ADR-0011, ADR-0014, ADR-0017, ADR-0018, and ADR-0019.

## Problem statement

Phase 9 completed governed execution, but Profile still persists only ToolID and Direct.

That leaves several architectural promises unimplemented.

### Profile-specific presentation has no home

The same canonical Tool may need a different model-facing alias or description in different Profile contexts.

Changing Catalog would make that global. Changing only MCP would make it protocol-specific.

### Profile cannot tighten execution policy

Invocation receives Profile scope but currently learns only whether Tool membership exists.

It cannot distinguish inherited policy, Profile-required approval, or a Profile-local deny.

### Profile cannot express binding-local serialization

Phase 9 established ExecutionGuard, but Profile supplies no binding-specific execution constraint.

### Current Profile UI must remain scalable

The Tool picker already needs to scale to large Catalogs. Adding policy, alias, descriptions, and concurrency controls directly into the picker would recreate the dense UI problems already corrected in this project.

### Credential scope is not ready

Credential has no persistent inventory/store or selector.

Persisting Credential references now would create unvalidated dead configuration and manual opaque-reference entry.

## Scope

Phase 10 includes:

1. replace internal Profile ToolSelection with immutable ToolBinding;
2. preserve ProfileID + ToolID as relationship identity;
3. add optional alias/title/description overrides;
4. add PolicyConstraint = inherit | require_approval | deny;
5. add ConcurrencyConstraint = inherit | serial;
6. preserve direct/dynamic/hybrid normalization;
7. add Profile SQLite schema v2 migration with behavior-preserving defaults;
8. evolve Invocation Profile lookup into scoped binding-governance resolution;
9. combine binding policy with global Policy using most-restrictive-wins semantics;
10. add binding-local serialization on ProfileID + ToolID in addition to baseline ExecutionGuard;
11. apply binding presentation overrides to downstream MCP direct/describe/search-result projections;
12. extend CLI/OpenAPI/Wails Profile contracts with ToolBinding fields;
13. update Web/Desktop Profile management with separate presentation and execution-governance surfaces;
14. preserve Catalog-backed Tool selection with no manual ToolID workflow;
15. add migration, security, adapter, UI, and end-to-end tests.

## Non-goals

Phase 10 does not include:

- Credential inventory/store CRUD;
- Credential scope/reference configuration on ToolBinding;
- manual Credential reference entry;
- editable global Policy storage/DSL;
- conditional per-binding policy expressions;
- a binding-level allow override;
- interactive approval UX;
- Provider-wide or resource-key-specific concurrency configured by ToolBinding;
- per-binding timeout/result-size overrides;
- Catalog schema/source metadata overrides;
- Profile override-aware Catalog lexical ranking;
- authenticated remote principals;
- public progress streaming;
- automatic retry policy;
- a separate ToolBinding ID.

## Ubiquitous language

### ToolBinding

The Profile-owned relationship between one Profile and one canonical Tool.

~~~text
ToolBinding
  ToolID
  Direct
  Alias
  TitleOverride
  DescriptionOverride
  PolicyConstraint
  ConcurrencyConstraint
~~~

Empty alias/title/description values mean inherit canonical metadata after normalization.

### Effective Tool projection

~~~text
EffectiveName
  = Alias if present
  = Tool.Name otherwise

EffectiveTitle
  = TitleOverride if present
  = Tool.Title otherwise

EffectiveDescription
  = DescriptionOverride if present
  = Tool.Description otherwise
~~~

ToolID, ProviderID, schemas, tags, provenance, and source key remain canonical Catalog values.

### PolicyConstraint

~~~text
inherit
require_approval
deny
~~~

There is no allow.

### Effective Policy Decision

Global Policy and ToolBinding are combined by:

~~~text
allow < require_approval < deny
~~~

The most restrictive value wins.

### ConcurrencyConstraint

~~~text
inherit
serial
~~~

serial means no overlapping execution for the same ProfileID + ToolID through Profile scope.

### Scoped Binding Governance

The Invocation-owned normalized projection returned by the Profile scope boundary.

~~~text
ScopedBindingGovernance
  ProfileReference
  PolicyConstraint
  ConcurrencyConstraint
~~~

It excludes alias/title/description because presentation is not an Invocation concern.

## ToolBinding Domain invariants

### Tool reference

Every binding contains a valid canonical ToolID.

Duplicate ToolID bindings inside one Profile are rejected.

### Direct normalization

~~~text
direct  -> Direct = true
dynamic -> Direct = false
hybrid  -> preserve requested Direct
~~~

### Alias

Alias is optional.

When non-empty it must be trimmed, use the machine-name character set A-Z/a-z/0-9/underscore/dot/hyphen, begin and end with an alphanumeric character, and stay within a bounded length.

Alias is Profile-local and is combined with Provider namespace for direct MCP export.

Application-level replacement validates effective direct-export collisions before persistence.

### Presentation overrides

TitleOverride and DescriptionOverride are optional bounded strings.

Whitespace-only values normalize to empty/inherit.

They cannot change schemas, tags, Provider, source key, or Tool identity.

### Policy constraint

Only inherit, require_approval, and deny are valid.

Unknown persisted values fail closed.

### Concurrency constraint

Only inherit and serial are valid.

Unknown persisted values fail closed.

## Policy composition

Execution becomes:

~~~text
Resolve Profile ToolBinding
        |
        v
Validate Arguments
        |
        v
Derive Governance Facts
        |
        v
Evaluate global Policy
        |
        v
Apply binding PolicyConstraint
        |
        v
Effective Policy Decision
        |
        +--> deny
        +--> require Approval
        |
        v
Execution Guard
~~~

Decision matrix:

| Global decision | Binding constraint | Effective decision |
| --- | --- | --- |
| allow | inherit | allow |
| allow | require_approval | require_approval |
| allow | deny | deny |
| require_approval | inherit | require_approval |
| require_approval | require_approval | require_approval |
| require_approval | deny | deny |
| deny | any | deny |

Binding-generated reason codes are stable Mender codes.

If global Policy errors, Invocation uses the existing internal Policy failure path.

Audit records the effective policy kind/reason.

## Binding-local concurrency

Runtime keeps the Phase 9 baseline ExecutionGuard.

Profile binding serial adds a lock keyed by ProfileID + ToolID.

Semantics:

- same Profile + same Tool cannot overlap;
- another Tool in the same Profile may overlap;
- same Tool through another Profile may overlap;
- Catalog-scope execution is unaffected;
- a stricter baseline Runtime guard still applies.

Guard waiting remains inside Invocation timeout/cancellation.

## Profile scope integration

The current boundary:

~~~text
AllowsTool(profileRef, toolID) -> bool
~~~

evolves to:

~~~text
ResolveBindingGovernance(profileRef, toolID)
  -> found + ScopedBindingGovernance
~~~

Requirements:

- resolution happens before canonical Tool lookup;
- absent binding returns rejected/tool_outside_scope;
- denial exposes no hidden Tool metadata;
- Profile persistence remains behind Bootstrap/application adaptation;
- Invocation imports no SQLite/MCP types;
- presentation overrides do not enter Invocation.

Catalog scope has no Profile binding governance.

## MCP projection

### Direct export

Direct Tool uses Provider namespace plus effective binding name.

Title/description use effective Profile presentation.

Tool schemas remain canonical.

### Dynamic describe

mender_describe_tool returns effective binding name/title/description plus canonical IDs/schemas/tags.

### Dynamic search

Catalog search/ranking remains canonical in Phase 10.

For returned ToolIDs, MCP projection replaces name/title/description with effective binding presentation.

Searching solely by override text is deferred.

### Dynamic call

mender_call_tool continues to send ToolID plus Profile InvocationContext into shared Invocation.

No binding policy/concurrency enforcement lives in MCP.

## CLI behavior

Profile management JSON evolves from membership objects to ToolBinding-shaped objects.

Existing inputs with only toolId/direct remain valid and inherit new defaults.

mender tool call --profile automatically consumes binding policy/concurrency through Invocation.

Catalog-scope Tool calls are unaffected.

Canonical mender tool describe remains Catalog metadata. Profile management commands expose configured overrides.

## Persistence

Profile SQLite schema advances from version 1 to version 2.

profile_tools gains:

~~~text
alias
title_override
description_override
policy_constraint
concurrency_constraint
~~~

Migration defaults preserve Phase 9 behavior.

Coverage includes v1 -> v2 preservation, full round-trip, future-version fail closed, malformed persisted state fail closed, uniqueness, and cascade behavior.

## External contracts

### OpenAPI / SDK

The existing ProfileTool wire schema is extended for compatibility with optional request fields:

~~~text
alias
titleOverride
descriptionOverride
policyConstraint
concurrencyConstraint
~~~

Responses expose normalized values.

Domain terminology becomes ToolBinding while the wire name ProfileTool may remain until a deliberate major API rename.

### Desktop/Wails

Binding structs expose the same fields and are regenerated.

Desktop host maps them into the shared SDK contract.

### Management HTTP

Atomic PUT /profiles/{slug}/tools remains the primary bulk persistence path.

The complete desired binding set is validated before repository commit.

Existing add/remove endpoints remain compatibility conveniences.

## Management UX

Membership and configuration remain separate.

### Choose Tools

The existing Catalog-backed picker remains focused on search/browse, Provider filter, and membership selection. Existing Direct convenience for hybrid mode may remain.

It does not add policy, alias, descriptions, concurrency, or Credential fields.

### Selected Tool management

Each selected Tool row displays compact metadata and governance badges.

Focused actions:

- Presentation: direct export, alias, title override, description override.
- Governance: policy constraint and concurrency constraint.

These categories are not collapsed into one dense form.

Descriptions remain truncated in rows. Full text is edited/viewed in the focused Presentation surface.

Raw ToolID remains secondary metadata, not a required input.

### Governance messaging

The UI explains:

- inherit: normal Runtime behavior;
- require approval: current default Approval implementation fails closed until an approval adapter exists;
- deny: Tool remains selected/discoverable but Invocation is rejected;
- serial: only this Profile/Tool binding is serialized.

No UI implies that binding policy can override a global deny.

## Security invariants

1. ToolBinding cannot change canonical ToolID.
2. ToolBinding cannot override Tool schemas.
3. Profile scope rejects missing bindings before Tool lookup.
4. Binding policy only tightens global Policy.
5. No binding-level allow exists.
6. Binding policy stores no raw arguments.
7. Binding serialization wait stays inside timeout/cancellation.
8. Presentation overrides never change Invocation/Audit identity.
9. Alias collision cannot silently shadow another direct Tool.
10. Profile SQLite v1 rows retain existing behavior after migration.
11. Credential references/secrets are not introduced in Phase 10.
12. Normal UI binding management never requires manual ToolID entry.

## Test strategy

### Profile Domain

Cover ToolBinding defaults, alias bounds, presentation normalization, policy/concurrency enums, Direct normalization, duplicate rejection, immutable accessors, and binding lookup.

### Profile Application

Cover add defaults, atomic full replacement, complete Tool validation, effective direct-name collision rejection, failed-request rollback, and configuration updates without changing membership identity.

### SQLite

Cover v1 -> v2 migration, default preservation, full round-trip, future-version fail closed, malformed state fail closed, uniqueness, and cascade.

### Invocation

Cover the complete global/binding policy matrix, binding deny, require-approval, global deny non-relaxation, Catalog scope, serial wait/cancellation, same-binding serialization, different-binding concurrency, and Audit effective policy classification.

### MCP

Cover alias direct export, effective title/description, describe/search projection, canonical schema preservation, collision behavior, and shared Profile-scoped Invocation.

### CLI / HTTP / Desktop

Cover old request compatibility, new fields round-trip, governed Profile calls, generated contracts, and host mappings.

### Web/Desktop UI

Cover membership-focused picker, governance badges, separate Presentation/Governance surfaces, description truncation, atomic saves, safe errors, and no manual ToolID entry.

### Repository validation

~~~text
pnpm check
pnpm security:check
~~~

## Work packages and commit segmentation

### 1. Architecture decision

Commit:

~~~text
docs: define profile tool binding governance
~~~

Contents: ADR-0019 and ADR index.

### 2. Phase plan and architecture/product docs

Commit:

~~~text
docs: plan profile tool binding governance phase
~~~

Contents: this plan, Roadmap Phase 10, and architecture/product terminology updates. No implementation code.

### 3. ToolBinding Domain model

Commit:

~~~text
refactor(profile): introduce tool binding domain model
~~~

### 4. Profile persistence v2

Commit:

~~~text
feat(profile): persist tool binding governance
~~~

### 5. Profile application and collision validation

Commit:

~~~text
feat(profile): manage tool binding configuration
~~~

### 6. Scoped binding governance in Invocation

Commit:

~~~text
feat(invocation): apply profile binding policy constraints
~~~

### 7. Binding-local execution serialization

Commit:

~~~text
feat(invocation): serialize configured profile bindings
~~~

### 8. MCP Profile projection

Commit:

~~~text
feat(mcp): project profile tool binding metadata
~~~

### 9. Adapter contracts

Commit:

~~~text
feat(adapters): expose profile tool binding configuration
~~~

### 10. Management UX

Commit:

~~~text
feat(app): manage profile tool binding governance
~~~

### 11. End-to-end governance coverage

Commit:

~~~text
test(runtime): cover profile binding governance end to end
~~~

### 12. Closeout

Commit:

~~~text
docs: complete profile tool binding governance phase
~~~

## Exit criteria

Phase 10 is complete only when:

- [x] Profile Domain uses ToolBinding as the canonical Profile-to-Tool relationship.
- [x] Existing Profile rows migrate without behavior change.
- [x] Existing API clients that send only ToolID/direct remain compatible.
- [x] Alias/title/description overrides never mutate Catalog.
- [x] Direct MCP export uses effective alias/presentation safely.
- [x] Dynamic MCP projections return effective binding presentation.
- [x] Profile binding policy supports inherit/require_approval/deny.
- [x] Binding policy cannot weaken global Policy.
- [x] Binding require-approval uses existing ApprovalGate.
- [x] Binding deny is a canonical policy rejection and is audited.
- [x] Binding-local serial execution is enforced inside Invocation timeout/cancellation.
- [x] Baseline Runtime ExecutionGuard remains authoritative/additive.
- [x] Catalog scope remains unaffected by Profile binding constraints.
- [x] Profile SQLite v2 migration and future-version fail-closed tests pass.
- [x] CLI/OpenAPI/Wails contracts expose normalized binding configuration.
- [x] Normal UI binding management never requires manual ToolID.
- [x] Membership selection is not overloaded with governance fields.
- [x] Presentation and execution governance use separate focused UI surfaces.
- [x] Credential references/secrets are not persisted by ToolBinding in Phase 10.
- [x] Go race tests pass.
- [x] pnpm check passes.
- [x] pnpm security:check passes.

## Closeout

Phase 10 completed on 2026-09-27.

Implementation commits:

~~~text
75f6773 docs: define profile tool binding governance
161dd54 docs: plan profile tool binding governance phase
9da344a refactor(profile): introduce tool binding domain model
cfaaa8a feat(profile): persist tool binding governance
796d445 feat(profile): manage tool binding configuration
a6e3552 feat(invocation): apply profile binding policy constraints
8d4db9e feat(invocation): serialize configured profile bindings
aec9bac feat(mcp): project profile tool binding metadata
a9ba6ae feat(adapters): expose profile tool binding configuration
64770ac feat(app): manage profile tool binding governance
ce1c3e6 test(runtime): cover profile binding governance end to end
~~~

Verification evidence:

- `pnpm check` passed after the end-to-end binding governance coverage. OpenAPI SDK regeneration, Wails binding regeneration, desktop architecture checks, golangci-lint, all Go tests, all Go race tests, ESLint, strict TypeScript, 5 Vitest files / 28 tests, and Web/Desktop production builds are green.
- `pnpm security:check` passed. Pinned `govulncheck v1.8.0` found no vulnerabilities in backend or desktop, and production pnpm audit reported no known vulnerabilities at high severity.
- Profile SQLite schema v2 migrates v1 membership rows to behavior-compatible ToolBindings with empty presentation overrides and inherited policy/concurrency defaults; malformed binding state and future schemas fail closed.
- Runtime end-to-end tests verify binding deny, binding require-approval, Catalog-scope independence, and same-binding serial execution. Phase 9 governed execution behavior remains covered.
- MCP tests verify Profile alias/title/description projection for direct and dynamic exposure while canonical ToolID/schemas remain Catalog-owned; normalized exported-name collisions fail closed.
- CLI, management HTTP, OpenAPI SDK, Wails bindings, Desktop host mapping, and shared Web/Desktop UI expose the same normalized ToolBinding contract while old ToolID/direct request shapes remain accepted.
- The Profile management UI keeps Catalog-backed membership selection separate from Presentation and Governance dialogs, preserves existing binding fields during membership/direct changes, truncates long descriptions in rows, and never requires manual ToolID entry.

Credential scope remains intentionally deferred until Mender has a real Credential inventory, validation boundary, selector UX, and scoped resolver enforcement.

## Deferred follow-up

After Phase 10:

- Credential inventory/store plus ToolBinding Credential scope;
- editable global Policy rules consuming GovernanceFacts;
- interactive Approval adapters and UX;
- resource/provider/repository/session keyed guards;
- Profile override-aware search ranking;
- remote authenticated principals;
- public progress streaming;
- result DLP/redaction;
- automatic retry policy using stable failure codes.

Credential scope should be planned together with a real Credential inventory and selector, not as an opaque string field.
