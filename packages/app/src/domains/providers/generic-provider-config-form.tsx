import { useState } from "react";

import type { Credential, ProviderPlugin, Sandbox } from "@mender/sdk";
import { Link } from "react-router";
import { Input } from "@mender/ui/components/input";
import { Textarea } from "@mender/ui/components/textarea";

import {
  MAX_SCHEMA_DEPTH,
  additionalPropertiesSchema,
  cloneJSONValue,
  enumIndex,
  enumLabel,
  genericSchemaSupportError,
  humanize,
  isRecord,
  numberValue,
  schemaProperties,
  schemaType,
  stringArray,
  stringValue,
  uiHint,
} from "./generic-provider-config-schema";
import type {
  ConfigSchema,
  ConfigValue,
  SchemaObject,
  UIHint,
} from "./generic-provider-config-schema";

import { t } from "../../i18n/i18n";

export function GenericProviderConfigForm({
  schema,
  uiSchema,
  value,
  credentials,
  sandboxes,
  pluginId,
  onChange,
}: {
  schema: ProviderPlugin["configSchema"];
  uiSchema: ProviderPlugin["uiSchema"];
  value: ConfigValue;
  credentials: Credential[];
  sandboxes?: Sandbox[];
  pluginId?: string | undefined;
  onChange: (value: ConfigValue) => void;
}) {
  // The built-in Workspace plugin uses a resource selector instead of a raw
  // sandboxId input. LLMs/Agents must never be able to select execution placement.
  if (pluginId === "io.mender.provider.workspace") {
    const available = (sandboxes ?? []).filter((item) => item.state === "ready" && item.backendAvailable && (item.backend === "local_workspace" || item.backend === "docker" || item.backend === "daytona"));
    const selected = typeof value.sandboxId === "string" ? value.sandboxId : "";
    return <div className="space-y-3">
      <label className="block space-y-2 text-sm">
        <span className="font-medium">{t("Sandbox")}</span>
        <select aria-label={t("Sandbox")} className="h-9 w-full rounded-lg border border-input bg-background px-3" value={selected} onChange={(e) => onChange({...value,sandboxId:e.target.value})}>
          <option value="">{t("Choose a sandbox")}</option>
          {!available.some((item)=>item.sandboxId===selected) && selected ? <option value={selected} disabled>{t("Unavailable sandbox")}</option> : null}
          {available.map((item)=><option key={item.sandboxId} value={item.sandboxId}>{item.name} · {item.backend === "docker" ? t("Docker") : item.backend === "daytona" ? t("Daytona") : t(item.isolation === "host" ? "Host (not isolated)" : "Restricted host")}</option>)}
        </select>
      </label>
      <p className="text-xs leading-5 text-muted-foreground">{t("Workspace Tools share the selected Sandbox filesystem and its fixed isolation boundary.")}</p>
      {available.length === 0 ? <p className="text-xs text-muted-foreground">{t("No ready Sandbox is available on this Runtime host.")}</p> : null}
      <Link to="/sandboxes/new" className="inline-flex text-xs font-medium text-primary underline-offset-4 hover:underline">{t("Create sandbox")}</Link>
    </div>;
  }
  const supportError = genericSchemaSupportError(schema);
  if (supportError) {
    return <SchemaNotice>{supportError}</SchemaNotice>;
  }

  if (schema === undefined || schema === true) {
    return (
      <RawObjectEditor
        label={t("Provider configuration")}
        value={value}
        onChange={onChange}
      />
    );
  }

  if (schema === false) {
    return <SchemaNotice>{t("This plugin schema rejects every configuration.")}</SchemaNotice>;
  }

  return (
    <ObjectFields
      schema={schema}
      uiSchema={uiSchema}
      value={value}
      credentials={credentials}
      depth={0}
      onChange={onChange}
    />
  );
}

function ObjectFields({
  schema,
  uiSchema,
  value,
  credentials,
  depth,
  onChange,
}: {
  schema: SchemaObject;
  uiSchema: ProviderPlugin["uiSchema"];
  value: ConfigValue;
  credentials: Credential[];
  depth: number;
  onChange: (value: ConfigValue) => void;
}) {
  const properties = schemaProperties(schema);
  const required = new Set(stringArray(schema.required));
  const knownNames = new Set(Object.keys(properties));
  const additional = additionalPropertiesSchema(schema);

  return (
    <div className="space-y-4">
      {Object.entries(properties).map(([name, childSchema]) => (
        <SchemaField
          key={name}
          name={name}
          schema={childSchema}
          hint={uiHint(uiSchema, name)}
          required={required.has(name)}
          value={value[name]}
          credentials={credentials}
          depth={depth + 1}
          onChange={(nextValue) => {
            const next = { ...value };
            if (nextValue === undefined) {
              delete next[name];
            } else {
              next[name] = nextValue;
            }
            onChange(next);
          }}
        />
      ))}

      {additional !== false ? (
        <AdditionalPropertiesEditor
          value={value}
          knownNames={knownNames}
          onChange={onChange}
        />
      ) : null}
    </div>
  );
}

function SchemaField({
  name,
  schema,
  hint,
  required,
  value,
  credentials,
  depth,
  onChange,
}: {
  name: string;
  schema: ConfigSchema;
  hint: UIHint;
  required: boolean;
  value: unknown;
  credentials: Credential[];
  depth: number;
  onChange: (value: unknown) => void;
}) {
  if (depth > MAX_SCHEMA_DEPTH) {
    return (
      <SchemaNotice>
        {t("Configuration field {{name}} exceeds the supported nesting depth.", { name })}
      </SchemaNotice>
    );
  }
  if (schema === false) {
    return (
      <SchemaNotice>
        {t("Configuration field {{name}} cannot be configured.", { name })}
      </SchemaNotice>
    );
  }
  if (schema === true) {
    return (
      <JSONValueEditor
        label={hint.label ?? humanize(name)}
        description={hint.description}
        value={value}
        required={required}
        onChange={onChange}
      />
    );
  }

  const label = hint.label ?? stringValue(schema.title) ?? humanize(name);
  const description = hint.description ?? stringValue(schema.description);
  const control = hint.control ?? stringValue(schema["x-mender-control"]);
  const enumValues = Array.isArray(schema.enum) ? schema.enum : undefined;
  const type = schemaType(schema, value);

  if (control === "credential") {
    return (
      <FieldShell label={label} required={required} description={description}>
        <select
          aria-label={label}
          value={typeof value === "string" ? value : ""}
          onChange={(event) =>
            onChange(event.target.value === "" ? undefined : event.target.value)
          }
          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
        >
          {!required ? <option value="">{t("None")}</option> : null}
          {required && value === undefined ? (
            <option value="" disabled>
              {t("Choose a Credential")}
            </option>
          ) : null}
          {credentials.map((credential) => (
            <option key={credential.reference} value={credential.reference}>
              {credential.displayName + " · " + credential.reference}
            </option>
          ))}
        </select>
      </FieldShell>
    );
  }

  if (enumValues !== undefined) {
    const selectedIndex = enumIndex(enumValues, value);
    return (
      <FieldShell label={label} required={required} description={description}>
        <select
          aria-label={label}
          value={selectedIndex}
          onChange={(event) => {
            const index = Number(event.target.value);
            onChange(
              Number.isInteger(index) && index >= 0
                ? cloneJSONValue(enumValues[index])
                : undefined,
            );
          }}
          className="h-8 w-full rounded-lg border border-input bg-background px-2.5 text-sm outline-none focus:ring-2 focus:ring-ring/40"
        >
          {!required ? <option value="-1">{t("None")}</option> : null}
          {required && selectedIndex === "-1" ? (
            <option value="-1" disabled>
              {t("Choose a value")}
            </option>
          ) : null}
          {enumValues.map((item, index) => (
            <option key={index} value={String(index)}>
              {enumLabel(item)}
            </option>
          ))}
        </select>
      </FieldShell>
    );
  }

  if (type === "boolean") {
    return (
      <FieldShell label={label} required={required} description={description}>
        <label className="flex items-center gap-2 rounded-lg border border-input px-3 py-2 text-sm">
          <input
            aria-label={label}
            type="checkbox"
            checked={value === true}
            onChange={(event) => onChange(event.target.checked)}
          />
          {t(value === true ? "Enabled" : "Disabled")}
        </label>
      </FieldShell>
    );
  }

  if (type === "number" || type === "integer") {
    return (
      <FieldShell label={label} required={required} description={description}>
        <Input
          aria-label={label}
          type="number"
          step={type === "integer" ? 1 : "any"}
          placeholder={hint.placeholder}
          value={typeof value === "number" ? String(value) : ""}
          onChange={(event) => {
            const raw = event.target.value;
            if (raw === "") {
              onChange(undefined);
              return;
            }
            const parsed = Number(raw);
            onChange(Number.isFinite(parsed) ? parsed : undefined);
          }}
        />
      </FieldShell>
    );
  }

  if (type === "object") {
    const objectValue = isRecord(value) ? value : {};
    return (
      <div className="space-y-3 rounded-lg border border-border p-3">
        <div>
          <div className="text-sm font-medium">
            {label}
            {!required ? (
              <span className="ml-1 font-normal text-muted-foreground">
                {t("(optional)")}
              </span>
            ) : null}
          </div>
          {description ? (
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {description}
            </p>
          ) : null}
        </div>
        <ObjectFields
          schema={schema}
          uiSchema={undefined}
          value={objectValue}
          credentials={credentials}
          depth={depth}
          onChange={(nextValue) => onChange(nextValue)}
        />
      </div>
    );
  }

  if (type === "array") {
    return (
      <JSONArrayEditor
        label={label}
        description={description}
        required={required}
        value={Array.isArray(value) ? value : []}
        onChange={onChange}
      />
    );
  }

  const maxLength = numberValue(schema.maxLength);
  const textarea =
    hint.control === "textarea" ||
    (maxLength !== undefined && maxLength > 160);

  return (
    <FieldShell label={label} required={required} description={description}>
      {textarea ? (
        <Textarea
          aria-label={label}
          placeholder={hint.placeholder}
          value={typeof value === "string" ? value : ""}
          onChange={(event) =>
            onChange(
              event.target.value === "" && !required
                ? undefined
                : event.target.value,
            )
          }
        />
      ) : (
        <Input
          aria-label={label}
          placeholder={hint.placeholder}
          value={typeof value === "string" ? value : ""}
          onChange={(event) =>
            onChange(
              event.target.value === "" && !required
                ? undefined
                : event.target.value,
            )
          }
        />
      )}
    </FieldShell>
  );
}

function JSONArrayEditor({
  label,
  description,
  required,
  value,
  onChange,
}: {
  label: string;
  description?: string | undefined;
  required: boolean;
  value: unknown[];
  onChange: (value: unknown) => void;
}) {
  const serialized = JSON.stringify(value, null, 2);
  const [text, setText] = useState(serialized);
  const [error, setError] = useState("");

  return (
    <FieldShell label={label} required={required} description={description}>
      <Textarea
        aria-label={label}
        className="min-h-28 font-mono text-xs"
        value={text}
        onChange={(event) => {
          const nextText = event.target.value;
          setText(nextText);
          if (nextText.trim() === "" && !required) {
            setError("");
            onChange(undefined);
            return;
          }
          try {
            const parsed: unknown = JSON.parse(nextText);
            if (!Array.isArray(parsed)) {
              setError(t("Enter a JSON array."));
              return;
            }
            setError("");
            onChange(parsed);
          } catch {
            setError(t("Enter valid JSON."));
          }
        }}
      />
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </FieldShell>
  );
}

function JSONValueEditor({
  label,
  description,
  required,
  value,
  onChange,
}: {
  label: string;
  description?: string | undefined;
  required: boolean;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const serialized =
    value === undefined ? "" : JSON.stringify(value, null, 2);
  const [text, setText] = useState(serialized);
  const [error, setError] = useState("");

  return (
    <FieldShell label={label} required={required} description={description}>
      <Textarea
        aria-label={label}
        className="min-h-24 font-mono text-xs"
        value={text}
        onChange={(event) => {
          const nextText = event.target.value;
          setText(nextText);
          if (nextText.trim() === "" && !required) {
            setError("");
            onChange(undefined);
            return;
          }
          try {
            setError("");
            onChange(JSON.parse(nextText) as unknown);
          } catch {
            setError(t("Enter valid JSON."));
          }
        }}
      />
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </FieldShell>
  );
}

function RawObjectEditor({
  label,
  value,
  onChange,
}: {
  label: string;
  value: ConfigValue;
  onChange: (value: ConfigValue) => void;
}) {
  const serialized = JSON.stringify(value, null, 2);
  const [text, setText] = useState(serialized);
  const [error, setError] = useState("");

  return (
    <FieldShell
      label={label}
      required
      description={t("This plugin does not publish a structured form schema. Enter a JSON object.")}
    >
      <Textarea
        aria-label={label}
        className="min-h-40 font-mono text-xs"
        value={text}
        onChange={(event) => {
          const nextText = event.target.value;
          setText(nextText);
          try {
            const parsed: unknown = JSON.parse(nextText);
            if (!isRecord(parsed)) {
              setError(t("Provider configuration must be a JSON object."));
              return;
            }
            setError("");
            onChange(parsed);
          } catch {
            setError(t("Enter valid JSON."));
          }
        }}
      />
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </FieldShell>
  );
}

function AdditionalPropertiesEditor({
  value,
  knownNames,
  onChange,
}: {
  value: ConfigValue;
  knownNames: Set<string>;
  onChange: (value: ConfigValue) => void;
}) {
  const additional = Object.fromEntries(
    Object.entries(value).filter(([name]) => !knownNames.has(name)),
  );
  const serialized = JSON.stringify(additional, null, 2);
  const [text, setText] = useState(serialized);
  const [error, setError] = useState("");

  return (
    <details className="rounded-lg border border-border">
      <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">
        {t("Additional properties")}
      </summary>
      <div className="space-y-2 border-t border-border px-4 py-4">
        <Textarea
          aria-label={t("Additional properties")}
          className="min-h-28 font-mono text-xs"
          value={text}
          onChange={(event) => {
            const nextText = event.target.value;
            setText(nextText);
            try {
              const parsed: unknown = JSON.parse(nextText);
              if (!isRecord(parsed)) {
                setError(t("Additional properties must be a JSON object."));
                return;
              }
              const known = Object.fromEntries(
                Object.entries(value).filter(([name]) => knownNames.has(name)),
              );
              setError("");
              onChange({ ...known, ...parsed });
            } catch {
              setError(t("Enter valid JSON."));
            }
          }}
        />
        {error ? <p className="text-xs text-destructive">{error}</p> : null}
      </div>
    </details>
  );
}

function FieldShell({
  label,
  required,
  description,
  children,
}: {
  label: string;
  required: boolean;
  description?: string | undefined;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <div className="text-sm font-medium">
        {label}
        {!required ? (
          <span className="ml-1 font-normal text-muted-foreground">
            {t("(optional)")}
          </span>
        ) : null}
      </div>
      {children}
      {description ? (
        <p className="text-xs leading-5 text-muted-foreground">{description}</p>
      ) : null}
    </div>
  );
}

function SchemaNotice({ children }: { children: React.ReactNode }) {
  return (
    <p className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
      {children}
    </p>
  );
}
