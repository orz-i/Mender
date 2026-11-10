import { cleanup,fireEvent,render,screen } from "@testing-library/react";
import { MemoryRouter,Route,Routes } from "react-router";
import { afterEach,expect,it,vi } from "vitest";
import type { ManagementClient,Sandbox,SandboxToolSuite } from "@mender/sdk";
import { resetLocaleForTesting,setLocale } from "../../i18n/i18n";
import { SandboxToolsPanel } from "./sandbox-tools-panel";
import { SandboxDetailPage } from "./sandbox-detail-page";

afterEach(()=>{cleanup();resetLocaleForTesting();});
const sandbox:Sandbox={
 sandboxId:"sbx_00000000000000000000000000000999",name:"Docs Workspace",
 backend:"local_workspace",isolation:"host",state:"ready",localRoot:"/tmp/mender-docs",
 ownerKind:"local_user",ownerRef:"fixture",activeLeases:0,backendAvailable:true,
};
const suite:SandboxToolSuite={
 suiteId:"sandbox.workspace",sandboxId:sandbox.sandboxId,status:"ready",available:true,
 tools:["read","search","edit","shell"].map((name,i)=>({
  toolId:"tool_"+String(i+1).padStart(32,"0"),sourceKey:"workspace."+name,name,title:name,
 })),
};
function show(client:Partial<ManagementClient>,fullPage=false){
 resetLocaleForTesting();setLocale("en");
 render(<MemoryRouter initialEntries={[fullPage?"/sandboxes/"+sandbox.sandboxId:"/"]}>
  <Routes>
   <Route path="/" element={<SandboxToolsPanel client={client as ManagementClient} sandbox={sandbox}/>}/>
   <Route path="/sandboxes/:sandboxId" element={<SandboxDetailPage client={client as ManagementClient}/>}/>
  </Routes>
 </MemoryRouter>);
}
it("reports canonical Workspace capabilities without provisioning or retiring tools",async()=>{
 const getSandboxTools=vi.fn().mockResolvedValue(suite);
 show({getSandboxTools});
 expect(await screen.findByText("read")).toBeTruthy();
 expect(screen.getByText("shell")).toBeTruthy();
 expect(getSandboxTools).toHaveBeenCalledExactlyOnceWith(sandbox.sandboxId);
 expect(screen.queryByRole("button",{name:/Prepare|Retire|Adopt/i})).toBeNull();
});
it("reports an incompatible Sandbox without suggesting a fake Tool installation",async()=>{
 const incompatible:SandboxToolSuite={...suite,status:"incompatible",available:false,tools:[],
  issue:"Browser-only backend cannot provide filesystem or shell"};
 show({getSandboxTools:vi.fn().mockResolvedValue(incompatible)});
 expect(await screen.findByText(/Browser-only backend/)).toBeTruthy();
 expect(screen.queryByRole("button",{name:/Prepare|Retire|Adopt/i})).toBeNull();
});
it("loads Workspace capabilities only when the Sandbox detail tab is selected",async()=>{
 const getSandboxTools=vi.fn().mockResolvedValue(suite);
 show({getSandbox:vi.fn().mockResolvedValue(sandbox),getSandboxTools},true);
 expect(await screen.findByText("Docs Workspace")).toBeTruthy();
 expect(getSandboxTools).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button",{name:"Tools"}));
 expect(await screen.findByText("search")).toBeTruthy();
 expect(getSandboxTools).toHaveBeenCalledOnce();
});
