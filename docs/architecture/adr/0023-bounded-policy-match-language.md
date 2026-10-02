# ADR-0023: Bound Policy matching to stable execution metadata and reviewed Governance Facts

- Status: Accepted
- Date: 2026-09-27

> **Supersession note (2026-10-02):** ADR-0032 supersedes this ADR's invocation-only Match shape and deferred-Principal decision. Policy now requires explicit governance Actions and may match bounded Principal kind/reference; its raw-argument/secret minimization and reviewed Governance Facts constraints remain active.

## Context

PolicyRequest currently contains canonical Tool metadata, InvocationContext, bounded Governance Facts, Caller, and a CredentialResolver availability boolean.

A persistent Policy model must not turn that request into unrestricted access to raw Tool arguments, secret values, arbitrary environment state, model prompts, Provider protocol frames, or invocation results.

It must also avoid treating Caller as an authenticated identity. Caller is adapter-supplied descriptive metadata, not a Principal.

Governance Facts are explicitly bounded, but the default extractor reports unknown for Tools without a reviewed fact mapper.

If a rule that expects an effect/resource silently treats unknown facts as safe, a default-allow configuration can bypass the intended rule.

## Decision

Policy Rule Match supports only the following initial dimensions:

~~~text
Origins[]
ScopeKinds[]
ProfileRefs[]
ProviderIDs[]
ToolIDs[]
FactKnowledge
EffectsAny[]
ResourcesAny[]
~~~

Empty selector lists mean wildcard for that dimension.

Across dimensions, conditions are AND.

Within each list, values are OR.

### Origins

Supported values mirror stable Invocation origins:

~~~text
cli
mcp
management_http
desktop
internal
~~~

Origin is adapter provenance, not user identity.

### Scope

ScopeKinds supports catalog and profile.

ProfileRefs contains canonical Profile references used by Invocation.

When ProfileRefs is non-empty, only matching Profile scope can satisfy the rule.

Normal UI selects Profiles by human-readable slug/display metadata and stores the canonical reference internally.

### Provider and Tool identity

ProviderIDs and ToolIDs use canonical Mender identity.

Normal UI selects Providers/Tools from existing inventory and never asks the user to type opaque IDs.

Policy Application validates references before persisting a Rule.

Deleted referenced resources must either be blocked by usage checks or cause an explicit management conflict. Silent dangling references are not accepted.

### Fact knowledge

FactKnowledge has:

~~~text
any
known
unknown
~~~

The zero/default value at request boundaries normalizes to any.

A rule may explicitly match unknown facts.

This is the recommended mechanism for policies such as unknown facts -> require approval or unknown facts -> deny.

### Effect predicates

EffectsAny is a bounded list of effect classifications.

A rule with EffectsAny matches when Governance Facts are known and at least one requested effect exists in the invocation fact set.

If Facts are unknown, an EffectsAny rule does not match.

Users that require fail-closed behavior for unknown facts must add an explicit FactKnowledge=unknown rule.

### Resource predicates

ResourcesAny contains bounded matchers:

~~~text
ResourceMatcher
  Kind
  Operator
  Value
~~~

Initial operators are exact and prefix.

Regex, glob, arbitrary expressions, and user code are not supported.

A Resource matcher matches only known Governance Facts.

exact requires the fact value to equal Value.

prefix requires the fact value to begin with Value.

At least one resource matcher must match for the ResourcesAny dimension to match.

### Fact predicate combination

If a Rule contains both EffectsAny and ResourcesAny, both dimensions must match.

Example:

~~~text
origin = mcp
effect = filesystem.write
resource = path prefix /workspace/release
~~~

requires all three dimensions.

### Unknown facts are explicit, not magical

Phase 12 does not globally force unknown facts to deny/approval because every upstream MCP Tool currently defaults to unknown unless a reviewed extractor exists. Such a migration would make existing installations fail closed immediately.

Instead:

- migration keeps DefaultDecision=allow;
- fact-dependent rules do not match unknown facts;
- UI explains unknown-fact behavior;
- users may add an explicit unknown-fact rule;
- future phases may add reviewed fact extractors per Tool/Provider.

### Caller is not a rule dimension

Caller is excluded from persistent Rule Match in Phase 12.

It is descriptive adapter metadata and must not become an authorization identity.

Authenticated Principal matching remains deferred until a real identity/authentication model exists.

### Credential identity is not a rule dimension

Credential References and secret values remain outside PolicyRequest in Phase 12.

Policy may govern a Tool/Provider/Profile that happens to require Credentials, but it does not match individual Credential identity.

This preserves Phase 11 minimization boundaries.

Future non-secret Credential metadata rules require a separate ADR defining exactly which metadata crosses the Policy boundary.

### Raw arguments remain excluded

Policy never receives the raw JSON argument document.

Only reviewed Governance Facts may represent argument-derived security meaning.

### Validation limits

Policy Match has bounded cardinalities and string lengths.

Application validation rejects unknown origins/scope kinds/fact knowledge/operators, duplicate references, nonexistent Tool/Provider/Profile references, resource matchers with empty kind/value, and fact strings exceeding Governance Fact bounds.

## Alternatives considered

### Match raw JSON paths in Tool arguments

Rejected because it couples Policy to arbitrary Tool schemas, exposes raw user/model input to Policy, and recreates an unbounded expression language.

### Match Caller strings

Rejected because Caller is not authenticated identity.

### Regex resource matchers

Rejected initially because regex complexity and surprising matching behavior are unnecessary for the first editable Policy model.

### Treat unknown facts as an empty known fact set

Rejected because unknown and known-empty have different security meaning.

### Automatically deny all unknown facts

Rejected as the migration default because current MCP Tools commonly have no reviewed fact extractor.

## Consequences

### Positive

- Policy consumes a small auditable vocabulary;
- no raw arguments or secrets cross into Policy;
- unknown facts remain distinguishable from known-empty facts;
- rules can immediately govern by origin, Profile, Provider, and Tool even before richer fact extraction exists;
- future reviewed effect/resource extractors fit without changing the Policy Rule model.

### Negative / trade-offs

- effect/resource rules are only useful when a reviewed extractor can provide known facts;
- callers cannot yet be governed as identities;
- Credential-specific policy remains deferred;
- exact/prefix matching is intentionally less expressive than general policy languages.

## Follow-up

Phase 12 implements persistent rule management and configured evaluation. Reviewed Tool/Provider-specific fact extractors and authenticated Principal matching remain separate work.
