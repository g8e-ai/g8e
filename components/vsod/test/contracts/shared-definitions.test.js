// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * Shared Definitions Contract Tests
 *
 * Verifies that every VSOD constant loaded from shared JSON actually matches
 * the value in the shared JSON source of truth. If a key is missing from the
 * JSON, renamed, or drifts in value, these tests will catch it before the
 * mismatch propagates to production.
 *
 * These tests complement constants-enforcement.test.js (which scans for raw
 * string literals in service code) by verifying the loaded values themselves
 * are correct.
 */

import { describe, it, expect } from 'vitest';
import { createRequire } from 'module';
import path from 'path';
import { fileURLToPath } from 'url';

import {
    OperatorStatus,
    OperatorType,
    CloudOperatorSubtype,
} from '@vsod/constants/operator.js';
import { ApiKeyStatus } from '@vsod/constants/auth.js';
import {
    AITaskId,
} from '@vsod/constants/ai.js';

import { Collections } from '@vsod/constants/collections.js';
import { CACHE_PREFIX } from '@vsod/constants/kv_keys.js';
import { PubSubChannel } from '@vsod/constants/channels.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sharedDir = path.resolve(__dirname, '../../../../shared/constants');
const require = createRequire(import.meta.url);
const STATUS = require(path.join(sharedDir, 'status.json'));
const COLLECTIONS = require(path.join(sharedDir, 'collections.json'));
const KV_KEYS = require(path.join(sharedDir, 'kv_keys.json'));
const CHANNELS = require(path.join(sharedDir, 'channels.json'));

describe('VSOD Shared Definitions Contract', () => {

    describe('OperatorStatus matches shared/constants/status.json', () => {
        it('AVAILABLE', () => expect(OperatorStatus.AVAILABLE).toBe(STATUS['g8e.status']['available']));
        it('UNAVAILABLE', () => expect(OperatorStatus.UNAVAILABLE).toBe(STATUS['g8e.status']['unavailable']));
        it('OFFLINE', () => expect(OperatorStatus.OFFLINE).toBe(STATUS['g8e.status']['offline']));
        it('BOUND', () => expect(OperatorStatus.BOUND).toBe(STATUS['g8e.status']['bound']));
        it('STALE', () => expect(OperatorStatus.STALE).toBe(STATUS['g8e.status']['stale']));
        it('ACTIVE', () => expect(OperatorStatus.ACTIVE).toBe(STATUS['g8e.status']['active']));
        it('STOPPED', () => expect(OperatorStatus.STOPPED).toBe(STATUS['g8e.status']['stopped']));
        it('TERMINATED', () => expect(OperatorStatus.TERMINATED).toBe(STATUS['g8e.status']['terminated']));

        it('covers all keys in shared JSON', () => {
            const jsonKeys = Object.keys(STATUS['g8e.status']);
            const vsodKeys = Object.keys(OperatorStatus);
            expect(vsodKeys.length).toBe(jsonKeys.length);
        });
    });

    describe('ApiKeyStatus matches shared/constants/status.json', () => {
        it('ACTIVE', () => expect(ApiKeyStatus.ACTIVE).toBe(STATUS['api.key.status']['active']));
        it('REVOKED', () => expect(ApiKeyStatus.REVOKED).toBe(STATUS['api.key.status']['revoked']));
        it('EXPIRED', () => expect(ApiKeyStatus.EXPIRED).toBe(STATUS['api.key.status']['expired']));
        it('SUSPENDED', () => expect(ApiKeyStatus.SUSPENDED).toBe(STATUS['api.key.status']['suspended']));
    });

    describe('OperatorType matches shared/constants/status.json', () => {
        it('SYSTEM', () => expect(OperatorType.SYSTEM).toBe(STATUS['g8e.type']['system']));
        it('CLOUD', () => expect(OperatorType.CLOUD).toBe(STATUS['g8e.type']['cloud']));
    });

    describe('CloudOperatorSubtype matches shared/constants/status.json', () => {
        it('AWS', () => expect(CloudOperatorSubtype.AWS).toBe(STATUS['cloud.subtype']['aws']));
        it('GCP', () => expect(CloudOperatorSubtype.GCP).toBe(STATUS['cloud.subtype']['gcp']));
        it('AZURE', () => expect(CloudOperatorSubtype.AZURE).toBe(STATUS['cloud.subtype']['azure']));
        it('G8E_POD', () => expect(CloudOperatorSubtype.G8E_POD).toBe(STATUS['cloud.subtype']['g8e_pod']));

        it('covers all keys in shared JSON', () => {
            const jsonKeys = Object.keys(STATUS['cloud.subtype']);
            const vsodKeys = Object.keys(CloudOperatorSubtype);
            expect(vsodKeys.length).toBe(jsonKeys.length);
        });
    });

    describe('AITaskId matches shared/constants/status.json', () => {
        it('COMMAND', () => expect(AITaskId.COMMAND).toBe(STATUS['ai.task.id']['command']));
        it('DIRECT_COMMAND', () => expect(AITaskId.DIRECT_COMMAND).toBe(STATUS['ai.task.id']['direct.command']));
        it('FILE_EDIT', () => expect(AITaskId.FILE_EDIT).toBe(STATUS['ai.task.id']['file.edit']));
        it('FS_LIST', () => expect(AITaskId.FS_LIST).toBe(STATUS['ai.task.id']['fs.list']));
        it('FS_READ', () => expect(AITaskId.FS_READ).toBe(STATUS['ai.task.id']['fs.read']));
        it('PORT_CHECK', () => expect(AITaskId.PORT_CHECK).toBe(STATUS['ai.task.id']['port.check']));
        it('FETCH_LOGS', () => expect(AITaskId.FETCH_LOGS).toBe(STATUS['ai.task.id']['fetch.logs']));
        it('FETCH_HISTORY', () => expect(AITaskId.FETCH_HISTORY).toBe(STATUS['ai.task.id']['fetch.history']));
        it('FETCH_FILE_HISTORY', () => expect(AITaskId.FETCH_FILE_HISTORY).toBe(STATUS['ai.task.id']['fetch.file.history']));
        it('RESTORE_FILE', () => expect(AITaskId.RESTORE_FILE).toBe(STATUS['ai.task.id']['restore.file']));
        it('FETCH_FILE_DIFF', () => expect(AITaskId.FETCH_FILE_DIFF).toBe(STATUS['ai.task.id']['fetch.file.diff']));

        it('covers all keys in shared JSON', () => {
            const jsonKeys = Object.keys(STATUS['ai.task.id']);
            const vsodKeys = Object.keys(AITaskId);
            expect(vsodKeys.length).toBe(jsonKeys.length);
        });
    });

    describe('Collections matches shared/constants/collections.json', () => {
        it('USERS', () => expect(Collections.USERS).toBe(COLLECTIONS['collections']['users']));
        it('WEB_SESSIONS', () => expect(Collections.WEB_SESSIONS).toBe(COLLECTIONS['collections']['web_sessions']));
        it('OPERATOR_SESSIONS', () => expect(Collections.OPERATOR_SESSIONS).toBe(COLLECTIONS['collections']['operator_sessions']));
        it('LOGIN_AUDIT', () => expect(Collections.LOGIN_AUDIT).toBe(COLLECTIONS['collections']['login_audit']));
        it('AUTH_ADMIN_AUDIT', () => expect(Collections.AUTH_ADMIN_AUDIT).toBe(COLLECTIONS['collections']['auth_admin_audit']));
        it('ACCOUNT_LOCKS', () => expect(Collections.ACCOUNT_LOCKS).toBe(COLLECTIONS['collections']['account_locks']));
        it('API_KEYS', () => expect(Collections.API_KEYS).toBe(COLLECTIONS['collections']['api_keys']));
        it('ORGANIZATIONS', () => expect(Collections.ORGANIZATIONS).toBe(COLLECTIONS['collections']['organizations']));
        it('OPERATORS', () => expect(Collections.OPERATORS).toBe(COLLECTIONS['collections']['operators']));
        it('OPERATOR_USAGE', () => expect(Collections.OPERATOR_USAGE).toBe(COLLECTIONS['collections']['operator_usage']));
        it('CASES', () => expect(Collections.CASES).toBe(COLLECTIONS['collections']['cases']));
        it('INVESTIGATIONS', () => expect(Collections.INVESTIGATIONS).toBe(COLLECTIONS['collections']['investigations']));
        it('TASKS', () => expect(Collections.TASKS).toBe(COLLECTIONS['collections']['tasks']));
        it('MEMORIES', () => expect(Collections.MEMORIES).toBe(COLLECTIONS['collections']['memories']));
        it('PLATFORM_SETTINGS', () => expect(Collections.PLATFORM_SETTINGS).toBe(COLLECTIONS['collections']['platform_settings']));
        it('CONSOLE_AUDIT', () => expect(Collections.CONSOLE_AUDIT).toBe(COLLECTIONS['collections']['console_audit']));

        it('covers all keys in shared JSON', () => {
            const jsonKeys = Object.keys(COLLECTIONS['collections']);
            const vsodKeys = Object.keys(Collections);
            expect(vsodKeys.length).toBe(jsonKeys.length);
        });
    });

    describe('CACHE_PREFIX matches shared/constants/kv_keys.json', () => {
        it('matches cache.prefix', () => expect(CACHE_PREFIX).toBe(KV_KEYS['cache.prefix']));
    });

    describe('PubSubChannel matches shared/constants/channels.json', () => {
        it('CMD_PREFIX', () => expect(PubSubChannel.CMD_PREFIX).toBe(CHANNELS['pubsub']['prefixes']['cmd'] + CHANNELS['pubsub']['separator']));
        it('RESULTS_PREFIX', () => expect(PubSubChannel.RESULTS_PREFIX).toBe(CHANNELS['pubsub']['prefixes']['results'] + CHANNELS['pubsub']['separator']));
        it('HEARTBEAT_PREFIX', () => expect(PubSubChannel.HEARTBEAT_PREFIX).toBe(CHANNELS['pubsub']['prefixes']['heartbeat'] + CHANNELS['pubsub']['separator']));
        it('AUTH_PUBLISH_PREFIX', () => expect(PubSubChannel.AUTH_PUBLISH_PREFIX).toBe(CHANNELS['pubsub']['auth']['publish.prefix']));
        it('AUTH_PUBLISH_SESSION_PREFIX', () => expect(PubSubChannel.AUTH_PUBLISH_SESSION_PREFIX).toBe(CHANNELS['pubsub']['auth']['publish.session.prefix']));
        it('AUTH_RESPONSE_PREFIX', () => expect(PubSubChannel.AUTH_RESPONSE_PREFIX).toBe(CHANNELS['pubsub']['auth']['response.prefix']));
        it('AUTH_RESPONSE_SESSION_PREFIX', () => expect(PubSubChannel.AUTH_RESPONSE_SESSION_PREFIX).toBe(CHANNELS['pubsub']['auth']['response.session.prefix']));
        it('AUTH_SESSION_PREFIX', () => expect(PubSubChannel.AUTH_SESSION_PREFIX).toBe(CHANNELS['pubsub']['auth']['session.prefix']));
    });

});
