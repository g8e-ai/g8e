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
import { DeviceLinkData, DeviceLinkClaim, DownloadTokenData, DownloadAuditEntry } from '@g8ed/models/auth_models.js';
import { DeviceLinkStatus, DownloadEventType } from '@g8ed/constants/auth.js';

const validDeviceLinkInput = {
    token:           'dlnk_abc123def456',
    user_id:         'user-001',
    organization_id: 'org-001',
    status:          DeviceLinkStatus.PENDING,
};

describe('DeviceLinkData [UNIT - PURE LOGIC]', () => {

    describe('parse', () => {
        it('should parse with required fields', () => {
            const model = DeviceLinkData.parse(validDeviceLinkInput);
            expect(model.token).toBe('dlnk_abc123def456');
            expect(model.user_id).toBe('user-001');
            expect(model.status).toBe(DeviceLinkStatus.PENDING);
        });

        it('should default optional fields to null', () => {
            const model = DeviceLinkData.parse(validDeviceLinkInput);
            expect(model.operator_id).toBeNull();
            expect(model.web_session_id).toBeNull();
            expect(model.name).toBeNull();
            expect(model.max_uses).toBeNull();
            expect(model.created_at).toBeNull();
            expect(model.expires_at).toBeNull();
            expect(model.used_at).toBeNull();
            expect(model.revoked_at).toBeNull();
            expect(model.device_info).toBeNull();
        });

        it('should default uses to 0', () => {
            const model = DeviceLinkData.parse(validDeviceLinkInput);
            expect(model.uses).toBe(0);
        });

        it('should default claims to empty array', () => {
            const model = DeviceLinkData.parse(validDeviceLinkInput);
            expect(model.claims).toEqual([]);
        });

        it('should throw when token is missing', () => {
            const { token: _, ...rest } = validDeviceLinkInput;
            expect(() => DeviceLinkData.parse(rest)).toThrow('validation failed');
        });

        it('should throw when user_id is missing', () => {
            const { user_id: _, ...rest } = validDeviceLinkInput;
            expect(() => DeviceLinkData.parse(rest)).toThrow('validation failed');
        });

        it('should throw when status is missing', () => {
            const { status: _, ...rest } = validDeviceLinkInput;
            expect(() => DeviceLinkData.parse(rest)).toThrow('validation failed');
        });

        it('should parse claims as DeviceLinkClaim instances', () => {
            const model = DeviceLinkData.parse({
                ...validDeviceLinkInput,
                claims: [{ system_fingerprint: 'fp-abc123', hostname: 'host-a' }],
            });
            expect(model.claims).toHaveLength(1);
            expect(model.claims[0]).toBeInstanceOf(DeviceLinkClaim);
            expect(model.claims[0].system_fingerprint).toBe('fp-abc123');
        });
    });

    describe('forKV()', () => {
        it('should return a plain object', () => {
            const model = DeviceLinkData.parse(validDeviceLinkInput);
            const result = model.forKV();
            expect(typeof result).toBe('object');
            expect(result).not.toBeNull();
        });

        it('should be identical to forDB()', () => {
            const model = DeviceLinkData.parse(validDeviceLinkInput);
            expect(model.forKV()).toEqual(model.forDB());
        });

        it('should contain the correct field values', () => {
            const model = DeviceLinkData.parse(validDeviceLinkInput);
            const flat = model.forKV();
            expect(flat.token).toBe('dlnk_abc123def456');
            expect(flat.user_id).toBe('user-001');
            expect(flat.status).toBe(DeviceLinkStatus.PENDING);
        });

        it('should serialize nested claims as plain objects', () => {
            const model = DeviceLinkData.parse({
                ...validDeviceLinkInput,
                claims: [{ system_fingerprint: 'fp-xyz', hostname: 'host-b', operator_id: 'op-001' }],
            });
            const flat = model.forKV();
            expect(Array.isArray(flat.claims)).toBe(true);
            expect(flat.claims[0].system_fingerprint).toBe('fp-xyz');
            expect(flat.claims[0].hostname).toBe('host-b');
            expect(flat.claims[0].operator_id).toBe('op-001');
        });

        it('should round-trip through fromKV()', () => {
            const model = DeviceLinkData.parse({
                ...validDeviceLinkInput,
                operator_id: 'op-round-trip',
                uses:        2,
            });
            const restored = DeviceLinkData.fromKV(model.forKV());
            expect(restored).toBeInstanceOf(DeviceLinkData);
            expect(restored.token).toBe(model.token);
            expect(restored.user_id).toBe(model.user_id);
            expect(restored.operator_id).toBe('op-round-trip');
            expect(restored.uses).toBe(2);
        });
    });

    describe('fromKV()', () => {
        it('should parse a plain object into DeviceLinkData', () => {
            const model = DeviceLinkData.parse(validDeviceLinkInput);
            const restored = DeviceLinkData.fromKV(model.forKV());
            expect(restored).toBeInstanceOf(DeviceLinkData);
            expect(restored.token).toBe(model.token);
            expect(restored.user_id).toBe(model.user_id);
            expect(restored.status).toBe(model.status);
        });

        it('should restore claims as DeviceLinkClaim instances after round-trip', () => {
            const model = DeviceLinkData.parse({
                ...validDeviceLinkInput,
                claims: [{ system_fingerprint: 'fp-restored', hostname: 'host-c' }],
            });
            const restored = DeviceLinkData.fromKV(model.forKV());
            expect(restored.claims).toHaveLength(1);
            expect(restored.claims[0]).toBeInstanceOf(DeviceLinkClaim);
            expect(restored.claims[0].system_fingerprint).toBe('fp-restored');
        });
    });
});

describe('DeviceLinkClaim [UNIT - PURE LOGIC]', () => {
    it('should parse with required system_fingerprint', () => {
        const claim = DeviceLinkClaim.parse({ system_fingerprint: 'fp-abc' });
        expect(claim.system_fingerprint).toBe('fp-abc');
    });

    it('should throw when system_fingerprint is missing', () => {
        expect(() => DeviceLinkClaim.parse({})).toThrow('validation failed');
    });

    it('should default optional fields to null', () => {
        const claim = DeviceLinkClaim.parse({ system_fingerprint: 'fp-abc' });
        expect(claim.hostname).toBeNull();
        expect(claim.operator_id).toBeNull();
    });

    it('should preserve hostname and operator_id when provided', () => {
        const claim = DeviceLinkClaim.parse({
            system_fingerprint: 'fp-abc',
            hostname:           'host-a',
            operator_id:        'op-001',
        });
        expect(claim.hostname).toBe('host-a');
        expect(claim.operator_id).toBe('op-001');
    });

    it('should set claimed_at automatically', () => {
        const claim = DeviceLinkClaim.parse({ system_fingerprint: 'fp-abc' });
        expect(claim.claimed_at).toBeDefined();
    });
});

describe('DownloadTokenData [UNIT - PURE LOGIC]', () => {
    describe('parse', () => {
        it('should parse with required user_id', () => {
            const model = DownloadTokenData.parse({ user_id: 'user-001' });
            expect(model.user_id).toBe('user-001');
        });

        it('should default operator_id to null', () => {
            const model = DownloadTokenData.parse({ user_id: 'user-001' });
            expect(model.operator_id).toBeNull();
        });

        it('should preserve operator_id when provided', () => {
            const model = DownloadTokenData.parse({ user_id: 'user-001', operator_id: 'op-001' });
            expect(model.operator_id).toBe('op-001');
        });

        it('should throw when user_id is missing', () => {
            expect(() => DownloadTokenData.parse({})).toThrow('validation failed');
        });

    });

    describe('forKV()', () => {
        it('should return a plain object', () => {
            const model = new DownloadTokenData({ user_id: 'user-001' });
            const result = model.forKV();
            expect(typeof result).toBe('object');
            expect(result).not.toBeNull();
        });

        it('should be identical to forDB()', () => {
            const model = new DownloadTokenData({ user_id: 'user-001', operator_id: 'op-001' });
            expect(model.forKV()).toEqual(model.forDB());
        });

        it('should contain the correct field values', () => {
            const model = new DownloadTokenData({ user_id: 'user-001', operator_id: 'op-abc' });
            const flat = model.forKV();
            expect(flat.user_id).toBe('user-001');
            expect(flat.operator_id).toBe('op-abc');
        });
    });

    describe('fromKV()', () => {
        it('should parse a plain object into DownloadTokenData', () => {
            const model = new DownloadTokenData({ user_id: 'user-001', operator_id: 'op-001' });
            const restored = DownloadTokenData.fromKV(model.forKV());
            expect(restored).toBeInstanceOf(DownloadTokenData);
            expect(restored.user_id).toBe('user-001');
            expect(restored.operator_id).toBe('op-001');
        });

        it('should restore operator_id as null when absent', () => {
            const model = new DownloadTokenData({ user_id: 'user-002' });
            const restored = DownloadTokenData.fromKV(model.forKV());
            expect(restored.operator_id).toBeNull();
        });

        it('should round-trip cleanly through forKV() → fromKV()', () => {
            const model = DownloadTokenData.parse({ user_id: 'user-round', operator_id: 'op-round' });
            const restored = DownloadTokenData.fromKV(model.forKV());
            expect(restored.user_id).toBe(model.user_id);
            expect(restored.operator_id).toBe(model.operator_id);
        });
    });
});

describe('DownloadAuditEntry [UNIT - PURE LOGIC]', () => {
    describe('parse', () => {
        it('should parse with required fields', () => {
            const entry = DownloadAuditEntry.parse({
                event_type:   DownloadEventType.DOWNLOAD_TOKEN_SUCCESS,
                token_prefix: 'dlt_AbCdEf...',
            });
            expect(entry.event_type).toBe(DownloadEventType.DOWNLOAD_TOKEN_SUCCESS);
            expect(entry.token_prefix).toBe('dlt_AbCdEf...');
        });

        it('should default optional fields to null', () => {
            const entry = DownloadAuditEntry.parse({
                event_type:   DownloadEventType.DOWNLOAD_TOKEN_FAILED,
                token_prefix: 'dlt_...',
            });
            expect(entry.ip_address).toBeNull();
            expect(entry.user_agent).toBeNull();
            expect(entry.user_id).toBeNull();
            expect(entry.operator_id).toBeNull();
        });

        it('should set timestamp automatically', () => {
            const entry = DownloadAuditEntry.parse({
                event_type:   DownloadEventType.DOWNLOAD_TOKEN_FAILED,
                token_prefix: 'dlt_...',
            });
            expect(entry.timestamp).toBeInstanceOf(Date);
        });

        it('should throw when event_type is missing', () => {
            expect(() => DownloadAuditEntry.parse({ token_prefix: 'dlt_...' })).toThrow('validation failed');
        });

        it('should throw when token_prefix is missing', () => {
            expect(() => DownloadAuditEntry.parse({
                event_type: DownloadEventType.DOWNLOAD_TOKEN_SUCCESS,
            })).toThrow('validation failed');
        });

        it('should preserve all optional fields when provided', () => {
            const entry = DownloadAuditEntry.parse({
                event_type:   DownloadEventType.DOWNLOAD_TOKEN_SUCCESS,
                token_prefix: 'dlt_AbCdEf...',
                ip_address:   '10.0.0.1',
                user_agent:   'g8e.operator/1.0',
                user_id:      'user-001',
                operator_id:  'op-001',
            });
            expect(entry.ip_address).toBe('10.0.0.1');
            expect(entry.user_agent).toBe('g8e.operator/1.0');
            expect(entry.user_id).toBe('user-001');
            expect(entry.operator_id).toBe('op-001');
        });
    });

    describe('forDB()', () => {
        it('should serialize timestamp as ISO string', () => {
            const entry = DownloadAuditEntry.parse({
                event_type:   DownloadEventType.DOWNLOAD_TOKEN_SUCCESS,
                token_prefix: 'dlt_...',
            });
            const flat = entry.forDB();
            expect(typeof flat.timestamp).toBe('string');
            expect(() => new Date(flat.timestamp)).not.toThrow();
        });

        it('should use DownloadEventType constants — not raw strings', () => {
            const entry = DownloadAuditEntry.parse({
                event_type:   DownloadEventType.DOWNLOAD_TOKEN_FAILED,
                token_prefix: 'dlt_...',
            });
            const flat = entry.forDB();
            expect(flat.event_type).toBe(DownloadEventType.DOWNLOAD_TOKEN_FAILED);
        });
    });
});
