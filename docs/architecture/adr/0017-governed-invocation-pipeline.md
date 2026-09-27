# ADR-0017: Govern invocation with explicit execution scope and preflight/postflight stages

- Status: Accepted
- Date: 2026-09-26

## Context

ADR-0007 established one shared Invocation pipeline so CLI, MCP, Agent Skills, Web, and Desktop execution cannot bypass policy, credentials, timeout, and Audit.

The implementation has since exposed two governance gaps.

First, Profile is documented as an operational and security boundary, but Profile membership is not part of the canonical Invocation request. Downstream MCP adapters enforce membership before calling Invocation, while local CLI and management calls can invoke a Tool directly by ToolID. The same Tool therefore reaches the shared executor through different authorization context depending on the adapter.

Second, Invocation currently proves only that arguments are syntactically valid JSON. It does not validate the JSON instance against the canonical Tool input schema before policy and execution. The pipeline also has no explicit postflight stage for output-schema validation, result-size limits, or future result policy.

Future argument-aware policy creates another constraint: passing arbitrary raw arguments directly into Policy would expand the sensitive-data surface and couple Policy to every Tool schema.

Mender needs a stronger execution contract without turning into an agent framework or moving protocol-specific behavior into Domain.

## Decision

Mender will treat Invocation as a governed execution protocol with explicit execution context and ordered preflight/execution/postflight stages.

### Execution context and scope

Every Invocation request must carry an explicit execution context.

The context contains:

- an execution origin;
- the existing caller/audit label;
- one explicit execution scope.

Initial scope kinds are:

- **Catalog scope**: privileged access to the canonical Catalog subject to normal Policy. This scope exists for trusted local/control-plane workflows and must be selected explicitly by their adapters.
- **Profile scope**: access is limited to one Profile. Invocation resolves the Profile through an application boundary and rejects ToolIDs outside Profile membership before resolving or executing the Tool.

There is no implicit empty scope.

Downstream MCP servers always invoke with Profile scope. Existing local CLI and local management execution may use explicit Catalog scope to preserve local-first administrative behavior. A CLI may additionally opt into Profile scope.

The caller label remains attribution metadata. It is not promoted into an authenticated principal. Remote identity, authentication, and authorization remain separate future decisions.

### Governed pipeline

The shared pipeline becomes:

~~~text
Allocate Invocation ID
        |
        v
Resolve Execution Scope
        |
        +--> Profile scope: load Profile + enforce Tool membership
        |
        v
Resolve Tool
        |
        v
Validate Arguments against Tool Input Schema
        |
        v
Derive bounded Governance Facts
        |
        v
Evaluate Policy
        |
        +--> Deny / Require Approval
        |
        v
Apply Execution Guard / execution budget
        |
        v
Execute through ToolExecutor
        |
        v
Normalize canonical outcome
        |
        v
Apply Result Guard
        |
        +--> output-schema validation
        +--> bounded result limits
        +--> future result policy processors
        |
        v
Record data-minimized Audit
        |
        v
Return canonical Invocation outcome
~~~

Adapters may perform syntax/protocol checks needed to construct the request, but they may not replace the scope, validation, policy, guard, execution, or Audit decisions owned by the shared pipeline.

### Input validation

Invocation validates the argument JSON instance against the canonical Tool input schema before Policy or execution.

Mender treats canonical schemas as JSON Schema 2020-12 documents. Boolean schemas retain their standard semantics.

Schema validation does not mutate arguments. In particular, schema defaults are annotations and are not silently applied as execution inputs.

An argument-validation failure is an expected governed outcome and does not reach the executor.

A schema that Mender itself cannot compile or evaluate is not treated as a caller mistake. It fails closed as a runtime/tool-definition failure and is auditable separately from invalid arguments.

### Governance facts

Policy does not receive arbitrary raw arguments by default.

After input validation, Invocation may ask a governance-fact extractor for a bounded, typed fact set derived from the validated arguments and Tool context. Facts must be designed for policy use rather than act as an alternate copy of arbitrary Tool input.

Initial fact categories may include declared effects and bounded resource identifiers such as filesystem paths or network hosts. Unsupported/generic Tools report an explicit unknown/empty fact state.

Upstream-provided hints are not automatically trusted as security facts.

The existing PolicyRequest is extended with Invocation scope/origin and Governance Facts, not with unrestricted raw JSON.

### Execution guard

Invocation owns an explicit execution-guard stage before provider execution.

The stage exists to support resource-governance rules such as unlimited, provider-serialized, or Tool-serialized execution without teaching adapters or individual providers to bypass the central pipeline.

Phase 9 may keep permissive defaults where no stricter rule is configured. Resource-specific keyed locking is deferred until a concrete Tool/runtime needs it.

The execution timeout/budget covers waiting for the execution guard as well as provider execution so queued work cannot wait forever outside Invocation lifecycle controls.

### Result guard

Executor output is not returned directly to adapters.

A Result Guard receives the Tool, execution context, and normalized outcome. It enforces core postflight invariants before the outcome leaves Invocation.

Phase 9 requires:

- output-schema validation for completed results when a Tool declares an output schema;
- a bounded canonical result-size limit;
- fail-closed handling for invalid or oversized results.

Mender will not silently truncate structured JSON inside the canonical runtime because truncation can change semantics or invalidate schemas. Presentation adapters may independently abbreviate human-facing rendering.

Generic secret/PII/DLP scanning is not introduced in Phase 9. Credential values remain isolated at the execution boundary under ADR-0007, and future result-redaction policy must have an explicit threat model rather than rely on broad heuristic regex replacement.

### Runtime events

Audit remains a durable, data-minimized terminal record. Runtime progress is a separate concern.

Invocation may emit bounded lifecycle/progress events through an observer/reporter port. Events must not contain raw arguments, resolved credentials, or full result payloads by default.

The core event contract may be established in Phase 9 without requiring a public SSE/WebSocket API. A no-op observer remains valid for adapters that do not consume progress.

## Alternatives considered

### Keep Profile checks in each adapter

This preserves the current implementation but makes the security boundary depend on which adapter called Invocation. New adapters could accidentally omit the check.

### Require every invocation to use a Profile

This gives one simple security model but unnecessarily removes the explicit privileged Catalog workflow used by local administration and direct CLI operation. Catalog scope makes elevated behavior explicit instead of encoding it as a missing Profile.

### Give Policy the complete raw argument JSON

This is flexible but expands the sensitive-data surface, couples policy rules to arbitrary provider schemas, and makes safe logging/persistence harder.

### Validate schemas only inside providers

Providers already receive arguments too late for Mender policy and approval to rely on valid input. Different providers would also enforce different validation semantics.

### Let adapters truncate or sanitize provider results before Invocation sees them

This recreates adapter-specific security semantics and prevents Audit/outcome classification from knowing whether execution produced an invalid or oversized result.

### Store progress events in Audit

This would turn the intentionally minimal terminal Audit record into a high-volume event store and create pressure to retain arguments/results in security history.

## Consequences

### Positive

- Profile becomes an actual Invocation security boundary rather than an MCP-only precondition.
- Local privileged execution remains possible through an explicit Catalog scope.
- Policy and executors only receive schema-valid arguments.
- Future argument-aware Policy can use bounded facts without copying arbitrary input.
- Result correctness and size limits become adapter-independent.
- Concurrency/resource governance has one pipeline stage.
- Runtime progress can evolve independently from durable Audit.

### Negative / trade-offs

- Invocation requests and tests become more explicit because scope/origin can no longer be omitted.
- Profile resolution adds an application dependency to Invocation for Profile-scoped calls.
- JSON Schema validation adds runtime cost and may require a direct dependency with its own compatibility/security review.
- Generic MCP Tools may initially yield unknown governance facts.
- Result-size limits can reject Tools that previously returned very large payloads.
- A lifecycle observer and execution guard add ports even when the default implementation is no-op/permissive.

## Follow-up

Phase 9 implements the governed pipeline and migrates all existing adapters to explicit execution context.

Profile ToolBinding configuration, editable Policy/Credential management, remote authenticated principals, generic DLP, and resource-specific keyed concurrency remain follow-up work rather than being folded into this decision.
