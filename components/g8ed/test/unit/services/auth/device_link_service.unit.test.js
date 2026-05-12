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
import { DeviceLinkService } from '@g8ed/services/auth/device_link_service.js';
import { createMockCacheAside } from '@test/mocks/cache-aside.mock.js';
import { createMockInternalHttpClient } from '@test/mocks/internal-http-client.mock.js';
import { DeviceLinkData } from '@g8ed/models/auth_models.js';
import { DeviceLinkStatus, DeviceLinkError, DEVICE_LINK_TTL_SECONDS, DEVICE_LINK_TTL_MIN_SECONDS, DEVICE_LINK_TTL_MAX_SECONDS, LOCK_TTL_MS, LOCK_RETRY_DELAY_MS, LOCK_MAX_RETRIES } from '@g8ed/constants/auth.js';
import { OperatorStatus, OperatorType } from '@g8ed/constants/operator.js';
import { Collections } from '@g8ed/constants/collections.js';
import { KVKey } from '@g8ed/constants/kv_keys.js';
import { now, addSeconds, toISOString } from '@g8ed/models/base.js';

describe('DeviceLinkService', () => {
    let service;
    let mockCache;
    let mockOperatorService;
    let mockWebSessionService;
    let mockDeviceRegistration;
    let mockInternalHttpClient;

    const mockG8eContext = {
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
            terminateOperator: vi.fn(),
            queryOperators: vi.fn(),
            queryListedOperators: vi.fn().mockResolvedValue([]),
            collectionName: Collections.OPERATORS
        };
        mockWebSessionService = {
            getUserActiveSession: vi.fn()
        };
        mockDeviceRegistration = {
            registerDevice: vi.fn()
        };
        mockInternalHttpClient = createMockInternalHttpClient();

        service = new DeviceLinkService({
            cacheAsideService: mockCache,
            operatorService: mockOperatorService,
            webSessionService: mockWebSessionService,
            deviceRegistrationService: mockDeviceRegistration,
            internalHttpClient: mockInternalHttpClient
        });
    });

    describe('generateLink', () => {
        it('should successfully generate a single-operator pending link', async () => {
            mockOperatorService.getOperator.mockResolvedValue({
                user_id: mockG8eContext.user_id,
                status: OperatorStatus.OFFLINE
            });

            const result = await service.generateLink(mockG8eContext);

            expect(result.success).toBe(true);
            expect(result.token).toMatch(/^dlk_/);
            expect(result.operator_command).toContain(result.token);
            
            const key = KVKey.deviceLink(result.token);
            expect(mockCache.kvSetJson).toHaveBeenCalledWith(
                key,
                expect.objectContaining({
                    status: DeviceLinkStatus.PENDING,
                    operator_id: mockG8eContext.operator_id
                }),
                expect.any(Number)
            );
        });

        it('should fail if operator is not found', async () => {
            mockOperatorService.getOperator.mockResolvedValue(null);
            const result = await service.generateLink(mockG8eContext);
            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.OPERATOR_NOT_FOUND);
        });

        it('should fail if operator belongs to different user', async () => {
            mockOperatorService.getOperator.mockResolvedValue({
                user_id: 'other-user',
                status: OperatorStatus.OFFLINE
            });

            const result = await service.generateLink(mockG8eContext);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.OPERATOR_WRONG_USER);
        });

        it('should fail if operator is terminated', async () => {
            mockOperatorService.getOperator.mockResolvedValue({
                user_id: mockG8eContext.user_id,
                status: OperatorStatus.TERMINATED
            });

            const result = await service.generateLink(mockG8eContext);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.OPERATOR_TERMINATED);
        });
    });

    describe('createLink', () => {
        it('should successfully create a multi-use active link via substrate', async () => {
            const mockResponse = {
                success: true,
                token: 'dlk_mock_123',
                name: 'Test Link',
                max_uses: 5,
                operator_command: 'g8e.operator --device-token dlk_mock_123',
                expires_at: toISOString(addSeconds(now(), DEVICE_LINK_TTL_SECONDS))
            };
            mockInternalHttpClient.createDeviceLink.mockResolvedValue(mockResponse);

            const result = await service.createLink({
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                name: 'Test Link',
                max_uses: 5,
                ttl_seconds: DEVICE_LINK_TTL_SECONDS
            });

            expect(result.success).toBe(true);
            expect(result.token).toBe('dlk_mock_123');
            expect(mockInternalHttpClient.createDeviceLink).toHaveBeenCalledWith(expect.objectContaining({
                user_id: mockG8eContext.user_id,
                name: 'Test Link',
                max_uses: 5
            }));
        });

        it('should fail if max_uses is below minimum', async () => {
            const result = await service.createLink({
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                max_uses: 0
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.MAX_USES_INVALID);
        });

        it('should fail if max_uses is not provided', async () => {
            const result = await service.createLink({
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.MAX_USES_INVALID);
        });

        it('should fail if max_uses exceeds maximum', async () => {
            const result = await service.createLink({
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                max_uses: 10001
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.MAX_USES_INVALID);
        });

        it('should fail if ttl_seconds is below minimum', async () => {
            const result = await service.createLink({
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                max_uses: 5,
                ttl_seconds: DEVICE_LINK_TTL_MIN_SECONDS - 1
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.TTL_INVALID);
        });

        it('should fail if ttl_seconds exceeds maximum', async () => {
            const result = await service.createLink({
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                max_uses: 5,
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
                user_id: mockG8eContext.user_id,
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
                user_id: mockG8eContext.user_id,
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
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                operator_id: mockG8eContext.operator_id,
                web_session_id: mockG8eContext.web_session_id,
                status: DeviceLinkStatus.PENDING,
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(token), linkData.forKV());
            mockCache.kvTtl.mockResolvedValue(3600);
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: mockG8eContext.operator_id,
                operator_session_id: 'op-sess-1'
            });

            const result = await service.registerDevice(token, mockDeviceInfo);

            expect(result.success).toBe(true);
            expect(mockDeviceRegistration.registerDevice).toHaveBeenCalled();
            
            const updated = mockCache._getKV(KVKey.deviceLink(token));
            expect(updated.status).toBe(DeviceLinkStatus.USED);
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
                user_id: mockG8eContext.user_id,
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
                user_id: mockG8eContext.user_id,
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
                user_id: mockG8eContext.user_id,
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
                user_id: mockG8eContext.user_id,
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
                user_id: mockG8eContext.user_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(0);
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(null);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockOperatorService.queryListedOperators.mockResolvedValue([]);
            mockOperatorService.createOperatorSlot.mockResolvedValue({ success: true, operator_id: 'op-new' });
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.DEVICE_ALREADY_REGISTERED);
        });

        it('should fail if link is exhausted', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockG8eContext.user_id,
                max_uses: 1,
                uses: 1,
                status: DeviceLinkStatus.EXHAUSTED,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.LINK_EXHAUSTED);
        });

        it('should fail if registration lock cannot be acquired', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockG8eContext.user_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSet.mockResolvedValue(null);
            mockCache.kvSadd.mockResolvedValue(1); // Ensure SADD returns 1 to proceed
            mockOperatorService.queryListedOperators.mockResolvedValue([]);
            mockDeviceRegistration.registerDevice.mockResolvedValue({ success: true, operator_id: 'op-1' });

            const setTimeoutMock = vi.fn((fn, delay) => {
                fn();
                return 1;
            });
            vi.stubGlobal('setTimeout', setTimeoutMock);

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            vi.unstubAllGlobals();

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.REGISTRATION_BUSY);
        });



        it('should fail if device registration fails', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockG8eContext.user_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            
            mockOperatorService.queryListedOperators.mockResolvedValue([]);
            mockOperatorService.createOperatorSlot.mockResolvedValue({ success: true, operator_id: 'op-new' });
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: false,
                error: 'Registration failed'
            });

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(false);
            expect(result.error).toBe('Registration failed');
            expect(mockCache.kvSrem).toHaveBeenCalled();
        });

        it('should set status to EXHAUSTED when max uses reached', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockG8eContext.user_id,
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
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            
            mockOperatorService.queryListedOperators.mockResolvedValue([
                { id: 'op-1', status: OperatorStatus.ACTIVE }
            ]);
            mockOperatorService.createOperatorSlot.mockResolvedValue({ success: true, operator_id: 'op-new' });
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: 'op-new',
                operator_session_id: 'op-sess-2'
            });

            const result = await service.registerDevice(validToken, { ...mockDeviceInfo, system_fingerprint: 'new-device-fingerprint' });

            expect(result.success).toBe(true);
            const updated = mockCache._getKV(KVKey.deviceLink(validToken));
            expect(updated.status).toBe(DeviceLinkStatus.EXHAUSTED);
        });

        it('should reuse offline operator slot if one exists', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            
            mockOperatorService.queryListedOperators.mockResolvedValue([
                { id: 'op-offline', status: OperatorStatus.OFFLINE }
            ]);
            
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: 'op-offline',
                operator_session_id: 'op-sess-1'
            });

            const result = await service.registerDevice(token, mockDeviceInfo);

            expect(result.success).toBe(true);
            expect(result.operator_id).toBe('op-offline');
            expect(mockOperatorService.createOperatorSlot).not.toHaveBeenCalled();
            expect(mockDeviceRegistration.registerDevice).toHaveBeenCalledWith(expect.objectContaining({
                operator_id: null
            }));
        });

        it('should reuse existing operator slot if fingerprint matches', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                max_uses: 5,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvSet.mockResolvedValue('OK');
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            
            mockOperatorService.queryListedOperators.mockResolvedValue([
                { id: 'op-matching', status: OperatorStatus.ACTIVE, system_fingerprint: mockDeviceInfo.system_fingerprint }
            ]);
            
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: 'op-matching',
                operator_session_id: 'op-sess-1'
            });

            const result = await service.registerDevice(validToken, mockDeviceInfo);

            expect(result.success).toBe(true);
            expect(result.operator_id).toBe('op-matching');
            expect(mockOperatorService.createOperatorSlot).not.toHaveBeenCalled();
            expect(mockDeviceRegistration.registerDevice).toHaveBeenCalledWith(expect.objectContaining({
                operator_id: null
            }));
        });

        it('should use adaptive retry count based on device link width (max_uses)', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                max_uses: 10,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            
            let lockAttempts = 0;
            mockCache.kvSet.mockImplementation(() => {
                lockAttempts++;
                if (lockAttempts < 28) {
                    return Promise.resolve(null);
                }
                return Promise.resolve('OK');
            });
            
            mockOperatorService.queryListedOperators.mockResolvedValue([]);
            mockOperatorService.createOperatorSlot.mockResolvedValue({ success: true, operator_id: 'op-new' });
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: 'op-new',
                operator_session_id: 'op-sess-1'
            });

            const setTimeoutMock = vi.fn((fn, delay) => {
                fn();
                return 1;
            });
            vi.stubGlobal('setTimeout', setTimeoutMock);

            const result = await service.registerDevice(token, mockDeviceInfo);

            vi.unstubAllGlobals();

            expect(result.success).toBe(true);
            expect(lockAttempts).toBeGreaterThan(LOCK_MAX_RETRIES);
        });

        it('should handle high-concurrency registration with adaptive retries', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: mockG8eContext.user_id,
                organization_id: mockG8eContext.organization_id,
                max_uses: 20,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            
            let lockAttempts = 0;
            mockCache.kvSet.mockImplementation(() => {
                lockAttempts++;
                if (lockAttempts < 50) {
                    return Promise.resolve(null);
                }
                return Promise.resolve('OK');
            });
            
            mockOperatorService.queryListedOperators.mockResolvedValue([]);
            mockOperatorService.createOperatorSlot.mockResolvedValue({ success: true, operator_id: 'op-new' });
            mockCache.kvSadd.mockResolvedValue(1);
            mockCache.kvGet.mockResolvedValue(`${validToken}:abc123def456:123456`);
            mockCache.kvTtl.mockResolvedValue(3600);
            mockCache.kvExpire.mockResolvedValue(1);
            mockWebSessionService.getUserActiveSession.mockResolvedValue('web-sess-1');
            mockDeviceRegistration.registerDevice.mockResolvedValue({
                success: true,
                operator_id: 'op-new',
                operator_session_id: 'op-sess-1'
            });

            const setTimeoutMock = vi.fn((fn, delay) => {
                fn();
                return 1;
            });
            vi.stubGlobal('setTimeout', setTimeoutMock);

            const result = await service.registerDevice(token, mockDeviceInfo);

            vi.unstubAllGlobals();

            expect(result.success).toBe(true);
            expect(lockAttempts).toBeGreaterThan(LOCK_MAX_RETRIES);
        });

        it('should still fail after adaptive max retries if lock cannot be acquired', async () => {
            const token = validToken;
            const linkData = new DeviceLinkData({
                token,
                user_id: mockG8eContext.user_id,
                max_uses: 10,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600),
                claims: []
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());
            mockCache.kvSet.mockResolvedValue(null);
            mockCache.kvSadd.mockResolvedValue(1);
            mockOperatorService.queryListedOperators.mockResolvedValue([]);
            mockDeviceRegistration.registerDevice.mockResolvedValue({ success: true, operator_id: 'op-1' });

            const setTimeoutMock = vi.fn((fn, delay) => {
                fn();
                return 1;
            });
            vi.stubGlobal('setTimeout', setTimeoutMock);

            const result = await service.registerDevice(token, mockDeviceInfo);

            vi.unstubAllGlobals();

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.REGISTRATION_BUSY);
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
        it('should successfully list links via substrate', async () => {
            const mockLinks = [{ token: 'dlk_1' }, { token: 'dlk_2' }];
            mockInternalHttpClient.listDeviceLinks.mockResolvedValue({
                success: true,
                links: mockLinks
            });

            const result = await service.listLinks(mockG8eContext.user_id);

            expect(result.success).toBe(true);
            expect(result.links).toEqual(mockLinks);
            expect(mockInternalHttpClient.listDeviceLinks).toHaveBeenCalledWith(mockG8eContext.user_id);
        });

        it('should handle substrate failure', async () => {
            mockInternalHttpClient.listDeviceLinks.mockResolvedValue({
                success: false,
                error: 'Substrate error'
            });

            const result = await service.listLinks(mockG8eContext.user_id);

            expect(result.success).toBe(false);
            expect(result.error).toBe('Substrate error');
        });
    });

    describe('deleteLink', () => {
        it('should successfully delete an inactive or expired link', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockG8eContext.user_id,
                status: DeviceLinkStatus.USED,
                expires_at: addSeconds(now(), -3600),
                uses: 1,
                max_uses: 1
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.deleteLink(validToken, mockG8eContext.user_id);

            expect(result.success).toBe(true);
            expect(mockCache.kvDel).toHaveBeenCalledWith(KVKey.deviceLink(validToken));
            expect(mockCache.kvSrem).toHaveBeenCalledWith(KVKey.deviceLinkList(mockG8eContext.user_id), validToken);
        });

        it('should succeed if link does not exist', async () => {
            mockCache.kvGetJson.mockResolvedValue(null);

            const result = await service.deleteLink(validToken, mockG8eContext.user_id);

            expect(result.success).toBe(true);
            expect(mockCache.kvSrem).toHaveBeenCalledWith(KVKey.deviceLinkList(mockG8eContext.user_id), validToken);
        });

        it('should fail if user does not own the link', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: 'other-user',
                status: DeviceLinkStatus.USED,
                expires_at: addSeconds(now(), -3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.deleteLink(validToken, mockG8eContext.user_id);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.UNAUTHORIZED);
        });

        it('should fail if link is active and not expired', async () => {
            const linkData = new DeviceLinkData({
                token: validToken,
                user_id: mockG8eContext.user_id,
                status: DeviceLinkStatus.ACTIVE,
                expires_at: addSeconds(now(), 3600)
            });

            mockCache._seedKV(KVKey.deviceLink(validToken), linkData.forKV());

            const result = await service.deleteLink(validToken, mockG8eContext.user_id);

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.CANNOT_DELETE_ACTIVE);
        });
    });

    describe('revokeLink', () => {
        it('should successfully revoke a link via substrate', async () => {
            mockInternalHttpClient.deleteDeviceLink.mockResolvedValue({ success: true });

            const result = await service.revokeLink('dlk_token', mockG8eContext.user_id);

            expect(result.success).toBe(true);
            expect(mockInternalHttpClient.deleteDeviceLink).toHaveBeenCalledWith('dlk_token', mockG8eContext.user_id);
        });

        it('should handle substrate failure', async () => {
            mockInternalHttpClient.deleteDeviceLink.mockRejectedValue(new Error('Network error'));

            const result = await service.revokeLink('dlk_token', mockG8eContext.user_id);

            expect(result.success).toBe(false);
            expect(result.error).toBe('Network error');
        });
    });

    describe('isValidTokenFormat', () => {
        it('should return true for valid device link token', async () => {
            const { isValidTokenFormat } = await import('@g8ed/services/auth/device_link_service.js');
            expect(isValidTokenFormat(validToken)).toBe(true);
        });

        it('should return false for invalid token format', async () => {
            const { isValidTokenFormat } = await import('@g8ed/services/auth/device_link_service.js');
            expect(isValidTokenFormat(invalidToken)).toBe(false);
        });

        it('should return false for null or undefined', async () => {
            const { isValidTokenFormat } = await import('@g8ed/services/auth/device_link_service.js');
            expect(isValidTokenFormat(null)).toBe(false);
            expect(isValidTokenFormat(undefined)).toBe(false);
        });

        it('should return false for non-string', async () => {
            const { isValidTokenFormat } = await import('@g8ed/services/auth/device_link_service.js');
            expect(isValidTokenFormat(123)).toBe(false);
            expect(isValidTokenFormat({})).toBe(false);
        });
    });

    describe('isValidDownloadTokenFormat', () => {
        it('should return true for valid download token', async () => {
            const { isValidDownloadTokenFormat } = await import('@g8ed/services/auth/device_link_service.js');
            const downloadToken = 'dlt_' + 'a'.repeat(32);
            expect(isValidDownloadTokenFormat(downloadToken)).toBe(true);
        });

        it('should return false for invalid download token', async () => {
            const { isValidDownloadTokenFormat } = await import('@g8ed/services/auth/device_link_service.js');
            expect(isValidDownloadTokenFormat(invalidToken)).toBe(false);
        });

        it('should return false for null or undefined', async () => {
            const { isValidDownloadTokenFormat } = await import('@g8ed/services/auth/device_link_service.js');
            expect(isValidDownloadTokenFormat(null)).toBe(false);
            expect(isValidDownloadTokenFormat(undefined)).toBe(false);
        });
    });
});
