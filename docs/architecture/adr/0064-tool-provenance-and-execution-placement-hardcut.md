# ADR-0064: Separate Tool provenance, Agent-as-Tool ownership, Workspace context, and Sandbox execution placement

- **Status:** Accepted as target architecture; stages B–H are **not implemented**
- **Date:** 2026-11-07 (Mac host)
- **Implementation plan:** [Hard-cut plan](../../planning/tool-workspace-sandbox-agent-hardcut-2026-11-07.md)
- **Phase A evidence:** [Ownership inventory](../../planning/tool-hardcut-phase-a-inventory-2026-11-07.md) · [Migration / dry-run contract](../../planning/tool-hardcut-phase-a-migration-contract-2026-11-07.md)
- **Refines or partially supersedes:** ADR-0003, ADR-0030, ADR-0038, ADR-0039, ADR-0046, ADR-0047, ADR-0049, ADR-0052, ADR-0056, ADR-0058, ADR-0061. This ADR replaces the prior **mandatory Provider parent** and **sourceId == ProviderID** assumptions, the **per-Sandbox Tool Suite Provider** as an enduring runtime identity, and Provider-keyed Workspace target aliases. Other security and lifecycle decisions remain in effect.

## Context

Mender is a tool/resource aggregation, governance, and supply platform for external LLMs and agents. It **does not run the external caller's Agent Loop**. Configured Mender Agents are **Agent as Tool**: callable, individually owned delegated capabilities backed by agent-native executors and Task/Session lifecycles, not generic execution environments.

The present canonical Tool model requires a ProviderID (`catalog/domain/tool.go` and `tools.provider_id`). A configured Agent is exposed through an internal Agent Delegation Provider. A Sandbox may be exposed through separate Workspace and SSH Jobs Providers. The Catalog, Profile source picker, MCP discovery and direct tool names rely on those ProviderIDs. Workspace selection is separately represented by Agent workspace paths, Provider-keyed Workspace target aliases and Sandbox roots. The same underlying project can therefore be registered several times; users see an Agent/Sandbox both as a resource and as a second Tool Service.

These facades were deliberate, audited intermediates, not random dead code. In particular, they enforce exact ownership, Profile ToolBinding and permission boundaries. Removing them without an equivalent identity and authorization model would silently widen access when several per-Sandbox ToolIDs collapse into one canonical operation.

## Decision

### 1. Keep one Tool Catalog; replace mandatory Provider ancestry with a typed provenance

A Tool remains a canonical, independently executable `ToolID` with one `SourceKey` and schemas. It owns **callable operation metadata**, not a Sandbox or workspace path.

The Catalog stores a required, verified `SourceRef {kind, id}` whose supported kinds are:

| Source kind | Identity | Responsibility |
| --- | --- | --- |
| `provider` | a real configured ProviderID | Connection, discovery, Plugin binding and independent upstream Tool execution |
| `agent` | exact AgentID | One Agent-as-Tool resource's delegated operations; each Agent has its own ToolIDs |
| `native` | host-pinned key such as `workspace` or `jobs` | Reviewed canonical Workspace/Job operations implemented by Mender |

SourceRef is a **typed value and read-only discovery projection**, not a Source CRUD aggregate or independent permission. The host validates source ownership and availability. A Provider is **not** the parent of `agent` or `native` Tools. Do not fabricate an internal Provider to satisfy Catalog or export-name construction.

A plugin's Extension/Installation is an implementation pin and does not imply configured resource ownership, Tool selection or authorization. External Provider instances and their ToolIDs retain their identity. Agent-owned ToolIDs are preserved only when their old 1:1 ownership and source-key mapping are verified. For the many-to-one collapse of Sandbox Workspace/Jobs Tools, assign **new canonical native ToolIDs** and revoke old runtime invocation IDs: never let one old Sandbox-specific ToolID silently become a generic global capability.

### 2. Define Workspace as context, Sandbox as placement

- A WorkspaceRef identifies an **approved logical work target**, with a verified owner and target-specific root or reference. It is not a mount, a process or proof of isolation.
- Local, Docker, SSH and external Sandbox roots remain the **real enforcement domain** of each backend. A Workspace placement relation records the exact verified Workspace/Sandbox association; two equal-looking paths or aliases are not evidence of equivalent content or permission.
- Sandbox owns backend binding, capability negotiation, isolation, run state, generation, admission, durable lease and cleanup. It **does not own** the definition or catalog identity of generic `workspace.read`, `workspace.search`, `workspace.edit` or `workspace.shell`.
- Reuse existing typed Workspace application and Sandbox Router `Resolve/Acquire`. No second execution engine, generic scheduler, hidden provisioning, arbitrary mount, remote synchronization or fall back to Host is introduced.
- A Workspace can be referenced by several Agent instances and native Tools, but Agent workspace authorization and Sandbox acquisition authorization remain distinct. A normal local Agent may use an approved cwd **without** provisioning a Sandbox.
- Workspace is initially a domain identity plus shared picker/management relationships, **not** necessarily a global navigation item.

### 3. Deterministic selection before any side effects

Only capability-bearing Mender-native operations use Workspace/Sandbox placement. External MCP/OpenAPI/Model Tools continue through their Provider execution and Provider Activation gates.

For Workspace operations, choose an explicit, authorized WorkspaceRef from the call/scope or owner-approved default; auto-select only when exactly one eligible Workspace exists. Then filter **registered** Sandbox placements by verified capability/contract version, isolation, ownership, grants, ready state and exact run/backend identity. Select the pinned/default target or a **unique eligible** target. Ambiguity, missing placement, unsupported capability or stale generation **fails closed**. No model-provided filesystem path, raw SandboxID, alias coincidence or unverified Tool annotation grants access.

Before Policy, Invocation records reviewed resource/effect facts with exact intended Tool/Workspace/Sandbox identities. Policy/Approval and budget admission precede backend acquisition or Credential resolution. Recheck identity, generation, grants and pinned backend after approval, before reserving a lease. Invoke only the originally approved target; do not retry a mutating operation on another environment after timeout or uncertain result. No silent security-class downgrade.

### 4. Agent as Tool remains a separate execution lifetime

An Agent Tool source is the exact AgentID, not a Provider or Sandbox. Continue the current `delegate/status/result/cancel/resume` operation family where supported. The UI presents one selectable Agent capability with its task-control actions, not a separately managed Provider service.

Agent Task/Session, ACP v1, native approval, workspace root, grants, permission policy, immutable attempt evidence, `unknown` and resume retain Agent ownership. Mender never adopts the subordinate Agent Loop. Binding an Agent to a Workspace or a Sandbox does **not** assert that the ACP child process is isolated there. Governed tool delivery into an Agent is separately admitted per Tool invocation; a future true remote ACP process placement requires actual backend process-stream, isolation and lifecycle contracts.

### 5. Capability selection and target authority are orthogonal

A Profile ToolBinding grants **only the named operation** to the Profile (subject to global Policy, approval, and its tightening constraints). It does not grant access to a Workspace or Sandbox. For native Tools that replace per-Sandbox Tools, the Profile ToolBinding must additionally carry a **restrictive, explicitly authorized target scope** limiting exactly which legacy targets that Profile could invoke. This is a ToolBinding restriction, **not** a new global authorization/Source registry. Runtime checks are conjunctive:

`ToolBinding + binding target scope + Workspace grant + Sandbox grant/acquire + Policy/Approval + Runtime Budget`.

Agent calls additionally require AgentGrant and the correct Agent Workspace grant; task-control calls revalidate their own Task ownership and caller. A new workspace, plugin, source, sandbox or Tool never creates grants by itself. Explicit owner Catalog-scope execution remains subject to its existing owner authority and target admission; it cannot infer a new remote placement by name.

When old per-Sandbox ToolBindings have differing aliases, delivery exposure, policies, concurrency or Credential restrictions, one new ToolBinding cannot always represent them: the **migration must block and require explicit, auditable conflict resolution** rather than choose a permissive union. New target scope must be enforced on every invocation, not only during discovery or profile setup.

### 6. Source-first discovery remains; the source identity changes

Preserve the grouping and progressive `sources → tools → describe → invoke` experience from ADR-0056 and exact Profile restriction. Replace its `sourceId=providerId` assertion with a stable encoded typed SourceRef (e.g., `agent:agent_...`, `native:workspace`, `provider:prov_...`; exact serialization fixed in contract tests). The opaque encoding does not supply authorization. Profile-scoped search must not disclose unbound sources.

For Direct/Hybrid MCP, exported **names, title and descriptions** are presentation decisions separate from provenance; collisions must be detected and rejected, never silently shadowed. Provider namespaces can still be used for real Providers but no longer synthesize internal Agent/Sandbox Providers. Old external names may change for collapsed tools; enumerate those differences in dry-run manifest and update clients/exported configuration atomically at cutover. Retain old external Provider tool behavior.

### 7. One forward-only migration release, no runtime compatibility path

Implement and test a **read-only, deterministic dry-run** before permitting the actual cut. Cross-context tools, Profile/MCP delivery, Policy, Agent, Sandbox and Workspaces share one local SQLite file but have separate application/domain ownership. A narrowly scoped **infrastructure-only migration coordinator** must run after a maintenance/lock boundary and before new Runtime resources start; it commits all cross-context identity, references, triggers, FTS and schema version updates as one atomic transaction. Independent context migrations may still create brand-new databases, but must not perform half-upgrades of existing populated legacy stores.

Require a verified consistent backup, dry-run with zero unresolved blockers, no concurrent old writers, `foreign_key_check`, `integrity_check`, key/count/revision comparisons and an actual recovery rehearsal. On failure, roll back. Old binaries fail closed on future schemas; rollback means **stop and restore pre-upgrade whole-database backup**, not dual-read/downgrade.

Remove old facade Provider rows, config/route management, Provider-keyed workspace targets, API routes and UI controls from the **final running code**. Keep immutable historical audit and Agent/SSH job evidence, with an audit-only legacy identity lookup where necessary. This lookup must never permit legacy ToolID invocation. Never replay unknown Agent Tasks or possibly executed SSH Jobs during migration.

## Invariants and acceptance

1. No Provider requirement for `agent` or `native` Catalog Tools. Every Tool has exactly one verified provenance, and old facade rows are absent after upgrade.
2. Mender-native Workspace/Jobs capability appears once per canonical operation, not once per Sandbox. Sandboxes advertise execution contracts and acquire leases rather than publishing duplicate generic Tool schemas.
3. Every new permitted action/target combination is demonstrably allowed by **no more principals and no more effective authority** than before the hard cut. If the comparison cannot be proved, block before mutation.
4. Profile ToolBinding, owner scope, Agent/Workspace/Sandbox grants, global Policy, Credential References, approvals, budget, lease, result guarding and Audit remain enforced at their owning boundaries.
5. No auto-provision, implicit grants, non-equivalent Workspace placement, Host fallback, unknown task replay or backend substitution.
6. MCP source-first and Direct/Hybrid, CLI, SDK, HTTP, Wails, i18n and native GUI agree on the new identities and supply behavior. Historical diagnostics remain interpretable without executable legacy bridges.
7. End-to-end disposable DB tests must include: two Agents on one Backend; two Sandboxes sharing a logical project but with distinct roots/permissions; per-target Profile constraints and revocation; SSH durable job recovery; uncertain Agent status; MCP name conflicts; external Provider regression; crash/rollback and restart idempotence.

## Alternatives considered

1. **Only reorganize UI, hide managed Providers:** cheap and low risk, but leaves Provider-centric source identity, duplicated Workspace aliases and internal lifecycle coupling.
2. **Keep one Tool Suite Provider per Sandbox indefinitely:** preserves old ToolIDs but violates the capability/placement separation and multiplies generic Tools as sandboxes grow.
3. **Give one native Workspace Tool unrestricted selection over all eligible Sandboxes:** appears simple but widens old Profile rights when a different Sandbox had a grant but no corresponding ToolBinding; rejected.
4. **Introduce a generic Resource/Source/ToolInstance hierarchy and global scheduler:** supports arbitrary placements but duplicates existing context ownership and governance; rejected.
5. **Hard-delete old ToolIDs and reselect Profiles automatically:** destroys explicit user authorization, MCP delivery names and historical evidence; rejected.
6. **Use ACP Agent as Tool as a generic Sandbox runtime:** confuses a delegated reasoning engine with an execution environment and wrongly claims isolation; rejected.

## Consequences and follow-up

- Positive: capability-oriented Tool Catalog, reusable Workspaces, fewer technical resource clones, truthful Agent-as-Tool and Sandbox boundaries, reuse of security/runtime infrastructure.
- Cost: breaking Catalog source and native Tool identity migrations; a strict target-limited ToolBinding; SourceRef-aware discovery/name generation and Policy matchers; coordinated cross-context SQL migration; customer-facing MCP export changes and separate Wails acceptance.
- Not complete: adoption of an explicit Workspace identity, target-scope representation and possible conflicting Profile presentations still require implementation. They cannot be “solved” through a UI rename or a migration default.
- Stage A freezes the **target contract**, not the current Runtime. Subsequent B–H code changes and deployment must obey the attached inventory/manifest contract. Do not claim runtime acceptance until tested on disposable DB and real native Wails.

## Protocol and storage references (verified 2026-11)

- [MCP 2026-07-28 Tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools): tool name/schema, list/call and explicit authority/approval expectations; MCP Tool annotations do not create Mender execution authority.
- [ACP v1 Session Setup](https://agentclientprotocol.com/protocol/v1/session-setup): session working directory and external MCP server configuration; cwd is not a verified Mender Sandbox process placement.
- [OpenAI Agents SDK multi-agent patterns](https://openai.github.io/openai-agents-python/multi_agent/): Agent as Tool retains control with the calling manager; it is distinct from a handoff and does not imply Mender hosts the calling Agent Loop.
- [SQLite schema alteration](https://sqlite.org/lang_altertable.html), [online backup API](https://sqlite.org/backup.html): forward-only table rebuild, foreign-key validation and recoverable backups.