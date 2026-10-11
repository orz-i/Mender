import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MenderClient, Sandbox } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CreateSandboxPage } from "./sandboxes-page";

afterEach(cleanup);

const cachedImage = `docker.io/library/alpine@sha256:${"a".repeat(64)}`;
const created: Sandbox = {
  sandboxId: "sbx_0123456789abcdef0123456789abcdef",
  name: "Safe Container",
  backend: "docker",
  isolation: "container",
  state: "ready",
  localRoot: "",
  ownerKind: "local_user",
  ownerRef: "owner",
  activeLeases: 0,
  backendAvailable: true,
};

function showCreate(client: Partial<MenderClient>) {
  const passed: Partial<MenderClient> = {
    listCredentials: vi.fn().mockResolvedValue({credentials: []}),
    listDockerImages: vi.fn().mockResolvedValue({images:[{
      imageDigest:cachedImage, displayName:"alpine:3.22",
    }]}),
    ...client,
  };
  render(
    <MemoryRouter initialEntries={["/sandboxes/new"]}>
      <Routes>
        <Route path="/sandboxes/new" element={<CreateSandboxPage client={passed as MenderClient} />} />
        <Route path="/sandboxes/:sandboxId" element={<div>Sandbox detail route reached</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

function runtimeGovernanceStub() {
  return { backend: { restrictedHostAvailable: false } };
}

describe("Docker Sandbox creation UX", () => {
  it("never offers Docker creation when the host Engine is not verified", async () => {
    const createDockerSandbox = vi.fn();
    showCreate({
      getRuntimeGovernance: vi.fn().mockResolvedValue(runtimeGovernanceStub()),
      getSandboxBackends: vi.fn().mockResolvedValue({
        dockerAvailable: false,
        dockerStatus: "Docker Engine socket has not been configured",
      }),
      createDockerSandbox,
    });
    expect(await screen.findByText(/Docker Engine socket has not been configured/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Docker/ })).toBeNull();
    expect(createDockerSandbox).not.toHaveBeenCalled();
  });

  it("automatically unlocks Docker when the local host discovers the Engine later", async () => {
    const getSandboxBackends=vi.fn()
      .mockResolvedValueOnce({dockerAvailable:false,dockerStatus:"Docker Engine not detected"})
      .mockResolvedValue({dockerAvailable:true,dockerStatus:"ready"});
    let tick:(()=>void)|undefined;
    const original=window.setInterval.bind(window);
    const spy=vi.spyOn(window,"setInterval").mockImplementation((handler,delay)=>{
      if(delay===5000 && typeof handler==="function"){
        tick=handler as ()=>void;
        return -1;
      }
      return original(handler,delay);
    });
    try {
      showCreate({
        getRuntimeGovernance: vi.fn().mockResolvedValue(runtimeGovernanceStub()),
        getSandboxBackends,
      });
      expect(await screen.findByText("Docker Engine not detected")).toBeTruthy();
      expect(screen.queryByRole("button",{name:/Docker/})).toBeNull();
      if(!tick)throw new Error("Docker backend auto-refresh timer missing");
      await act(async()=>{tick?.();await Promise.resolve();});
      expect(screen.getByRole("button",{name:/Docker/})).toBeTruthy();
      expect(getSandboxBackends).toHaveBeenCalledTimes(2);
    } finally {
      spy.mockRestore();
    }
  });

  it("disables a Docker create form on Engine loss and re-enables on same-Engine recovery",async()=>{
    const getSandboxBackends=vi.fn()
      .mockResolvedValueOnce({dockerAvailable:true,dockerStatus:"ready"})
      .mockResolvedValueOnce({dockerAvailable:false,dockerStatus:"Docker Engine disconnected or insecure; retrying"})
      .mockResolvedValue({dockerAvailable:true,dockerStatus:"ready"});
    let tick:(()=>void)|undefined;
    const original=window.setInterval.bind(window);
    const spy=vi.spyOn(window,"setInterval").mockImplementation((handler,delay)=>{
      if(delay===5000 && typeof handler==="function"){
        tick=handler as ()=>void;
        return -1;
      }
      return original(handler,delay);
    });
    try {
      showCreate({
        getRuntimeGovernance:vi.fn().mockResolvedValue(runtimeGovernanceStub()),
        getSandboxBackends,
      });
      fireEvent.click(await screen.findByRole("button",{name:/Docker/}));
      await screen.findByRole("option",{name:/alpine:3.22/});
      fireEvent.change(screen.getByRole("textbox",{name:"Sandbox name"}),{target:{value:"Guarded Docker"}});
      expect(screen.getByRole<HTMLButtonElement>("button",{name:"Next"}).disabled).toBe(false);
      if(!tick)throw new Error("Docker health refresh timer missing");
      await act(async()=>{tick?.();await Promise.resolve();});
      expect(screen.getByRole<HTMLButtonElement>("button",{name:"Next"}).disabled).toBe(true);
      expect(screen.getByRole("status").textContent).toContain("Docker Engine disconnected or insecure; retrying");
      await act(async()=>{tick?.();await Promise.resolve();});
      expect(screen.getByRole<HTMLButtonElement>("button",{name:"Next"}).disabled).toBe(false);
      expect(getSandboxBackends).toHaveBeenCalledTimes(3);
    }finally{
      spy.mockRestore();
    }
  });

  it("requires a reviewed pinned image and creates through the typed Docker API", async () => {
    const createDockerSandbox = vi.fn().mockResolvedValue(created);
    showCreate({
      getRuntimeGovernance: vi.fn().mockResolvedValue(runtimeGovernanceStub()),
      getSandboxBackends: vi.fn().mockResolvedValue({ dockerAvailable: true, dockerStatus: "ready" }),
      createDockerSandbox,
    });

    fireEvent.click(await screen.findByRole("button", { name: /Docker/ }));
    const nameField = screen.getByRole<HTMLInputElement>("textbox", { name: "Sandbox name" });
    fireEvent.click(screen.getByText("Enter image digest manually"));
    const digestField = screen.getByRole<HTMLInputElement>("textbox", { name: "Immutable image digest" });
    fireEvent.change(nameField, { target: { value: "Safe Container" } });
    fireEvent.change(digestField, { target: { value: "alpine:latest" } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Next" }).disabled).toBe(true);
    expect(screen.queryByRole("spinbutton", { name: "CPU (millicores)" })).toBeNull();
    fireEvent.change(digestField, { target: { value: cachedImage } });
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.queryByRole("textbox", { name: "Sandbox name" })).toBeNull();
    expect(screen.getByRole("spinbutton", { name: "CPU (millicores)" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(screen.getByText(/Workspace files are ephemeral/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    await waitFor(() => expect(createDockerSandbox).toHaveBeenCalledWith({
      name: "Safe Container",
      imageDigest: cachedImage,
      userId: 10001,
      groupId: 10001,
      cpuMilli: 1000,
      memoryMiB: 512,
      workspaceMiB: 64,
      pids: 64,
    }));
    expect(await screen.findByText("Sandbox detail route reached")).toBeTruthy();
  });

  it("creates Docker from a selected locally verified image without typing a digest", async () => {
    const redisImage=`docker.io/library/redis@sha256:${"b".repeat(64)}`;
    const createDockerSandbox=vi.fn().mockResolvedValue(created);
    showCreate({
      getRuntimeGovernance:vi.fn().mockResolvedValue(runtimeGovernanceStub()),
      getSandboxBackends:vi.fn().mockResolvedValue({dockerAvailable:true,dockerStatus:"ready"}),
      listDockerImages:vi.fn().mockResolvedValue({images:[
        {imageDigest:cachedImage,displayName:"alpine:3.22"},
        {imageDigest:redisImage,displayName:"redis:8-alpine"},
      ]}),
      createDockerSandbox,
    });
    fireEvent.click(await screen.findByRole("button",{name:/Docker/}));
    fireEvent.change(screen.getByRole("textbox",{name:"Sandbox name"}),{target:{value:"Redis isolated"}});
    const select=screen.getByRole<HTMLSelectElement>("combobox",{name:"Cached Docker image"});
    expect(await screen.findByRole("option",{name:/redis:8-alpine/})).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button",{name:"Next"}).disabled).toBe(true);
    fireEvent.change(select,{target:{value:redisImage}});
    fireEvent.click(screen.getByRole("button",{name:"Next"}));
    fireEvent.click(screen.getByRole("button",{name:"Review"}));
    expect(screen.getByText(redisImage)).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Create sandbox"}));
    await waitFor(()=>expect(createDockerSandbox).toHaveBeenCalledWith(expect.objectContaining({
      name:"Redis isolated",imageDigest:redisImage,
    })));
    expect(await screen.findByText("Sandbox detail route reached")).toBeTruthy();
  });

  it("keeps identity and resource limits in separate steps, preserving edits after Back", async () => {
    const createDockerSandbox = vi.fn();
    showCreate({
      getRuntimeGovernance: vi.fn().mockResolvedValue(runtimeGovernanceStub()),
      getSandboxBackends: vi.fn().mockResolvedValue({ dockerAvailable: true, dockerStatus: "ready" }),
      createDockerSandbox,
    });

    fireEvent.click(await screen.findByRole("button", { name: /Docker/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "Sandbox name" }), { target: { value: "Preview only" } });
    expect(await screen.findByRole("option",{name:/alpine:3.22/})).toBeTruthy();
    expect(screen.queryByRole("spinbutton", { name: "Memory (MiB)" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Next" }));

    const memory = screen.getByRole<HTMLInputElement>("spinbutton", { name: "Memory (MiB)" });
    fireEvent.change(memory, { target: { value: "128" } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Review" }).disabled).toBe(true);
    fireEvent.change(memory, { target: { value: "1024" } });
    fireEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Sandbox name" }).value).toBe("Preview only");
    expect(screen.queryByRole("spinbutton", { name: "Memory (MiB)" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getByRole<HTMLInputElement>("spinbutton", { name: "Memory (MiB)" }).value).toBe("1024");
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(screen.queryByRole("spinbutton", { name: "Memory (MiB)" })).toBeNull();
    expect(screen.getByText("1024")).toBeTruthy();
    expect(createDockerSandbox).not.toHaveBeenCalled();
  });

  it("takes an uncertain persisted result to its detail instead of retriggering Docker create", async () => {
    const createDockerSandbox = vi.fn().mockResolvedValue({ ...created, state: "unknown" });
    showCreate({
      getRuntimeGovernance: vi.fn().mockResolvedValue(runtimeGovernanceStub()),
      getSandboxBackends: vi.fn().mockResolvedValue({ dockerAvailable: true, dockerStatus: "ready" }),
      createDockerSandbox,
    });
    fireEvent.click(await screen.findByRole("button", { name: /Docker/ }));
    fireEvent.change(screen.getByRole("textbox", { name: "Sandbox name" }), { target: { value: "Uncertain Container" } });
    await screen.findByRole("option",{name:/alpine:3.22/});
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    fireEvent.click(screen.getByRole("button", { name: "Create sandbox" }));
    expect(await screen.findByText("Sandbox detail route reached")).toBeTruthy();
    expect(createDockerSandbox).toHaveBeenCalledTimes(1);
  });
});
