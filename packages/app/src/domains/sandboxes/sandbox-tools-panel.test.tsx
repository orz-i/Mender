import { cleanup,fireEvent,render,screen,waitFor,within } from "@testing-library/react";
import { MemoryRouter,Route,Routes } from "react-router";
import { afterEach,expect,it,vi } from "vitest";
import type { MenderClient,Sandbox,SandboxToolSuite } from "@mender/sdk";
import { resetLocaleForTesting,setLocale } from "../../i18n/i18n";
import { SandboxToolsPanel } from "./sandbox-tools-panel";
import { SandboxDetailPage } from "./sandbox-detail-page";

afterEach(()=>{cleanup();resetLocaleForTesting();});
const sandbox:Sandbox={
 sandboxId:"sbx_00000000000000000000000000000999",name:"Docs Workspace",
 backend:"local_workspace",isolation:"host",state:"ready",localRoot:"/tmp/mender-docs",
 ownerKind:"local_user",ownerRef:"fixture",activeLeases:0,backendAvailable:true,
};
const empty:SandboxToolSuite={
 suiteId:"sandbox.workspace",sandboxId:sandbox.sandboxId,status:"missing",
 available:true,tools:[],
};
const ready:SandboxToolSuite={
 ...empty,status:"ready",providerId:"prov_00000000000000000000000000000011",
 providerName:"Docs Workspace Workspace",
 tools:["read","search","edit","shell"].map((name,i)=>({
  toolId:"tool_"+String(i+1).padStart(32,"0"),sourceKey:"workspace."+name,
  name,title:name,
 })),
};
function show(client:Partial<MenderClient>,fullPage=false){
 resetLocaleForTesting();setLocale("en");
 const props={client:client as MenderClient,sandbox};
 render(<MemoryRouter initialEntries={[fullPage?"/sandboxes/"+sandbox.sandboxId:"/"]}>
  <Routes>
   <Route path="/" element={<SandboxToolsPanel {...props}/>}/>
   <Route path="/sandboxes/:sandboxId" element={<SandboxDetailPage client={client as MenderClient}/>}/>
   <Route path="/tools" element={<div>Internal provider management</div>}/>
  </Routes>
 </MemoryRouter>);
}
it("prepares a Sandbox suite only on owner action and requires confirmation to retire it",async()=>{
 const getSandboxTools=vi.fn().mockResolvedValue(empty);
 const ensureSandboxTools=vi.fn().mockResolvedValue(ready);
 const removeSandboxTools=vi.fn().mockResolvedValue(undefined);
 show({getSandboxTools,ensureSandboxTools,removeSandboxTools});
 expect(await screen.findByText("No Sandbox tools")).toBeTruthy();
 expect(ensureSandboxTools).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button",{name:"Prepare Workspace Tools"}));
 expect(await screen.findByText("Docs Workspace · read")).toBeTruthy();
 expect(screen.getByText("Docs Workspace · shell")).toBeTruthy();
 expect(ensureSandboxTools).toHaveBeenCalledWith(sandbox.sandboxId);
 fireEvent.click(screen.getByRole("button",{name:"Retire Workspace Tools"}));
 const dialog=screen.getByRole("dialog",{name:"Retire Workspace Tools"});
 expect(removeSandboxTools).not.toHaveBeenCalled();
 fireEvent.click(within(dialog).getByRole("button",{name:"Confirm"}));
 await waitFor(()=>expect(removeSandboxTools).toHaveBeenCalledWith(sandbox.sandboxId));
 expect(await screen.findByText("No Sandbox tools")).toBeTruthy();
});
it("rejects an unavailable or conflicting Sandbox Suite without a fake Add action",async()=>{
 const incompatible:SandboxToolSuite={...empty,status:"incompatible",available:false,
  issue:"Browser-only backend cannot provide filesystem or shell"};
 show({getSandboxTools:vi.fn().mockResolvedValue(incompatible)});
 expect(await screen.findByText(/Browser-only backend/)).toBeTruthy();
 expect(screen.queryByRole("button",{name:"Prepare Workspace Tools"})).toBeNull();
 cleanup();
 show({getSandboxTools:vi.fn().mockResolvedValue({...empty,status:"conflict"})});
 expect(await screen.findByText(/Multiple Workspace Providers target this Sandbox/)).toBeTruthy();
 expect(screen.queryByRole("button",{name:"Prepare Workspace Tools"})).toBeNull();
 fireEvent.click(screen.getByRole("button",{name:"View tool services"}));
 expect(await screen.findByText("Internal provider management")).toBeTruthy();
});
it("displays a separate Tools topic without loading it on Sandbox overview",async()=>{
 const getSandboxTools=vi.fn().mockResolvedValue(ready);
 show({getSandbox:vi.fn().mockResolvedValue(sandbox),getSandboxTools},true);
 expect(await screen.findByText("Docs Workspace")).toBeTruthy();
 expect(getSandboxTools).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button",{name:"Tools"}));
 expect(await screen.findByText("Docs Workspace · search")).toBeTruthy();
 expect(getSandboxTools).toHaveBeenCalledOnce();
});
