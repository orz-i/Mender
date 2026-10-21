import { useCallback, useEffect, useId, useRef, useState } from "react";

import type { Credential, ProviderPlugin } from "@mender/sdk";
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

type ReportDraftIssue = (path: string, issue: string | null) => void;

/** A JSON field can have a local invalid draft even while its last valid
 * parent value remains schema-valid. The form reports those drafts separately. */
export function GenericProviderConfigForm({
  schema,
  uiSchema,
  value,
  credentials,
  onChange,
  onDraftErrorChange,
}: {
  schema: ProviderPlugin["configSchema"];
  uiSchema: ProviderPlugin["uiSchema"];
  value: ConfigValue;
  credentials: Credential[];
  onChange: (value: ConfigValue) => void;
  onDraftErrorChange?: (error: string | null) => void;
}) {
  const [draftIssues, setDraftIssues] = useState<Record<string, string>>({});
  const reportDraftIssue = useCallback<ReportDraftIssue>((path, issue) => {
    setDraftIssues((current) => {
      if (current[path] === (issue ?? undefined)) return current;
      const next = { ...current };
      if (issue) next[path] = issue;
      else delete next[path];
      return next;
    });
  }, []);
  const draftError = Object.values(draftIssues)[0] ?? null;
  useEffect(() => { onDraftErrorChange?.(draftError); }, [draftError, onDraftErrorChange]);

  const supportError = genericSchemaSupportError(schema);
  if (supportError) {
    return <SchemaNotice>{supportError}</SchemaNotice>;
  }

  if (schema === undefined || schema === true) {
    return (
      <RawObjectEditor
        label={t("Provider configuration")}
        value={value}
        path="$"
        reportDraftIssue={reportDraftIssue}
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
      path="$"
      reportDraftIssue={reportDraftIssue}
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
  path,
  reportDraftIssue,
  onChange,
}: {
  schema: SchemaObject;
  uiSchema: ProviderPlugin["uiSchema"];
  value: ConfigValue;
  credentials: Credential[];
  depth: number;
  path: string;
  reportDraftIssue: ReportDraftIssue;
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
          path={path + "." + name}
          reportDraftIssue={reportDraftIssue}
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
          path={path + ".*"}
          reportDraftIssue={reportDraftIssue}
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
  path,
  reportDraftIssue,
  onChange,
}: {
  name: string;
  schema: ConfigSchema;
  hint: UIHint;
  required: boolean;
  value: unknown;
  credentials: Credential[];
  depth: number;
  path: string;
  reportDraftIssue: ReportDraftIssue;
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
        path={path}
        reportDraftIssue={reportDraftIssue}
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
          path={path}
          reportDraftIssue={reportDraftIssue}
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
        value={Array.isArray(value) ? value : undefined}
        path={path}
        reportDraftIssue={reportDraftIssue}
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

/** Preserve the user's exact JSON text across parent echoes of its parsed value,
 * but load new authoritative configuration when the serialized value changes. */
function useJSONTextDraft(serialized: string, path: string, reportDraftIssue: ReportDraftIssue) {
  const [text, setText] = useState(serialized);
  const [error, setError] = useState("");
  const errorId = useId();
  const lastPublished = useRef<string | null>(null);

  useEffect(() => {
    if (lastPublished.current === serialized) {
      lastPublished.current = null;
      return;
    }
    lastPublished.current = null;
    setText(serialized);
    setError("");
  }, [serialized]);

  useEffect(() => {
    reportDraftIssue(path, error || null);
    return () => reportDraftIssue(path, null);
  }, [path, error, reportDraftIssue]);

  function markPublished(next: unknown) {
    const normalized = next === undefined ? "" : JSON.stringify(next, null, 2);
    lastPublished.current = normalized === serialized ? null : normalized;
  }
  return { text, setText, error, setError, errorId, markPublished };
}

function JSONDraftError({ message, id }: { message: string; id: string }) {
  return message ? <p id={id} role="alert" className="text-xs leading-5 text-destructive">{message}</p> : null;
}

function JSONArrayEditor({
  label,
  description,
  required,
  value,
  path,
  reportDraftIssue,
  onChange,
}: {
  label: string;
  description?: string | undefined;
  required: boolean;
  value: unknown[] | undefined;
  path: string;
  reportDraftIssue: ReportDraftIssue;
  onChange: (value: unknown) => void;
}) {
  const draft = useJSONTextDraft(value === undefined ? "" : JSON.stringify(value, null, 2), path, reportDraftIssue);
  return (
    <FieldShell label={label} required={required} description={description}>
      <Textarea
        aria-label={label}
        aria-invalid={Boolean(draft.error)}
        aria-describedby={draft.error ? draft.errorId : undefined}
        className="min-h-28 font-mono text-xs"
        value={draft.text}
        onChange={(event) => {
          const nextText = event.target.value;
          draft.setText(nextText);
          if (nextText.trim() === "" && !required) {
            draft.setError("");
            draft.markPublished(undefined);
            onChange(undefined);
            return;
          }
          try {
            const parsed: unknown = JSON.parse(nextText);
            if (!Array.isArray(parsed)) {
              draft.setError(t("Enter a JSON array."));
              return;
            }
            draft.setError("");
            draft.markPublished(parsed);
            onChange(parsed);
          } catch {
            draft.setError(t("Enter valid JSON."));
          }
        }}
      />
      <JSONDraftError message={draft.error} id={draft.errorId} />
    </FieldShell>
  );
}

function JSONValueEditor({
  label,
  description,
  required,
  value,
  path,
  reportDraftIssue,
  onChange,
}: {
  label: string;
  description?: string | undefined;
  required: boolean;
  value: unknown;
  path: string;
  reportDraftIssue: ReportDraftIssue;
  onChange: (value: unknown) => void;
}) {
  const serialized = value === undefined ? "" : JSON.stringify(value, null, 2);
  const draft = useJSONTextDraft(serialized, path, reportDraftIssue);
  return (
    <FieldShell label={label} required={required} description={description}>
      <Textarea
        aria-label={label}
        aria-invalid={Boolean(draft.error)}
        aria-describedby={draft.error ? draft.errorId : undefined}
        className="min-h-24 font-mono text-xs"
        value={draft.text}
        onChange={(event) => {
          const nextText = event.target.value;
          draft.setText(nextText);
          if (nextText.trim() === "") {
            if (required) {
              draft.setError(t("Enter valid JSON."));
              return;
            }
            draft.setError("");
            draft.markPublished(undefined);
            onChange(undefined);
            return;
          }
          try {
            const parsed: unknown = JSON.parse(nextText);
            draft.setError("");
            draft.markPublished(parsed);
            onChange(parsed);
          } catch {
            draft.setError(t("Enter valid JSON."));
          }
        }}
      />
      <JSONDraftError message={draft.error} id={draft.errorId} />
    </FieldShell>
  );
}

function RawObjectEditor({
  label,
  value,
  path,
  reportDraftIssue,
  onChange,
}: {
  label: string;
  value: ConfigValue;
  path: string;
  reportDraftIssue: ReportDraftIssue;
  onChange: (value: ConfigValue) => void;
}) {
  const draft = useJSONTextDraft(JSON.stringify(value, null, 2), path, reportDraftIssue);
  return (
    <FieldShell
      label={label}
      required
      description={t("This plugin does not publish a structured form schema. Enter a JSON object.")}
    >
      <Textarea
        aria-label={label}
        aria-invalid={Boolean(draft.error)}
        aria-describedby={draft.error ? draft.errorId : undefined}
        className="min-h-40 font-mono text-xs"
        value={draft.text}
        onChange={(event) => {
          const nextText = event.target.value;
          draft.setText(nextText);
          try {
            const parsed: unknown = JSON.parse(nextText);
            if (!isRecord(parsed)) {
              draft.setError(t("Provider configuration must be a JSON object."));
              return;
            }
            draft.setError("");
            draft.markPublished(parsed);
            onChange(parsed);
          } catch {
            draft.setError(t("Enter valid JSON."));
          }
        }}
      />
      <JSONDraftError message={draft.error} id={draft.errorId} />
    </FieldShell>
  );
}

function AdditionalPropertiesEditor({
  value,
  knownNames,
  path,
  reportDraftIssue,
  onChange,
}: {
  value: ConfigValue;
  knownNames: Set<string>;
  path: string;
  reportDraftIssue: ReportDraftIssue;
  onChange: (value: ConfigValue) => void;
}) {
  const additional = Object.fromEntries(
    Object.entries(value).filter(([name]) => !knownNames.has(name)),
  );
  const draft = useJSONTextDraft(JSON.stringify(additional, null, 2), path, reportDraftIssue);
  return (
    <details className="rounded-lg border border-border">
      <summary className="cursor-pointer select-none px-4 py-3 text-sm font-medium">
        {t("Additional properties")}
      </summary>
      <div className="space-y-2 border-t border-border px-4 py-4">
        <Textarea
          aria-label={t("Additional properties")}
          aria-invalid={Boolean(draft.error)}
          aria-describedby={draft.error ? draft.errorId : undefined}
          className="min-h-28 font-mono text-xs"
          value={draft.text}
          onChange={(event) => {
            const nextText = event.target.value;
            draft.setText(nextText);
            try {
              const parsed: unknown = JSON.parse(nextText);
              if (!isRecord(parsed)) {
                draft.setError(t("Additional properties must be a JSON object."));
                return;
              }
              const known = Object.fromEntries(
                Object.entries(value).filter(([name]) => knownNames.has(name)),
              );
              draft.setError("");
              draft.markPublished(parsed);
              onChange({ ...known, ...parsed });
            } catch {
              draft.setError(t("Enter valid JSON."));
            }
          }}
        />
        <JSONDraftError message={draft.error} id={draft.errorId} />
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
