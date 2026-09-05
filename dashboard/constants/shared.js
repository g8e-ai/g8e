// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Shared Constants Loader
 * Loads canonical wire-protocol values from protocol/constants/*.json.
 * These JSON files are the single source of truth shared across VSE (Python),
 * VSA (Go), and g8ed (JavaScript).
 *
 * Usage: import { _EVENTS, _STATUS, _MSG, _COLLECTIONS, _KV, _INTENTS } from './shared.js';
 * Single source of truth for all canonical wire-protocol values.
 */

import { createRequire } from 'module';
import { fileURLToPath } from 'url';
import path from 'path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);

const sharedDir = path.resolve(__dirname, '../../protocol/constants');

// ─── Helper functions ────────────────────────────────────────────────

function unwrapValue(obj) {
  if (obj === null || typeof obj !== 'object') return obj;
  if (Array.isArray(obj)) return obj.map(unwrapValue);
  const keys = Object.keys(obj);
  const isLeaf = keys.includes('value') && keys.every(k =>
    ['value', '_go_const', '_python_const', 'description', '_mutation'].includes(k)
  );
  if (isLeaf) return obj.value;
  const result = {};
  for (const key of keys) result[key] = unwrapValue(obj[key]);
  return result;
}

function snakeToDot(key) {
  return key.replace(/_/g, '.');
}

// ─── Per-file transforms ─────────────────────────────────────────────

function transformEvents(raw) {
  const events = raw.events || raw;
  const result = {};
  for (const [, entry] of Object.entries(events)) {
    const value = typeof entry === 'object' && entry !== null ? entry.value : entry;
    if (typeof value !== 'string') continue;
    const stripped = value.startsWith('g8e.v1.') ? value.slice('g8e.v1.'.length) : value;
    const parts = stripped.split('.');
    let node = result;
    for (let i = 0; i < parts.length; i++) {
      if (i === parts.length - 1) node[parts[i]] = value;
      else { node[parts[i]] = node[parts[i]] || {}; node = node[parts[i]]; }
    }
  }
  return result;
}

function transformStatus(raw) {
  const status = raw.status || raw;
  const result = {};
  for (const [snakeKey, group] of Object.entries(status)) {
    const dotKey = snakeToDot(snakeKey);
    const unwrapped = unwrapValue(group);
    const inner = {};
    for (const [innerKey, innerValue] of Object.entries(unwrapped)) {
      inner[snakeToDot(innerKey)] = innerValue;
    }
    result[dotKey] = inner;
  }

  // Rename operator.history.event.type → history.event.type
  if (result['operator.history.event.type']) {
    result['history.event.type'] = result['operator.history.event.type'];
    delete result['operator.history.event.type'];
  }

  // Fix session.key.prefix: strip .session suffix from inner keys
  if (result['session.key.prefix']) {
    const fixed = {};
    for (const [k, v] of Object.entries(result['session.key.prefix'])) {
      fixed[k.endsWith('.session') ? k.replace('.session', '') : k] = v;
    }
    result['session.key.prefix'] = fixed;
  }

  // Fix session.event.type: g8e.bound → operator.bound, g8e.unbound → operator.unbound
  if (result['session.event.type']) {
    const set = result['session.event.type'];
    if (set['g8e.bound']) { set['operator.bound'] = set['g8e.bound']; delete set['g8e.bound']; }
    if (set['g8e.unbound']) { set['operator.unbound'] = set['g8e.unbound']; delete set['g8e.unbound']; }
  }

  // Fix command.error.type: g8e.resolution.error → operator.resolution.error
  if (result['command.error.type']?.['g8e.resolution.error']) {
    result['command.error.type']['operator.resolution.error'] = result['command.error.type']['g8e.resolution.error'];
    delete result['command.error.type']['g8e.resolution.error'];
  }

  return result;
}

function transformCollections(raw) {
  const collections = raw.collections || raw;
  const result = {};
  for (const [snakeKey, entry] of Object.entries(collections)) {
    const value = typeof entry === 'object' && entry !== null ? entry.value : entry;
    result[snakeToDot(snakeKey)] = value;
  }
  return { collections: result };
}

function transformKvKeys(raw) {
  const kvKeys = raw.kv_keys || raw;
  const result = {};
  for (const [pascalKey, entry] of Object.entries(kvKeys)) {
    const value = typeof entry === 'object' && entry !== null ? entry.value : entry;
    result[pascalKey === 'CachePrefix' ? 'cache.version' : pascalKey] = value;
  }
  return result;
}

function transformChannels(raw) {
  return unwrapValue(raw);
}

function transformPubsub(raw) {
  return unwrapValue(raw);
}

function transformSenders(raw) {
  const senders = raw.senders || raw;
  const result = { message: { sender: {} } };
  for (const [, entry] of Object.entries(senders)) {
    const value = typeof entry === 'object' && entry !== null ? entry.value : entry;
    if (typeof value !== 'string' || !value.startsWith('g8e.v1.source.')) continue;
    const stripped = value.replace('g8e.v1.source.', '');
    const parts = stripped.split('.');
    let node = result.message.sender;
    for (let i = 0; i < parts.length; i++) {
      if (i === parts.length - 1) node[parts[i]] = value;
      else { node[parts[i]] = node[parts[i]] || {}; node = node[parts[i]]; }
    }
  }
  return result;
}

function transformHeaders(raw) {
  const unwrapped = unwrapValue(raw.headers || raw);
  const mapping = {
    'WebSessionID': 'x-vso.session-id',
    'UserID': 'x-vso.user-id',
    'OrganizationID': 'x-vso.organization-id',
    'CaseID': 'x-vso.case-id',
    'InvestigationID': 'x-vso.investigation-id',
    'TaskID': 'x-vso.task-id',
    'SourceComponent': 'x-vso.source-component',
    'BoundOperators': 'x-vso.bound-operators',
    'ExecutionID': 'x-vso.execution-id',
    'RequestedWith': 'http.requested-with',
    'CacheControl': 'http.cache-control',
    'Pragma': 'http.pragma',
    'Cookie': 'http.cookie',
    'SetCookie': 'http.set-cookie',
    'LastEventID': 'http.last-event-id',
    'AccessControlRequestHeaders': 'http.access-control-req-headers',
    'AccessControlRequestMethod': 'http.access-control-req-method',
    'AccessControlAllowOrigin': 'http.access-control-allow-origin',
    'AccessControlAllowCredentials': 'http.access-control-allow-creds',
    'ContentType': 'http.content-type',
    'XForwardedHost': 'http.x-forwarded-host',
    'XForwardedProto': 'http.x-forwarded-proto',
  };
  const result = {};
  for (const [pascalKey, dotKey] of Object.entries(mapping)) {
    if (unwrapped[pascalKey]) result[dotKey] = unwrapped[pascalKey];
  }
  return result;
}

// ─── Load and transform all 12 protocol JSON files ───────────────────

export const _EVENTS       = transformEvents(require(path.join(sharedDir, 'events.json')));
export const _STATUS       = transformStatus(require(path.join(sharedDir, 'status.json')));
export const _MSG          = transformSenders(require(path.join(sharedDir, 'senders.json')));
export const _COLLECTIONS  = transformCollections(require(path.join(sharedDir, 'collections.json')));
export const _KV           = transformKvKeys(require(path.join(sharedDir, 'kv_keys.json')));
export const _CHANNELS     = transformChannels(require(path.join(sharedDir, 'channels.json')));
export const _PUBSUB       = transformPubsub(require(path.join(sharedDir, 'pubsub.json')));
export const _INTENTS      = unwrapValue(require(path.join(sharedDir, 'intents.json')));
export const _PROMPTS      = unwrapValue(require(path.join(sharedDir, 'prompts.json')));
export const _TIMESTAMP    = unwrapValue(require(path.join(sharedDir, 'timestamp.json')));
export const _HEADERS      = transformHeaders(require(path.join(sharedDir, 'headers.json')));
export const _DOCUMENT_IDS = unwrapValue(require(path.join(sharedDir, 'document_ids.json')));

// ─── Fail-fast validation ────────────────────────────────────────────

export function assertPath(obj, pathParts, label) {
  let node = obj;
  for (const part of pathParts) {
    if (node == null || typeof node !== 'object' || !(part in node)) {
      throw new Error(`[shared.js] Validation failed: ${label} — missing key "${part}" in path [${pathParts.join('][')}]`);
    }
    node = node[part];
  }
  if (node === undefined) {
    throw new Error(`[shared.js] Validation failed: ${label} — resolved to undefined at path [${pathParts.join('][')}]`);
  }
}

assertPath(_EVENTS, ['app', 'case', 'created'], '_EVENTS.app.case.created');
assertPath(_EVENTS, ['ai', 'llm', 'chat', 'iteration', 'started'], '_EVENTS.ai.llm.chat.iteration.started');
assertPath(_STATUS, ['operator.status', 'available'], '_STATUS.operator.status.available');
assertPath(_STATUS, ['user.role', 'user'], '_STATUS.user.role.user');
assertPath(_STATUS, ['auth.provider', 'local'], '_STATUS.auth.provider.local');
assertPath(_STATUS, ['auth.method', 'kv.pubsub'], '_STATUS.auth.method.kv.pubsub');
assertPath(_STATUS, ['session.type', 'web'], '_STATUS.session.type.web');
assertPath(_STATUS, ['session.key.prefix', 'web'], '_STATUS.session.key.prefix.web');
assertPath(_STATUS, ['session.event.type', 'operator.bound'], '_STATUS.session.event.type.operator.bound');
assertPath(_STATUS, ['history.event.type', 'created'], '_STATUS.history.event.type.created');
assertPath(_STATUS, ['command.error.type', 'operator.resolution.error'], '_STATUS.command.error.type.operator.resolution.error');
assertPath(_STATUS, ['component.name', 'client'], '_STATUS.component.name.client');
assertPath(_MSG, ['message', 'sender', 'user', 'chat'], '_MSG.message.sender.user.chat');
assertPath(_MSG, ['message', 'sender', 'ai', 'assistant'], '_MSG.message.sender.ai.assistant');
assertPath(_COLLECTIONS, ['collections', 'users'], '_COLLECTIONS.collections.users');
assertPath(_COLLECTIONS, ['collections', 'bound.sessions'], '_COLLECTIONS.collections.bound.sessions');
assertPath(_KV, ['cache.version'], '_KV.cache.version');
assertPath(_CHANNELS, ['channels', 'Subscribe'], '_CHANNELS.channels.Subscribe');
assertPath(_CHANNELS, ['channels', 'PrefixCmd'], '_CHANNELS.channels.PrefixCmd');
assertPath(_PUBSUB, ['pubsub', 'FieldAction'], '_PUBSUB.pubsub.FieldAction');
assertPath(_PUBSUB, ['pubsub', 'FieldMessage'], '_PUBSUB.pubsub.FieldMessage');
assertPath(_HEADERS, ['x-vso.session-id'], '_HEADERS.x-vso.session-id');
assertPath(_DOCUMENT_IDS, ['document_ids', 'platform_settings'], '_DOCUMENT_IDS.document_ids.platform_settings');
