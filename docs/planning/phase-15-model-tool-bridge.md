# Phase 15 Plan — Model Tool Bridge Provider

Status: In progress

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

- [ ] io.mender.provider.model-tools is registered.
- [ ] config strictly supports OpenAI/Anthropic/Gemini.
- [ ] model API Credential is an inventory reference.
- [ ] Provider sync resolves no Credential and makes no provider API request.
- [ ] web_search is the only v1 capability.
- [ ] canonical input/output schemas are stable.
- [ ] normalized result omits reasoning/raw provider state.
- [ ] OpenAI requires observable web_search_call.
- [ ] Anthropic requires observable server web_search call.
- [ ] Anthropic pause_turn continuation is bounded.
- [ ] Gemini requires observable google_search_call.
- [ ] all three normalize answer/sources/queries.
- [ ] fixed official origins are used in persisted-config path.
- [ ] redirects are refused and ambient proxy is disabled.
- [ ] response/error/result bounds are covered.
- [ ] API keys never enter Tool schema/result/Audit/error.
- [ ] Profile Credential scope blocks network access before provider execution.
- [ ] Credential deletion recognizes Model Tool Bridge usage.
- [ ] stable SourceKey preserves ToolID across config refresh.
- [ ] generic Runtime has no engine/vendor lifecycle branch.
- [ ] CLI registers all three engine variants.
- [ ] generic Management API/SDK/Wails need no engine-specific endpoint.
- [ ] Web/Desktop manage engine/model/capability/authentication.
- [ ] cross-engine normalized fixtures are covered.
- [ ] Go lint/test/race pass.
- [ ] frontend lint/typecheck/tests/build pass.
- [ ] security scans pass.

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
