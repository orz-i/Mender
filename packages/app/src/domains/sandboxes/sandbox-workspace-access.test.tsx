import { cleanup,fireEvent,render,screen,waitFor } from "@testing-library/react";
import type { ManagementClient,Profile,Sandbox,ToolSummary,WorkspaceRecord,ProfileToolTarget } from "@mender/sdk";
import { afterEach,expect,it,vi } from "vitest";
import { resetLocaleForTesting,setLocale } from "../../i18n/i18n";
import { SandboxWorkspaceAccess } from "./sandbox-workspace-access";

const sandbox:Sandbox={sandboxId:"sbx_11111111111111111111111111111111",name:"Project Sandbox",backend:"local_workspace",localRoot:"/tmp/example-project",isolation:"host",state:"ready",ownerKind:"local_user",ownerRef:"os:501",activeLeases:0,backendAvailable:true};
const workspace:WorkspaceRecord={workspaceId:"wrk_22222222222222222222222222222222",displayName:"Project Sandbox",verifiedRoot:sandbox.localRoot,createdAt:"2026-11-10T00:00:00Z"};
const tool:ToolSummary={rank:1,toolId:"tool_33333333333333333333333333333333",sourceId:"native:workspace",sourceKind:"native",name:"read",title:"Read file",description:"Read reviewed Workspace"};
const profile:Profile={profileId:"prof_44444444444444444444444444444444",slug:"my-profile",displayName:"My Profile",revision:0,skills:[],tools:[{toolId:tool.toolId,alias:"",titleOverride:"",descriptionOverride:"",policyConstraint:"inherit",concurrencyConstraint:"inherit",credentialRefs:[]}]};
function setup(hasCapability:boolean){
 let workspaces:WorkspaceRecord[]=[];
 let attached=false;
 let grant=false;
 let targets:ProfileToolTarget[]=[];
 const createWorkspace=vi.fn().mockImplementation(()=>{workspaces=[workspace];return Promise.resolve(workspace);});
 const linkWorkspaceSandbox=vi.fn().mockImplementation(()=>{attached=true;return Promise.resolve();});
 const grantWorkspaceProfile=vi.fn().mockImplementation(()=>{grant=true;return Promise.resolve();});
 const revokeWorkspaceProfile=vi.fn().mockImplementation(()=>{grant=false;targets=[];return Promise.resolve();});
 const addProfileToolTarget=vi.fn().mockImplementation((_slug:string,target:{toolId:string;workspaceId:string;sandboxId:string})=>{
   const result={profileId:profile.profileId,...target};targets=[result];return Promise.resolve(result);
 });
 const client={
  listWorkspaces:vi.fn().mockImplementation(()=>Promise.resolve({workspaces})),
  listProfiles:vi.fn().mockResolvedValue({profiles:[profile]}),
  getSandboxProfileGrants:vi.fn().mockResolvedValue({sandboxId:sandbox.sandboxId,grants:hasCapability?[{profileId:profile.profileId,capabilities:["file.read"]}]:[]}),
  browseTools:vi.fn().mockResolvedValue({tools:[tool]}),
  createWorkspace,linkWorkspaceSandbox,
  listWorkspacePlacements:vi.fn().mockImplementation(()=>Promise.resolve({placements:attached?[{workspaceId:workspace.workspaceId,sandboxId:sandbox.sandboxId,backendRoot:sandbox.localRoot}]:[]})),
  listWorkspaceProfileGrants:vi.fn().mockImplementation(()=>Promise.resolve({profileIds:grant?[profile.profileId]:[]})),
  listProfileToolTargets:vi.fn().mockImplementation(()=>Promise.resolve({targets})),
  grantWorkspaceProfile,revokeWorkspaceProfile,addProfileToolTarget,
 } as unknown as ManagementClient;
 resetLocaleForTesting();setLocale("en");
 render(<SandboxWorkspaceAccess client={client} sandbox={sandbox}/>);
 return {client,createWorkspace,linkWorkspaceSandbox,grantWorkspaceProfile,revokeWorkspaceProfile,addProfileToolTarget};
}
afterEach(()=>{cleanup();resetLocaleForTesting();});
it("registers and attaches a Workspace without implicitly granting Profile or tool access",async()=>{
 const {createWorkspace,linkWorkspaceSandbox,grantWorkspaceProfile,addProfileToolTarget}=setup(false);
 fireEvent.click(screen.getByRole("button",{name:"Configure Workspace execution"}));
 fireEvent.click(await screen.findByRole("button",{name:"Register this Workspace"}));
 await waitFor(()=>expect(createWorkspace).toHaveBeenCalledExactlyOnceWith({displayName:sandbox.name,verifiedRoot:sandbox.localRoot}));
 fireEvent.click(await screen.findByRole("button",{name:"Attach Sandbox"}));
 await waitFor(()=>expect(linkWorkspaceSandbox).toHaveBeenCalledWith(workspace.workspaceId,sandbox.sandboxId));
 expect(grantWorkspaceProfile).not.toHaveBeenCalled();
 expect(addProfileToolTarget).not.toHaveBeenCalled();
 expect(await screen.findByText("Workspace access not granted")).toBeTruthy();
 expect((await screen.findByRole<HTMLButtonElement>("button",{name:"Authorize exact tool target"})).disabled).toBe(true);
});
it("requires an explicit Workspace grant and matching Sandbox capability before enabling an exact tool target",async()=>{
 const {createWorkspace,linkWorkspaceSandbox,grantWorkspaceProfile,revokeWorkspaceProfile,addProfileToolTarget}=setup(true);
 fireEvent.click(screen.getByRole("button",{name:"Configure Workspace execution"}));
 fireEvent.click(await screen.findByRole("button",{name:"Register this Workspace"}));
 await waitFor(()=>expect(createWorkspace).toHaveBeenCalledOnce());
 fireEvent.click(await screen.findByRole("button",{name:"Attach Sandbox"}));
 await waitFor(()=>expect(linkWorkspaceSandbox).toHaveBeenCalledOnce());
 expect((await screen.findByRole<HTMLButtonElement>("button",{name:"Authorize exact tool target"})).disabled).toBe(true);
 fireEvent.click(screen.getByRole("button",{name:"Grant Workspace access"}));
 await waitFor(()=>expect(grantWorkspaceProfile).toHaveBeenCalledWith(workspace.workspaceId,profile.profileId));
 const add=await screen.findByRole<HTMLButtonElement>("button",{name:"Authorize exact tool target"});
 await waitFor(()=>expect(add.disabled).toBe(false));
 fireEvent.click(add);
 await waitFor(()=>expect(addProfileToolTarget).toHaveBeenCalledWith(profile.slug,{toolId:tool.toolId,workspaceId:workspace.workspaceId,sandboxId:sandbox.sandboxId}));
 expect(await screen.findByText("Target authorized")).toBeTruthy();
 fireEvent.click(screen.getByRole("button",{name:"Revoke Workspace access"}));
 await waitFor(()=>expect(revokeWorkspaceProfile).toHaveBeenCalledOnce());
 expect(await screen.findByText("Target not authorized")).toBeTruthy();
});
