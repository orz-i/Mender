# Phase 19 Plan — Skill Resource Management & Distribution

Status: Complete

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

## Completion evidence

Phase 19 landed in segmented commits covering the complete lifecycle:

- `864d1e2` — define ADR-0029 and the Phase 19 governance contract;
- `0d465d4` — add bounded Agent Skills validation and deterministic resource manifests;
- `a3deae2` — register release-safe built-in `mender-tools`;
- `937576a` — persist immutable managed Skill installations;
- `2e27245` — compose the Runtime Skill Library;
- `e9c1dfb` — add CLI installation/trust/enable lifecycle;
- `f1e7db7` — synchronize the Desktop module's indirect YAML dependency;
- `d4f2950` — document the managed Skill CLI;
- `668a3d8` — add exact local deployment adapters and drift detection;
- `dd625ff` — expose deployment lifecycle through the CLI;
- `0e65125` — export deterministic skills-only Agent Plugins 1.0 packages;
- `cf4cec1` — add OpenAPI/HTTP/Wails/SDK Skill management contracts;
- `99b46f7` — add shared Web/Desktop Skill Library UI;
- `edb92c9` — pin protobuf code generation to local versioned Go plugins;
- `ec082ce` — add an official Go vulnerability database fallback for unavailable canonical-network paths.

The resulting Skill lifecycle is:

~~~text
Agent Skills directory
  -> bounded validation / no execution
  -> exact immutable installation + digest
  -> explicit trust
  -> explicit enablement
  -> exact deployment / drift observation
  -> optional deterministic Agent Plugin export
~~~

Built-in and managed installations share one Runtime inventory. Local third-party content starts untrusted/disabled. Deployed installations cannot be disabled, untrusted, or removed until exact deployments are removed. Host deployment copies may drift; the managed installation remains canonical and immutable.

Supported deployment targets are:

~~~text
agents-project
github-project
claude-project
agents-user
copilot-user
~~~

Agent Plugin export uses the Agent Plugins 1.0 canonical schema and writes `plugin.json` plus `skills/<name>/...`. It does not implicitly generate `mcp.json`.

Management boundaries are explicit:

- HTTP/Web may list, inspect, trust/untrust, enable/disable, delete managed installations, list deployments, and undeploy;
- HTTP intentionally has no arbitrary server-path Skill install/deploy endpoint;
- Desktop adds host-local directory pickers for install and project deployment through Wails;
- CLI exposes the full local lifecycle, deployment, drift, and Agent Plugin export surface.

Final verification on the completed commit sequence:

~~~text
pnpm check                         PASS
pnpm security:check                PASS
i18n                               626 / 626 zh-CN
Vitest                              9 files / 59 tests
Go lint/test/race                   PASS
OpenAPI SDK drift                   PASS
Wails binding drift                PASS
Desktop architecture               PASS
Web/Desktop production builds      PASS
Provider SDK/example gates          PASS
Go module isolation                PASS
production dependency audit        no known vulnerabilities
govulncheck                         no reachable vulnerabilities
~~~

The canonical `vuln.go.dev` endpoint was unreachable from the development network during final verification, so `security-go.mjs` probes it first and falls back only on database/network availability failures to the official Go vulnerability database at `storage.googleapis.com/go-vulndb`. Real vulnerability, compilation, or analysis failures remain fail-closed.

Buf hosted code generation was also intermittently unavailable. `buf.gen.yaml` now invokes the same pinned `protoc-gen-go v1.36.12` and `protoc-gen-go-grpc v1.6.2` via local `go run`. A compatibility check confirmed zero generated protocol diff before this change, and the normal `pnpm provider-plugin:proto:check` plus full `pnpm check` now pass without hosted execution.

## Exit criteria

- [x] Agent Skills-compatible content is validated without execution.
- [x] Resource manifest/digest is deterministic and bounded.
- [x] Built-in `mender-tools` is represented in the Skill Library.
- [x] Managed local-directory installations are immutable and versionable side by side.
- [x] Trust and enablement are explicit, separate states.
- [x] CLI supports list/inspect/validate/install/lifecycle in JSON mode.
- [x] Local deployment adapters materialize exact installations safely.
- [x] Deployment drift is detectable.
- [x] Agent Plugin 1.0 export is deterministic.
- [x] Management/Web/Desktop can inspect and manage Skill resources without loading Skill code.
- [x] Existing Tool/Profile/Policy/Credential/Invocation behavior is unchanged.
- [x] `pnpm check` and `pnpm security:check` pass.
