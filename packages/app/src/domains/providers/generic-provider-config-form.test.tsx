import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ProviderPlugin } from "@mender/sdk";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useState } from "react";
import { resetLocaleForTesting, setLocale } from "../../i18n/i18n";
import { GenericProviderConfigForm } from "./generic-provider-config-form";

function FormHarness({ schema, initial }: {
  schema: ProviderPlugin["configSchema"];
  initial: Record<string, unknown>;
}) {
  const [value, setValue] = useState(initial);
  const [draftError, setDraftError] = useState<string | null>(null);
  return (
    <div>
      <GenericProviderConfigForm
        schema={schema}
        uiSchema={undefined}
        value={value}
        credentials={[]}
        onChange={setValue}
        onDraftErrorChange={setDraftError}
      />
      <button type="button" onClick={() => setValue({ restored: true, records: [9], nested: { valid: 6 } })}>Load another config</button>
      <button type="button" disabled={draftError !== null}>Save configuration</button>
      <output data-testid="form-value">{JSON.stringify(value)}</output>
    </div>
  );
}

beforeEach(() => { resetLocaleForTesting(); setLocale("en"); });
afterEach(() => { cleanup(); resetLocaleForTesting(); });

describe("dynamic JSON schema draft integrity", () => {
  it("retains invalid raw JSON locally, blocks Save, and never submits its previous valid object", () => {
    render(<FormHarness schema={undefined} initial={{ endpoint: "https://a.example" }}/>);
    const editor = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Provider configuration" });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(false);
    fireEvent.change(editor, { target: { value: '{"endpoint":' } });
    expect(editor.value).toBe('{"endpoint":');
    expect(editor.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByRole("alert").textContent).toContain("Enter valid JSON.");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(true);
    expect(screen.getByTestId("form-value").textContent).toContain("https://a.example");
    fireEvent.change(editor, { target: { value: '{"endpoint":"https://b.example"}' } });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(false);
    expect(screen.getByTestId("form-value").textContent).toContain("https://b.example");
  });

  it("tracks invalid JSON arrays and additional JSON properties independently", () => {
    const schema = { type: "object", properties: { records: { type: "array" }, title: {type:"string"} } };
    render(<FormHarness schema={schema} initial={{ records: [1], flag: true, title: "Before" }}/>);
    const list = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Records" });
    const additional = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Additional properties" });
    fireEvent.change(list, { target: { value: "[1," } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(true);
    fireEvent.change(screen.getByRole("textbox", { name: "Title" }), { target: { value: "After" } });
    expect(list.value).toBe("[1,");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(true);
    fireEvent.change(additional, { target: { value: '{"flag":' } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(true);
    fireEvent.change(list, { target: { value: "[1,2]" } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(true);
    fireEvent.change(additional, { target: { value: '{"flag":false}' } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(false);
    expect(screen.getByTestId("form-value").textContent).toContain('"flag":false');
  });

  it("preserves formatting while typing a valid value, but resyncs when the parent loads a different config", () => {
    render(<FormHarness schema={{ type: "object", properties: { nested: true } }} initial={{ nested: { valid: 1 } }}/>);
    const editor = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Nested" });
    const formatted = '{  "valid": 2 }';
    fireEvent.change(editor, { target: { value: formatted } });
    expect(editor.value).toBe(formatted);
    fireEvent.change(editor, { target: { value: '{ "valid":' } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Load another config" }));
    expect(editor.value).toBe('{\n  "valid": 6\n}');
    expect(editor.getAttribute("aria-invalid")).toBe("false");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(false);
  });

  it("distinguishes a cleared optional array from a deliberate empty JSON array", () => {
    render(<FormHarness schema={{ type:"object", properties: { records: { type:"array" } } }} initial={{ records: [1] }}/>);
    const editor = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Records" });
    fireEvent.change(editor, { target: { value: "" } });
    expect(editor.value).toBe("");
    expect(screen.getByTestId("form-value").textContent).toBe("{}");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(false);
    fireEvent.change(editor, { target: { value: "[]" } });
    expect(editor.value).toBe("[]");
    expect(screen.getByTestId("form-value").textContent).toBe('{"records":[]}');
  });

  it("treats non-object raw JSON as an invalid draft and allows recovery", () => {
    render(<FormHarness schema={true} initial={{ restored: true }}/>);
    const editor = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Provider configuration" });
    fireEvent.change(editor, { target: { value: "[1,2]" } });
    expect(screen.getByRole("alert").textContent).toContain("Provider configuration must be a JSON object.");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(true);
    fireEvent.change(editor, { target: { value: '{"restored":true}' } });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save configuration" }).disabled).toBe(false);
  });
});
