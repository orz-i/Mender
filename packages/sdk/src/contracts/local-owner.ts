/** Local Desktop owner configuration for governed Agent Tools. */
export interface AgentToolPlacement {
  agentId: string;
  workspaceKey: string;
  workspaceRoot: string;
  sandboxId: string;
  toolIds: string[];
  updatedAt: string;
}

/** A native Agent permission request visible to the Desktop owner. */
export interface AgentApproval {
  approvalId: string;
  taskId: string;
  agentId: string;
  kind: "read" | "edit" | "execute" | "other";
  target?: string;
  canAllow: boolean;
  status: "pending" | "allowed_once" | "rejected" | "expired";
  decisionSource?: "owner" | "policy";
  createdAt: string;
  expiresAt: string;
  resolvedAt?: string;
}

export type AgentApprovalMode = "manual" | "workspace_read" | "trusted";

export interface AgentApprovalPolicy {
  agentId: string;
  workspaceKey: string;
  mode: AgentApprovalMode;
  expiresAt?: string;
  updatedAt?: string;
}

export interface PendingAgentApproval {
  approval: AgentApproval;
  agentName: string;
  workspaceKey: string;
}

export interface AgentApprovalCapability {
  listAgentApprovals(taskId: string): Promise<{ approvals: AgentApproval[] }>;
  decideAgentApproval(approvalId: string, choice: "allow_once" | "reject"): Promise<AgentApproval>;
  listPendingAgentApprovals(): Promise<{ approvals: PendingAgentApproval[] }>;
  getAgentApprovalPolicy(agentId: string, workspaceKey: string): Promise<AgentApprovalPolicy>;
  listAgentApprovalPolicies(agentId: string): Promise<{ policies: AgentApprovalPolicy[] }>;
  setAgentApprovalPolicy(agentId: string, workspaceKey: string, mode: AgentApprovalMode): Promise<AgentApprovalPolicy>;
  releasePendingAgentApprovals(agentId: string, workspaceKey: string): Promise<{ released: number }>;
}

export interface AgentToolPlacementCapability {
  listAgentToolPlacements(agentId: string): Promise<{ placements: AgentToolPlacement[] }>;
  configureAgentToolPlacement(
    agentId: string,
    workspaceKey: string,
    sandboxId: string,
    toolIds: string[],
  ): Promise<AgentToolPlacement>;
  disableAgentToolPlacement(agentId: string, workspaceKey: string): Promise<void>;
}

/** Host-supplied features. Every operation still requires backend owner authorization. */
export interface LocalOwnerCapabilities {
  readonly approvals?: AgentApprovalCapability;
  readonly toolPlacements?: AgentToolPlacementCapability;
}

/** Inputs and results for host-local Profile artifact delivery. */
export interface ExportProfilePluginInput {
  name: string;
  version: string;
  description: string;
  remoteMCPURL: string;
}

export interface ProfileAgentPluginExport {
  profileSlug: string;
  profileRevision: number;
  destination: string;
  name: string;
  hasMCP: boolean;
}

export interface ProfileCLIInstructions {
  profileSlug: string;
  databasePath: string;
  platform: string;
  search: string[];
  describe: string[];
  call: string[];
  mcpStdio: string[];
}
