# CLI Reference

Mender's CLI is the primary local management and execution interface.

It calls the same application/runtime boundaries used by other adapters; it does not require a local management HTTP server.

## Database

Resolution order:

1. `--db <path>`
2. `MENDER_DB`
3. `~/.mender/mender.db`

Example:

```bash
mender --db ./mender.db provider list
```

## Output

Human-readable text is the default.

For automation and agents, always use:

```bash
--output json
```

Successful JSON commands emit exactly one JSON document on stdout. Errors and diagnostics use stderr.

## Provider management

Add a remote MCP Provider:

```bash
mender provider add http \
  --namespace github \
  --name "GitHub" \
  --endpoint https://example.com/mcp
```

Add a local stdio MCP Provider:

```bash
mender provider add stdio \
  --namespace local-tools \
  --name "Local tools" \
  --exec /path/to/server \
  --arg --stdio
```

Persistent stdio configuration intentionally does not store environment values.

List Providers:

```bash
mender provider list --output json
```

Refresh one Provider using ProviderID or namespace:

```bash
mender provider sync github --output json
```

## Tool discovery

Search compact metadata:

```bash
mender tool search "create issue" --output json
```

Restrict discovery to a Profile:

```bash
mender tool search "create issue" \
  --profile default \
  --output json
```

Load the full schema only after selecting a Tool:

```bash
mender tool describe tool_... --output json
```

This is the intended progressive-disclosure workflow for large Catalogs.

## Tool invocation

Invoke with inline JSON:

```bash
mender tool call tool_... \
  --args '{"title":"Bug"}' \
  --output json
```

Or use a JSON file:

```bash
mender tool call tool_... \
  --args-file ./arguments.json \
  --output json
```

The CLI reloads the Tool's persisted Provider configuration, activates its executor, and enters the shared Invocation pipeline.

## Profiles

Create:

```bash
mender profile create \
  --slug default \
  --name "Default" \
  --mode hybrid
```

Available modes:

- `direct`
- `dynamic`
- `hybrid`

Manage membership:

```bash
mender profile add-tool default tool_... --direct
mender profile remove-tool default tool_...
```

Inspect:

```bash
mender profile list --output json
mender profile show default --output json
```

## MCP serving

Serve a persisted Profile over stdio:

```bash
mender serve mcp --profile default --stdio
```

Serve it over stateless Streamable HTTP:

```bash
mender serve mcp --profile default --http 127.0.0.1:8080
```

Before serving, Mender activates all persisted MCP Providers referenced by the Profile.

## Agent Skill

The repository Skill lives at:

```text
skills/mender-tools/SKILL.md
```

Its primary workflow is:

```text
tool search --output json
  -> tool describe --output json
  -> tool call --output json
```

The Skill intentionally contains no duplicated Tool Catalog.

## Exit codes

- `0`: success
- `1`: internal/runtime setup failure
- `2`: usage or validation error
- `3`: missing configuration/resource
- `4`: policy or approval denial
- `5`: provider/execution failure

## Current limits

- Persistent stdio environment values are not supported because Credential-backed secret injection has not yet been designed for CLI configuration.
- MCP input-required results can be represented by the runtime, but CLI continuation is not yet implemented.
- MCP Tasks extension support remains deferred pending the adapter extension boundary.
