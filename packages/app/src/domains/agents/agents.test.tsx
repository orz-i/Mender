import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { Agent, AgentTools, AgentTask, MenderClient } from "@mender/sdk";
import { MemoryRouter, Route, Routes } from "react-router";
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
  it("loads advertised ACP model choices only when editing and keeps the same Agent",async()=>{
    const inspectAgentModels=vi.fn().mockResolvedValue({current:"fixture-one",
      models:[{id:"fixture-one",name:"Fixture One"},{id:"fixture-two",name:"Fixture Two"}]});
    const updateAgentSettings=vi.fn().mockResolvedValue({...agent,defaultModel:"fixture-two"});
    const client=show("/agents/"+agent.agentId,{inspectAgentModels,updateAgentSettings});
    expect(await screen.findByText("Code Reviewer")).toBeTruthy();
    expect(inspectAgentModels).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Edit default model"}));
    await waitFor(()=>expect(inspectAgentModels).toHaveBeenCalledWith(agent.agentId));
    const picker=await screen.findByRole("combobox",{name:"Agent model"});
    fireEvent.change(picker,{target:{value:"fixture-two"}});
    fireEvent.click(screen.getByRole("button",{name:"Save"}));
    await waitFor(()=>expect(updateAgentSettings).toHaveBeenCalledWith(agent.agentId,"fixture-two"));
    expect(client.createAgent).not.toHaveBeenCalled();
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
});
