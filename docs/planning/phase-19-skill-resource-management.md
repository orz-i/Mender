# Phase 19 Plan — Skill Resource Management & Distribution

Status: In Progress

## Objective

Turn the repository's existing thin `mender-tools` Agent Skill into the first member of a managed Skill Library, then add safe versioned installation and explicit distribution without changing Mender's Tool execution/governance boundary.

Architecture is governed by [ADR-0029](../architecture/adr/0029-skill-resource-management.md).

## External interoperability baseline

Phase 19 follows the current portable ecosystem rather than introducing a Mender-only Skill format:

- Agent Skills defines the `SKILL.md` directory structure, required `name` / `description`, optional resources, and progressive disclosure model.
- Agent Skills marks `allowed-tools` experimental; Mender treats it as review/compatibility metadata only.
- GitHub/Copilot currently discovers project Skills from `.github/skills`, `.claude/skills`, and `.agents/skills`, and personal Skills from `~/.copilot/skills` and `~/.agents/skills`.
- Agent Plugins 1.0 packages portable Skills under `skills/` and MCP configuration in root `mcp.json`.

References:

- https://agentskills.io/specification
- https://agent-plugins.org/specification
- https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills

## Domain language

### Skill

Logical Mender identity for an Agent Skill lineage.

It owns stable Mender `SkillID` and display/index metadata, not mutable content.

### Skill Installation

One immutable exact content snapshot.

~~~text
InstallationID
SkillID
name
description
source kind
source reference
digest
version hint
installedAt
trust state
enabled
~~~

### Skill Resource

One file inside the exact installation:

~~~text
relative path
size
sha256
kind
executable hint
~~~

Resource paths are always relative to the Skill root.

### Skill Deployment

A materialized exact installation exposed to one agent host target/scope.

~~~text
DeploymentID
InstallationID
target
scope
project root if applicable
destination
deployed digest
deployedAt
~~~

Deployment is not ownership. The library remains canonical.

## Architecture

~~~text
Skill source
   │
   ▼
Ingestion / validation
   │
   ▼
Skill Library
 ├─ logical Skill
 ├─ exact Installation
 ├─ resource manifest/digest
 └─ trust + enablement
   │
   ├───────────────┐
   ▼               ▼
Deployment      Export
adapters        adapters
   │               │
   ▼               ▼
.agent dirs     Agent Plugin
                   │
                   └─ optional MCP config
~~~

No Skill path bypasses the governed Tool Invocation pipeline.

## Stage 1 — Core Skill model and validator

Create a new backend bounded context under `backend/internal/skill`.

Initial responsibilities:

- parse/validate Agent Skills `SKILL.md` frontmatter;
- enforce portable name/description constraints;
- preserve optional license/compatibility/metadata/allowed-tools;
- recursively inventory bounded Skill resources;
- reject symlinks/path escape;
- compute per-file SHA-256 and deterministic installation digest;
- classify `SKILL.md`, scripts, references, assets, and other resources;
- never execute content.

Checks:

- valid minimal Skill;
- invalid/missing frontmatter;
- invalid name or directory mismatch;
- size/file-count boundaries;
- symlink rejection;
- deterministic digest independent of directory iteration order;
- content change changes digest.

## Stage 2 — Built-in Skill Library

Register repository `skills/mender-tools` as a built-in installation.

Built-in registration must:

- validate through the same core validator;
- have stable logical identity;
- expose exact digest/resources;
- be trusted/enabled by construction;
- not copy Tool schemas or Provider catalogs.

The current CLI Skill contract test remains authoritative for its workflow semantics.

## Stage 3 — Persistent managed installations

Add SQLite-backed installation metadata and a managed filesystem store.

Initial source kinds:

- `builtin`;
- `local-directory`.

Managed local-directory installation copies an immutable validated snapshot into the Mender Skill root.

Persist:

- SkillID and InstallationID;
- normalized metadata;
- digest;
- source kind/reference;
- trust/enabled state;
- installed timestamp.

Do not persist Skill file contents in SQLite.

Removal is blocked while deployments reference the installation.

## Stage 4 — CLI lifecycle

Add a `mender skill` command family with stable JSON output:

~~~text
mender skill list
mender skill inspect <skill-or-installation>
mender skill validate <directory>
mender skill install <directory>
mender skill trust <installation>
mender skill untrust <installation>
mender skill enable <installation>
mender skill disable <installation>
mender skill remove <installation>
~~~

The CLI never executes Skill scripts.

Built-in content is listable/inspectable but not removable.

## Stage 5 — Local deployment adapters

Initial explicit deployment targets:

~~~text
agents-project   -> <project>/.agents/skills/<name>
github-project   -> <project>/.github/skills/<name>
claude-project   -> <project>/.claude/skills/<name>
agents-user      -> ~/.agents/skills/<name>
copilot-user     -> ~/.copilot/skills/<name>
~~~

Deployment rules:

- exact InstallationID required;
- installation must be trusted and enabled;
- default behavior copies content;
- project root/user home is explicit and normalized;
- destination escape/symlink hazards fail closed;
- existing unrelated content is never silently overwritten;
- deployment records exact digest;
- inspect reports drift when materialized content changes.

CLI:

~~~text
mender skill deploy <installation> --target ... [--project ...]
mender skill undeploy <deployment>
mender skill deployments
~~~

## Stage 6 — Agent Plugin export

Export one or more enabled Skills as Agent Plugins 1.0.

Initial export contains:

~~~text
plugin.json
skills/<name>/...
~~~

A later option may emit `mcp.json` pointing at a selected Mender Profile exposure endpoint where the target format can represent it safely.

Export is deterministic and never mutates installed Skills.

## Stage 7 — Management surfaces

Expose Skill inventory and lifecycle through the same management semantics used by Web/Desktop:

- list/inspect Skills/installations;
- trust/enable status;
- resource/security inventory;
- deployment list/status/drift;
- Desktop local-directory picker for host-local install;
- Web does not accept arbitrary server filesystem paths.

No arbitrary Skill-provided frontend code is loaded.

## Security limits

Initial defaults should be explicit constants with tests:

- bounded total file count;
- bounded individual file size;
- bounded total installation bytes;
- no symlink resources;
- no path escape;
- no device/socket/special files;
- frontmatter bounds from Agent Skills;
- metadata count/key/value limits owned by Mender where the portable format is otherwise unbounded;
- deterministic hashing over normalized relative paths and content digests.

Trust policy is host-owned and independent of portable frontmatter.

## Persistence / compatibility

Phase 19 creates new Skill-specific state and does not change existing Provider/Profile/Tool/Audit identity.

The existing `skills/mender-tools` source remains in the repository for source distribution. Runtime registration treats it as built-in content; managed third-party installations live outside the repository.

No Provider Plugin protocol or Tool schema migration is required.

## Commit segmentation

1. Governance:

~~~text
docs(skill): define skill resource management phase
~~~

2. Core model/validator:

~~~text
feat(skill): add Agent Skill validation and manifests
~~~

3. Built-in library:

~~~text
feat(skill): register built-in mender tools skill
~~~

4. Persistence/install lifecycle:

~~~text
feat(skill): persist managed skill installations
~~~

5. CLI lifecycle:

~~~text
feat(cli): manage skill resources
~~~

6. Deployment:

~~~text
feat(skill): add local skill deployment adapters
~~~

7. Agent Plugin export:

~~~text
feat(skill): export portable agent plugins
~~~

8. Management/UI and completion:

~~~text
feat(api): manage skill resources
feat(ui): manage skill library
docs(skill): complete skill resource management phase
~~~

Skill/workflow directories `.agents/`, `.pi/`, and `.intent-loop/` remain outside all commits.

## Current implementation progress

Completed in the first Phase 19 implementation tranche:

- Agent Skills frontmatter/resource validation with bounded ingestion and no execution;
- deterministic per-resource and installation SHA-256 manifests;
- stable Mender SkillID / exact InstallationID types;
- built-in `mender-tools` compiled into release binaries with a parity test against the repository source;
- immutable managed local-directory snapshots plus SQLite installation metadata;
- side-by-side exact installations for changed content from one logical source lineage;
- explicit trust and enablement state (local third-party installations start untrusted/disabled);
- shared Runtime Skill Library combining built-in and managed installations;
- CLI `skill list/inspect/validate/install/trust/untrust/enable/disable/remove` with stable JSON output;
- tranche verification: full `pnpm check` and `pnpm security:check` pass after synchronizing the Desktop module's indirect YAML dependency.

Next work starts with explicit deployment records/adapters and drift detection, then Agent Plugin export and management/Web/Desktop surfaces.

## Exit criteria

- [x] Agent Skills-compatible content is validated without execution.
- [x] Resource manifest/digest is deterministic and bounded.
- [x] Built-in `mender-tools` is represented in the Skill Library.
- [x] Managed local-directory installations are immutable and versionable side by side.
- [x] Trust and enablement are explicit, separate states.
- [x] CLI supports list/inspect/validate/install/lifecycle in JSON mode.
- [ ] Local deployment adapters materialize exact installations safely.
- [ ] Deployment drift is detectable.
- [ ] Agent Plugin 1.0 export is deterministic.
- [ ] Management/Web/Desktop can inspect and manage Skill resources without loading Skill code.
- [ ] Existing Tool/Profile/Policy/Credential/Invocation behavior is unchanged.
- [ ] `pnpm check` and `pnpm security:check` pass.
