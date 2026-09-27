# ADR-0021: Bind Provider authentication to Credential References and enforce Profile credential scope

- Status: Accepted
- Date: 2026-09-27

## Context

MCP Provider authentication is connection-level.

Runtime activates one upstream MCP connection per ProviderID and registers that connection as the Provider executor.

Sensitive HTTP headers are currently persisted only as environment templates. This avoids plaintext storage but remains outside a Credential inventory.

Phase 10 made ToolBinding the Profile × Tool governance unit. Credential scope belongs on this relationship, but merely storing references would not protect a shared authenticated MCP connection.

A Profile that lacks permission for a connection credential must not be able to call a Tool merely because another Profile caused the Provider to be activated.

## Decision

Provider authentication declares Credential References, and Profile ToolBinding declares an allowlist of Credential References that execution may resolve.

### ToolBinding credential allowlist

ToolBinding gains:

~~~text
CredentialRefs[]
~~~

The list contains canonical credential:// References.

Default is empty.

Empty means Profile-scoped execution receives no inventory-backed Credential access.

Catalog scope remains privileged and receives the full Runtime CredentialResolver.

Normal UI edits CredentialRefs through inventory selection. It does not ask for raw References.

### Profile validation

Profile Application validates every selected Credential Reference against Credential inventory before saving a ToolBinding set.

Unknown references fail the atomic replacement.

Credential References are metadata only and are safe to persist in Profile SQLite.

### Invocation scoped resolver

Invocation never resolves secret bytes itself.

Before ToolExecutor execution, Invocation derives the executor CredentialResolver:

~~~text
Catalog scope
  -> full Runtime resolver

Profile scope
  -> allowlist wrapper over Runtime resolver
~~~

The wrapper rejects any Reference not present in the ToolBinding credential allowlist.

The wrapped resolver remains the only secret capability passed to ToolExecutor.

Audit and Policy do not receive CredentialRefs or resolved bytes by default.

### Provider credential bindings

Provider adapter configuration may declare fixed authentication requirements.

Initial structures:

~~~text
HTTP Credential Header
  HeaderName
  Scheme?
  CredentialRef

stdio Credential Environment
  EnvironmentName
  CredentialRef
~~~

Provider configuration persists only these references and non-secret formatting metadata.

### Connection activation

Runtime uses the full CredentialResolver when activating a configured Provider.

For HTTP:

~~~text
CredentialRef
  -> resolve secret
  -> Scheme + secret
  -> HTTP header
  -> connect
~~~

For stdio:

~~~text
CredentialRef
  -> resolve secret
  -> explicit child environment entry
  -> connect
~~~

Secret values exist only in connection/process infrastructure memory.

They are not written back into Provider config.

### Shared connection authorization

An activated MCP connection records only the required Credential References.

Before every Tool call, the MCP executor resolves each required Reference through the resolver capability supplied by Invocation.

This acts as an execution-boundary authorization proof.

If a Profile ToolBinding did not allow a required Reference, the call fails before upstream CallTool.

Catalog scope succeeds because it receives the full resolver.

### Provider identity and account switching

A Provider configuration has one fixed authentication binding.

ToolBinding cannot choose a different account for the same Provider connection.

If two accounts must be used, register two Providers with distinct Provider identities/configurations.

This keeps Catalog provenance and connection identity stable.

### Legacy environment header templates

Existing HTTP header environment templates remain readable and executable for backward compatibility.

They are explicitly legacy and are not represented by Credential inventory References.

New normal management UX prefers Credential-backed sensitive headers.

Legacy template values remain subject to the existing no-plaintext-secret validation.

Because legacy templates do not have a Credential Reference, ToolBinding CredentialRefs cannot express their authorization. They are a compatibility exception, not proof of scoped Credential governance.

A later migration phase may remove or automatically convert this legacy path.

### Failure semantics

Credential access failures are expected operational failures.

Stable classifications distinguish:

- credential outside Profile scope;
- Credential metadata not found;
- secret source unavailable;
- environment variable missing;
- native keyring operation failure.

Messages are bounded and never include secret values.

### UI separation

Provider connection metadata and Provider authentication are separate categories.

The Provider editor does not mix secret binding controls into a generic headers textarea.

Normal Provider authentication chooses Credentials from inventory.

Profile ToolBinding adds a separate Credentials action/dialog next to Presentation and Governance.

The Tool picker remains membership-only.

## Alternatives considered

### Resolve Provider credentials once and trust the shared connection for every Profile

Rejected because any Profile using that Provider could inherit another Profile's authenticated session.

### Let ToolBinding choose a different Provider credential at call time

This conflicts with a shared connection whose authentication was established earlier. Supporting account switching requires session pooling keyed by credential identity and is not Phase 11.

### Persist resolved Provider secrets in config

Rejected by ADR-0020 and ADR-0007.

### Give Invocation secret bytes

Rejected because Invocation, Policy, Audit, and lifecycle code would become secret-bearing.

## Consequences

### Positive

- Profile credential scope becomes enforceable;
- shared authenticated MCP sessions cannot be used by Profiles lacking required refs;
- Provider config stores only references;
- HTTP and stdio authentication gain one inventory model;
- Catalog scope remains an explicit privileged local path.

### Negative / trade-offs

- each Provider configuration is tied to one auth identity;
- connection activation can fail when a Credential source is unavailable;
- existing environment-template authentication remains a documented legacy exception;
- Provider/UI contracts grow new structured auth fields.

## Follow-up

Future work may add session pools keyed by Provider + credential identity, authenticated remote principals, automatic migration of legacy environment templates, and policy rules that reason over Credential metadata without seeing secret values.
