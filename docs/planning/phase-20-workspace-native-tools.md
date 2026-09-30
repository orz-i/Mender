# Phase 20 Plan — Workspace Native Tools Provider

Status: Complete

## Objective

Add a first-party local Workspace Provider that gives external agent harnesses a minimal governed file/process capability surface—`read`, `search`, `edit`, and `shell`—while Mender remains a Tool/Resource runtime rather than an Agent Loop.

Architecture is governed by [ADR-0030](../architecture/adr/0030-workspace-native-tools-provider.md).

## Product boundary

Phase 20 does **not** add:

- model API calls;
- system/developer/user prompt construction;
- conversation/session state for models;
- planner/reasoner loops;
- model routing;
- memory/compaction/retry;
- autonomous stop/continue decisions.

External harnesses continue to own those concerns.

Mender supplies governed capabilities and resources:

~~~text
External Agent / Harness
  ├─ model
  ├─ prompt/context
  ├─ agent loop
  └─ tool selection
          |
          v
      Mender Profile
          |
          +-- Workspace Tools
          +-- MCP/OpenAPI/Model/External Tools
          +-- Skills/resources
          |
          v
 Profile -> Policy -> Invocation -> Audit
~~~

## External baseline

Phase 20 follows established execution-boundary patterns rather than inventing a coding-specific Agent runtime:

- OpenAI Sandbox Agents explicitly separate harness/control-plane responsibilities from workspace execution.
- OpenAI Shell treats arbitrary shell commands as dangerous execution that should be isolated/controlled/audited.
- OpenAI Apply Patch uses structured file operations and calls out path validation and atomicity/error handling.
- Gemini CLI separates Read/Edit/Search/Glob from Shell and roots filesystem tools inside a configured directory.
- Claude Code exposes read/edit/Bash as separate permission classes.

References:

- https://developers.openai.com/api/docs/guides/agents/sandboxes
- https://developers.openai.com/api/docs/guides/tools-shell
- https://developers.openai.com/api/docs/guides/tools-apply-patch
- https://google-gemini.github.io/gemini-cli/docs/tools/file-system.html
- https://docs.anthropic.com/en/docs/claude-code/cli-usage

## Domain / plugin language

### Workspace Provider

One configured local workspace root exposed through the built-in plugin:

~~~text
io.mender.provider.workspace
~~~

Provider identity remains the canonical Mender Provider identity. Root path is plugin-owned non-secret configuration.

### Workspace Tool

One canonical Tool discovered from the Workspace Provider.

Stable SourceKeys:

~~~text
workspace.read
workspace.search
workspace.edit
workspace.shell
~~~

ToolIDs remain Mender-generated stable identities preserved across synchronization while SourceKeys remain stable.

### Workspace path

A Tool argument path resolved relative to the Provider root.

Structured filesystem Tools never accept a caller-selected host root.

### Host shell

A command executed by the Mender process user through the platform shell with a validated initial workspace cwd, timeout/cancellation, bounded output, and reduced environment.

It is not an OS sandbox.

## Provider configuration

Initial configuration:

~~~json
{
  "root": "/absolute/path/to/workspace"
}
~~~

V1 intentionally keeps security bounds host-owned constants rather than making every limit caller-configurable.

Configuration validation:

- valid JSON object with no unknown fields;
- `root` required;
- root path absolute;
- existing directory;
- canonicalizable without escaping through a symlinked configured root;
- no Credential References.

InspectConfig returns no Credential References and a bounded human-readable connection summary containing the canonical workspace root.

The ConfigSchema uses generic string/path presentation metadata compatible with the existing schema-driven Web/Desktop renderer.

## Tool contracts

### `workspace.read`

Input:

~~~json
{
  "path": "src/main.go",
  "offset": 1,
  "limit": 200
}
~~~

- `path`: required normalized workspace-relative file path.
- `offset`: optional 1-based starting line.
- `limit`: optional bounded line count.

Output:

~~~json
{
  "path": "src/main.go",
  "content": "...",
  "startLine": 1,
  "endLine": 120,
  "truncated": false
}
~~~

Rules:

- regular text file only;
- no symlink escape;
- bounded raw bytes and returned bytes/lines;
- invalid UTF-8 returns a canonical Tool failure rather than lossy content.

### `workspace.search`

Input:

~~~json
{
  "mode": "content",
  "path": "backend",
  "query": "GovernanceFactExtractor",
  "pattern": "**/*.go",
  "regex": false,
  "caseSensitive": true,
  "maxResults": 100
}
~~~

Modes:

- `files`: match workspace-relative file paths against a glob/pattern;
- `content`: scan bounded regular text files and return matching line snippets.

Rules:

- optional `path` scopes search below one workspace-relative directory;
- no symlink traversal;
- bounded visited files, per-file bytes, aggregate bytes, matches, and snippet length;
- regular expression compilation is bounded by Go's regexp engine and input limits;
- implementation uses Go filesystem APIs, not a shell command;
- V1 may not implement full gitignore semantics; this limitation is explicit.

### `workspace.edit`

Input supports exactly one file operation:

~~~json
{
  "path": "src/main.go",
  "operation": "replace",
  "edits": [
    {
      "oldText": "old",
      "newText": "new",
      "expectedReplacements": 1
    }
  ]
}
~~~

or:

~~~json
{
  "path": "notes.md",
  "operation": "create",
  "content": "..."
}
~~~

or:

~~~json
{
  "path": "obsolete.txt",
  "operation": "delete"
}
~~~

Rules:

- one workspace-relative regular file;
- create fails when target exists;
- replace fails when target is absent, non-regular, oversized, invalid UTF-8, or any expected replacement count differs;
- all replacements are calculated before writing;
- delete rejects directories/symlinks/special files;
- update/create uses same-directory temporary content then replacement where supported;
- output reports operation/path and bounded mutation metadata, not the complete file content.

### `workspace.shell`

Input:

~~~json
{
  "command": "pnpm test",
  "cwd": ".",
  "timeoutMs": 30000
}
~~~

Output:

~~~json
{
  "exitCode": 0,
  "stdout": "...",
  "stderr": "...",
  "timedOut": false,
  "truncated": false
}
~~~

Rules:

- command is required and bounded;
- cwd is optional, workspace-relative, directory-only, and root-contained;
- timeout uses a bounded default and maximum;
- stdout/stderr are bounded independently/collectively;
- cancellation terminates/reaps the process best-effort with platform-specific handling;
- environment starts from a small non-secret allow-list needed for normal process/runtime operation;
- Tool has no Credential resolver requirement;
- host command may access resources outside workspace using host-user OS authority.

## Path containment

Create one internal path resolver shared by read/search/edit and shell cwd validation.

For a requested relative path:

~~~text
validate relative form
  -> lexical clean/join under canonical root
  -> verify lexical containment
  -> inspect existing parent components
  -> reject escaping symlinks
  -> enforce target-type rules
~~~

Tests must cover:

- `..` traversal;
- absolute paths;
- nested legal paths;
- symlink to outside root;
- symlinked parent directory;
- missing target for create;
- special-file rejection where practical;
- root edge cases.

Do not rely on string prefix checks alone.

## Governance Facts routing

Current Runtime uses one global `UnknownExtractor`.

Phase 20 introduces config-aware reviewed extraction without activating the Provider.

### Generic optional capability

Add an optional built-in Provider plugin capability conceptually equivalent to:

~~~go
type GovernanceFactProvider interface {
    ExtractGovernanceFacts(
        context.Context,
        json.RawMessage, // persisted non-secret Provider config
        catalogdomain.Tool,
        invocationdomain.JSONValue,
    ) (invocationdomain.GovernanceFacts, error)
}
~~~

Exact naming/package placement may vary to preserve dependency boundaries.

### Runtime router

The Invocation extractor receives the canonical Tool, loads the persisted generic Provider configuration by ProviderID, and looks up the statically linked plugin registration.

If and only if that built-in plugin implements the reviewed fact capability, extraction runs locally.

Otherwise:

~~~text
UnknownGovernanceFacts
~~~

Constraints:

- no Provider activation;
- no network;
- no external plugin process;
- no Credential resolution;
- no filesystem/process side effects from the router itself;
- extractor failures fail Invocation preflight rather than silently downgrading known facts.

### Workspace fact taxonomy

Initial reviewed facts:

| Tool | Effects | Resources |
|---|---|---|
| `workspace.read` | `filesystem.read` | `workspace_path:<normalized path>` |
| `workspace.search` | `filesystem.search` | `workspace_path:<normalized search root>` |
| `workspace.edit` | `filesystem.write` | `workspace_path:<normalized path>` |
| `workspace.shell` | `process.execute` | `workspace_path:<normalized cwd>` |

Shell does not claim that its command has only a process effect. Policy can target ProviderID/ToolID/effect/cwd, require approval, or deny it.

## Stage 1 — Governance documentation

Deliver:

- ADR-0030;
- this Phase 20 plan;
- ADR index / roadmap entry.

Commit:

~~~text
docs(workspace): define native workspace tools phase
~~~

Implementation does not begin before this commit.

## Stage 2 — Governance fact routing

Responsibilities:

- add optional reviewed fact extraction capability to the built-in Provider boundary;
- add a config-aware router using persisted Provider configuration;
- preserve `UnknownGovernanceFacts` for unsupported/external Tools;
- route without Provider activation;
- add deterministic unit/integration tests.

Commit:

~~~text
feat(invocation): route provider governance facts
~~~

## Stage 3 — Workspace Provider and structured file Tools

Create a focused package under:

~~~text
backend/internal/workspace/
  infrastructure/plugin/
~~~

Exact subpackages may split path/search/edit helpers when useful.

Responsibilities:

- descriptor/config schema;
- root validation/canonicalization;
- path resolver;
- static discovery of read/search/edit/shell descriptors (shell execution may land next commit);
- read implementation;
- search implementation;
- deterministic create/replace/delete edit;
- workspace Governance Facts implementation for structured Tools;
- bounded canonical JSON outcomes.

Commit:

~~~text
feat(workspace): add native file tools provider
~~~

## Stage 4 — Workspace shell

Responsibilities:

- platform shell selection;
- root-contained initial cwd;
- timeout/cancellation;
- bounded stdout/stderr;
- reduced non-secret environment;
- canonical completed/failed result handling;
- host-authority limitation tests/docs.

Commit:

~~~text
feat(workspace): add governed shell tool
~~~

## Stage 5 — Runtime / operator integration

Responsibilities:

- register Workspace in `providerplugin/builtin`;
- prove stable Provider/Tool synchronization and execution;
- prove Profile-scoped Invocation/Policy/Audit path;
- add `mender provider add workspace --root ...` convenience CLI if the existing CLI has no generic built-in creation path;
- verify management HTTP generic plugin catalog/create/update projection;
- verify existing generic Web/Desktop Provider renderer can create/edit the new built-in without Workspace-specific UI branches;
- update CLI/Web/Desktop/provider development docs as needed;
- update built-in plugin count/identity tests.

No Workspace-specific HTTP endpoint.

Commit(s):

~~~text
feat(runtime): compose workspace provider
feat(cli): configure workspace providers
docs(workspace): document workspace provider
~~~

Commits may be combined only when checks and write boundaries stay clear.

## Stage 6 — Completion gates

Focused gates include:

~~~text
go test ./backend/internal/workspace/...
go test ./backend/internal/invocation/...
go test ./backend/internal/providerplugin/...
go test ./backend/internal/bootstrap/...
go test ./backend/internal/cli/...
~~~

Then run repository gates:

~~~text
pnpm check
pnpm security:check
~~~

Also inspect staged/committed paths to ensure:

- no `.agents/`;
- no `.pi/`;
- no `.intent-loop/`;
- no unrelated pre-existing management UI changes.

Completion documentation records exact evidence and known V1 limitations.

## V1 security bounds

Use named constants with tests. Initial values may be tuned during implementation while staying intentionally conservative.

Required categories:

- max command bytes;
- default/max shell timeout;
- max stdout/stderr/result bytes;
- max read file/return bytes and line count;
- max edit input/file bytes and replacement count;
- max search visited files;
- max search per-file/aggregate bytes;
- max search result count;
- max path length / path segments where host APIs do not already provide a stricter bound.

Do not make security limits unbounded through Provider configuration in Phase 20.

## V1 known limitations

Expected and acceptable when documented:

- host shell is not a kernel/container sandbox;
- search is an in-process bounded implementation and may not match `ripgrep` feature/performance;
- structured file Tools are text-first;
- no LSP/AST/code-intelligence Tools;
- no persistent shell/session/PTY;
- no background process manager;
- no shell command semantic analyzer;
- no automatic Profile templates marketed as Agents.

## Completion evidence

Phase 20 landed in document-first segmented commits:

- `52c28b9` — define ADR-0030, the Phase 20 plan, and roadmap contract;
- `6134f56` — add non-activating, built-in-only reviewed Governance Fact routing while external/unreviewed Providers remain unknown;
- `709eda8` — add the Workspace Provider configuration plus bounded `read`, `search`, and deterministic `edit` capabilities;
- `1a1f071` — add bounded host `shell`, reduced environment inheritance, timeout handling, Unix process-group cancellation, and Windows target support;
- `5ba043b` — register Workspace as the fourth built-in Provider and prove stable ToolIDs plus Profile/Policy/Audit/generic-management integration;
- `6f0ab1e` — add `mender provider add workspace --root ...` through the generic registration/synchronization path;
- `337e875` — document Workspace operation across CLI/Web/provider guidance;
- `3a6efad` — harden structured filesystem execution with Go `os.Root` root-scoped operations after the repository security linter identified a WalkDir/open TOCTOU risk.

The completed capability surface is:

~~~text
io.mender.provider.workspace
  workspace.read
  workspace.search
  workspace.edit
  workspace.shell
~~~

Structured file operations retain explicit relative-path/symlink checks and use root-scoped open/read/remove/rename operations so filesystem actions cannot escape solely through a check/use race. `edit` refuses stale or ambiguous replacement counts before mutation and uses a same-root temporary file plus rename for replacement. Search remains bounded and in-process.

`workspace.shell` is intentionally different: its initial cwd must be inside the Workspace, its environment/output/timeout are bounded, and Unix cancellation targets the process group, but the command still has the Mender host user's OS authority. It is not presented as a kernel/container sandbox and its Governance Facts claim only `process.execute` plus the initial `workspace_path`, not inferred hidden command effects.

Runtime acceptance proves:

- first sync creates exactly four canonical Tools and later sync preserves SourceKey-stable ToolIDs;
- a Profile may expose only `read`/`edit`, with `shell` rejected as outside scope;
- a Policy matching `filesystem.write` plus `workspace_path` rejects `edit` before filesystem mutation;
- successful Workspace invocation produces the normal Audit record;
- `/api/v1/provider-plugins` exposes the Workspace ConfigSchema/capabilities through the existing generic management contract;
- Web/Desktop require no Workspace-specific frontend branch;
- no model API, prompt, conversation, planner, memory, compaction, retry, or Agent Loop was added.

Final verification:

~~~text
go test focused Workspace/Invocation/Provider/Bootstrap/CLI/HTTP suites   PASS
go test -race Workspace + Bootstrap                                       PASS
golangci-lint ./backend/...                                               0 issues
Windows Workspace package cross-compile                                   PASS
pnpm check                                                                PASS
i18n                                                                      651 / 651 zh-CN
Vitest                                                                    9 files / 59 tests
Web/Desktop production builds                                             PASS
pnpm security:check                                                       PASS
govulncheck backend/desktop/SDK/example                                   no reachable vulnerabilities
pnpm audit --prod --audit-level high                                      no known vulnerabilities
~~~

The pre-existing uncommitted management-UI redesign files remained untouched by Phase 20 commits. `.agents/`, `.pi/`, and `.intent-loop/` also remained outside every commit.

## Exit criteria

- [x] ADR-0030 and Phase 20 plan are committed before implementation.
- [x] built-in Workspace plugin appears in Provider plugin inventory.
- [x] synchronization exposes exactly four stable Workspace SourceKeys.
- [x] read/search/edit root containment and I/O bounds have focused tests.
- [x] edit stale/ambiguous mutation fails closed.
- [x] shell timeout/output/cwd behavior and host-authority limitation have focused tests.
- [x] Workspace reviewed Governance Facts reach Policy through non-activating routing.
- [x] unsupported/external Tools still produce unknown facts.
- [x] Profile-scoped Workspace invocation uses shared Policy/guards/Audit.
- [x] CLI and generic management paths can configure the Workspace Provider.
- [x] Mender has no new model/Agent Loop responsibility.
- [x] `pnpm check` and `pnpm security:check` pass.
- [x] protected skill/intent-loop directories and unrelated pre-existing UI work are absent from Phase 20 commits.
