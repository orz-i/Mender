import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { Agent, AgentTools, AgentTask, MenderClient } from "@mender/sdk";
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
function show(path:string,overrides:Partial<MenderClient>={}){
  const client:Partial<MenderClient>={
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
    <Route path="/agents" element={<AgentsPage client={client as MenderClient}/>}/>
    <Route path="/agents/new" element={<AgentCreatePage client={client as MenderClient}/>}/>
    <Route path="/agents/:agentId" element={<AgentDetailPage client={client as MenderClient}/>}/>
    <Route path="/tools" element={<div>Provider creation target</div>}/>
    <Route path="/plugins" element={<div>Plugin catalogue</div>}/>
  </Routes></MemoryRouter>);
  return client;
}
describe("Agents focused management",()=>{
  it("lists previews and does not show create forms on the inventory",async()=>{
    show("/agents");
    expect(await screen.findByText("Code Reviewer")).toBeTruthy();
    expect(screen.getByText("CLI missing")).toBeTruthy();
    expect(screen.queryByRole("textbox",{name:"Agent name"})).toBeNull();
    fireEvent.click(screen.getByRole("button",{name:"Add agent"}));
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
    fireEvent.click(within(openCodeRow).getByRole("button",{name:"View"}));
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
    const client={createAgent} as unknown as MenderClient;
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
    expect(screen.getByText(/Creating an Agent prepares its governed tools/)).toBeTruthy();
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
    fireEvent.click(screen.getByRole("button",{name:"View Agent tools"}));
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
    fireEvent.click(await screen.findByRole("button",{name:"View"}));
    expect(await screen.findByText("fixture reviewed")).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button",{name:"Close"})[0]!);
    fireEvent.click(screen.getByRole("button",{name:"Activity"}));
    expect(await screen.findByText("agent.task.submit")).toBeTruthy();
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
    fireEvent.click(await screen.findByRole("button",{name:"查看"}));
    expect(await screen.findByText("ACP 会话创建失败")).toBeTruthy();
    expect(screen.queryByText("ACP session creation failed")).toBeNull();
  });
});
