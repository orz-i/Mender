import { cleanup,fireEvent,render,screen,waitFor } from "@testing-library/react";
import type { ManagementClient,Profile,Sandbox,ToolSummary,ProfileToolTarget } from "@mender/sdk";
import { afterEach,expect,it,vi } from "vitest";
import { resetLocaleForTesting,setLocale } from "../../i18n/i18n";
import { SandboxJobsAccess } from "./sandbox-jobs-access";

const sandbox:Sandbox={sandboxId:"sbx_11111111111111111111111111111111",name:"SSH Jobs sandbox",backend:"ssh_remote",localRoot:"/workspace",isolation:"remote",state:"ready",ownerKind:"local_user",ownerRef:"os:501",activeLeases:0,backendAvailable:true};
const tool:ToolSummary={rank:1,toolId:"tool_22222222222222222222222222222222",sourceId:"native:jobs",sourceKind:"native",name:"submit",title:"Submit SSH Job",description:"Bounded SSH execution"};
const profile:Profile={profileId:"prof_33333333333333333333333333333333",slug:"ssh-jobs",displayName:"SSH Jobs Profile",revision:0,skills:[],tools:[{toolId:tool.toolId,alias:"",titleOverride:"",descriptionOverride:"",policyConstraint:"inherit",concurrencyConstraint:"inherit",credentialRefs:[]}]};
function mount(capabilities:string[]){
 let current:ProfileToolTarget[]=[];
 const addProfileToolTarget=vi.fn().mockImplementation((_slug:string,value:Partial<ProfileToolTarget>)=>{
  const item={profileId:profile.profileId,...value} as ProfileToolTarget;current=[item];return Promise.resolve(item);
 });
 const removeProfileToolTarget=vi.fn().mockImplementation(()=>{current=[];return Promise.resolve();});
 const client={
  listProfiles:vi.fn().mockResolvedValue({profiles:[profile]}),
  browseTools:vi.fn().mockResolvedValue({tools:[tool]}),
  getSandboxProfileGrants:vi.fn().mockResolvedValue({sandboxId:sandbox.sandboxId,grants:capabilities.length?[{profileId:profile.profileId,capabilities}]:[]}),
  listProfileToolTargets:vi.fn().mockImplementation(()=>Promise.resolve({targets:current})),
  addProfileToolTarget,removeProfileToolTarget,
 } as unknown as ManagementClient;
 resetLocaleForTesting();setLocale("en");render(<SandboxJobsAccess client={client} sandbox={sandbox}/>);
 return {client,addProfileToolTarget,removeProfileToolTarget};
}
afterEach(()=>{cleanup();resetLocaleForTesting();});
it("does not create a Profile Job target without a separate Sandbox Job capability grant",async()=>{
 const {addProfileToolTarget}=mount([]);
 fireEvent.click(screen.getByRole("button",{name:"Configure SSH Jobs targets"}));
 expect(await screen.findByText("Submit SSH Job")).toBeTruthy();
 expect(screen.getByText("Grant this Job capability in the Sandbox Profile access tab first.")).toBeTruthy();
 expect(screen.getByRole<HTMLButtonElement>("button",{name:"Authorize exact Job target"}).disabled).toBe(true);
 expect(addProfileToolTarget).not.toHaveBeenCalled();
});
it("creates and retires only the selected canonical native Job target, never submits a remote job",async()=>{
 const {addProfileToolTarget,removeProfileToolTarget}=mount(["job.submit"]);
 fireEvent.click(screen.getByRole("button",{name:"Configure SSH Jobs targets"}));
 fireEvent.click(await screen.findByRole("button",{name:"Authorize exact Job target"}));
 await waitFor(()=>expect(addProfileToolTarget).toHaveBeenCalledWith(profile.slug,{
  toolId:tool.toolId,workspaceId:"",sandboxId:sandbox.sandboxId,
 }));
 expect(await screen.findByText("Target authorized")).toBeTruthy();
 fireEvent.click(screen.getByRole("button",{name:"Remove Job target"}));
 await waitFor(()=>expect(removeProfileToolTarget).toHaveBeenCalledWith(profile.slug,tool.toolId,sandbox.sandboxId,""));
 expect(await screen.findByText("Target not authorized")).toBeTruthy();
});
