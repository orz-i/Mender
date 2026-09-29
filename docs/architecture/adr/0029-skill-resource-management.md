# ADR-0029: Manage Agent Skills as versioned resources and keep deployment host-specific

- Status: Accepted
- Date: 2026-09-28

## Context

Mender already ships one thin Agent Skill at `skills/mender-tools/SKILL.md`. It teaches agents to progressively discover and invoke governed Mender Tools through the stable CLI JSON contract:

~~~text
search -> describe -> call
~~~

That arrangement is intentionally different from Provider plugins and canonical Tools:

- Provider plugins bring executable capabilities into Mender.
- Tools are governed executable capabilities in the Catalog.
- Profiles expose bounded views over Tools.
- Agent Skills provide procedural instructions, references, scripts, and assets that teach an agent how to use capabilities.

The Agent Skills specification has matured into a portable directory format centered on `SKILL.md`, with optional `scripts/`, `references/`, and `assets/`. Current compatible clients discover skills from host-specific locations such as project/user `.agents/skills`, `.claude/skills`, `.github/skills`, and `~/.copilot/skills`.

A second portable packaging layer, Agent Plugins 1.0, defines a plugin manifest with fixed `skills/` and optional `mcp.json` locations. It can package Skills and MCP connectivity together for compatible clients.

Mender currently has no Skill inventory, version/install identity, trust state, update model, deployment records, or portable export path. Copying the repository Skill manually into host directories would make those directories the de facto source of truth and would create drift across clients.

## Decision

Mender will introduce a **Skill Library** as a first-class resource-management capability.

The library manages canonical Skill content and lifecycle metadata while preserving the Agent Skills format as the content contract.

### Agent Skills is the canonical content format

A Skill installation contains one Agent Skills-compatible directory:

~~~text
skill-name/
├── SKILL.md
├── scripts/
├── references/
├── assets/
└── ...
~~~

Mender does not invent a replacement Skill DSL.

`SKILL.md` frontmatter is parsed and validated against the portable Agent Skills constraints that Mender supports:

- `name` is required, lowercase, hyphenated, bounded, and matches the Skill directory;
- `description` is required and bounded;
- optional `license`, `compatibility`, `metadata`, and experimental `allowed-tools` are preserved;
- referenced resources remain relative to the Skill root.

Unknown portable content files may be retained subject to host security limits.

### Skill identity and installation identity are distinct

Portable `name` is not a globally unique database identity.

Mender owns:

~~~text
SkillID          skill_...
InstallationID   sinst_...
Name             mender-tools
Source           builtin | local-directory | archive | git | agent-plugin
Source reference optional
Content digest   sha256:...
Version hint     optional metadata, not identity
InstalledAt
~~~

Multiple installations/versions of one logical Skill may coexist.

An installation is immutable after ingestion. Updating a Skill creates another exact installation and explicit activation/deployment may move to it.

Mender does not automatically follow `latest`.

### Trust and enablement are separate from installation

Mender distinguishes:

- **installed** — content exists in the managed library;
- **trusted** — an administrator/user has accepted the content provenance/risk;
- **enabled** — Mender may deploy or export the installation;
- **deployed** — one materialized copy is present at a specific client target/scope.

A built-in Mender Skill may start trusted/enabled by construction. Third-party installations do not inherit trust merely because they parsed successfully.

### Skill content is not an execution authorization boundary

`allowed-tools` is experimental portable metadata. Mender may surface it as a compatibility or review hint, but it never grants execution authority.

Agent execution through Mender remains governed by:

~~~text
Profile
-> Invocation scope
-> Policy
-> Credential scope
-> execution/result guards
-> Audit
~~~

A Skill must not bypass Mender by silently selecting another execution path when Mender rejects an invocation.

### Managed store is the source of truth

Canonical installed content lives below a Mender-owned root, conceptually:

~~~text
~/.mender/skills/
  sinst_.../
    SKILL.md
    scripts/
    references/
    assets/
~~~

Host-specific directories are deployment targets, not canonical storage.

Mender records the installation digest and deployment target so drift can be detected.

### Deployment is adapter-specific

Mender materializes enabled Skill installations into supported host locations using explicit deployment adapters.

Initial target families may include:

- Agents-compatible project/user directories;
- Claude-compatible project directories;
- GitHub/Copilot-compatible project/user directories.

Default deployment uses a copy/materialization model rather than a symlink. A future opt-in link mode may exist, but Mender does not depend on symlink semantics for correctness.

Deployment never rewrites the canonical installation.

### Package ingestion is bounded and fail-closed

Before content enters the managed store Mender validates and inventories it.

At minimum:

- no path escape outside the Skill root;
- no symlink traversal in managed content;
- bounded file count and total/file sizes;
- required `SKILL.md`;
- valid frontmatter and Skill name/directory match;
- UTF-8 text requirements where applicable;
- content digest over a deterministic file manifest;
- executable/script inventory for review.

Future source adapters may add archive, Git, signature/provenance, secret scanning, and dependency/network analysis.

Validation success does not imply trust.

### Built-in Skill registration

The existing `skills/mender-tools` Skill becomes the first built-in Skill installation.

It remains thin and continues to use the stable CLI JSON contract. Mender does not generate one Skill per Tool or copy Tool schemas into Skill resources.

### Agent Plugin export is a delivery adapter

Mender may export selected enabled Skills into Agent Plugins 1.0 packages:

~~~text
plugin.json
skills/
  <skill>/
    SKILL.md
    ...
mcp.json
~~~

Agent Plugin packaging is downstream delivery. It is not the persistence model for the Skill Library and is not the same concept as Mender Provider Plugin packaging.

### Remote MCP Skill extensions remain adapters

Draft/implementation-specific MCP Skill extensions may be supported later as import/export adapters. They do not define Mender's Skill domain model until a stable interoperable contract justifies that dependency.

## Security invariants

1. Skill validation never grants Tool authority.
2. Skill scripts are untrusted content unless the installation is trusted.
3. Installing content does not execute scripts.
4. Listing/inspecting Skills does not execute Skill content.
5. Deployment is explicit and records exact installation identity/digest.
6. Deployment targets cannot escape the explicitly selected user/project root.
7. Existing Mender Policy/Profile/Credential/Invocation governance remains authoritative.
8. Secret material is not stored in Skill metadata or deployment records.
9. Updates are explicit exact-installation changes; no automatic latest retargeting.
10. Host-specific deployment copies are not the canonical source of truth.

## Alternatives considered

### Treat every Skill as a Tool

Rejected because Skills are procedural context/resources rather than canonical executable capabilities. It would blur discovery and authorization boundaries.

### Generate one Skill per Tool

Rejected because it duplicates Catalog metadata/schema, scales poorly, and defeats progressive disclosure.

### Let .agents/.claude/.github directories be the database

Rejected because multiple clients/scopes create divergent mutable copies and provide no exact installation/version identity.

### Fold Skills into Provider Plugin packages

Rejected because Provider plugins are upstream capability adapters while Skills are downstream agent workflow resources.

### Use Agent Plugin as the internal persistence model

Rejected because Agent Plugin is a portable delivery format combining multiple component classes. Mender needs installation/trust/deployment lifecycle independent of any one export package.

### Automatically update Git-hosted Skills

Rejected because Skill instructions/scripts affect agent behavior and may expand authority. Updates require explicit review and activation.

## Consequences

Positive:

- Mender can manage Agent Skills without inventing a proprietary content format;
- exact installation identity and digests make deployment reproducible;
- one canonical store can target multiple agent clients;
- third-party Skill trust can be reviewed separately from parsing;
- existing Mender execution governance remains the security boundary;
- Agent Plugin export can be added without changing core persistence.

Trade-offs:

- Skill installation adds filesystem/package security work;
- host deployment paths vary and require adapters;
- version hints in portable metadata are advisory rather than sufficient package identity;
- script execution remains client-dependent and cannot be made safe by metadata alone;
- Git/archive/registry provenance requires additional source adapters.

## Follow-up

Phase 19 implements this decision incrementally:

1. Skill domain, validation, deterministic resource manifest/digest, and built-in `mender-tools` inventory;
2. persistent managed Skill installations and explicit trust/enablement;
3. CLI list/inspect/validate/install lifecycle;
4. local deployment adapters and drift detection;
5. Agent Plugin export;
6. management API/Wails/Web/Desktop Skill inventory and deployment surfaces;
7. full repository/security verification.
