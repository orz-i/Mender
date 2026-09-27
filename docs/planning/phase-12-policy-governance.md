# Phase 12 Plan — Persistent Policy Governance

Status: Planned

## Objective

Replace the local AllowAll Policy placeholder with a persistent, editable, deterministic PolicySet that evaluates stable Invocation metadata and bounded Governance Facts.

Phase 12 adds:

- persistent default Policy decision;
- unordered rules with deterministic most-restrictive composition;
- bounded matching by origin/scope/Profile/Provider/Tool/Governance Facts;
- management CRUD through CLI/OpenAPI/Wails/Web/Desktop;
- one shared evaluator in the Invocation pipeline;
- end-to-end Audit and security coverage.

Profile ToolBinding policy remains a second tightening layer and cannot weaken the global Policy result.

## Architectural baseline

Phase 12 follows ADR-0007, ADR-0017, ADR-0019, ADR-0022, and ADR-0023.

## Problem statement

The current Policy runtime is intentionally minimal:

~~~text
AllowAll
  -> allow/local.default
~~~

This was appropriate before there was a stable Invocation context, Governance Facts, ToolBinding governance, and Credential scope.

It is no longer sufficient because:

- administrators cannot define global execution policy;
- the Policy management page is read-only;
- origin/Profile/Provider/Tool distinctions cannot be governed centrally;
- Governance Facts exist but have no persistent consumer;
- default-deny operation is impossible;
- all configurable restrictions currently live only at Profile ToolBinding level.

At the same time, a full arbitrary policy language would be premature and risky.

Phase 12 therefore implements a deliberately bounded Policy model rather than a general-purpose expression engine.

## Scope

Phase 12 includes:

1. Policy RuleID and immutable Rule/PolicySet Domain models;
2. persistent DefaultDecision;
3. deterministic unordered rule evaluation;
4. Match selectors for origin, scope kind, Profile reference, ProviderID, ToolID, fact knowledge, effects, and resources;
5. exact/prefix resource matching only;
6. Policy application use cases and reference validation;
7. Policy SQLite schema version 1;
8. behavior-compatible migration to default allow with zero rules;
9. Invocation-to-Policy anti-corruption adapter;
10. configured Runtime evaluator replacing local AllowAll;
11. existing ToolBinding PolicyConstraint applied after configured Policy;
12. Policy status upgraded from local_allow_all/read-only to configured/editable;
13. management HTTP/OpenAPI/SDK contracts;
14. Wails/Desktop bindings;
15. CLI Policy management;
16. list-first Web/Desktop Policy management;
17. deterministic Audit reason codes;
18. reference/deletion integration where lifecycle permits;
19. end-to-end tests for default allow/default deny/rule overlap/Profile tightening;
20. security and race verification.

## Non-goals

Phase 12 does not include:

- authenticated Principal/user/role policy;
- Caller-string authorization;
- Credential Reference or secret matching;
- raw Tool argument access;
- JSONPath/JQ/expression-language matching;
- regex/glob resource matchers;
- JavaScript/Rego/CEL/custom policy code;
- rule priority or ordering;
- interactive approval UI;
- remote policy distribution;
- organization/team policy inheritance;
- reviewed fact extraction for every MCP Tool;
- result/output policy rules;
- time-of-day rules;
- network CIDR parser or filesystem path canonicalization beyond reviewed Governance Facts.

## Ubiquitous language

### PolicySet

One Runtime-wide Policy configuration.

~~~text
PolicySet
  DefaultDecision
  Rules[]
~~~

### DefaultDecision

The decision returned when no enabled Rule matches.

Values:

~~~text
allow
require_approval
deny
~~~

### Rule

One independently managed Policy rule.

~~~text
Rule
  RuleID
  DisplayName
  Enabled
  Decision
  Match
  CreatedAt
  UpdatedAt
~~~

### RuleID

Mender-generated stable identity.

Normal UI never asks users to enter it.

### Match

A bounded set of selectors.

~~~text
Match
  Origins[]
  ScopeKinds[]
  ProfileRefs[]
  ProviderIDs[]
  ToolIDs[]
  FactKnowledge
  EffectsAny[]
  ResourcesAny[]
~~~

### ResourceMatcher

~~~text
ResourceMatcher
  Kind
  Operator
  Value
~~~

Operator is exact or prefix.

## Decision composition

### No matching rules

~~~text
effective = DefaultDecision
~~~

### One or more matching rules

DefaultDecision is ignored.

Matching decisions combine:

~~~text
allow < require_approval < deny
~~~

The most restrictive matching rule wins.

If multiple matching rules share the winning decision, the lexicographically smallest RuleID supplies the reason code.

### ToolBinding composition

After global Policy evaluation:

~~~text
configured Policy decision
        +
Profile ToolBinding constraint
        ↓
Invocation effective decision
~~~

The existing ToolBinding composition remains unchanged.

## Rule reason codes

Default decisions:

~~~text
policy.default.allow
policy.default.require_approval
policy.default.deny
~~~

Rule decisions use a stable reason derived from RuleID.

Audit stores the final effective kind/reason only.

## Match semantics

Empty selectors mean wildcard.

Across dimensions: AND.

Within a selector list: OR.

### Origins

~~~text
cli
mcp
management_http
desktop
internal
~~~

### ScopeKinds

~~~text
catalog
profile
~~~

### ProfileRefs

Canonical ProfileID strings.

A non-empty ProfileRefs dimension can only match Profile scope.

### ProviderIDs / ToolIDs

Canonical Catalog identities.

### FactKnowledge

~~~text
any
known
unknown
~~~

Request-boundary empty normalizes to any.

### EffectsAny

Requires known Governance Facts.

Matches when at least one configured effect is present.

### ResourcesAny

Requires known Governance Facts.

Matches when at least one ResourceMatcher matches one ResourceFact.

### Fact combination

When EffectsAny and ResourcesAny are both non-empty, both must match.

## Unknown Governance Facts

Unknown is not known-empty.

Effect/resource conditions do not match unknown facts.

Migration does not globally deny unknown facts because current generic MCP Tools use UnknownExtractor.

Administrators can explicitly create unknown-fact require-approval or deny rules.

The UI explains this distinction.

## Policy Domain invariants

### RuleID

Generated opaque stable ID with prefix rule_.

### DisplayName

Non-empty, trimmed, bounded human-readable name.

### Decision

Only allow, require_approval, deny.

### Enabled

Explicit boolean.

### Selector bounds

Every selector list has a bounded maximum size.

Duplicate values normalize consistently.

### Origin/scope/fact enums

Unknown enum values fail closed.

### Effects

Trimmed, non-empty, bounded strings compatible with Governance Fact limits.

### Resources

Kind/value are trimmed and bounded.

Operator is exact or prefix.

### Cross-field validation

- ProfileRefs with ScopeKinds restricted only to catalog is invalid.
- EffectsAny/ResourcesAny with FactKnowledge=unknown is invalid because such facts cannot match.
- A Rule with every selector empty is valid but is explicitly global and the UI warns.
- Rule decision allow is valid in global Policy even though ToolBinding has no allow.

## Policy Application

Ports:

~~~text
Repository
RuleIDGenerator
Clock
ReferenceLookup
~~~

ReferenceLookup validates current Profiles, Providers, and Tools.

Use cases:

~~~text
GetPolicy
SetDefaultDecision
CreateRule
UpdateRule
DeleteRule
SetRuleEnabled
Evaluate
~~~

### Reference validation

Create/Update validates configured references before persistence.

Normal UI chooses references from current inventories.

Provider deletion should report Policy references as in-use where the existing deletion boundary permits.

Tool removal caused by upstream synchronization is not silently retargeted to another Tool. A stale ToolID selector can never match a different Tool identity. Management projections should surface unresolved references when encountered rather than inventing replacement identity.

## Policy SQLite

Schema version 1 owns:

~~~text
policy_schema_migrations
policy_settings
policy_rules
~~~

policy_settings has one row containing DefaultDecision and updated_at.

policy_rules stores:

- rule_id;
- display_name;
- enabled;
- decision;
- match_json;
- created_at;
- updated_at.

Match JSON is non-secret bounded configuration and is fully validated on restore.

Migration inserts default allow when no settings exist.

Unknown future versions fail closed.

Malformed stored decision/match state fails closed.

## Evaluator boundary

Policy Domain/Application does not import Invocation application types.

Bootstrap owns an adapter:

~~~text
Invocation PolicyRequest
        ↓
map stable fields
        ↓
Policy EvaluateRequest
        ↓
configured Policy evaluator
        ↓
Policy Decision
~~~

Mapped fields are Origin, Scope kind/reference, ProviderID, ToolID, and Governance fact knowledge/effects/resources.

Caller is intentionally not mapped as identity.

CredentialRefs and secrets are not mapped.

## Runtime composition

Runtime gains a Policy composition root:

~~~text
Policy
  Get
  SetDefault
  CreateRule
  UpdateRule
  DeleteRule
  SetRuleEnabled
  Evaluator
  Close
~~~

New Runtime startup opens the Policy SQLite store before composing InvokeTool.

InvokeTool receives the configured evaluator rather than policylocal.AllowAll.

RejectApproval remains the ApprovalGate default.

## Policy status

Existing status endpoint remains for compatibility but changes semantics.

Conceptual response:

~~~text
mode = configured
editable = true
approval = fail_closed_when_required
defaultDecision
ruleCount
~~~

Policy rules live under the full Policy resource rather than the status object.

## Management API

Initial operations:

~~~text
GET    /policy
PUT    /policy/default
POST   /policy/rules
PUT    /policy/rules/{ruleId}
DELETE /policy/rules/{ruleId}
POST   /policy/rules/{ruleId}/enable
POST   /policy/rules/{ruleId}/disable
GET    /policy/status
~~~

Create request does not accept RuleID.

Responses include RuleID as secondary identity.

All selectors are non-secret.

## CLI

Initial commands:

~~~text
mender policy show
mender policy set-default <allow|require_approval|deny>

mender policy rule add
mender policy rule remove <rule-id>
mender policy rule enable <rule-id>
mender policy rule disable <rule-id>
~~~

Rule add supports bounded repeated selectors for origins/scope/Profile/Provider/Tool/effects and exact/prefix resources.

Advanced CLI may expose stable IDs/reference strings because it is an administrative interface.

Normal Web/Desktop management resolves human selections and does not ask for opaque IDs.

## Web/Desktop UX

Policy becomes list-first.

### Summary

Top section shows Default decision, enabled Rule count, and Approval behavior.

Default decision has its own focused edit Dialog.

### Rules

Rule table shows Display name, Decision badge, Enabled/disabled, compact scope/target summary, and actions.

Raw RuleID is not a primary column.

### Rule editor

To preserve one-screen-one-category, Add/Edit uses a stepped Dialog:

~~~text
1. Decision
   - display name
   - enabled
   - allow / require approval / deny

2. Scope and targets
   - origins
   - Catalog/Profile scope
   - Profile selectors
   - Provider selectors
   - Tool selectors

3. Governance Facts
   - fact knowledge
   - effect selectors
   - resource matchers
~~~

Only one step is visible at a time.

Provider/Profile/Tool selectors are inventory-backed.

There is no raw ID text field in normal UI.

The Facts step states that effect/resource selectors do not match unknown facts.

A global wildcard Rule shows a warning before save.

require_approval shows that the current Approval adapter fails closed.

## Reference lifecycle

### Provider deletion

Existing Provider deletion usage checks are extended to include Policy Provider selectors where practical.

### Profile deletion

If a Profile deletion use case is present, Policy Profile selectors participate in its usage check.

### Tool disappearance on sync

Catalog synchronization may remove a Tool because the upstream Provider no longer exposes it.

Policy never retargets that selector.

The persisted ToolID becomes inactive.

Management projection should display missing/unresolved target metadata until the Rule is edited or removed.

This is safer than matching a new Tool by name.

## Security invariants

1. Rule order never changes evaluation.
2. Any matching deny dominates all matching allow/approval rules.
3. DefaultDecision is used only when no Rule matches.
4. ToolBinding policy can only tighten the configured global result.
5. Policy receives no raw arguments.
6. Policy receives no secret values.
7. Policy receives no CredentialRefs in Phase 12.
8. Caller is not treated as authenticated identity.
9. Unknown facts remain distinguishable from known-empty facts.
10. Fact predicates do not match unknown facts.
11. Regex/custom code is not supported.
12. Normal UI does not ask for RuleID/ProviderID/ProfileID/ToolID.
13. Rule reference validation occurs before persistence.
14. SQLite restore fails closed on malformed/future state.
15. Audit stores bounded effective reason codes, not full rule match details.
16. require_approval still fails closed without interactive Approval.
17. Existing installations preserve allow behavior at migration.

## Test strategy

### Policy Domain

Cover DefaultDecision validation, Rule invariants, selector normalization, resource exact/prefix validation, wildcard Rule, unordered most-restrictive evaluation, deterministic same-decision reason selection, and default-deny allowlist behavior.

### Policy Application

Cover reference validation, create/update/delete/enable/disable, invalid/missing references, repository errors, and evaluation requests.

### SQLite

Cover fresh migration default allow, full Rule round-trip, match_json validation, future-version fail closed, malformed decision/matcher fail closed, and timestamps.

### Invocation integration

Cover configured default allow, default deny, matching allow over default deny, deny over allow, require_approval, and ToolBinding tightening.

### Governance Facts

Cover known effect match, known resource exact/prefix, unknown non-match, and explicit unknown-fact Rule matching.

### Adapters

Cover configured/editable status, RuleID generation on create, generated OpenAPI/Wails contracts, CLI validation, and UI inventory selectors.

### Audit

Cover effective default reason, effective Rule reason, ToolBinding tightening reason, and absence of raw match details.

### Repository validation

Before closeout:

~~~text
pnpm check
pnpm security:check
~~~

## Work packages and commit segmentation

### 1. Architecture decisions

Commit:

~~~text
docs: define policy governance architecture
~~~

### 2. Phase plan and architecture/product docs

Commit:

~~~text
docs: plan policy governance phase
~~~

### 3. Policy Domain and application

Commit:

~~~text
feat(policy): add persistent rule domain and use cases
~~~

### 4. Policy SQLite persistence

Commit:

~~~text
feat(policy): persist policy configuration
~~~

### 5. Configured evaluator

Commit:

~~~text
feat(policy): evaluate configured policy rules
~~~

### 6. Runtime/Invocation composition

Commit:

~~~text
feat(invocation): compose persistent policy evaluation
~~~

### 7. Adapter contracts

Commit:

~~~text
feat(adapters): expose policy governance configuration
~~~

### 8. Management UX

Commit:

~~~text
feat(app): manage policy rules
~~~

### 9. End-to-end governance coverage

Commit:

~~~text
test(runtime): cover persistent policy governance end to end
~~~

### 10. Closeout

Commit:

~~~text
docs: complete policy governance phase
~~~

## Exit criteria

- [ ] PolicySet is the canonical persistent global Policy model.
- [ ] Fresh migration preserves current allow behavior.
- [ ] Rules have no priority/order semantics.
- [ ] No-match uses DefaultDecision.
- [ ] Matching rules use deny > require_approval > allow.
- [ ] Default deny plus explicit allow rules works.
- [ ] same-decision Rule reason selection is deterministic.
- [ ] Rule Match supports origin/scope/Profile/Provider/Tool selectors.
- [ ] FactKnowledge distinguishes any/known/unknown.
- [ ] known effect/resource exact/prefix rules evaluate correctly.
- [ ] effect/resource rules do not match unknown facts.
- [ ] Policy receives no raw arguments, secrets, Caller identity, or CredentialRefs.
- [ ] reference validation occurs before Rule persistence.
- [ ] Policy SQLite future/malformed state fails closed.
- [ ] Runtime uses configured evaluator instead of local AllowAll.
- [ ] ToolBinding constraint remains a second tightening layer.
- [ ] require_approval continues through the existing fail-closed ApprovalGate.
- [ ] Audit records stable effective Policy reasons.
- [ ] Policy status reports configured/editable/default/rule count.
- [ ] CLI/OpenAPI/Wails contracts manage the same Policy model.
- [ ] normal Web/Desktop Rule management uses inventory selectors and no raw IDs.
- [ ] Policy editor separates Decision, Scope/Targets, and Governance Facts.
- [ ] Go race tests pass.
- [ ] pnpm check passes.
- [ ] pnpm security:check passes.

## Deferred follow-up

- interactive Approval workflows;
- authenticated Principal/user/role matching;
- reviewed Governance Fact extractors for more Tools/Providers;
- Credential metadata predicates under a separate minimization ADR;
- result/output/DLP Policy;
- organization/team Policy layering;
- remote Policy distribution;
- scheduled/time-window rules;
- richer network/filesystem semantic matchers.
