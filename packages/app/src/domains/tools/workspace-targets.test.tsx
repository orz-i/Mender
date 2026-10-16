import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { MenderClient, Sandbox } from "@mender/sdk";

import { WorkspaceTargetSettings } from "./workspace-targets";

const defaultID = "sbx_00000000000000000000000000000001";
const alternateID = "sbx_00000000000000000000000000000002";
const providerID = "prov_00000000000000000000000000000001";

function localSandbox(id: string, name: string): Sandbox {
  return {
    sandboxId: id, name, backend: "local_workspace", isolation: "host",
    state: "ready", backendAvailable: true, activeLeases: 0,
    localRoot: "/tmp/" + name, ownerKind: "local_user", ownerRef: "owner",
  };
}

describe("Workspace Tool target management", () => {
  it("adds a target without cloning Tools and exposes an explicit schema refresh", async () => {
    const bindWorkspaceToolTarget = vi.fn().mockResolvedValue(undefined);
    const ensureSandboxTools = vi.fn().mockResolvedValue({ status: "ready", tools: [] });
    const listWorkspaceToolTargets = vi.fn().mockResolvedValue({ workspaces: [] });
    const client = {
      listWorkspaceToolTargets,
      listSandboxes: vi.fn().mockResolvedValue({
        sandboxes: [localSandbox(defaultID, "default"), localSandbox(alternateID, "alternate")],
      }),
      bindWorkspaceToolTarget,
      ensureSandboxTools,
    } as unknown as MenderClient;

    render(
      <MemoryRouter>
        <WorkspaceTargetSettings client={client} providerId={providerID} defaultSandboxId={defaultID} />
      </MemoryRouter>,
    );

    fireEvent.click(await screen.findByRole("button", { name: "Add workspace" }));
    fireEvent.change(screen.getByLabelText("Workspace key"), { target: { value: "project-b" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => {
      expect(bindWorkspaceToolTarget).toHaveBeenCalledWith(providerID, "project-b", alternateID);
    });

    fireEvent.click(await screen.findByRole("button", { name: "Refresh tools" }));
    await waitFor(() => expect(ensureSandboxTools).toHaveBeenCalledWith(defaultID));
    expect(listWorkspaceToolTargets).toHaveBeenCalledWith(providerID);
  });
});
