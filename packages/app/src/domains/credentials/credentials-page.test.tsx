import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { Credential, MenderClient } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale, t } from "../../i18n/i18n";
import { CredentialsPage } from "./credentials-page";
import { suggestedCredentialName } from "./credential-form";

const credential: Credential = {
  reference: "credential://openai/tunnel-api-key",
  namespace: "openai",
  name: "tunnel-api-key",
  displayName: "Platform API Key",
  source: "native",
  environmentVariable: "",
  createdAt: "2026-10-10T00:00:00Z",
  updatedAt: "2026-10-10T00:00:00Z",
};

function showPage(overrides: Partial<MenderClient> = {}) {
  const listCredentials = vi.fn().mockResolvedValue({ credentials: [] });
  const createNativeCredential = vi.fn().mockResolvedValue(credential);
  const createEnvironmentCredential = vi.fn().mockResolvedValue({ ...credential, source: "environment" });
  const client = {
    listCredentials,
    createNativeCredential,
    createEnvironmentCredential,
    ...overrides,
  } as MenderClient;
  render(<TooltipProvider><CredentialsPage client={client} /></TooltipProvider>);
  fireEvent.click(screen.getByRole("button", { name: t("Add credential") }));
  return { listCredentials, createNativeCredential, createEnvironmentCredential };
}

function fillNativeForm() {
  fireEvent.click(screen.getByText("Advanced settings"));
  fireEvent.change(screen.getByRole("textbox", { name: "Credential namespace" }), {
    target: { value: "OpenAI" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "Credential name" }), {
    target: { value: "Tunnel API Key" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), {
    target: { value: "Platform API Key" },
  });
  fireEvent.change(screen.getByLabelText("Credential secret"), {
    target: { value: "test-placeholder-secret" },
  });
}

beforeEach(() => {
  window.localStorage.clear();
  resetLocaleForTesting();
  setLocale("en");
});

afterEach(() => {
  cleanup();
  resetLocaleForTesting();
  window.localStorage.clear();
});

describe("Credential create validation and feedback", () => {
  it("creates from a Chinese display name without a required ASCII identity field", async () => {
    const { createNativeCredential } = showPage();
    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), {
      target: { value: "项目密钥" },
    });
    fireEvent.change(screen.getByLabelText("Credential secret"), {
      target: { value: "fixture-only-secret" },
    });
    expect(screen.getByText("Advanced settings").closest("details")?.open).toBe(false);
    const submit = screen.getByRole<HTMLButtonElement>("button", { name: "Create credential" });
    expect(submit.disabled).toBe(false);
    fireEvent.click(submit);
    await waitFor(() => expect(createNativeCredential).toHaveBeenCalledWith({
      namespace: "personal", name: suggestedCredentialName("项目密钥"),
      displayName: "项目密钥", secret: "fixture-only-secret",
    }));
  });

  it("chooses a free name from existing Credential inventory and preserves an explicit override", async () => {
    const existingName = suggestedCredentialName("项目密钥");
    const existing = { ...credential, namespace: "personal", name: existingName,
      displayName: "Older key", reference: `credential://personal/${existingName}` };
    const listCredentials = vi.fn().mockResolvedValue({ credentials: [existing] });
    showPage({ listCredentials });
    await screen.findByText("Older key");

    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), {
      target: { value: "项目密钥" },
    });
    fireEvent.click(screen.getByText("Advanced settings"));
    const name = screen.getByRole<HTMLInputElement>("textbox", { name: "Credential name" });
    expect(name.value).toBe(`${existingName}-2`);
    fireEvent.change(name, { target: { value: "manually-chosen" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), {
      target: { value: "新的显示名称" },
    });
    expect(name.value).toBe("manually-chosen");
    expect(screen.getByText("credential://personal/manually-chosen")).toBeTruthy();
  });

  it("derives a valid SSH key name from a non-Latin title while keeping the SSH namespace", async () => {
    const { createNativeCredential } = showPage();
    fireEvent.change(screen.getByRole("combobox", { name: "Credential source" }), {
      target: { value: "ssh-key" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "SSH credential name" }), {
      target: { value: "日本語 SSH 鍵" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Paste private key" }));
    fireEvent.change(screen.getByRole("textbox", { name: "SSH private key" }), {
      target: { value: "-----BEGIN OPENSSH PRIVATE KEY-----\nfixture-only\n-----END OPENSSH PRIVATE KEY-----" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));
    await waitFor(() => expect(createNativeCredential).toHaveBeenCalledWith({
      namespace: "ssh", name: suggestedCredentialName("日本語 SSH 鍵"),
      displayName: "日本語 SSH 鍵",
      secret: "-----BEGIN OPENSSH PRIVATE KEY-----\nfixture-only\n-----END OPENSSH PRIVATE KEY-----",
    }));
  });

  it("creates a personal credential from a readable name without requiring internal IDs", async () => {
    const { createNativeCredential } = showPage();
    const advanced = screen.getByText("Advanced settings").closest("details");
    expect(advanced?.open).toBe(false);
    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), {
      target: { value: "Project GitHub" },
    });
    fireEvent.change(screen.getByLabelText("Credential secret"), {
      target: { value: "fixture-secret-not-real" },
    });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create credential" }).disabled).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));
    await waitFor(() => expect(createNativeCredential).toHaveBeenCalledWith({
      namespace: "personal", name: "project-github",
      displayName: "Project GitHub", secret: "fixture-secret-not-real",
    }));
  });

  it("clears confidential material when switching credential types", () => {
    showPage();
    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), {
      target: { value: "Work Key" },
    });
    fireEvent.change(screen.getByLabelText("Credential secret"), {
      target: { value: "fixture-secret-not-real" },
    });
    fireEvent.change(screen.getByRole("combobox", { name: "Credential source" }), {
      target: { value: "environment" },
    });
    expect(screen.queryByDisplayValue("fixture-secret-not-real")).toBeNull();
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Credential display name" }).value).toBe("");
    fireEvent.change(screen.getByRole("combobox", { name: "Credential source" }), {
      target: { value: "native" },
    });
    expect(screen.getByLabelText<HTMLInputElement>("Credential secret").value).toBe("");
    fireEvent.click(screen.getByText("Advanced settings"));
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Credential namespace" }).value).toBe("personal");
  });

  it("opens identity details on duplicate reference while preserving the entered secret", async () => {
    const createNativeCredential = vi.fn().mockRejectedValue(
      new Error("credential: reference already exists"),
    );
    showPage({ createNativeCredential });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), {
      target: { value: "Repeated Key" },
    });
    fireEvent.change(screen.getByLabelText("Credential secret"), {
      target: { value: "fixture-secret-not-real" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));
    const form = within(screen.getByRole("dialog", { name: "Add credential" }));
    expect((await form.findByRole("alert")).textContent).toContain("already exists");
    expect(screen.getByText("Advanced settings").closest("details")?.open).toBe(true);
    expect(screen.getByLabelText<HTMLInputElement>("Credential secret").value).toBe("fixture-secret-not-real");
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Credential name" }).value).toBe("repeated-key");
  });
  it("creates a native SSH private key credential with multiline secret and fixed SSH namespace", async()=>{
    const sshCredential:Credential={...credential,
      reference:"credential://ssh/ssh-build",namespace:"ssh",name:"ssh-build",
      displayName:"SSH Build",source:"native",environmentVariable:"",
    };
    const listCredentials=vi.fn().mockResolvedValueOnce({credentials:[]})
      .mockResolvedValue({credentials:[sshCredential]});
    const {createNativeCredential,createEnvironmentCredential}=showPage({listCredentials});
    fireEvent.change(screen.getByRole("combobox",{name:"Credential source"}),{
      target:{value:"ssh-key"},
    });
    expect(screen.queryByRole("textbox",{name:"Credential namespace"})).toBeNull();
    expect(screen.queryByRole("textbox",{name:"Credential display name"})).toBeNull();
    fireEvent.change(screen.getByRole("textbox",{name:"SSH credential name"}),{
      target:{value:"SSH Build"},
    });
    const secret="-----BEGIN OPENSSH PRIVATE KEY-----\nfixture-only\n-----END OPENSSH PRIVATE KEY-----";
    fireEvent.click(screen.getByRole("button",{name:"Paste private key"}));
    const textarea=screen.getByRole<HTMLTextAreaElement>("textbox",{name:"SSH private key"});
    fireEvent.change(textarea,{target:{value:secret}});
    expect(screen.queryByText("credential://ssh/ssh-build")).toBeNull();
    fireEvent.click(screen.getByRole("button",{name:"Create credential"}));
    await waitFor(()=>expect(createNativeCredential).toHaveBeenCalledWith({
      namespace:"ssh",name:"ssh-build",displayName:"SSH Build",secret,
    }));
    expect(createEnvironmentCredential).not.toHaveBeenCalled();
    expect(await screen.findByText("SSH Build")).toBeTruthy();
    expect(screen.getByText("SSH private key")).toBeTruthy();
    expect(screen.queryByText("fixture-only")).toBeNull();
  });

  it("does not reflect rejected SSH private-key parser details near the secret",async()=>{
    const createNativeCredential=vi.fn().mockRejectedValue(
      new Error("ssh private key: invalid key format: private fixture"),
    );
    showPage({createNativeCredential});
    fireEvent.change(screen.getByRole("combobox",{name:"Credential source"}),{
      target:{value:"ssh-key"},
    });
    fireEvent.change(screen.getByRole("textbox",{name:"SSH credential name"}),{
      target:{value:"SSH Build"},
    });
    fireEvent.click(screen.getByRole("button",{name:"Paste private key"}));
    fireEvent.change(screen.getByRole("textbox",{name:"SSH private key"}),{
      target:{value:"-----BEGIN OPENSSH PRIVATE KEY-----\nprivate fixture"},
    });
    fireEvent.click(screen.getByRole("button",{name:"Create credential"}));
    const alert=await within(screen.getByRole("dialog",{name:"Add credential"}))
      .findByRole("alert");
    expect(alert.textContent).toContain("not a supported PEM or OpenSSH");
    expect(alert.textContent).not.toContain("invalid key format");
    expect(alert.textContent).not.toContain("private fixture");
  });

  it("canonicalizes the screenshot's mixed-case and spaced identifiers before creating", async () => {
    const listCredentials = vi.fn()
      .mockResolvedValueOnce({ credentials: [] })
      .mockResolvedValue({ credentials: [credential] });
    const { createNativeCredential } = showPage({ listCredentials });
    fillNativeForm();

    expect(screen.getByText("credential://openai/tunnel-api-key")).toBeTruthy();
    const submit = screen.getByRole<HTMLButtonElement>("button", { name: "Create credential" });
    expect(submit.disabled).toBe(false);
    fireEvent.click(submit);

    await waitFor(() => expect(createNativeCredential).toHaveBeenCalledWith({
      namespace: "openai",
      name: "tunnel-api-key",
      displayName: "Platform API Key",
      secret: "test-placeholder-secret",
    }));
    expect(await screen.findByText("Platform API Key")).toBeTruthy();
    expect(screen.queryByRole("dialog", { name: "Add credential" })).toBeNull();
  });

  it("imports a local RSA key file without displaying its contents and submits an optional passphrase", async () => {
    const createNativeCredential = vi.fn().mockResolvedValue(credential);
    showPage({ createNativeCredential });
    fireEvent.change(screen.getByRole("combobox", { name: "Credential source" }), {
      target: { value: "ssh-key" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "SSH credential name" }), {
      target: { value: "Rocky" },
    });
    const secret = "-----BEGIN RSA PRIVATE KEY-----\nLOCAL-TEST-ONLY\n-----END RSA PRIVATE KEY-----";
    const keyFile = new File([secret], "id_rsa", { type: "text/plain" });
    Object.defineProperty(keyFile, "text", { value: vi.fn().mockResolvedValue(secret) });
    fireEvent.change(screen.getByLabelText("SSH private key file"), {
      target: { files: [keyFile] },
    });
    expect(await screen.findByText(/Key loaded from id_rsa/)).toBeTruthy();
    expect(screen.queryByRole("textbox", { name: "SSH private key" })).toBeNull();
    expect(screen.queryByText("LOCAL-TEST-ONLY")).toBeNull();
    fireEvent.change(screen.getByLabelText("SSH key passphrase (optional)"), {
      target: { value: "import-only-passphrase" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));
    await waitFor(() => expect(createNativeCredential).toHaveBeenCalledWith({
      namespace: "ssh",
      name: "rocky",
      displayName: "Rocky",
      secret,
      sshKeyPassphrase: "import-only-passphrase",
    }));
  });

  it("rejects an oversized SSH key file before posting the credential", () => {
    const { createNativeCredential } = showPage();
    fireEvent.change(screen.getByRole("combobox", { name: "Credential source" }), {
      target: { value: "ssh-key" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "SSH credential name" }), {
      target: { value: "Rocky" },
    });
    const tooLarge = new File([new Uint8Array(64 * 1024 + 1)], "id_rsa");
    fireEvent.change(screen.getByLabelText("SSH private key file"), {
      target: { files: [tooLarge] },
    });
    expect(screen.getByText("SSH key file must be between 1 byte and 64 KiB.")).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create credential" }).disabled).toBe(true);
    expect(createNativeCredential).not.toHaveBeenCalled();
  });

  it("guides encrypted SSH key users without echoing parser errors", async () => {
    const createNativeCredential = vi.fn().mockRejectedValue(
      new Error("ssh private key: passphrase required"),
    );
    showPage({ createNativeCredential });
    fireEvent.change(screen.getByRole("combobox", { name: "Credential source" }), {
      target: { value: "ssh-key" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "SSH credential name" }), {
      target: { value: "Rocky" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Paste private key" }));
    fireEvent.change(screen.getByRole("textbox", { name: "SSH private key" }), {
      target: { value: "-----BEGIN OPENSSH PRIVATE KEY-----\nfixture" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));
    expect(await screen.findByText("This SSH private key is encrypted. Enter its passphrase.")).toBeTruthy();
    expect(screen.queryByText("passphrase required")).toBeNull();
  });

  it("disables submission for malformed canonical identifiers and oversized display names", () => {
    const { createNativeCredential } = showPage();
    fillNativeForm();
    const namespace = screen.getByRole<HTMLInputElement>("textbox", { name: "Credential namespace" });
    fireEvent.change(namespace, { target: { value: "OpenAI!" } });

    expect(namespace.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText("Invalid namespace or name")).toBeTruthy();
    expect(screen.getByText("Use valid namespace and name identifiers before creating a credential.")).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create credential" }).disabled).toBe(true);

    fireEvent.change(namespace, { target: { value: "OpenAI" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), {
      target: { value: "x".repeat(161) },
    });
    expect(screen.getByText("Display name is too long (maximum 160 bytes).")).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create credential" }).disabled).toBe(true);
    expect(createNativeCredential).not.toHaveBeenCalled();
  });

  it("validates the environment variable name before submitting a normalized reference", async () => {
    const { createEnvironmentCredential } = showPage();
    fireEvent.change(screen.getByRole("combobox", { name: "Credential source" }), {
      target: { value: "environment" },
    });
    fireEvent.click(screen.getByText("Advanced settings"));
    fireEvent.change(screen.getByRole("textbox", { name: "Credential namespace" }), {
      target: { value: "OpenAI" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential name" }), {
      target: { value: "Tunnel API Key" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Credential display name" }), {
      target: { value: "Platform API Key" },
    });
    const environment = screen.getByRole<HTMLInputElement>("textbox", { name: "Credential environment variable" });
    fireEvent.change(environment, { target: { value: "OPENAI KEY" } });

    expect(environment.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText("Enter a valid environment variable name.")).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Create credential" }).disabled).toBe(true);

    fireEvent.change(environment, { target: { value: "OPENAI_API_KEY" } });
    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));
    await waitFor(() => expect(createEnvironmentCredential).toHaveBeenCalledWith({
      namespace: "openai",
      name: "tunnel-api-key",
      displayName: "Platform API Key",
      environmentVariable: "OPENAI_API_KEY",
    }));
  });

  it("keeps conflicts inside the dialog and allows correction without losing the secret", async () => {
    const createNativeCredential = vi.fn()
      .mockRejectedValueOnce("create native credential: credential: reference already exists")
      .mockResolvedValueOnce(credential);
    showPage({ createNativeCredential });
    fillNativeForm();
    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));

    const dialog = within(screen.getByRole("dialog", { name: "Add credential" }));
    const alert = await dialog.findByRole("alert");
    expect(alert.textContent).toContain("A credential with this reference already exists.");
    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(screen.getByLabelText<HTMLInputElement>("Credential secret").value).toBe("test-placeholder-secret");

    fireEvent.change(screen.getByRole("textbox", { name: "Credential name" }), {
      target: { value: "Tunnel API Key New" },
    });
    expect(dialog.queryByRole("alert")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));
    await waitFor(() => expect(createNativeCredential).toHaveBeenCalledTimes(2));
    expect(createNativeCredential).toHaveBeenLastCalledWith(expect.objectContaining({
      name: "tunnel-api-key-new",
    }));
  });

  it("explains native keyring unavailability in Chinese without exposing an untrusted error", async () => {
    setLocale("zh-CN");
    const createNativeCredential = vi.fn()
      .mockRejectedValue(new Error("create native credential: store secret: credential: secret source unavailable: native keyring write failed"));
    showPage({ createNativeCredential });
    // The rendered form is localized; keep test data independent from the actual secret.
    fireEvent.click(screen.getByText("高级设置"));
    fireEvent.change(screen.getByRole("textbox", { name: "凭据命名空间" }), { target: { value: "OpenAI" } });
    fireEvent.change(screen.getByRole("textbox", { name: "凭据名称" }), { target: { value: "Tunnel API Key" } });
    fireEvent.change(screen.getByRole("textbox", { name: "凭据显示名称" }), { target: { value: "Platform API Key" } });
    fireEvent.change(screen.getByLabelText("凭据机密"), { target: { value: "test-placeholder-secret" } });
    fireEvent.click(screen.getByRole("button", { name: "创建凭据" }));

    const alert = await within(screen.getByRole("dialog")).findByRole("alert");
    expect(alert.textContent).toContain("系统凭据存储不可用");
    expect(alert.textContent).not.toContain("native keyring write failed");
    expect(alert.textContent).not.toContain("test-placeholder-secret");
    expect(screen.queryByText("发生意外错误")).toBeNull();
  });

  it("never reflects unknown backend error strings near the secret field", async () => {
    const createNativeCredential = vi.fn()
      .mockRejectedValue(new Error("untrusted backend: test-placeholder-secret"));
    showPage({ createNativeCredential });
    fillNativeForm();
    fireEvent.click(screen.getByRole("button", { name: "Create credential" }));

    const alert = await within(screen.getByRole("dialog", { name: "Add credential" })).findByRole("alert");
    expect(alert.textContent).toContain("Credential creation failed. Check the fields and try again.");
    expect(alert.textContent).not.toContain("test-placeholder-secret");
    expect(alert.textContent).not.toContain("untrusted backend");
  });
});
