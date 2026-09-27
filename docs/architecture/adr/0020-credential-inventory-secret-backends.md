# ADR-0020: Separate Credential inventory metadata from secret material

- Status: Accepted
- Date: 2026-09-27

## Context

Mender already enforces that secret values do not enter Invocation, Policy, Profile, Audit, or normal Provider persistence.

The Credential bounded context currently contains only an opaque Reference of the form credential://namespace/name and a resolver that always fails closed.

This is not yet usable:

- users cannot create or discover Credentials;
- Profile management cannot select Credentials from an inventory;
- there is no persistent metadata model;
- there is no secure secret backend;
- the Credentials management surface is intentionally read-only;
- MCP Provider authentication still relies on environment templates for sensitive HTTP headers.

Persisting secret values in SQLite would violate ADR-0007 and make database access, backups, and migrations secret-bearing.

At the same time, requiring only an OS keyring would make headless and CI deployments difficult on Linux systems without a usable Secret Service.

## Decision

Credential inventory metadata and secret material are stored through separate ports and backends.

### Credential identity

Credential Reference remains the canonical human-readable identity:

~~~text
credential://namespace/name
~~~

Users do not type this complete Reference in normal management flows.

Create forms collect namespace and name separately and derive the Reference.

### Credential metadata

The Credential Domain introduces immutable metadata:

~~~text
Credential
  Reference
  DisplayName
  Source
  EnvironmentVariable?
  CreatedAt
  UpdatedAt
~~~

Initial Source values:

~~~text
native
environment
~~~

Metadata contains no secret bytes.

### SQLite inventory

Credential metadata is persisted in a Credential-owned SQLite schema.

The table stores:

- canonical Reference;
- display name;
- source kind;
- environment variable name for environment-backed Credentials;
- timestamps.

It never stores secret values, tokens, passwords, API keys, resolved environment values, or keyring payloads.

Credential SQLite follows the same forward-only migration and future-version fail-closed policy as other Mender stores.

### Native secret backend

Source native stores secret material in the operating-system credential store through a narrow SecretStore port.

The first implementation uses github.com/zalando/go-keyring and the fixed service name Mender.

The account/key is the canonical Credential Reference.

Mender does not add a plaintext or SQLite fallback when the native keyring is unavailable.

A native-secret operation fails closed and surfaces a safe operational error.

### Environment source

Source environment stores only the environment variable name in Credential metadata.

Resolution reads the current Mender process environment at execution time.

This source exists for headless Linux, CI, container/service deployments, and installations without an available desktop keyring.

Environment variable values are never copied into SQLite, Profile, Provider config, Audit, or API responses.

### Resolver

The production CredentialResolver composes inventory metadata with secret backends.

~~~text
Resolve(reference)
  -> lookup Credential metadata
  -> source native      -> OS keyring Get
  -> source environment -> process environment lookup
  -> return fresh secret bytes
~~~

The resolver returns a fresh byte slice.

Callers remain responsible for minimizing secret lifetime.

### CRUD semantics

Normal management operations are:

- create native Credential with one transient secret value;
- create environment Credential with an environment-variable name;
- list/get metadata;
- rotate a native secret;
- delete a Credential when no governance/config references remain.

List/get responses never contain secret material and never echo the secret value.

Native creation writes the secret backend first and compensates if metadata persistence fails.

Native deletion removes secret material before metadata so failure prefers a visible-but-unresolvable Credential over an orphaned stored secret.

### No secret echo

Create/rotate requests may carry secret material across the local management boundary.

Responses never echo it.

Errors, Audit, logs, lifecycle events, generated SDK models, and Wails return models must not include it.

### Credential deletion safety

Credential deletion fails with an in-use conflict while the Reference is used by a Profile ToolBinding credential allowlist or a Provider credential binding.

The Credential Application consumes a usage-checking port; it does not import Profile or MCP persistence directly.

### Management UX

Credentials becomes a list-first management workflow.

Create Credential asks for namespace, name, display name, and source.

Native source additionally asks for the secret value.

Environment source asks for the environment-variable name.

The UI shows the derived Reference as read-only metadata after creation.

Normal management never asks the user to paste a complete credential:// Reference.

Rotate secret is a focused action for native Credentials only.

## Alternatives considered

### Store encrypted secrets in SQLite

This still requires a root encryption key and creates a new key-management problem.

### Store plaintext secrets in SQLite

Rejected because database access, backups, diagnostics, and migration tooling would all become secret-bearing surfaces.

### Native keyring only

Rejected because headless/CI Linux may not have a usable Secret Service or DBus session.

### Environment variables only

This preserves the current deployment model but provides no inventory, selector UX, rotation workflow, or persistent metadata.

### Introduce opaque CredentialID as primary identity

A separate opaque ID would reintroduce unnecessary copy/paste identity. Namespace + name already form a stable human-manageable identity.

## Consequences

### Positive

- secret bytes stay outside SQLite and normal application state;
- UI can provide a real Credential selector;
- desktop/local installs can use native OS secret storage;
- headless installs can remain environment-backed;
- deleting a referenced Credential fails safely;
- ToolBinding and Provider configuration can reference one canonical inventory.

### Negative / trade-offs

- native keyring behavior depends on host OS/session;
- WSL or headless Linux may require environment-backed Credentials;
- Credential creation/rotation introduces a sensitive local request path that requires strict no-logging handling;
- inventory metadata and native secret writes cannot be one database transaction.

## Follow-up

ADR-0021 defines how Provider authentication and Profile ToolBinding scope consume Credential References without leaking secret values into configuration or adapters.
