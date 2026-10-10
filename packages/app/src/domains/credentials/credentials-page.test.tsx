import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { Credential, MenderClient } from "@mender/sdk";
import { TooltipProvider } from "@mender/ui/components/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetLocaleForTesting, setLocale, t } from "../../i18n/i18n";
import { CredentialsPage } from "./credentials-page";

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
