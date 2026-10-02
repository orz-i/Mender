# ADR-0032: Govern Provider activation with explicit Principals and Policy actions

- Status: Accepted
- Date: 2026-10-02

## Context

Mender's Tool invocation path is centrally governed, but Provider activation still happens outside that admission boundary.

Today a configured Provider may be opened before the Invocation Policy decision. Depending on Provider type, activation can start a native plugin process, resolve discovery Credentials, connect to upstream services, execute control-plane RPCs, discover Tools, and register an executor route.

Management and CLI Tool calls currently activate a Provider before entering InvokeTool. Starting a managed MCP server activates every Provider referenced by the Profile before the endpoint accepts Tool calls. A later Tool-level deny therefore cannot prove that no Provider-side capability was acquired first.

Invocation also carries Caller, but Caller is descriptive adapter metadata rather than a security subject. It cannot safely participate in authorization, especially now that Profiles can be exposed through managed tunnels.

Mender needs one governance vocabulary for capability acquisition and Tool execution without becoming an Agent harness.

## Decision

Mender will hard-cut governance around three explicit concepts:

~~~text
Principal
  + GovernanceContext
  + GovernanceAction
  -> Policy
  -> Approval
  -> admitted operation
~~~

The first supported actions are:

~~~text
tool.invoke
provider.activate
~~~

There is no runtime compatibility path for Caller-based contexts or invocation-only Policy inputs.

### Principal is a security subject

A Principal contains:

~~~text
Principal
  Kind
  Reference
~~~

Initial live kinds are:

- **local_user** — the local Runtime owner's OS user identity asserted by a trusted local adapter/composition root;
- **service** — a Mender-owned service identity such as one downstream Profile MCP endpoint.

References are bounded stable identifiers. They are not display names, arbitrary HTTP header values, model-provided strings, or Tool arguments. Every live GovernanceContext requires a non-zero Principal.

V1 assurance is intentionally bounded:

- CLI, Desktop, and loopback Management HTTP execute with the Runtime owner's local_user Principal. This identifies the authority under which Mender executes; it does not claim loopback HTTP authenticates an individual peer process.
- downstream MCP Tool calls execute as a service Principal for the concrete Profile endpoint. Mender does not invent a remote end-user identity that the transport did not authenticate and propagate.
- internal maintenance paths use an explicit Mender service Principal.

Remote end-user OAuth/OIDC identity is deferred.

### GovernanceContext becomes shared security vocabulary

Origin and execution scope are no longer Invocation-only concepts. They join Principal in a small governance-domain context:

~~~text
GovernanceContext
  Origin
  Principal
  Scope
    catalog
    profile:<ProfileID>
~~~

Invocation consumes this context rather than owning a Caller-bearing copy. Provider activation receives the same context supplied by the operation that caused activation.

The context contains no model prompt, raw Tool arguments, secret values, HTTP request objects, or protocol SDK types.

### Policy becomes action-aware

Policy evaluation becomes:

~~~text
PolicyEvaluation
  Action
  Origin
  PrincipalKind
  PrincipalReference
  ScopeKind
  ScopeReference
  ProviderID
  ToolID?
  FactKnowledge?
  Effects?
  Resources?
~~~

Rule Match adds:

~~~text
Actions[]
PrincipalKinds[]
PrincipalRefs[]
~~~

Actions is mandatory and non-empty for every persisted Rule. An empty action selector is not a wildcard because future governance actions must not silently inherit old rule intent.

Action invariants:

#### tool.invoke

- ProviderID is required.
- ToolID is required.
- Governance Fact knowledge is required.
- Effect/resource predicates are valid.
- Profile ToolBinding may further tighten the decision.

#### provider.activate

- ProviderID is required.
- ToolID must be empty.
- Governance Facts/effects/resources are absent.
- Profile ToolBinding policy does not participate because activation is Provider capability acquisition, not one ToolBinding execution.

Principal kind/reference and origin/scope may constrain either action. Matching remains unordered and most-restrictive.

### Activation admission precedes capability acquisition

The governed order is:

~~~text
Receive operation with GovernanceContext
        |
        v
Resolve persisted Provider metadata only
        |
        v
Evaluate Policy(action=provider.activate)
        |
        +--> deny -> audit + stop
        |
        +--> require_approval
        |       |
        |       +--> current fail-closed ApprovalGate -> audit + stop
        |
        v
Provider Manager Ensure
        |
        +--> resolve plugin/package
        +--> ValidateConfig
        +--> Plugin.Open
        +--> external process start
        +--> discovery Credential resolution
        +--> List/Discover Tools
        +--> executor route registration
        |
        v
Activation Audit
~~~

No Plugin.Open, external process launch, discovery secret resolution, Tool discovery RPC, or route registration may occur before activation admission succeeds.

providerplugin.Manager.Ensure remains a low-level mechanism. Production Runtime/adapters may not expose it as an ungated operation. Tests may exercise the mechanism directly.

### Activation call sites carry the causing Principal

All production activation paths migrate, including:

- CLI Tool calls;
- Desktop Tool calls;
- Management HTTP Tool calls;
- Provider create/sync/rebind flows that activate a Provider;
- managed MCP server startup and Profile Provider activation;
- internal recovery paths that re-open a Provider.

A managed MCP server startup uses the Principal of the control operation that starts/restarts it. Later downstream Tool invocations use the Profile MCP service Principal. Capability acquisition and later use may therefore have different Principals.

### Approval remains fail closed

provider.activate uses the same ApprovalGate abstraction as Tool invocation. Phase 22 does not add interactive approval, so require_approval prevents activation under the existing RejectApproval implementation.

### Audit records Principal, not Caller

Invocation Audit removes Caller and persists:

~~~text
principal_kind
principal_ref
~~~

No caller field remains in runtime domain objects, management contracts, OpenAPI, generated SDK, Wails bindings, Web/Desktop projections, or the new SQLite schema.

Provider activation receives a separate minimal durable audit record rather than pretending activation is a Tool invocation. It records activation identity, ProviderID, action, origin, scope, Principal, Policy decision/reason, Approval outcome, terminal status, timestamps, and bounded failure classification/summary. It stores no raw Provider config, Tool payloads, resolved Credentials, or protocol frames.

### One-way persistence migration, no runtime bridge

Policy SQLite advances one schema version. Existing Rule match JSON is rewritten once to add:

~~~json
{"actions":["tool.invoke"]}
~~~

The new runtime parser requires the new shape and does not accept missing Actions.

Audit SQLite advances one schema version. Existing caller values are converted once to audit-only legacy Principal snapshots:

~~~text
principal_kind = legacy
principal_ref  = <bounded previous caller or deterministic fallback>
~~~

New live GovernanceContexts cannot construct a legacy Principal. After migration, runtime reads/writes only principal_kind and principal_ref; there is no dual-write or caller fallback.

### Management/UI hard cut

OpenAPI, generated TypeScript SDK, Wails bindings, CLI JSON, Web, and Desktop move directly to the new contracts.

Policy management exposes action and Principal selectors. V1 has no Principal inventory service, so:

- Action and Principal Kind use structured controls.
- Principal Reference matching is an advanced bounded text input using canonical references visible in Audit/runtime projections.
- No opaque Principal Ref is required for ordinary kind-based rules.

## Security properties

Phase 22 establishes:

1. every Tool invocation has an explicit non-anonymous Principal;
2. every Provider activation has an explicit non-anonymous Principal;
3. Policy distinguishes capability acquisition from Tool execution;
4. old Tool rules do not silently govern a new action class;
5. denied activation cannot launch Provider code or resolve discovery Credentials;
6. downstream MCP does not masquerade as an authenticated human;
7. Audit attributes both activation and invocation to bounded Principal identity.

It does not claim loopback peer authentication or OS-level process sandboxing.

## Hard cut

Phase 22 removes rather than deprecates:

- InvocationContext.Caller;
- PolicyRequest.Caller;
- ApprovalRequest.Caller;
- Audit caller contract fields;
- invocation-only Policy evaluation shape;
- ungated public Runtime Provider activation entry points.

Generated clients are regenerated against the new contract. Old clients are not supported with compatibility fields.

## Alternatives considered

### Add a Policy check inside CallConfiguredTool only

Rejected because managed MCP startup, Provider sync/rebind, and other activation paths could still acquire capabilities outside that adapter.

### Treat Provider activation as a synthetic Tool invocation

Rejected because activation has no ToolID or Tool arguments and can happen before Tool discovery.

### Keep Caller beside Principal

Rejected because dual attribution fields prolong ambiguity about which value is security-relevant.

### Trust tunnel/provider headers as Principal identity

Rejected. Unverified transport metadata is not promoted to identity.

### Make Actions optional for existing rules

Rejected. Empty action wildcard would let old Tool rules silently match future governance actions.

## Consequences

Positive:

- Provider capability acquisition becomes Policy-governed before side effects.
- Policy can express different rules for activation and invocation.
- Principal becomes an authorization dimension instead of an audit label.
- downstream MCP identity semantics are explicit.
- Audit can answer what authority caused activation and execution.
- future authenticated remote Principals can use the same boundary.

Costs:

- this is a breaking internal/API/persistence change;
- every Invocation and activation call site must construct GovernanceContext;
- Policy storage and all management adapters require coordinated migration;
- Principal Ref UX is basic until an inventory/directory exists;
- require_approval still fails closed rather than pausing.

## Deferred

- interactive approval and MCP multi-round-trip continuation;
- OAuth/OIDC end-user identity for remote MCP;
- OS/container/VM sandbox backends;
- runtime quotas/rate limits/resource budgets;
- Principal directory/group/role management;
- per-peer authentication for loopback Management HTTP.

## Follow-up

Phase 22 implements this hard cut and removes all Caller-era runtime contracts.
