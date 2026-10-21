import { useCallback, useEffect, useId, useRef, useState } from "react";

import type { Credential, ProviderPlugin } from "@mender/sdk";
import { Button } from "@mender/ui/components/button";
import { Input } from "@mender/ui/components/input";
import { Textarea } from "@mender/ui/components/textarea";

import {
  MAX_SCHEMA_DEPTH,
  additionalPropertiesSchema,
  cloneJSONValue,
  enumIndex,
  enumLabel,
  genericConfigDefaults,
  genericConfigFieldValidationError,
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

function visibleSchemaError(error: string | null): string | null {
  if (!error) return null;
  if (error === "This field is required.") return t("This field is required.");
  if (error === "must be an integer.") return t("Enter a whole number.");
  if (error === "must be an absolute URI.") return t("Enter an absolute URI.");
  if (error === "must be a number.") return t("Enter a valid number.");
  const minimum = /^must be at least (-?\d+(?:\.\d+)?)\.$/.exec(error);
  if (minimum) return t("Value must be at least {{value}}.", {value: minimum[1]!});
  const maximum = /^must be at most (-?\d+(?:\.\d+)?)\.$/.exec(error);
  if (maximum) return t("Value must be at most {{value}}.", {value: maximum[1]!});
  return error;
}

function fieldDescriptionIds(id: string, description?: string, error?: string | null): string | undefined {
  const ids = [description ? id + "-description" : null, error ? id + "-error" : null]
    .filter((item): item is string => item !== null);
  return ids.length ? ids.join(" ") : undefined;
}

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
  showRequired = true,
}: {
  schema: SchemaObject;
  uiSchema: ProviderPlugin["uiSchema"];
  value: ConfigValue;
  credentials: Credential[];
  depth: number;
  path: string;
  reportDraftIssue: ReportDraftIssue;
  onChange: (value: ConfigValue) => void;
  showRequired?: boolean;
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
          required={showRequired && required.has(name)}
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
  const fieldId = useId();
  const validationError = visibleSchemaError(
    genericConfigFieldValidationError(schema, value, required),
  );
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
        schemaError={validationError}
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
  const fieldDescription = fieldDescriptionIds(fieldId, description, validationError);

  if (control === "credential") {
    return (
      <FieldShell label={label} required={required} description={description}
        controlId={fieldId} error={validationError}>
        <select
          id={fieldId}
          aria-label={label}
          aria-required={required}
          aria-invalid={Boolean(validationError)}
          aria-describedby={fieldDescription}
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
      <FieldShell label={label} required={required} description={description}
        controlId={fieldId} error={validationError}>
        <select
          id={fieldId}
          aria-label={label}
          aria-required={required}
          aria-invalid={Boolean(validationError)}
          aria-describedby={fieldDescription}
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
      <FieldShell label={label} required={required} description={description}
        controlId={fieldId} error={validationError}>
        <label className="flex items-center gap-2 rounded-lg border border-input px-3 py-2 text-sm">
          <input
            id={fieldId}
            aria-label={label}
            aria-required={required}
            aria-invalid={Boolean(validationError)}
            aria-describedby={fieldDescription}
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
      <NumericSchemaEditor
        label={label}
        description={description}
        required={required}
        schema={schema}
        value={typeof value === "number" ? value : undefined}
        schemaError={validationError}
        path={path}
        reportDraftIssue={reportDraftIssue}
        onChange={onChange}
      />
    );
  }

  if (type === "object") {
    // An absent optional object is not the same configuration as an empty
    // object. Let users opt into its fields rather than persisting {} by
    // accident or presenting all its advanced inputs up front.
    if (!required && value === undefined) {
      return (
        <div className="min-w-0 space-y-1.5 rounded-lg border border-border/70 px-3 py-2.5">
          <div className="flex items-center justify-between gap-2">
            <p className="min-w-0 text-sm font-medium">
              {label}
              <span className="ml-1 font-normal text-muted-foreground">{t("(optional)")}</span>
            </p>
            <Button type="button" size="sm" variant="outline"
              aria-label={t("Add {{name}}", { name: label })}
              onClick={() => onChange(genericConfigDefaults(schema))}>
              {t("Add")}
            </Button>
          </div>
          {description ? (
            <p className="text-xs leading-5 text-muted-foreground">{description}</p>
          ) : null}
        </div>
      );
    }

    const objectValue = isRecord(value) ? value : {};
    return (
      <fieldset className="min-w-0 space-y-3 rounded-lg border border-border p-3">
        <legend className="px-1 text-sm font-medium">
          {label}
          {!required ? (
            <span aria-hidden="true" className="ml-1 font-normal text-muted-foreground">
              {t("(optional)")}
            </span>
          ) : null}
        </legend>
        {!required ? (
          <div className="flex justify-end">
            <Button type="button" size="sm" variant="ghost"
              aria-label={t("Remove {{name}}", { name: label })}
              onClick={() => onChange(undefined)}>
              {t("Remove")}
            </Button>
          </div>
        ) : null}
        {description ? (
          <p className="text-xs leading-5 text-muted-foreground">{description}</p>
        ) : null}
        {validationError && !isRecord(value) ? (
          <p role="alert" className="text-xs leading-5 text-destructive">{validationError}</p>
        ) : null}
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
      </fieldset>
    );
  }

  if (type === "array") {
    return (
      <JSONArrayEditor
        label={label}
        description={description}
        required={required}
        value={Array.isArray(value) ? value : undefined}
        schemaError={validationError}
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
    <FieldShell label={label} required={required} description={description}
      controlId={fieldId} error={validationError}>
      {textarea ? (
        <Textarea
          id={fieldId}
          aria-label={label}
          aria-required={required}
          aria-invalid={Boolean(validationError)}
          aria-describedby={fieldDescription}
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
          id={fieldId}
          aria-label={label}
          aria-required={required}
          aria-invalid={Boolean(validationError)}
          aria-describedby={fieldDescription}
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

/** Use text+decimal keyboard instead of type=number: browsers normalize partial
 * tokens such as "1e" or "-" to an empty value and would delete valid data. */
function NumericSchemaEditor({
  label,
  description,
  required,
  schema,
  value,
  schemaError,
  path,
  reportDraftIssue,
  onChange,
}: {
  label: string;
  description?: string | undefined;
  required: boolean;
  schema: SchemaObject;
  value: number | undefined;
  schemaError: string | null;
  path: string;
  reportDraftIssue: ReportDraftIssue;
  onChange: (value: unknown) => void;
}) {
  const draft = useJSONTextDraft(value === undefined ? "" : String(value), path, reportDraftIssue);
  const error = draft.error || schemaError;
  const type = schemaType(schema, value);
  return (
    <FieldShell label={label} required={required} description={description}
      controlId={draft.errorId} error={error}>
      <Input
        id={draft.errorId}
        aria-label={label}
        aria-required={required}
        aria-invalid={Boolean(error)}
        aria-describedby={fieldDescriptionIds(draft.errorId, description, error)}
        type="text"
        inputMode={type === "integer" ? "numeric" : "decimal"}
        value={draft.text}
        onChange={(event) => {
          const raw = event.target.value;
          draft.setText(raw);
          if (raw.trim() === "") {
            if (required) {
              draft.setError(t("This field is required."));
              return;
            }
            draft.setError("");
            draft.markPublished(undefined);
            onChange(undefined);
            return;
          }
          const normalized = raw.trim();
          if (!/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(normalized)) {
            draft.setError(t("Enter a valid number."));
            return;
          }
          const parsed = Number(normalized);
          if (!Number.isFinite(parsed)) {
            draft.setError(t("Enter a valid number."));
            return;
          }
          if (type === "integer" && !Number.isSafeInteger(parsed)) {
            draft.setError(t("Enter a whole number."));
            return;
          }
          const minimum = numberValue(schema.minimum);
          const maximum = numberValue(schema.maximum);
          if (minimum !== undefined && parsed < minimum) {
            draft.setError(t("Value must be at least {{value}}.", {value: minimum}));
            return;
          }
          if (maximum !== undefined && parsed > maximum) {
            draft.setError(t("Value must be at most {{value}}.", {value: maximum}));
            return;
          }
          draft.setError("");
          draft.markPublished(parsed);
          onChange(parsed);
        }}
      />
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
  schemaError,
  path,
  reportDraftIssue,
  onChange,
}: {
  label: string;
  description?: string | undefined;
  required: boolean;
  value: unknown[] | undefined;
  schemaError: string | null;
  path: string;
  reportDraftIssue: ReportDraftIssue;
  onChange: (value: unknown) => void;
}) {
  const draft = useJSONTextDraft(value === undefined ? "" : JSON.stringify(value, null, 2), path, reportDraftIssue);
  const error = draft.error || schemaError;
  return (
    <FieldShell label={label} required={required} description={description}
      controlId={draft.errorId} error={error}>
      <Textarea
        id={draft.errorId}
        aria-label={label}
        aria-required={required}
        aria-invalid={Boolean(error)}
        aria-describedby={fieldDescriptionIds(draft.errorId, description, error)}
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
    </FieldShell>
  );
}

function JSONValueEditor({
  label,
  description,
  required,
  value,
  schemaError,
  path,
  reportDraftIssue,
  onChange,
}: {
  label: string;
  description?: string | undefined;
  required: boolean;
  value: unknown;
  schemaError: string | null;
  path: string;
  reportDraftIssue: ReportDraftIssue;
  onChange: (value: unknown) => void;
}) {
  const serialized = value === undefined ? "" : JSON.stringify(value, null, 2);
  const draft = useJSONTextDraft(serialized, path, reportDraftIssue);
  const error = draft.error || schemaError;
  return (
    <FieldShell label={label} required={required} description={description}
      controlId={draft.errorId} error={error}>
      <Textarea
        id={draft.errorId}
        aria-label={label}
        aria-required={required}
        aria-invalid={Boolean(error)}
        aria-describedby={fieldDescriptionIds(draft.errorId, description, error)}
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
      controlId={draft.errorId}
      error={draft.error}
      description={t("This plugin does not publish a structured form schema. Enter a JSON object.")}
    >
      <Textarea
        id={draft.errorId}
        aria-label={label}
        aria-required
        aria-invalid={Boolean(draft.error)}
        aria-describedby={fieldDescriptionIds(draft.errorId, t("This plugin does not publish a structured form schema. Enter a JSON object."), draft.error)}
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
  controlId,
  error,
  children,
}: {
  label: string;
  required: boolean;
  description?: string | undefined;
  controlId?: string;
  error?: string | null;
  children: React.ReactNode;
}) {
  const errorRef = useRef<HTMLParagraphElement | null>(null);
  const mounted = useRef(false);
  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      return;
    }
    // Keep newly created inline errors visible within a scrolling dialog,
    // without stealing the user's focus or jumping to another invalid field.
    if (error && controlId && document.activeElement?.id === controlId &&
      typeof errorRef.current?.scrollIntoView === "function") {
      errorRef.current.scrollIntoView({block: "nearest", inline: "nearest"});
    }
  }, [controlId, error]);
  return (
    <div className="min-w-0 space-y-1.5" onFocusCapture={(event) => {
      if (typeof event.target.scrollIntoView === "function") {
        event.target.scrollIntoView({block: "nearest", inline: "nearest"});
      }
    }}>
      <div className="text-sm font-medium">
        {controlId ? <label htmlFor={controlId}>{label}</label> : label}
        {!required ? (
          <span className="ml-1 font-normal text-muted-foreground">
            {t("(optional)")}
          </span>
        ) : null}
      </div>
      {children}
      {description ? (
        <p id={controlId ? controlId + "-description" : undefined}
          className="text-xs leading-5 text-muted-foreground">{description}</p>
      ) : null}
      {error ? (
        <p ref={errorRef} id={controlId ? controlId + "-error" : undefined}
          role="alert" className="text-xs leading-5 text-destructive">{error}</p>
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
