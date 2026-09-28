# ADR-0026: Bridge provider-hosted model tools through a normalized agentic Provider

- Status: Accepted
- Date: 2026-09-27

## Context

Phase 13 established Provider Plugin Runtime as Mender's upstream extension boundary, and Phase 14 validated it with a non-MCP OpenAPI Provider.

The next planned Provider class is materially different from both MCP and OpenAPI: tools that execute inside an LLM provider's own model runtime.

Current provider APIs expose "built-in", "server", "hosted", or "grounding" tools that the model can invoke during one model request:

- OpenAI Responses API built-in tools include web search, file search, code interpreter, shell, computer use, image generation, remote MCP/connectors, skills, and related hosted capabilities.
- Anthropic distinguishes client tools from server tools. Current server tools include web search, web fetch, code execution, and tool search. Server tools execute on Anthropic infrastructure and participate in a server-side agentic loop that can return `pause_turn`.
- Gemini distinguishes server-side built-in tools such as Google Search, URL Context, File Search, Google Maps, and Code Execution from client-side Computer Use/function calling. The recommended Interactions API exposes tool call/result steps and citations.

These are not ordinary function-call APIs.

For example, an OpenAI web search request enables `web_search` in a Responses API request and the model executes one or more search actions before producing a cited answer. Anthropic web search appears as `server_tool_use` plus `web_search_tool_result` blocks inside the Messages API agentic loop. Gemini Google Search appears as `google_search_call` / result steps and a grounded model output.

Therefore this is not equivalent to:

~~~text
Tool arguments
  -> HTTP endpoint for tool
  -> Tool result
~~~

It is closer to:

~~~text
Mender Tool invocation
  -> model request with one hosted capability enabled
  -> provider-side model/tool loop
  -> hosted tool call/result steps
  -> final model answer with citations
  -> normalized Mender Tool result
~~~

Mender must not confuse this role with an agent choosing arbitrary Mender Tools. The model inside this Provider is an implementation detail of one governed Tool invocation.

The bridge must preserve Mender's existing boundaries:

- Catalog remains canonical/protocol-independent.
- Invocation still owns Policy, Profile scope, Credential scope, execution/result guards, and Audit.
- Provider credentials are inventory-backed references and resolved only at execution.
- model/provider reasoning traces must not leak into Audit or canonical results.
- the bridge must verify that a requested hosted capability actually executed rather than accepting an ungrounded model-only answer.

## Decision

Mender will add one built-in **Model Tool Bridge Provider**:

~~~text
io.mender.provider.model-tools
~~~

A configured Provider chooses one model API engine and exposes a bounded set of Mender-defined bridge capabilities.

Initial engines:

~~~text
openai
anthropic
gemini
~~~

### Initial capability

Phase 15 v1 exposes only:

~~~text
web_search
~~~

This is the common, externally observable hosted capability across the three engines:

- OpenAI: Responses API `web_search`.
- Anthropic: Messages API `web_search_20260318` with name `web_search`.
- Gemini: Interactions API `google_search`.

Other provider-hosted capabilities are deliberately deferred until their result/lifecycle semantics are reviewed independently:

- file search / retrieval stores;
- URL context / web fetch;
- code execution;
- shell;
- computer use;
- image generation;
- Google Maps;
- provider MCP/connectors;
- tool search;
- skills;
- long-running/deep-research agents.

### Why one plugin instead of one plugin per vendor

OpenAI, Anthropic, and Gemini differ at their wire protocols, but the Mender integration class is the same:

~~~text
normalized bridge capability
  -> one model API engine
  -> provider-hosted agentic tool execution
  -> normalized result
~~~

Engine differences belong behind an internal adapter boundary.

Creating `io.mender.provider.openai-tools`, `...anthropic-tools`, and `...gemini-tools` would make engine choice a top-level plugin identity and repeat configuration/lifecycle/UX code.

The plugin owns a discriminated engine config instead.

### Configuration v1

Conceptual persisted configuration:

~~~json
{
  "engine": "openai",
  "model": "gpt-5.5",
  "credentialRef": "credential://openai/work",
  "capabilities": ["web_search"]
}
~~~

Anthropic example:

~~~json
{
  "engine": "anthropic",
  "model": "claude-opus-5-5",
  "credentialRef": "credential://anthropic/work",
  "capabilities": ["web_search"]
}
~~~

Gemini example:

~~~json
{
  "engine": "gemini",
  "model": "gemini-3.8-flash",
  "credentialRef": "credential://gemini/work",
  "capabilities": ["web_search"]
}
~~~

Rules:

- engine is one reviewed engine value;
- model is non-empty, bounded metadata;
- credentialRef is a canonical Mender Credential Reference;
- capabilities are explicit, unique, and reviewed;
- v1 requires `web_search` when the list is non-empty and rejects unknown capabilities;
- resolved API keys are never persisted;
- provider API endpoints are fixed by the adapter in v1 and are not user-configurable.

Custom/OpenAI-compatible endpoints are deferred because endpoint overrides change credential-origin/trust semantics and may not implement hosted tool behavior faithfully.

### Provider opening

Opening the Provider is local/configuration-only.

~~~text
Plugin.Open
  -> strict config decode
  -> build engine adapter
  -> build static Tool snapshot
  -> no model API call
  -> no Credential resolution
~~~

This is intentional.

Provider synchronization must not:

- spend model tokens;
- run web searches;
- resolve API keys;
- require a Profile to authorize execution credentials.

Model API credentials are resolved only during `ToolExecutor.Execute`.

### Discovery

The initial Provider exposes one canonical Tool when `web_search` is enabled.

Stable SourceKey:

~~~text
web_search
~~~

Stable Tool name:

~~~text
web_search
~~~

Title/description identify the configured engine/model without making those values part of source identity.

Changing model or engine under one Provider configuration changes implementation semantics but does not change the canonical web-search capability identity. PluginID remains immutable; engine/model are plugin configuration.

A future migration may decide that changing engine requires a new Provider identity if operational experience shows provenance needs stricter identity. Phase 15 records engine/model in Provider configuration and Audit-safe Provider metadata, while Tool SourceKey remains capability-oriented.

### Canonical input schema

The v1 bridge Tool accepts only:

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

The bridge does not expose provider-internal hosted-tool schemas as Mender Tool inputs.

This distinction is critical:

~~~text
Mender Tool input
  = task/request supplied by Mender caller

Provider hosted-tool input
  = internal search queries generated by provider model
~~~

Provider-generated search queries are output metadata, not caller-controlled raw tool arguments.

### Canonical output schema

A successful web-search result normalizes to JSON:

~~~json
{
  "answer": "final grounded answer text",
  "sources": [
    {
      "url": "https://example.com/article",
      "title": "Article"
    }
  ],
  "queries": [
    "provider-generated search query"
  ]
}
~~~

Schema rules:

- `answer` is required, non-empty, bounded UTF-8 text;
- `sources` is required and may be empty when a provider reports no cited source;
- source URL/title are bounded;
- sources are de-duplicated deterministically;
- `queries` is required and may be empty when the provider does not expose generated queries;
- no raw provider response is returned;
- no reasoning/thought content is returned;
- no provider encrypted/internal state is returned.

This fits the existing canonical JSON Outcome without adding a new Domain result kind.

### Agentic execution verification

A bridge invocation must not accept a model-only response that skipped the requested hosted tool.

For each engine the adapter verifies an observable hosted-tool call marker.

#### OpenAI

Use Responses API with only the hosted `web_search` tool enabled and require tool use through `tool_choice`.

The adapter verifies at least one `web_search_call` output item.

OpenAI Responses web search returns:

- `web_search_call` output items with search/open/find actions;
- a final message output with text and URL-citation annotations.

The adapter normalizes final text, cited source URLs/titles, and exposed search queries.

#### Anthropic

Use Messages API with one current direct web-search server tool definition.

Current models/settings do not uniformly support forced `tool_choice: tool/any`. Therefore the bridge does not depend on forced tool choice for Anthropic v1.

Instead:

- provide only the web-search server tool;
- provide a strong system/user instruction that this canonical Tool must execute web search and must not answer from memory;
- set the tool's caller mode to direct where appropriate to avoid implicitly provisioning code execution for dynamic filtering;
- verify at least one `server_tool_use` block named `web_search`;
- verify/normalize final text and citations;
- treat "no hosted tool call" as a Tool failure.

If the response ends with `pause_turn`, the adapter performs bounded continuation using the provider-documented continuation pattern. The maximum continuation count is fixed and bounded.

The adapter does not expose encrypted search-result content or thinking blocks.

#### Gemini

Use the recommended Interactions API with only `google_search` enabled.

The request explicitly instructs the model to search rather than answer from memory.

The adapter verifies at least one `google_search_call` step.

It normalizes:

- final model output text;
- URL-citation annotations;
- generated search queries from visible search-call arguments.

Thought steps/signatures and search-widget HTML are not returned as canonical Tool content.

### Prompt boundary

The bridge necessarily sends model input because the provider's model decides how to operate its hosted tool.

The prompt is adapter-owned and bounded.

Conceptually:

~~~text
system/developer instruction:
  You are executing Mender's web_search capability.
  You MUST use the enabled hosted search tool.
  Do not answer from model memory alone.
  Produce a concise answer grounded in tool results.

user:
  <query>
~~~

Caller input is inserted as user/task content, never concatenated into a higher-authority instruction string.

The bridge does not pass Mender Policy text, Credential values, Audit metadata, or unrelated conversation history to the model provider.

Each Tool invocation is stateless in Phase 15.

### Credentials

One model API Credential Reference is configured per Provider.

`InspectConfig` returns that reference.

Execution:

~~~text
Profile-scoped CredentialResolver
  -> resolve configured API credential
  -> build one provider API request
  -> zero resolved byte slice where practical
~~~

Therefore existing Profile ToolBinding Credential scope applies without new Invocation logic.

If the Profile lacks the Provider API Credential Reference, the model API request must not be sent.

### Engine adapter boundary

The Model Tool Bridge plugin owns an internal interface conceptually like:

~~~go
type Engine interface {
    Search(context.Context, SearchRequest, CredentialResolver) (SearchResult, error)
}
~~~

or a narrower credential-bytes boundary internal to the plugin.

Engine adapters own:

- official endpoint;
- authentication header;
- request shape;
- hosted-tool configuration;
- hosted-tool-use verification;
- provider response parsing;
- continuation behavior;
- citation/query extraction;
- provider error normalization.

Generic Provider Runtime does not switch on engine.

### Network policy

Phase 15 uses fixed official API origins:

~~~text
OpenAI    https://api.openai.com
Anthropic https://api.anthropic.com
Gemini    https://generativelanguage.googleapis.com
~~~

Adapters:

- use HTTPS only;
- do not accept userinfo;
- do not automatically follow redirects;
- do not inherit ambient proxy routing/credentials;
- use bounded request/response sizes;
- use Invocation context for cancellation/timeouts;
- never include API key values in errors.

Tests replace endpoints through package-private adapter construction, not persisted configuration.

### OpenAI adapter

V1 uses:

~~~text
POST https://api.openai.com/v1/responses
Authorization: Bearer <Credential>
Content-Type: application/json
~~~

Request shape includes:

- configured model;
- one `web_search` tool;
- required tool use;
- bounded input instruction/query;
- source/citation metadata requested where the API supports it;
- no persistent response state required by Mender.

The adapter ignores reasoning items and returns only normalized answer/citation/query metadata.

### Anthropic adapter

V1 uses:

~~~text
POST https://api.anthropic.com/v1/messages
x-api-key: <Credential>
anthropic-version: <reviewed version>
Content-Type: application/json
~~~

Request shape includes:

- configured model;
- bounded max tokens;
- one direct web-search server tool;
- bounded system/user instructions;
- no unrelated client tools.

A web-search tool error represented inside a successful HTTP response is mapped to canonical Tool failure.

### Gemini adapter

V1 uses the recommended Interactions API:

~~~text
POST https://generativelanguage.googleapis.com/v1beta/interactions
x-goog-api-key: <Credential>
Content-Type: application/json
~~~

Request shape includes:

- configured model;
- one `google_search` tool;
- bounded input instruction/query.

The adapter requires a visible `google_search_call` step and normalizes final text annotations.

### Provider errors versus Tool errors

Wire/API/auth/configuration failures are Provider failures through the existing executor error mapping.

Examples:

- HTTP 401/403 from model API;
- incompatible model/tool configuration;
- invalid provider response shape;
- response too large;
- bounded continuation exhausted.

Expected hosted-tool execution failures become canonical Tool failures when the provider returns a successful model API response that explicitly reports a search-tool failure.

A response that completes without the required hosted search call is a Tool failure:

~~~text
hosted web search was not executed
~~~

because the model request ran, but did not fulfill the canonical Tool contract.

### Usage/cost metadata

Provider-hosted search incurs provider model/tool usage and billing.

Phase 15 does not add token/search-cost fields to canonical Outcome or Audit.

Adapters may parse usage internally for tests/diagnostics, but normal Tool results remain data-minimized.

A later observability ADR may add bounded provider usage metadata if there is a cross-provider design.

### Citations

The canonical result preserves URL/title citations because providers require or strongly expect source attribution for hosted web search output.

Mender does not strip citations and then present the final answer as uncited provider-generated text.

Web/Desktop Tool invocation can render the canonical JSON today; richer citation rendering can be added later without changing the Tool result schema.

### No chain-of-thought persistence

Provider responses may contain reasoning/thought/internal step objects.

Mender does not:

- expose them in Tool output;
- persist them in Audit;
- include them in Provider errors;
- use them as Governance Facts.

Only provider-visible final answer text, search-call query metadata, and citations are normalized.

### Management UX

Provider creation remains plugin-first.

Selecting `Model Tools` shows:

1. Engine
   - OpenAI
   - Anthropic
   - Gemini
2. Model
   - free-form bounded model identifier in v1
3. Capabilities
   - Web search enabled
4. Authentication
   - inventory Credential selector

Normal users never enter API key bytes or raw Credential Reference text.

The Provider details view shows engine/model/capability names but not secret material.

### CLI

Conceptual creation:

~~~text
mender provider add model-tools   --namespace research   --name "Research Search"   --engine openai   --model gpt-5.5   --credential credential://openai/work   --capability web_search
~~~

The CLI may accept the stable Credential Reference because it is the advanced administrative surface, consistent with existing OpenAPI `--credential` behavior.

### Security invariants

1. The model API Credential is resolved only during governed execution.
2. Provider synchronization performs no billable model request and resolves no secret.
3. Profile Credential scope blocks execution before model API network access.
4. Fixed official origins prevent arbitrary credential exfiltration through config.
5. Redirects are not automatically followed.
6. Ambient HTTP proxy configuration is not inherited.
7. Input query is user content, never higher-authority prompt text.
8. The bridge verifies actual hosted search execution.
9. Raw provider responses, reasoning, thought signatures, encrypted search state, and widget HTML are not canonical Tool output or Audit.
10. API errors are bounded and must not echo credential values.
11. Response bytes, text, citations, source count, and continuation count are bounded.
12. All normal Invocation Policy/Profile/Audit/result-guard behavior remains unchanged.

## Alternatives considered

### Expose vendor hosted-tool schemas directly as canonical Tool schemas

Rejected.

Those schemas are model/provider-internal execution protocols. The external caller does not directly call `web_search_call` or Anthropic `server_tool_use`; the provider model generates those calls.

### Treat the model itself as Mender's top-level agent

Rejected.

Mender is executing one governed Tool. The provider model must not gain access to the rest of Mender's catalog or decide arbitrary Mender Tool calls.

### Add one Provider plugin per model vendor

Rejected for v1.

The integration class and Mender semantics are shared; vendor differences fit behind an engine adapter. A future split remains possible if lifecycle/trust/provenance diverge materially.

### Implement all provider built-in tools immediately

Rejected.

Code execution, file search, computer use, image generation, Maps, shell, deep research, and similar capabilities introduce artifacts, sessions, files, side effects, interactive actions, or asynchronous execution that deserve separate canonical result/security review.

### Return raw provider JSON as Tool output

Rejected.

It would leak provider-specific reasoning/internal metadata into Mender's canonical surface and make downstream agents depend on unstable vendor wire formats.

### Allow custom API base URLs

Rejected in Phase 15.

Hosted-tool compatibility and credential-origin guarantees would become ambiguous. Provider-compatible gateways can be reviewed later as a separate trust/configuration feature.

## Consequences

### Positive

- validates a third Provider execution class: provider-side agentic hosted tools;
- reuses Provider Plugin Runtime without changing Invocation Domain;
- preserves Profile Credential governance for model API keys;
- gives agents one stable web-search Tool schema independent of model vendor;
- keeps vendor wire formats and tool versions inside infrastructure;
- preserves source attribution;
- avoids exposing provider chain-of-thought/internal state;
- provides a clean foundation for future hosted tool capabilities.

### Negative / trade-offs

- one Mender Tool invocation includes a model generation step and can be slower/costlier than a direct API Tool;
- exact answer wording is model-generated and non-deterministic;
- Anthropic/Gemini v1 must verify hosted-tool use after the request rather than relying on universally supported forced tool choice;
- supported model IDs/tool compatibility still depend on provider service behavior;
- v1 supports only web search;
- citations are normalized, so provider-specific rich attribution metadata is intentionally discarded;
- token/tool billing metadata is not exposed yet;
- provider API changes require bridge adapter maintenance.

## Current external references

Reviewed on 2026-09-27:

- OpenAI tool overview:
  https://developers.openai.com/api/docs/guides/tools
- OpenAI Responses web search:
  https://developers.openai.com/api/docs/guides/tools-web-search
- Anthropic tool overview:
  https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview
- Anthropic server tools:
  https://platform.claude.com/docs/en/agents-and-tools/tool-use/server-tools
- Anthropic web search:
  https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool
- Anthropic tool reference:
  https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-reference
- Gemini Interactions API overview:
  https://ai.google.dev/gemini-api/docs/interactions-overview
- Gemini Google Search:
  https://ai.google.dev/gemini-api/docs/google-search
- Gemini built-in tool combination:
  https://ai.google.dev/gemini-api/docs/tool-combination

## Follow-up

Phase 15 implements this decision in segmented commits:

1. architecture/phase plan;
2. plugin config/capability/result model;
3. shared bounded model-API HTTP client and engine interface;
4. OpenAI web-search adapter;
5. Anthropic web-search adapter with bounded continuation;
6. Gemini Google Search adapter;
7. Provider Plugin/Instance and Runtime end-to-end governance;
8. CLI/API/Wails/Web/Desktop management;
9. cross-engine hardening/normalization/security tests;
10. closeout documentation and verification.

Later phases can add reviewed hosted capabilities such as URL fetch/context, file search, code execution, or other provider tools without changing the `web_search` Tool contract.
