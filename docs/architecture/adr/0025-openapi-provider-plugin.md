# ADR-0025: Map OpenAPI operations into governed Tools through a bounded Provider plugin

- Status: Accepted
- Date: 2026-09-27

## Context

Phase 13 established Provider Plugin Runtime as the extension boundary for upstream Tool sources.

The next architectural validation is intentionally non-MCP: turn an OpenAPI Description into canonical Mender Tools and execute those Tools through ordinary HTTP APIs.

This integration is valuable because it proves that Provider plugins are not merely a renamed MCP transport abstraction. An OpenAPI Provider has materially different semantics:

- discovery comes from an API description rather than `tools/list`;
- execution constructs HTTP requests directly rather than calling an MCP Tool;
- authentication can differ per operation;
- parameter locations and serialization are part of the source protocol;
- one Provider document can expose a very large operation catalog;
- OpenAPI descriptions can contain references, Markdown, server templates, callbacks, and security alternatives;
- OpenAPI Schema dialects differ between OAS 3.0 and OAS 3.1+;
- remote descriptions and external references can accidentally become SSRF/file-read capabilities.

The current OpenAPI Specification release is 3.2.1 (2026-09-10). OAS 3.2.1 continues to define `operationId` as an optional but unique operation identity, allows operation/path/root server overrides, and defines Security Requirement alternatives as OR while schemes inside one requirement are AND.

Mender must support useful OpenAPI APIs without allowing the source protocol to bypass:

- canonical Tool identity;
- input validation;
- Profile scope;
- Policy;
- Credential scope;
- execution/result guards;
- Audit;
- Provider instance lifecycle.

The first implementation also needs a realistic security boundary. The OpenAPI Specification explicitly warns that external resources can be dereferenced by tooling, references can create cycles/resource exhaustion, and URI/base-URI handling can redirect retrieval to unintended locations.

## Decision

Mender will add a built-in OpenAPI Provider plugin:

~~~text
io.mender.provider.openapi
~~~

It maps selected OpenAPI 3 operations into canonical Mender Tools and executes them through the shared Invocation pipeline.

### Dependency

The initial implementation pins:

~~~text
github.com/getkin/kin-openapi v0.145.0
~~~

The library is used only inside the OpenAPI plugin/infrastructure package.

Reasons:

- current support for OpenAPI 3.0, 3.1, and 3.2;
- document validation;
- resolved OpenAPI object model;
- request/response schema validation support;
- explicit Loader control over external references;
- active maintenance and a narrow fit for runtime request execution.

No kin-openapi types enter Catalog, Invocation, Profile, Policy, Credential, Audit, or generic Provider Plugin contracts.

### Supported OpenAPI versions

Phase 14 accepts OpenAPI 3.x documents supported by the pinned parser:

~~~text
3.0.x
3.1.x
3.2.x
~~~

Swagger/OpenAPI 2.0 conversion is not enabled in this phase.

Unsupported/future document versions fail closed.

### Configuration

OpenAPI plugin configuration version 1 is conceptually:

~~~json
{
  "source": {
    "kind": "url",
    "url": "https://example.com/openapi.yaml"
  },
  "serverUrl": "https://api.example.com",
  "operations": {
    "include": [],
    "exclude": []
  },
  "credentials": {
    "ApiKeyAuth": "credential://example/api-key"
  }
}
~~~

File source is also supported:

~~~json
{
  "source": {
    "kind": "file",
    "path": "/absolute/path/to/openapi.yaml"
  }
}
~~~

Rules:

- exactly one source kind is configured;
- file paths must be absolute;
- configured source URL and execution server URL must pass explicit HTTP endpoint policy;
- non-loopback HTTP is rejected in favor of HTTPS;
- URL userinfo is rejected;
- `serverUrl` is optional only when each selected operation can resolve an absolute effective server;
- include/exclude entries use canonical OpenAPI Tool SourceKeys;
- Credential values are never persisted, only Credential References.

### Description loading

Description retrieval is explicit Provider configuration, not arbitrary reference resolution.

For URL sources:

- only `http` / `https` are accepted;
- non-loopback HTTP is rejected;
- userinfo is rejected;
- response size is bounded;
- fetch timeout is bounded;
- redirect behavior is bounded and validated;
- the response is treated as untrusted bytes.

For file sources:

- the configured path is explicit and absolute;
- input size is bounded;
- only the configured file is read.

### External references

Phase 14 supports **internal references only**.

~~~text
$ref: "#/..."
~~~

External document/file/network references are rejected by the OpenAPI loader.

Mender does not install a permissive `ReadFromURIFunc`.

This is an intentional security/product boundary, not a parser limitation. Multi-document OpenAPI descriptions are deferred until Mender has a reviewed reference resolver with:

- scheme/host policy;
- source-root boundaries;
- redirect policy;
- byte/depth/document-count budgets;
- cycle handling;
- deterministic caching;
- clear local-file authority semantics.

### Operation selection

The plugin considers ordinary HTTP operations under `paths`.

Initial executable methods:

~~~text
GET
POST
PUT
PATCH
DELETE
~~~

Callbacks and webhooks are not converted into Tools.

Operation filters apply to canonical SourceKeys:

- empty `include` means all otherwise-supported operations;
- non-empty `include` is an allowlist;
- `exclude` removes matches after inclusion;
- duplicate/unknown filter entries are validation errors when practical.

Unsupported selected operations fail synchronization with a bounded diagnostic naming the SourceKey and unsupported feature. They are not silently omitted.

This makes missing Tools observable and lets users intentionally filter a large API to the subset Mender can safely execute.

### Tool SourceKey

Canonical Tool SourceKey is stable within one Provider:

~~~text
operationId
~~~

when a non-empty `operationId` exists.

Otherwise:

~~~text
<lowercase-method> <path-template>
~~~

Examples:

~~~text
getUser
get /users/{id}
~~~

The OAS document validator is relied upon for operationId uniqueness when present.

Fallback SourceKeys are unique because one Path Item can contain at most one operation for a method.

Changing operationId, method, or path therefore changes SourceKey and is treated as source identity change, consistent with the existing Catalog synchronization model.

### Tool machine name

Tool `Name` is derived deterministically from the SourceKey/operation metadata and sanitized for cross-adapter machine use.

Rules:

- prefer operationId when it already fits Mender/MCP machine-name constraints;
- otherwise derive a method/path slug;
- unsupported runes become underscores;
- bounded length;
- deterministic hash suffix resolves sanitization/truncation collisions;
- names are unique inside one Provider snapshot.

Tool ID remains Mender-generated and is not derived from Tool Name.

### Tool presentation

Mapping:

~~~text
title       <- operation.summary, then operationId/SourceKey fallback
description <- operation.description with summary when useful
tags        <- openapi, http, method, bounded operation tags
~~~

OpenAPI CommonMark content is treated as text metadata.

The plugin does not render upstream Markdown/HTML as trusted executable UI.

### Canonical input shape

Every OpenAPI Tool InputSchema has an object root.

Parameters are namespaced by OpenAPI location to avoid collisions:

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

Examples:

~~~json
{
  "path": { "id": "123" },
  "query": { "limit": 20 },
  "body": { "name": "updated" }
}
~~~

Authentication is never represented in this Tool input object.

### Schema normalization

Canonical Catalog schema remains JSON Schema.

For OAS 3.1/3.2, Schema Objects are normalized into self-contained JSON Schema suitable for Mender validation.

For OAS 3.0, OpenAPI-specific schema behavior is translated where required, including at minimum:

- `nullable`;
- boolean `exclusiveMinimum` / `exclusiveMaximum`;
- internal component schema references.

Internal OpenAPI component references are rewritten into a Tool-local `$defs` graph so schemas do not retain broken `#/components/...` references after extraction.

Reference cycles are represented through `$defs` rather than recursively inlined.

The generated canonical schema is deterministic for an unchanged document.

### Parameters and serialization

Phase 14 supports a bounded parameter subset.

Supported locations:

~~~text
path
query
header
cookie
~~~

Initial serialization support focuses on standard/default OpenAPI forms for:

- scalar string/number/integer/boolean values;
- arrays where the standard style/explode combination is deterministic;
- query `deepObject` only if implemented and covered by tests.

Operations requiring unsupported parameter content/style/serialization fail synchronization unless filtered out.

Authentication-like header/cookie parameters must not become ordinary Tool arguments. Security credentials belong in OpenAPI Security Scheme bindings.

### Request body

Initial body support is JSON only.

Supported content types:

- `application/json`;
- concrete `application/*+json` media types.

When more than one JSON media type exists, selection is deterministic and documented.

Unsupported body media types such as multipart, form-encoded, XML, arbitrary binary, and streaming content fail synchronization unless the operation is filtered out.

The Tool input `body` property uses the selected request body schema.

### Server resolution

Effective OpenAPI server follows the protocol override hierarchy:

~~~text
Operation servers
  -> Path Item servers
  -> OpenAPI root servers
~~~

The plugin chooses one server deterministically.

Configured `serverUrl`, when present, explicitly overrides document servers for every operation.

Server variables may use declared defaults.

The final execution base URL must be absolute HTTP(S), contain no userinfo, and pass the same non-loopback HTTPS rule used by other remote Provider connections.

Relative document server URLs may be resolved against a URL description source when unambiguous. File sources require an explicit absolute `serverUrl` when the description does not provide an absolute server.

### Authentication

OpenAPI Security Scheme credentials are bound through Mender Credential References.

Phase 14 supports:

- `apiKey` in header/query/cookie;
- HTTP Bearer authentication.

Deferred:

- HTTP Basic credential decomposition;
- OAuth2 token acquisition/refresh;
- OpenID Connect flows;
- mutual TLS;
- arbitrary custom signing schemes.

Security Requirement semantics are preserved:

- schemes inside one requirement are AND;
- requirement objects are OR alternatives;
- `{}` is anonymous access.

For each selected operation, the plugin chooses a deterministic satisfiable requirement based on configured supported bindings.

If no requirement is satisfiable, synchronization fails for that operation rather than discovering a Tool that cannot execute.

The chosen operation requirement determines which Credential References are resolved at call time.

### Credential scope

`InspectConfig` returns all Credential References present in OpenAPI security bindings.

At execution time the plugin resolves only the Credential References required by the selected operation.

Therefore Profile ToolBinding Credential scope remains effective:

~~~text
Profile Binding resolver
  -> OpenAPI operation executor
  -> resolve required scheme credential
  -> denied if outside binding scope
~~~

Secret bytes never enter Tool schemas, descriptions, Provider projections, Policy, or normal Audit.

### HTTP execution

The Provider instance owns:

- parsed/validated document;
- selected operation index keyed by Tool SourceKey;
- deterministic request construction metadata;
- HTTP client.

Execution:

~~~text
canonical Tool arguments
  -> operation lookup by SourceKey
  -> construct HTTP request
  -> apply path/query/header/cookie parameters
  -> apply JSON body
  -> apply selected security requirement
  -> validate request against OpenAPI operation where practical
  -> execute
  -> validate/normalize response
  -> canonical Outcome
~~~

The HTTP client does not follow arbitrary redirects while carrying Provider credentials.

Redirect responses are treated as explicit upstream responses/failures unless a separately reviewed same-origin redirect policy is added.

### Response semantics

Initial successful responses are JSON-oriented.

For successful 2xx responses:

- empty body maps to JSON `null`;
- JSON body maps to canonical JSON content;
- declared JSON response schemas may produce a canonical Tool OutputSchema;
- multiple successful schemas may be represented through a bounded `anyOf`.

Unsupported successful non-JSON/binary/streaming responses cause synchronization rejection for the operation unless no response body is expected.

Non-2xx responses map to an expected Tool failure with:

- status code;
- bounded, sanitized diagnostic text where safe;
- no unbounded raw response persistence.

HTTP response bodies are read with an explicit byte limit before parsing.

### Output schema

OutputSchema is emitted only when Mender can describe the possible successful canonical JSON result reliably.

If successful responses have no body, `null` is represented.

If response schemas are absent/ambiguous beyond the supported subset, OutputSchema may be omitted rather than inventing a false guarantee.

Result Guard remains authoritative whenever an OutputSchema is present.

### Instance lifecycle

OpenAPI Provider instances use the generic Provider Instance Manager unchanged.

The instance is safe to reuse concurrently for calls after document discovery.

Updating Provider config changes the generic config revision and causes normal Manager replacement.

No OpenAPI-specific activation lifecycle is added to Runtime.

### Management UX

Provider creation remains plugin-first.

Selecting `OpenAPI` reveals an OpenAPI-owned configuration form.

Initial form concepts:

1. Description source
   - URL or absolute local file path.
2. API server
   - optional absolute override.
3. Operation selection
   - optional include/exclude SourceKeys.
4. Authentication
   - inventory-backed Security Scheme bindings after document inspection where practical.

Normal UX does not ask for ProviderID, ToolID, or raw Credential Reference text.

No arbitrary plugin JavaScript/HTML is injected.

### CLI

CLI gains an OpenAPI plugin path, conceptually:

~~~text
mender provider add openapi   --namespace example   --name "Example API"   --spec https://example.com/openapi.yaml   --server https://api.example.com
~~~

File source may use a separate `--spec-file` flag.

Generic `provider plugins`, `provider list`, and `provider sync` require no OpenAPI-specific changes.

Authentication binding flags may remain deferred to structured management surfaces if a concise safe CLI grammar is not available.

## Security invariants

1. OpenAPI descriptions are untrusted input.
2. External `$ref` retrieval is disabled in Phase 14.
3. Explicit description URL/file reads are size/time bounded.
4. No URL userinfo is accepted.
5. Non-loopback HTTP is rejected for source/server configuration.
6. Security Scheme secrets come only from CredentialResolver.
7. OpenAPI Security Scheme credentials never become Tool arguments.
8. Authentication-like ordinary parameters are rejected rather than injected into model-visible schemas.
9. Provider HTTP redirects cannot leak credentials to another origin.
10. Response reads are byte-bounded.
11. Markdown/description text is not trusted HTML.
12. All invocation still enters shared Mender Policy/Profile/Audit governance.
13. Unsupported operations fail visibly rather than silently weakening semantics.

## Alternatives considered

### Generate an MCP server from OpenAPI and consume it as MCP

This could reuse the existing MCP plugin but adds an unnecessary protocol hop, another process/server lifecycle, duplicated authentication mapping, and hides source provenance behind generated MCP semantics.

Rejected because Phase 13 explicitly exists so non-MCP providers can implement ToolSource/ToolExecutor directly.

### Convert every OpenAPI parameter into a flat Tool input object

This is compact but creates collisions between path/query/header/cookie names and makes serialization origin ambiguous.

Rejected in favor of location-namespaced inputs.

### Treat security headers/keys as normal Tool arguments

This is easy to generate but puts secret material into model-visible schemas/prompts and bypasses Credential scope.

Rejected.

### Permit all external references through the parser

This maximizes compatibility with multi-file descriptions.

Rejected for the first phase because it converts untrusted `$ref` values into hidden filesystem/network authority and complicates cycle/resource budgets.

### Support every OpenAPI media type/style immediately

This would increase apparent compatibility but make execution semantics and canonical Outcomes unreliable.

Rejected in favor of an explicit JSON-focused subset whose unsupported operations fail synchronization.

### Store the full OpenAPI document in Provider config

This makes upload/import self-contained but causes large Provider projections, duplicates source data in SQLite, complicates update detection, and may persist examples/document content that users did not expect Mender to copy.

Rejected for Phase 14. Provider config stores source location and execution/auth policy; document bytes are loaded into the active instance.

## Consequences

### Positive

- proves Provider plugins are protocol-independent rather than MCP transport wrappers;
- large HTTP APIs become governed Mender Tool catalogs without bespoke adapters;
- Profile/Policy/Credential/Audit behavior remains shared;
- stable SourceKeys preserve Tool IDs across ordinary document refreshes;
- location-namespaced input schemas avoid parameter ambiguity;
- Credential inventory maps naturally to OpenAPI security schemes;
- external reference authority is bounded from the start;
- JSON-only execution matches current canonical Outcome/result-guard model.

### Negative / trade-offs

- Phase 14 intentionally supports only a subset of OpenAPI execution semantics;
- multi-document descriptions with external refs require bundling first;
- OAuth2/mTLS APIs need later authentication extensions;
- multipart/binary/streaming APIs are unavailable initially;
- OAS 3.0 schema normalization requires adapter-specific conversion;
- users may need include/exclude filters for large specs containing unsupported operations;
- file-based sources are local/deployment-specific;
- document refresh changes are observed on Provider sync/reload rather than continuously watched.

## Follow-up

Phase 14 implements the OpenAPI Provider plugin in segmented work:

1. architecture/phase plan;
2. pin parser dependency and config/source security boundaries;
3. load/validate descriptions with external refs disabled;
4. discover operations and generate canonical Tool schemas;
5. implement HTTP request serialization and JSON response mapping;
6. implement Security Scheme Credential bindings;
7. register the built-in plugin and add Runtime end-to-end tests;
8. add CLI/API/Wails/Web/Desktop management surfaces;
9. validate large-catalog filtering and security failure cases;
10. close out docs and verification.

Later phases may add reviewed multi-document reference resolution, OAuth flows, multipart/binary content, richer operation discovery previews, or model-hosted Tool Provider plugins.
