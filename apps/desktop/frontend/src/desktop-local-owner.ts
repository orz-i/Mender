import {
  MenderClientInvariantError,
  type AgentApproval,
  type AgentApprovalPolicy,
  type AgentToolPlacement,
  type LocalOwnerCapabilities,
  type PendingAgentApproval,
} from "@mender/sdk";

import {
  Service,
  type AgentApproval as BindingAgentApproval,
  type AgentApprovalPolicy as BindingAgentApprovalPolicy,
  type AgentPendingApproval as BindingAgentPendingApproval,
  type AgentToolPlacement as BindingAgentToolPlacement,
} from "../bindings/github.com/orz-i/Mender/backend/internal/desktop/binding";

export function createDesktopLocalOwnerCapabilities(): Required<LocalOwnerCapabilities> {
  return {
    approvals: {
      async listAgentApprovals(taskId) {
        const result = await Service.ListAgentApprovals(taskId);
        return { approvals: (result.approvals ?? []).map(agentApproval) };
      },
      async decideAgentApproval(approvalId, choice) {
        return agentApproval(await Service.DecideAgentApproval(approvalId, choice));
      },
      async listPendingAgentApprovals() {
        const result = await Service.ListPendingAgentApprovals();
        return { approvals: (result.approvals ?? []).map(pendingAgentApproval) };
      },
      async getAgentApprovalPolicy(agentId, key) {
        return agentApprovalPolicy(await Service.GetAgentApprovalPolicy(agentId, key));
      },
      async listAgentApprovalPolicies(agentId) {
        const result = await Service.ListAgentApprovalPolicies(agentId);
        return { policies: (result.policies ?? []).map(agentApprovalPolicy) };
      },
      async setAgentApprovalPolicy(agentId, key, mode) {
        return agentApprovalPolicy(await Service.SetAgentApprovalPolicy(agentId, key, mode));
      },
      async releasePendingAgentApprovals(agentId, key) {
        return Service.ReleasePendingAgentApprovals(agentId, key);
      },
    },
    toolPlacements: {
      async listAgentToolPlacements(agentId) {
        const result = await Service.ListAgentToolPlacements(agentId);
        return { placements: (result.placements ?? []).map(agentToolPlacement) };
      },
      async configureAgentToolPlacement(agentId, key, sandboxId, toolIds) {
        return agentToolPlacement(await Service.ConfigureAgentToolPlacement(agentId, key, sandboxId, toolIds));
      },
      async disableAgentToolPlacement(agentId, key) {
        await Service.DisableAgentToolPlacement(agentId, key);
      },
    },
  };
}

function agentApproval(value: BindingAgentApproval): AgentApproval {
  switch (value.status) {
    case "pending":
    case "allowed_once":
    case "rejected":
    case "expired":
      break;
    default:
      throw new MenderClientInvariantError(`Unknown Agent approval status: ${value.status}`);
  }
  switch (value.kind) {
    case "read":
    case "edit":
    case "execute":
    case "other":
      break;
    default:
      throw new MenderClientInvariantError(`Unknown Agent approval kind: ${value.kind}`);
  }
  const { decisionSource, ...fields } = value;
  if (decisionSource !== undefined && decisionSource !== "owner" && decisionSource !== "policy") {
    throw new MenderClientInvariantError(`Unknown Agent approval decision source: ${decisionSource}`);
  }
  return {
    ...fields,
    kind: value.kind,
    status: value.status,
    ...(decisionSource === undefined ? {} : { decisionSource }),
  };
}

function agentApprovalPolicy(value: BindingAgentApprovalPolicy): AgentApprovalPolicy {
  if (value.mode !== "manual" && value.mode !== "workspace_read" && value.mode !== "trusted") {
    throw new MenderClientInvariantError(`Unknown Agent approval mode: ${value.mode}`);
  }
  return { ...value, mode: value.mode };
}

function pendingAgentApproval(value: BindingAgentPendingApproval): PendingAgentApproval {
  return { ...value, approval: agentApproval(value.approval) };
}

function agentToolPlacement(value: BindingAgentToolPlacement): AgentToolPlacement {
  return { ...value, toolIds: value.toolIds ?? [] };
}
