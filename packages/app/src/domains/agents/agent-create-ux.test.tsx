import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { AgentCreatePage } from "./agent-create-page";

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

function show(path = "/agents/new?pluginId=io.mender.agent.opencode",
  createAgent = vi.fn().mockResolvedValue({ agentId: "agent_00000000000000000000000000000001" })) {
  const client = { createAgent } as unknown as MenderClient;
  render(<MemoryRouter initialEntries={[path]}><Routes>
    <Route path="/agents/new" element={<AgentCreatePage client={client} />} />
    <Route path="/agents/:agentId" element={<p>Created Agent</p>} />
  </Routes></MemoryRouter>);
  return createAgent;
}

describe("Agent creation form task focus and errors", () => {
  it("shows progress without repeated backend heading for generic and plugin-pinned creation", () => {
    show("/agents/new");
    expect(screen.getByText("Step 1 of 3")).toBeTruthy();
    expect(screen.getAllByText("Select backend")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: /OpenCode/ }));
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(screen.getByText("Step 2 of 3")).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Agent name" })).toBeTruthy();
  });

  it("explains invalid paths, keeps the optional model behind Advanced settings, and retains it through Review", () => {
    show();
    expect(screen.getByText("Step 1 of 2")).toBeTruthy();
    const advanced = screen.getByText("Advanced settings").closest("details");
    expect(advanced?.hasAttribute("open")).toBe(false);

    fireEvent.change(screen.getByRole("textbox", { name: "Agent name" }), { target: { value: "   " } });
    expect(screen.getByText("Enter an agent name, not just spaces.")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "Agent name" }), { target: { value: " Local Helper " } });
    fireEvent.change(screen.getByRole("textbox", { name: "Workspace directory" }), {
      target: { value: "relative/workspace" },
    });
    const root = screen.getByRole("textbox", { name: "Workspace directory" });
    expect(root.getAttribute("aria-invalid")).toBe("true");
    expect(root.getAttribute("aria-describedby")).toBe("agent-root-error");
    expect(screen.getByRole("alert").textContent).toContain("Enter an absolute workspace directory");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Review" }).disabled).toBe(true);

    fireEvent.change(root, { target: { value: "   " } });
    expect(screen.getByRole("alert").textContent).toContain("Enter an absolute workspace directory");
    fireEvent.change(root, { target: { value: "   /tmp/agent-folder   " } });
    expect(screen.queryByText("Enter an absolute workspace directory starting with /.")).toBeNull();
    fireEvent.click(advanced!.querySelector("summary")!);
    fireEvent.change(screen.getByRole("textbox", { name: "Default model (optional)" }), {
      target: { value: "  available-model  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(screen.getByText("Step 2 of 2")).toBeTruthy();
    expect(screen.getByText("Local Helper")).toBeTruthy();
    expect(screen.getByText("/tmp/agent-folder")).toBeTruthy();
    expect(screen.getByText("available-model")).toBeTruthy();
    expect(screen.getByText("Creating does not grant Profile access.")).toBeTruthy();

    fireEvent.click(screen.getAllByRole("button", { name: "Back" })[1]!);
    const reopened = screen.getByText("Advanced settings").closest("details");
    expect(reopened?.hasAttribute("open")).toBe(true);
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Default model (optional)" }).value)
      .toBe("  available-model  ");
  });

  it("keeps the review and all draft values after a create failure, then submits unchanged normalized data", async () => {
    const createAgent = vi.fn()
      .mockRejectedValueOnce(new Error("Cannot create Agent in this workspace"))
      .mockResolvedValue({ agentId: "agent_00000000000000000000000000000001" });
    show("/agents/new?pluginId=io.mender.agent.grok", createAgent);
    fireEvent.change(screen.getByRole("textbox", { name: "Agent name" }), { target: { value: "  Test Reviewer  " } });
    fireEvent.change(screen.getByRole("textbox", { name: "Workspace directory" }), { target: { value: "  /tmp/agent-test  " } });
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    fireEvent.click(screen.getByRole("button", { name: "Create agent" }));

    expect(await screen.findByRole("alert")).toHaveProperty("textContent", "Cannot create Agent in this workspace");
    expect(screen.getByText("Step 2 of 2")).toBeTruthy();
    expect(screen.getByText("Test Reviewer")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: "Back" })[1]!);
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Workspace directory" }).value)
      .toBe("  /tmp/agent-test  ");
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    fireEvent.click(screen.getByRole("button", { name: "Create agent" }));
    await waitFor(() => expect(createAgent).toHaveBeenCalledTimes(2));
    expect(createAgent).toHaveBeenLastCalledWith({
      name: "Test Reviewer", backend: "grok",
      workspaceRoot: "/tmp/agent-test", isolation: "restricted_host", defaultModel: "",
    });
    expect(await screen.findByText("Created Agent")).toBeTruthy();
  });

  it("shows Chinese progress and correctable field errors", () => {
    setLocale("zh-CN");
    show();
    expect(screen.getByText("第 1 / 2 步")).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox", { name: "工作目录" }), { target: { value: "abc" } });
    expect(screen.getByRole("alert").textContent).toContain("请输入以 / 开头的工作目录绝对路径");
  });
});
