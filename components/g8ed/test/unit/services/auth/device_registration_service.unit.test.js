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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { DeviceRegistrationService } from '@g8ed/services/auth/device_registration_service.js';
import { OperatorStatus, OperatorType } from '@g8ed/constants/operator.js';
import { OperatorSessionRole, DeviceLinkError } from '@g8ed/constants/auth.js';
import { EventType } from '@g8ed/constants/events.js';
import { SystemInfo } from '@g8ed/models/operator_model.js';
import { G8eHttpContext } from '@g8ed/models/request_models.js';
import { getTestServices } from '@test/helpers/test-services.js';
import { TestCleanupHelper } from '@test/helpers/test-cleanup.js';

describe('DeviceRegistrationService', () => {
    let service;
    let services;
    let cleanup;
    let sseService;
    let operatorService;
    let webSessionService;
    let userService;

    const mockG8eContextRaw = {
        user_id: 'user-123',
        web_session_id: 'web-session-456',
        organization_id: 'org-789'
    };
    let mockG8eContext;

    const mockDeviceInfo = {
        system_fingerprint: 'abc123def456',
        hostname: 'test-host',
        os: 'linux',
        arch: 'x64',
        username: 'test-user',
        ip_address: '192.168.1.100'
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        services = await getTestServices();
        mockG8eContext = G8eHttpContext.parse(mockG8eContextRaw);
        
        // Setup real services with spies for verification
        sseService = services.sseService;
        operatorService = services.operatorService;
        webSessionService = services.webSessionService;
        userService = services.userService;

        vi.spyOn(sseService, 'publishEvent').mockResolvedValue(true);
        vi.spyOn(operatorService, 'getOperator');
        vi.spyOn(operatorService, 'claimOperatorSlot');
        
        vi.spyOn(webSessionService, 'createWebSession');
        vi.spyOn(webSessionService, 'endSession').mockResolvedValue(true);
        
        vi.spyOn(userService, 'getUser');
        vi.spyOn(userService, 'updateUserOperator').mockResolvedValue(true);
        
        vi.spyOn(operatorService, 'relayRegisterDeviceLinkToG8ee').mockResolvedValue({
            success: true,
            operator_session_id: 'test-session-id'
        });
        vi.spyOn(operatorService, 'relayListenSessionAuthToG8ee').mockResolvedValue({
            success: true
        });

        cleanup = new TestCleanupHelper(services.cacheAsideService, null, {
            operatorsCollection: services.operatorService.collectionName
        });

        service = new DeviceRegistrationService({
            operatorService,
            userService,
            sseService,
            internalHttpClient: services.internalHttpClient
        });
    });

    afterEach(async () => {
        if (cleanup) {
            await cleanup.cleanup();
        }
    });

    describe('registerDevice', () => {
        it('should return failure if fingerprint is missing', async () => {
            const result = await service.registerDevice({
                operator_id: 'op-1',
                deviceInfo: {},
                g8eContext: mockG8eContext
            });
            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.MISSING_FINGERPRINT);
        });

        it('should return failure if operator is not found', async () => {
            operatorService.getOperator.mockResolvedValue(null);
            const result = await service.registerDevice({
                operator_id: 'non-existent',
                deviceInfo: mockDeviceInfo,
                g8eContext: mockG8eContext
            });
            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.OPERATOR_NOT_FOUND);
        });

        it('should return failure if user is not found', async () => {
            operatorService.getOperator.mockResolvedValue({ id: 'op-1' });
            userService.getUser.mockResolvedValue(null);
            const result = await service.registerDevice({
                operator_id: 'op-1',
                deviceInfo: mockDeviceInfo,
                g8eContext: mockG8eContext
            });
            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.USER_NOT_FOUND);
        });

        it('should successfully register a device', async () => {
            const operatorId = 'op-1';
            const operatorSessionId = 'op-sess-999';
            const mockUser = {
                id: mockG8eContext.user_id,
                email: 'test@example.com',
                name: 'Test User',
                organization_id: mockG8eContext.organization_id,
                roles: [OperatorSessionRole.OPERATOR]
            };

            operatorService.getOperator.mockResolvedValue({
                operator_id: operatorId,
                status: OperatorStatus.AVAILABLE
            });
            userService.getUser.mockResolvedValue(mockUser);
            operatorService.relayRegisterDeviceLinkToG8ee.mockResolvedValue({
                success: true,
                operator_session_id: operatorSessionId
            });

            const result = await service.registerDevice({
                operator_id: operatorId,
                deviceInfo: mockDeviceInfo,
                g8eContext: mockG8eContext
            });

            expect(result.success).toBe(true);
            expect(result.operator_session_id).toBe(operatorSessionId);
            expect(result.system_info).toBeInstanceOf(SystemInfo);
            
            expect(operatorService.relayRegisterDeviceLinkToG8ee).toHaveBeenCalledWith(
                expect.objectContaining({
                    operator_id: operatorId,
                    operator_type: OperatorType.SYSTEM
                }),
                mockG8eContext
            );
            
            expect(operatorService.relayListenSessionAuthToG8ee).toHaveBeenCalledWith(
                expect.objectContaining({
                    operator_session_id: operatorSessionId,
                    operator_id: operatorId,
                    user_id: mockG8eContext.user_id
                }),
                mockG8eContext
            );
            
            // Verify direct SSE notification
            expect(sseService.publishEvent).toHaveBeenCalledWith(
                mockG8eContext.web_session_id,
                expect.objectContaining({
                    type: EventType.OPERATOR_STATUS_UPDATED_ACTIVE
                })
            );
        });

        it('should relay G8eHttpContext with bound_operators to g8ee', async () => {
            const operatorId = 'op-relay';
            const operatorSessionId = 'op-sess-relay';
            const mockUser = {
                id: mockG8eContext.user_id,
                email: 'test@example.com',
                organization_id: mockG8eContext.organization_id,
                roles: [OperatorSessionRole.OPERATOR],
            };

            operatorService.getOperator.mockResolvedValue({
                operator_id: operatorId,
                status: OperatorStatus.AVAILABLE,
            });
            userService.getUser.mockResolvedValue(mockUser);
            operatorService.relayRegisterDeviceLinkToG8ee.mockResolvedValue({
                success: true,
                operator_session_id: operatorSessionId
            });

            await service.registerDevice({
                operator_id: operatorId,
                deviceInfo: mockDeviceInfo,
                g8eContext: mockG8eContext,
            });

            expect(operatorService.relayRegisterDeviceLinkToG8ee).toHaveBeenCalledWith(
                expect.objectContaining({
                    operator_id: operatorId,
                    operator_type: OperatorType.SYSTEM
                }),
                mockG8eContext
            );

            expect(operatorService.relayListenSessionAuthToG8ee).toHaveBeenCalledWith(
                expect.objectContaining({
                    operator_session_id: operatorSessionId,
                    operator_id: operatorId,
                    user_id: mockG8eContext.user_id
                }),
                mockG8eContext
            );
        });

        it('should return failure if g8ee authentication fails', async () => {
            const operatorId = 'op-1';
            operatorService.getOperator.mockResolvedValue({ status: OperatorStatus.AVAILABLE });
            userService.getUser.mockResolvedValue({ id: 'u1' });
            operatorService.relayRegisterDeviceLinkToG8ee.mockResolvedValue({
                success: false,
                error: 'Authentication failed'
            });

            const result = await service.registerDevice({
                operator_id: operatorId,
                deviceInfo: mockDeviceInfo,
                g8eContext: mockG8eContext
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe('Authentication failed');
        });
    });
});
