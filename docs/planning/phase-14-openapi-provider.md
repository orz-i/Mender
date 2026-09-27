# Phase 14 Plan — OpenAPI Provider Plugin

Status: In progress

## Objective

Use the Provider Plugin Runtime completed in Phase 13 to implement the first non-MCP Provider:

~~~text
io.mender.provider.openapi
~~~

The plugin converts selected OpenAPI 3 operations into canonical Mender Tools and executes them as governed HTTP calls.

The phase is successful only if OpenAPI support can be added without changing:

- Catalog Provider/Tool Domain semantics;
- Invocation orchestration stages;
- Profile ToolBinding Domain;
- Policy Domain;
- Audit Domain;
- generic Provider Instance Manager behavior.

ADR-0025 defines the architecture and security boundary.

## Current external baseline

As of 2026-09-27:

- OpenAPI 3.2.1 is the latest published specification, released 2026-09-10.
- operationId remains optional but unique when present.
- Security Requirement entries are OR alternatives; schemes inside one entry are AND.
- OpenAPI explicitly warns tooling about external reference retrieval, URI-resolution risks, cycles, and resource exhaustion.
- github.com/getkin/kin-openapi v0.145.0 is the current stable release and supports OpenAPI 3.0, 3.1, and 3.2.
- kin-openapi Loader exposes explicit external-reference policy. A custom ReadFromURIFunc bypasses IsExternalRefsAllowed, so Phase 14 intentionally does not install one.

References:

- https://spec.openapis.org/oas/v3.2.1.html
- https://github.com/getkin/kin-openapi/releases
- https://github.com/getkin/kin-openapi/blob/master/openapi3/loader.go

## Architectural acceptance test

Desired shape:

~~~text
Catalog
  ^
  | ToolSource
  |
OpenAPI Instance
  |
  +-- parsed document
  +-- operation index
  +-- HTTP client
  +-- security binding metadata
  |
  | ToolExecutor
  v
Invocation
~~~

Generic Runtime remains:

~~~text
ProviderID
  -> provider_configs
  -> Plugin Registry
  -> Provider Instance Manager
  -> ToolSource / ToolExecutor
~~~

No OpenAPI-specific lifecycle branch belongs in generic Runtime after plugin registration.

## Ubiquitous language

### OpenAPI Provider

One canonical Mender Provider whose plugin is io.mender.provider.openapi.

### Description source

The explicit source used to load the OpenAPI Description.

Initial kinds:

~~~text
url
file
~~~

### Operation

One executable OpenAPI Path Item HTTP operation.

### Operation SourceKey

Stable Provider-local identity used by Catalog synchronization.

### Effective server

The absolute HTTP(S) base URL used to execute one operation after applying OpenAPI server override semantics or an explicit Provider override.

### Security binding

Mapping from one OpenAPI Security Scheme name to one Mender Credential Reference.

### Supported operation

An operation whose method, parameters, content types, server, schemas, and authentication all fit Phase 14 execution semantics.

## Scope

Phase 14 includes:

- built-in OpenAPI Provider plugin registration;
- kin-openapi dependency pin;
- URL/file OpenAPI Description loading;
- OpenAPI 3.0/3.1/3.2 parsing and validation;
- internal reference resolution;
- explicit rejection of external references;
- operation include/exclude filtering;
- stable SourceKey generation;
- deterministic Tool Name/title/description/tag mapping;
- canonical input schema generation;
- OAS 3.0 schema normalization required for Mender JSON Schema validation;
- JSON request body handling;
- bounded path/query/header/cookie parameter serialization;
- effective server resolution;
- API key and HTTP Bearer Credential bindings;
- deterministic Security Requirement selection;
- HTTP execution;
- JSON/empty successful response mapping;
- non-2xx Tool failure mapping;
- bounded HTTP reads/timeouts;
- plugin config inspection for Credential usage;
- Runtime end-to-end tests;
- CLI creation workflow;
- generic management/API/Wails support;
- Web/Desktop OpenAPI Provider form;
- upgrade/product/development documentation;
- security and repository verification.

## Non-goals

Phase 14 does not include:

- Swagger/OpenAPI 2 conversion;
- external/multi-document reference resolution;
- callbacks as Tools;
- webhooks as Tools;
- OAuth2 token acquisition/refresh;
- OpenID Connect;
- mTLS;
- AWS-style/custom request signing;
- HTTP Basic account decomposition;
- multipart/form-data;
- application/x-www-form-urlencoded bodies;
- XML bodies;
- binary upload/download;
- streaming responses;
- SSE/WebSocket APIs;
- arbitrary response header exposure;
- long-running task adaptation;
- file watching/automatic sync;
- OpenAPI marketplace/registry;
- arbitrary plugin UI injection;
- external third-party plugin packaging.

## Package layout

Preferred backend layout:

~~~text
backend/internal/openapi/infrastructure/plugin/
  config.go
  plugin.go
  source.go
  document.go
  operation.go
  schema.go
  request.go
  response.go
  security.go
  http.go
  *_test.go
~~~

The generic Provider Plugin package must not import OpenAPI infrastructure.

## Dependency

Add to backend/go.mod:

~~~text
github.com/getkin/kin-openapi v0.145.0
~~~

The dependency is pinned directly.

Use the smallest package subset required.

## Plugin descriptor

Descriptor:

~~~text
PluginID      io.mender.provider.openapi
Version       1.0.0
DisplayName   OpenAPI
ConfigVersion 1
~~~

## Configuration v1

Conceptual shape:

~~~go
type Config struct {
    Source      SourceConfig
    ServerURL   string
    Operations  OperationFilter
    Credentials map[string]credentialdomain.Reference
}

type SourceConfig struct {
    Kind string
    URL  string
    Path string
}

type OperationFilter struct {
    Include []string
    Exclude []string
}
~~~

Persisted JSON:

~~~json
{
  "source": {
    "kind": "url",
    "url": "https://example.com/openapi.yaml"
  },
  "serverUrl": "https://api.example.com",
  "operations": {
    "include": ["getUser"],
    "exclude": []
  },
  "credentials": {
    "ApiKeyAuth": "credential://example/api"
  }
}
~~~

File source:

~~~json
{
  "source": {
    "kind": "file",
    "path": "/absolute/path/to/openapi.yaml"
  }
}
~~~

### Configuration invariants

- source kind is exactly url or file;
- URL source contains URL only;
- file source contains absolute path only;
- serverUrl is optional but if present must be valid absolute HTTP(S);
- include/exclude values are trimmed, unique, bounded;
- a SourceKey cannot appear in both include and exclude;
- Credential map key is a bounded Security Scheme name;
- every Credential Reference parses canonically;
- no resolved secret value can exist in config;
- JSON unknown fields are rejected.

## Source loading

### URL source

Validation:

- absolute http or https;
- no URL userinfo;
- plain HTTP only on localhost/loopback;
- no fragment.

Runtime read:

- bounded timeout;
- maximum document bytes: initially 16 MiB;
- explicit Accept for JSON/YAML/text;
- redirects disabled or tightly bounded with target revalidation;
- non-2xx fetch is activation failure;
- body read through a byte limit plus overflow detection.

### File source

Validation:

- absolute path;
- non-empty;
- no implicit working-directory interpretation.

Runtime read:

- regular file;
- maximum 16 MiB;
- no sibling/reference reads through the parser.

## OpenAPI loader policy

Construct a fresh loader per instance/document load.

Required policy:

~~~text
Context               = activation context
IsExternalRefsAllowed = false
ReadFromURIFunc        = nil
~~~

Then:

~~~text
parse
-> resolve internal refs
-> validate document
-> index supported operations
~~~

Validation diagnostics returned to management are bounded.

## Source version support

Accept OpenAPI major version 3 only.

Reviewed families:

~~~text
3.0.x
3.1.x
3.2.x
~~~

Unknown/future minor versions fail closed until reviewed.

## External reference detection

Tests must prove these are rejected:

~~~text
$ref: "./schemas.yaml#/..."
$ref: "https://example.com/schema.yaml#/..."
$ref: "file:///tmp/schema.yaml#/..."
~~~

Internal cyclic references remain supported where the generated Tool-local $defs graph can represent them.

## Operation enumeration

Enumerate Paths deterministically.

Stable ordering:

~~~text
path lexical
method fixed order
~~~

Fixed method order:

~~~text
GET
POST
PUT
PATCH
DELETE
~~~

For every selected operation collect:

- method;
- path template;
- operation object;
- effective parameters after operation overrides;
- effective server;
- effective Security Requirements;
- request JSON media type/schema;
- success response metadata.

## SourceKey

Algorithm:

~~~text
if operationId is non-empty:
  SourceKey = operationId
else:
  SourceKey = lower(method) + " " + path
~~~

Examples:

~~~text
getUser
get /users/{id}
~~~

Reject duplicate SourceKeys defensively.

## Tool Name

Machine-name algorithm:

1. prefer operationId;
2. otherwise method + normalized path;
3. replace unsupported characters with underscore;
4. bound to Tool name limit;
5. detect snapshot collision;
6. append stable hash suffix derived from SourceKey on collision/truncation.

Tests cover punctuation, Unicode, long names, and collisions.

## Tool presentation

Title precedence:

~~~text
operation.summary
operationId
SourceKey
~~~

Description uses operation summary/description plus method/path without redundant duplication.

Initial tags:

~~~text
openapi
http
http:<method>
oas:<operation-tag>
~~~

All metadata is bounded to Catalog invariants.

## Operation filtering

Configuration filter works on SourceKey.

Algorithm:

~~~text
all operations
  -> if include non-empty, retain include set
  -> remove exclude set
  -> validate retained operations
~~~

Unknown exact include entries fail sync.

Unsupported selected operations fail synchronization with a bounded diagnostic rather than being silently omitted.

## Supported operation validation

Reject selected operations that require:

- unsupported HTTP method;
- unsupported parameter style/content;
- request media type outside JSON subset;
- successful body outside JSON subset;
- unresolved/invalid server;
- unsupported authentication with no satisfiable alternative;
- source schema that cannot be normalized safely;
- sensitive ordinary parameters that should be Credential bindings.

Errors name SourceKey plus bounded category/reason.

## Canonical Tool input schema

Every Tool schema has an object root:

~~~json
{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "path": {},
    "query": {},
    "headers": {},
    "cookies": {},
    "body": {}
  }
}
~~~

Only groups present for the operation are emitted.

Example call arguments:

~~~json
{
  "path": { "id": "123" },
  "query": { "limit": 20 },
  "body": { "name": "updated" }
}
~~~

Authentication never appears in this object.

### Path

All OpenAPI path parameters are required.

### Query

Requiredness follows OpenAPI.

### Headers

Only ordinary non-authentication headers are included.

Header name matching is case-insensitive for sensitive screening.

### Cookies

Ordinary cookie parameters may be supported only when clearly non-authentication metadata.

### Body

The body property uses the selected JSON request body schema.

If requestBody is required, root required includes body.

## Schema normalization

### Self-contained schema graph

Extracted Tool schemas cannot retain broken refs such as:

~~~text
#/components/schemas/User
~~~

Build Tool-local:

~~~json
{
  "$defs": {
    "User": {}
  }
}
~~~

and rewrite refs:

~~~text
#/components/schemas/User
-> #/$defs/User
~~~

Preserve cycles through $defs rather than recursive inline expansion.

### OAS 3.1 / 3.2

Normalize supported JSON Schema 2020-12 fields conservatively.

### OAS 3.0

Translate at minimum:

- nullable true -> null-inclusive JSON Schema;
- boolean exclusiveMinimum / exclusiveMaximum -> numeric JSON Schema form;
- component refs -> Tool-local $defs.

Request-side normalization should avoid requiring readOnly values where practical.

Executor-level OpenAPI request validation remains defense in depth.

## Parameter serialization

Implement explicit serializers.

### Path

Initial:

- default simple style;
- scalar;
- array only when covered by tests.

### Query

Initial:

- default form;
- scalar;
- array explode true/false;
- deepObject only if implemented comprehensively.

### Header

Initial:

- default simple;
- scalar;
- bounded array.

Reject CR/LF/NUL in constructed header values.

### Cookie

Initial:

- default form;
- scalar;
- bounded array where supported.

## Sensitive ordinary parameters

Reject selected operations with ordinary header/cookie names that clearly represent secrets.

At minimum screen bounded names such as:

~~~text
Authorization
Proxy-Authorization
X-API-Key
api-key
access-token
token
password
secret
session
~~~

Reuse existing repository conventions where practical.

A future phase may add explicit parameter-to-Credential bindings.

## Request body media types

Selection preference:

1. exact application/json;
2. deterministic concrete media type ending in +json.

Reject unsupported body media types.

Set selected Content-Type on execution.

## Effective server

### Explicit override

If serverUrl exists, use it for every selected operation.

### Document server

Otherwise choose according to OpenAPI override rules:

~~~text
operation.servers[0]
pathItem.servers[0]
doc.servers[0]
~~~

Variables use declared defaults.

Final URL:

- absolute HTTP(S) or explicitly resolved relative URL;
- no userinfo;
- HTTP loopback-only.

For URL source, relative server can be resolved against the description URL when unambiguous.

For file source, relative/absent server requires explicit serverUrl.

## Security Scheme binding

Config example:

~~~json
"credentials": {
  "ApiKeyAuth": "credential://demo/api",
  "BearerAuth": "credential://demo/token"
}
~~~

### Supported apiKey

Locations:

- header;
- query;
- cookie.

The description defines placement/name; config chooses Credential Reference.

### Supported HTTP bearer

Require type http and scheme bearer.

Execution adds Bearer authorization.

### Unsupported security

Basic/OAuth2/OpenID/mTLS do not become partially working Tools.

If another OR alternative is supported/configured, that alternative may be selected.

## Security Requirement selection

For each operation:

1. use operation security override when present, otherwise root security;
2. absent/empty security means anonymous;
3. iterate alternatives deterministically;
4. empty requirement object is anonymous;
5. non-empty requirement is satisfiable only if every scheme exists, is supported, and has a configured Credential binding;
6. choose first satisfiable alternative;
7. if none, reject operation during sync.

## Config inspection

InspectConfig:

- strict-decodes config;
- returns all configured Credential References through generic ConfigMetadata;
- does not fetch/parse the description;
- does not open network connections;
- does not resolve secrets.

## HTTP client policy

One reusable HTTP client per Provider instance.

Settings:

- context-driven/bounded timeout;
- no automatic redirect following;
- no cookie jar/session persistence;
- explicit review of proxy behavior;
- no plugin-configured ambient credentials.

## Request construction

Given ExecutionRequest:

1. locate operation by request.Tool.SourceKey;
2. decode location maps/body;
3. serialize path;
4. serialize query;
5. serialize ordinary headers/cookies;
6. resolve operation Credential Refs;
7. apply Security Scheme values;
8. marshal JSON body;
9. build request with effective server + path;
10. validate against OpenAPI operation where practical;
11. execute;
12. map response.

SourceKey, not Tool Name, is authoritative dispatch identity.

## Credential handling

For each required ref:

~~~text
CredentialResolver.Resolve
-> use only long enough to construct outbound request
-> zero returned bytes where practical
~~~

Never put secret values in:

- errors;
- logging;
- Audit;
- URL userinfo;
- Catalog;
- Provider config;
- Tool args.

## Response handling

### Status

2xx is success.

All other status codes become Tool failures.

3xx is not followed automatically.

### Read bound

Initial HTTP response cap: 8 MiB plus overflow detection.

### Success with no body

Map to JSON null.

### JSON success

Parse body as JSON and return the JSON value directly as canonical complete content.

No status/body envelope is added in Phase 14.

### Non-2xx

Failure message may include:

- status;
- bounded printable/redacted text or JSON summary.

Never persist whole error body.

## Output schema discovery

Collect successful 2xx response forms.

For each:

- no body -> null schema;
- supported JSON schema -> normalized schema;
- unsupported non-JSON body -> operation unsupported;
- missing schema -> may force OutputSchema omission.

If multiple reliable forms exist, use bounded anyOf.

If output cannot be represented reliably, leave OutputSchema nil.

## Error classification

Examples:

- description load/fetch failure -> Provider activation/sync error;
- invalid OpenAPI document -> source validation error;
- unsupported selected operation -> sync error;
- missing scoped Credential -> existing credential-outside-scope behavior;
- non-2xx -> Tool error;
- response too large -> bounded operational failure;
- declared JSON that is invalid JSON -> bounded Tool/Provider error.

Avoid new Domain classes unless generic consumers require them.

## Synchronization

DiscoverTools uses the already loaded instance document.

Generic SyncConfiguredProvider reloads the instance, therefore refreshes the description.

Stable SourceKey preserves ToolID.

Removed operations are removed by existing snapshot replacement.

## Large API behavior

OpenAPI discovery feeds the canonical Catalog.

Mender continues:

~~~text
browse/search
-> describe
-> invoke
~~~

No eager model schema injection is added.

Add generated large-spec tests with hundreds or about 1,000 operations for deterministic discovery/filter behavior.

## Generic management API

No protocol-specific top-level management endpoint is needed.

Existing generic endpoints remain:

~~~text
GET  /provider-plugins
POST /providers
PUT  /providers/{providerId}
POST /providers/{providerId}/sync
~~~

Provider config remains small because it stores source location/filter/auth refs, not full document bytes.

## Web/Desktop UX

### Add Provider

Step 1:

~~~text
Provider
- Namespace
- Name
- Description
- Plugin: MCP | OpenAPI
~~~

When OpenAPI selected:

### Description

~~~text
Source: Remote URL | Local file
Spec URL / Absolute file path
Optional API server override
~~~

### Operations

Initial controls:

- include SourceKeys;
- exclude SourceKeys.

Future work can replace this with preview/picker.

### Authentication

Security Scheme names come from the description.

Ideal UX is a document preview/inspect step. If that expands the phase too far, v1 may use structured scheme-name plus inventory Credential rows validated on sync.

Never ask for raw Credential Reference when inventory selection is available.

### View Provider

Show:

- Plugin = OpenAPI;
- source kind/location;
- server override;
- operation filter counts;
- configured security scheme names;
- synchronized Tool count.

No resolved secrets.

## CLI

Add:

~~~text
mender provider add openapi
~~~

Flags:

~~~text
--namespace
--name
--description
--spec-url
--spec-file
--server
--include
--exclude
~~~

Exactly one of spec-url/spec-file.

Credential mapping can use repeated structured flags only if grammar remains clear.

Generic provider plugins/list/sync require no special change.

## Test strategy

### Config

- URL/file exclusivity;
- absolute file;
- endpoint policy;
- duplicate include/exclude;
- Credential refs;
- unknown fields;
- InspectConfig refs sorted/deduped.

### Source

- URL fetch;
- file read;
- size overflow;
- timeout;
- non-2xx;
- redirect refusal;
- external ref refusal.

### Document

Fixtures:

- 3.0;
- 3.1;
- 3.2;
- invalid version;
- invalid description;
- internal refs;
- cycles;
- external refs;
- server variables.

### Operation identity

- operationId;
- fallback method/path;
- stable sorting;
- name collisions;
- include/exclude.

### Schema

- path/query/body groups;
- requiredness;
- component refs -> $defs;
- recursive schema;
- 3.0 nullable;
- 3.0 exclusive bounds;
- 3.1 type arrays.

### Parameters

- path escaping;
- scalar query;
- array query;
- header CRLF rejection;
- cookie encoding;
- unsupported style rejection.

### Security

- anonymous;
- apiKey header/query/cookie;
- bearer;
- AND;
- OR;
- empty requirement;
- unsupported scheme fallback;
- no satisfiable requirement;
- Profile resolver denying ref.

### Execution

Use httptest.Server.

Cover:

- GET path/query;
- POST JSON;
- response JSON;
- no-content/null;
- non-2xx;
- response too large;
- redirect not followed;
- cancellation;
- output schema.

### Runtime integration

Full path:

~~~text
RegisterConfiguredProvider(OpenAPI)
-> SyncConfiguredProvider
-> Catalog search/describe
-> CallConfiguredTool
-> Profile binding
-> governed Invocation
-> Audit
~~~

Confirm:

- generic Runtime has no OpenAPI switch;
- instance reuse;
- source update + sync preserves ToolID for same SourceKey;
- operation removal deletes Tool;
- Profile Credential scope denies missing auth ref.

### Adapters

- CLI add OpenAPI URL/file;
- provider plugin list includes OpenAPI;
- generic HTTP create/update;
- Wails binding;
- shared Web/Desktop form;
- SDK strict/generated.

### Security

- external-ref SSRF fixture;
- file external-ref fixture;
- source cap;
- response cap;
- auth parameters excluded from Tool schema;
- secret absent from config/catalog/audit/errors;
- malicious Markdown stays text.

## Work packages and commit segmentation

### 1. Architecture and phase plan

Files:

- docs/architecture/adr/0025-openapi-provider-plugin.md
- ADR index
- this plan
- roadmap

Commit:

~~~text
docs(architecture): define openapi provider plugin
~~~

### 2. Dependency and plugin config

Pin parser dependency and implement descriptor/config/InspectConfig.

Commit:

~~~text
feat(openapi): add provider plugin configuration
~~~

### 3. Safe document loading

URL/file readers, byte/time bounds, external-ref rejection, document validation.

Commit:

~~~text
feat(openapi): load bounded api descriptions
~~~

### 4. Operation discovery and schemas

SourceKey/name/presentation, filters, schema normalization, Tool discovery.

Commit:

~~~text
feat(openapi): discover operations as tools
~~~

### 5. HTTP execution

Server resolution, parameter/body serialization, response mapping.

Commit:

~~~text
feat(openapi): execute json http operations
~~~

### 6. Security Scheme bindings

apiKey/bearer and Security Requirement selection.

Commit:

~~~text
feat(openapi): bind operation authentication
~~~

### 7. Runtime registration and end-to-end coverage

Register built-in plugin and prove generic lifecycle/governance.

Commit:

~~~text
test(runtime): cover openapi provider end to end
~~~

### 8. CLI and management adapters

CLI creation path plus any required generic DTO typing.

Commit:

~~~text
feat(adapters): expose openapi provider configuration
~~~

### 9. Web/Desktop UX

Dedicated OpenAPI config form under plugin-first shell.

Commit:

~~~text
feat(app): manage openapi providers
~~~

### 10. Hardening and scale verification

Large-document/filter/security/race coverage.

Commit:

~~~text
test(openapi): harden provider execution
~~~

### 11. Closeout

Product/development/upgrade docs and verification evidence.

Commit:

~~~text
docs: complete openapi provider phase
~~~

## Exit criteria

Phase 14 is complete when:

- [ ] io.mender.provider.openapi is a registered built-in plugin.
- [ ] kin-openapi is pinned and isolated to OpenAPI infrastructure.
- [ ] OpenAPI 3.0/3.1/3.2 fixtures parse/validate.
- [ ] external refs are rejected.
- [ ] URL/file source reads are bounded.
- [ ] selected operations produce deterministic SourceKeys and Tool names.
- [ ] canonical Tool input schemas are object-rooted and self-contained.
- [ ] OAS 3.0 nullable/exclusive semantics required by fixtures are normalized.
- [ ] internal component refs/cycles work through Tool-local $defs.
- [ ] include/exclude filtering is deterministic.
- [ ] JSON GET/POST/PUT/PATCH/DELETE operations execute.
- [ ] supported parameter serialization is covered.
- [ ] application/json and concrete +json bodies work.
- [ ] apiKey header/query/cookie works through Credential References.
- [ ] HTTP Bearer works through Credential References.
- [ ] Security Requirement AND/OR semantics are preserved for supported schemes.
- [ ] auth secrets never appear in Tool input schemas.
- [ ] Profile Credential scope blocks unauthorized OpenAPI calls.
- [ ] redirect behavior cannot leak Provider credentials.
- [ ] response bodies are bounded and JSON successes normalize to canonical Outcomes.
- [ ] non-2xx maps to expected Tool failure.
- [ ] stable SourceKey preserves ToolID across description refresh.
- [ ] generic Runtime contains no OpenAPI-specific lifecycle branch.
- [ ] CLI supports plugin-first OpenAPI registration.
- [ ] Web/Desktop support OpenAPI configuration.
- [ ] OpenAPI/SDK/Wails contracts remain generated and strict.
- [ ] large-catalog filtering is covered.
- [ ] Go race/lint/tests pass.
- [ ] frontend lint/typecheck/tests/build pass.
- [ ] security scans pass.

## Deferred follow-up

Candidates after Phase 14:

- reviewed external/multi-document refs;
- document upload/import with safe management projection;
- operation preview/picker before Provider creation;
- OAuth2/OIDC token management;
- HTTP Basic/mTLS;
- multipart/form/binary/streaming;
- OpenAPI overlays;
- richer response/header metadata;
- model-hosted Tool Bridge Provider.
