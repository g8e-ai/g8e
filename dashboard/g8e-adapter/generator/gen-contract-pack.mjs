// Deterministic contract pack generator for the g8e observe frontend.
//
// Reads canonical protocol JSON (the source of truth for wire shapes) and the
// audited compiled adapter dist (the source of truth for runtime config and
// the endpoint allowlist), then emits a self-contained contract pack that a
// generator-neutral builder (Lovable, Notion, or other) consumes to produce a
// deployable SPA. The generated SPA wraps the audited g8e-adapter package; it
// does not rewrite adapter transport code.
//
// Usage:
//   node generator/gen-contract-pack.mjs           # write outputs
//   node generator/gen-contract-pack.mjs --check   # fail if outputs are stale
//
// Re-running against identical inputs produces byte-identical files. The
// manifest records the schema version and SHA-256 of every output so a check
// mode can detect drift without rewriting files.
//
// This script uses only Node built-ins (node:crypto, node:fs, node:path) and
// imports the compiled adapter dist. No new dependencies are introduced. Run
// `npm run build` first so dist/ is current.

import { createHash } from 'node:crypto';
import { readFile, writeFile, mkdir, readdir, rm } from 'node:fs/promises';
import { join, dirname, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

import { RUNTIME_CONFIG_SCHEMA_VERSION } from '../dist/config/runtime_config.js';
import { ALLOWED_ENDPOINTS } from '../dist/api/allowlist.js';
import {
  AGENT_LIFECYCLE_STATUSES,
  RUN_LIFECYCLE_STATUSES,
  RUN_KINDS,
  EVAL_VERIFICATION_STATUSES,
  MEASUREMENT_STATUSES,
  SNAPSHOT_FRESHNESS_VALUES,
  DOWNLOAD_PRIVACY_CLASSIFICATIONS,
} from '../dist/types/enums.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ADAPTER_ROOT = join(__dirname, '..');
const REPO_ROOT = join(ADAPTER_ROOT, '..', '..');
const PROTOCOL_MODELS = join(REPO_ROOT, 'protocol', 'models');
const PROTOCOL_CONSTANTS = join(REPO_ROOT, 'protocol', 'constants');
const OUT_DIR = join(ADAPTER_ROOT, 'contract-pack');
const FIXTURES_DIR = join(OUT_DIR, 'fixtures');

const CHECK_MODE = process.argv.includes('--check');

// ---------------------------------------------------------------------------
// Deterministic JSON serialization (sorted keys, 2-space indent, trailing
// newline). Object key insertion order does not affect output.
// ---------------------------------------------------------------------------

function stableSerialize(value) {
  const seen = new WeakSet();
  const sort = (v) => {
    if (v === null || typeof v !== 'object') return v;
    if (Array.isArray(v)) return v.map(sort);
    if (seen.has(v)) throw new Error('cycle detected in contract pack data');
    seen.add(v);
    const sorted = {};
    for (const key of Object.keys(v).sort()) sorted[key] = sort(v[key]);
    return sorted;
  };
  return JSON.stringify(sort(value), null, 2) + '\n';
}

async function writeText(filePath, content) {
  await mkdir(dirname(filePath), { recursive: true });
  await writeFile(filePath, content, 'utf8');
}

async function readJson(filePath) {
  return JSON.parse(await readFile(filePath, 'utf8'));
}

function sha256(content) {
  return createHash('sha256').update(content, 'utf8').digest('hex');
}

// ---------------------------------------------------------------------------
// Enum registry. Values come from the audited adapter dist (the compiled
// enums.ts), so generated models.ts enums match the adapter byte-for-byte.
// ---------------------------------------------------------------------------

const ENUMS = [
  { name: 'AgentLifecycleStatus', constName: 'AGENT_LIFECYCLE_STATUSES', values: AGENT_LIFECYCLE_STATUSES },
  { name: 'RunLifecycleStatus', constName: 'RUN_LIFECYCLE_STATUSES', values: RUN_LIFECYCLE_STATUSES },
  { name: 'RunKind', constName: 'RUN_KINDS', values: RUN_KINDS },
  { name: 'EvalVerificationStatus', constName: 'EVAL_VERIFICATION_STATUSES', values: EVAL_VERIFICATION_STATUSES },
  { name: 'MeasurementStatus', constName: 'MEASUREMENT_STATUSES', values: MEASUREMENT_STATUSES },
  { name: 'SnapshotFreshness', constName: 'SNAPSHOT_FRESHNESS_VALUES', values: SNAPSHOT_FRESHNESS_VALUES },
  { name: 'DownloadPrivacyClassification', constName: 'DOWNLOAD_PRIVACY_CLASSIFICATIONS', values: DOWNLOAD_PRIVACY_CLASSIFICATIONS },
];

function findEnumByValues(values) {
  for (const e of ENUMS) {
    if (e.values.length === values.length && e.values.every((v, i) => v === values[i])) return e;
  }
  return null;
}

function pascalCase(name) {
  return name.split('_').map((s) => s.charAt(0).toUpperCase() + s.slice(1)).join('');
}

// ---------------------------------------------------------------------------
// Protocol model loading. Browser-facing read models and event payloads only.
// Producer request/response models are mTLS-internal and excluded from the
// builder contract.
// ---------------------------------------------------------------------------

const observeApi = await readJson(join(PROTOCOL_MODELS, 'observe_api.json'));
const eventPayloads = await readJson(join(PROTOCOL_MODELS, 'observe_event_payloads.json'));
const classification = await readJson(join(PROTOCOL_CONSTANTS, 'event_dashboard_classification.json'));

const BROWSER_MODELS = [
  'observed_measurement',
  'agent_state_projection',
  'overview_counters',
  'overview_measurements',
  'run_summary',
  'run_task',
  'evidence_safe_link',
  'run_detail',
  'eval_summary',
  'eval_metric_summary',
  'eval_detail',
  'download_artifact',
  'observe_bootstrap_snapshot',
];

const EVENT_PAYLOAD_MODELS = [
  'agent_status_updated_payload',
  'run_status_updated_payload',
  'eval_run_completed_payload',
  'eval_metric_recorded_payload',
];

const EXCLUDED_MODELS = new Set([
  'observe_producer_agent_state_request',
  'observe_producer_run_state_request',
  'observe_producer_response',
]);

// Inline object models discovered during type resolution (e.g. success_rate).
const inlineModels = new Map();

function modelFields(modelName) {
  const def = observeApi[modelName] ?? eventPayloads[modelName];
  if (!def) throw new Error(`unknown model: ${modelName}`);
  const fields = {};
  for (const key of Object.keys(def)) {
    if (key.startsWith('_')) continue;
    fields[key] = def[key];
  }
  return fields;
}

// ---------------------------------------------------------------------------
// TypeScript models + validators generation (contract-pack/models.ts).
// ---------------------------------------------------------------------------

function generateEnums() {
  const lines = [];
  for (const e of ENUMS) {
    const union = e.values.map((v) => `'${v}'`).join(' | ');
    lines.push(`export type ${e.name} = ${union};`);
    lines.push(`export const ${e.constName}: readonly ${e.name}[] = [`);
    lines.push(`  ${e.values.map((v) => `'${v}'`).join(', ')},`);
    lines.push(`] as const;`);
    lines.push(`export function is${e.name}(value: unknown): value is ${e.name} {`);
    lines.push(`  return typeof value === 'string' && (${e.constName} as readonly string[]).includes(value);`);
    lines.push(`}`);
    lines.push('');
  }
  return lines.join('\n');
}

function generateInterfaceClean(modelName) {
  const fields = modelFields(modelName);
  const name = pascalCase(modelName);
  const lines = [`export interface ${name} {`];
  for (const key of Object.keys(fields)) {
    const field = fields[key];
    const required = field.required !== false;
    const opt = required ? '' : '?';
    let base;
    switch (field.type) {
      case 'string':
        if (field.enum) {
          const e = findEnumByValues(field.enum);
          base = e ? e.name : field.enum.map((v) => `'${v}'`).join(' | ');
        } else {
          base = 'string';
        }
        break;
      case 'integer':
      case 'number':
        base = 'number';
        break;
      case 'boolean':
        base = 'boolean';
        break;
      case 'object':
        if (field.model) {
          base = pascalCase(field.model);
        } else if (field.properties) {
          const inlineName = `${pascalCase(modelName)}${pascalCase(key)}`;
          if (!inlineModels.has(inlineName)) inlineModels.set(inlineName, field.properties);
          base = inlineName;
        } else {
          base = 'Record<string, unknown>';
        }
        break;
      case 'array':
        base = field.items ? `${pascalCase(field.items)}[]` : 'unknown[]';
        break;
      default:
        throw new Error(`unsupported field type ${field.type} on ${modelName}.${key}`);
    }
    lines.push(`  ${key}${opt}: ${base};`);
  }
  lines.push('}');
  return lines.join('\n');
}

function generateInlineInterface(name, properties) {
  const lines = [`export interface ${name} {`];
  for (const key of Object.keys(properties).sort()) {
    const field = properties[key];
    const required = field.required !== false;
    const opt = required ? '' : '?';
    let base;
    switch (field.type) {
      case 'string':
        base = field.enum ? (findEnumByValues(field.enum)?.name ?? field.enum.map((v) => `'${v}'`).join(' | ')) : 'string';
        break;
      case 'integer':
      case 'number':
        base = 'number';
        break;
      case 'boolean':
        base = 'boolean';
        break;
      default:
        base = 'Record<string, unknown>';
    }
    lines.push(`  ${key}${opt}: ${base};`);
  }
  lines.push('}');
  return lines.join('\n');
}

// Validator generation. Emits a structural type guard per model.
function validatorForType(field, accessor) {
  // Returns an array of condition strings (without surrounding if). The
  // accessor is the JS expression for the field value.
  const conds = [];
  const required = field.required !== false;
  switch (field.type) {
    case 'string':
      if (field.enum) {
        const e = findEnumByValues(field.enum);
        if (e) {
          conds.push(`typeof ${accessor} === 'string' && (${e.constName} as readonly string[]).includes(${accessor})`);
        } else {
          conds.push(`typeof ${accessor} === 'string' && ${JSON.stringify(field.enum)}.includes(${accessor} as string)`);
        }
      } else {
        conds.push(`typeof ${accessor} === 'string'`);
      }
      break;
    case 'integer':
      conds.push(`typeof ${accessor} === 'number' && Number.isInteger(${accessor})`);
      break;
    case 'number':
      conds.push(`typeof ${accessor} === 'number'`);
      break;
    case 'boolean':
      conds.push(`typeof ${accessor} === 'boolean'`);
      break;
    case 'object':
      if (field.model) {
        conds.push(`is${pascalCase(field.model)}(${accessor})`);
      } else if (field.properties) {
        const inlineName = `${pascalCase(field._parentModel)}${pascalCase(field._fieldName)}`;
        conds.push(`is${inlineName}(${accessor})`);
      } else {
        conds.push(`typeof ${accessor} === 'object' && ${accessor} !== null && !Array.isArray(${accessor})`);
      }
      break;
    case 'array':
      if (field.items) {
        conds.push(`Array.isArray(${accessor}) && ${accessor}.every((x) => is${pascalCase(field.items)}(x))`);
      } else {
        conds.push(`Array.isArray(${accessor})`);
      }
      break;
    default:
      throw new Error(`unsupported field type ${field.type}`);
  }
  return { conds, required };
}

function generateValidator(modelName) {
  const fields = modelFields(modelName);
  const name = pascalCase(modelName);
  const lines = [];
  lines.push(`export function is${name}(value: unknown): value is ${name} {`);
  lines.push(`  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;`);
  lines.push(`  const v = value as Record<string, unknown>;`);
  for (const key of Object.keys(fields)) {
    const field = { ...fields[key], _parentModel: modelName, _fieldName: key };
    const accessor = `v.${key}`;
    const { conds, required } = validatorForType(field, accessor);
    const check = conds.join(' && ');
    if (required) {
      lines.push(`  if (!(${check})) return false; /* ${key} */`);
    } else {
      lines.push(`  if (v.${key} !== undefined && !(${check})) return false; /* ${key} */`);
    }
  }
  lines.push(`  return true;`);
  lines.push(`}`);
  return lines.join('\n');
}

function generateInlineValidator(name, properties) {
  const lines = [];
  lines.push(`export function is${name}(value: unknown): value is ${name} {`);
  lines.push(`  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;`);
  lines.push(`  const v = value as Record<string, unknown>;`);
  for (const key of Object.keys(properties).sort()) {
    const field = properties[key];
    const required = field.required !== false;
    const accessor = `v.${key}`;
    let check;
    switch (field.type) {
      case 'string':
        check = field.enum
          ? `typeof ${accessor} === 'string' && (${findEnumByValues(field.enum)?.constName ?? JSON.stringify(field.enum)} as readonly string[]).includes(${accessor})`
          : `typeof ${accessor} === 'string'`;
        break;
      case 'integer':
        check = `typeof ${accessor} === 'number' && Number.isInteger(${accessor})`;
        break;
      case 'number':
        check = `typeof ${accessor} === 'number'`;
        break;
      case 'boolean':
        check = `typeof ${accessor} === 'boolean'`;
        break;
      default:
        check = `typeof ${accessor} === 'object' && ${accessor} !== null`;
    }
    if (required) {
      lines.push(`  if (!(${check})) return false; /* ${key} */`);
    } else {
      lines.push(`  if (v.${key} !== undefined && !(${check})) return false; /* ${key} */`);
    }
  }
  lines.push(`  return true;`);
  lines.push(`}`);
  return lines.join('\n');
}

function generateModelsTs() {
  inlineModels.clear();
  const sections = [];
  sections.push('// AUTO-GENERATED by generator/gen-contract-pack.mjs. Do not edit by hand.');
  sections.push('// Typed observe read models, event payloads, and validators derived from');
  sections.push('// protocol/models/observe_api.json and protocol/models/observe_event_payloads.json.');
  sections.push('// Field names and enum values match the Go wire shapes byte-for-byte. The');
  sections.push('// audited g8e-adapter package is the source of truth for runtime config and the');
  sections.push('// endpoint allowlist; builders import this file for typed models/validators and');
  sections.push('// wrap the adapter for transport. Regenerate with `npm run gen:contract-pack`.');
  sections.push('');
  sections.push(generateEnums());
  sections.push('export type UTCDatetime = string;');
  sections.push('');

  // Inline models first (dependencies), then browser models, then event payloads.
  // Generate interfaces for browser models to populate inlineModels.
  const interfaceBlocks = [];
  for (const m of BROWSER_MODELS) interfaceBlocks.push(generateInterfaceClean(m));
  for (const m of EVENT_PAYLOAD_MODELS) interfaceBlocks.push(generateInterfaceClean(m));

  // Inline interfaces discovered during resolution.
  const inlineBlocks = [];
  for (const [name, props] of inlineModels) inlineBlocks.push(generateInlineInterface(name, props));

  sections.push(inlineBlocks.join('\n\n'));
  sections.push('');
  sections.push(interfaceBlocks.join('\n\n'));
  sections.push('');

  // ObservePage is generic over its item type.
  sections.push('export interface ObservePage<T> {');
  sections.push('  schema_version: string;');
  sections.push('  items: T[];');
  sections.push('  cursor?: string;');
  sections.push('  has_more: boolean;');
  sections.push('  limit: number;');
  sections.push('}');
  sections.push('');

  // Validators.
  const validatorBlocks = [];
  for (const [name, props] of inlineModels) validatorBlocks.push(generateInlineValidator(name, props));
  for (const m of BROWSER_MODELS) validatorBlocks.push(generateValidator(m));
  for (const m of EVENT_PAYLOAD_MODELS) validatorBlocks.push(generateValidator(m));
  sections.push(validatorBlocks.join('\n\n'));
  sections.push('');

  return sections.join('\n');
}

// ---------------------------------------------------------------------------
// Runtime config JSON schema (matches the audited adapter parseRuntimeConfig).
// ---------------------------------------------------------------------------

function generateRuntimeConfigSchema() {
  return stableSerialize({
    $schema: 'https://json-schema.org/draft/2020-12/schema',
    $id: 'https://g8e/contract-pack/runtime-config.schema.json',
    title: 'FrontendRuntimeConfig',
    type: 'object',
    additionalProperties: false,
    required: ['schema_version', 'gateway_base_url', 'passkey_rp_id', 'passkey_rp_name', 'app_name'],
    properties: {
      schema_version: { type: 'string', const: RUNTIME_CONFIG_SCHEMA_VERSION },
      gateway_base_url: {
        type: 'string',
        description: 'Absolute Gateway origin. HTTPS required except loopback http (localhost, 127.0.0.1, [::1]). No userinfo, fragment, path, or query.',
        pattern: '^https://[^/?#]+$|^http://(localhost|127\\.0\\.0\\.1|\\[::1\\])(:\\d+)?/?$',
      },
      passkey_rp_id: {
        type: 'string',
        description: 'Bare domain (no protocol, port, path, fragment, or userinfo).',
        pattern: '^[^/?#:@]+$',
      },
      passkey_rp_name: { type: 'string', minLength: 1 },
      app_name: { type: 'string', minLength: 1 },
      documentation_base_url: {
        type: 'string',
        description: 'Optional docs origin. HTTPS or loopback http only.',
        pattern: '^https://[^/?#]+$|^http://(localhost|127\\.0\\.0\\.1|\\[::1\\])(:\\d+)?/?$',
      },
      features: {
        type: 'object',
        additionalProperties: false,
        properties: {
          design_preview: { type: 'boolean' },
          eval_publication: { type: 'boolean' },
          downloads: { type: 'boolean' },
        },
      },
    },
  });
}

// ---------------------------------------------------------------------------
// Curated OpenAPI for the allowlisted browser operations.
// ---------------------------------------------------------------------------

function openApiSchemaRef(modelName) {
  return { $ref: `#/components/schemas/${pascalCase(modelName)}` };
}

function generateOpenApi() {
  const paths = {};
  for (const ep of ALLOWED_ENDPOINTS) {
    // Convert the adapter regex pathPattern back to an OpenAPI path with params.
    const source = ep.pathPattern.source;
    let path = source
      .replace(/^\^/, '')
      .replace(/\$$/, '')
      .replace(/\\\//g, '/')
      .replace(/\\\./g, '.')
      .replace(/\[\^\/\]\+/g, '{id}');
    const method = ep.method.toLowerCase();
    const op = {
      operationId: ep.name,
      summary: ep.streaming ? 'SSE stream (server-push)' : ep.name,
    };
    if (method === 'get') {
      if (ep.streaming) {
        op.responses = {
          '200': { description: 'Server-Sent Events stream', content: { 'text/event-stream': { schema: { type: 'string' } } } },
        };
      } else {
        op.responses = { '200': { description: 'OK' } };
      }
      if (ep.name === 'observe_runs' || ep.name === 'observe_evals' || ep.name === 'observe_downloads' || ep.name === 'sse_events') {
        op.parameters = [
          { name: 'cursor', in: 'query', required: false, schema: { type: 'string' } },
          { name: 'limit', in: 'query', required: false, schema: { type: 'integer' } },
        ];
      }
      if (ep.name === 'sse_events') {
        op.parameters = [
          { name: 'since_id', in: 'query', required: false, schema: { type: 'integer' } },
          { name: 'limit', in: 'query', required: false, schema: { type: 'integer' } },
        ];
      }
      if (ep.name === 'sse_stream') {
        op.parameters = [{ name: 'since_id', in: 'query', required: false, schema: { type: 'integer' } }];
      }
    } else {
      op.requestBody = { required: true, content: { 'application/json': { schema: { type: 'object' } } } };
      op.responses = { '200': { description: 'OK' } };
    }
    if (!paths[path]) paths[path] = {};
    paths[path][method] = op;
  }

  // Attach response schema refs for observe reads.
  const observeResponses = {
    '/api/v1/observe/bootstrap': { '200': openApiSchemaRef('observe_bootstrap_snapshot') },
    '/api/v1/observe/runs': { '200': openApiSchemaRef('observe_page_run_summary') },
    '/api/v1/observe/runs/{id}': { '200': openApiSchemaRef('run_detail') },
    '/api/v1/observe/evals': { '200': openApiSchemaRef('observe_page_eval_summary') },
    '/api/v1/observe/evals/{id}': { '200': openApiSchemaRef('eval_detail') },
    '/api/v1/observe/downloads': { '200': openApiSchemaRef('observe_page_download_artifact') },
    '/api/v1/observe/downloads/{id}': { '200': openApiSchemaRef('download_artifact') },
  };
  for (const [p, resp] of Object.entries(observeResponses)) {
    if (paths[p]?.get) {
      paths[p].get.responses = { '200': { description: 'OK', content: { 'application/json': { schema: resp['200'] } } } };
    }
  }

  const components = { schemas: {} };
  for (const m of BROWSER_MODELS) {
    components.schemas[pascalCase(m)] = { $ref: `#/x-models/${m}` };
  }
  components.schemas['ObservePageRunSummary'] = { $ref: '#/x-models/observe_page_run_summary' };
  components.schemas['ObservePageEvalSummary'] = { $ref: '#/x-models/observe_page_eval_summary' };
  components.schemas['ObservePageDownloadArtifact'] = { $ref: '#/x-models/observe_page_download_artifact' };

  const doc = {
    openapi: '3.0.3',
    info: {
      title: 'g8e Observe Browser API',
      version: RUNTIME_CONFIG_SCHEMA_VERSION,
      description: 'Curated read-only operations available to a passkey-authenticated browser SPA. All requests use credentials: include. The endpoint allowlist is enforced by the audited g8e-adapter; builders must not call non-allowlisted routes. mTLS producer endpoints are excluded (ensemble-internal).',
    },
    servers: [{ url: '/api/v1', description: 'Gateway base URL from runtime config' }],
    paths,
    components,
    'x-models': { ...observeApi, ...eventPayloads },
  };
  return stableSerialize(doc);
}

// ---------------------------------------------------------------------------
// Event schemas (the four dashboard event payloads + sentinel events).
// ---------------------------------------------------------------------------

function generateEventSchemas() {
  const families = classification.families ?? {};
  const observeFamilies = {};
  for (const [k, v] of Object.entries(families)) {
    if (k === 'g8e.v1.app.agent' || k === 'g8e.v1.app.run' || k === 'g8e.v1.ai.eval') {
      observeFamilies[k] = v;
    }
  }
  const doc = {
    schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
    description: 'Supported dashboard event payloads carried inside SSE envelopes. The adapter normalizes the outer push envelope, parses nested string events, validates recognized type/data pairs, and prevents routing IDs from becoming trusted payload.',
    event_types: {
      'app.agent.status.updated': { family: 'g8e.v1.app.agent', payload_model: 'agent_status_updated_payload' },
      'app.run.status.updated': { family: 'g8e.v1.app.run', payload_model: 'run_status_updated_payload' },
      'ai.eval.run.completed': { family: 'g8e.v1.ai.eval', payload_model: 'eval_run_completed_payload' },
      'ai.eval.metric.recorded': { family: 'g8e.v1.ai.eval', payload_model: 'eval_metric_recorded_payload' },
    },
    sentinel_event_types: {
      truncated: { description: 'Replay gap marker. Carries since_id and limit. Triggers snapshot reconciliation.' },
      error: { description: 'Replay failure marker. Carries a reason string. Triggers snapshot reconciliation.' },
    },
    families: observeFamilies,
    payloads: {
      agent_status_updated_payload: eventPayloads.agent_status_updated_payload,
      run_status_updated_payload: eventPayloads.run_status_updated_payload,
      eval_run_completed_payload: eventPayloads.eval_run_completed_payload,
      eval_metric_recorded_payload: eventPayloads.eval_metric_recorded_payload,
    },
  };
  return stableSerialize(doc);
}

// ---------------------------------------------------------------------------
// Typed fixture scenarios. Every fixture parses through the generated
// validators. No fabricated operational numbers: resource/throughput cards
// are unavailable, evals are partial/unavailable, task counts are truthful.
// ---------------------------------------------------------------------------

const ISO = '2026-09-09T12:00:00Z';

function agentProjection(overrides = {}) {
  return {
    schema_version: '1.0.0',
    agent_id: 'user-a:sage',
    display_name: 'Sage',
    role: 'sage',
    status: 'running',
    freshness: 'observed',
    observed_at: ISO,
    ...overrides,
  };
}

function runSummary(overrides = {}) {
  return {
    schema_version: '1.0.0',
    run_id: 'inv-001',
    run_kind: 'investigation',
    display_name: 'Disclosure-safe case title',
    status: 'running',
    completed_tasks: 0,
    total_tasks: 0,
    has_receipts: false,
    evidence_count: 0,
    observed_at: ISO,
    ...overrides,
  };
}

function bootstrapSnapshot(overrides = {}) {
  return {
    schema_version: '1.0.0',
    agents: [],
    overview: {
      schema_version: '1.0.0',
      agents_running: 0,
      agents_running_freshness: 'observed',
      tasks_in_queue: 0,
      tasks_in_queue_freshness: 'observed',
      generated_at: ISO,
    },
    measurements: { schema_version: '1.0.0' },
    recent_runs: [],
    latest_evals: [],
    downloads: [],
    generated_at: ISO,
    ...overrides,
  };
}

function sseEvent(type, payload, id = 1) {
  return { id, type, payload, recognized: true };
}

function generateFixtures() {
  const fixtures = {};

  fixtures['unauthenticated.json'] = stableSerialize({
    scenario: 'unauthenticated',
    description: 'No session. Bootstrap status is false; the SPA shows the login ceremony. No observe data is available.',
    bootstrap_status: false,
    auth: { status: 'unauthenticated' },
    observe: { available: false, reason: 'unauthenticated' },
  });

  fixtures['bootstrap.json'] = stableSerialize({
    scenario: 'bootstrap',
    description: 'Authenticated first paint. One running agent, one active investigation run, empty evals, no downloads. Resource/throughput measurements omitted (unavailable).',
    snapshot: bootstrapSnapshot({
      agents: [agentProjection()],
      active_run: runSummary(),
      overview: {
        schema_version: '1.0.0',
        agents_running: 1,
        agents_running_freshness: 'observed',
        tasks_in_queue: 0,
        tasks_in_queue_freshness: 'observed',
        generated_at: ISO,
      },
      recent_runs: [runSummary()],
    }),
  });

  fixtures['live-stream.json'] = stableSerialize({
    scenario: 'live-stream',
    description: 'Connected SSE stream with nested typed agent and run status update events. Routing IDs are not part of the trusted payload.',
    events: [
      sseEvent('app.agent.status.updated', {
        schema_version: '1.0.0',
        agent_id: 'user-a:sage',
        display_name: 'Sage',
        role: 'sage',
        status: 'running',
        observed_at: ISO,
      }, 10),
      sseEvent('app.run.status.updated', {
        schema_version: '1.0.0',
        run_id: 'inv-001',
        run_kind: 'investigation',
        display_name: 'Disclosure-safe case title',
        status: 'running',
        completed_tasks: 0,
        total_tasks: 0,
        observed_at: ISO,
      }, 11),
    ],
  });

  fixtures['reconnect.json'] = stableSerialize({
    scenario: 'reconnect',
    description: 'Reconnect after a dropped connection. The adapter reopens EventSource with since_id and reconciles the snapshot.',
    events: [
      sseEvent('app.agent.status.updated', {
        schema_version: '1.0.0',
        agent_id: 'user-a:sage',
        display_name: 'Sage',
        role: 'sage',
        status: 'waiting',
        observed_at: ISO,
      }, 20),
    ],
    transport: { connection_state: 'reconnecting', last_event_id: 20 },
  });

  fixtures['replay-gap.json'] = stableSerialize({
    scenario: 'replay-gap',
    description: 'Truncation sentinel during replay. The adapter triggers snapshot reconciliation; SSE is invalidation, not durable truth.',
    events: [
      { id: 30, type: 'truncated', recognized: true, payload: { since_id: 25, limit: 100 } },
    ],
    transport: { connection_state: 'connected', last_event_id: 30, reconcile_reason: 'truncated' },
  });

  fixtures['empty-evals.json'] = stableSerialize({
    scenario: 'empty-evals',
    description: 'Authenticated snapshot with no published evals. The evals section shows an explicit empty state, not a placeholder.',
    snapshot: bootstrapSnapshot(),
  });

  fixtures['partial-verification.json'] = stableSerialize({
    scenario: 'partial-verification',
    description: 'One eval summary with projection_validated verification status. The complete verifier is not implemented; verified is never claimed.',
    snapshot: bootstrapSnapshot({
      latest_evals: [{
        schema_version: '1.0.0',
        run_id: 'eval-001',
        suite_id: 'suite-a',
        suite_version: '1.0.0',
        arm_id: 'arm-1',
        status: 'completed',
        verification_status: 'projection_validated',
        receipt_count: 3,
        metric_count: 2,
        published_projection_sha256: 'a'.repeat(64),
        completed_at: ISO,
        observed_at: ISO,
      }],
    }),
  });

  fixtures['stale-telemetry.json'] = stableSerialize({
    scenario: 'stale-telemetry',
    description: 'Overview counters marked stale. The SPA shows a stale label, not a fabricated fresh value.',
    snapshot: bootstrapSnapshot({
      overview: {
        schema_version: '1.0.0',
        agents_running: 1,
        agents_running_freshness: 'stale',
        tasks_in_queue: 0,
        tasks_in_queue_freshness: 'stale',
        generated_at: ISO,
      },
    }),
  });

  fixtures['unavailable-metrics.json'] = stableSerialize({
    scenario: 'unavailable-metrics',
    description: 'Resource and throughput measurements omitted. The SPA shows unavailable cards, not fabricated CPU/RAM/throughput values.',
    snapshot: bootstrapSnapshot({
      measurements: { schema_version: '1.0.0' },
    }),
  });

  fixtures['wrong-user-rejection.json'] = stableSerialize({
    scenario: 'wrong-user-rejection',
    description: 'A cross-user observe read is rejected by the Gateway. The browser receives no other user projections, evals, or downloads.',
    auth: { status: 'authenticated' },
    observe: { available: true, cross_user_isolation: true, rejected_user: 'user-b', reason: 'forbidden' },
  });

  fixtures['download-availability.json'] = stableSerialize({
    scenario: 'download-availability',
    description: 'One public-safe download artifact in the catalog with a SHA-256 hash and authenticated URL. Restricted artifacts never appear.',
    snapshot: bootstrapSnapshot({
      downloads: [{
        schema_version: '1.0.0',
        artifact_id: 'art-001',
        filename: 'report.json',
        media_type: 'application/json',
        byte_size: 1024,
        sha256: 'b'.repeat(64),
        privacy_classification: 'public_safe',
        source_run_id: 'eval-001',
        download_url: '/api/v1/observe/downloads/art-001',
        generated_at: ISO,
      }],
    }),
  });

  return fixtures;
}

// ---------------------------------------------------------------------------
// Builder prompt (Packet 8B). Encodes every Packet 8C requirement as
// instructions the builder must follow. Generated, not hand-edited.
// ---------------------------------------------------------------------------

function generateBuilderPrompt() {
  return `# g8e Observe Frontend Builder Prompt

This prompt instructs a generator-neutral builder (Lovable, Notion, or other supported SPA builder) to produce a deployable observability frontend that wraps the audited g8e-adapter package. The generated SPA connects to a local g8e Gateway over HTTPS, authenticates with WebAuthn passkeys, reads only typed user-scoped observe data, and renders normalized nested SSE updates. The builder generates presentation code only; it does not rewrite adapter transport code.

## Hard constraints

1. Preserve the adapter boundary. The audited g8e-adapter package owns runtime config parsing, the endpoint allowlist, credentialed fetch, WebAuthn ceremonies, SSE normalization, snapshot reconciliation, typed stores, and the safe presentation registry. Generated code imports from the adapter and calls its exported APIs. Generated code never reimplements transport, auth, SSE parsing, or allowlist enforcement.
2. Execute Gateway requests only in the top-level browser context. No server-side proxy, no service-worker relay, no iframe delegation. The SPA runs at a single top-level origin.
3. Use the absolute configured Gateway origin from FrontendRuntimeConfig for every request. Never hardcode an origin, never derive it from window.location, never allow a relative URL.
4. Include credentials on every fetch and every EventSource. All requests use credentials: 'include'. The session cookie is cross-origin Secure HttpOnly.
5. Implement the exact WebAuthn and nested SSE contracts documented in this pack. Use the adapter's webauthn and sse modules; do not hand-roll base64url conversion, attestation/assertion wire shapes, or envelope parsing.
6. Call only allowlisted operations. The adapter's endpoint allowlist is the complete set of reachable Gateway routes. Do not add a generic arbitrary-path request helper. Do not call SSE push, producer, audit, blob, filesystem, pub/sub, MCP, A2A, approval, chat, tool, or eval-launch routes.
7. Isolate visibly labeled design-preview fixtures. When features.design_preview is enabled, fixtures are shown in a clearly labeled design-preview mode that never mixes with connected data. In production, design-preview is disabled unless runtime configuration deliberately enables it.
8. Show unavailable states instead of placeholder values. Every dynamic field has a typed source or an explicit unavailable/unsupported/stale/empty/loading/error state. Never fabricate a value when the typed source is absent.

## What the SPA must display

Populate every section in the homepage matrix from typed stores only. Static sections remain static. Dynamic values come only from observe reads, normalized safe events, runtime health, or explicit unavailable state.

- Connection state: disconnected, connecting, connected, reconnecting, unauthenticated. Use non-color labels.
- Auth state: unauthenticated, bootstrapping, enrolling, authenticating, authenticated, error.
- Overview counters: agents running and tasks in queue, each with a freshness label (observed, stale, unavailable). Success rate only when a typed success_rate measurement exists.
- Agent roster: each agent's display name, role, status label, and freshness. Throughput only when a typed throughput measurement exists.
- Active run and recent runs: display name, run kind, status label, completed/total task counts. No fabricated task counts.
- Latest evals: run id, verification label, receipt count, metric count. Partial verification shown as projection_validated, never as verified until the complete verifier exists.
- Downloads: filename, media type, byte size, SHA-256, privacy classification, authenticated URL. Only public_safe artifacts appear.
- Live narrative: bounded, virtualized live event rows from normalized safe events. Unknown events appear only as a bounded diagnostic row and cannot mutate projections or counters.

## What the SPA must NOT display

Do not display throughput, CPU, RAM, VRAM, disk, parameter counts, quantization, artifact formats, file sizes, success rates, or verification claims unless the corresponding typed observed source exists. Resource and throughput cards remain unavailable until a real host telemetry collector exists. Remove dead controls and screenshot-only calls to action. "Watch Live" authenticates or focuses the stream; it never starts work.

## Accessibility and responsive behavior

- Keyboard navigation across all interactive controls with visible focus.
- Semantic landmarks and headings (header, main, nav, section).
- Non-color status labels for every state (text label plus color).
- Reduced motion support (respect prefers-reduced-motion).
- Bounded or virtualized live rows so a long event stream does not block the main thread.
- Screen-reader connection announcements when the SSE connection state changes.
- Mobile-first login and live-status layout.

## Design-preview mode

Design-preview mode is explicit, visibly labeled, and disabled in production unless runtime configuration deliberately enables it. Fixtures never mix with connected data. A design-preview banner is shown whenever the mode is active.

## Acceptance

The generated SPA must pass the acceptance commands in README.md. The connected page contains no fixture leakage, fabricated values, dead controls, unsupported claims, or mutation surface. At least one supported builder can consume this pack and produce a deployable SPA that connects through the untouched audited adapter.
`;
}

// ---------------------------------------------------------------------------
// README for the contract pack.
// ---------------------------------------------------------------------------

function generateReadme() {
  return `# g8e Observe Frontend Contract Pack

This contract pack is the deterministic, generator-neutral input a builder (Lovable, Notion, or other supported SPA builder) consumes to produce a deployable observability frontend that wraps the audited g8e-adapter package. The pack is generated from canonical protocol JSON and the audited adapter; regenerate it with \`npm run gen:contract-pack\` from \`dashboard/g8e-adapter/\`.

## Contents

- \`builder-prompt.md\` — the prompt to give a builder. Encodes every hard constraint the generated SPA must satisfy.
- \`runtime-config.schema.json\` — JSON Schema for FrontendRuntimeConfig. The SPA reads its config from a JSON script tag and validates it with the adapter's parseRuntimeConfig.
- \`observe.openapi.json\` — curated OpenAPI 3.0 for the allowlisted browser operations. mTLS producer endpoints are excluded.
- \`event-schemas.json\` — the four dashboard event payloads plus sentinel events, with family classifications.
- \`models.ts\` — standalone TypeScript models and validators derived from protocol JSON. Builders import these for typed presentation code.
- \`fixtures/\` — typed fixture scenarios for unauthenticated, bootstrap, connected live stream, reconnect, replay gap, empty evals, partial verification, stale telemetry, unavailable metrics, wrong-user rejection, and download availability. Every fixture parses through the generated validators.
- \`manifest.json\` — schema version and SHA-256 of every output. Detects drift.

## Workflow

1. A builder consumes \`builder-prompt.md\` plus the models, OpenAPI, event schemas, and fixtures.
2. The builder generates presentation code that imports the audited g8e-adapter for transport, auth, SSE, and state.
3. The generated SPA is deployed at a top-level origin.
4. The owner connects the origin to a local Gateway with \`./g8e gw connect <origin>\`.
5. The SPA authenticates with a passkey, reads typed observe data, and renders normalized SSE updates.

## Acceptance commands

Run from \`dashboard/g8e-adapter/\`:

\`\`\`bash
npm run gen:contract-pack:check   # fail if committed outputs are stale
npm test                          # adapter + contract pack tests
npm run lint                      # type-check adapter and host
npm run build                     # build the audited adapter
npm run build:host                # build the minimal host
\`\`\`

The connected page must contain no fixture leakage, fabricated values, dead controls, unsupported claims, or mutation surface. Real-browser acceptance (exact-origin CORS, WebAuthn authenticator, SSE credentials, two-user isolation) is an owner-operated gate documented in the release plan.

## Determinism

Re-running the generator against identical inputs produces byte-identical files. JSON outputs use sorted keys and a fixed 2-space indent. The manifest records the SHA-256 of every output; \`gen:contract-pack:check\` regenerates in memory and compares hashes without rewriting files.
`;
}

// ---------------------------------------------------------------------------
// Manifest with schema versions and SHA-256 of every output.
// ---------------------------------------------------------------------------

async function buildManifest(outputs) {
  const files = [];
  for (const [relPath, content] of outputs.entries()) {
    files.push({ path: relPath, sha256: sha256(content) });
  }
  files.sort((a, b) => a.path.localeCompare(b.path));
  return stableSerialize({
    schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
    generated_at: 'deterministic',
    generator: 'dashboard/g8e-adapter/generator/gen-contract-pack.mjs',
    files,
  });
}

// ---------------------------------------------------------------------------
// Main: assemble outputs, then write or check.
// ---------------------------------------------------------------------------

async function main() {
  const modelsTs = generateModelsTs();
  const fixtures = generateFixtures();

  const outputs = new Map();
  outputs.set('README.md', generateReadme());
  outputs.set('builder-prompt.md', generateBuilderPrompt());
  outputs.set('runtime-config.schema.json', generateRuntimeConfigSchema());
  outputs.set('observe.openapi.json', generateOpenApi());
  outputs.set('event-schemas.json', generateEventSchemas());
  outputs.set('models.ts', modelsTs);
  for (const [name, content] of Object.entries(fixtures)) {
    outputs.set(`fixtures/${name}`, content);
  }
  // Manifest is computed after all other outputs are finalized.
  const manifest = await buildManifest(outputs);
  outputs.set('manifest.json', manifest);

  if (CHECK_MODE) {
    let stale = 0;
    for (const [relPath, content] of outputs.entries()) {
      const abs = join(OUT_DIR, relPath);
      let existing;
      try {
        existing = await readFile(abs, 'utf8');
      } catch {
        console.error(`missing: ${relPath}`);
        stale++;
        continue;
      }
      if (existing !== content) {
        console.error(`stale: ${relPath}`);
        stale++;
      }
    }
    if (stale > 0) {
      console.error(`\n${stale} contract pack output(s) are stale or missing. Run \`npm run gen:contract-pack\` to regenerate.`);
      process.exit(1);
    }
    console.log('contract pack is up to date.');
    return;
  }

  // Write mode: clear the output directory first for determinism, then write.
  await rm(OUT_DIR, { recursive: true, force: true });
  for (const [relPath, content] of outputs.entries()) {
    await writeText(join(OUT_DIR, relPath), content);
  }
  console.log(`contract pack written: ${outputs.size} files in ${relative(ADAPTER_ROOT, OUT_DIR)}/`);
}

main().catch((err) => {
  console.error('contract pack generation failed:', err);
  process.exit(1);
});
