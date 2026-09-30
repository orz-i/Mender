# ADR-0030: Provide governed local workspace capabilities without becoming an Agent harness

- Status: Accepted
- Date: 2026-09-30

## Context

Mender is a local-first Tool/Resource runtime and gateway for LLMs and agents. Its product boundary deliberately excludes the model-facing Agent Loop: model calls, prompt/context construction, conversation state, planning, retry, compaction, memory, and stopping decisions belong to the consuming agent/harness.

Phase 19 made portable Agent Skills a managed Resource surface. The Tool side still lacks a first-party local capability set sufficient for an external harness to inspect and modify a workspace and run project commands. Today a coding or creative harness must usually bring its own filesystem/shell implementation or reach those capabilities through an unrelated upstream Provider.

The minimal high-value local capability set is:

~~~text
read
search
edit
shell
~~~

These capabilities are not inherently "coding" tools. The same workspace primitives can support code, writing, research artifacts, data preparation, or other file-oriented work. A Profile can expose different subsets and governance constraints without Mender deciding how an agent reasons or loops.

Contemporary agent runtimes reinforce two relevant patterns:

- OpenAI's sandbox guidance separates the harness/control plane from the execution/workspace plane, and its shell guidance treats arbitrary command execution as a capability requiring isolation/controls/audit rather than as a harmless file helper.
- OpenAI's apply-patch guidance recommends structured file mutations with path validation and explicit application results.
- Gemini CLI exposes filesystem operations under a configured root and separates Read/Edit/Search from Shell.
- Claude Code permissions distinguish read-only filesystem operations, edits, and Bash execution as different authority classes.

References:

- https://developers.openai.com/api/docs/guides/agents/sandboxes
- https://developers.openai.com/api/docs/guides/tools-shell
- https://developers.openai.com/api/docs/guides/tools-apply-patch
- https://google-gemini.github.io/gemini-cli/docs/tools/file-system.html
- https://docs.anthropic.com/en/docs/claude-code/cli-usage

## Decision

Mender will add a statically linked first-party Provider plugin:

~~~text
io.mender.provider.workspace
~~~

A configured Workspace Provider represents one explicit local workspace root and exposes four stable canonical Tools:

~~~text
workspace.read
workspace.search
workspace.edit
workspace.shell
~~~

The plugin is capability infrastructure for external harnesses. It does not add an Agent Loop to Mender.

### One Provider owns one workspace root

Workspace configuration has one required absolute root directory.

The root is persisted as non-secret Provider configuration and is part of that Provider instance's execution boundary. Tool arguments use workspace-relative paths; callers do not select arbitrary host roots per invocation.

The Provider validates the configured root before activation and reports a bounded connection summary suitable for management surfaces.

### Structured filesystem Tools are root-contained

`read`, `search`, and `edit` use structured Go filesystem operations rather than invoking a shell implementation.

Before operating on a path Mender must:

1. reject empty/absolute caller paths where a relative path is required;
2. clean and join the path against the configured workspace root;
3. reject lexical traversal outside the root;
4. evaluate existing path components without following a symlink outside the root;
5. reject symlink-backed target/resource escapes;
6. apply explicit size/result/depth/count bounds.

The workspace root itself is canonicalized during configuration/activation.

V1 intentionally does not grant structured file Tools access outside that root.

### `workspace.read`

`read` reads one regular text file inside the workspace.

V1 supports bounded text reads with line-oriented offset/limit controls and returns structured metadata including the normalized relative path and truncation state.

It rejects directories, special files, escaping symlinks, and oversized/unbounded reads.

### `workspace.search`

`search` provides bounded workspace-local discovery without relying on shell commands.

V1 supports two modes:

~~~text
files    -> find relative file paths matching a bounded glob/pattern
content  -> search bounded regular text files for a literal or regular-expression query
~~~

Search never follows directory/file symlinks outside the root, caps scanned files/bytes/results, and returns relative paths plus bounded match metadata.

A future optimized implementation may use a dedicated search library/index while preserving this Tool contract. V1 does not require `rg` or another host executable.

### `workspace.edit`

`edit` is one structured mutation Tool rather than an unrestricted write primitive.

V1 supports explicit operations on one workspace-relative file:

~~~text
create
replace
delete
~~~

`replace` contains one or more exact `oldText -> newText` replacements. Each replacement must match the expected occurrence count (default one). Missing, stale, or ambiguous content fails instead of guessing.

The complete mutation is validated before commit. Update/create uses a same-directory temporary file plus replace/rename where the platform permits so callers do not observe intentionally partial content.

Mender does not silently auto-correct a failed edit with a model call.

### `workspace.shell` is a high-authority escape hatch

`shell` accepts a command plus an optional workspace-relative working directory and a bounded timeout.

It executes through the host platform shell so normal command composition is possible. Mender owns:

- workspace-contained initial cwd validation;
- timeout/cancellation;
- bounded stdout/stderr capture;
- process cleanup;
- a reduced explicit environment rather than blindly forwarding arbitrary Mender secrets;
- canonical failure/result normalization and Audit through Invocation.

However, V1 host shell execution is **not an OS sandbox**. A shell command runs with the filesystem/process authority of the Mender host user and may itself change directory or access resources outside the configured workspace when the OS permits it.

The workspace root constrains the structured filesystem Tools and the initial shell cwd; it must not be described as a kernel/container security boundary.

A later execution-backend abstraction may add container/VM/OS sandbox implementations without changing the canonical four Tool identities.

### Profiles remain capability boundaries, not Agent definitions

A Profile may combine any subset of Workspace Tools with other Mender Tools.

Examples:

~~~text
review profile
  workspace.read
  workspace.search

writer profile
  workspace.read
  workspace.search
  workspace.edit
  web search

coding profile
  workspace.read
  workspace.search
  workspace.edit
  workspace.shell
  GitHub / MCP / OpenAPI tools
~~~

Profile ToolBindings may apply aliases/presentation, Credential scope, restrictive policy, and serialization exactly as they do for other canonical Tools.

Mender does not infer an Agent role from a Profile and does not construct prompts from it.

### Reviewed Governance Facts are routed without Provider activation

The current Invocation composition installs one global `UnknownExtractor`, so every Tool currently yields unknown Governance Facts even when a first-party structured Tool has arguments that can be mapped safely.

Phase 20 will add a host-owned routing extractor.

The routing path is:

~~~text
Invocation
  -> ProviderID from canonical Tool
  -> persisted Provider plugin configuration
  -> built-in plugin registration
  -> optional reviewed config-aware fact extractor
  -> bounded GovernanceFacts

unregistered / external / unsupported extractor
  -> UnknownGovernanceFacts
~~~

Fact extraction must not activate a Provider, launch an external plugin process, resolve Credentials, touch the network, or execute the Tool.

The generic Provider Plugin contract may expose an **optional Mender-hosted extractor capability** implemented by statically linked built-ins. External Provider code does not become trusted Policy input merely by returning facts over the plugin protocol.

Workspace facts use stable effect/resource vocabulary such as:

~~~text
workspace.read
  effect: filesystem.read
  resource: workspace_path:<normalized-relative-path>

workspace.search
  effect: filesystem.search
  resource: workspace_path:<normalized-relative-root>

workspace.edit
  effect: filesystem.write
  resource: workspace_path:<normalized-relative-path>

workspace.shell
  effect: process.execute
  resource: workspace_path:<normalized-relative-cwd>
~~~

The Policy request already contains ProviderID and ToolID, so a relative workspace resource remains unambiguous within a rule that also targets a Provider/Tool. Shell facts intentionally do not claim knowledge of every effect hidden inside an arbitrary command string.

### Generic management remains the default

Workspace is a built-in Provider plugin with declarative ConfigSchema/UISchema metadata.

Web/Desktop should use the existing generic Provider configuration renderer unless a concrete usability gap is demonstrated. The management HTTP API already accepts generic `pluginId + config` Provider creation.

CLI may add a small `provider add workspace` convenience command because the CLI currently exposes explicit first-party creation commands. That convenience must still call the generic Provider registration/synchronization application path.

No Workspace-specific management API is introduced.

## Security boundaries

Phase 20 must fail closed for structured filesystem paths.

At minimum:

- configured root is absolute, existing, and a directory;
- relative Tool paths cannot be empty where a file path is required;
- absolute paths and `..` escapes are rejected;
- symlink traversal/targets that escape the root are rejected;
- special files/devices/sockets are rejected;
- read/edit/search have bounded file and aggregate bytes;
- search has bounded visited files and result count;
- edit refuses ambiguous/stale replacements;
- shell initial cwd is root-contained;
- shell timeout and output are bounded;
- shell receives no arbitrary Credential resolution capability and does not inherit secret Provider configuration;
- all four Tools still execute only through the shared governed Invocation pipeline.

The shell host-authority limitation is explicit product behavior, not an implementation bug hidden by documentation.

## Consequences

### Positive

- External coding/creative/research harnesses can obtain a useful local capability surface entirely through Mender.
- Profiles become practical Agent capability boundaries without becoming Agent definitions.
- Structured read/search/edit calls become much more governable than encoding the same operations inside shell strings.
- Stable canonical Tool identities remain independent of any one model or agent SDK.
- Future sandbox backends can evolve behind the Workspace execution boundary.

### Costs

- Local filesystem and process execution materially increase host risk.
- Correct path/symlink containment is security-sensitive and requires dedicated tests.
- Search implemented in-process is initially less feature-rich/optimized than mature host tools.
- Shell cannot honestly provide fine-grained side-effect facts from arbitrary command text.
- Config-aware Governance Fact routing adds a new optional Provider plugin/runtime capability.

## Alternatives considered

### Make Mender a complete Agent harness

Rejected. Model calls, prompts, context, planning, loops, memory, retries, and compaction remain consumer responsibilities. Crossing this boundary would turn Mender into a competing orchestration framework instead of capability infrastructure.

### Provide only `shell`

Rejected. Shell can emulate filesystem operations but destroys structured intent, weakens Policy facts, increases prompt/tool ambiguity, and makes read-only/edit-specific Profiles difficult to express safely.

### Expose one Tool per coding-agent implementation

Rejected. The four primitives are domain-neutral workspace capabilities. Coding, writing, review, research, and operations behavior belongs to the external harness plus Profile composition.

### Treat workspace cwd as a sandbox

Rejected. A process working directory does not restrict OS authority. Phase 20 documents host authority explicitly and leaves true sandbox/container backends for later design.

### Trust external Provider plugins to emit Policy facts

Rejected. Policy consumes security-sensitive bounded metadata. Only Mender-owned reviewed mappings may turn raw arguments/configuration into known Governance Facts; unknown/external Tools remain unknown unless a future separately trusted extension boundary is designed.

## Follow-up

A future phase may add execution backends such as containers, VM sandboxes, macOS/Linux sandbox primitives, or remote workspaces. That work should preserve the canonical Workspace Tool contracts and make isolation an explicit backend/property rather than redefining Profile semantics.
