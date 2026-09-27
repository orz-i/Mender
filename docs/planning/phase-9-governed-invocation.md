# Phase 9 Plan — Governed Invocation Execution

Status: Planned

## Objective

Upgrade Mender's shared Invocation pipeline from a common executor entry point into a governed execution runtime.

Phase 9 makes execution context, Profile scope, input validation, bounded policy facts, canonical failure/rejection outcomes, execution/result guards, runtime lifecycle events, and Audit metadata part of one protocol-independent contract.

The phase preserves the existing local-first product model:

- trusted local/control-plane workflows can invoke through explicit Catalog scope;
- Profile-bound agent/protocol surfaces execute through Profile scope;
- every adapter converges on the same Invocation semantics.

Phase 9 does not turn Mender into an agent framework and does not make MCP, HTTP, CLI, React, Wails, or SQLite types part of the Invocation Domain.

## Research baseline

Research date: 2026-09-26.

The design is informed by the Pi tool-system execution model reviewed at:

- https://dg-ai-notes.pages.dev/modules/ch05-tools/

The useful concepts for Mender are:

- validate arguments before execution;
- separate execution hooks/guards from Tool implementation;
- preserve Tool-level failures as model-consumable results instead of collapsing them into transport failure;
- keep execution capabilities behind narrow interfaces;
- support progress without confusing progress events with final results;
- make concurrency semantics explicit.

Mender intentionally does not copy Pi's Agent Loop, conversation model, prompt/rendering layers, or model-facing ToolDefinition structure. Mender remains a Tool control plane and governed runtime.

## Problem statement

The current runtime already centralizes Tool resolution, Policy, Approval, timeout/cancellation, execution routing, CredentialResolver access, and Audit.

It still has important governance gaps.

### Profile is not yet an Invocation boundary

Profile membership is enforced primarily by the downstream MCP adapter. Invocation itself accepts ToolID, arguments, caller, and timeout without an execution scope.

This means an adapter can call the same Tool without carrying the Profile boundary that was supposed to authorize it.

### Arguments are JSON-valid but not schema-valid

Invocation currently validates that arguments contain one JSON value.

It does not validate the instance against Tool.InputSchema before Policy and execution.

### Expected failures use the Go error channel

Upstream MCP "isError: true", policy denial, approval denial, timeout, and other operational conditions are largely surfaced through Go errors.

This makes adapters reconstruct execution semantics independently and can turn recoverable Tool failures into downstream protocol/server failures.

### Results have no canonical postflight governance

A completed Tool result currently leaves the executor without a shared output-schema check or core result-size limit.

### Argument-aware policy needs a safer contract

Future Policy cannot safely consume arbitrary Tool input JSON by default. It needs bounded governance facts derived after schema validation.

### Runtime progress and concurrency are implicit

Audit is intentionally terminal and data-minimized. It should not become a progress event store.

Invocation also lacks a first-class stage for provider/Tool execution serialization when a runtime needs it.

## Scope

Phase 9 includes:

1. explicit InvocationContext with origin, caller, and execution scope;
2. Catalog and Profile execution scopes;
3. Profile membership enforcement inside Invocation;
4. JSON Schema input validation before Policy/execution;
5. bounded typed GovernanceFacts passed to Policy rather than raw arguments;
6. canonical InvocationOutcome kinds for completion, continuation, rejection, and failure;
7. Tool/provider/runtime/result-guard failure classification;
8. output-schema validation and bounded result-size enforcement;
9. an ExecutionGuard application stage with permissive defaults and serial guard implementations;
10. data-minimized lifecycle/progress event contracts separated from Audit;
11. Audit metadata for scope/origin/outcome classification;
12. consistent CLI, MCP, management HTTP, and desktop outcome mapping;
13. migration and integration coverage for the changed contracts.

## Non-goals

Phase 9 does not include:

- an agent loop, conversation history, planner, or automatic Tool retry;
- Profile ToolBinding metadata such as aliases, description overrides, per-Tool credential scope, or persisted execution constraints;
- editable Policy or Credential management UI;
- a general credential store redesign;
- remote management authentication/authorization or an authenticated Principal model;
- generic secret/PII/DLP scanning of arbitrary Tool results;
- silent truncation of canonical structured JSON;
- resource-specific keyed locking such as per-repository/per-browser-session locks;
- public SSE/WebSocket progress APIs;
- durable storage of progress events;
- MCP Tasks continuation support beyond the protocol support already available;
- semantic/vector Tool discovery.

These items may build on Phase 9 but must not expand this phase until its core execution contract is complete.

## Ubiquitous language

### Invocation Context

The non-secret context under which one Tool invocation is evaluated.

Initial fields:

~~~text
InvocationContext
  Origin
  Caller
  Scope
~~~

"Caller" remains an attribution label. It is not an authenticated user identity.

### Execution Origin

A stable classification of the adapter/runtime entry point.

Initial values:

~~~text
cli
mcp
management_http
desktop
internal
~~~

Agent Skills currently invoke Mender through the CLI and therefore use CLI origin unless a future Skill runtime adapter provides its own execution entry point.

Adapters may share application code, but Audit and Policy must not infer origin by parsing the free-form caller label.

### Execution Scope

The authorization boundary in which ToolID is interpreted for execution.

Initial variants:

~~~text
CatalogScope

ProfileScope
  ProfileID
~~~

There is no zero-value/implicit scope at the application boundary.

### Catalog Scope

An explicit privileged scope over the canonical Catalog.

It does not mean "bypass Policy". It means Profile membership is not the visibility boundary for this invocation.

Only trusted local/control-plane composition paths may create Catalog scope in Phase 9.

### Profile Scope

An execution scope bound to one Profile.

Invocation must verify membership before resolving Tool metadata for execution so a Profile-scoped caller cannot use invocation errors as a Tool-existence oracle outside the Profile.

### Governance Facts

A bounded typed projection derived from validated arguments for Policy.

Facts are not arbitrary JSON and are not automatically persisted.

The initial model should be intentionally small:

~~~text
GovernanceFacts
  Knowledge: known | unknown
  Effects[]?
  Resources[]?

ResourceFact
  Kind
  Value
~~~

Exact effect/resource enums are implementation-reviewed domain values. Values are bounded before entering Policy.

Generic MCP Tools may legitimately produce "unknown" facts.

### Execution Guard

An application port that controls whether/when provider execution may enter its critical section.

Initial infrastructure modes:

~~~text
unlimited
provider_serial
tool_serial
~~~

Phase 9 does not persist user-selectable guard configuration. The default composition remains unlimited unless a concrete composition/test selects a serial guard.

### Result Guard

The post-execution stage that validates and bounds normalized outcomes before adapters receive them.

Core Phase 9 checks:

- declared output-schema validation;
- canonical result-size limit.

### Invocation Outcome

The protocol-independent result of a governed invocation.

Initial kinds:

~~~text
complete
input_required
task
rejected
failed
~~~

### Rejected Outcome

An invocation that Mender intentionally did not execute or could not accept before provider execution.

Initial stable codes:

~~~text
tool_not_found
tool_outside_scope
invalid_arguments
policy_denied
approval_denied
~~~

Messages are safe, bounded presentation hints. Raw arguments are not attached.

### Failed Outcome

An invocation that attempted or entered execution but did not produce an acceptable completion/continuation outcome.

Initial sources and codes:

~~~text
tool
  tool_error

provider
  provider_error

runtime
  executor_unavailable
  timed_out
  cancelled

result_guard
  output_schema_mismatch
  result_too_large
~~~

Implementations may add stable codes when a distinct recovery behavior exists. Provider-specific arbitrary strings do not become Mender stable codes automatically.

### Invocation Event

A bounded, ephemeral lifecycle/progress signal.

It is not an Audit Record and is not a Tool result.

## Architectural ownership

Phase 9 preserves the existing bounded contexts.

~~~text
Catalog
  owns canonical Tool schemas/metadata
       |
       v
Invocation
  owns execution context, governed pipeline, outcomes
   |       |          |          |
   v       v          v          v
Profile  Policy    Credential   Audit
 scope   decision   resolver    terminal record
   |
   v
ToolExecutor / Provider infrastructure
~~~

Invocation consumes Profile through an Invocation-owned application port. It does not read Profile persistence directly.

JSON Schema validation is an Invocation application/infrastructure concern operating on canonical Catalog schemas. It does not make a validation-library type part of Catalog Domain.

Runtime observers and guards are ports owned by the Invocation use case.

## Canonical pipeline

The target Phase 9 pipeline is:

~~~text
Construct InvocationRequest
        |
        v
Validate request context
        |
        v
Allocate InvocationID
        |
        v
Resolve execution scope
        |
        +--> ProfileScope
        |      |
        |      v
        |   load Profile
        |      |
        |      v
        |   membership check
        |      |
        |      +--> rejected: tool_outside_scope
        |
        v
Resolve canonical Tool
        |
        +--> rejected: tool_not_found
        |
        v
Parse arguments as JSON value
        |
        v
Validate against Tool.InputSchema
        |
        +--> rejected: invalid_arguments
        |
        v
Derive bounded GovernanceFacts
        |
        v
Evaluate Policy
        |
        +--> rejected: policy_denied
        |
        +--> RequireApproval
        |      |
        |      +--> rejected: approval_denied
        |
        v
Create execution budget
        |
        v
Acquire ExecutionGuard
        |
        +--> failed: timed_out / cancelled where applicable
        |
        v
Resolve ToolExecutor
        |
        +--> failed: executor_unavailable
        |
        v
Execute
        |
        +--> Tool-reported failure -> failed/tool
        +--> provider transport failure -> failed/provider
        +--> timeout/cancel -> failed/runtime
        |
        v
Normalize outcome
        |
        v
Apply ResultGuard
        |
        +--> failed/result_guard
        |
        v
Record terminal Audit
        |
        v
Emit terminal lifecycle event
        |
        v
Return InvocationOutcome
~~~

An internal failure that prevents safe outcome construction or required Audit persistence still returns a Go error.

## Execution context and adapter rules

### CLI

"mender tool call" keeps backward-compatible local administrative behavior by constructing Catalog scope explicitly.

Phase 9 adds optional:

~~~text
--profile <slug-or-id>
~~~

When supplied, the CLI resolves the Profile and invokes through Profile scope.

The Agent Skill may continue to use CLI Catalog scope unless a Profile is explicitly selected. Phase 9 does not silently change the Skill's Tool visibility.

### Downstream MCP

Every direct Tool and "mender_call_tool" invocation uses Profile scope from the MCP server instance.

Profile membership inside Invocation is authoritative for execution.

Search/describe remain Profile-scoped discovery operations outside Invocation and continue to enforce their own visibility constraints.

Adapter-side membership checks may remain only when required for discovery/projection behavior; they are not a substitute for Invocation enforcement.

### Management HTTP

The loopback management Tool-call endpoint constructs explicit Catalog scope in Phase 9.

A syntactically malformed HTTP envelope can still produce HTTP 400 before Invocation.

Once a valid Invocation request is constructed, canonical "rejected" or "failed" outcomes are returned as the Invocation result body instead of being reclassified as arbitrary HTTP transport failures.

Internal server errors remain 5xx.

### Desktop

Desktop bindings construct the same application request/context as management Web workflows and expose the same InvocationOutcome projection.

Wails-specific types do not enter Invocation.

## Input Schema validation

### Validation position

Validation happens after Tool resolution and before GovernanceFacts, Policy, Approval, or executor routing.

The executor must never see arguments that failed canonical input validation.

### Schema semantics

Canonical Tool schemas continue to be stored as raw protocol-independent JSON Schema.

Phase 9 validates using JSON Schema 2020-12 semantics.

Boolean schemas are valid:

- "true" accepts any JSON instance;
- "false" rejects every instance.

Adapters may impose stricter protocol envelope constraints. For example, MCP Tool calls use object arguments. Such protocol constraints remain at the MCP boundary and do not redefine the canonical schema type.

### No implicit mutation

Mender does not apply schema defaults, coercion, string-to-number conversion, property insertion, or other hidden argument rewriting.

The exact validated JSON bytes/value remain the execution input.

### Validator dependency

Implementation must review and add a direct dependency only if needed for complete JSON Schema evaluation.

A transitive dependency already present in the module is not considered an approved direct runtime dependency merely because it exists in "go.sum".

The selected validator must:

- support the required 2020-12 features used by Mender Tools;
- support boolean schemas;
- operate without leaking its library types into Domain;
- have tests for invalid schemas and invalid instances;
- have acceptable license/security/update characteristics.

Compiled-schema caching may be added only after correctness is established and measurement shows value.

## Governance Facts and Policy

### Fact safety

GovernanceFacts exist to reduce the need for raw-argument Policy.

Fact extraction must:

- run only after schema validation;
- return bounded typed values;
- avoid copying arbitrary nested JSON;
- avoid resolved credentials;
- avoid implicit persistence/logging;
- return an explicit unknown state when facts cannot be established safely.

### Initial behavior

Phase 9 establishes the contract and passes GovernanceFacts into PolicyRequest.

The default generic extractor may return unknown.

Tests use concrete extractors to prove Policy can make a decision from facts without receiving raw arguments.

MCP annotations remain hints only and are not automatically converted into trusted effects.

### Policy compatibility

The local default Policy may continue to allow execution.

Changing from "AllowAll" to configurable production Policy is not a Phase 9 requirement.

Phase 9's requirement is that when Policy does evaluate execution, it receives the same scope/origin/fact contract regardless of adapter.

## Canonical outcome model

### Complete

Contains one canonical JSON value.

Before return, ResultGuard validates a declared output schema and size constraints.

### InputRequired

Retains the existing continuation request ID, message, and optional schema semantics.

Phase 9 does not implement a new continuation API.

### Task

Retains the existing task ID/status/message semantics.

Phase 9 does not implement Task polling/cancellation APIs unless required by an existing concrete adapter.

### Rejected

Represents expected pre-execution governance outcomes.

Rejected outcomes are audited and returned without calling ToolExecutor.

A safe message can guide a human/agent, but must not expose Tool metadata outside Profile scope.

For Profile scope, an invalid/outside ToolID should prefer a non-enumerating message such as "tool is not available in this execution scope".

### Failed

Represents expected operational failure after execution was attempted/entered or while preparing the execution boundary.

Tool-level failure content may be normalized into protocol-independent JSON for the caller, subject to ResultGuard limits.

Provider/runtime failure messages are bounded and sanitized before exposure.

No failure content is automatically copied into Audit.

## MCP Tool failure mapping

Upstream MCP:

~~~text
CallToolResult
  IsError = true
~~~

maps to:

~~~text
InvocationOutcome
  kind   = failed
  source = tool
  code   = tool_error
~~~

rather than to a Go error solely because the Tool reported failure.

Downstream MCP maps a safe "rejected" or "failed" outcome to:

~~~text
CallToolResult
  IsError = true
  Content = safe canonical failure/rejection projection
~~~

A genuine Mender internal error remains a server/protocol error.

This rule allows an agent to inspect a Tool failure and decide whether to change arguments or choose another Tool without confusing that failure with a broken MCP transport.

## Result Guard

### Output schema

If Tool.OutputSchema is declared, a Complete outcome must validate against it.

Mismatch produces:

~~~text
kind   = failed
source = result_guard
code   = output_schema_mismatch
~~~

The invalid payload is not persisted to Audit.

### Result size

Canonical content-bearing outcomes have an application-owned byte limit.

The limit is configurable at composition for tests/deployments and has a documented repository default.

If a result exceeds the limit, Phase 9 does not truncate the canonical JSON. It returns:

~~~text
kind   = failed
source = result_guard
code   = result_too_large
~~~

Human UI rendering may still abbreviate already-accepted results independently.

### Redaction

Phase 9 does not claim generic output secret detection.

Audit failure-summary redaction remains in force.

A future result-redaction processor can be introduced behind ResultGuard when it can be tested against an explicit threat model.

## Execution Guard

ExecutionGuard is called after Policy/Approval and inside the Invocation execution budget.

Initial guard infrastructure should support:

- unlimited/no-op;
- serial by ProviderID;
- serial by ToolID.

The default production composition remains unlimited unless explicitly selected.

The goal in Phase 9 is to make concurrency authority explicit and testable, not to invent persistent per-Tool settings before Profile ToolBinding exists.

Guard acquisition must:

- honor cancellation;
- honor the Invocation timeout/budget;
- always release on executor return/panic-safe application flow;
- avoid global serialization when only one Provider/Tool is constrained.

Resource-key-specific serialization is deferred.

## Lifecycle and progress events

### Separation from Audit

Audit remains one durable terminal record per Invocation outcome.

Invocation events are ephemeral observer signals.

Initial lifecycle event kinds may include:

~~~text
accepted
scope_resolved
arguments_validated
policy_decided
approval_resolved
execution_waiting
execution_started
progress
terminal
~~~

The exact event enum is an implementation contract and must remain data-minimized.

### Progress reporter

ToolExecutor may receive a restricted reporter/callback capability.

Progress payloads are bounded and must not contain:

- raw argument JSON by default;
- resolved credentials;
- full Tool result payloads;
- arbitrary protocol frames.

Late progress reports after a terminal outcome are ignored.

Observer callbacks are best-effort and do not return an application error that can replace a governed outcome. Audit remains the required durable record.

Phase 9 does not add public Web streaming. Tests establish lifecycle ordering and late-report behavior.

## Credential handling

ADR-0007 remains authoritative.

Phase 9 does not move secret resolution earlier in the pipeline.

GovernanceFacts and Policy receive no resolved secret bytes.

ToolExecutor continues to be the boundary allowed to consume CredentialResolver.

ResultGuard must not obtain CredentialResolver merely to inspect output.

## Audit evolution

### Required metadata

Phase 9 extends Audit so a terminal record can identify:

- InvocationID;
- ToolID and ProviderID when resolved;
- execution origin;
- scope kind;
- ProfileID when Profile-scoped;
- caller label;
- policy kind/reason;
- approval outcome;
- terminal status;
- canonical outcome kind;
- stable outcome/rejection/failure code;
- failure source where applicable;
- bounded/redacted error summary;
- start/finish timestamps.

Audit still does not persist:

- raw arguments;
- GovernanceFacts by default;
- full Tool result/failure content;
- Credential references;
- secret values;
- progress payloads;
- raw protocol frames.

### Migration

Audit SQLite receives a forward-only migration owned by the Audit context.

The migration must preserve existing rows.

Existing "result_kind" projection compatibility should be retained during Phase 9 where practical while new outcome metadata becomes authoritative for new records.

An older binary must continue to fail closed when it sees the newer Audit schema version under ADR-0014.

## External contracts

### CLI JSON

The stable JSON result gains new "kind" values:

~~~text
rejected
failed
~~~

Rejected projection includes at least:

~~~text
invocationId
kind
code
message
~~~

Failed projection includes at least:

~~~text
invocationId
kind
source
code
message
~~~

Optional structured failure content is included only when the executor produced safe canonical content.

In "--output json" mode, stdout still contains exactly one JSON document when Invocation produced an outcome.

Rejected/failed outcomes use a documented non-zero exit status without requiring callers to parse human stderr.

### OpenAPI

"InvocationResult" is extended for the new outcome kinds and their fields.

The management call endpoint distinguishes:

- transport/envelope errors before Invocation;
- canonical Invocation outcomes;
- internal server failure.

Generated TypeScript contracts are regenerated, not hand-edited.

### Wails

Desktop bindings expose the same Invocation outcome structure through generated/typed bindings.

No Wails type becomes canonical runtime state.

### MCP

Both direct and dynamic calls pass Profile scope and use the shared outcome mapper.

Expected Tool/rejection failures map to MCP Tool error results.

## Security review

Phase 9 is security-sensitive.

Required invariants:

1. Profile membership is enforced inside Invocation for Profile scope.
2. Membership is checked before Tool metadata resolution where necessary to avoid cross-Profile enumeration.
3. No adapter can create an implicit/no-scope invocation.
4. Invalid arguments never reach Policy facts extraction that assumes schema validity or ToolExecutor execution.
5. Policy does not receive unrestricted raw arguments.
6. GovernanceFacts are bounded and non-secret.
7. CredentialResolver remains execution-boundary-only.
8. ResultGuard runs before adapters receive completed/failure content.
9. Oversized/invalid output is rejected without being persisted to Audit.
10. Runtime progress cannot append to a terminal invocation.
11. ExecutionGuard wait obeys timeout/cancellation.
12. Audit remains free of arguments, results, facts, credentials, and progress payloads.
13. Upstream Tool failure is not mistaken for Mender internal failure.
14. Internal failure is not disguised as a normal Tool error result.

## Test strategy

### Invocation Domain/Application

Table-driven tests cover:

- zero/invalid execution context rejection;
- Catalog scope;
- Profile scope allowed membership;
- Profile scope denied membership;
- denied membership does not resolve the hidden Tool;
- Tool not found in Catalog scope;
- valid object schema arguments;
- invalid required/property/type arguments;
- boolean true/false schemas;
- schema-evaluation internal failure;
- GovernanceFacts passed to Policy;
- raw arguments absent from PolicyRequest;
- Policy deny;
- approval deny;
- execution timeout/cancellation;
- execution guard wait cancellation;
- tool failure outcome;
- provider failure outcome;
- result output-schema mismatch;
- result size rejection;
- internal Audit failure remains Go error.

### Execution Guard

Concurrency tests cover:

- unlimited allows overlap;
- Provider serial blocks same Provider but not unrelated Providers;
- Tool serial blocks same Tool;
- cancellation while waiting;
- release after execution failure;
- race-enabled test suite remains clean.

### Runtime Events

Tests cover:

- deterministic stage ordering for a successful invocation;
- rejection emits terminal event without execution-started;
- progress is bounded;
- progress after terminal state is ignored;
- observer delivery failure/absence cannot replace the governed outcome or required Audit record.

### MCP

Integration tests cover:

- direct Profile-scoped call;
- dynamic "mender_call_tool" Profile-scope denial;
- upstream "isError: true" becomes canonical failed/tool;
- downstream failed/tool becomes "CallToolResult.IsError = true";
- internal Mender error remains MCP/server error.

### CLI

Integration tests cover:

- Catalog-scope default call;
- "--profile" allowed/denied calls;
- JSON complete outcome compatibility;
- JSON rejected outcome;
- JSON failed outcome;
- non-zero exit semantics without corrupting JSON stdout.

### Management/Web/Desktop

Tests cover:

- management Tool call constructs Catalog scope;
- OpenAPI generated client handles new outcomes;
- product UI can render rejected/failed detail without treating it as network failure;
- Wails binding parity;
- no host-specific runtime semantics leak into "packages/app".

### Audit/SQLite

Tests cover:

- forward migration from current Audit schema;
- future-version fail-closed behavior;
- new scope/origin/outcome metadata round-trip;
- old rows remain readable;
- forbidden sensitive columns remain absent.

### Repository validation

Before phase closeout:

~~~text
pnpm check
pnpm security:check
~~~

Network-requiring security checks follow the existing Phase 8 policy.

## Work packages and commit segmentation

Phase 9 is intentionally split so each commit leaves one understandable contract in place.

### 1. Architecture decisions

Commit:

~~~text
docs: define governed invocation architecture
~~~

Contents:

- ADR-0017 explicit scope + preflight/postflight pipeline;
- ADR-0018 canonical outcome model;
- ADR index.

Status: complete at Phase 9 planning start.

### 2. Phase plan and repository architecture docs

Commit:

~~~text
docs: plan governed invocation execution phase
~~~

Contents:

- this Phase 9 plan;
- roadmap/status update;
- architecture principle pipeline update;
- system/product summary updates where needed.

No implementation code.

### 3. Execution context and Profile scope

Commit:

~~~text
refactor(invocation): add execution context and profile scope
~~~

Contents:

- InvocationContext / Origin / Scope domain values;
- Invocation-owned Profile lookup port;
- authoritative Profile membership enforcement;
- Catalog-scope explicit requests;
- tests proving no adapter-only security dependency.

Adapters may receive temporary compile-fix wiring in this commit only when required to keep the repository green; behavior mapping remains for the dedicated adapter commit.

### 4. Canonical Invocation outcomes

Commit:

~~~text
refactor(invocation): introduce canonical invocation outcomes
~~~

Contents:

- replace/extend Result with InvocationOutcome;
- Rejected and Failed variants;
- stable code/source model;
- classification of timeout/cancel/executor/provider/Tool conditions;
- Go error boundary narrowed to internal failures.

No adapter-specific formatting beyond minimal compile wiring.

### 5. Input validation and Governance Facts

Commit:

~~~text
feat(invocation): validate arguments and derive governance facts
~~~

Contents:

- JSON Schema validator boundary;
- reviewed direct validator dependency if required;
- invalid-argument Rejected outcome;
- GovernanceFacts contract;
- PolicyRequest scope/origin/facts;
- no raw argument Policy path;
- validation/fact tests.

### 6. Execution and Result guards

Commit:

~~~text
feat(invocation): add execution and result guards
~~~

Contents:

- ExecutionGuard port and in-memory unlimited/provider/tool implementations;
- timeout-aware acquisition;
- ResultGuard;
- output-schema validation;
- canonical result-size limit;
- guard concurrency and result tests.

### 7. Lifecycle progress events

Commit:

~~~text
feat(invocation): add lifecycle progress events
~~~

Contents:

- Invocation observer/event contract;
- restricted executor progress reporter;
- no-op/default sink;
- terminal/late-progress semantics;
- data-minimization tests.

No public streaming transport.

### 8. Audit migration

Commit:

~~~text
feat(audit): persist governed invocation metadata
~~~

Contents:

- new Audit metadata/domain invariants;
- SQLite forward migration;
- old-row compatibility;
- future-version compatibility tests;
- sensitive-data absence tests.

### 9. Adapter contract migration

Commit:

~~~text
feat(adapters): map governed invocation outcomes
~~~

Contents:

- CLI Catalog/Profile scopes and JSON/exit behavior;
- MCP Profile scope and "isError" mapping;
- management HTTP canonical outcome contract;
- OpenAPI regeneration;
- Wails binding regeneration;
- shared Web/Desktop outcome rendering.

This commit must not reimplement Policy/Profile rules in adapters.

### 10. End-to-end governance matrix

Commit:

~~~text
test(runtime): cover governed execution end to end
~~~

Contents:

- cross-adapter integration matrix;
- Profile denial cannot execute;
- invalid schema input cannot execute;
- upstream Tool error round-trip;
- output guard failure;
- concurrency/race tests;
- regression tests for existing Complete/InputRequired/Task paths.

### 11. Closeout

Commit:

~~~text
docs: complete governed invocation execution phase
~~~

Contents:

- Phase 9 Status -> Complete;
- evidence from checks/tests;
- documented external contract changes;
- development/operator guidance;
- explicit deferred follow-up items.

Small corrective commits remain preferable to combining unrelated fixes if implementation uncovers defects.

## Exit criteria

Phase 9 is complete only when all of the following are true:

- [ ] Every Invocation request has explicit origin/caller/scope.
- [ ] Profile-scoped invocation enforces membership inside Invocation.
- [ ] Downstream MCP calls cannot bypass Profile scope.
- [ ] Local CLI/control-plane Catalog scope is explicit rather than represented by an absent Profile.
- [ ] Tool arguments are validated against canonical input schema before Policy/execution.
- [ ] Invalid arguments never reach ToolExecutor.
- [ ] Policy receives bounded GovernanceFacts instead of unrestricted raw arguments.
- [ ] Complete/InputRequired/Task behavior remains supported.
- [ ] Rejected and Failed are first-class canonical outcomes.
- [ ] Upstream MCP "isError: true" is a Tool failure outcome, not automatically a Go/server error.
- [ ] Internal Mender failures remain distinguishable from Tool failures.
- [ ] Declared output schemas are validated before Complete leaves Invocation.
- [ ] Canonical result-size limits are enforced without silent JSON truncation.
- [ ] ExecutionGuard supports tested unlimited/provider/tool semantics.
- [ ] Guard waiting honors cancellation/timeout.
- [ ] Runtime lifecycle/progress events are separated from durable Audit.
- [ ] Audit records scope/origin/outcome classification without arguments/results/facts/secrets.
- [ ] Audit migration preserves existing rows and future-version fail-closed behavior.
- [ ] CLI machine JSON exposes governed outcomes deterministically.
- [ ] CLI supports optional Profile-scoped Tool invocation.
- [ ] MCP maps safe rejected/failed outcomes to Tool-call error results.
- [ ] Management HTTP and Desktop expose the same canonical outcome semantics.
- [ ] OpenAPI and Wails generated contracts are synchronized.
- [ ] Go race tests pass.
- [ ] "pnpm check" passes.
- [ ] Required security checks pass before closeout.

## Deferred follow-up

After Phase 9, the next governance work can be planned from concrete runtime contracts instead of adapter-specific behavior.

Likely follow-ups include:

- Profile ToolBinding with alias, model-facing metadata override, Policy binding, Credential scope, and persisted execution constraints;
- editable Policy rules consuming GovernanceFacts;
- credential-store and credential-scope management;
- authenticated remote principals and remote control-plane authorization;
- resource-key-specific execution guards;
- public progress streaming in management/remote adapters;
- result redaction/DLP with an explicit threat model;
- retry policy based on stable failure codes;
- richer native/local Tool capability interfaces.

None of these are prerequisites for completing the governed execution core.
