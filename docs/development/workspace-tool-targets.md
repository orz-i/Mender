# Reusing Sandbox Workspace Tools across approved targets

Phase 40 adds optional target selection without creating a second Workspace Tool service. This guide complements [ADR-0047](../architecture/adr/0047-agent-runtime-configuration-workspace-reuse.md) and [the Phase 40 acceptance contract](../planning/phase-40-agent-runtime-configuration-workspace-reuse.md).

## User-visible flow

1. Create or choose a Sandbox, open its **Tools** section and explicitly prepare the Workspace Tool Suite. Mender creates one managed service with four stable canonical Tools: `read`, `search`, `edit`, `shell`.
2. In **Tools**, select the service and choose **Manage workspaces**. The original Sandbox remains the default; **Add workspace** chooses an existing, ready, compatible Sandbox and gives it a unique lower-case alias such as `project-b`.
3. Select **Refresh tools** to publish the optional `workspace` input for existing synchronized ToolIDs. No new ProviderID, ToolID or Profile ToolBinding is created.
4. From the chosen Sandbox detail, explicitly grant the relevant Profile its allowed `read/search/edit/shell` capabilities. Each Sandbox has separate capability grants and `sandbox.acquire` admission. The Profile still needs its normal ToolBinding and `tool.invoke` Policy.

The caller chooses an *alias*, not a raw Sandbox ID or host directory. Omitting `workspace` always targets the original pinned Sandbox. For example:

```json
{"path":"README.md"}
```

```json
{"workspace":"project-b","path":"README.md"}
```

The same `workspace` selector can be added to `search`, `edit`, and `shell` input. Their remaining arguments and output schemas are unchanged. The selector is stripped before passing arguments to the Sandbox backend.

## CLI / management

```sh
mender sandbox tools <default-sandbox-id> --prepare
mender sandbox targets bind <tool-service-provider-id> project-b <target-sandbox-id>
mender sandbox targets list <tool-service-provider-id>
mender sandbox targets unbind <tool-service-provider-id> project-b
```

The same owner-only operations are supported through loopback management HTTP, TypeScript SDK and Wails/Desktop. Registration does **not** publish new Tools or Profile permissions. Unbind only blocks future selection and does not silently alter the target Sandbox grant inventory.

## Governance and limits

- Alias lookup requires the exact managed Workspace service. An unrelated Provider cannot use its aliases. Backend mismatch, incompatible capability contracts, missing/retired Sandboxes and owner mismatch fail closed.
- Policy sees a reviewed `sandbox_id` fact *and* the relative `workspace_path` fact. The executor verifies that the Sandbox it would acquire is the exact target approved at Policy preflight. If a target changes between preflight and execution, the call is rejected **before** Sandbox acquisition.
- The Sandbox Router still independently checks Profile grants per requested capability, run generation, lifecycle readiness, Policy and durable execution lease. A Profile permitted in one Sandbox cannot use that grant in another.
- An additional active target blocks retirement of the owning Workspace Tool Suite or removal of the referenced Sandbox until it is explicitly detached.
- This is Tool execution on a selected Sandbox, **not** Agent/LLM workspace switching, session orchestration or mounting an arbitrary directory. Docker, SSH, browser or remote support continues to depend on that exact Sandbox backend's verified Workspace contracts; there is no host fallback.
- The saved alias-to-Sandbox binding is user administration; no backend process, credentials or external model are launched by discovery or listing.
