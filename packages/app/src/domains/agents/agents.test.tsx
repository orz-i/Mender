// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { Agent, AgentTools, AgentTask, ManagementClient, PluginCatalogueEntry } from "@mender/sdk";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { AgentsPage } from "./agents-page";
import { AgentBackendPage } from "./agent-backend-page";
import { agentBackendDetails, agentBackendOptions } from "./agent-backends";
import { AgentCreatePage } from "./agent-create-page";
import { AgentDetailPage } from "./agent-detail-page";

const agent:Agent={
  agentId:"agent_00000000000000000000000000000101",name:"Code Reviewer",
  backend:"codex",pluginId:"io.mender.agent.codex",extensionId:"main",
  workspaceRoot:"/tmp/mender-agent-fixture",isolation:"restricted_host",
  backendAvailable:false,issue:"codex-acp adapter not installed",
  createdAt:"2026-10-11T10:00:00Z",
};
const task:AgentTask={
  taskId:"atask_00000000000000000000000000000202",agentId:agent.agentId,
  status:"completed",result:"fixture reviewed",createdAt:"2026-10-11T10:00:00Z",
  updatedAt:"2026-10-11T10:01:00Z",cancelRequested:false,
};
const suite:AgentTools={suiteId:"agent.delegation",status:"ready",
  providerId:"prov_00000000000000000000000000000001",providerName:"Code Reviewer",
  tools:["delegate","status","result","cancel"].map((name,i)=>({
    toolId:"tool_"+String(i+1).padStart(32,"0"),sourceKey:"agent."+name,name,title:name,
  })),
};
const presetPlugins = agentBackendOptions.map((backend): PluginCatalogueEntry => ({
  key: agentBackendDetails[backend].pluginId + "@builtin",
  pluginId: agentBackendDetails[backend].pluginId,
  extensionId: "main",
  kind: "agent.backend",
  marketplaceAction: "create.agent",
  displayName: agentBackendDetails[backend].name,
  description: "Native Agent backend",
  packageVersion: "1.0.0",
  sourceKind: "builtin",
  status: "enabled",
  enabled: true,
  capabilities: ["tasks"],
  usageCount: 0,
}));

beforeEach(()=>{resetLocaleForTesting();setLocale("en");});
afterEach(()=>{cleanup();resetLocaleForTesting();});
function TestPluginRouteSwitch(){
  const navigate=useNavigate();
  return <div>{(["codex","grok","opencode"] as const).map(backend=>
    <button key={backend} type="button"
      onClick={()=>void navigate("/agents/new?create=1&pluginId=io.mender.agent."+backend)}>
      Switch to {backend}
    </button>)}</div>;
}
function show(path:string,overrides:Partial<ManagementClient>={}){
  const client:Partial<ManagementClient>={
    listAgents:vi.fn().mockResolvedValue({agents:[agent]}),
    listPluginCatalogue:vi.fn().mockResolvedValue({plugins:presetPlugins}),
    getAgent:vi.fn().mockResolvedValue(agent),
    getAgentTools:vi.fn().mockResolvedValue(suite),
    ensureAgentTools:vi.fn().mockResolvedValue(suite),
    listAgentTasks:vi.fn().mockResolvedValue({tasks:[task]}),
    getAgentTask:vi.fn().mockResolvedValue(task),
    listAgentAdmissions:vi.fn().mockResolvedValue({events:[{
      agentId:agent.agentId,action:"agent.task.submit",origin:"mcp",principalKind:"service",
      scopeKind:"profile",policyKind:"allow",policyReason:"default_allow",status:"admitted",
      occurredAt:"2026-10-11T10:00:00Z",
    }]}),
    listAgentGrants:vi.fn().mockResolvedValue({profileIds:[]}),
    listProfiles:vi.fn().mockResolvedValue({profiles:[]}),
    createAgent:vi.fn().mockResolvedValue(agent),
    grantAgentProfile:vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  render(<MemoryRouter initialEntries={[path]}><Routes>
    <Route path="/agents" element={<AgentsPage client={client as ManagementClient}/>}/>
    <Route path="/agents/new" element={<AgentCreatePage client={client as ManagementClient}/>}/>
    <Route path="/agents/backend/:backend" element={<AgentBackendPage client={client as ManagementClient}/>}/>
    <Route path="/agents/:agentId" element={<AgentDetailPage client={client as ManagementClient}/>}/>
    <Route path="/tools" element={<div>Provider creation target</div>}/>
    <Route path="/plugins" element={<div>Plugin catalogue</div>}/>
  </Routes></MemoryRouter>);
  return client;
}
describe("Agents focused management",()=>{
  it("shows all four preset backends even without configured Agents, with no filters, search, or Add action", async () => {
    show("/agents", { listAgents: vi.fn().mockResolvedValue({ agents: [] }) });
    const catalog = await screen.findByRole("list", { name: "Available Agent backends" });
    expect(within(catalog).getAllByRole("listitem")).toHaveLength(4);
    for (const name of ["Codex", "Grok", "OpenCode", "Antigravity"]) {
      expect(within(catalog).getByRole("link", { name: "Configure " + name })).toBeTruthy();
    }
    expect(within(catalog).getAllByText("Ready to configure")).toHaveLength(4);
    expect(screen.queryByRole("textbox", { name: "Search agents" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Add agent" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Available agents/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Unavailable agents/ })).toBeNull();
  });

  it("opens pinned configuration from an unconfigured preset without generic backend selection", async () => {
    show("/agents", { listAgents: vi.fn().mockResolvedValue({ agents: [] }) });
    const destination = await screen.findByRole("link", { name: "Configure OpenCode" });
    expect(destination.getAttribute("href")).toBe(
      "/agents/new?pluginId=io.mender.agent.opencode&from=agents",
    );
    fireEvent.click(destination);
    expect(await screen.findByRole("textbox", { name: "Agent name" })).toBeTruthy();
    expect(screen.getAllByText("Configure agent").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: /Codex/ })).toBeNull();
    fireEvent.click(screen.getAllByRole("button", { name: "Back" })[0]!);
    expect(await screen.findByRole("list", { name: "Available Agent backends" })).toBeTruthy();
  });

  it("opens the exact existing Agent instance when a preset has one configured resource", async () => {
    const opencode = { ...agent, agentId: "agent_opencode", name: "OpenCode Workspace",
      backend: "opencode" as const, pluginId: "io.mender.agent.opencode", backendAvailable: true };
    const getAgent = vi.fn().mockResolvedValue(opencode);
    show("/agents", {
      listAgents: vi.fn().mockResolvedValue({ agents: [opencode] }), getAgent,
    });
    const card = await screen.findByRole("link", { name: "Configure OpenCode" });
    expect(card.getAttribute("href")).toBe("/agents/agent_opencode?view=configuration");
    expect(within(card).getByText("Configured · CLI detected")).toBeTruthy();
    expect(within(card).getByText("1 configured workspaces")).toBeTruthy();
    fireEvent.click(card);
    expect((await screen.findByRole("button", { name: "Configuration" })).getAttribute("aria-pressed")).toBe("true");
    expect(await screen.findByText("OpenCode Workspace")).toBeTruthy();
    expect(getAgent).toHaveBeenCalledWith(opencode.agentId);
  });

  it("does not silently choose one workspace when a preset has multiple configurations", async () => {
    const mac = { ...agent, agentId: "agent_mac", name: "Mac Agent",
      workspaceRoot: "/Users/mini/Coding/Mender", backendAvailable: true };
    const win = { ...agent, agentId: "agent_win", name: "Windows Agent",
      workspaceRoot: "C:\\Users\\mini\\Projects\\Mender", backendAvailable: true };
    const getAgent = vi.fn();
    show("/agents", {
      listAgents: vi.fn().mockResolvedValue({ agents: [mac, win] }), getAgent,
    });
    const card = await screen.findByRole("link", { name: "Configure Codex" });
    expect(card.getAttribute("href")).toBe("/agents/backend/codex");
    expect(within(card).getByText("2 configured workspaces")).toBeTruthy();
    fireEvent.click(card);
    const workspaces = await screen.findByRole("region", { name: "Configured Agent workspaces" });
    expect(within(workspaces).getByRole("link", { name: /Mac Agent/ })).toBeTruthy();
    expect(within(workspaces).getByRole("link", { name: /Windows Agent/ })).toBeTruthy();
    expect(getAgent).not.toHaveBeenCalled();
  });

  it("keeps unavailable presets visible with actionable details rather than a fake create flow", async () => {
    show("/agents", {
      listAgents: vi.fn().mockResolvedValue({ agents: [] }),
      listPluginCatalogue: vi.fn().mockResolvedValue({ plugins: presetPlugins.map((plugin) =>
        plugin.pluginId === "io.mender.agent.codex"
          ? { ...plugin, status: "unavailable", enabled: false, issue: "codex-acp adapter not installed" }
          : plugin) }),
    });
    const codex = await screen.findByRole("link", { name: "Configure Codex" });
    expect(codex.getAttribute("href")).toBe("/agents/backend/codex");
    expect(within(codex).getByText("Backend unavailable")).toBeTruthy();
    fireEvent.click(codex);
    expect(await screen.findByText("codex-acp adapter not installed")).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Configure workspace" })).toBeNull();
  });

  it("treats an unavailable catalog lookup as unverified, not an empty Agent installation", async () => {
    show("/agents", {
      listAgents: vi.fn().mockResolvedValue({ agents: [] }),
      listPluginCatalogue: vi.fn().mockRejectedValue(new Error("Catalogue offline")),
    });
    expect(await screen.findByText(/Catalogue offline/)).toBeTruthy();
    const catalog = screen.getByRole("list", { name: "Available Agent backends" });
    expect(within(catalog).getAllByText("Status unverified")).toHaveLength(4);
    expect(within(catalog).getByRole("link", { name: "Configure Grok" }).getAttribute("href"))
      .toBe("/agents/backend/grok");
    expect(within(catalog).getAllByRole("listitem")).toHaveLength(4);
  });

  it("redirects the generic Agent creation URL to the preset list instead of showing a duplicate selector", async()=>{
    show("/agents/new");
    expect(await screen.findByRole("list",{name:"Available Agent backends"})).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Select backend"})).toBeNull();
    expect(screen.queryByRole("textbox",{name:"Agent name"})).toBeNull();
  });
  it("refuses unknown plugin IDs instead of falling back to Codex",()=>{
    show("/agents/new?create=1&pluginId=io.mender.provider.mcp");
    expect(screen.getByText("Unknown Agent backend plugin.")).toBeTruthy();
    expect(screen.queryByRole("textbox",{name:"Agent name"})).toBeNull();
    expect(screen.queryByRole("button",{name:"Save configuration"})).toBeNull();
  });
  it("pins the marketplace plugin so Back cannot switch to an unrelated Agent",async()=>{
    show("/agents/new?create=1&pluginId=io.mender.agent.opencode");
    expect(screen.getByRole("textbox",{name:"Agent name"})).toBeTruthy();
    expect(screen.queryByRole("button",{name:"Continue"})).toBeNull();
    fireEvent.click(screen.getAllByRole("button",{name:"Back"})[1]!);
    expect(await screen.findByText("Plugin catalogue")).toBeTruthy();
    expect(screen.queryByRole("button",{name:/Codex/})).toBeNull();
  });
  it("resets stale form and backend when only marketplace pluginId changes on the same mounted route",async()=>{
    const createAgent=vi.fn().mockResolvedValue(agent);
    const client={createAgent} as unknown as ManagementClient;
    render(<MemoryRouter initialEntries={["/agents/new?create=1&pluginId=io.mender.agent.grok"]}>
      <TestPluginRouteSwitch/>
      <Routes>
        <Route path="/agents/new" element={<AgentCreatePage client={client}/>}/>
      </Routes>
    </MemoryRouter>);
    fireEvent.change(screen.getByRole("textbox",{name:"Agent name"}),{target:{value:"Stale Grok form"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:"/tmp/old-grok"}});
    fireEvent.click(screen.getByRole("button",{name:"Review"}));
    expect(screen.getByText("Grok")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Switch to opencode"}));
    expect(screen.getByRole<HTMLInputElement>("textbox",{name:"Agent name"}).value).toBe("");
    expect(screen.queryByText("Stale Grok form")).toBeNull();
    fireEvent.change(screen.getByRole("textbox",{name:"Agent name"}),{target:{value:"Temporary OpenCode"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:"/tmp/new-opencode"}});
    fireEvent.click(screen.getByRole("button",{name:"Review"}));
    expect(screen.getByText("OpenCode")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Switch to codex"}));
    expect(screen.getByRole<HTMLInputElement>("textbox",{name:"Agent name"}).value).toBe("");
    fireEvent.change(screen.getByRole("textbox",{name:"Agent name"}),{target:{value:"Test Codex"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:"/tmp/codex"}});
    fireEvent.click(screen.getByRole("button",{name:"Review"}));
    expect(screen.getByText("Codex")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Switch to opencode"}));
    expect(screen.getByRole<HTMLInputElement>("textbox",{name:"Agent name"}).value).toBe("");
    fireEvent.change(screen.getByRole("textbox",{name:"Agent name"}),{target:{value:"Verified OpenCode"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:"/tmp/verified-opencode"}});
    fireEvent.click(screen.getByRole("button",{name:"Review"}));
    expect(screen.getByText("OpenCode")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Save configuration"}));
    await waitFor(()=>expect(createAgent).toHaveBeenCalledWith({
      name:"Verified OpenCode",backend:"opencode",workspaceRoot:"/tmp/verified-opencode",
      isolation:"restricted_host",defaultModel:"",
    }));
  });
  it("configures only selected Agent backend then requires review before create",async()=>{
    const client=show("/agents/new?pluginId=io.mender.agent.grok");
    expect(await screen.findByRole("textbox",{name:"Agent name"})).toBeTruthy();
    expect(screen.queryByText("Codex CLI requires the separately installed codex-acp adapter.")).toBeNull();
    expect(screen.getByRole<HTMLButtonElement>("button",{name:"Review"}).disabled).toBe(true);
    fireEvent.change(screen.getByRole("textbox",{name:"Agent name"}),{target:{value:"Grok Reviewer"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:"/tmp/grok-sandbox"}});
    fireEvent.click(screen.getByRole("button",{name:"Review"}));
    expect(screen.getByText("Creating does not grant Profile access.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Help for Save configuration"}));
    expect(await screen.findByText(/Saving an Agent workspace prepares governed tools/)).toBeTruthy();
    expect(client.createAgent).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Save configuration"}));
    await waitFor(()=>expect(client.createAgent).toHaveBeenCalledWith({
      name:"Grok Reviewer",backend:"grok",workspaceRoot:"/tmp/grok-sandbox",
      isolation:"restricted_host",defaultModel:"",
    }));
  });
  it("opens OpenCode-specific creation from the built-in marketplace without cloning Grok",async()=>{
    const client=show("/agents/new?pluginId=io.mender.agent.opencode");
    expect(screen.getByRole("textbox",{name:"Agent name"})).toBeTruthy();
    expect(screen.getByText(/OpenCode may need local account and config access/)).toBeTruthy();
    expect(screen.queryByText("Grok CLI uses its built-in ACP stdio transport.")).toBeNull();
    fireEvent.change(screen.getByRole("textbox",{name:"Agent name"}),{target:{value:"OpenCode Local"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:"/tmp/opencode-fixture"}});
    fireEvent.click(screen.getByRole("button",{name:"Review"}));
    expect(screen.getByText("OpenCode")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Save configuration"}));
    await waitFor(()=>expect(client.createAgent).toHaveBeenCalledWith({
      name:"OpenCode Local",backend:"opencode",workspaceRoot:"/tmp/opencode-fixture",
      isolation:"restricted_host",defaultModel:"",
    }));
  });
  it("opens Antigravity-specific creation from the built-in marketplace with native YOLO off by default",async()=>{
    const client=show("/agents/new?pluginId=io.mender.agent.antigravity");
    expect(screen.getByRole("textbox",{name:"Agent name"})).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Help for Antigravity permissions"}));
    expect(await screen.findByText(/Configure workspace authorization after creation to enable 24-hour YOLO/)).toBeTruthy();
    fireEvent.change(screen.getByRole("textbox",{name:"Agent name"}),{target:{value:"Antigravity Local"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Workspace directory"}),{target:{value:"/tmp/agy-fixture"}});
    fireEvent.click(screen.getByRole("button",{name:"Review"}));
    expect(screen.getByText("Antigravity")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Save configuration"}));
    await waitFor(()=>expect(client.createAgent).toHaveBeenCalledWith({
      name:"Antigravity Local",backend:"antigravity",workspaceRoot:"/tmp/agy-fixture",
      isolation:"restricted_host",defaultModel:"",
    }));
  });
  it("summarizes task outcomes before technical metadata and opens unknown result without retrying", async () => {
    const unknown: AgentTask = {
      ...task, taskId: "atask_unknown_001", title: "Check remote migration",
      status: "unknown", result: "", createdAt: "2026-10-11T13:00:00Z",
    };
    const failed: AgentTask = {
      ...task, taskId: "atask_failed_002", title: "Audit provider connection",
      status: "failed", result: "", createdAt: "2026-10-11T12:00:00Z",
    };
    const getAgentTask = vi.fn().mockResolvedValue(unknown);
    const client = show("/agents/" + agent.agentId, {
      listAgentTasks: vi.fn().mockResolvedValue({ tasks: [task, failed, unknown] }),
      listAgentGrants: vi.fn().mockResolvedValue({ profileIds: ["prof_work"] }),
      getAgentTask,
    });
    const summary = await screen.findByText("Recent tasks");
    const metrics = document.querySelector('[data-slot="agent-task-summary"]') as HTMLElement;
    expect(metrics).toBeTruthy();
    expect(within(metrics).getByText("Needs attention").parentElement?.textContent).toContain("2");
    expect(within(metrics).getByText("Profile access").parentElement?.textContent).toContain("1");
    expect(screen.getByText("2 task results need review")).toBeTruthy();
    expect(screen.getByText("Unknown outcomes require checking actual deliverables; never retry automatically.")).toBeTruthy();
    expect(summary).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "View task: Check remote migration" }));
    expect(await screen.findByRole("dialog", { name: "Agent task result" })).toBeTruthy();
    expect(screen.getByText(/Unknown does not mean the task succeeded or failed/)).toBeTruthy();
    expect(getAgentTask).toHaveBeenCalledWith(unknown.taskId);
    expect(client.ensureAgentTools).not.toHaveBeenCalled();
  });

  it("selects ACP model and reasoning effort together without duplicating Agent",async()=>{
    const inspectAgentModels=vi.fn().mockResolvedValue({current:"fixture-one",
      models:[{id:"fixture-one",name:"Fixture One"},{id:"fixture-two",name:"Fixture Two"}],
      reasoningEffort:{current:"medium",choices:[{id:"medium",name:"Medium"},{id:"xhigh",name:"Extra High"}]}});
    const updateAgentSettings=vi.fn().mockResolvedValue({...agent,defaultModel:"fixture-two",defaultReasoningEffort:"xhigh"});
    const client=show("/agents/"+agent.agentId,{inspectAgentModels,updateAgentSettings});
    expect(await screen.findByText("Code Reviewer")).toBeTruthy();
    expect(inspectAgentModels).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Edit runtime defaults"}));
    await waitFor(()=>expect(inspectAgentModels).toHaveBeenCalledWith(agent.agentId));
    const picker=await screen.findByRole("combobox",{name:"Agent model"});
    fireEvent.change(picker,{target:{value:"fixture-two"}});
    fireEvent.change(await screen.findByRole("combobox",{name:"Reasoning effort"}),{target:{value:"xhigh"}});
    fireEvent.click(screen.getByRole("button",{name:"Save"}));
    await waitFor(()=>expect(updateAgentSettings).toHaveBeenCalledWith(agent.agentId,"fixture-two","xhigh"));
    expect(client.createAgent).not.toHaveBeenCalled();
  });
  it("refreshes a ready Agent Tool schema only after explicit action",async()=>{
    const client=show("/agents/"+agent.agentId);
    fireEvent.click(await screen.findByRole("button",{name:/^Tools/}));
    expect(await screen.findByText("Code Reviewer · delegate")).toBeTruthy();
    expect(client.ensureAgentTools).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Refresh Agent tools"}));
    await waitFor(()=>expect(client.ensureAgentTools).toHaveBeenCalledWith(agent.agentId));
  });
  it("repairs a missing Agent suite only on explicit owner action",async()=>{
    const client=show("/agents/"+agent.agentId,{
      getAgentTools:vi.fn().mockResolvedValue({suiteId:"agent.delegation",status:"missing",tools:[]}),
    });
    fireEvent.click(await screen.findByRole("button",{name:/^Tools/}));
    expect(await screen.findByText("No Agent tools")).toBeTruthy();
    expect(client.ensureAgentTools).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Prepare Agent tools"}));
    await waitFor(()=>expect(client.ensureAgentTools).toHaveBeenCalledWith(agent.agentId));
  });
  it("places Profile grants and tasks on separate views and requires an explicit action",async()=>{
    const profile={
      profileId:"prof_00000000000000000000000000000123",displayName:"Development",
    };
    const client=show("/agents/"+agent.agentId,{
      listProfiles:vi.fn().mockResolvedValue({profiles:[profile]}),
    });
    expect(await screen.findByText("codex-acp adapter not installed")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Tools"}));
    expect(await screen.findByText("Code Reviewer · delegate")).toBeTruthy();
    expect(client.ensureAgentTools).not.toHaveBeenCalled();
    expect(screen.queryByText("fixture reviewed")).toBeNull();
    fireEvent.click(screen.getByRole("button",{name:/^Access/}));
    expect(await screen.findByText("No Agent grants")).toBeTruthy();
    expect(screen.queryByRole("combobox",{name:"Profile"})).toBeNull();
    fireEvent.click(screen.getByRole("button",{name:"Grant Profile"}));
    expect(await screen.findByRole("combobox",{name:"Profile"})).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Grant"}));
    await waitFor(()=>expect(client.grantAgentProfile).toHaveBeenCalledWith(agent.agentId,profile.profileId));
    fireEvent.click(screen.getByRole("button",{name:/^Tasks/}));
    fireEvent.click(await screen.findByRole("button",{name:/^View Task #/}));
    expect(await screen.findByText("fixture reviewed")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button",{name:"Close"})[0]!);
    fireEvent.click(screen.getByRole("button",{name:"Activity"}));
    expect(await screen.findByText("agent.task.submit")).toBeTruthy();
  });
  it("shows explicit Task names while retaining an honest legacy ID fallback and scoped actions", async () => {
    setLocale("zh-CN");
    const namedTask: AgentTask = {...task, title:"复核 MCP OAuth 连接"};
    const legacyTask: AgentTask = {...task, taskId:"atask_00000000000000000000000000000203",
      status:"unknown",result:"",createdAt:"2026-10-11T09:00:00Z"};
    const getAgentTask = vi.fn().mockImplementation((id:string) =>
      Promise.resolve(id===namedTask.taskId ? namedTask : legacyTask));
    show("/agents/"+agent.agentId, {
      listAgentTasks:vi.fn().mockResolvedValue({tasks:[namedTask,legacyTask]}),
      getAgentTask,
    });
    fireEvent.click(await screen.findByRole("button",{name:/^任务/}));
    const namedRow=await screen.findByRole("row",{name:/复核 MCP OAuth 连接/});
    expect(within(namedRow).getByText("复核 MCP OAuth 连接")).toBeTruthy();
    expect(within(namedRow).getByText("#00000202")).toBeTruthy();
    expect(within(namedRow).getByRole("cell",{name:/复核 MCP OAuth 连接/}).getAttribute("title")).toBe(namedTask.taskId);
    const oldRow=screen.getByRole("row",{name:/任务 #00000203/});
    expect(within(oldRow).getByRole("cell",{name:/任务 #00000203/}).getAttribute("title")).toBe(legacyTask.taskId);
    expect(within(oldRow).queryByText("复核 MCP OAuth 连接")).toBeNull();
    fireEvent.click(within(namedRow).getByRole("button",{name:"查看 复核 MCP OAuth 连接"}));
    const dialog=await screen.findByRole("dialog",{name:"智能体任务结果"});
    expect(within(dialog).getByText("复核 MCP OAuth 连接")).toBeTruthy();
    expect(within(dialog).getByText(namedTask.taskId)).toBeTruthy();
    await waitFor(()=>expect(getAgentTask).toHaveBeenCalledWith(namedTask.taskId));
  });

  it("keeps cancel confirmation targeted to the original full TaskID for a named queued Task", async () => {
    setLocale("zh-CN");
    const queuedTask:AgentTask={...task,status:"queued",title:"核对配置档供应",result:""};
    const cancelAgentTask=vi.fn().mockResolvedValue({taskId:queuedTask.taskId,cancelRequested:true});
    show("/agents/"+agent.agentId,{
      listAgentTasks:vi.fn().mockResolvedValue({tasks:[queuedTask]}),
      cancelAgentTask,
    });
    fireEvent.click(await screen.findByRole("button",{name:/^任务/}));
    const row=await screen.findByRole("row",{name:/核对配置档供应/});
    fireEvent.click(within(row).getByRole("button",{name:"取消任务 核对配置档供应"}));
    const dialog=await screen.findByRole("dialog",{name:"取消智能体任务"});
    expect(cancelAgentTask).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole("button",{name:"确认"}));
    await waitFor(()=>expect(cancelAgentTask).toHaveBeenCalledWith(queuedTask.taskId));
  });

  it("separates trusted native progress, worker heartbeat and the admitted deadline", async () => {
    const uncertainTask:AgentTask = {
      ...task, status:"unknown", result:"", timeoutSeconds:1800,
      diagnostic:"Agent outcome unknown after timeout",
      observation:{
        phase:"leader_reaped", activity:"uncertain",
        lastObservedAt:"2026-10-11T10:30:00Z",
        lastNativeProgressAt:"2026-10-11T10:29:45Z",
        lastWorkerHeartbeatAt:"2026-10-11T10:29:55Z",
        sessionRecorded:true, sessionRecoverability:"unavailable", leaderReaped:true,
      },
    };
    show("/agents/"+agent.agentId,{
      listAgentTasks:vi.fn().mockResolvedValue({tasks:[uncertainTask]}),
      getAgentTask:vi.fn().mockResolvedValue(uncertainTask),
    });
    fireEvent.click(await screen.findByRole("button",{name:/^Tasks/}));
    fireEvent.click(await screen.findByRole("button",{name:/^View Task #/}));
    const dialog=await screen.findByRole("dialog",{name:"Agent task result"});
    const technicalDetails=within(dialog).getByText("Execution details").closest("details");
    expect(technicalDetails).toBeTruthy();
    expect(technicalDetails!.open).toBe(false);
    const unknownWarning=within(dialog).getByText(/Unknown does not mean the task succeeded or failed/);
    expect(Boolean(unknownWarning.compareDocumentPosition(technicalDetails!) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(true);
    fireEvent.click(within(dialog).getByText("Execution details"));
    expect(technicalDetails!.open).toBe(true);
    expect(within(dialog).getByText("30 minutes")).toBeTruthy();
    expect(within(dialog).getByText("Task execution deadline")).toBeTruthy();
    const native=within(dialog).getByText("Last native Agent progress").nextElementSibling?.textContent;
    const worker=within(dialog).getByText("Mender worker heartbeat").nextElementSibling?.textContent;
    const lifecycle=within(dialog).getByText("Last observed").nextElementSibling?.textContent;
    expect(native).toBeTruthy();
    expect(worker).toBeTruthy();
    expect(native).not.toBe(worker);
    expect(native).not.toBe(lifecycle);
    expect(within(dialog).getByText(/Unknown does not mean the task succeeded or failed/)).toBeTruthy();
    expect(within(dialog).queryByRole("button",{name:/^(Resume|Retry|Continue)$/})).toBeNull();
  });

  it("shows an unknown outcome as a localized caution in a scroll-safe read-only task dialog",async()=>{
    setLocale("zh-CN");
    const unknownTask:AgentTask={
      ...task,status:"unknown",result:"",diagnostic:"Agent outcome unknown after timeout",
    };
    const client=show("/agents/"+agent.agentId,{
      listAgentTasks:vi.fn().mockResolvedValue({tasks:[unknownTask]}),
      getAgentTask:vi.fn().mockResolvedValue(unknownTask),
    });
    fireEvent.click(await screen.findByRole("button",{name:/^任务/}));
    fireEvent.click(await screen.findByRole("button",{name:/^查看 任务 #/}));
    const dialog=await screen.findByRole("dialog",{name:"智能体任务结果"});
    const badge=within(dialog).getByText("状态未知").closest('[data-slot="badge"]');
    expect(badge?.className).toContain("bg-sunlight/20");
    expect(within(dialog).queryByText("unknown")).toBeNull();
    expect(within(dialog).getByText("该任务没有可核对的执行证据。")).toBeTruthy();
    expect(within(dialog).getByText("任务超时后无法确认智能体执行结果")).toBeTruthy();
    const details=within(dialog).getByText("执行详情").closest("details");
    expect(details?.open).toBe(false);
    fireEvent.click(within(dialog).getByText("执行详情"));
    expect(details?.open).toBe(true);
    expect(within(dialog).getByText("该任务没有可核对的执行证据。")).toBeTruthy();
    expect(within(dialog).getByRole("button",{name:"刷新状态"})).toBeTruthy();
    expect(dialog.className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(dialog.querySelector('[data-slot="dialog-body"]')?.className).toContain("overflow-y-auto");
    expect(client.getAgentTask).toHaveBeenCalledWith(unknownTask.taskId);
  });

  it("localizes bounded ACP prompt stream failures in the owner Task result dialog",async()=>{
    setLocale("zh-CN");
    const failedTask:AgentTask={
      ...task,status:"failed",result:"",diagnostic:"ACP prompt stream closed before completion",
    };
    show("/agents/"+agent.agentId,{
      listAgentTasks:vi.fn().mockResolvedValue({tasks:[failedTask]}),
      getAgentTask:vi.fn().mockResolvedValue(failedTask),
    });
    expect(await screen.findByText("Code Reviewer")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:/^任务/}));
    fireEvent.click(await screen.findByRole("button",{name:/^查看 任务 #/}));
    expect(await screen.findByText("ACP 任务完成前，输出流已关闭")).toBeTruthy();
    expect(screen.queryByText("ACP prompt stream closed before completion")).toBeNull();
  });
  it("localizes unadvertised configured ACP models without exposing model identifiers",async()=>{
    setLocale("zh-CN");
    const failedTask:AgentTask={
      ...task,status:"failed",result:"",diagnostic:"Configured Agent model not advertised by ACP session",
    };
    show("/agents/"+agent.agentId,{
      listAgentTasks:vi.fn().mockResolvedValue({tasks:[failedTask]}),
      getAgentTask:vi.fn().mockResolvedValue(failedTask),
    });
    expect(await screen.findByText("Code Reviewer")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:/^任务/}));
    fireEvent.click(await screen.findByRole("button",{name:/^查看 任务 #/}));
    expect(await screen.findByText("当前 ACP 会话未提供所配置的智能体模型")).toBeTruthy();
    expect(screen.queryByText("Configured Agent model not advertised by ACP session")).toBeNull();
  });
  it("localizes safe Agent failure stages in the owner Task result dialog",async()=>{
    setLocale("zh-CN");
    const failedTask:AgentTask={
      ...task,status:"failed",result:"",diagnostic:"ACP session creation failed",
    };
    show("/agents/"+agent.agentId,{
      listAgentTasks:vi.fn().mockResolvedValue({tasks:[failedTask]}),
      getAgentTask:vi.fn().mockResolvedValue(failedTask),
    });
    expect(await screen.findByText("Code Reviewer")).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:/^任务/}));
    fireEvent.click(await screen.findByRole("button",{name:/^查看 任务 #/}));
    expect(await screen.findByText("ACP 会话创建失败")).toBeTruthy();
    expect(screen.queryByText("ACP session creation failed")).toBeNull();
  });
  for (const {diagnostic,localized} of [
    {diagnostic:"ACP permission request is invalid",localized:"ACP 权限请求格式不符合要求"},
    {diagnostic:"Agent requested an unsupported ACP client capability",localized:"智能体请求了 Mender 未提供的 ACP 客户端能力"},
  ]) {
    it(`localizes safe permission diagnostic ${diagnostic}`,async()=>{
      setLocale("zh-CN");
      const failedTask:AgentTask={...task,status:"failed",result:"",diagnostic};
      show("/agents/"+agent.agentId,{
        listAgentTasks:vi.fn().mockResolvedValue({tasks:[failedTask]}),
        getAgentTask:vi.fn().mockResolvedValue(failedTask),
      });
      fireEvent.click(await screen.findByRole("button",{name:/^任务/}));
      fireEvent.click(await screen.findByRole("button",{name:/^查看 任务 #/}));
      expect(await screen.findByText(localized)).toBeTruthy();
      expect(screen.queryByText(diagnostic)).toBeNull();
    });
  }
  for (const status of ["completed","failed"] as const) {
    it(`discloses missing native audit coverage for a ${status} Host task`,async()=>{
      const host={...agent,isolation:"host" as const};
      const hostTask:AgentTask={...task,status,
        result:status==="completed"?"task completed":"",
        ...(status==="failed"?{diagnostic:"Agent execution failed"}:{}),
      };
      show("/agents/"+agent.agentId,{
        getAgent:vi.fn().mockResolvedValue(host),
        listAgentTasks:vi.fn().mockResolvedValue({tasks:[hostTask]}),
        getAgentTask:vi.fn().mockResolvedValue(hostTask),
      });
      fireEvent.click(await screen.findByRole("button",{name:/^Tasks/}));
      fireEvent.click(await screen.findByRole("button",{name:/^View Task #/}));
      const note=await screen.findByRole("note");
      expect(note.textContent).toContain("ACP approvals are voluntary requests made by the Agent");
      expect(note.textContent).toContain("zero approvals does not prove");
    });
  }
  it("omits the Host-only audit coverage notice for restricted Agent tasks",async()=>{
    show("/agents/"+agent.agentId);
    fireEvent.click(await screen.findByRole("button",{name:/^Tasks/}));
    fireEvent.click(await screen.findByRole("button",{name:/^View Task #/}));
    expect(screen.queryByRole("note")).toBeNull();
  });
  it("localizes the Host audit coverage notice without showing raw native activity",async()=>{
    setLocale("zh-CN");
    show("/agents/"+agent.agentId,{
      getAgent:vi.fn().mockResolvedValue({...agent,isolation:"host" as const}),
    });
    fireEvent.click(await screen.findByRole("button",{name:/^任务/}));
    fireEvent.click(await screen.findByRole("button",{name:/^查看 任务 #/}));
    expect((await screen.findByRole("note")).textContent).toContain("并不构成原生文件与 Shell 操作的完整记录");
    expect((await screen.findByRole("note")).textContent).toContain("没有任何审批记录不代表未发生任何操作");
  });
  it("keeps all backend statuses visible together rather than filtering out unhealthy presets", async () => {
    const readyAgent: Agent = {
      ...agent, agentId: "agent_opencode", name: "OpenCode Instance",
      backend: "opencode", pluginId: "io.mender.agent.opencode",
      backendAvailable: true,
    };
    show("/agents", {
      listAgents: vi.fn().mockResolvedValue({ agents: [agent, readyAgent] }),
    });
    const catalog = await screen.findByRole("list", { name: "Available Agent backends" });
    expect(within(catalog).getAllByRole("listitem")).toHaveLength(4);
    expect(within(catalog).getByText("Configured · CLI unavailable")).toBeTruthy();
    expect(within(catalog).getByText("Configured · CLI detected")).toBeTruthy();
    expect(within(catalog).getAllByText("Ready to configure")).toHaveLength(2);
    expect(screen.queryByRole("button", { name: /All agents/ })).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Search agents" })).toBeNull();
  });
});
