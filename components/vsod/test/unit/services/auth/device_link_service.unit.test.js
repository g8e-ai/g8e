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

import { describe, it, expect, vi, beforeEach } from 'vitest';
import crypto from 'crypto';
import { DeviceLinkService } from '@vsod/services/auth/device_link_service.js';
import { createMockCacheAside } from '@test/mocks/cache-aside.mock.js';
import { DeviceLinkData } from '@vsod/models/auth_models.js';
import { DeviceLinkStatus, DeviceLinkError, DEVICE_LINK_TTL_SECONDS, DEVICE_LINK_TTL_MIN_SECONDS, DEVICE_LINK_TTL_MAX_SECONDS, LOCK_TTL_MS, LOCK_RETRY_DELAY_MS, LOCK_MAX_RETRIES, DEFAULT_DEVICE_LINK_MAX_USES, DEVICE_LINK_MAX_USES_MIN, DEVICE_LINK_MAX_USES_MAX } from '@vsod/constants/auth.js';
import { OperatorStatus, OperatorType } from '@vsod/constants/operator.js';
import { Collections } from '@vsod/constants/collections.js';
import { KVKey } from '@vsod/constants/kv_keys.js';
import { now, addSeconds } from '@vsod/models/base.js';

describe('DeviceLinkService', () => {
    let service;
    let mockCache;
    let mockOperatorService;
    let mockWebSessionService;
    let mockDeviceRegistration;

    const mockVsoContext = {
        user_id: 'user-123',
        organization_id: 'org-456',
        operator_id: 'op-789',
        web_session_id: 'web-sess-abc'
    };

    const mockDeviceInfo = {
        system_fingerprint: 'abc123def456',
        hostname: 'host-1',
        os: 'linux',
        arch: 'x64',
        username: 'admin'
    };

    const validToken = 'dlk_' + 'a'.repeat(32);
    const invalidToken = 'invalid_token_format';

    beforeEach(() => {
        mockCache = createMockCacheAside();
        mockOperatorService = {
            getOperator: vi.fn(),
            createOperatorSlot: vi.fn(),
            collectionName: Collections.OPERATORS
        };
        mockWebSessionService = {
            getUserActiveSession: vi.fn()
        };
        mockDeviceRegistration = {
            registerDevice: vi.fn()
        };

        service = new DeviceLinkService({
            cacheAsideService: mockCache,
            operatorService: mockOperatorService,
            webSessionService: mockWebSessionService,
            deviceRegistrationService: mockDeviceRegistration
        });
    });

    describe('generateLink', () => {
        it('should successfully generate a single-operator pending link', async () => {
            mockOperatorService.getOperator.mockResolvedValue({
                user_id: mockVsoContext.user_id,
                status: OperatorStatus.AVAILABLE
            });

            const result = await service.generateLink(mockVsoContext);

            expect(result.success).toBe(true);
            expect(result.token).toMatch(/^dlk_/);
            expect(result.operator_command).toContain(result.token);
            
            const key = KVKey.deviceLink(result.token);
            expect(mockCache.kvSetJson).toHaveBeenCalledWith(
                key,
                expect.objectContaining({
                    status: DeviceLinkStatus.PENDING,
                    operator_id: mockVsoContext.operator_id
                }),
                expect.any(Number)
            );
        });

        it('should fail if operator is not found', async () => {
            mockOperatorService.getOperator.mockResolvedValue(null);
            const result = await service.generateLink(mockVsoContext);
            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.OPERATOR_NOT_FOUND);
        });

        it('should fail if operator belongs to different user', async () => {
            mockOperatorService.getOperator.mockResolvedValue({
                user_id: 'other-user',
                status: OperatorStatus.AVAILABLE
            });

            const result = await service.generateLink(mockVsoContext);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.OPERATOR_WRONG_USER);
        });

        it('should fail if operator is terminated', async () => {
            mockOperatorService.getOperator.mockResolvedValue({
                user_id: mockVsoContext.user_id,
                status: OperatorStatus.TERMINATED
            });

            const result = await service.generateLink(mockVsoContext);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.OPERATOR_TERMINATED);
        });
    });

    describe('createLink', () => {
        it('should successfully create a multi-use active link', async () => {
            const result = await service.createLink({
                user_id: mockVsoContext.user_id,
                organization_id: mockVsoContext.organization_id,
                name: 'Test Link',
                max_uses: 5,
                ttl_seconds: DEVICE_LINK_TTL_SECONDS
            });

            expect(result.success).toBe(true);
            expect(result.token).toMatch(/^dlk_/);
            expect(result.name).toBe('Test Link');
            expect(result.max_uses).toBe(5);
            expect(result.operator_command).toContain(result.token);
            expect(result.expires_at).toBeDefined();
        });

        it('should create link without name', async () => {
            const result = await service.createLink({
                user_id: mockVsoContext.user_id,
                organization_id: mockVsoContext.organization_id,
                max_uses: DEFAULT_DEVICE_LINK_MAX_USES
            });

            expect(result.success).toBe(true);
            expect(result.name).toBeNull();
        });

        it('should fail if max_uses is below minimum', async () => {
            const result = await service.createLink({
                user_id: mockVsoContext.user_id,
                organization_id: mockVsoContext.organization_id,
                max_uses: 0
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.MAX_USES_INVALID);
        });

        it('should fail if max_uses exceeds maximum', async () => {
            const result = await service.createLink({
                user_id: mockVsoContext.user_id,
                organization_id: mockVsoContext.organization_id,
                max_uses: 10001
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.MAX_USES_INVALID);
        });

        it('should fail if ttl_seconds is below minimum', async () => {
            const result = await service.createLink({
                user_id: mockVsoContext.user_id,
                organization_id: mockVsoContext.organization_id,
                ttl_seconds: DEVICE_LINK_TTL_MIN_SECONDS - 1
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.TTL_INVALID);
        });

        it('should fail if ttl_seconds exceeds maximum', async () => {
            const result = await service.createLink({
                user_id: mockVsoContext.user_id,
                organization_id: mockVsoContext.organization_id,
                ttl_seconds: DEVICE_LINK_TTL_MAX_SECONDS + 1
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.TTL_INVALID);
        });
    });

    describe('getLink', () => {
        it('should successfully retrieve a valid link', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.getLink(validToken);

            expect(result.success).toBe(true);
            expect(result.data.token).toBe(validToken);
            expect(result.data.status).toBe(DeviceLinkStatus.ACTIVE);
        });

        it('should fail if token format is invalid', async () => {
            const result = await service.getLink(invalidToken);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.INVALID_TOKEN_FORMAT);
        });

        it('should fail if link is not found', async () => {
            mockCache.kvGetJson.mockResolvedValue(null);

            const result = await service.getLink(validToken);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_NOT_FOUND);
        });

        it('should fail if link is expired', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), -3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.getLink(validToken);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_EXPIRED);
        });
    });

    describe('registerDevice', () => {
        it('should successfully register a device via PENDING single-operator link', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: mockVsoContext.user_id,
                operator_id: mockVsoContext.operator_id,
                status: DeviceLinkStatus.PENDING,
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(token), linkData.forKV());
            mockCache.kvTtl.mockResolvedValue(3600);
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: mockVsoContext.operator_id,
                operator_session_id: 'op-sess-1'
            });

            const result = await service.registerDevice(token, mockDeviceInfo);

            expect(result.success).toBe(true);
            expect(mockDeviceRegistration.registerDevice).toHaveBeenCalled();
            
            const updated = mockCache._getKV(KVKey.deviceLink(token));
            expect(updated.status).toBe(DeviceLinkStatus.USED);
        });

        it('should successfully register via ACTIVE multi-use link', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: mockVsoContext.user_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(token), linkData.forKV());
            
            mockCache.kvSadd.mockResolvedValue(1); 
            mockCache.kvIncr.mockResolvedValue(1);
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(`${token}:abc123def456:123456`); 
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            
            mockCache.queryDocuments.mockResolvedValue([
                { operator_id: 'op-avail', status: OperatorStatus.AVAILABLE }
            ]);
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: 'op-avail',
                operator_session_id: 'op-sess-1'
            });

            const result = await service.registerDevice(token, mockDeviceInfo);

            expect(result.success).toBe(true);
            expect(mockCache.kvSetJson).toHaveBeenCalledWith(
                KVKey.deviceLink(token),
                expect.objectContaining({ uses: 1 }),
                expect.any(Number)
            );
        });

        it('should fail if token format is invalid', async () => {
            const result = await service.registerDevice(invalidToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.INVALID_TOKEN_FORMAT);
        });

        it('should fail if system_fingerprint is missing', async () => {
            const deviceInfoNoFingerprint = { ...mockDeviceInfo, system_fingerprint: undefined };
            const result = await service.registerDevice(validToken, deviceInfoNoFingerprint);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.MISSING_FINGERPRINT);
        });

        it('should fail if system_fingerprint is invalid (empty after sanitization)', async () => {
            const deviceInfoInvalidFingerprint = { ...mockDeviceInfo, system_fingerprint: '!!!' };
            const result = await service.registerDevice(validToken, deviceInfoInvalidFingerprint);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.INVALID_FINGERPRINT);
        });

        it('should fail if link is not found', async () => {
            mockCache.kvGetJson.mockResolvedValue(null);

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_NOT_FOUND);
        });

        it('should fail if link is expired', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), -3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_EXPIRED);
        });

        it('should fail if link is revoked', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.REVOKED,
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_REVOKED);
        });

        it('should fail if link is already used (single-operator)', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.USED,
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_ALREADY_USED);
        });

        it('should fail if link has invalid status', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: 'INVALID_STATUS',
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe('Device link is INVALID_STATUS');
        });

        it('should fail if device fingerprint already registered', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(0); 

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.DEVICE_ALREADY_REGISTERED);
        });

        it('should fail if link is exhausted', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                max_uses: 1,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(1); 
            mockCache.kvIncr.mockResolvedValue(2); 

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_EXHAUSTED);
            expect(mockCache.kvDecr).toHaveBeenCalled();
            expect(mockCache.kvSrem).toHaveBeenCalled();
        });

        it('should fail if registration lock cannot be acquired', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvIncr.mockResolvedValue(1);
            mockCache.kvSet.mockResolvedValue(null); 

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.REGISTRATION_BUSY);
            expect(mockCache.kvDecr).toHaveBeenCalled();
            expect(mockCache.kvSrem).toHaveBeenCalled();
        });

        it('should create new operator slot when none available', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                organization_id: mockVsoContext.organization_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvIncr.mockResolvedValue(1);
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            
            mockCache.queryDocuments.mockResolvedValue([
                { operator_id: 'op-1', status: OperatorStatus.ACTIVE }
            ]);
            mockOperatorService.createOperatorSlot.mockResolvedValue('new-op-id');
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: 'new-op-id',
                operator_session_id: 'op-sess-1'
            });

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(true);
            expect(mockOperatorService.createOperatorSlot).toHaveBeenCalledWith(
                expect.objectContaining({
                    userId: mockVsoContext.user_id,
                    organizationId: mockVsoContext.organization_id,
                    operatorType: OperatorType.SYSTEM
                })
            );
        });

        it('should fail if operator slot creation fails', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvIncr.mockResolvedValue(1);
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            
            mockCache.queryDocuments.mockResolvedValue([
                { operator_id: 'op-1', status: OperatorStatus.ACTIVE }
            ]);
            mockOperatorService.createOperatorSlot.mockResolvedValue(null);

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.SLOT_CREATE_FAILED);
            expect(mockCache.kvDecr).toHaveBeenCalled();
            expect(mockCache.kvSrem).toHaveBeenCalled();
        });

        it('should fail if device registration fails', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvIncr.mockResolvedValue(1);
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            
            mockCache.queryDocuments.mockResolvedValue([
                { operator_id: 'op-avail', status: OperatorStatus.AVAILABLE }
            ]);
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: false,
                error: 'Registration failed'
            });

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe('Registration failed');
            expect(mockCache.kvDecr).toHaveBeenCalled();
            expect(mockCache.kvSrem).toHaveBeenCalled();
        });

        it('should set status to EXHAUSTED when max uses reached', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                max_uses: 2,
                uses: 1,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: [{ 
                    system_fingerprint: 'abc123def456',
                    operator_id: 'op-1', 
                    claimed_at: now() 
                }]
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvIncr.mockResolvedValue(2);
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            
            mockCache.queryDocuments.mockResolvedValue([
                { operator_id: 'op-avail', status: OperatorStatus.AVAILABLE }
            ]);
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: 'op-avail',
                operator_session_id: 'op-sess-2'
            });

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(true);
            const updated = mockCache._getKV(KVKey.deviceLink(validToken));
            expect(updated.status).toBe(DeviceLinkStatus.EXHAUSTED);
        });
    });

    describe('cancelLink', () => {
        it('should successfully delete a link if owned by user', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: 'user-123',
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600)
            });
            mockCache._seedKV(KVKey.deviceLink(token), linkData.forKV());

            const result = await service.cancelLink(token, 'user-123');

            expect(result.success).toBe(true);
            expect(mockCache.kvDel).toHaveBeenCalledWith(KVKey.deviceLink(token));
            expect(mockCache.kvSrem).toHaveBeenCalledWith(KVKey.deviceLinkList('user-123'), token);
        });

        it('should succeed if link does not exist', async () => {
            mockCache.kvGetJson.mockResolvedValue(null);

            const result = await service.cancelLink(validToken, 'user-123');

            expect(result.success).toBe(true);
        });

        it('should fail if user does not own the link', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: 'other-user',
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600)
            });
            mockCache._seedKV(KVKey.deviceLink(token), linkData.forKV());

            const result = await service.cancelLink(token, 'user-123');

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.UNAUTHORIZED);
        });
    });

    describe('listLinks', () => {
        it('should successfully list all links for a user', async () => {
            const token1 = 'dlk_' + 'a'.repeat(32);
            const token2 = 'dlk_' + 'b'.repeat(32);

            const linkData1 = new DeviceLinkData({
                token: token1,
                user_id: mockVsoContext.user_id,
                name: 'Link 1',
                max_uses: 5,
                uses: 2,
                status: DeviceLinkStatus.ACTIVE,
                created_at: addSeconds(now(), -7200),
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            const linkData2 = new DeviceLinkData({
                token: token2,
                user_id: mockVsoContext.user_id,
                name: 'Link 2',
                max_uses: 3,
                uses: 1,
                status: DeviceLinkStatus.ACTIVE,
                created_at: addSeconds(now(), -3600),
                expires_at: addSeconds(now(), 7200),
                claims: []
            });

            mockCache.kvSmembers.mockResolvedValue([token1, token2]);
            mockCache.kvGetJson
                .mockResolvedValueOnce(linkData1.forKV())
                .mockResolvedValueOnce(linkData2.forKV());

            const result = await service.listLinks(mockVsoContext.user_id);

            expect(result.success).toBe(true);
            expect(result.links).toHaveLength(2);
            expect(result.links[0].token).toBe(token2);
            expect(result.links[1].token).toBe(token1);
            expect(result.links[0].name).toBe('Link 2');
            expect(result.links[1].name).toBe('Link 1');
        });

        it('should mark expired links as EXPIRED', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), -3600),
                claims: []
            });

            mockCache.kvSmembers.mockResolvedValue([token]);
            mockCache.kvGetJson.mockResolvedValue(linkData.forKV());

            const result = await service.listLinks(mockVsoContext.user_id);

            expect(result.success).toBe(true);
            expect(result.links[0].status).toBe(DeviceLinkStatus.EXPIRED);
        });

        it('should remove expired tokens from the list', async () => {
            const token1 = validToken;
            const token2 = 'dlk_' + 'b'.repeat(32);

            const linkData = new DeviceLinkData({
                token: token1,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache.kvSmembers.mockResolvedValue([token1, token2]);
            mockCache.kvGetJson
                .mockResolvedValueOnce(linkData.forKV())
                .mockResolvedValueOnce(null);

            const result = await service.listLinks(mockVsoContext.user_id);

            expect(result.success).toBe(true);
            expect(result.links).toHaveLength(1);
            expect(mockCache.kvSrem).toHaveBeenCalledWith(KVKey.deviceLinkList(mockVsoContext.user_id), token2);
        });

        it('should return empty list if user has no links', async () => {
            mockCache.kvSmembers.mockResolvedValue([]);

            const result = await service.listLinks(mockVsoContext.user_id);

            expect(result.success).toBe(true);
            expect(result.links).toHaveLength(0);
        });

        it('should sort links by created_at descending', async () => {
            const token1 = 'dlk_' + 'a'.repeat(32);
            const token2 = 'dlk_' + 'b'.repeat(32);

            const linkData1 = new DeviceLinkData({
                token: token1,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                created_at: addSeconds(now(), -7200),
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            const linkData2 = new DeviceLinkData({
                token: token2,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                created_at: addSeconds(now(), -3600),
                expires_at: addSeconds(now(), 7200),
                claims: []
            });

            mockCache.kvSmembers.mockResolvedValue([token1, token2]);
            mockCache.kvGetJson
                .mockResolvedValueOnce(linkData1.forKV())
                .mockResolvedValueOnce(linkData2.forKV());

            const result = await service.listLinks(mockVsoContext.user_id);

            expect(result.success).toBe(true);
            expect(result.links[0].token).toBe(token2);
            expect(result.links[1].token).toBe(token1);
        });
    });

    describe('deleteLink', () => {
        it('should successfully delete an inactive or expired link', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.USED,
                expires_at: addSeconds(now(), -3600),
                uses: 1,
                max_uses: 1
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.deleteLink(validToken, mockVsoContext.user_id);

            expect(result.success).toBe(true);
            expect(mockCache.kvDel).toHaveBeenCalledWith(KVKey.deviceLink(validToken));
            expect(mockCache.kvSrem).toHaveBeenCalledWith(KVKey.deviceLinkList(mockVsoContext.user_id), validToken);
        });

        it('should succeed if link does not exist', async () => {
            mockCache.kvGetJson.mockResolvedValue(null);

            const result = await service.deleteLink(validToken, mockVsoContext.user_id);

            expect(result.success).toBe(true);
            expect(mockCache.kvSrem).toHaveBeenCalledWith(KVKey.deviceLinkList(mockVsoContext.user_id), validToken);
        });

        it('should fail if user does not own the link', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: 'other-user',
                status: DeviceLinkStatus.USED,
                expires_at: addSeconds(now(), -3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.deleteLink(validToken, mockVsoContext.user_id);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.UNAUTHORIZED);
        });

        it('should fail if link is active and not expired', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.deleteLink(validToken, mockVsoContext.user_id);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.CANNOT_DELETE_ACTIVE);
        });
    });

    describe('revokeLink', () => {
        it('should successfully revoke a link', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                uses: 1,
                max_uses: 5,
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.revokeLink(validToken, mockVsoContext.user_id);

            expect(result.success).toBe(true);
            expect(mockCache.kvSetJson).toHaveBeenCalledWith(
                KVKey.deviceLink(validToken),
                expect.objectContaining({
                    status: 'revoked',
                    revoked_at: expect.any(String)
                }),
                DEVICE_LINK_TTL_MIN_SECONDS
            );
            expect(mockCache.kvSrem).toHaveBeenCalledWith(KVKey.deviceLinkList(mockVsoContext.user_id), validToken);
        });

        it('should fail if link is not found', async () => {
            const getLinkResult = { success: false, error: DeviceLinkError.LINK_NOT_FOUND };
            vi.spyOn(service, 'getLink').mockResolvedValue(getLinkResult);

            const result = await service.revokeLink(validToken, mockVsoContext.user_id);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_NOT_FOUND);
        });

        it('should fail if user does not own the link', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: 'other-user',
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.revokeLink(validToken, mockVsoContext.user_id);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.UNAUTHORIZED);
        });

        it('should fail if link is expired', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockVsoContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), -3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.revokeLink(validToken, mockVsoContext.user_id);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_EXPIRED);
        });
    });

    describe('isValidTokenFormat', () => {
        it('should return true for valid device link token', async () => {
            const { isValidTokenFormat } = await import('@vsod/services/auth/device_link_service.js');
            expect(isValidTokenFormat(validToken)).toBe(true);
        });

        it('should return false for invalid token format', async () => {
            const { isValidTokenFormat } = await import('@vsod/services/auth/device_link_service.js');
            expect(isValidTokenFormat(invalidToken)).toBe(false);
        });

        it('should return false for null or undefined', async () => {
            const { isValidTokenFormat } = await import('@vsod/services/auth/device_link_service.js');
            expect(isValidTokenFormat(null)).toBe(false);
            expect(isValidTokenFormat(undefined)).toBe(false);
        });

        it('should return false for non-string', async () => {
            const { isValidTokenFormat } = await import('@vsod/services/auth/device_link_service.js');
            expect(isValidTokenFormat(123)).toBe(false);
            expect(isValidTokenFormat({})).toBe(false);
        });
    });

    describe('isValidDownloadTokenFormat', () => {
        it('should return true for valid download token', async () => {
            const { isValidDownloadTokenFormat } = await import('@vsod/services/auth/device_link_service.js');
            const downloadToken = 'dlt_' + 'a'.repeat(32);
            expect(isValidDownloadTokenFormat(downloadToken)).toBe(true);
        });

        it('should return false for invalid download token', async () => {
            const { isValidDownloadTokenFormat } = await import('@vsod/services/auth/device_link_service.js');
            expect(isValidDownloadTokenFormat(invalidToken)).toBe(false);
        });

        it('should return false for null or undefined', async () => {
            const { isValidDownloadTokenFormat } = await import('@vsod/services/auth/device_link_service.js');
            expect(isValidDownloadTokenFormat(null)).toBe(false);
            expect(isValidDownloadTokenFormat(undefined)).toBe(false);
        });
    });
});
