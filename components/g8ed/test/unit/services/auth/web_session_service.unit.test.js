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
import { WebSessionService } from '@g8ed/services/auth/web_session_service.js';
import { createMockCacheAside } from '@test/mocks/cache-aside.mock.js';
import { Collections } from '@g8ed/constants/collections.js';
import { SessionType, SessionEndReason, SessionEventType, ABSOLUTE_SESSION_TIMEOUT_SECONDS, SessionSuspiciousReason } from '@g8ed/constants/session.js';
import { WebSessionDocument } from '@g8ed/models/auth_models.js';
import { addSeconds, now } from '@g8ed/models/base.js';
import { AuthProvider } from '@g8ed/constants/auth.js';
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

function makeService() {
    const cache = createMockCacheAside();
    const mockBootstrapService = makeMockBootstrapService();
    const service = new WebSessionService({
        cacheAsideService: cache,
        bootstrapService: mockBootstrapService,
    });
    return { service, cache, mockBootstrapService };
}

function makeSessionData(overrides = {}) {
    return {
        user_id: 'user-001',
        user_data: { email: 'test@example.com', name: 'Test User' },
        api_key: 'g8e_testapikey',
        ...overrides,
    };
}

function makeWebSession(id, overrides = {}) {
    const ts = now();
    const futureDate = addSeconds(ts, 3600);
    return {
        id,
        session_type: SessionType.WEB,
        user_id: 'user-001',
        user_data: { email: 'test@example.com', name: 'Test User' },
        absolute_expires_at: futureDate,
        idle_expires_at: futureDate,
        created_at: ts,
        is_active: true,
        suspicious_activity: false,
        ip_changes: 0,
        client_ip: '127.0.0.1',
        ...overrides,
    };
}

describe('WebSessionService [UNIT]', () => {
    let service;
    let cache;

    beforeEach(() => {
        ({ service, cache } = makeService());
    });

    describe('constructor', () => {
        it('throws when cacheAsideService is not provided', () => {
            expect(() => new WebSessionService({
            })).toThrow('cacheAsideService is required');
        });

        it('sets sessionsCollection to Collections.WEB_SESSIONS', () => {
            expect(service.sessionsCollection).toBe(Collections.WEB_SESSIONS);
        });

        it('defaults absoluteSessionTimeout to ABSOLUTE_SESSION_TIMEOUT_SECONDS', () => {
            expect(service.absoluteSessionTimeout).toBe(ABSOLUTE_SESSION_TIMEOUT_SECONDS);
        });
    });

    describe('createWebSession', () => {
        it('should successfully create a web session', async () => {
            const sessionData = makeSessionData();
            cache.createDocument.mockResolvedValue({ success: true });
            
            const session = await service.createWebSession(sessionData, { ip: '192.168.1.1' });

            expect(session.id).toMatch(/^web_session_/);
            expect(session.user_id).toBe(sessionData.user_id);
            expect(session.client_ip).toBe('192.168.1.1');
            expect(cache.createDocument).toHaveBeenCalledWith(
                Collections.WEB_SESSIONS,
                session.id,
                expect.any(Object),
                expect.any(Number)
            );
            expect(cache.kvZadd).toHaveBeenCalled();
        });

        it('should throw error if user_id is missing', async () => {
            await expect(service.createWebSession({})).rejects.toThrow('createWebSession requires user_id');
        });

        it('should encrypt sensitive fields if key is present', async () => {
            const sessionData = makeSessionData();
            cache.createDocument.mockResolvedValue({ success: true });
            
            await service.createWebSession(sessionData);

            const persistedData = cache.createDocument.mock.calls[0][2];
            expect(persistedData.api_key).toHaveProperty('encrypted', true);
        });
    });

    describe('validateSession', () => {
        it('should return null if sessionId is missing', async () => {
            const result = await service.validateSession(null);
            expect(result).toBeNull();
        });

        it('should return null if session is not found', async () => {
            cache.getDocument.mockResolvedValue(null);
            const result = await service.validateSession('missing');
            expect(result).toBeNull();
        });

        it('should return null and end session if integrity check fails', async () => {
            const session = makeWebSession('sess-1');
            delete session.user_data; // Force integrity failure
            cache.getDocument.mockResolvedValue(session);
            
            const result = await service.validateSession('sess-1');
            expect(result).toBeNull();
            expect(cache.deleteDocument).toHaveBeenCalled();
        });

        it('should return null and end session if absolute timeout exceeded', async () => {
            const session = makeWebSession('sess-1', {
                absolute_expires_at: addSeconds(now(), -10)
            });
            cache.getDocument.mockResolvedValue(session);
            
            const result = await service.validateSession('sess-1');
            expect(result).toBeNull();
            expect(cache.deleteDocument).toHaveBeenCalled();
        });

        it('should handle IP changes and detect suspicious activity', async () => {
            const session = makeWebSession('sess-1', { client_ip: '1.1.1.1', ip_changes: 3 });
            cache.getDocument.mockResolvedValue(session);
            
            const result = await service.validateSession('sess-1', { ip: '2.2.2.2' });
            
            expect(result.ip_changes).toBe(4);
            expect(result.suspicious_activity).toBe(true);
            expect(cache.updateDocument).toHaveBeenCalledWith(
                Collections.WEB_SESSIONS,
                'sess-1',
                expect.objectContaining({ suspicious_activity: true })
            );
        });

        it('should return validated and decrypted session', async () => {
            const sessionData = makeSessionData();
            cache.createDocument.mockResolvedValue({ success: true });
            const created = await service.createWebSession(sessionData);
            
            const persistedData = cache.createDocument.mock.calls[0][2];
            cache.getDocument.mockResolvedValue(persistedData);
            
            const result = await service.validateSession(created.id);
            expect(result).not.toBeNull();
            expect(result.api_key).toBe(sessionData.api_key);
        });
    });

    describe('refreshSession', () => {
        it('should update session expiration', async () => {
            const session = makeWebSession('sess-1');
            cache.getDocument.mockResolvedValue(session);
            cache.updateDocument.mockResolvedValue({ success: true });

            const result = await service.refreshSession('sess-1');
            expect(result).toBe(true);
            expect(cache.updateDocument).toHaveBeenCalledWith(
                Collections.WEB_SESSIONS,
                'sess-1',
                expect.objectContaining({
                    idle_expires_at: expect.any(Date),
                    last_activity: expect.any(Date)
                })
            );
        });
    });

    describe('updateSession', () => {
        it('should merge user_data deeply', async () => {
            const session = makeWebSession('sess-1', {
                user_data: { email: 'a@b.com', name: 'Old' }
            });
            cache.getDocument.mockResolvedValue(session);
            cache.updateDocument.mockResolvedValue({ success: true });

            const result = await service.updateSession('sess-1', {
                user_data: { name: 'New' }
            });

            expect(result.user_data.email).toBe('a@b.com');
            expect(result.user_data.name).toBe('New');
        });
    });

    describe('endSession', () => {
        it('should delete session and remove from user tracking', async () => {
            const session = makeWebSession('sess-1', { user_id: 'user-1' });
            cache.getDocument.mockResolvedValue(session);
            
            const result = await service.endSession('sess-1');
            
            expect(result).toBe(true);
            expect(cache.deleteDocument).toHaveBeenCalledWith(Collections.WEB_SESSIONS, 'sess-1');
            expect(cache.kvZrem).toHaveBeenCalled();
        });
    });

    describe('regenerateWebSession', () => {
        it('should create new session and end old one', async () => {
            const oldSession = makeWebSession('old-sess');
            cache.getDocument.mockResolvedValue(oldSession);
            cache.createDocument.mockResolvedValue({ success: true });
            
            const result = await service.regenerateWebSession('old-sess');
            
            expect(result.id).not.toBe('old-sess');
            expect(cache.deleteDocument).toHaveBeenCalledWith(Collections.WEB_SESSIONS, 'old-sess');
        });
    });

    describe('getUserActiveSessions', () => {
        it('should return list of active session IDs', async () => {
            cache.kvZrange.mockResolvedValue(['s1', 's2']);
            cache.getDocument
                .mockResolvedValueOnce(makeWebSession('s1'))
                .mockResolvedValueOnce(makeWebSession('s2', { is_active: false }));
            
            const result = await service.getUserActiveSessions('u1');
            
            expect(result).toEqual(['s1']);
            expect(cache.kvZrem).toHaveBeenCalledWith(expect.any(String), 's2');
        });
    });

    describe('getSessionCount', () => {
        it('should scan KV for session keys', async () => {
            cache.kvScan.mockResolvedValueOnce(['0', ['key1', 'key2']]);
            const count = await service.getSessionCount();
            expect(count).toBe(2);
        });
    });

    describe('validateAndUpdateActivity', () => {
        it('should validate and update last_activity', async () => {
            const session = makeWebSession('sess-1');
            cache.getDocument.mockResolvedValue(session);
            cache.updateDocument.mockResolvedValue({ success: true });

            const result = await service.validateAndUpdateActivity('sess-1');
            expect(result).not.toBeNull();
            expect(cache.updateDocument).toHaveBeenCalledWith(
                Collections.WEB_SESSIONS,
                'sess-1',
                expect.objectContaining({ last_activity: expect.any(Date) })
            );
        });

        it('should return null if validation fails', async () => {
            cache.getDocument.mockResolvedValue(null);
            const result = await service.validateAndUpdateActivity('invalid');
            expect(result).toBeNull();
        });
    });

    describe('bindOperatorToWebSession', () => {
        it('should bind operator if not already present', async () => {
            const session = makeWebSession('sess-1');
            cache.getDocument.mockResolvedValue(session);
            cache.updateDocument.mockResolvedValue({ success: true });

            const result = await service.bindOperatorToWebSession('sess-1', 'op-1');
            expect(result).toBe(true);
            expect(cache.updateDocument).toHaveBeenCalledWith(
                Collections.WEB_SESSIONS,
                'sess-1',
                expect.objectContaining({ operator_ids: ['op-1'] })
            );
        });

        it('should return true and not update if operator already bound', async () => {
            const session = makeWebSession('sess-1', { operator_ids: ['op-1'] });
            cache.getDocument.mockResolvedValue(session);

            const result = await service.bindOperatorToWebSession('sess-1', 'op-1');
            expect(result).toBe(true);
            expect(cache.updateDocument).not.toHaveBeenCalled();
        });
    });

    describe('unbindOperatorFromWebSession', () => {
        it('should remove operator if present', async () => {
            const session = makeWebSession('sess-1', { operator_ids: ['op-1', 'op-2'] });
            cache.getDocument.mockResolvedValue(session);
            cache.updateDocument.mockResolvedValue({ success: true });

            const result = await service.unbindOperatorFromWebSession('sess-1', 'op-1');
            expect(result).toBe(true);
            expect(cache.updateDocument).toHaveBeenCalledWith(
                Collections.WEB_SESSIONS,
                'sess-1',
                expect.objectContaining({ operator_ids: ['op-2'] })
            );
        });
    });

    describe('extendSession', () => {
        it('should extend absolute and idle timeouts', async () => {
            const session = makeWebSession('sess-1');
            cache.getDocument.mockResolvedValue(session);
            cache.updateDocument.mockResolvedValue({ success: true });

            const result = await service.extendSession('sess-1');
            expect(result).toBe(true);
            expect(cache.updateDocument).toHaveBeenCalledWith(
                Collections.WEB_SESSIONS,
                'sess-1',
                expect.objectContaining({
                    absolute_expires_at: expect.any(Date),
                    idle_expires_at: expect.any(Date)
                })
            );
        });
    });

    describe('updateAllUserSessions', () => {
        it('should update multiple sessions for a user', async () => {
            cache.kvZrange.mockResolvedValue(['s1', 's2']);
            cache.getDocument.mockImplementation(async (coll, id) => makeWebSession(id));
            cache.updateDocument.mockResolvedValue({ success: true });

            const count = await service.updateAllUserSessions('u1', { organization_id: 'org-new' });
            expect(count).toBe(2);
            expect(cache.updateDocument).toHaveBeenCalledTimes(2);
        });
    });

    describe('invalidateAllUserSessions', () => {
        it('should end all sessions for a user', async () => {
            cache.kvZrange.mockResolvedValue(['s1', 's2']);
            cache.getDocument.mockImplementation(async (coll, id) => makeWebSession(id, { user_id: 'u1' }));
            cache.deleteDocument.mockResolvedValue({ success: true });

            const count = await service.invalidateAllUserSessions('u1');
            expect(count).toBe(2);
            expect(cache.deleteDocument).toHaveBeenCalledTimes(2);
            expect(cache.kvDel).toHaveBeenCalled();
        });
    });

    describe('getUserActiveSession', () => {
        it('should return the most recent active session ID', async () => {
            cache.kvZrevrange.mockResolvedValue(['s2', 's1']);
            cache.getDocument.mockResolvedValueOnce(makeWebSession('s2', { is_active: true }));

            const result = await service.getUserActiveSession('u1');
            expect(result).toBe('s2');
        });
    });

    describe('_safeGetUserWebSessionIds', () => {
        it('should handle WRONGTYPE errors by deleting the key', async () => {
            cache.kvZrange.mockRejectedValue(new Error('WRONGTYPE operation against a key holding the wrong kind of value'));
            cache.kvDel.mockResolvedValue(1);

            const result = await service._safeGetUserWebSessionIds('u1');
            expect(result).toEqual([]);
            expect(cache.kvDel).toHaveBeenCalled();
        });
    });
});
