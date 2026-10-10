import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import type {MenderClient,Sandbox} from "@mender/sdk";
import {MemoryRouter,Route,Routes} from "react-router";
import {afterEach,describe,expect,it,vi} from "vitest";

import {SandboxDetailPage} from "./sandboxes-page";

afterEach(()=>{cleanup();vi.restoreAllMocks();});
const local:Sandbox={
 sandboxId:"sbx_0123456789abcdef0123456789abcdef",
 name:"Test Sandbox",backend:"local_workspace",isolation:"host",state:"ready",
 localRoot:"/tmp/test-sandbox",ownerKind:"local_user",ownerRef:"owner",activeLeases:0,backendAvailable:true,
};
function mount(client:Partial<MenderClient>){
 const effective=client as MenderClient;
 render(<MemoryRouter initialEntries={[`/sandboxes/${local.sandboxId}`]}><Routes>
  <Route path="/sandboxes/:sandboxId" element={<SandboxDetailPage client={effective}/>} />
 </Routes></MemoryRouter>);
}

describe("Sandbox lifecycle management",()=>{
 it("offers Start/Stop only for ready/stopped local resources and updates state",async()=>{
  const stopSandbox=vi.fn().mockResolvedValue({...local,state:"stopped"});
  const startSandbox=vi.fn().mockResolvedValue({...local,state:"ready"});
  mount({getSandbox:vi.fn().mockResolvedValue(local),stopSandbox,startSandbox});
  fireEvent.click(await screen.findByRole("button",{name:"Manage sandbox"}));
  fireEvent.click(await screen.findByRole("button",{name:"Stop sandbox"}));
  await waitFor(()=>expect(stopSandbox).toHaveBeenCalledWith(local.sandboxId));
  const startButton=await screen.findByRole("button",{name:"Start sandbox"});
  fireEvent.click(startButton);
  await waitFor(()=>expect(startSandbox).toHaveBeenCalledWith(local.sandboxId));
  expect(await screen.findByRole("button",{name:"Stop sandbox"})).toBeTruthy();
 });
 it("disables lifecycle actions when a persisted execution lease is active",async()=>{
  const stopSandbox=vi.fn();const destroySandbox=vi.fn();
  mount({getSandbox:vi.fn().mockResolvedValue({...local,activeLeases:1}),stopSandbox,destroySandbox});
  fireEvent.click(await screen.findByRole("button",{name:"Manage sandbox"}));
  const stop=await screen.findByRole<HTMLButtonElement>("button",{name:"Stop sandbox"});
  expect(stop.disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Destroy sandbox"}).disabled).toBe(true);
  expect(screen.getAllByText("Cannot change lifecycle while a Sandbox execution lease is active.").length).toBeGreaterThanOrEqual(1);
  expect(stopSandbox).not.toHaveBeenCalled();expect(destroySandbox).not.toHaveBeenCalled();
 });
 it("requires confirmation for tombstoning and never promises to erase host files",async()=>{
  const confirm=vi.spyOn(window,"confirm").mockReturnValue(false);
  const destroySandbox=vi.fn().mockResolvedValue({...local,state:"destroyed"});
  mount({getSandbox:vi.fn().mockResolvedValue(local),destroySandbox});
  fireEvent.click(await screen.findByRole("button",{name:"Manage sandbox"}));
  const destroy=await screen.findByRole("button",{name:"Destroy sandbox"});
  fireEvent.click(destroy);
  expect(confirm).toHaveBeenCalledOnce();expect(destroySandbox).not.toHaveBeenCalled();
  confirm.mockReturnValue(true);
  fireEvent.click(destroy);
  await waitFor(()=>expect(destroySandbox).toHaveBeenCalledWith(local.sandboxId));
  expect(await screen.findByText("Host files remain untouched after Sandbox destruction.")).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Destroy sandbox"})).toBeNull();
 });
 it("keeps the security explanation separate from the sandbox identity preview",async()=>{
  mount({getSandbox:vi.fn().mockResolvedValue(local)});
  expect(await screen.findByText("Test Sandbox")).toBeTruthy();
  expect(screen.getByText(local.sandboxId)).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Stop sandbox"})).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Security boundary"}));
  expect(screen.getByText("Host mode is not isolated and uses the Runtime user's local file and process permissions.")).toBeTruthy();
  expect(screen.queryByText(local.sandboxId)).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Sandbox overview"}));
  expect(screen.getByText(local.sandboxId)).toBeTruthy();
 });

 it("never displays unsupported lifecycle actions on unregistered E2B backends",async()=>{
  mount({getSandbox:vi.fn().mockResolvedValue({...local,backend:"e2b",isolation:"remote"})});
  expect(await screen.findByText("Test Sandbox")).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Start sandbox"})).toBeNull();
  expect(screen.queryByRole("button",{name:"Stop sandbox"})).toBeNull();
  expect(screen.queryByRole("button",{name:"Destroy sandbox"})).toBeNull();
 });
});
