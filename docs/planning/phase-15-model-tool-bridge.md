# Phase 15 Plan — Model Tool Bridge Provider

Status: Complete

## Objective

Implement the first Mender Provider whose upstream execution is a model-provider-hosted agentic tool loop.

Plugin:

~~~text
io.mender.provider.model-tools
~~~

Initial engines:

~~~text
openai
anthropic
gemini
~~~

Initial normalized capability:

~~~text
web_search
~~~

The provider model is an implementation detail of one governed Mender Tool invocation. It must not become a general agent over Mender's Catalog.

ADR-0026 defines the architecture and security boundary.

## External baseline

Reviewed on 2026-09-27.

OpenAI:
- Responses API is the recommended agentic API.
- web_search is a hosted tool.
- Responses returns web_search_call items and final text with URL citations.
- required/specific tool choice can require search.

Anthropic:
- server tools execute on Anthropic infrastructure.
- web search returns server_tool_use + web_search_tool_result blocks and cited final text.
- provider-side execution can return pause_turn.
- forced tool choice is not uniformly supported by current models/settings, so Mender verifies actual tool execution.

Gemini:
- Interactions API is the recommended unified API for new applications.
- google_search is a server-side built-in tool.
- grounded interactions expose google_search_call steps, generated queries, final text, and URL citations.

References:
- https://developers.openai.com/api/docs/guides/tools
- https://developers.openai.com/api/docs/guides/tools-web-search
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/server-tools
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool
- https://ai.google.dev/gemini-api/docs/interactions-overview
- https://ai.google.dev/gemini-api/docs/google-search
- https://ai.google.dev/gemini-api/docs/tool-combination

## Architectural acceptance test

~~~text
Catalog
  ^
  | ToolSource
  |
Model Tool Bridge Instance
  |
  +-- normalized capability snapshot
  +-- engine adapter
  +-- fixed provider origin
  |
  | ToolExecutor
  v
Invocation
  |
  +-- Profile Credential scope
  +-- Policy
  +-- Audit
  +-- Result Guard
~~~

Generic Runtime changes only to register the plugin.

No engine/vendor switch belongs in Catalog, Invocation, Profile, Policy, Audit, or Provider Instance Manager.

## Scope

Phase 15 includes:

- strict engine/model/Credential/capability config;
- OpenAI, Anthropic, and Gemini engine adapters;
- normalized web_search Tool schema;
- normalized answer/source/query result schema;
- actual hosted-tool-use verification;
- bounded provider HTTP requests/responses;
- bounded Anthropic pause_turn continuation;
- Credential resolution only during execution;
- Profile Credential scope;
- Runtime end-to-end tests;
- CLI creation;
- generic Management/Wails configuration;
- Web/Desktop first-party form;
- cross-engine normalization/security tests.

## Non-goals

Phase 15 does not include:

- a model chat Provider;
- a model acting as an agent over Mender's Catalog;
- file search/vector-store lifecycle;
- URL Context/web fetch;
- code execution or shell;
- computer use;
- image generation;
- Google Maps;
- tool search;
- provider MCP/connectors;
- skills;
- deep research/background jobs;
- binary/file artifacts;
- custom API base URLs;
- token/cost fields in Audit/Outcome;
- reasoning/thought exposure.

## Package layout

Preferred layout:

~~~text
backend/internal/modeltools/infrastructure/plugin/
  config.go
  result.go
  engine.go
  http.go
  prompt.go
  openai.go
  anthropic.go
  gemini.go
  plugin.go
  *_test.go
~~~

## Configuration v1

~~~json
{
  "engine": "openai",
  "model": "gpt-5.5",
  "credentialRef": "credential://openai/work",
  "capabilities": ["web_search"]
}
~~~

Rules:

- engine is openai, anthropic, or gemini;
- model is trimmed, non-empty, and bounded;
- credentialRef is canonical;
- capabilities are explicit and unique;
- v1 supports only web_search;
- at least one capability is required;
- unknown JSON fields fail;
- resolved API keys are never persisted;
- API endpoint override is not user-configurable.

InspectConfig returns the model API Credential Reference without network access.

## Provider open semantics

Plugin.Open:

1. strict-decodes config;
2. selects internal engine adapter;
3. creates static Tool snapshot;
4. creates bounded HTTP client;
5. resolves no Credential;
6. contacts no model API.

Provider sync must therefore spend no model tokens/tool calls.

## web_search Tool contract

Stable identity:

~~~text
SourceKey = web_search
Name      = web_search
~~~

Input:

~~~json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["query"],
  "properties": {
    "query": {
      "type": "string",
      "minLength": 1,
      "maxLength": 8192
    }
  }
}
~~~

Output:

~~~json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["answer", "sources", "queries"],
  "properties": {
    "answer": { "type": "string" },
    "sources": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["url", "title"],
        "properties": {
          "url": { "type": "string" },
          "title": { "type": "string" }
        }
      }
    },
    "queries": {
      "type": "array",
      "items": { "type": "string" }
    }
  }
}
~~~

Caller query is the Mender Tool input. Provider-generated search queries are output metadata.

## Normalized result

Internal result:

~~~go
type SearchResult struct {
    Answer  string
    Sources []Source
    Queries []string
}
~~~

Normalization must:

- require/bound final answer;
- bound source/query counts and fields;
- validate source URLs as HTTP(S);
- de-duplicate sources by URL;
- de-duplicate queries;
- produce deterministic canonical JSON;
- exclude reasoning, thought blocks, encrypted state, widget HTML, raw provider JSON, IDs, and usage metadata.

## Prompt boundary

Adapter-owned instruction:

~~~text
You are executing Mender's web_search capability.
You must use the enabled hosted web-search tool.
Do not answer from model memory alone.
Return a grounded final answer.
~~~

Caller query is sent as separate user/input content, never concatenated into higher-authority instructions.

Each invocation is stateless in v1.

## Hosted-tool verification

Success requires:

1. provider request completed;
2. at least one engine-specific hosted-search marker exists;
3. final answer exists;
4. normalized result passes bounds.

No hosted search marker becomes canonical Tool failure.

### OpenAI verification

Require at least one web_search_call output item.

Collect final text/citations from message output and query metadata from search action items.

### Anthropic verification

Require at least one server_tool_use block named web_search.

Collect generated query, search results/citations, and final text.

If stop_reason is pause_turn, continue with the returned assistant content up to a fixed cap of 3 continuations.

Encrypted search content may exist transiently only for continuation; it never enters canonical result/Audit.

### Gemini verification

Require at least one google_search_call step.

Collect generated queries, final model output text, and url_citation annotations.

Ignore thought steps, signatures, usage, and search-suggestions HTML.

## Shared HTTP policy

All adapters use:

- fixed official HTTPS origins;
- no automatic redirect;
- no ambient proxy;
- Invocation context cancellation/deadline;
- bounded provider response bytes;
- bounded diagnostics;
- no API key in errors.

Official origins:

~~~text
OpenAI    https://api.openai.com
Anthropic https://api.anthropic.com
Gemini    https://generativelanguage.googleapis.com
~~~

Test endpoints are injected package-internally and are not persisted.

## Credential execution

At each call:

~~~text
CredentialResolver.Resolve(config.CredentialRef)
  -> provider request
  -> zero returned byte slice where practical
~~~

Instances do not cache resolved API keys.

This makes existing Profile Credential scope authoritative.

## OpenAI engine

Endpoint:

~~~text
POST https://api.openai.com/v1/responses
Authorization: Bearer <secret>
~~~

Request includes configured model, one web_search tool, required tool use, bounded bridge instruction, and user query.

Ignore reasoning/internal output items.

## Anthropic engine

Endpoint:

~~~text
POST https://api.anthropic.com/v1/messages
x-api-key: <secret>
anthropic-version: <reviewed value>
~~~

Use one direct web-search server tool. Initial reviewed tool type:

~~~text
web_search_20260318
~~~

No unrelated client tools are supplied.

Web-search result errors inside HTTP 200 become Tool failures.

## Gemini engine

Endpoint:

~~~text
POST https://generativelanguage.googleapis.com/v1beta/interactions
x-goog-api-key: <secret>
~~~

Request includes configured model, bounded bridge input, and one google_search tool.

## Failure mapping

Provider adapter error:
- network error;
- non-2xx provider response;
- auth failure;
- invalid JSON/response shape;
- incompatible model/tool configuration;
- response too large;
- Anthropic continuation exhausted.

Canonical Tool failure:
- provider response succeeded but hosted search did not execute;
- provider reports hosted search tool error;
- final grounded answer is missing.

## Runtime integration

Only generic bootstrap registration is added:

~~~go
providerPlugins.Register(modeltools.New())
~~~

Normal path remains:

~~~text
RegisterConfiguredProvider
-> provider_configs
-> SyncConfiguredProvider
-> Discover web_search
-> CallConfiguredTool
-> Invocation governance
-> restricted CredentialResolver
-> engine adapter
-> hosted search
-> normalized JSON Outcome
~~~

## Runtime tests

Cover:

- descriptor registration;
- generic register/sync/search/describe/call;
- sync makes no provider API request;
- stable ToolID across model/config refresh;
- allowed Profile with API Credential succeeds;
- denied Profile fails credential_outside_scope before provider network access;
- Credential deletion detects Provider usage;
- Audit excludes API key/query/result/raw provider response beyond existing terminal metadata.

## CLI

Add:

~~~text
mender provider add model-tools
~~~

Flags:

~~~text
--namespace
--name
--description
--engine openai|anthropic|gemini
--model <model-id>
--credential credential://namespace/name
--capability web_search
~~~

## Management / Web / Desktop

No engine-specific management endpoint.

Generic config object remains sufficient.

Web/Desktop first-party form:

1. Engine: OpenAI / Anthropic / Gemini.
2. Model identifier.
3. Capability: Web search.
4. Separate Authentication Dialog with one inventory Credential selector.

Normal UI never asks for API key bytes or raw Credential Reference text.

Provider detail shows engine/model/capabilities only.

## Hardening tests

Cover:

- API key absent from errors/results;
- Profile denial prevents network call;
- redirect refused;
- ambient proxy disabled;
- raw reasoning/thought ignored;
- missing hosted-tool marker for each engine -> Tool failure;
- provider response cap;
- answer/source/query count/byte caps;
- malformed/duplicate citations normalization;
- query remains user content;
- Anthropic continuation cap and transient encrypted content;
- byte-equivalent normalized fixtures across engines when semantic inputs match.

## Work packages

1. docs(architecture): define model tool bridge provider
2. feat(model-tools): add bridge provider configuration
3. feat(model-tools): add bounded engine transport
4. feat(model-tools): bridge openai web search
5. feat(model-tools): bridge anthropic web search
6. feat(model-tools): bridge gemini web search
7. test(runtime): cover model tool bridge end to end
8. feat(adapters): expose model tool bridge providers
9. feat(app): manage model tool bridge providers
10. test(model-tools): harden hosted tool bridge
11. docs: complete model tool bridge phase

## Exit criteria

- [x] io.mender.provider.model-tools is registered.
- [x] config strictly supports OpenAI/Anthropic/Gemini.
- [x] model API Credential is an inventory reference.
- [x] Provider sync resolves no Credential and makes no provider API request.
- [x] web_search is the only v1 capability.
- [x] canonical input/output schemas are stable.
- [x] normalized result omits reasoning/raw provider state.
- [x] OpenAI requires observable web_search_call.
- [x] Anthropic requires observable server web_search call.
- [x] Anthropic pause_turn continuation is bounded.
- [x] Gemini requires observable google_search_call.
- [x] all three normalize answer/sources/queries.
- [x] fixed official origins are used in persisted-config path.
- [x] redirects are refused and ambient proxy is disabled.
- [x] response/error/result bounds are covered.
- [x] API keys never enter Tool schema/result/Audit/error.
- [x] Profile Credential scope blocks network access before provider execution.
- [x] Credential deletion recognizes Model Tool Bridge usage.
- [x] stable SourceKey preserves ToolID across config refresh.
- [x] generic Runtime has no engine/vendor lifecycle branch.
- [x] CLI registers all three engine variants.
- [x] generic Management API/SDK/Wails need no engine-specific endpoint.
- [x] Web/Desktop manage engine/model/capability/authentication.
- [x] cross-engine normalized fixtures are covered.
- [x] Go lint/test/race pass.
- [x] frontend lint/typecheck/tests/build pass.
- [x] security scans pass.

## Closeout

Phase 15 completed on 2026-09-27.

The Model Tool Bridge now validates a third Provider execution class: one Mender Tool invocation can delegate to a provider-side model/hosted-tool loop without making that provider model a general agent over Mender's Catalog.

Implemented boundary and behavior:

- `io.mender.provider.model-tools` is registered beside MCP and OpenAPI. Generic Runtime only registers the plugin; OpenAI/Anthropic/Gemini switching remains inside `backend/internal/modeltools/infrastructure/plugin`.
- Config v1 stores only engine, model identifier, one model API Credential Reference, and reviewed capabilities. V1 exposes only `web_search`. Provider open/sync is local: it selects an engine adapter and emits the static Tool snapshot without resolving the Credential or contacting a model API.
- The canonical Tool identity is `web_search`. Caller input is only a bounded `query`; provider-generated search queries are normalized output metadata rather than caller-controlled hosted-tool arguments.
- Canonical output is `{answer, sources, queries}` with deterministic de-duplication/sorting and byte/count bounds. Raw provider JSON, IDs, usage metadata, reasoning/thought blocks, Anthropic encrypted search state, Gemini thought signatures, and search-widget HTML are never emitted as Tool content.
- The shared engine HTTP boundary uses fixed official production origins in the normal configuration path, disables redirect following and ambient proxy routing, honors invocation context cancellation/timeouts, caps raw responses at 4 MiB, and surfaces non-2xx failures without copying provider response bodies into errors.
- OpenAI Responses uses hosted `web_search`, requires tool use, verifies an observable `web_search_call`, and normalizes URL citations plus visible search-action query metadata.
- Anthropic Messages uses the reviewed direct web-search server tool, verifies `server_tool_use` named `web_search`, maps explicit server-tool result errors to Tool failure, and handles `pause_turn` through a bounded continuation loop. Provider-returned encrypted search content is retained only transiently when required for continuation and is not placed in canonical output.
- Gemini Interactions uses `google_search`, verifies a `google_search_call`, runs statelessly with `store=false`, and normalizes final model text plus URL citations/query metadata while ignoring thought/signature/widget state.
- Cross-engine fixtures representing the same semantic search result produce byte-identical canonical JSON after normalization. Missing hosted-search markers are rejected as Tool failures rather than accepted as model-memory answers.
- Model API Credentials are resolved only inside `Instance.Execute` through the Invocation-provided CredentialResolver. The resolved byte slice is zeroed after use. Runtime coverage proves Provider sync succeeds even when the configured environment Credential has no value, Credential deletion recognizes Provider usage, model changes preserve the stable `web_search` ToolID, and a Profile without the Credential receives `credential_outside_scope` before provider network execution.
- CLI supports `provider add model-tools` for OpenAI, Anthropic, and Gemini. Generic Management API/SDK/Wails configuration requires no vendor-specific endpoint. Shared Web/Desktop UI exposes engine/model/Web Search configuration and a separate inventory-backed Model API Credential selector.
- Provider authentication-flow tests now wait for the actual reopened configuration Dialog fields rather than matching the page-level `Add provider` label, removing a pre-existing asynchronous test race that became visible under the expanded Provider test suite.

Validation evidence:

~~~text
pnpm sdk:check                         PASS
pnpm desktop:bindings:check            PASS
pnpm desktop:architecture:check        PASS
pnpm go:lint                           PASS
pnpm go:test                           PASS
pnpm go:test:race                      PASS
pnpm lint                              PASS
pnpm typecheck                         PASS
pnpm test                              PASS (5 files / 35 tests)
pnpm build                             PASS (Web + Desktop; existing chunk-size warnings only)
backend govulncheck                    PASS (No vulnerabilities found)
desktop govulncheck                    PASS (No vulnerabilities found)
pnpm security:js                       PASS (No known production dependency vulnerabilities)
~~~

The aggregate `pnpm security:go` invocation exceeded the local bridge's synchronous call window. Its backend and desktop govulncheck constituents were run separately; the first desktop attempt hit a transient `vuln.go.dev` network timeout and the immediate retry completed successfully with no vulnerabilities found.

Phase 15 adds no SQLite migration. Model Tools configuration uses generic `provider_configs` v1. A binary that predates `io.mender.provider.model-tools` can still understand the database schema but fails closed when asked to activate/synchronize that unknown plugin.

The bridge intentionally stops at normalized `web_search`. File search, fetch/URL context, code execution, deep research/background tasks, computer use, image generation, and usage/cost observability require separate result/lifecycle/security review rather than inheriting web-search semantics.

## Deferred follow-up

- normalized fetch capability using Anthropic web fetch / Gemini URL Context;
- file search with reviewed corpus identity/lifecycle;
- code execution with artifact modeling;
- deep-research/background tasks;
- computer use;
- image generation;
- usage/cost observability;
- custom provider-compatible endpoints;
- richer citation rendering.
