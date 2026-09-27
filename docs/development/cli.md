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

JSON commands emit exactly one JSON document on stdout whenever Invocation produces a canonical outcome. `rejected` and `failed` outcomes still use a non-zero process exit code; human diagnostics use stderr.

## Provider management

List the Provider plugins built into the current Mender binary:

```bash
mender provider plugins
mender provider plugins --output json
```

The initial built-in plugin is `io.mender.provider.mcp`. MCP connection transport is selected below that plugin rather than represented as a top-level Provider kind.

Add a remote MCP Provider:

```bash
mender provider add mcp http \
  --namespace github \
  --name "GitHub" \
  --endpoint https://example.com/mcp
```

Non-loopback Streamable HTTP endpoints must use HTTPS. Plain HTTP is accepted only for `localhost` / loopback development endpoints. URL userinfo such as `https://user:password@example.com/...` is rejected so Provider configuration cannot become a hidden credential store.

Add a local stdio MCP Provider:

```bash
mender provider add mcp stdio \
  --namespace local-tools \
  --name "Local tools" \
  --exec /path/to/server \
  --arg --stdio
```

Persistent stdio configuration never stores resolved secret environment values. Stdio Provider processes do not inherit Mender's arbitrary ambient environment: only a small non-secret process/runtime allow-list is passed automatically, plus explicit Credential-backed environment entries configured through the management/API surfaces.

The historical `provider add http` and `provider add stdio` paths remain hidden compatibility aliases for Phase 5 scripts. New scripts should use the plugin-first commands above.

Provider Credential bindings are currently configured through Web/Desktop or the structured management API. The Provider CLI intentionally does not yet add a separate authentication-binding flag DSL.

`provider list` reports Plugin identity as the primary implementation dimension; MCP transport appears only as connection detail.

List Providers:

```bash
mender provider list --output json
```

Refresh one Provider using ProviderID or namespace:

```bash
mender provider sync github --output json
```

## Policy

The CLI manages the same persistent global PolicySet used by MCP, Web, Desktop, and direct Tool invocation.

Show the current default decision and Rules:

```bash
mender policy show
mender policy show --output json
```

Change the decision used only when no enabled Rule matches:

```bash
mender policy set-default deny
mender policy set-default allow
mender policy set-default require_approval
```

Add a Rule:

```bash
mender policy rule add \
  --name "Require approval for MCP release writes" \
  --decision require_approval \
  --origin mcp \
  --scope profile \
  --profile-ref prof_... \
  --provider-id prov_... \
  --tool-id tool_... \
  --fact-knowledge known \
  --effect filesystem.write \
  --resource prefix:path:/workspace/release
```

Rule selectors are repeatable. Resource matchers use `exact:KIND:VALUE` or `prefix:KIND:VALUE`. Regex/custom expressions are intentionally unsupported.

Enable, disable, or remove a Rule:

```bash
mender policy rule disable rule_...
mender policy rule enable rule_...
mender policy rule remove rule_...
```

Policy Rule order has no meaning and there is no priority field. If no Rule matches, Mender uses the configured default decision. If multiple enabled Rules match, decisions combine as:

```text
deny > require_approval > allow
```

That means a matching deny cannot be overridden by an allow Rule. To build an allowlist, use `default=deny` plus explicit allow Rules.

`fact-knowledge unknown` is explicit. Effect/resource predicates only match known Governance Facts; they do not treat unknown facts as an empty known set. `require_approval` currently enters the existing fail-closed Approval gate, so it rejects until an interactive approval workflow exists.

The CLI is an advanced administrative interface, so stable Rule/Profile/Provider/Tool identities may be supplied as metadata selectors. Normal Web/Desktop Policy management resolves these through inventory selectors instead of asking users to type opaque IDs.

## Credentials

Credential inventory metadata is stored in Mender; secret values are not stored in the SQLite database.

List Credentials:

```bash
mender credential list --output json
```

Create an environment-backed Credential:

```bash
mender credential add environment github work \
  --display-name "GitHub work" \
  --env GITHUB_TOKEN
```

Create a native-keyring Credential by reading secret bytes from stdin:

```bash
printf '%s' "$GITHUB_TOKEN" | \
  mender credential add native github work \
    --display-name "GitHub work" \
    --secret-stdin
```

Rotate a native Credential the same way:

```bash
printf '%s' "$NEW_GITHUB_TOKEN" | \
  mender credential rotate github work --secret-stdin
```

Remove an unused Credential:

```bash
mender credential remove github work
```

There is deliberately no normal `--secret` flag: native secret values should not be placed in shell history or process arguments. List/create/rotate responses contain metadata only and never echo the secret value.

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

Catalog scope is the default for local administrative use. To invoke through a Profile security boundary, pass a Profile slug or ProfileID:

```bash
mender tool call tool_... \
  --profile default \
  --args '{"title":"Bug"}' \
  --output json
```

Profile-scoped calls enforce Tool membership before Provider activation and again inside the shared Invocation pipeline. The CLI activates the persisted Provider only for a Tool that is available in the selected execution scope.

Canonical JSON invocation kinds are `complete`, `input_required`, `task`, `rejected`, and `failed`. A rejected outcome includes `code` and `message`; a failed outcome includes `source`, `code`, and `message`.

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

Manage membership and optional ToolBinding governance:

```bash
mender profile add-tool default tool_... --direct

mender profile add-tool default tool_... \
  --alias create_prod_issue \
  --title "Create production issue" \
  --description "Create an issue in the production tracker" \
  --policy require_approval \
  --concurrency serial \
  --credential-ref credential://github/work

mender profile remove-tool default tool_...
```

ToolBinding policy accepts `inherit`, `require_approval`, or `deny`; it can only tighten the global Policy decision. Concurrency accepts `inherit` or `serial`, where `serial` applies only to that Profile + Tool binding. `--credential-ref` is an advanced metadata-only CLI path and may be repeated; the referenced Credential must already exist in inventory. Normal Web/Desktop management uses a selector instead of requiring raw Reference entry. Profile JSON always returns normalized binding fields. Existing clients that supply only ToolID/direct retain inherited defaults and an empty Credential allowlist.

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

Before serving, Mender ensures every configured Provider referenced by the Profile has an active plugin instance. Downstream serving remains an MCP exposure adapter; upstream Provider activation is plugin-generic.

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

## Build version

Inspect the running CLI build without opening the Runtime database:

```bash
mender version
mender --output json version
```

Release builds report version, source commit, and build date. Development builds report explicit development defaults.

## Exit codes

- `0`: success
- `1`: internal/runtime setup failure
- `2`: CLI usage error or `invalid_arguments` Invocation rejection
- `3`: missing configuration/resource or Tool unavailable in the selected scope
- `4`: policy or approval rejection
- `5`: provider/runtime/result-guard execution failure

## Current limits

- Provider Credential authentication bindings are managed through Web/Desktop or the structured management API; the Provider CLI does not yet expose a dedicated authentication-binding flag DSL.
- MCP input-required results can be represented by the runtime, but CLI continuation is not yet implemented.
- MCP Tasks extension support remains deferred pending the adapter extension boundary.
