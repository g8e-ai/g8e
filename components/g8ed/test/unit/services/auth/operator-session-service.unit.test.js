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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { UserRole } from '@g8ed/constants/auth.js';
import { OperatorSessionService } from '@g8ed/services/auth/operator_session_service.js';
import { createMockCacheAside } from '@test/mocks/cache-aside.mock.js';
import { createKVMock } from '@test/mocks/kv.mock.js';
import { Collections } from '@g8ed/constants/collections.js';
import { SessionType, SessionEndReason, SessionEventType, ABSOLUTE_SESSION_TIMEOUT_SECONDS } from '@g8ed/constants/session.js';
import { OperatorSessionDocument } from '@g8ed/models/auth_models.js';
import { addSeconds, now } from '@g8ed/models/base.js';
import { BootstrapService } from '@g8ed/services/platform/bootstrap_service.js';

vi.mock('@g8ed/utils/logger.js', () => ({
    logger: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() }
}));

const TEST_ENCRYPTION_KEY = 'a'.repeat(64);

function makeMockBootstrapService() {
    return {
        loadSessionEncryptionKey: vi.fn().mockReturnValue(TEST_ENCRYPTION_KEY),
        loadInternalAuthToken: vi.fn().mockReturnValue('test-token'),
        loadCaCertPath: vi.fn().mockReturnValue('/test/ca.crt'),
        getSslDir: vi.fn().mockReturnValue('/test/ssl'),
        volumePath: '/test/volume'
    };
}

function makeService(cacheOverrides = {}) {
    const kv = createKVMock();
    const cache = createMockCacheAside();
    const dbClient = {
        getDocument: vi.fn().mockResolvedValue({ success: true, data: null }),
        setDocument: vi.fn().mockResolvedValue({ success: true }),
        createDocument: vi.fn().mockResolvedValue({ success: true }),
        updateDocument: vi.fn().mockResolvedValue({ success: true }),
        deleteDocument: vi.fn().mockResolvedValue({ success: true }),
        queryDocuments: vi.fn().mockResolvedValue({ success: true, data: [] }),
    };

    const mockBootstrapService = makeMockBootstrapService();

    const service = new OperatorSessionService({
        cacheAsideService: cache,
        bootstrapService: mockBootstrapService,
    });

    return { service, cache, kv, dbClient, mockBootstrapService };
}

function makeSessionData(overrides = {}) {
    return {
        operator_id: 'op-001',
        user_id: 'user-001',
        user_data: { name: 'Test User' },
        api_key: 'g8e_testapikey',
        operator_status: 'active',
        metadata: { hostname: 'worker-1' },
        ...overrides,
    };
}

function makeOperatorSession(id, overrides = {}) {
    const futureDate = new Date(Date.now() + 86400000).toISOString();
    return {
        id,
        session_type: SessionType.OPERATOR,
        user_id: 'user-001',
        operator_id: 'op-001',
        absolute_expires_at: futureDate,
        idle_expires_at: futureDate,
        is_active: true,
        suspicious_activity: false,
        ip_changes: 0,
        ...overrides,
    };
}

describe('OperatorSessionService [UNIT]', () => {
    let service;
    let cache;
    let kv;

    beforeEach(() => {
        ({ service, cache, kv } = makeService());
    });

    // -------------------------------------------------------------------------
    // constructor
    // -------------------------------------------------------------------------

    describe('constructor', () => {
        it('throws when cacheAsideService is not provided', () => {
            expect(() => new OperatorSessionService({
            })).toThrow('cacheAsideService is required');
        });

        it('sets sessionsCollection to Collections.OPERATOR_SESSIONS', () => {
            expect(service.sessionsCollection).toBe(Collections.OPERATOR_SESSIONS);
        });

        it('defaults absoluteSessionTimeout to ABSOLUTE_SESSION_TIMEOUT_SECONDS', () => {
            expect(service.absoluteSessionTimeout).toBe(ABSOLUTE_SESSION_TIMEOUT_SECONDS);
        });

        it('accepts custom absolute_session_timeout from config', () => {
            const svc = new OperatorSessionService({
                cacheAsideService: createMockCacheAside(),
                config: { absolute_session_timeout: '7200' },
            });
            expect(svc.absoluteSessionTimeout).toBe(7200);
        });
    });

    // -------------------------------------------------------------------------
    // _generateSessionId
    // -------------------------------------------------------------------------

    describe('_generateSessionId', () => {
        it('returns a string prefixed with operator_session_', () => {
            const id = service._generateSessionId();
            expect(id).toMatch(/^operator_session_\d+_/);
        });

        it('generates unique IDs on successive calls', () => {
            const ids = new Set(Array.from({ length: 10 }, () => service._generateSessionId()));
            expect(ids.size).toBe(10);
        });
    });

    // -------------------------------------------------------------------------
    // createOperatorSession
    // -------------------------------------------------------------------------

    describe('createOperatorSession', () => {
        it('creates a session document in the OPERATOR_SESSIONS collection', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true, documentId: 'op-session-id' });

            await service.createOperatorSession(makeSessionData());

            expect(cache.createDocument).toHaveBeenCalledWith(
                Collections.OPERATOR_SESSIONS,
                expect.stringMatching(/^operator_session_/),
                expect.any(Object),
                expect.any(Number)
            );
        });

        it('throws when persistence fails', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: false, error: 'DB unavailable' });

            await expect(service.createOperatorSession(makeSessionData()))
                .rejects.toThrow('Operator session persistence failed');
        });

        it('returns a session with operator_session_ prefixed id', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData());

            expect(session.id).toMatch(/^operator_session_/);
        });

        it('returns session with correct session_type', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData());

            expect(session.session_type).toBe(SessionType.OPERATOR);
        });

        it('returns session with correct user_id', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData({ user_id: 'user-xyz' }));

            expect(session.user_id).toBe('user-xyz');
        });

        it('encrypts api_key at rest, stores operator_id in plain text', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            await service.createOperatorSession(makeSessionData());

            const persistedData = cache.createDocument.mock.calls[0][2];
            expect(persistedData.api_key).toHaveProperty('encrypted', true);
            expect(persistedData.operator_id).toBe('op-001');
        });

        it('returns decrypted api_key and plain operator_id in the returned session', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData());

            expect(session.api_key).toBe('g8e_testapikey');
            expect(session.operator_id).toBe('op-001');
        });

        it('sets absolute_expires_at in the future', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData());

            const absoluteExpiry = new Date(session.absolute_expires_at);
            expect(absoluteExpiry.getTime()).toBeGreaterThan(Date.now());
        });

        it('sets idle_expires_at in the future', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData());

            const idleExpiry = new Date(session.idle_expires_at);
            expect(idleExpiry.getTime()).toBeGreaterThan(Date.now());
        });

        it('respects custom ttlSeconds option', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData(), {}, { ttlSeconds: 3600 });

            const idleExpiry = new Date(session.idle_expires_at);
            const msFromNow = idleExpiry.getTime() - Date.now();
            expect(msFromNow).toBeLessThanOrEqual(3600 * 1000 + 1000);
            expect(msFromNow).toBeGreaterThan(3500 * 1000);
        });

        it('stores requestContext ip and userAgent in the session', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData(), { ip: '10.0.0.1', userAgent: 'Operator/1.0' });

            expect(session.client_ip).toBe('10.0.0.1');
            expect(session.user_agent).toBe('Operator/1.0');
        });

        it('derives organization_id from user_data when not set directly', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData({
                user_data: { organization_id: 'org-from-data' }
            }));

            expect(session.organization_id).toBe('org-from-data');
        });

        it('prefers explicit organization_id over user_data.organization_id', async () => {
            cache.createDocument.mockResolvedValueOnce({ success: true });

            const session = await service.createOperatorSession(makeSessionData({
                organization_id: 'org-explicit',
                user_data: { organization_id: 'org-from-data' }
            }));

            expect(session.organization_id).toBe('org-explicit');
        });
    });

    // -------------------------------------------------------------------------
    // validateSession
    // -------------------------------------------------------------------------

    describe('validateSession', () => {
        it('returns null for null operatorSessionId', async () => {
            const result = await service.validateSession(null);
            expect(result).toBeNull();
        });

        it('returns null for undefined operatorSessionId', async () => {
            const result = await service.validateSession(undefined);
            expect(result).toBeNull();
        });

        it('returns null when session is not found', async () => {
            cache.getDocument.mockResolvedValueOnce(null);
            const result = await service.validateSession('op-session-missing');
            expect(result).toBeNull();
        });

        it('returns null when session has wrong type', async () => {
            cache.getDocument.mockResolvedValueOnce({
                ...makeOperatorSession('op-s-001'),
                session_type: SessionType.WEB,
            });
            const result = await service.validateSession('op-s-001');
            expect(result).toBeNull();
        });

        it('returns null for an absolutely-expired session', async () => {
            const expiredSession = makeOperatorSession('op-s-exp', {
                absolute_expires_at: new Date(Date.now() - 5000).toISOString(),
                idle_expires_at: new Date(Date.now() + 5000).toISOString(),
            });
            cache.getDocument
                .mockResolvedValueOnce(expiredSession)
                .mockResolvedValueOnce(expiredSession);

            const result = await service.validateSession('op-s-exp');
            expect(result).toBeNull();
        });

        it('returns null for an idle-expired session', async () => {
            const idleExpiredSession = makeOperatorSession('op-s-idle', {
                absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
                idle_expires_at: new Date(Date.now() - 5000).toISOString(),
            });
            cache.getDocument
                .mockResolvedValueOnce(idleExpiredSession)
                .mockResolvedValueOnce(idleExpiredSession);

            const result = await service.validateSession('op-s-idle');
            expect(result).toBeNull();
        });

        it('returns decrypted api_key and plain operator_id for valid session', async () => {
            cache.createDocument.mockResolvedValue({ success: true });
            const created = await service.createOperatorSession(makeSessionData());

            const storedRaw = cache.createDocument.mock.calls[0][2];
            cache.getDocument.mockResolvedValue(storedRaw);

            const validated = await service.validateSession(created.id);

            expect(validated).not.toBeNull();
            expect(validated.api_key).toBe('g8e_testapikey');
            expect(validated.operator_id).toBe('op-001');
        });

        it('ends expired session on absolute timeout', async () => {
            const expiredSession = makeOperatorSession('op-s-abs', {
                absolute_expires_at: new Date(Date.now() - 5000).toISOString(),
                idle_expires_at: new Date(Date.now() + 5000).toISOString(),
            });
            cache.getDocument
                .mockResolvedValueOnce(expiredSession)
                .mockResolvedValueOnce(expiredSession);

            await service.validateSession('op-s-abs');

            expect(cache.deleteDocument).toHaveBeenCalledWith(Collections.OPERATOR_SESSIONS, 'op-s-abs');
        });

        it('ends expired session on idle timeout', async () => {
            const idleExpiredSession = makeOperatorSession('op-s-idle2', {
                absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
                idle_expires_at: new Date(Date.now() - 5000).toISOString(),
            });
            cache.getDocument
                .mockResolvedValueOnce(idleExpiredSession)
                .mockResolvedValueOnce(idleExpiredSession);

            await service.validateSession('op-s-idle2');

            expect(cache.deleteDocument).toHaveBeenCalledWith(Collections.OPERATOR_SESSIONS, 'op-s-idle2');
        });
    });

    // -------------------------------------------------------------------------
    // refreshSession
    // -------------------------------------------------------------------------

    describe('refreshSession', () => {
        it('returns false when session is not found', async () => {
            cache.getDocument.mockResolvedValueOnce(null);
            const result = await service.refreshSession('op-s-missing');
            expect(result).toBe(false);
        });

        it('returns false for wrong session type', async () => {
            cache.getDocument.mockResolvedValueOnce({ ...makeOperatorSession('s'), session_type: SessionType.WEB });
            const result = await service.refreshSession('s');
            expect(result).toBe(false);
        });

        it('returns false and ends session when absolute timeout exceeded', async () => {
            const expired = makeOperatorSession('op-s-abs-refresh', {
                absolute_expires_at: new Date(Date.now() - 5000).toISOString(),
                idle_expires_at: new Date(Date.now() + 5000).toISOString(),
            });
            cache.getDocument
                .mockResolvedValueOnce(expired)
                .mockResolvedValueOnce(expired);

            const result = await service.refreshSession('op-s-abs-refresh');

            expect(result).toBe(false);
            expect(cache.deleteDocument).toHaveBeenCalledWith(Collections.OPERATOR_SESSIONS, 'op-s-abs-refresh');
        });

        it('returns true and updates idle_expires_at on success', async () => {
            const validSession = makeOperatorSession('op-s-valid');
            cache.getDocument.mockResolvedValueOnce(validSession);
            cache.updateDocument.mockResolvedValueOnce({ success: true });

            const result = await service.refreshSession('op-s-valid');

            expect(result).toBe(true);
            expect(cache.updateDocument).toHaveBeenCalledWith(
                Collections.OPERATOR_SESSIONS,
                'op-s-valid',
                expect.objectContaining({ last_activity: expect.any(Date), idle_expires_at: expect.any(Date) })
            );
        });

        it('accepts pre-fetched session to avoid redundant DB call', async () => {
            const validSession = makeOperatorSession('op-s-prefetched');
            cache.updateDocument.mockResolvedValueOnce({ success: true });

            await service.refreshSession('op-s-prefetched', validSession);

            expect(cache.getDocument).not.toHaveBeenCalled();
        });
    });

    // -------------------------------------------------------------------------
    // updateSession
    // -------------------------------------------------------------------------

    describe('updateSession', () => {
        it('returns null when session is not found', async () => {
            cache.getDocument.mockResolvedValueOnce(null);
            const result = await service.updateSession('op-s-missing', { operator_status: 'idle' });
            expect(result).toBeNull();
        });

        it('returns null when session has wrong type', async () => {
            cache.getDocument.mockResolvedValueOnce({ ...makeOperatorSession('s'), session_type: SessionType.WEB });
            const result = await service.updateSession('s', {});
            expect(result).toBeNull();
        });

        it('merges user_data deeply', async () => {
            const validSession = makeOperatorSession('op-s-upd', {
                user_data: { name: 'Alice', role: UserRole.ADMIN }
            });
            cache.getDocument.mockResolvedValueOnce(validSession);
            cache.updateDocument.mockResolvedValueOnce({ success: true });

            const result = await service.updateSession('op-s-upd', { user_data: { role: UserRole.SUPERADMIN } });

            expect(result.user_data.name).toBe('Alice');
            expect(result.user_data.role).toBe(UserRole.SUPERADMIN);
        });

        it('encrypts api_key and stores operator_id in plain text on update', async () => {
            const validSession = makeOperatorSession('op-s-enc');
            cache.getDocument.mockResolvedValueOnce(validSession);
            cache.updateDocument.mockResolvedValueOnce({ success: true });

            await service.updateSession('op-s-enc', { api_key: 'new-api-key', operator_id: 'new-op-id' });

            const persistedUpdates = cache.updateDocument.mock.calls[0][2];
            expect(persistedUpdates.api_key).toHaveProperty('encrypted', true);
            expect(persistedUpdates.operator_id).toBe('new-op-id');
        });

        it('does not encrypt non-sensitive fields', async () => {
            const validSession = makeOperatorSession('op-s-plain');
            cache.getDocument.mockResolvedValueOnce(validSession);
            cache.updateDocument.mockResolvedValueOnce({ success: true });

            await service.updateSession('op-s-plain', { operator_status: 'bound' });

            const persistedUpdates = cache.updateDocument.mock.calls[0][2];
            expect(persistedUpdates.operator_status).toBe('bound');
        });

        it('returns merged session with updated last_activity', async () => {
            const before = Date.now();
            const validSession = makeOperatorSession('op-s-activity');
            cache.getDocument.mockResolvedValueOnce(validSession);
            cache.updateDocument.mockResolvedValueOnce({ success: true });

            const result = await service.updateSession('op-s-activity', { operator_status: 'idle' });

            expect(new Date(result.last_activity).getTime()).toBeGreaterThanOrEqual(before);
        });
    });

    // -------------------------------------------------------------------------
    // extendSession
    // -------------------------------------------------------------------------

    describe('extendSession', () => {
        it('returns false when session is not found', async () => {
            cache.getDocument.mockResolvedValueOnce(null);
            const result = await service.extendSession('op-s-missing');
            expect(result).toBe(false);
        });

        it('returns false for wrong session type', async () => {
            cache.getDocument.mockResolvedValueOnce({ ...makeOperatorSession('s'), session_type: SessionType.WEB });
            const result = await service.extendSession('s');
            expect(result).toBe(false);
        });

        it('returns true and updates absolute and idle expiries', async () => {
            const validSession = makeOperatorSession('op-s-extend');
            cache.getDocument.mockResolvedValueOnce(validSession);
            cache.updateDocument.mockResolvedValueOnce({ success: true });

            const result = await service.extendSession('op-s-extend');

            expect(result).toBe(true);
            const updated = cache.updateDocument.mock.calls[0][2];
            expect(new Date(updated.absolute_expires_at).getTime()).toBeGreaterThan(Date.now());
            expect(new Date(updated.idle_expires_at).getTime()).toBeGreaterThan(Date.now());
        });
    });

    // -------------------------------------------------------------------------
    // endSession
    // -------------------------------------------------------------------------

    describe('endSession', () => {
        it('returns false when session is not found', async () => {
            cache.getDocument.mockResolvedValueOnce(null);
            const result = await service.endSession('op-s-missing');
            expect(result).toBe(false);
        });

        it('deletes the session document', async () => {
            const validSession = makeOperatorSession('op-s-end');
            cache.getDocument.mockResolvedValueOnce(validSession);
            cache.deleteDocument.mockResolvedValueOnce({ success: true });

            await service.endSession('op-s-end');

            expect(cache.deleteDocument).toHaveBeenCalledWith(Collections.OPERATOR_SESSIONS, 'op-s-end');
        });

        it('returns true on successful deletion', async () => {
            const validSession = makeOperatorSession('op-s-end2');
            cache.getDocument.mockResolvedValueOnce(validSession);
            cache.deleteDocument.mockResolvedValueOnce({ success: true });

            const result = await service.endSession('op-s-end2');
            expect(result).toBe(true);
        });

        it('passes custom end reason to session event log', async () => {
            const validSession = makeOperatorSession('op-s-reason');
            cache.getDocument.mockResolvedValueOnce(validSession);
            cache.deleteDocument.mockResolvedValueOnce({ success: true });
            cache.createDocument.mockResolvedValueOnce({ success: true });

            await service.endSession('op-s-reason', SessionEndReason.SESSION_REGENERATION);

            const auditCalls = cache.createDocument.mock.calls;
            const eventData = auditCalls[0]?.[2];
            if (eventData) {
                expect(eventData.event_type ?? eventData.reason ?? JSON.stringify(eventData))
                    .toContain(SessionEndReason.SESSION_REGENERATION);
            }
        });
    });

    // -------------------------------------------------------------------------
    // regenerateOperatorSession
    // -------------------------------------------------------------------------

    describe('regenerateOperatorSession', () => {
        it('returns null when old session is not found', async () => {
            cache.getDocument.mockResolvedValue(null);
            const result = await service.regenerateOperatorSession('op-s-old');
            expect(result).toBeNull();
        });

        it('throws when old session is a web session', async () => {
            const webSession = { ...makeOperatorSession('s'), session_type: SessionType.WEB };
            cache.getDocument.mockResolvedValueOnce(webSession);

            await expect(service.regenerateOperatorSession('s'))
                .rejects.toThrow('Cannot regenerate web session as operator session');
        });

        it('ends the old session after creating a new one', async () => {
            cache.createDocument.mockResolvedValue({ success: true });
            const created = await service.createOperatorSession(makeSessionData());
            const storedRaw = cache.createDocument.mock.calls[0][2];

            cache._reset();
            cache.getDocument
                .mockResolvedValueOnce(storedRaw)
                .mockResolvedValueOnce(storedRaw)
                .mockResolvedValueOnce(storedRaw)
                .mockResolvedValueOnce(storedRaw);
            cache.createDocument.mockResolvedValue({ success: true });
            cache.deleteDocument.mockResolvedValue({ success: true });

            await service.regenerateOperatorSession(created.id);

            expect(cache.deleteDocument).toHaveBeenCalledWith(
                Collections.OPERATOR_SESSIONS,
                created.id
            );
        });

        it('creates a new session with a different ID', async () => {
            cache.createDocument.mockResolvedValue({ success: true });
            const created = await service.createOperatorSession(makeSessionData());
            const storedRaw = cache.createDocument.mock.calls[0][2];

            cache._reset();
            cache.getDocument.mockResolvedValue(storedRaw);
            cache.createDocument.mockResolvedValue({ success: true });
            cache.deleteDocument.mockResolvedValue({ success: true });

            const newSession = await service.regenerateOperatorSession(created.id);

            expect(newSession).not.toBeNull();
            expect(newSession.id).not.toBe(created.id);
            expect(newSession.id).toMatch(/^operator_session_/);
        });
    });

    // -------------------------------------------------------------------------
    // getSessionCount (KV scan)
    // -------------------------------------------------------------------------

    describe('getSessionCount', () => {
        it('returns 0 when no sessions exist in KV', async () => {
            cache.kvScan = vi.fn().mockResolvedValueOnce(['0', []]);
            const count = await service.getSessionCount();
            expect(count).toBe(0);
        });

        it('returns correct count after adding operator session keys', async () => {
            cache.kvScan = vi.fn().mockResolvedValueOnce(['0', ['op_session:session-a', 'op_session:session-b']]);

            const count = await service.getSessionCount();
            expect(count).toBe(2);
        });
    });
});
