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

import { describe, it, expect } from 'vitest';
import {
    AccountLockData,
    FailedAttemptsData,
    LoginAttemptEntry,
    IpTrackEntry,
    LoginAuditEntry,
    AuthAuditEntry,
    ApiKeyDocument,
    SessionDocument,
    WebSessionDocument,
    OperatorSessionDocument,
    DeviceLinkClaim,
    DeviceLinkData,
} from '@vsod/models/auth_models.js';
import { SessionType } from '@vsod/constants/session.js';
import { ApiKeyStatus, AuthMethod, DeviceLinkStatus } from '@vsod/constants/auth.js';

function futureDate(offsetMs = 3600000) {
    return new Date(Date.now() + offsetMs).toISOString();
}

function pastDate(offsetMs = 3600000) {
    return new Date(Date.now() - offsetMs).toISOString();
}

// ---------------------------------------------------------------------------
// AccountLockData
// ---------------------------------------------------------------------------

describe('AccountLockData [UNIT]', () => {
    it('throws on missing identifier', () => {
        expect(() => AccountLockData.parse({ failed_attempts: 5, last_attempt_ip: '127.0.0.1' })).toThrow();
    });

    it('defaults failed_attempts to 0 when not provided', () => {
        const d = AccountLockData.parse({ identifier: 'user@example.com' });
        expect(d.failed_attempts).toBe(0);
    });

    it('defaults last_attempt_ip to null when not provided', () => {
        const d = AccountLockData.parse({ identifier: 'user@example.com' });
        expect(d.last_attempt_ip).toBeNull();
    });

    it('parses with required fields', () => {
        const d = AccountLockData.parse({ identifier: 'user@example.com', failed_attempts: 5, last_attempt_ip: '127.0.0.1' });
        expect(d.identifier).toBe('user@example.com');
        expect(d.failed_attempts).toBe(5);
        expect(d.last_attempt_ip).toBe('127.0.0.1');
    });

    it('defaults locked_at to current time', () => {
        const before = Date.now();
        const d = AccountLockData.parse({ identifier: 'u@e.com', failed_attempts: 5, last_attempt_ip: '127.0.0.1' });
        expect(d.locked_at).toBeInstanceOf(Date);
        expect(d.locked_at.getTime()).toBeGreaterThanOrEqual(before);
    });

    it('defaults attempt_history to empty array', () => {
        const d = AccountLockData.parse({ identifier: 'u@e.com', failed_attempts: 5, last_attempt_ip: '127.0.0.1' });
        expect(d.attempt_history).toEqual([]);
    });

    it('serializes via forWire with string timestamp', () => {
        const d = AccountLockData.parse({ identifier: 'u@e.com', failed_attempts: 5, last_attempt_ip: '127.0.0.1' });
        const wire = d.forWire();
        expect(typeof wire.locked_at).toBe('string');
    });

    it('round-trips through parse', () => {
        const original = AccountLockData.parse({ identifier: 'u@e.com', failed_attempts: 3, last_attempt_ip: '10.0.0.1' });
        const restored = AccountLockData.parse(original.forWire());
        expect(restored.identifier).toBe(original.identifier);
        expect(restored.failed_attempts).toBe(original.failed_attempts);
    });
});

// ---------------------------------------------------------------------------
// FailedAttemptsData
// ---------------------------------------------------------------------------

describe('FailedAttemptsData [UNIT]', () => {
    it('defaults count to 0 when not provided', () => {
        const d = FailedAttemptsData.parse({});
        expect(d.count).toBe(0);
    });

    it('defaults first_attempt to current time when not provided', () => {
        const before = Date.now();
        const d = FailedAttemptsData.parse({});
        expect(d.first_attempt).toBeInstanceOf(Date);
        expect(d.first_attempt.getTime()).toBeGreaterThanOrEqual(before);
    });

    it('parses with required fields', () => {
        const d = FailedAttemptsData.parse({ count: 2, first_attempt: new Date().toISOString() });
        expect(d.count).toBe(2);
        expect(d.first_attempt).toBeInstanceOf(Date);
    });

    it('defaults last_attempt to current time', () => {
        const before = Date.now();
        const d = FailedAttemptsData.parse({ count: 1, first_attempt: new Date().toISOString() });
        expect(d.last_attempt).toBeInstanceOf(Date);
        expect(d.last_attempt.getTime()).toBeGreaterThanOrEqual(before);
    });

    it('defaults history to empty array', () => {
        const d = FailedAttemptsData.parse({ count: 1, first_attempt: new Date().toISOString() });
        expect(d.history).toEqual([]);
    });

    it('accepts and preserves history entries', () => {
        const history = [{ ip: '10.0.0.1', timestamp: new Date().toISOString() }];
        const d = FailedAttemptsData.parse({ count: 1, first_attempt: new Date().toISOString(), history });
        expect(d.history).toHaveLength(1);
    });
});

// ---------------------------------------------------------------------------
// LoginAttemptEntry
// ---------------------------------------------------------------------------

describe('LoginAttemptEntry [UNIT]', () => {
    it('defaults ip to null when not provided', () => {
        const d = LoginAttemptEntry.parse({ timestamp: new Date().toISOString() });
        expect(d.ip).toBeNull();
    });

    it('defaults timestamp to current time when not provided', () => {
        const before = Date.now();
        const d = LoginAttemptEntry.parse({ ip: '127.0.0.1' });
        expect(d.timestamp).toBeInstanceOf(Date);
        expect(d.timestamp.getTime()).toBeGreaterThanOrEqual(before);
    });

    it('parses with required fields', () => {
        const d = LoginAttemptEntry.parse({ ip: '127.0.0.1', timestamp: new Date().toISOString() });
        expect(d.ip).toBe('127.0.0.1');
        expect(d.timestamp).toBeInstanceOf(Date);
    });

    it('defaults user_agent to null', () => {
        const d = LoginAttemptEntry.parse({ ip: '127.0.0.1', timestamp: new Date().toISOString() });
        expect(d.user_agent).toBeNull();
    });

    it('defaults device_fingerprint to null', () => {
        const d = LoginAttemptEntry.parse({ ip: '127.0.0.1', timestamp: new Date().toISOString() });
        expect(d.device_fingerprint).toBeNull();
    });

    it('accepts all optional fields', () => {
        const d = LoginAttemptEntry.parse({
            ip: '127.0.0.1',
            timestamp: new Date().toISOString(),
            user_agent: 'Mozilla/5.0',
            device_fingerprint: 'abc123',
        });
        expect(d.user_agent).toBe('Mozilla/5.0');
        expect(d.device_fingerprint).toBe('abc123');
    });
});

// ---------------------------------------------------------------------------
// IpTrackEntry
// ---------------------------------------------------------------------------

describe('IpTrackEntry [UNIT]', () => {
    it('throws on missing id', () => {
        expect(() => IpTrackEntry.parse({})).toThrow();
    });

    it('parses with required id', () => {
        const d = IpTrackEntry.parse({ id: 'user@example.com' });
        expect(d.id).toBe('user@example.com');
    });

    it('defaults ts to current time', () => {
        const before = Date.now();
        const d = IpTrackEntry.parse({ id: 'user@example.com' });
        expect(d.ts).toBeInstanceOf(Date);
        expect(d.ts.getTime()).toBeGreaterThanOrEqual(before);
    });

    it('accepts explicit ts', () => {
        const ts = new Date('2026-01-01T00:00:00Z');
        const d = IpTrackEntry.parse({ id: 'u@e.com', ts: ts.toISOString() });
        expect(d.ts.getTime()).toBe(ts.getTime());
    });
});

// ---------------------------------------------------------------------------
// LoginAuditEntry
// ---------------------------------------------------------------------------

describe('LoginAuditEntry [UNIT]', () => {
    const base = {
        event_type: 'login.failed',
        identifier: 'user@example.com',
        identifier_redacted: 'use***@example.com',
    };

    it('throws on missing event_type', () => {
        expect(() => LoginAuditEntry.parse({ identifier: 'u@e.com', identifier_redacted: 'u***' })).toThrow();
    });

    it('throws on missing identifier', () => {
        expect(() => LoginAuditEntry.parse({ event_type: 'x', identifier_redacted: 'u***' })).toThrow();
    });

    it('throws on missing identifier_redacted', () => {
        expect(() => LoginAuditEntry.parse({ event_type: 'x', identifier: 'u@e.com' })).toThrow();
    });

    it('parses with required fields', () => {
        const d = LoginAuditEntry.parse(base);
        expect(d.event_type).toBe('login.failed');
        expect(d.identifier).toBe('user@example.com');
        expect(d.identifier_redacted).toBe('use***@example.com');
    });

    it('defaults timestamp to current time', () => {
        const before = Date.now();
        const d = LoginAuditEntry.parse(base);
        expect(d.timestamp).toBeInstanceOf(Date);
        expect(d.timestamp.getTime()).toBeGreaterThanOrEqual(before);
    });

    it('defaults ip, user_agent, device_fingerprint to null', () => {
        const d = LoginAuditEntry.parse(base);
        expect(d.ip).toBeNull();
        expect(d.user_agent).toBeNull();
        expect(d.device_fingerprint).toBeNull();
    });

    it('defaults metadata to empty object', () => {
        const d = LoginAuditEntry.parse(base);
        expect(d.metadata).toEqual({});
    });

    it('accepts all optional fields', () => {
        const d = LoginAuditEntry.parse({
            ...base,
            ip: '127.0.0.1',
            user_agent: 'Test/1.0',
            device_fingerprint: 'fp-001',
            metadata: { anomalies: [] },
        });
        expect(d.ip).toBe('127.0.0.1');
        expect(d.metadata.anomalies).toEqual([]);
    });

    it('serializes via forWire with string timestamp', () => {
        const wire = LoginAuditEntry.parse(base).forWire();
        expect(typeof wire.timestamp).toBe('string');
    });
});

// ---------------------------------------------------------------------------
// AuthAuditEntry
// ---------------------------------------------------------------------------

describe('AuthAuditEntry [UNIT]', () => {
    const base = { event_type: 'operator.auth.success', result: 'success' };

    it('throws on missing event_type', () => {
        expect(() => AuthAuditEntry.parse({ result: 'success' })).toThrow();
    });

    it('throws on missing result', () => {
        expect(() => AuthAuditEntry.parse({ event_type: 'x' })).toThrow();
    });

    it('parses with required fields', () => {
        const d = AuthAuditEntry.parse(base);
        expect(d.event_type).toBe('operator.auth.success');
        expect(d.result).toBe('success');
    });

    it('defaults user_id, api_key_prefix, ip, user_agent to null', () => {
        const d = AuthAuditEntry.parse(base);
        expect(d.user_id).toBeNull();
        expect(d.api_key_prefix).toBeNull();
        expect(d.ip).toBeNull();
        expect(d.user_agent).toBeNull();
    });

    it('defaults auth_method to AuthMethod.KV_PUBSUB', () => {
        const d = AuthAuditEntry.parse(base);
        expect(d.auth_method).toBe(AuthMethod.KV_PUBSUB);
    });

    it('defaults metadata to empty object', () => {
        expect(AuthAuditEntry.parse(base).metadata).toEqual({});
    });

    it('accepts explicit auth_method override', () => {
        const d = AuthAuditEntry.parse({ ...base, auth_method: AuthMethod.SESSION });
        expect(d.auth_method).toBe(AuthMethod.SESSION);
    });

    it('serializes via forWire with string timestamp', () => {
        const wire = AuthAuditEntry.parse(base).forWire();
        expect(typeof wire.timestamp).toBe('string');
    });
});

// ---------------------------------------------------------------------------
// ApiKeyDocument
// ---------------------------------------------------------------------------

describe('ApiKeyDocument [UNIT]', () => {
    const base = { user_id: 'user-001', client_name: 'My Client' };

    it('throws on missing user_id', () => {
        expect(() => ApiKeyDocument.parse({ client_name: 'x' })).toThrow();
    });

    it('throws on missing client_name', () => {
        expect(() => ApiKeyDocument.parse({ user_id: 'user-001' })).toThrow();
    });

    it('parses with required fields', () => {
        const d = ApiKeyDocument.parse(base);
        expect(d.user_id).toBe('user-001');
        expect(d.client_name).toBe('My Client');
    });

    it('defaults organization_id, operator_id to null', () => {
        const d = ApiKeyDocument.parse(base);
        expect(d.organization_id).toBeNull();
        expect(d.operator_id).toBeNull();
    });

    it('defaults permissions to empty array', () => {
        expect(ApiKeyDocument.parse(base).permissions).toEqual([]);
    });

    it('defaults status to ApiKeyStatus.ACTIVE', () => {
        expect(ApiKeyDocument.parse(base).status).toBe(ApiKeyStatus.ACTIVE);
    });

    it('defaults last_used_at and expires_at to null', () => {
        const d = ApiKeyDocument.parse(base);
        expect(d.last_used_at).toBeNull();
        expect(d.expires_at).toBeNull();
    });

    it('accepts explicit permissions and status', () => {
        const d = ApiKeyDocument.parse({ ...base, permissions: ['read'], status: ApiKeyStatus.REVOKED });
        expect(d.permissions).toEqual(['read']);
        expect(d.status).toBe(ApiKeyStatus.REVOKED);
    });

    it('round-trips through forDB', () => {
        const original = ApiKeyDocument.parse({ ...base, organization_id: 'org-001', permissions: ['read', 'write'] });
        const restored = ApiKeyDocument.parse(original.forDB());
        expect(restored.user_id).toBe(original.user_id);
        expect(restored.permissions).toEqual(original.permissions);
    });
});

// ---------------------------------------------------------------------------
// SessionDocument — polymorphic dispatch
// ---------------------------------------------------------------------------

describe('SessionDocument.parse polymorphic dispatch [UNIT]', () => {
    const expiry = futureDate();
    const baseSession = {
        id: 'session-001',
        session_type: SessionType.WEB,
        user_id: 'user-001',
        absolute_expires_at: expiry,
        idle_expires_at: expiry,
    };

    it('returns WebSessionDocument for session_type=web', () => {
        const d = SessionDocument.parse({ ...baseSession, session_type: SessionType.WEB });
        expect(d).toBeInstanceOf(WebSessionDocument);
    });

    it('returns OperatorSessionDocument for session_type=operator', () => {
        const d = SessionDocument.parse({
            ...baseSession,
            session_type: SessionType.OPERATOR,
            operator_id: 'op-001',
        });
        expect(d).toBeInstanceOf(OperatorSessionDocument);
    });

    it('returns base SessionDocument for invalid session_type', () => {
        const d = SessionDocument.parse({ ...baseSession, session_type: 'INVALID' });
        expect(d).toBeInstanceOf(SessionDocument);
    });
});

// ---------------------------------------------------------------------------
// WebSessionDocument
// ---------------------------------------------------------------------------

describe('WebSessionDocument [UNIT]', () => {
    const expiry = futureDate();
    const base = {
        id: 'ws-001',
        session_type: SessionType.WEB,
        user_id: 'user-001',
        absolute_expires_at: expiry,
        idle_expires_at: expiry,
    };

    it('throws on missing id', () => {
        expect(() => WebSessionDocument.parse({ session_type: SessionType.WEB, user_id: 'u', absolute_expires_at: expiry, idle_expires_at: expiry })).toThrow();
    });

    it('throws on missing user_id', () => {
        expect(() => WebSessionDocument.parse({ id: 'ws-001', session_type: SessionType.WEB, absolute_expires_at: expiry, idle_expires_at: expiry })).toThrow();
    });

    it('parses with required fields', () => {
        const d = WebSessionDocument.parse(base);
        expect(d.id).toBe('ws-001');
        expect(d.user_id).toBe('user-001');
        expect(d.session_type).toBe(SessionType.WEB);
    });

    it('keeps operator_id if provided', () => {
        const d = WebSessionDocument.parse({ ...base, operator_id: 'op-001' });
        expect(d.operator_id).toBe('op-001');
    });

    it('defaults is_active to true', () => {
        expect(WebSessionDocument.parse(base).is_active).toBe(true);
    });

    it('defaults suspicious_activity to false', () => {
        expect(WebSessionDocument.parse(base).suspicious_activity).toBe(false);
    });

    it('defaults ip_changes to 0', () => {
        expect(WebSessionDocument.parse(base).ip_changes).toBe(0);
    });

    it('accepts client_ip and user_agent', () => {
        const d = WebSessionDocument.parse({ ...base, client_ip: '10.0.0.1', user_agent: 'Test/1.0' });
        expect(d.client_ip).toBe('10.0.0.1');
        expect(d.user_agent).toBe('Test/1.0');
    });

    it('round-trips through forDB', () => {
        const original = WebSessionDocument.parse({ ...base, organization_id: 'org-001' });
        const restored = WebSessionDocument.parse(original.forDB());
        expect(restored.id).toBe(original.id);
        expect(restored.user_id).toBe(original.user_id);
        expect(restored.organization_id).toBe(original.organization_id);
    });
});

// ---------------------------------------------------------------------------
// OperatorSessionDocument
// ---------------------------------------------------------------------------

describe('OperatorSessionDocument [UNIT]', () => {
    const expiry = futureDate();
    const base = {
        id: 'op-session-001',
        session_type: SessionType.OPERATOR,
        user_id: 'user-001',
        operator_id: 'op-001',
        absolute_expires_at: expiry,
        idle_expires_at: expiry,
    };

    it('throws on missing operator_id', () => {
        expect(() => OperatorSessionDocument.parse({
            id: 'x', session_type: SessionType.OPERATOR, user_id: 'u',
            absolute_expires_at: expiry, idle_expires_at: expiry,
        })).toThrow();
    });

    it('parses with required fields', () => {
        const d = OperatorSessionDocument.parse(base);
        expect(d.operator_id).toBe('op-001');
        expect(d.session_type).toBe(SessionType.OPERATOR);
    });

    it('accepts optional operator_status', () => {
        const d = OperatorSessionDocument.parse({ ...base, operator_status: 'active' });
        expect(d.operator_status).toBe('active');
    });

    it('round-trips through forDB', () => {
        const original = OperatorSessionDocument.parse(base);
        const restored = OperatorSessionDocument.parse(original.forDB());
        expect(restored.operator_id).toBe(original.operator_id);
        expect(restored.session_type).toBe(original.session_type);
    });
});

// ---------------------------------------------------------------------------
// DeviceLinkClaim
// ---------------------------------------------------------------------------

describe('DeviceLinkClaim [UNIT]', () => {
    it('throws on missing system_fingerprint', () => {
        expect(() => DeviceLinkClaim.parse({})).toThrow();
    });

    it('parses with required system_fingerprint', () => {
        const d = DeviceLinkClaim.parse({ system_fingerprint: 'fp-001' });
        expect(d.system_fingerprint).toBe('fp-001');
    });

    it('defaults hostname and operator_id to null', () => {
        const d = DeviceLinkClaim.parse({ system_fingerprint: 'fp-001' });
        expect(d.hostname).toBeNull();
        expect(d.operator_id).toBeNull();
    });

    it('defaults claimed_at to current time', () => {
        const before = Date.now();
        const d = DeviceLinkClaim.parse({ system_fingerprint: 'fp-001' });
        expect(d.claimed_at.getTime()).toBeGreaterThanOrEqual(before);
    });

    it('accepts hostname and operator_id', () => {
        const d = DeviceLinkClaim.parse({ system_fingerprint: 'fp-001', hostname: 'worker-1', operator_id: 'op-001' });
        expect(d.hostname).toBe('worker-1');
        expect(d.operator_id).toBe('op-001');
    });
});

// ---------------------------------------------------------------------------
// DeviceLinkData
// ---------------------------------------------------------------------------

describe('DeviceLinkData [UNIT]', () => {
    const base = {
        token: 'tok-abc123',
        user_id: 'user-001',
        status: DeviceLinkStatus.PENDING,
    };

    it('throws on missing token', () => {
        expect(() => DeviceLinkData.parse({ user_id: 'u', status: DeviceLinkStatus.PENDING })).toThrow();
    });

    it('throws on missing user_id', () => {
        expect(() => DeviceLinkData.parse({ token: 't', status: DeviceLinkStatus.PENDING })).toThrow();
    });

    it('throws on missing status', () => {
        expect(() => DeviceLinkData.parse({ token: 't', user_id: 'u' })).toThrow();
    });

    it('parses with required fields', () => {
        const d = DeviceLinkData.parse(base);
        expect(d.token).toBe('tok-abc123');
        expect(d.user_id).toBe('user-001');
        expect(d.status).toBe(DeviceLinkStatus.PENDING);
    });

    it('defaults uses to 0', () => {
        expect(DeviceLinkData.parse(base).uses).toBe(0);
    });

    it('defaults claims to empty array', () => {
        expect(DeviceLinkData.parse(base).claims).toEqual([]);
    });

    it('defaults organization_id, operator_id, web_session_id to null', () => {
        const d = DeviceLinkData.parse(base);
        expect(d.organization_id).toBeNull();
        expect(d.operator_id).toBeNull();
        expect(d.web_session_id).toBeNull();
    });

    it('parses claims as DeviceLinkClaim instances', () => {
        const claims = [{ system_fingerprint: 'fp-001', hostname: 'host-1' }];
        const d = DeviceLinkData.parse({ ...base, claims });
        expect(d.claims).toHaveLength(1);
        expect(d.claims[0]).toBeInstanceOf(DeviceLinkClaim);
        expect(d.claims[0].system_fingerprint).toBe('fp-001');
    });

    it('static fromKV is equivalent to parse', () => {
        const d1 = DeviceLinkData.parse(base);
        const d2 = DeviceLinkData.fromKV(base);
        expect(d2.token).toBe(d1.token);
        expect(d2.user_id).toBe(d1.user_id);
    });

    it('round-trips through forWire', () => {
        const original = DeviceLinkData.parse({
            ...base,
            name: 'My Device',
            max_uses: 2,
            uses: 1,
        });
        const wire = original.forWire();
        expect(typeof wire.token).toBe('string');
        expect(wire.uses).toBe(1);
        expect(wire.max_uses).toBe(2);
    });
});
