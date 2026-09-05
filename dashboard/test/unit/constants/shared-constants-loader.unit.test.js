// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Tier 1 unit tests for the shared constants loader.
 *
 * Imports the real `constants/shared.js` module without mocks and verifies
 * that every exported value is backed by the protocol SSOT
 * (`protocol/constants/*.json`). No synthesized fallback values that
 * fabricate canonical wire values absent from the SSOT are permitted.
 *
 * This test exercises the real import path independently of client mocks.
 */

import { describe, it, expect } from 'vitest';
import { createRequire } from 'module';
import { fileURLToPath } from 'url';
import path from 'path';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);
const sharedDir = path.resolve(__dirname, '../../../../protocol/constants');

import {
    _EVENTS,
    _STATUS,
    _MSG,
    _COLLECTIONS,
    _KV,
    _CHANNELS,
    _PUBSUB,
    _INTENTS,
    _PROMPTS,
    _TIMESTAMP,
    _HEADERS,
    _DOCUMENT_IDS,
} from '../../../constants/shared.js';

// ─── Helper: unwrap { value: ... } leaves ─────────────────────────────

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

// ─── Protocol SSOT snapshots ──────────────────────────────────────────

const eventsRaw = require(path.join(sharedDir, 'events.json'));
const statusRaw = require(path.join(sharedDir, 'status.json'));
const sendersRaw = require(path.join(sharedDir, 'senders.json'));
const collectionsRaw = require(path.join(sharedDir, 'collections.json'));
const kvKeysRaw = require(path.join(sharedDir, 'kv_keys.json'));
const channelsRaw = require(path.join(sharedDir, 'channels.json'));
const pubsubRaw = require(path.join(sharedDir, 'pubsub.json'));
const intentsRaw = require(path.join(sharedDir, 'intents.json'));
const promptsRaw = require(path.join(sharedDir, 'prompts.json'));
const timestampRaw = require(path.join(sharedDir, 'timestamp.json'));
const headersRaw = require(path.join(sharedDir, 'headers.json'));
const documentIdsRaw = require(path.join(sharedDir, 'document_ids.json'));

// ─── Tests ────────────────────────────────────────────────────────────

describe('shared.js unmocked import', () => {
    it('imports without throwing', () => {
        expect(_EVENTS).toBeDefined();
        expect(_STATUS).toBeDefined();
        expect(_MSG).toBeDefined();
        expect(_COLLECTIONS).toBeDefined();
        expect(_KV).toBeDefined();
        expect(_CHANNELS).toBeDefined();
        expect(_PUBSUB).toBeDefined();
        expect(_INTENTS).toBeDefined();
        expect(_PROMPTS).toBeDefined();
        expect(_TIMESTAMP).toBeDefined();
        expect(_HEADERS).toBeDefined();
        expect(_DOCUMENT_IDS).toBeDefined();
    });

    it('exports _EVENTS values that match protocol events.json', () => {
        // transformEvents strips 'g8e.v1.' prefix and nests by dot-separated parts.
        // Representative: AppCaseCreated → g8e.v1.app.case.created → _EVENTS.app.case.created
        expect(_EVENTS.app.case.created).toBe('g8e.v1.app.case.created');
        expect(_EVENTS.ai.llm.chat.iteration.started).toBe('g8e.v1.ai.llm.chat.iteration.started');

        // Verify against raw protocol
        expect(eventsRaw.events.AppCaseCreated.value).toBe('g8e.v1.app.case.created');
        expect(eventsRaw.events.AiLLMChatIterationStarted.value).toBe('g8e.v1.ai.llm.chat.iteration.started');
    });

    it('exports _STATUS values that match protocol status.json', () => {
        // operator_status → operator.status (snakeToDot on group key)
        expect(_STATUS['operator.status'].available).toBe('available');
        expect(statusRaw.status.operator_status.available.value).toBe('available');

        // user_role → user.role
        expect(_STATUS['user.role'].user).toBe('user');
        expect(_STATUS['user.role'].admin).toBe('admin');
        expect(_STATUS['user.role'].operator).toBe('operator');
        expect(statusRaw.status.user_role.user.value).toBe('user');

        // auth_provider → auth.provider
        expect(_STATUS['auth.provider'].local).toBe('local');
        expect(statusRaw.status.auth_provider.local.value).toBe('local');

        // auth_method → auth.method (inner keys get snakeToDot)
        expect(_STATUS['auth.method']['kv.pubsub']).toBe('kv_pubsub');
        expect(statusRaw.status.auth_method.kv_pubsub.value).toBe('kv_pubsub');

        // session_type → session.type
        expect(_STATUS['session.type'].web).toBe('web');
        expect(statusRaw.status.session_type.web.value).toBe('web');

        // session_key_prefix → session.key.prefix (inner keys get snakeToDot, then .session stripped)
        expect(_STATUS['session.key.prefix'].web).toBe('web_session');
        expect(statusRaw.status.session_key_prefix.web_session.value).toBe('web_session');

        // session_event_type → session.event.type (g8e.bound renamed to operator.bound)
        expect(_STATUS['session.event.type']['operator.bound']).toBe('g8e.bound');
        expect(statusRaw.status.session_event_type['g8e.bound'].value).toBe('g8e.bound');

        // operator_history_event_type → history.event.type (renamed)
        expect(_STATUS['history.event.type'].created).toBe('created');
        expect(statusRaw.status.operator_history_event_type.created.value).toBe('created');

        // command_error_type → command.error.type (g8e.resolution.error renamed to operator.resolution.error)
        expect(_STATUS['command.error.type']['operator.resolution.error']).toBe('g8e.resolution.error');
        expect(statusRaw.status.command_error_type['g8e.resolution.error'].value).toBe('g8e.resolution.error');

        // login_audit_event_type → login.audit.event.type
        expect(_STATUS['login.audit.event.type']['login.success']).toBe('login_success');
        expect(statusRaw.status.login_audit_event_type.login_success.value).toBe('login_success');

        // auth_audit_event_type → auth.audit.event.type
        expect(_STATUS['auth.audit.event.type']['auth.success']).toBe('auth_success');
        expect(statusRaw.status.auth_audit_event_type.auth_success.value).toBe('auth_success');

        // auth_audit_result → auth.audit.result
        expect(_STATUS['auth.audit.result'].success).toBe('success');
        expect(statusRaw.status.auth_audit_result.success.value).toBe('success');

        // download_audit_event_type → download.audit.event.type
        expect(_STATUS['download.audit.event.type']['download.token.success']).toBe('download_token_success');
        expect(statusRaw.status.download_audit_event_type.download_token_success.value).toBe('download_token_success');

        // session_end_reason → session.end.reason
        expect(_STATUS['session.end.reason']['user.logout']).toBe('user_logout');
        expect(statusRaw.status.session_end_reason.user_logout.value).toBe('user_logout');

        // session_suspicious_reason → session.suspicious.reason
        expect(_STATUS['session.suspicious.reason']['excessive.ip.changes']).toBe('excessive_ip_changes');
        expect(statusRaw.status.session_suspicious_reason.excessive_ip_changes.value).toBe('excessive_ip_changes');
    });

    it('does not fabricate absent _STATUS groups', () => {
        // These keys are absent from protocol status.json and must not be synthesized.
        expect(_STATUS['auth.mode']).toBeUndefined();
        expect(_STATUS['auth.admin.audit.event.type']).toBeUndefined();
        expect(_STATUS['api.key.status']).toBeUndefined();
        expect(_STATUS['device.link.status']).toBeUndefined();
        expect(_STATUS['device.link.success']).toBeUndefined();
    });

    it('does not fabricate absent _STATUS inner keys', () => {
        // user_role has admin, operator, owner, user — no superadmin.
        expect(_STATUS['user.role'].superadmin).toBeUndefined();
        // component_name has client, g8eo, g8eo-gateway — no vse, vsa, g8ed.
        expect(_STATUS['component.name'].vse).toBeUndefined();
        expect(_STATUS['component.name'].vsa).toBeUndefined();
        expect(_STATUS['component.name'].g8ed).toBeUndefined();
    });

    it('exports _MSG values that match protocol senders.json', () => {
        // transformSenders strips 'g8e.v1.source.' prefix and nests by dot-separated parts.
        expect(_MSG.message.sender.user.chat).toBe('g8e.v1.source.user.chat');
        expect(_MSG.message.sender.ai.assistant).toBe('g8e.v1.source.ai.assistant');
        expect(sendersRaw.senders.UserChat.value).toBe('g8e.v1.source.user.chat');
        expect(sendersRaw.senders.AiAssistant.value).toBe('g8e.v1.source.ai.assistant');
    });

    it('exports _COLLECTIONS values that match protocol collections.json', () => {
        // transformCollections applies snakeToDot to collection keys.
        expect(_COLLECTIONS.collections.users).toBe('users');
        expect(_COLLECTIONS.collections['bound.sessions']).toBe('bound_sessions');
        expect(collectionsRaw.collections.users.value).toBe('users');
        expect(collectionsRaw.collections.bound_sessions.value).toBe('bound_sessions');
    });

    it('does not fabricate absent _COLLECTIONS keys', () => {
        // These collections are absent from protocol collections.json.
        expect(_COLLECTIONS.collections['session.audit.logs']).toBeUndefined();
        expect(_COLLECTIONS.collections['api.keys']).toBeUndefined();
    });

    it('exports _KV values that match protocol kv_keys.json', () => {
        // transformKvKeys renames CachePrefix → cache.version.
        expect(_KV['cache.version']).toBe('g8e');
        expect(kvKeysRaw.kv_keys.CachePrefix.value).toBe('g8e');
    });

    it('exports _CHANNELS values that match protocol channels.json', () => {
        expect(_CHANNELS.channels.Subscribe).toBe('subscribe');
        expect(_CHANNELS.channels.PrefixCmd).toBe('cmd');
        expect(_CHANNELS).toEqual(unwrapValue(channelsRaw));
        expect(_CHANNELS.pubsub).toBeUndefined();
    });

    it('exports _PUBSUB values that match protocol pubsub.json', () => {
        expect(_PUBSUB.pubsub.FieldAction).toBe('action');
        expect(_PUBSUB.pubsub.FieldMessage).toBe('message');
        expect(_PUBSUB).toEqual(unwrapValue(pubsubRaw));
        expect(_PUBSUB.wire).toBeUndefined();
    });

    it('exports _INTENTS values that match protocol intents.json', () => {
        const unwrapped = unwrapValue(intentsRaw);
        expect(_INTENTS.intents).toBeDefined();
        expect(Object.keys(_INTENTS.intents).length).toBe(Object.keys(unwrapped.intents).length);
    });

    it('exports _PROMPTS values that match protocol prompts.json', () => {
        const unwrapped = unwrapValue(promptsRaw);
        expect(_PROMPTS).toBeDefined();
        // Verify structure matches
        expect(Object.keys(_PROMPTS)).toEqual(Object.keys(unwrapped));
    });

    it('exports _TIMESTAMP values that match protocol timestamp.json', () => {
        const unwrapped = unwrapValue(timestampRaw);
        expect(_TIMESTAMP).toBeDefined();
        expect(Object.keys(_TIMESTAMP)).toEqual(Object.keys(unwrapped));
    });

    it('exports _HEADERS values that match protocol headers.json', () => {
        // transformHeaders maps PascalCase protocol keys to dot-notation.
        // Representative: WebSessionID → x-vso.session-id
        expect(_HEADERS['x-vso.session-id']).toBe('X-G8E-Web-Session-ID');
        expect(headersRaw.headers.WebSessionID.value).toBe('X-G8E-Web-Session-ID');

        expect(_HEADERS['x-vso.user-id']).toBe('X-G8E-User-ID');
        expect(headersRaw.headers.UserID.value).toBe('X-G8E-User-ID');

        expect(_HEADERS['http.content-type']).toBe('Content-Type');
        expect(headersRaw.headers.ContentType.value).toBe('Content-Type');
    });

    it('does not fabricate absent _HEADERS keys', () => {
        // These headers are absent from protocol headers.json and must not be synthesized.
        expect(_HEADERS['http.x-internal-auth']).toBeUndefined();
        expect(_HEADERS['http.api-key']).toBeUndefined();
        expect(_HEADERS['http.x-session-id']).toBeUndefined();
        expect(_HEADERS['x-vso.new-case']).toBeUndefined();
        expect(_HEADERS['x-vso.service']).toBeUndefined();
        expect(_HEADERS['x-vso.client']).toBeUndefined();
        expect(_HEADERS['x-vso.operator-status']).toBeUndefined();
    });

    it('exports _DOCUMENT_IDS values that match protocol document_ids.json', () => {
        const unwrapped = unwrapValue(documentIdsRaw);
        expect(_DOCUMENT_IDS.document_ids.platform_settings).toBe('platform_settings');
        expect(unwrapped.document_ids.platform_settings).toBe('platform_settings');
    });
});
