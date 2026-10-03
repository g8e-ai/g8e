// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Reads the Gateway's swag-generated OpenAPI 2.0 document (GET /swagger/doc.json)
// into a flat, searchable shape for the API view. Only the subset swag emits is
// modeled; anything else is ignored rather than guessed at.

export interface SchemaNode {
  $ref?: string;
  type?: string;
  format?: string;
  description?: string;
  enum?: unknown[];
  items?: SchemaNode;
  additionalProperties?: SchemaNode | boolean;
  allOf?: SchemaNode[];
  properties?: Record<string, SchemaNode>;
  required?: string[];
}

export interface ApiParameter {
  name: string;
  in: 'query' | 'path' | 'header' | 'body' | 'formData';
  description?: string;
  required: boolean;
  type?: string;
  format?: string;
  schema?: SchemaNode;
  items?: SchemaNode;
  enum?: unknown[];
}

export interface ApiResponse {
  status: string;
  description: string;
  schema?: SchemaNode;
}

export interface ApiOperation {
  /** Stable key: "METHOD /path". */
  id: string;
  method: string;
  path: string;
  tag: string;
  summary: string;
  description: string;
  consumes: string[];
  produces: string[];
  parameters: ApiParameter[];
  responses: ApiResponse[];
}

export interface ApiDoc {
  title: string;
  operations: ApiOperation[];
  /** Operations grouped by their first tag, tags sorted, operations by path then method. */
  groups: { tag: string; operations: ApiOperation[] }[];
  definitions: Record<string, SchemaNode>;
}

export const UNTAGGED = 'other';

const METHODS = ['get', 'post', 'put', 'patch', 'delete', 'head', 'options'] as const;
const METHOD_ORDER: Record<string, number> = { GET: 0, POST: 1, PUT: 2, PATCH: 3, DELETE: 4, HEAD: 5, OPTIONS: 6 };

type Raw = Record<string, unknown>;

function isObject(v: unknown): v is Raw {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

function strings(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
}

function parameter(raw: unknown): ApiParameter | null {
  if (!isObject(raw) || !str(raw.name)) return null;
  return {
    name: str(raw.name),
    in: (str(raw.in) || 'query') as ApiParameter['in'],
    description: str(raw.description) || undefined,
    required: raw.required === true,
    type: str(raw.type) || undefined,
    format: str(raw.format) || undefined,
    schema: isObject(raw.schema) ? (raw.schema as SchemaNode) : undefined,
    items: isObject(raw.items) ? (raw.items as SchemaNode) : undefined,
    enum: Array.isArray(raw.enum) ? raw.enum : undefined,
  };
}

function compareStatus(a: ApiResponse, b: ApiResponse): number {
  return a.status.localeCompare(b.status, undefined, { numeric: true });
}

/** Parses a Swagger 2.0 document. Throws when the payload is not one. */
export function parseSpec(raw: unknown): ApiDoc {
  if (!isObject(raw) || !isObject(raw.paths)) {
    throw new Error('The Gateway returned something other than an OpenAPI document.');
  }

  const operations: ApiOperation[] = [];
  for (const [path, item] of Object.entries(raw.paths)) {
    if (!isObject(item)) continue;
    for (const m of METHODS) {
      const op = item[m];
      if (!isObject(op)) continue;
      const method = m.toUpperCase();
      const responses = isObject(op.responses)
        ? Object.entries(op.responses)
            .filter((e): e is [string, Raw] => isObject(e[1]))
            .map(([status, r]) => ({
              status,
              description: str(r.description),
              schema: isObject(r.schema) ? (r.schema as SchemaNode) : undefined,
            }))
            .sort(compareStatus)
        : [];
      operations.push({
        id: `${method} ${path}`,
        method,
        path,
        tag: strings(op.tags)[0] ?? UNTAGGED,
        summary: str(op.summary),
        description: str(op.description),
        consumes: strings(op.consumes),
        produces: strings(op.produces),
        parameters: (Array.isArray(op.parameters) ? op.parameters : []).map(parameter).filter((p): p is ApiParameter => p !== null),
        responses,
      });
    }
  }

  const byTag = new Map<string, ApiOperation[]>();
  for (const op of operations) {
    const list = byTag.get(op.tag) ?? [];
    list.push(op);
    byTag.set(op.tag, list);
  }
  const groups = [...byTag.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([tag, ops]) => ({
      tag,
      operations: ops.sort((a, b) => a.path.localeCompare(b.path) || (METHOD_ORDER[a.method] ?? 9) - (METHOD_ORDER[b.method] ?? 9)),
    }));

  const info = isObject(raw.info) ? raw.info : {};
  return {
    title: str(info.title) || 'Gateway API',
    operations,
    groups,
    definitions: isObject(raw.definitions) ? (raw.definitions as Record<string, SchemaNode>) : {},
  };
}

const REF_PREFIX = '#/definitions/';

/** The definition name a `$ref` points at, or undefined for non-definition refs. */
export function refName(ref: string | undefined): string | undefined {
  return ref?.startsWith(REF_PREFIX) ? ref.slice(REF_PREFIX.length) : undefined;
}

/** A schema collapsed to the one definition it names, looking through single-element `allOf` wrappers swag emits for annotated fields. */
export function schemaRef(schema: SchemaNode | undefined): string | undefined {
  if (!schema) return undefined;
  const direct = refName(schema.$ref);
  if (direct) return direct;
  if (schema.allOf?.length === 1) return refName(schema.allOf[0]?.$ref);
  return undefined;
}

/** Short human type label: `string`, `models.Foo`, `models.Foo[]`, `map<string, integer>`. */
export function typeLabel(schema: SchemaNode | undefined): string {
  if (!schema) return '';
  const ref = schemaRef(schema);
  if (ref) return ref;
  if (schema.type === 'array') return `${typeLabel(schema.items) || 'any'}[]`;
  if (schema.type === 'object' || schema.additionalProperties) {
    const ap = schema.additionalProperties;
    if (isObject(ap)) return `map<string, ${typeLabel(ap as SchemaNode) || 'any'}>`;
    return 'object';
  }
  if (!schema.type) return '';
  return schema.format ? `${schema.type} (${schema.format})` : schema.type;
}

/** Label for a non-body parameter, which carries its type inline rather than in `schema`. */
export function parameterType(p: ApiParameter): string {
  if (p.schema) return typeLabel(p.schema);
  if (p.type === 'array') return `${typeLabel(p.items) || 'any'}[]`;
  if (!p.type) return '';
  return p.format ? `${p.type} (${p.format})` : p.type;
}

/** Substring match across the fields a reader would search by. */
export function matchesQuery(op: ApiOperation, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return [op.method, op.path, op.tag, op.summary, op.description].some((f) => f.toLowerCase().includes(q));
}

export interface SchemaProperty {
  name: string;
  required: boolean;
  type: string;
  description?: string;
  /** Definition this property's type resolves to, when it can be expanded further. */
  ref?: string;
  enumValues?: string[];
}

function expandable(schema: SchemaNode): string | undefined {
  return schemaRef(schema) ?? (schema.type === 'array' ? schemaRef(schema.items) : undefined);
}

/** Property rows for a named definition, or [] when it has none (primitives, enums, maps). */
export function definitionProperties(defs: Record<string, SchemaNode>, name: string): SchemaProperty[] {
  const def = defs[name];
  if (!def?.properties) return [];
  const required = new Set(def.required ?? []);
  return Object.entries(def.properties)
    .map(([prop, schema]) => ({
      name: prop,
      required: required.has(prop),
      type: typeLabel(schema),
      description: schema.description || undefined,
      ref: expandable(schema),
      enumValues: schema.enum?.map(String),
    }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

/** Enum values for a named definition (swag emits Go string-typed enums as definitions). */
export function definitionEnum(defs: Record<string, SchemaNode>, name: string): string[] {
  return defs[name]?.enum?.map(String) ?? [];
}
