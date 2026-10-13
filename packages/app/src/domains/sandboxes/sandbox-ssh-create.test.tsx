import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import type {MenderClient,Sandbox} from "@mender/sdk";
import {MemoryRouter,Route,Routes} from "react-router";
import {afterEach,describe,expect,it,vi} from "vitest";
import {CreateSandboxPage} from "./sandboxes-page";

afterEach(cleanup);

const hostKey="ssh-ed25519 "+"A".repeat(68);
const credential={
  reference:"credential://ssh/build",namespace:"ssh",name:"build",
  displayName:"SSH build key",source:"native",environmentVariable:"",
  createdAt:"2026-10-11T00:00:00Z",updatedAt:"2026-10-11T00:00:00Z",
};
const sandbox:Sandbox={
  sandboxId:"sbx_00000000000000000000000000000123",name:"Remote Dev",
  backend:"ssh_remote",isolation:"remote",state:"ready",localRoot:"",
  ownerKind:"local_user",ownerRef:"os:fixture",activeLeases:0,backendAvailable:true,
};
function renderSSH(overrides:Partial<MenderClient>={},builtin=false){
 const client:Partial<MenderClient>={
  getRuntimeGovernance:vi.fn().mockResolvedValue({backend:{restrictedHostAvailable:false}}),
  getSandboxBackends:vi.fn().mockResolvedValue({dockerAvailable:false,dockerStatus:"not detected",daytonaConfigured:false,daytonaStatus:"disabled"}),
  listCredentials:vi.fn().mockResolvedValue({credentials:[credential,{
    ...credential,reference:"credential://other/api",namespace:"other",source:"environment",
  }]}),
  createSSHRemoteSandbox:vi.fn().mockResolvedValue(sandbox),
  ...overrides,
 };
 const location=builtin?"/sandboxes/new?pluginId=io.mender.sandbox.ssh-remote":"/sandboxes/new";
 render(<MemoryRouter initialEntries={[location]}><Routes>
  <Route path="/sandboxes/new" element={<CreateSandboxPage client={client as MenderClient}/>}/>
  <Route path="/sandboxes/:sandboxId" element={<div>SSH Sandbox details</div>}/>
  <Route path="/plugins/:key" element={<div>SSH plugin details</div>}/>
 </Routes></MemoryRouter>);
 return client;
}

describe("SSH Remote Sandbox creation",()=>{
 it("shows the selected first-party SSH-only form and requires pinned host identity",async()=>{
  const client=renderSSH({},true);
  expect(await screen.findByRole("textbox",{name:"Sandbox name"})).toBeTruthy();
  expect(screen.queryByLabelText("Cached Docker image")).toBeNull();
  expect(screen.queryByLabelText("Mender Credential Reference")).toBeNull();
  expect(screen.getByText(/SSH Remote is not container or VM isolation/)).toBeTruthy();
  const review=screen.getByRole<HTMLButtonElement>("button",{name:"Review"});
  expect(review.disabled).toBe(true);
  fireEvent.change(screen.getByRole("textbox",{name:"Sandbox name"}),{target:{value:"Remote Dev"}});
  fireEvent.change(screen.getByRole("textbox",{name:"SSH host"}),{target:{value:"test.example.com"}});
  fireEvent.change(screen.getByRole("spinbutton",{name:"SSH port"}),{target:{value:"2222"}});
  fireEvent.change(screen.getByRole("textbox",{name:"SSH login user"}),{target:{value:"root"}});
  fireEvent.change(screen.getByRole("textbox",{name:"Remote workspace root"}),{target:{value:"/home/agent/work"}});
  fireEvent.change(screen.getByRole("textbox",{name:"Pinned SSH host public key"}),{target:{value:hostKey}});
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Review"}).disabled).toBe(true);
  fireEvent.change(screen.getByRole("textbox",{name:"SSH login user"}),{target:{value:"agent"}});
  const chooser=screen.getByRole<HTMLSelectElement>("combobox",{name:"SSH private key credential"});
  expect(screen.getByRole("option",{name:/SSH build key/})).toBeTruthy();
  expect(screen.queryByRole("option",{name:/credential:\/\/other\/api/})).toBeNull();
  fireEvent.change(chooser,{target:{value:credential.reference}});
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Review"}).disabled).toBe(false);
  // RSA host keys are allowed by UI and verified by backend during provisioning.
  const pin=screen.getByRole("textbox",{name:"Pinned SSH host public key"});
  fireEvent.change(pin,{target:{value:"ssh-rsa "+ "A".repeat(68)}});
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Review"}).disabled).toBe(false);
  fireEvent.change(pin,{target:{value:"ssh-dss "+ "A".repeat(68)}});
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Review"}).disabled).toBe(true);
  fireEvent.change(pin,{target:{value:hostKey}});
  fireEvent.click(screen.getByRole("button",{name:"Review"}));
  expect(screen.getByText("Remote account (not isolated)")).toBeTruthy();
  expect(screen.getByText("test.example.com")).toBeTruthy();
  expect(screen.queryByText("Resource limits")).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Create sandbox"}));
  await waitFor(()=>expect(client.createSSHRemoteSandbox).toHaveBeenCalledWith({
    name:"Remote Dev",host:"test.example.com",port:2222,user:"agent",root:"/home/agent/work",
    hostPublicKey:hostKey,credentialRef:credential.reference,
  }));
  expect(await screen.findByText("SSH Sandbox details")).toBeTruthy();
 });
 it("does not submit when the SSH credential is missing",async()=>{
  const client=renderSSH({
    listCredentials:vi.fn().mockResolvedValue({credentials:[]}),
  });
  fireEvent.click(await screen.findByRole("button",{name:/SSH Remote/}));
  expect(await screen.findByText(/No SSH key credentials/)).toBeTruthy();
  expect(screen.getByRole<HTMLButtonElement>("button",{name:"Review"}).disabled).toBe(true);
  expect(client.createSSHRemoteSandbox).not.toHaveBeenCalled();
 });
 it("returns to its selected plugin instead of showing unrelated configuration",async()=>{
  renderSSH({},true);
  fireEvent.click(await screen.findByRole("button",{name:"Back"}));
  expect(await screen.findByText("SSH plugin details")).toBeTruthy();
 });
});
