# Management Form Identifier Audit (October 2026)

## Principle

The **UI** accepts human-readable text for Mender-owned, human-authored identifiers and presents the canonical value *before submission*. The domain remains authoritative and validates exactly the same canonical form on all adapters (CLI, HTTP, Desktop, SDK); no compatibility bridge or backend identifier renaming is introduced.

Canonicalization is intentionally bounded to **trim surrounding whitespace, lowercase ASCII/Latin letters, and replace runs of internal whitespace with a hyphen**. It must never silently delete punctuation, turn invalid characters into valid ones, pick a new unique identifier, or mutate existing stored identities. An input that still fails the domain grammar must produce an inline explanation and disable Create. If an identifier becomes a duplicate after canonicalization, the server's conflict protection stays in force.

## Survey of management input surfaces

| Surface | User input | Contract / case policy | Audit action |
| --- | --- | --- | --- |
| Credentials → Add | Namespace, name | Mender-owned canonical `credential://namespace/name`, lowercase; the names may end in a hyphen | **Already corrected** in `e490ebe`; use the shared canonicalization helper without changing the reference grammar |
| Profiles → Create | Slug | Lowercase, 1–63 ASCII, starts with `a-z`, ends in `a-z0-9`, middle `a-z0-9-` | **Fix**: preview/validate canonical slug, send only the validated canonical slug, localize creation failures inside modal |
| Providers → Add (MCP, OpenAPI, Model Tools, external plugins) | Namespace | Same strict slug grammar as Profile, immutable after creation | **Fix**: canonicalization/validation/preview independent of plugin; normalize imported MCP JSON server name only as Mender namespace, keep URL/headers untouched; avoid false conflict-recovery comparisons |
| Profiles → Tool presentation | Alias | Backend permits uppercase, digits, `_`, `.`, `-` (no whitespace); external Tool alias is model-facing | **Preserve case**: do not force to lowercase or transform externally-visible Tool names. Validate alias separately if edited; do not change binding identity |
| Profiles, Servers, Policy, Provider authentication | Profile/Provider/Tool/Credential selections | Existing canonical IDs returned by the inventory, not human-authored identifiers | **Preserve exact values**: no rewriting on mutation or lookup |
| Provider connection/authentication and generic plugin forms | Executable, arguments, directory, URL, HTTP header, env variable, OpenAPI operation SourceKeys, plugin schema config, model name, Credential Reference | Environment names, operation IDs, paths, plugin-owned strings and remote service names may be case-sensitive | **Preserve exact values**: domain- and provider-specific validation only; never apply generic lowercase normalization |
| Tunnels → Create | Display name, external Tunnel ID, selected Credential | Display name is human text; external ID is remote-owned and opaque | **Preserve external-ID case**; trim human-display whitespace as backend already does |
| Sandboxes → Create | Display name, absolute path, image digest | Name is human text; filesystem path and image digests retain their own semantics | **Preserve case** for all; continue strict backend validation |
| Policy → Rules | Display name and inventory targets / advanced Principal references | Rule ID is generated; advanced Principal refs are exact-match identities | **Preserve case** for names and exact-match values, no Slug coercion |
| Plugins/Skills/Runtime/Catalog/Audit | Installation selectors, search, filters, configuration supplied by plugins | Selections and queries, not new Mender human-owned slug identities | **No global normalization**; case-insensitive search may remain a presentation behavior |

## Acceptance checks

1. `Demo Profile` creates Profile slug `demo-profile` without a backend validation error, with a visible preview. Invalid canonical slugs remain disabled with inline guidance; existing IDs remain unchanged.
2. `OpenAI Agent` creates Provider namespace `openai-agent` through both standard create flow and imported MCP JSON. All provider plugin renderers use the same validation.
3. Rejected/duplicate creates keep the modal open, preserve the editable values, and display safe and localized errors **inside** the modal. The background inventory must not be the only place for creation errors.
4. Canonicalization collisions must **not** be silently resolved or reported as success; server uniqueness remains enforced.
5. No case rewriting of URLs, external tunnel IDs, tool aliases, OpenAPI operation SourceKeys, model names, filesystem paths, environment variable names, opaque IDs, or secret values.
6. Targeted UI tests, translation coverage, lint, strict TypeScript, full tests, and desktop/web build gates pass.
