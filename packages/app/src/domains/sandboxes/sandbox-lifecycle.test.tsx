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
  const stop=await screen.findByRole<HTMLButtonElement>("button",{name:"Stop sandbox"});
  expect(stop.disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Destroy sandbox"}).disabled).toBe(true);
  expect(screen.getByText("Cannot change lifecycle while a Sandbox execution lease is active.")).toBeTruthy();
  expect(stopSandbox).not.toHaveBeenCalled();expect(destroySandbox).not.toHaveBeenCalled();
 });
 it("requires confirmation for tombstoning and never promises to erase host files",async()=>{
  const confirm=vi.spyOn(window,"confirm").mockReturnValue(false);
  const destroySandbox=vi.fn().mockResolvedValue({...local,state:"destroyed"});
  mount({getSandbox:vi.fn().mockResolvedValue(local),destroySandbox});
  const destroy=await screen.findByRole("button",{name:"Destroy sandbox"});
  fireEvent.click(destroy);
  expect(confirm).toHaveBeenCalledOnce();expect(destroySandbox).not.toHaveBeenCalled();
  confirm.mockReturnValue(true);
  fireEvent.click(destroy);
  await waitFor(()=>expect(destroySandbox).toHaveBeenCalledWith(local.sandboxId));
  expect(await screen.findByText("Host files remain untouched after Sandbox destruction.")).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Destroy sandbox"})).toBeNull();
 });
 it("never displays unsupported lifecycle actions on unregistered E2B backends",async()=>{
  mount({getSandbox:vi.fn().mockResolvedValue({...local,backend:"e2b",isolation:"remote"})});
  expect(await screen.findByText("Test Sandbox")).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Start sandbox"})).toBeNull();
  expect(screen.queryByRole("button",{name:"Stop sandbox"})).toBeNull();
  expect(screen.queryByRole("button",{name:"Destroy sandbox"})).toBeNull();
 });
});
