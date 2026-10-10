import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import type {MenderClient,Sandbox} from "@mender/sdk";
import {MemoryRouter,Routes,Route} from "react-router";
import {afterEach,describe,expect,it,vi} from "vitest";
import {CreateSandboxPage} from "./sandboxes-page";

afterEach(cleanup);
const digest=`docker.io/library/python@sha256:${"a".repeat(64)}`;
const reference="credential://daytona/api-key";
const sample:Sandbox={sandboxId:"sbx_0123456789abcdef0123456789abcdef",name:"Remote Sandbox",backend:"daytona",isolation:"remote",state:"unknown",localRoot:"",ownerKind:"local_user",ownerRef:"owner",activeLeases:0,backendAvailable:true};
function renderDaytona(client:Partial<MenderClient>,configured=true){
 const provided:Partial<MenderClient>={
  getRuntimeGovernance:vi.fn().mockResolvedValue({backend:{restrictedHostAvailable:false}}),
  getSandboxBackends:vi.fn().mockResolvedValue({dockerAvailable:false,dockerStatus:"disabled",daytonaConfigured:configured,daytonaStatus:configured?"Experimental Daytona enabled by operator; credential and live provider validation required; may incur charges":"Experimental Daytona disabled; local operator must set MENDER_DAYTONA_EXPERIMENTAL=enabled before Runtime startup"}),
  listCredentials:vi.fn().mockResolvedValue({credentials:[]}),
  ...client,
 };
 render(<MemoryRouter initialEntries={["/sandboxes/new"]}><Routes><Route path="/sandboxes/new" element={<CreateSandboxPage client={provided as MenderClient}/>}/><Route path="/sandboxes/:sandboxId" element={<div>Daytona detail</div>}/></Routes></MemoryRouter>);
}
describe("Daytona Sandbox creation",()=>{
 it("does not offer remote provisioning when adapter is disabled",async()=>{
  const createDaytonaSandbox=vi.fn();renderDaytona({createDaytonaSandbox},false);
  expect(await screen.findByText(/MENDER_DAYTONA_EXPERIMENTAL=enabled/)).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Daytona"})).toBeNull();
  expect(createDaytonaSandbox).not.toHaveBeenCalled();
 });
 it("refuses remote provisioning without a chosen Mender credential",async()=>{
  const createDaytonaSandbox=vi.fn();renderDaytona({createDaytonaSandbox});
  fireEvent.click(await screen.findByRole("button",{name:/Daytona/}));
  expect(screen.getByText(/No credentials available/)).toBeTruthy();
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Next"}).disabled).toBe(true);
  expect(createDaytonaSandbox).not.toHaveBeenCalled();
 });
 it("requires valid remote limits before the authority review",async()=>{
  const createDaytonaSandbox=vi.fn();
  renderDaytona({listCredentials:vi.fn().mockResolvedValue({credentials:[{reference,displayName:"Daytona API"}]}),createDaytonaSandbox});
  fireEvent.click(await screen.findByRole("button",{name:/Daytona/}));
  fireEvent.change(screen.getByRole("textbox",{name:"Sandbox name"}),{target:{value:"Guarded Remote"}});
  fireEvent.change(screen.getByRole("textbox",{name:"Immutable image digest"}),{target:{value:digest}});
  fireEvent.change(screen.getByRole("combobox",{name:"Mender Credential Reference"}),{target:{value:reference}});
  fireEvent.click(screen.getByRole("button",{name:"Next"}));
  fireEvent.change(screen.getByRole("spinbutton",{name:"TTL (minutes)"}),{target:{value:"1"}});
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Review"}).disabled).toBe(true);
  fireEvent.change(screen.getByRole("spinbutton",{name:"TTL (minutes)"}),{target:{value:"60"}});
  fireEvent.click(screen.getByRole("button",{name:"Review"}));
  expect(screen.getByText("60")).toBeTruthy();
  expect(createDaytonaSandbox).not.toHaveBeenCalled();
 });
 it("passes only credential reference, fixed region, immutable image and provider TTL, then reviews unknown state",async()=>{
  const createDaytonaSandbox=vi.fn().mockResolvedValue(sample);
  renderDaytona({listCredentials:vi.fn().mockResolvedValue({credentials:[{reference,displayName:"Daytona API"}]}),createDaytonaSandbox});
  fireEvent.click(await screen.findByRole("button",{name:/Daytona/}));
  fireEvent.change(screen.getByRole("textbox",{name:"Sandbox name"}),{target:{value:"Remote Sandbox"}});
  fireEvent.change(screen.getByRole("textbox",{name:"Immutable image digest"}),{target:{value:digest}});
  fireEvent.change(screen.getByRole("combobox",{name:"Mender Credential Reference"}),{target:{value:reference}});
  expect(screen.queryByRole("spinbutton",{name:"Remote CPUs"})).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Next"}));
  expect(screen.getByRole("spinbutton",{name:"Remote CPUs"})).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Review"}));
  expect(screen.getByText(/TTL is not a USD spending cap/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Create sandbox"}));
  await waitFor(()=>expect(createDaytonaSandbox).toHaveBeenCalledWith({name:"Remote Sandbox",credentialRef:reference,imageDigest:digest,target:"us",cpu:1,memoryGiB:2,diskGiB:3,ttlMinutes:30}));
  expect(await screen.findByText("Daytona detail")).toBeTruthy();
  expect(createDaytonaSandbox).toHaveBeenCalledTimes(1);
 });
});
