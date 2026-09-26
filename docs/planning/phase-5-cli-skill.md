# Phase 5 Plan — CLI Discovery and Agent Skill

## Objective

Make Mender useful as a local-first product without requiring Web, Desktop, or an HTTP management API.

Phase 5 provides:

- Persistent MCP Provider connection configuration.
- Persistent Profiles.
- A stable command-line interface for provider, profile, search, describe, invoke, and MCP serving workflows.
- Stable machine-readable JSON output for agents and scripts.
- A thin Agent Skill that uses progressive disclosure through the CLI.

The CLI is an adapter over the existing application/runtime boundaries. It is not an HTTP client to Mender.

## CLI framework

Mender uses Cobra v1.10.2 for its command tree and flag handling.

The dependency is confined to the CLI adapter. Domain and application packages do not import Cobra.

## State location

The CLI accepts a repository-wide persistent flag:

```text
--db <path>
```

Resolution order:

1. Explicit `--db`.
2. `MENDER_DB` environment variable.
3. `~/.mender/mender.db`.

The CLI creates the parent directory when using the default path.

No secret values are persisted in Provider configuration in Phase 5.

## Stable output contract

Every command that returns data supports:

```text
--output text
--output json
```

Default is `text` for humans.

Agent Skills and scripts must use:

```text
--output json
```

JSON output rules:

- stdout contains exactly one JSON document on success.
- diagnostics and errors go to stderr.
- field names use lower camel case.
- IDs are always stable Mender IDs, never row numbers.
- command success never depends on parsing human text.
- unknown fields may be added in later versions.
- existing field meanings must not change without an explicit contract decision.

Phase 5 does not introduce JSONL because all current commands return one bounded document.

## Command tree

Initial command surface:

```text
mender
  provider
    add
      http
      stdio
    list
    sync <provider>
  profile
    create
    list
    show <profile>
    add-tool <profile> <tool-id>
    remove-tool <profile> <tool-id>
  tool
    search <query>
    describe <tool-id>
    call <tool-id>
  serve
    mcp
```

The noun `tool` is kept explicit so future resources or prompts do not collide with discovery commands.

## Provider workflows

### provider add http

Required:

- namespace.
- display name.
- endpoint.

Optional:

- description.

Flow:

```text
Catalog RegisterProvider
   -> MCP config persistence
   -> connect
   -> initial synchronization
```

If MCP config persistence fails after Provider creation, the composition layer compensates by removing the newly-created Provider.

If the initial network synchronization fails, Provider and config remain persisted so the user can fix availability and run `provider sync` later.

### provider add stdio

Required:

- namespace.
- display name.
- executable.

Optional:

- description.
- repeated argv.
- working directory.

Persistent stdio configuration stores executable + argv + directory.

It does not persist environment variable values in Phase 5 because they are frequently used to carry secrets. Secret/environment injection will be designed with Credential configuration later.

### provider list

Returns Catalog Provider metadata plus persisted MCP transport summary where available.

It does not establish network/process connections.

### provider sync

Resolves Provider by stable ProviderID or namespace, loads its MCP config, connects, and runs the existing Catalog synchronization use case.

## MCP Provider config persistence

Connection configuration remains adapter-owned infrastructure.

Logical SQLite table:

```text
mcp_provider_configs
  provider_id       PRIMARY KEY
  transport         http | stdio
  endpoint          nullable
  executable        nullable
  args_json         non-null
  directory         non-null
```

The table contains no token, password, secret, or environment-value columns.

## Catalog additions

Phase 5 adds application operations required by CLI configuration:

- ListProviders.
- FindProviderByNamespace.
- RemoveProvider for compensating failed registration.

Removing a Provider relies on Catalog-owned cascading Tool deletion.

## Profile application model

Profile becomes persistent in Phase 5.

Application operations:

- CreateProfile.
- GetProfile by slug.
- ListProfiles.
- AddTool.
- RemoveTool.

Profile application code validates Tool existence through a consumer-owned Catalog port before adding a Tool.

SQLite Profile storage owns:

```text
profiles
  id
  slug UNIQUE
  display_name
  exposure_mode

profile_tools
  profile_id
  tool_id
  direct
  PRIMARY KEY(profile_id, tool_id)
```

Profile persistence does not query Catalog tables for semantic validation. The Profile application boundary performs the cross-context check explicitly.

## Tool discovery workflow

### tool search

Default searches the whole Catalog.

Optional:

```text
--profile <slug>
```

When set, it passes the Profile ToolID allow-list to the existing scoped Catalog search.

Human output is concise.

JSON output exposes:

- rank.
- toolId.
- providerId.
- name.
- title.
- description.

It does not eagerly return full JSON schemas.

### tool describe

Returns the full canonical Tool projection including input/output schemas and tags.

This is the second progressive-disclosure step.

### tool call

Accepts arguments through one of:

```text
--args '<json object>'
--args-file <path>
```

They are mutually exclusive.

Before invoking, the CLI:

1. Describes the Tool to obtain ProviderID.
2. Loads persisted MCP config for that Provider when present.
3. Connects/registers the Provider executor without requiring a full synchronization.
4. Calls the existing shared InvokeTool pipeline.

Built-in/in-process executors can be registered directly by composition in future phases.

JSON output exposes:

- invocationId.
- kind.
- content or continuation/task metadata.

The CLI never prints secret material from Credential resolution.

## Profile workflows

### profile create

Flags:

- `--slug`.
- `--name`.
- `--mode direct|dynamic|hybrid`.

### profile add-tool

Adds an existing Tool by ToolID.

Optional:

- `--direct` for hybrid mode.

Direct/dynamic modes normalize this flag according to Profile Domain rules.

### profile remove-tool

Removes membership only; it does not delete the Catalog Tool.

## MCP serving workflow

```text
mender serve mcp --profile <slug> --stdio
mender serve mcp --profile <slug> --http :8080
```

The flags are mutually exclusive.

Startup:

1. Load Profile.
2. Resolve unique ProviderIDs referenced by Profile Tools.
3. Load and connect persisted MCP Provider configs.
4. Register upstream executors without mandatory resynchronization.
5. Build the existing Profile-scoped MCP server.
6. Serve over stdio or stateless Streamable HTTP.

The CLI does not create a second execution path.

## Agent Skill

Repository skill location:

```text
skills/mender-tools/SKILL.md
```

The Skill is intentionally thin.

It teaches an agent to use:

```text
mender tool search "<intent>" --output json
mender tool describe <tool-id> --output json
mender tool call <tool-id> --args '<json>' --output json
```

This preserves progressive disclosure:

```text
intent
  -> search compact metadata
  -> describe only selected Tool
  -> invoke
```

The Skill must not duplicate Tool schemas, Provider catalogs, or execution logic.

## Exit code contract

Initial stable exit classes:

- 0 success.
- 2 usage/validation error.
- 3 not found/configuration error.
- 4 policy/approval denial.
- 5 provider/execution error.
- 1 internal/unclassified error.

Cobra usage text is suppressed for runtime failures.

## Testing

### Application/infrastructure

- Profile create/list/get/add/remove.
- Profile SQLite round-trip.
- MCP config SQLite round-trip.
- Config schema has no environment/secret columns.
- Provider list/find/remove.

### CLI

Commands are tested through the Cobra root with in-memory stdout/stderr buffers and temporary SQLite databases.

At minimum:

- provider add/list/sync using a test Streamable HTTP MCP server.
- profile create/add/list/show.
- tool search/describe.
- tool call reaches upstream MCP through shared Invocation pipeline.
- JSON output parses as exactly one document.
- profile-scoped search does not leak Tools.
- serve command configuration rejects invalid flag combinations.

### Skill contract

A test or validation script checks that SKILL.md only references commands present in the CLI and uses `--output json`.

## Commit segmentation

1. Governance.

```text
docs: define CLI and Agent Skill contract
```

2. Persistence/application.

```text
feat(catalog): add provider administration queries
feat(profile): add persistent profile application
feat(mcp): persist provider connection configs
```

3. CLI.

```text
feat(cli): add provider and profile commands
feat(cli): add tool discovery and invocation commands
feat(cli): add MCP serving commands
```

4. Skill/integration.

```text
feat(skill): add mender tools Agent Skill
test(cli): cover local end-to-end workflows
docs: complete CLI and Agent Skill phase
```

## Exit checklist

- [x] Cobra v1.10.2 is pinned.
- [x] CLI directly uses application/runtime boundaries rather than an HTTP management API.
- [x] Default database path and MENDER_DB override work.
- [x] Provider list/find/remove operations exist.
- [x] HTTP MCP Provider config persists without secret material.
- [x] stdio MCP Provider config persists executable/argv/directory without environment values.
- [x] Provider sync works from persisted config.
- [x] Profile create/get/list/add/remove operations persist through SQLite.
- [x] Tool search supports Profile-scoped discovery.
- [x] Tool describe exposes full canonical schema only on demand.
- [x] Tool call connects the owning Provider and uses shared InvokeTool.
- [x] CLI JSON success output is exactly one document.
- [x] MCP serve loads Profile and upstream Provider configs.
- [x] MCP serve supports stdio.
- [x] MCP serve supports stateless Streamable HTTP.
- [x] Agent Skill uses search -> describe -> call with --output json.
- [x] CLI integration tests cover provider/profile/tool workflows.
- [x] Domain packages have no Cobra dependency.
- [x] pnpm check passes.
