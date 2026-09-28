import type { ProviderPlugin } from "@mender/sdk";

export const MAX_SCHEMA_DEPTH = 6;

export type ConfigSchema = NonNullable<ProviderPlugin["configSchema"]>;
export type SchemaObject = Record<string, unknown>;
export type ConfigValue = Record<string, unknown>;

export interface UIHint {
  label?: string;
  description?: string | undefined;
  placeholder?: string | undefined;
  control?: "text" | "textarea" | "credential";
}

export function genericConfigDefaults(
  schema: ProviderPlugin["configSchema"],
): ConfigValue {
  if (schema === undefined || typeof schema === "boolean") {
    return {};
  }
  if (isRecord(schema.default)) {
    return cloneRecord(schema.default);
  }

  const required = stringArray(schema.required);
  const result: ConfigValue = {};
  for (const [name, child] of Object.entries(schemaProperties(schema))) {
    const childDefault = schemaDefaultValue(child, required.includes(name));
    if (childDefault !== undefined) {
      result[name] = childDefault;
    }
  }
  return result;
}

export function genericConfigValidationError(
  schema: ProviderPlugin["configSchema"],
  value: ConfigValue,
): string | null {
  const supportError = genericSchemaSupportError(schema);
  if (supportError) {
    return supportError;
  }
  if (schema === undefined || schema === true) {
    return null;
  }
  if (schema === false) {
    return "This plugin schema rejects every configuration.";
  }
  return validateSchemaValue(schema, value, "$", 0);
}

export function genericSchemaSupportError(
  schema: ProviderPlugin["configSchema"],
): string | null {
  if (schema === undefined || typeof schema === "boolean") {
    return null;
  }
  return schemaSupportError(schema, "$", 0);
}

export function schemaProperties(
  schema: SchemaObject,
): Record<string, ConfigSchema> {
  if (!isRecord(schema.properties)) {
    return {};
  }
  const result: Record<string, ConfigSchema> = {};
  for (const [name, value] of Object.entries(schema.properties)) {
    if (typeof value === "boolean" || isRecord(value)) {
      result[name] = value;
    }
  }
  return result;
}

export function additionalPropertiesSchema(
  schema: SchemaObject,
): ConfigSchema | undefined {
  const value = schema.additionalProperties;
  return typeof value === "boolean" || isRecord(value) ? value : undefined;
}

export function schemaValue(value: unknown): ConfigSchema {
  return typeof value === "boolean" || isRecord(value) ? value : true;
}

export function schemaType(schema: SchemaObject, value: unknown): string {
  const explicit = stringValue(schema.type);
  if (explicit) {
    return explicit;
  }
  if (schema.properties !== undefined) {
    return "object";
  }
  if (schema.items !== undefined) {
    return "array";
  }
  if (typeof value === "boolean") {
    return "boolean";
  }
  if (typeof value === "number") {
    return "number";
  }
  if (Array.isArray(value)) {
    return "array";
  }
  if (isRecord(value)) {
    return "object";
  }
  return "string";
}

export function uiHint(
  uiSchema: ProviderPlugin["uiSchema"],
  name: string,
): UIHint {
  if (!isRecord(uiSchema) || !isRecord(uiSchema.properties)) {
    return {};
  }
  const raw = uiSchema.properties[name];
  if (!isRecord(raw)) {
    return {};
  }

  const control = stringValue(raw.control);
  const result: UIHint = {};
  const label = stringValue(raw.label);
  const description = stringValue(raw.description);
  const placeholder = stringValue(raw.placeholder);
  if (label) {
    result.label = label;
  }
  if (description) {
    result.description = description;
  }
  if (placeholder) {
    result.placeholder = placeholder;
  }
  if (
    control === "text" ||
    control === "textarea" ||
    control === "credential"
  ) {
    result.control = control;
  }
  return result;
}

export function enumIndex(values: unknown[], current: unknown): string {
  return String(values.findIndex((item) => sameJSONValue(item, current)));
}

export function enumLabel(value: unknown): string {
  return typeof value === "string" ? value : JSON.stringify(value);
}

export function cloneJSONValue<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

export function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

export function numberValue(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

export function humanize(value: string): string {
  return value
    .replace(/[_-]+/g, " ")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/^./, (character) => character.toUpperCase());
}

export function isRecord(
  value: unknown,
): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function schemaSupportError(
  schema: SchemaObject,
  path: string,
  depth: number,
): string | null {
  if (depth > MAX_SCHEMA_DEPTH) {
    return path + " exceeds the supported nesting depth.";
  }

  for (const keyword of [
    "$ref",
    "oneOf",
    "anyOf",
    "allOf",
    "not",
    "if",
    "then",
    "else",
    "dependentSchemas",
    "patternProperties",
    "propertyNames",
    "unevaluatedProperties",
    "unevaluatedItems",
    "contains",
    "prefixItems",
  ]) {
    if (schema[keyword] !== undefined) {
      return path + ' uses unsupported JSON Schema keyword "' + keyword + '".';
    }
  }

  if (Array.isArray(schema.type)) {
    return path + " uses an unsupported union type.";
  }

  const type = stringValue(schema.type);
  if (
    type !== undefined &&
    !["object", "array", "string", "number", "integer", "boolean"].includes(type)
  ) {
    return path + ' uses unsupported JSON Schema type "' + type + '".';
  }

  for (const [name, child] of Object.entries(schemaProperties(schema))) {
    if (isRecord(child)) {
      const error = schemaSupportError(
        child,
        path + "." + name,
        depth + 1,
      );
      if (error) {
        return error;
      }
    }
  }

  const items = schema.items;
  if (isRecord(items)) {
    const error = schemaSupportError(items, path + "[]", depth + 1);
    if (error) {
      return error;
    }
  } else if (items !== undefined && typeof items !== "boolean") {
    return path + ".items must be a JSON Schema object or boolean.";
  }

  const additional = schema.additionalProperties;
  if (isRecord(additional)) {
    return schemaSupportError(additional, path + ".*", depth + 1);
  }
  if (additional !== undefined && typeof additional !== "boolean") {
    return path + ".additionalProperties must be a JSON Schema object or boolean.";
  }

  return null;
}

function validateSchemaValue(
  schema: ConfigSchema,
  value: unknown,
  path: string,
  depth: number,
): string | null {
  if (depth > MAX_SCHEMA_DEPTH) {
    return path + " exceeds the supported nesting depth.";
  }
  if (schema === true) {
    return null;
  }
  if (schema === false) {
    return path + " is not allowed by the plugin schema.";
  }

  if (value === undefined) {
    return null;
  }

  if (
    Array.isArray(schema.enum) &&
    !schema.enum.some((item) => sameJSONValue(item, value))
  ) {
    return path + " must be one of the allowed values.";
  }

  const type = schemaType(schema, value);
  if (type === "object") {
    if (!isRecord(value)) {
      return path + " must be an object.";
    }
    const properties = schemaProperties(schema);
    for (const name of stringArray(schema.required)) {
      if (value[name] === undefined) {
        return path + "." + name + " is required.";
      }
    }
    for (const [name, childValue] of Object.entries(value)) {
      const childSchema = properties[name];
      if (childSchema !== undefined) {
        const error = validateSchemaValue(
          childSchema,
          childValue,
          path + "." + name,
          depth + 1,
        );
        if (error) {
          return error;
        }
        continue;
      }

      const additional = additionalPropertiesSchema(schema);
      if (additional === false) {
        return path + "." + name + " is not allowed by the plugin schema.";
      }
      if (additional !== undefined) {
        const error = validateSchemaValue(
          additional,
          childValue,
          path + "." + name,
          depth + 1,
        );
        if (error) {
          return error;
        }
      }
    }
    return null;
  }

  if (type === "array") {
    if (!Array.isArray(value)) {
      return path + " must be an array.";
    }
    const minItems = numberValue(schema.minItems);
    const maxItems = numberValue(schema.maxItems);
    if (minItems !== undefined && value.length < minItems) {
      return path + " must contain at least " + String(minItems) + " item(s).";
    }
    if (maxItems !== undefined && value.length > maxItems) {
      return path + " must contain at most " + String(maxItems) + " item(s).";
    }
    if (schema.items !== undefined) {
      const itemSchema = schemaValue(schema.items);
      for (let index = 0; index < value.length; index += 1) {
        const error = validateSchemaValue(
          itemSchema,
          value[index],
          path + "[" + String(index) + "]",
          depth + 1,
        );
        if (error) {
          return error;
        }
      }
    }
    return null;
  }

  if (type === "boolean") {
    return typeof value === "boolean" ? null : path + " must be a boolean.";
  }

  if (type === "number" || type === "integer") {
    if (typeof value !== "number" || !Number.isFinite(value)) {
      return path + " must be a number.";
    }
    if (type === "integer" && !Number.isInteger(value)) {
      return path + " must be an integer.";
    }
    const minimum = numberValue(schema.minimum);
    const maximum = numberValue(schema.maximum);
    if (minimum !== undefined && value < minimum) {
      return path + " must be at least " + String(minimum) + ".";
    }
    if (maximum !== undefined && value > maximum) {
      return path + " must be at most " + String(maximum) + ".";
    }
    return null;
  }

  if (typeof value !== "string") {
    return path + " must be a string.";
  }

  const minLength = numberValue(schema.minLength);
  const maxLength = numberValue(schema.maxLength);
  if (minLength !== undefined && value.length < minLength) {
    return path + " must contain at least " + String(minLength) + " character(s).";
  }
  if (maxLength !== undefined && value.length > maxLength) {
    return path + " must contain at most " + String(maxLength) + " character(s).";
  }
  if (typeof schema.pattern === "string") {
    try {
      if (!new RegExp(schema.pattern).test(value)) {
        return path + " does not match the required pattern.";
      }
    } catch {
      return path + " uses an invalid schema pattern.";
    }
  }
  if (schema.format === "uri") {
    try {
      const parsed = new URL(value);
      if (!parsed.protocol) {
        return path + " must be an absolute URI.";
      }
    } catch {
      return path + " must be an absolute URI.";
    }
  }
  return null;
}

function schemaDefaultValue(
  schema: ConfigSchema,
  required: boolean,
): unknown {
  if (typeof schema === "boolean") {
    return undefined;
  }
  if (schema.default !== undefined) {
    return cloneJSONValue(schema.default);
  }

  const type = schemaType(schema, undefined);
  if (type === "object") {
    const result: ConfigValue = {};
    let hasValue = false;
    const requiredChildren = stringArray(schema.required);
    for (const [name, child] of Object.entries(schemaProperties(schema))) {
      const childDefault = schemaDefaultValue(
        child,
        requiredChildren.includes(name),
      );
      if (childDefault !== undefined) {
        result[name] = childDefault;
        hasValue = true;
      }
    }
    return hasValue || required ? result : undefined;
  }
  if (type === "array" && required) {
    return [];
  }
  if (type === "boolean" && required) {
    return false;
  }
  return undefined;
}

function sameJSONValue(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

function cloneRecord(value: ConfigValue): ConfigValue {
  return cloneJSONValue(value);
}

export function stringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}
