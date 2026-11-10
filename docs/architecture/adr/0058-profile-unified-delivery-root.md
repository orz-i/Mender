# ADR-0058 — Profile as a unified capability bundle and delivery root

- Status: Accepted
- Date: 2026-10-23
- Extends: [ADR-0004](0004-runtime-modes-and-mcp.md), [ADR-0008](0008-mcp-adapter.md), [ADR-0016](0016-managed-downstream-mcp-servers.md), [ADR-0029](0029-skill-resource-management.md), [ADR-0031](0031-managed-profile-tunnels.md), [ADR-0039](0039-offerings-resource-tool-suites.md), [ADR-0055](0055-managed-mcp-profile-hot-refresh.md)
- Design and migration: Profile unified delivery plan (historical)

## Context and verified baseline

ADR-0004 originally calls for one application core deployed locally (direct CLI / MCP stdio / Agent Skills) or remotely (Profile-scoped Streamable HTTP), with Profile as the exposure/security boundary. Today the user completes the same task across unrelated surfaces:

- `Profile` persists exact ToolBindings, MCP direct/dynamic/hybrid mode, optional TunnelID and revision. It has no Skill membership.
- Runtime-managed MCP HTTP Server instances are *ephemeral*, kept under `bootstrap.Runtime`. The separate `/servers` UI starts/stops a server for one Profile. Client-launched MCP stdio is already `mender serve mcp --profile ... --stdio`.
- Skills are canonical immutable installations with exact digest, independent trust/enablement and target-specific deployment/drift records. `DeploySkill` acts on one SkillInstallationID, with no ProfileID.
- Agent Plugin exporter packages an arbitrary set of trusted/enabled Skills into `plugin.json` plus `skills/`; it is not yet Profile-scoped, and it does not emit MCP configuration.
- CLI `tool search` can optionally scope to a Profile, `tool call` can run in owner Catalog scope if `--profile` is omitted, and `tool describe` is unscoped. These owner operations must not be copied directly into a published Profile CLI interface.
- Profile list/detail/edit are now separate routes. Skills and Servers remain separate navigation groups.

This splits an intended *Profile bundle* among three ownership surfaces; simply moving buttons would leave authorization, exact-version tracking, target lifecycle and UX inconsistent.

## Decision

### 1. Profile owns the selection, not the foreign resources or processes

**Profile is the canonical bounded capability bundle and downstream exposure scope.** It selects exact canonical ToolIDs with existing binding constraints, plus exact immutable Skill InstallationIDs (with digest/version projection). Skill library, Catalog, Credential, Policy, Sandbox, Agent and Tunnel remain separate authorities. Source/Provider grouping remains a read-only view; selecting a source never implicitly authorizes all Tools.

Skill membership is independent from Tool membership: a Skill is a workflow/instruction/resource, not an executable Tool or a Tool authorization. A Profile may contain Tools only, Skills only, or both. Adding, removing, upgrading or revoking a Skill must be explicit, checked against trust/enablement, revisioned and audited. No automatically pinned `latest` and no content rewriting of installed Skills.

Profile revision identifies configured membership and policy references; presentation-specific MCP mode and Tunnel binding are *delivery settings*, not universal properties of a Skill/CLI bundle. Migrating existing Profile `mode`/`tunnel_id` to MCP delivery ownership requires a bounded, tested migration, not a compatibility bridge or a premature rename. Existing IDs, ToolBinding overrides and live endpoints are preserved during rollout.

### 2. Deployment/Delivery is a Profile-owned *relationship* with a channel adapter

Each delivery identifies `ProfileID`, target channel, explicit destination/scope, selected exact configuration revision plus pinned Skill digests (where materialized), and lifecycle/health suitable for that channel. One Profile can have zero or many deliveries. A delivery can never include a Tool or Skill outside the selected Profile. It must not silently expand execution scope when Profile membership changes.

Channel adapters:
- **MCP**: client-launched stdio (local configuration, no detached server toggle); Runtime-owned local loopback Streamable HTTP server; remote reachability through an explicitly configured Tunnel or authenticated remotely hosted daemon. Preserve ADR-0055 HTTP hot-refresh on next request. Do not equate a tunnel process starting with external readiness.
- **Local Agent Skills**: materialize *only Profile-selected exact installations* into explicit project/user host locations. Reuse Skill validation, trust, path confinement, digest and drift logic. Multi-Skill deployment requires checked batch compensation/atomic staging semantics before claiming all-or-nothing.
- **Local CLI**: distribute a Profile-bound invocation recipe/wrapper against the *existing CLI/runtime*. Search, Describe and Call must be scoped to the selected Profile, and actual execution must traverse existing Governance, Policy and Audit. Do not require a daemon for local CLI or introduce a second Tool executor. Administrator Catalog CLI remains a separate host/owner function.
- **Agent Plugin**: export portable Agent Plugins 1.0 containing exactly the Profile's enabled/trusted Skills and, when explicitly selected/representable, Profile-scoped MCP connectivity. Support skills-only, MCP-only or combined packages if the target format allows them. Use `plugin.json`, `skills/` and optional `mcp.json` conforming to the portable schema. A client-specific adapter (e.g. Claude) must be an explicit target rather than claimed interoperable merely because it is called a plugin. Do not conflate this downstream package with Mender inbound Provider plugins.

A delivery may have different observed state classes: managed HTTP listener/tunnel running, materialized Skill files current/drifted/missing, CLI configuration generated/installed, or Agent Plugin artifact exported. Do **not** flatten those into one fake "running" status. Profile "configured" is not "deployed", and "deployed" is not "healthy/connected".

### 3. Local vs remote is an orthogonal runtime/location dimension

Keep ADR-0004's one application core, not two runtimes. Deployment target host (current machine / explicit local project / remote managed host) and delivery interface (MCP/Skill/CLI/Plugin) are separate concepts. Mender on a remote daemon cannot assume arbitrary access to the *browser operator's* local user/project directories. Remote Skill/CLI installation requires an explicit authorized target-side adapter; a remote MCP URL by itself cannot install local files.

Remote public MCP needs a separate trusted HTTPS ingress and access control. A private Secure MCP Tunnel is not the same as a public stable HTTPS endpoint usable for public plugin submission. Loopback guards must not be weakened to make a new UI option work.

### 4. Profile is the sole user-facing delivery entry

Navigation keeps **Profiles**, **Skills** and **Tools**. Skills remains a library/inventory: inspect, import, trust, enable/disable, references/usage. It no longer offers a standalone Deploy action after Profile-based delivery is active. Profile detail gains **Overview / Tools / Skills / Deliveries** (Chinese equivalents), and one **Add delivery** flow that asks for the delivery format then only the required target settings. Individual Server/Tunnel runtime diagnostics remain reachable from the related MCP delivery; no second standalone server lifecycle owner.

Creation is staged: create Profile identity → add Tools/Skills → explicitly choose delivery. Do not require new users to understand MCP modes, Tunnel or host filesystem paths at Profile creation. Read-only detail stays distinct from mutation. Confirm the exact project/user filesystem target before writing. A single primary action per surface; avoid nested dialogs and verbose internal IDs.

### 5. Security, reliability, and lifecycle invariants

1. Existing Profile ToolID, provider ownership, Policy, Credential, approval, Sandbox grants and Invocation checks remain authoritative for every actual Tool call, whatever downstream adapter.
2. Skill trust/enablement is not execution authorization. Skill scripts may execute in an Agent host outside Mender's invocation control; never present them as fully Mender-governed calls.
3. No automatic Profile grant, Skill addition, latest-version activation, remote authorization or public exposure on installing a resource, opening a library page, or changing a delivery format.
4. Recorded delivery targets are explicit. Bound checks prevent traversal/symlink escape, secret embedding, name collision and unauthorized cross-profile substitution.
5. Skill/Plugin packages pin exact installation hashes and Profile revision. After Profile changes, previously materialized/exported packages become **out of date** pending explicit republish; no silent rewrite. Running managed MCP HTTP alone may pick up revised Tools on the next request per ADR-0055.
6. Keep runtime-observed health distinct from desired deployment configuration. Persisting desired auto-start state or reconciling a lost process must be a separately verified lifecycle change, not an incidental consequence of a UI move.
7. Avoid both a second generic Provider/Tool/Supply registry and a Profile-aware Agent Loop. Profile has no Agent Session/Task execution state.
8. Existing unassociated Skill deployments are not silently assigned to a Profile. Inspect/adopt explicitly or leave them alone until a documented, reversible owner migration. Avoid deleting user-installed files on mere UI navigation changes.
9. HTTP/CLI/Wails share core application use cases and authorization. A frontend-only association never claims that a deployment is governed or scoped.

## Alternatives considered

- **Keep Servers, Skill Deployment, and Agent Plugin export separate; add cross-links.** Least implementation work, but resource-selection scope and delivery ownership remain ambiguous.
- **Move every Skill installation and MCP Server runtime into the Profile aggregate.** Rejected: Profile would take over untrusted content storage, host-specific filesystem/process/network lifecycles and vendor transport configuration. Profile owns selection/relationships; domain-specific adapters retain execution responsibility.
- **Use MCP as the universal outbound format.** Rejected: local CLI and Agent Skills require no MCP daemon, and remote MCP cannot deploy local content.
- **Use Agent Plugin packages as Profile persistence.** Rejected: an external portable package is a delivery artifact, not Mender's internal authority model.
- **Single global deployment-type enum, one state machine.** Rejected: process readiness, file drift and artifact export have distinct evidence and recovery behavior.
- **Auto-attach all already-installed Skills to default Profile.** Rejected: silent capability expansion and opaque existing deployments.

## Consequences

- Positive: one clear "assemble → deliver → inspect" user journey; exact membership and authorization boundaries; reuse of current MCP/CLI/Skill infrastructure; local and remote runtime modes remain first class; transport-specific states remain truthful.
- Costs: new Profile-Skill membership and delivery association/query contracts; an explicit migration for legacy Skill deployment records; scoped CLI descriptions/search/call; target-specific export validation; frontend route/tests reorganization; remote deployment capability gaps stay explicit.
- This decision does **not** authorize automatic changes to production Profiles, Server/Tunnel processes, Trust, or host Skill directories. ADR-0016's ephemeral HTTP semantics remain in force until a later, separately accepted lifecycle change.

## Implementation status (2026-10-24)

The decision is accepted and implementation is **partial, not production-validated**:

- `dc3bde5`: Profile pins exact SkillInstallation IDs with trust/enablement and DB reference guards.
- `eedbbc3`: Profile owns host Skill materialization/undeploy; historical unowned Skill Deployments remain unclaimed, visible and removable.
- `5c43d76`: Profile-scoped MCP HTTP control and status UI; no persistent daemon auto-start change.
- Stage E: Profile-only CLI commands/owner-local recipes and portable Agent Plugins 1.0 package export (Skills-only, HTTPS MCP-only or combined). Local paths and host actions remain Desktop-only.
- **Deliberate limitations:** exported package and local CLI recipe are snapshots, not persistent delivery records with fleet-wide health, automatic artifact drift/reconciliation or client-side installation. User-provided remote MCP URL is validated structurally but **not verified as a Profile-scoped MCP server**. Agent-side scripts outside Mender Tool Invocation are not fully Mender-governed. Remote HTTPS ingress/authentication, host-specific remote Skill installation, native Wails runtime and actual consumer-client loading require Phase F acceptance evidence.

## External references (checked 2026-10-24)

- https://agentskills.io/specification — immutable directory format and progressive disclosure.
- https://agent-plugins.org/specification — portable manifest, skills/ and optional mcp.json.
- https://developers.openai.com/plugins/build/plugins — portable plugin packaging and product-specific extensions.
- https://modelcontextprotocol.io/specification/2026-07-28/basic/transports — stdio and Streamable HTTP.
- https://developers.openai.com/api/docs/guides/secure-mcp-tunnels — private tunnel scope and public plugin submission distinction.
