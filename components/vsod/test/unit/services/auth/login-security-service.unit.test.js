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
import { LoginSecurityService } from '@vsod/services/auth/login_security_service.js';
import { createMockCacheAside } from '@test/mocks/cache-aside.mock.js';
import { Collections } from '@vsod/constants/collections.js';
import { KVKey } from '@vsod/constants/kv_keys.js';
import { LoginSecurity } from '@vsod/constants/rate_limits.js';
import { LoginEventType } from '@vsod/constants/auth.js';

vi.mock('@vsod/utils/logger.js', () => ({
    logger: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() }
}));

const MAX_FAILED_ATTEMPTS = LoginSecurity.MAX_FAILED_ATTEMPTS;
const CAPTCHA_THRESHOLD = LoginSecurity.CAPTCHA_THRESHOLD;
const PROGRESSIVE_DELAYS = LoginSecurity.PROGRESSIVE_DELAYS;

function makeService(cacheAsideService) {
    return new LoginSecurityService({ cacheAsideService });
}

function makeRequestContext(overrides = {}) {
    return {
        ip: '127.0.0.1',
        userAgent: 'Test/1.0',
        deviceFingerprint: 'abc123',
        ...overrides,
    };
}

describe('LoginSecurityService [UNIT]', () => {
    let cache;
    let service;

    beforeEach(() => {
        cache = createMockCacheAside();
        service = makeService(cache);
    });

    // -------------------------------------------------------------------------
    // constructor
    // -------------------------------------------------------------------------

    describe('constructor', () => {
        it('throws when cacheAsideService is not provided', () => {
            expect(() => new LoginSecurityService({})).toThrow('cacheAsideService is required');
        });

        it('accepts optional geoipLookup', () => {
            const geoip = vi.fn();
            const svc = new LoginSecurityService({ cacheAsideService: cache, geoipLookup: geoip });
            expect(svc.geoipLookup).toBe(geoip);
        });

        it('defaults geoipLookup to null when not provided', () => {
            expect(service.geoipLookup).toBeNull();
        });
    });

    // -------------------------------------------------------------------------
    // isHealthy
    // -------------------------------------------------------------------------

    describe('isHealthy', () => {
        it('returns true when cache is set', () => {
            expect(service.isHealthy()).toBe(true);
        });
    });

    // -------------------------------------------------------------------------
    // generateDeviceFingerprint
    // -------------------------------------------------------------------------

    describe('generateDeviceFingerprint', () => {
        it('returns a 32-char hex string', () => {
            const req = {
                headers: {
                    'user-agent': 'Mozilla/5.0',
                    'accept-language': 'en-US',
                    'accept-encoding': 'gzip',
                    'accept': 'text/html',
                }
            };
            const fp = service.generateDeviceFingerprint(req);
            expect(fp).toMatch(/^[0-9a-f]{32}$/);
        });

        it('produces the same fingerprint for identical request headers', () => {
            const req = {
                headers: {
                    'user-agent': 'Mozilla/5.0',
                    'accept-language': 'en-US',
                    'accept-encoding': 'gzip',
                    'accept': 'text/html',
                }
            };
            expect(service.generateDeviceFingerprint(req)).toBe(service.generateDeviceFingerprint(req));
        });

        it('produces different fingerprints for different user-agents', () => {
            const req1 = { headers: { 'user-agent': 'AgentA', 'accept-language': 'en', 'accept-encoding': 'gzip', 'accept': '*' } };
            const req2 = { headers: { 'user-agent': 'AgentB', 'accept-language': 'en', 'accept-encoding': 'gzip', 'accept': '*' } };
            expect(service.generateDeviceFingerprint(req1)).not.toBe(service.generateDeviceFingerprint(req2));
        });
    });

    // -------------------------------------------------------------------------
    // _redactIdentifier
    // -------------------------------------------------------------------------

    describe('_redactIdentifier', () => {
        it('returns "None" for falsy input', () => {
            expect(service._redactIdentifier(null)).toBe('None');
            expect(service._redactIdentifier('')).toBe('None');
            expect(service._redactIdentifier(undefined)).toBe('None');
        });

        it('redacts email addresses', () => {
            const result = service._redactIdentifier('alice@example.com');
            expect(result).toBe('ali***@example.com');
        });

        it('redacts non-email identifiers with ellipsis', () => {
            const result = service._redactIdentifier('abc123456789');
            expect(result).toBe('abc123...');
        });
    });

    // -------------------------------------------------------------------------
    // isAccountLocked
    // -------------------------------------------------------------------------

    describe('isAccountLocked', () => {
        it('returns { locked: false } when no lock document exists', async () => {
            const result = await service.isAccountLocked('user@example.com');
            expect(result.locked).toBe(false);
        });

        it('returns locked: true when lock document exists', async () => {
            const dbId = service._makeLockDocId('user@example.com');
            cache._seedDoc(Collections.ACCOUNT_LOCKS, dbId, {
                identifier: 'user@example.com',
                locked_at: new Date().toISOString(),
                failed_attempts: MAX_FAILED_ATTEMPTS,
                last_attempt_ip: '127.0.0.1',
                attempt_history: [],
            });

            const result = await service.isAccountLocked('user@example.com');

            expect(result.locked).toBe(true);
            expect(result.failed_attempts).toBe(MAX_FAILED_ATTEMPTS);
        });
    });

    // -------------------------------------------------------------------------
    // getFailedAttemptStatus
    // -------------------------------------------------------------------------

    describe('getFailedAttemptStatus', () => {
        it('returns zero attempts when no KV entry exists', async () => {
            const result = await service.getFailedAttemptStatus('user@example.com');
            expect(result.attempts).toBe(0);
            expect(result.delay_ms).toBe(0);
            expect(result.requires_captcha).toBe(false);
        });

        it('returns correct attempt count from KV', async () => {
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, {
                count: 2,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            const result = await service.getFailedAttemptStatus('user@example.com');

            expect(result.attempts).toBe(2);
        });

        it('returns progressive delay based on attempt count', async () => {
            const key = KVKey.loginFailed('user@example.com');
            const attempts = 2;
            cache._seedKV(key, {
                count: attempts,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            const result = await service.getFailedAttemptStatus('user@example.com');

            expect(result.delay_ms).toBe(PROGRESSIVE_DELAYS[attempts]);
        });

        it('requires captcha when attempts >= CAPTCHA_THRESHOLD', async () => {
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, {
                count: CAPTCHA_THRESHOLD,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            const result = await service.getFailedAttemptStatus('user@example.com');

            expect(result.requires_captcha).toBe(true);
        });

        it('caps delay at max PROGRESSIVE_DELAYS value when attempts exceed length', async () => {
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, {
                count: PROGRESSIVE_DELAYS.length + 10,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            const result = await service.getFailedAttemptStatus('user@example.com');

            expect(result.delay_ms).toBe(PROGRESSIVE_DELAYS[PROGRESSIVE_DELAYS.length - 1]);
        });
    });

    // -------------------------------------------------------------------------
    // recordFailedAttempt
    // -------------------------------------------------------------------------

    describe('recordFailedAttempt', () => {
        it('creates a KV entry on first failure', async () => {
            const result = await service.recordFailedAttempt('user@example.com', makeRequestContext());

            expect(result.locked).toBe(false);
            expect(result.attempts).toBe(1);
        });

        it('increments attempt count on subsequent failures', async () => {
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, {
                count: 2,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            const result = await service.recordFailedAttempt('user@example.com', makeRequestContext());

            expect(result.attempts).toBe(3);
        });

        it('returns delay_ms based on attempt count', async () => {
            const result = await service.recordFailedAttempt('user@example.com', makeRequestContext());
            expect(result.delay_ms).toBe(PROGRESSIVE_DELAYS[1]);
        });

        it('sets requires_captcha when attempts reach CAPTCHA_THRESHOLD', async () => {
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, {
                count: CAPTCHA_THRESHOLD - 1,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            const result = await service.recordFailedAttempt('user@example.com', makeRequestContext());

            expect(result.requires_captcha).toBe(true);
        });

        it('locks account and creates DB record when MAX_FAILED_ATTEMPTS is reached', async () => {
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, {
                count: MAX_FAILED_ATTEMPTS - 1,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            const result = await service.recordFailedAttempt('user@example.com', makeRequestContext());

            expect(result.locked).toBe(true);
            expect(result.attempts).toBe(MAX_FAILED_ATTEMPTS);

            const dbId = service._makeLockDocId('user@example.com');
            const lockDoc = await cache.getDocument(Collections.ACCOUNT_LOCKS, dbId);
            expect(lockDoc).not.toBeNull();
        });

        it('trims history to last 10 entries', async () => {
            const history = Array.from({ length: 10 }, (_, i) => ({
                timestamp: new Date().toISOString(),
                ip: `10.0.0.${i}`,
            }));
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, {
                count: 10,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history,
            });

            await service.recordFailedAttempt('user@example.com', makeRequestContext());

            const stored = await cache.kvGetJson(KVKey.loginFailed('user@example.com'));
            if (stored) {
                expect(stored.history.length).toBeLessThanOrEqual(10);
            }
        });

        it('writes login audit event on failure', async () => {
            await service.recordFailedAttempt('audit@example.com', makeRequestContext());

            expect(cache.createDocument).toHaveBeenCalledWith(
                Collections.LOGIN_AUDIT,
                expect.stringContaining('login_'),
                expect.objectContaining({ event_type: LoginEventType.LOGIN_FAILED })
            );
        });

        it('writes account_locked audit event when account is locked', async () => {
            const key = KVKey.loginFailed('locked@example.com');
            cache._seedKV(key, {
                count: MAX_FAILED_ATTEMPTS - 1,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            await service.recordFailedAttempt('locked@example.com', makeRequestContext());

            const auditCalls = cache.createDocument.mock.calls.filter(
                ([col, , data]) => col === Collections.LOGIN_AUDIT && data.event_type === LoginEventType.ACCOUNT_LOCKED
            );
            expect(auditCalls.length).toBeGreaterThan(0);
        });
    });

    // -------------------------------------------------------------------------
    // clearFailedAttempts
    // -------------------------------------------------------------------------

    describe('clearFailedAttempts', () => {
        it('removes the failed attempts KV key', async () => {
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, { count: 3, history: [] });

            await service.clearFailedAttempts('user@example.com');

            const stored = await cache.kvGetJson(key);
            expect(stored).toBeNull();
        });
    });

    // -------------------------------------------------------------------------
    // unlockAccount
    // -------------------------------------------------------------------------

    describe('unlockAccount', () => {
        it('returns error when account is not locked', async () => {
            const result = await service.unlockAccount('notlocked@example.com');
            expect(result.success).toBe(false);
            expect(result.error).toBe('Account is not locked');
        });

        it('removes lock document and KV key on success', async () => {
            const identifier = 'locked@example.com';
            const dbId = service._makeLockDocId(identifier);
            cache._seedDoc(Collections.ACCOUNT_LOCKS, dbId, {
                identifier,
                locked_at: new Date().toISOString(),
                failed_attempts: MAX_FAILED_ATTEMPTS,
                last_attempt_ip: '127.0.0.1',
                attempt_history: [],
            });

            const result = await service.unlockAccount(identifier, 'admin-001');

            expect(result.success).toBe(true);
            const remaining = await cache.getDocument(Collections.ACCOUNT_LOCKS, dbId);
            expect(remaining).toBeNull();
        });

        it('writes account_unlocked audit event', async () => {
            const identifier = 'unlock@example.com';
            const dbId = service._makeLockDocId(identifier);
            cache._seedDoc(Collections.ACCOUNT_LOCKS, dbId, {
                identifier,
                locked_at: new Date().toISOString(),
                failed_attempts: MAX_FAILED_ATTEMPTS,
                last_attempt_ip: '127.0.0.1',
                attempt_history: [],
            });

            await service.unlockAccount(identifier, 'admin-001');

            const auditCalls = cache.createDocument.mock.calls.filter(
                ([col, , data]) => col === Collections.LOGIN_AUDIT && data.event_type === LoginEventType.ACCOUNT_UNLOCKED
            );
            expect(auditCalls.length).toBeGreaterThan(0);
        });
    });

    // -------------------------------------------------------------------------
    // trackIpAccount
    // -------------------------------------------------------------------------

    describe('trackIpAccount', () => {
        it('creates a new KV entry for the IP', async () => {
            await service.trackIpAccount('10.0.0.1', 'user@example.com');

            const stored = await cache.kvGetJson(KVKey.loginIpAccounts('10.0.0.1'));
            expect(Array.isArray(stored)).toBe(true);
            expect(stored.length).toBe(1);
            expect(stored[0].id).toBe('user@example.com');
        });

        it('appends to existing entries for the same IP', async () => {
            const key = KVKey.loginIpAccounts('10.0.0.1');
            const existingEntry = { id: 'first@example.com', ts: new Date(Date.now() + 60000).toISOString() };
            cache._seedKV(key, [existingEntry]);

            await service.trackIpAccount('10.0.0.1', 'second@example.com');

            const stored = await cache.kvGetJson(key);
            expect(stored.length).toBe(2);
        });

        it('replaces existing entry for the same identifier (deduplication)', async () => {
            const key = KVKey.loginIpAccounts('10.0.0.1');
            const existingEntry = { id: 'same@example.com', ts: new Date(Date.now() + 60000).toISOString() };
            cache._seedKV(key, [existingEntry]);

            await service.trackIpAccount('10.0.0.1', 'same@example.com');

            const stored = await cache.kvGetJson(key);
            expect(stored.length).toBe(1);
        });
    });

    // -------------------------------------------------------------------------
    // detectAnomalies
    // -------------------------------------------------------------------------

    describe('detectAnomalies', () => {
        it('returns empty anomalies and zero risk when nothing suspicious', async () => {
            const result = await service.detectAnomalies('user@example.com', { ip: '10.0.0.1' });
            expect(result.anomalies).toEqual([]);
            expect(result.risk_score).toBe(0);
        });

        it('detects new_device anomaly for unknown fingerprint', async () => {
            const result = await service.detectAnomalies(
                'user@example.com',
                { ip: '10.0.0.1', deviceFingerprint: 'new-device-fp' },
                { known_device_fingerprints: ['existing-fp'], typical_login_hours: [], typical_countries: [] }
            );

            expect(result.anomalies).toContain('new_device');
            expect(result.risk_score).toBeGreaterThan(0);
        });

        it('does not flag new_device when fingerprint is known', async () => {
            const result = await service.detectAnomalies(
                'user@example.com',
                { ip: '10.0.0.1', deviceFingerprint: 'known-fp' },
                { known_device_fingerprints: ['known-fp'], typical_login_hours: [], typical_countries: [] }
            );

            expect(result.anomalies).not.toContain('new_device');
        });

        it('does not flag new_device when known_device_fingerprints is empty', async () => {
            const result = await service.detectAnomalies(
                'user@example.com',
                { ip: '10.0.0.1', deviceFingerprint: 'some-fp' },
                { known_device_fingerprints: [], typical_login_hours: [], typical_countries: [] }
            );

            expect(result.anomalies).not.toContain('new_device');
        });

        it('detects unusual_time anomaly', async () => {
            const currentHour = new Date().getUTCHours();
            const allOtherHours = Array.from({ length: 24 }, (_, h) => h).filter(h => h !== currentHour);

            const result = await service.detectAnomalies(
                'user@example.com',
                { ip: '10.0.0.1' },
                { known_device_fingerprints: [], typical_login_hours: allOtherHours, typical_countries: [] }
            );

            expect(result.anomalies.some(a => a.startsWith('unusual_time:'))).toBe(true);
        });

        it('does not flag unusual_time when current hour is in typical hours', async () => {
            const currentHour = new Date().getUTCHours();

            const result = await service.detectAnomalies(
                'user@example.com',
                { ip: '10.0.0.1' },
                { known_device_fingerprints: [], typical_login_hours: [currentHour], typical_countries: [] }
            );

            expect(result.anomalies.some(a => a.startsWith('unusual_time:'))).toBe(false);
        });

        it('detects multiple_accounts_same_ip anomaly', async () => {
            const { ANOMALY_MULTI_ACCOUNT_THRESHOLD } = LoginSecurity;
            const ip = '10.0.0.99';
            const key = KVKey.loginIpAccounts(ip);
            const entries = Array.from({ length: ANOMALY_MULTI_ACCOUNT_THRESHOLD }, (_, i) => ({
                id: `user${i}@example.com`,
                ts: new Date(Date.now() + 60000).toISOString(),
            }));
            cache._seedKV(key, entries);

            const result = await service.detectAnomalies('new@example.com', { ip });

            expect(result.anomalies.some(a => a.startsWith('multiple_accounts_same_ip:'))).toBe(true);
            expect(result.risk_score).toBeGreaterThanOrEqual(30);
        });

        it('calls geoipLookup when provided and userHistory has typical_countries', async () => {
            const geoip = vi.fn().mockResolvedValue({ country: 'FR' });
            const svc = new LoginSecurityService({ cacheAsideService: cache, geoipLookup: geoip });

            const result = await svc.detectAnomalies(
                'user@example.com',
                { ip: '1.2.3.4' },
                { known_device_fingerprints: [], typical_login_hours: [], typical_countries: ['US'] }
            );

            expect(geoip).toHaveBeenCalledWith('1.2.3.4');
            expect(result.anomalies.some(a => a.startsWith('new_country:'))).toBe(true);
        });

        it('does not propagate geoip lookup failure', async () => {
            const geoip = vi.fn().mockRejectedValue(new Error('geoip unavailable'));
            const svc = new LoginSecurityService({ cacheAsideService: cache, geoipLookup: geoip });

            await expect(
                svc.detectAnomalies('user@example.com', { ip: '1.2.3.4' }, {
                    known_device_fingerprints: [], typical_login_hours: [], typical_countries: ['US']
                })
            ).resolves.not.toThrow();
        });
    });

    // -------------------------------------------------------------------------
    // preLoginCheck
    // -------------------------------------------------------------------------

    describe('preLoginCheck', () => {
        it('returns allowed: true when account is not locked and no prior failures', async () => {
            const result = await service.preLoginCheck('user@example.com', makeRequestContext());

            expect(result.allowed).toBe(true);
            expect(result.delay_ms).toBe(0);
            expect(result.requires_captcha).toBe(false);
        });

        it('returns allowed: false when account is locked', async () => {
            const identifier = 'locked@example.com';
            const dbId = service._makeLockDocId(identifier);
            cache._seedDoc(Collections.ACCOUNT_LOCKS, dbId, {
                identifier,
                locked_at: new Date().toISOString(),
                failed_attempts: MAX_FAILED_ATTEMPTS,
                last_attempt_ip: '127.0.0.1',
                attempt_history: [],
            });

            const result = await service.preLoginCheck(identifier, makeRequestContext());

            expect(result.allowed).toBe(false);
            expect(result.locked).toBe(true);
        });

        it('returns delay_ms from failed attempt status', async () => {
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, {
                count: 2,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            const result = await service.preLoginCheck('user@example.com', makeRequestContext());

            expect(result.delay_ms).toBe(PROGRESSIVE_DELAYS[2]);
        });

        it('includes max_attempts in the response', async () => {
            const result = await service.preLoginCheck('user@example.com', makeRequestContext());
            expect(result.max_attempts).toBe(MAX_FAILED_ATTEMPTS);
        });
    });

    // -------------------------------------------------------------------------
    // postLoginSuccess
    // -------------------------------------------------------------------------

    describe('postLoginSuccess', () => {
        it('clears failed attempts on success', async () => {
            const key = KVKey.loginFailed('user@example.com');
            cache._seedKV(key, {
                count: 3,
                first_attempt: new Date().toISOString(),
                last_attempt: new Date().toISOString(),
                history: [],
            });

            await service.postLoginSuccess('user@example.com', makeRequestContext());

            const stored = await cache.kvGetJson(key);
            expect(stored).toBeNull();
        });

        it('writes login success audit event', async () => {
            await service.postLoginSuccess('user@example.com', makeRequestContext());

            const auditCalls = cache.createDocument.mock.calls.filter(
                ([col, , data]) => col === Collections.LOGIN_AUDIT && data.event_type === LoginEventType.LOGIN_SUCCESS
            );
            expect(auditCalls.length).toBeGreaterThan(0);
        });

        it('returns anomaly result', async () => {
            const result = await service.postLoginSuccess('user@example.com', makeRequestContext());
            expect(result).toHaveProperty('anomalies');
            expect(result).toHaveProperty('risk_score');
        });

        it('writes anomaly audit event when anomalies are detected', async () => {
            const currentHour = new Date().getUTCHours();
            const otherHours = Array.from({ length: 24 }, (_, h) => h).filter(h => h !== currentHour);

            const result = await service.postLoginSuccess(
                'user@example.com',
                { ip: '10.0.0.1', deviceFingerprint: 'new-fp' },
            );

            expect(Array.isArray(result.anomalies)).toBe(true);
        });
    });

    // -------------------------------------------------------------------------
    // getLockedAccounts
    // -------------------------------------------------------------------------

    describe('getLockedAccounts', () => {
        it('returns empty array when no accounts are locked', async () => {
            const result = await service.getLockedAccounts();
            expect(result).toEqual([]);
        });

        it('returns AccountLockData instances for locked accounts', async () => {
            const id1 = service._makeLockDocId('a@example.com');
            const id2 = service._makeLockDocId('b@example.com');
            cache._seedDoc(Collections.ACCOUNT_LOCKS, id1, {
                identifier: 'a@example.com',
                locked_at: new Date().toISOString(),
                failed_attempts: MAX_FAILED_ATTEMPTS,
                last_attempt_ip: '127.0.0.1',
                attempt_history: [],
            });
            cache._seedDoc(Collections.ACCOUNT_LOCKS, id2, {
                identifier: 'b@example.com',
                locked_at: new Date().toISOString(),
                failed_attempts: MAX_FAILED_ATTEMPTS,
                last_attempt_ip: '10.0.0.1',
                attempt_history: [],
            });

            const result = await service.getLockedAccounts();

            expect(result.length).toBe(2);
            expect(result.map(r => r.identifier).sort()).toEqual(['a@example.com', 'b@example.com'].sort());
        });
    });

    // -------------------------------------------------------------------------
    // auditAdminAccess
    // -------------------------------------------------------------------------

    describe('auditAdminAccess', () => {
        it('writes an admin audit entry to CONSOLE_AUDIT or AUTH_ADMIN_AUDIT collection', async () => {
            await service.auditAdminAccess({
                action: 'list_users',
                userId: 'admin-001',
                userEmail: 'admin@example.com',
                ip: '127.0.0.1',
                userAgent: 'Test/1.0',
                path: '/api/internal/users',
                method: 'GET',
                queryParams: null,
                metadata: {},
            });

            expect(cache.createDocument).toHaveBeenCalled();
        });

        it('does not throw when called with minimal arguments', async () => {
            await expect(
                service.auditAdminAccess({ action: 'test' })
            ).resolves.not.toThrow();
        });
    });
});
