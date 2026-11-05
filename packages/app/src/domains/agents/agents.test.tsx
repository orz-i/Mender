// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { Agent, AgentTools, AgentTask, ManagementClient } from "@mender/sdk";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { AgentsPage } from "./agents-page";
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
    <Route path="/agents/:agentId" element={<AgentDetailPage client={client as ManagementClient}/>}/>
    <Route path="/tools" element={<div>Provider creation target</div>}/>
    <Route path="/plugins" element={<div>Plugin catalogue</div>}/>
  </Routes></MemoryRouter>);
  return client;
}
describe("Agents focused management",()=>{
  it("shortens repeated workspace paths in the inventory without losing full paths or search", async () => {
    const mac = { ...agent, agentId: "agent_mac", name: "Mac Agent",
      workspaceRoot: "/Users/mini/Coding/Mender", backendAvailable: true };
    const win = { ...agent, agentId: "agent_win", name: "Windows Agent",
      workspaceRoot: "C:\\Users\\mini\\Projects\\Mender", backendAvailable: true };
    show("/agents", {
      listAgents: vi.fn().mockResolvedValue({ agents: [mac, win] }),
      getAgent: vi.fn().mockImplementation((id: string) =>
        Promise.resolve([mac, win].find((item) => item.agentId === id))),
    });
    const macRow = await screen.findByRole("row", { name: /Mac Agent/ });
    const macCell = within(macRow).getByRole("cell", { name: "Workspace: /Users/mini/Coding/Mender" });
    expect(within(macCell).getByText("…/Coding/Mender")).toBeTruthy();
    expect(macCell.getAttribute("title")).toBe("/Users/mini/Coding/Mender");
    expect(within(macRow).queryByText("/Users/mini/Coding/Mender")).toBeNull();

    const winRow = screen.getByRole("row", { name: /Windows Agent/ });
    const winCell = within(winRow).getByRole("cell", { name: /Workspace: C:/ });
    expect(within(winCell).getByText("…/Projects/Mender")).toBeTruthy();
    expect(winCell.getAttribute("title")).toBe(win.workspaceRoot);
    fireEvent.change(screen.getByRole("textbox", { name: "Search agents" }), {
      target: { value: "/Users/mini/Coding/Mender" },
    });
    expect(screen.getByRole("row", { name: /Mac Agent/ })).toBeTruthy();
    expect(screen.queryByRole("row", { name: /Windows Agent/ })).toBeNull();

    fireEvent.click(within(screen.getByRole("row", { name: /Mac Agent/ }))
      .getByRole("link", { name: "Mac Agent" }));
    expect(await screen.findByText("/Users/mini/Coding/Mender")).toBeTruthy();
  });

  it("does not manufacture path parents for root or short workspace values", async () => {
    const root = { ...agent, agentId: "agent_root", name: "Root Agent", workspaceRoot: "/" };
    const short = { ...agent, agentId: "agent_short", name: "Short Agent", workspaceRoot: "src" };
    show("/agents", { listAgents: vi.fn().mockResolvedValue({ agents: [root, short] }) });
    expect(await screen.findByRole("row", { name: /Root Agent/ })).toBeTruthy();
    expect(within(screen.getByRole("row", { name: /Root Agent/ }))
      .getByRole("cell", { name: "Workspace: /" }).textContent).toContain("/");
    expect(within(screen.getByRole("row", { name: /Short Agent/ }))
      .getByRole("cell", { name: "Workspace: src" }).textContent).toContain("src");
  });
  it("lists previews and does not show create forms on the inventory",async()=>{
    show("/agents");
    expect(await screen.findByText("Code Reviewer")).toBeTruthy();
    expect(screen.getByText("CLI missing")).toBeTruthy();
    expect(screen.queryByRole("textbox",{name:"Agent name"})).toBeNull();
    fireEvent.click(screen.getByRole("button",{name:"Add agent"}));
    expect(await screen.findByRole("button",{name:"Help for Select backend"})).toBeTruthy();
    fireEvent.click(screen.getByRole("button",{name:"Help for Select backend"}));
    expect(await screen.findByText("Choose an installed Agent CLI backend. No Agent process starts during setup.")).toBeTruthy();
  });
  it("shows each backend's actual identity in inventory and details",async()=>{
    const opencode={...agent,agentId:"agent_00000000000000000000000000000200",
      name:"OpenCode Instance",backend:"opencode" as const,pluginId:"io.mender.agent.opencode"};
    const grok={...agent,agentId:"agent_00000000000000000000000000000201",
      name:"Grok Instance",backend:"grok" as const,pluginId:"io.mender.agent.grok"};
    const codex={...agent,agentId:"agent_00000000000000000000000000000202",
      name:"Codex Instance",backend:"codex" as const,pluginId:"io.mender.agent.codex"};
    show("/agents",{
      listAgents:vi.fn().mockResolvedValue({agents:[opencode,grok,codex]}),
      getAgent:vi.fn().mockImplementation((id:string)=>Promise.resolve([opencode,grok,codex].find(a=>a.agentId===id))),
    });
    for(const [fixture,label] of [[opencode,"OpenCode"],[grok,"Grok"],[codex,"Codex"]] as const){
      const row=await screen.findByRole("row",{name:new RegExp(fixture.name)});
      expect(within(row).getByText(label)).toBeTruthy();
    }
    const openCodeRow=screen.getByRole("row",{name:/OpenCode Instance/});
    fireEvent.click(within(openCodeRow).getByRole("link",{name:"OpenCode Instance"}));
    expect(await screen.findByText("OpenCode Instance")).toBeTruthy();
    expect(screen.getByText("OpenCode")).toBeTruthy();
    expect(screen.queryByText("Grok")).toBeNull();
  });
  it("does not silently default a generic Agent creation to Codex",()=>{
    show("/agents/new");
    const continueButton=screen.getByRole<HTMLButtonElement>("button",{name:"Continue"});
    expect(continueButton.disabled).toBe(true);
    const openCodeButton=screen.getByRole("button",{name:/OpenCode/});
    expect(openCodeButton.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(openCodeButton);
    expect(openCodeButton.getAttribute("aria-pressed")).toBe("true");
    expect(continueButton.disabled).toBe(false);
    fireEvent.click(continueButton);
    expect(screen.getByRole("textbox",{name:"Agent name"})).toBeTruthy();
  });
  it("refuses unknown plugin IDs instead of falling back to Codex",()=>{
    show("/agents/new?create=1&pluginId=io.mender.provider.mcp");
    expect(screen.getByText("Unknown Agent backend plugin.")).toBeTruthy();
    expect(screen.queryByRole("textbox",{name:"Agent name"})).toBeNull();
    expect(screen.queryByRole("button",{name:"Create agent"})).toBeNull();
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
    fireEvent.click(screen.getByRole("button",{name:"Create agent"}));
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
    fireEvent.click(screen.getByRole("button",{name:"Help for Create agent"}));
    expect(await screen.findByText(/Creating an Agent prepares its governed tools/)).toBeTruthy();
    expect(client.createAgent).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Create agent"}));
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
    fireEvent.click(screen.getByRole("button",{name:"Create agent"}));
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
    fireEvent.click(screen.getByRole("button",{name:"Create agent"}));
    await waitFor(()=>expect(client.createAgent).toHaveBeenCalledWith({
      name:"Antigravity Local",backend:"antigravity",workspaceRoot:"/tmp/agy-fixture",
      isolation:"restricted_host",defaultModel:"",
    }));
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
  it("renders mobile-card-ready inventory layout and navigates to agent detail", async () => {
    const readyAgent: Agent = {
      ...agent,
      agentId: "agent_00000000000000000000000000000102",
      name: "Autonomous Coder",
      backend: "antigravity",
      backendAvailable: true,
      workspaceRoot: "/tmp/autonomous-coder-workspace",
      isolation: "host",
    };
    const getAgent = vi.fn().mockImplementation((id: string) =>
      Promise.resolve(id === readyAgent.agentId ? readyAgent : agent)
    );
    show("/agents", {
      listAgents: vi.fn().mockResolvedValue({ agents: [agent, readyAgent] }),
      getAgent,
    });

    const table = await screen.findByRole("table", { name: "Agent inventory" });
    expect(table).toBeTruthy();
    expect(table.className).toContain("max-md:block");

    const thead = table.querySelector("thead");
    expect(thead?.className).toContain("max-md:sr-only");

    const tbody = table.querySelector("tbody");
    expect(tbody?.className).toContain("max-md:block");

    const rows = table.querySelectorAll("tbody tr");
    expect(rows.length).toBe(2);
    expect(rows[0]?.className).toContain("max-md:grid");
    expect(rows[1]?.className).toContain("max-md:grid");

    // Both agents show name and prominent availability status
    expect(within(table).getByText("Code Reviewer")).toBeTruthy();
    expect(within(table).getByText("CLI missing")).toBeTruthy();
    expect(within(table).getByText("Autonomous Coder")).toBeTruthy();
    expect(within(table).getByText("CLI detected")).toBeTruthy();

    // Backend, isolation summaries and workspace are rendered
    expect(within(table).getByText("Codex")).toBeTruthy();
    expect(within(table).getByText("Antigravity")).toBeTruthy();
    expect(within(table).getAllByText("Restricted host").length).toBeGreaterThanOrEqual(1);
    expect(within(table).getAllByText("Host (not isolated)").length).toBeGreaterThanOrEqual(1);
    expect(within(table).getByText("/tmp/mender-agent-fixture")).toBeTruthy();
    expect(within(table).getByText("/tmp/autonomous-coder-workspace")).toBeTruthy();

    // Details navigation entrypoint is accessible and functional
    const destinations = within(table).getAllByRole("link");
    expect(destinations.length).toBe(2);
    fireEvent.click(within(table).getByRole("link", { name: "Autonomous Coder" }));
    expect(await screen.findByText("Autonomous Coder")).toBeTruthy();
    expect(getAgent).toHaveBeenCalledWith(readyAgent.agentId);
  });
  it("filters agents by availability tab in responsive inventory", async () => {
    const readyAgent: Agent = {
      ...agent,
      agentId: "agent_00000000000000000000000000000102",
      name: "Available Agent",
      backend: "antigravity",
      backendAvailable: true,
      workspaceRoot: "/tmp/available",
      isolation: "host",
    };
    show("/agents", {
      listAgents: vi.fn().mockResolvedValue({ agents: [agent, readyAgent] }),
    });

    const table = await screen.findByRole("table", { name: "Agent inventory" });
    expect(within(table).getByText("Code Reviewer")).toBeTruthy();
    expect(within(table).getByText("Available Agent")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /Available agents/ }));
    expect(within(table).getByText("Available Agent")).toBeTruthy();
    expect(within(table).queryByText("Code Reviewer")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: /Unavailable agents/ }));
    expect(within(table).getByText("Code Reviewer")).toBeTruthy();
    expect(within(table).queryByText("Available Agent")).toBeNull();
  });
});
