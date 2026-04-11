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
import { DeviceRegistrationService } from '@vsod/services/auth/device_registration_service.js';
import { OperatorStatus, OperatorType } from '@vsod/constants/operator.js';
import { OperatorSessionRole, DeviceLinkError } from '@vsod/constants/auth.js';
import { EventType } from '@vsod/constants/events.js';
import { SystemInfo } from '@vsod/models/operator_model.js';
import { getTestServices } from '@test/helpers/test-services.js';
import { TestCleanupHelper } from '@test/helpers/test-cleanup.js';

describe('DeviceRegistrationService', () => {
    let service;
    let services;
    let cleanup;
    let sseService;
    let operatorService;
    let operatorSessionService;
    let userService;
    let sessionAuthListener;

    const mockVsoContext = {
        user_id: 'user-123',
        web_session_id: 'web-session-456',
        organization_id: 'org-789'
    };

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
        
        // Setup real services with spies for verification
        sseService = services.sseService;
        operatorService = services.operatorService;
        operatorSessionService = services.operatorSessionService;
        userService = services.userService;
        sessionAuthListener = services.sessionAuthListener;

        vi.spyOn(sseService, 'publishEvent').mockResolvedValue(true);
        vi.spyOn(operatorService, 'getOperator');
        vi.spyOn(operatorService, 'claimOperatorSlot');
        
        vi.spyOn(operatorSessionService, 'createOperatorSession');
        vi.spyOn(operatorSessionService, 'endSession').mockResolvedValue(true);
        
        vi.spyOn(userService, 'getUser');
        vi.spyOn(userService, 'updateUserOperator').mockResolvedValue(true);
        
        vi.spyOn(sessionAuthListener, 'listen').mockImplementation(() => {});

        cleanup = new TestCleanupHelper(services.cacheAsideService);

        service = new DeviceRegistrationService({
            operatorService,
            operatorSessionService,
            userService,
            sseService,
            internalHttpClient: services.internalHttpClient,
            sessionAuthListener
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
                vsoContext: mockVsoContext
            });
            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.MISSING_FINGERPRINT);
        });

        it('should return failure if operator is not found', async () => {
            operatorService.getOperator.mockResolvedValue(null);
            const result = await service.registerDevice({
                operator_id: 'non-existent',
                deviceInfo: mockDeviceInfo,
                vsoContext: mockVsoContext
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
                vsoContext: mockVsoContext
            });
            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.USER_NOT_FOUND);
        });

        it('should successfully register a device', async () => {
            const operatorId = 'op-1';
            const operatorSessionId = 'op-sess-999';
            const mockUser = {
                id: mockVsoContext.user_id,
                email: 'test@example.com',
                name: 'Test User',
                organization_id: mockVsoContext.organization_id,
                roles: [OperatorSessionRole.OPERATOR]
            };

            operatorService.getOperator.mockResolvedValue({
                operator_id: operatorId,
                status: OperatorStatus.AVAILABLE
            });
            userService.getUser.mockResolvedValue(mockUser);
            operatorSessionService.createOperatorSession.mockResolvedValue({ id: operatorSessionId });
            operatorService.claimOperatorSlot.mockResolvedValue(true);

            const result = await service.registerDevice({
                operator_id: operatorId,
                deviceInfo: mockDeviceInfo,
                vsoContext: mockVsoContext
            });

            expect(result.success).toBe(true);
            expect(result.operator_session_id).toBe(operatorSessionId);
            expect(result.system_info).toBeInstanceOf(SystemInfo);
            
            expect(operatorService.claimOperatorSlot).toHaveBeenCalledWith(operatorId, expect.objectContaining({
                operator_session_id: operatorSessionId,
                web_session_id: mockVsoContext.web_session_id
            }));
            
            const claimCall = operatorService.claimOperatorSlot.mock.calls[0][1];
            expect(claimCall.system_info).toBeInstanceOf(SystemInfo);
            expect(claimCall.system_info.system_fingerprint).toBe(mockDeviceInfo.system_fingerprint);
            expect(claimCall.system_info.hostname).toBe(mockDeviceInfo.hostname);
            
            expect(userService.updateUserOperator).toHaveBeenCalledWith(mockVsoContext.user_id, operatorId, OperatorStatus.ACTIVE);
            
            // Verify direct SSE notification
            expect(sseService.publishEvent).toHaveBeenCalledWith(
                mockVsoContext.web_session_id,
                expect.objectContaining({
                    type: EventType.OPERATOR_STATUS_UPDATED_ACTIVE
                })
            );
            
            expect(sessionAuthListener.listen).toHaveBeenCalled();
        });

        it('should relay VSOHttpContext with bound_operators to VSE', async () => {
            const operatorId = 'op-relay';
            const operatorSessionId = 'op-sess-relay';
            const mockUser = {
                id: mockVsoContext.user_id,
                email: 'test@example.com',
                organization_id: mockVsoContext.organization_id,
                roles: [OperatorSessionRole.OPERATOR],
            };

            operatorService.getOperator.mockResolvedValue({
                operator_id: operatorId,
                status: OperatorStatus.AVAILABLE,
            });
            userService.getUser.mockResolvedValue(mockUser);
            operatorSessionService.createOperatorSession.mockResolvedValue({ id: operatorSessionId });
            operatorService.claimOperatorSlot.mockResolvedValue(true);
            vi.spyOn(operatorService, 'relayRegisterOperatorSessionToVse').mockResolvedValue(true);

            await service.registerDevice({
                operator_id: operatorId,
                deviceInfo: mockDeviceInfo,
                vsoContext: mockVsoContext,
            });

            expect(operatorService.relayRegisterOperatorSessionToVse).toHaveBeenCalledTimes(1);
            const relayArg = operatorService.relayRegisterOperatorSessionToVse.mock.calls[0][0];

            expect(relayArg.user_id).toBe(mockVsoContext.user_id);
            expect(relayArg.bound_operators).toHaveLength(1);
            expect(relayArg.bound_operators[0].operator_id).toBe(operatorId);
            expect(relayArg.bound_operators[0].operator_session_id).toBe(operatorSessionId);
            expect(relayArg.bound_operators[0].status).toBe(OperatorStatus.ACTIVE);

            const listenerArg = sessionAuthListener.listen.mock.calls[0][0];
            expect(listenerArg.operator_id).toBe(operatorId);
            expect(listenerArg.operator_session_id).toBe(operatorSessionId);
            expect(listenerArg.user_id).toBe(mockVsoContext.user_id);
        });

        it('should rollback session if claiming slot fails', async () => {
            const operatorId = 'op-1';
            const operatorSessionId = 'op-sess-999';
            operatorService.getOperator.mockResolvedValue({ status: OperatorStatus.AVAILABLE });
            userService.getUser.mockResolvedValue({ id: 'u1' });
            operatorSessionService.createOperatorSession.mockResolvedValue({ id: operatorSessionId });
            operatorService.claimOperatorSlot.mockResolvedValue(false);

            const result = await service.registerDevice({
                operator_id: operatorId,
                deviceInfo: mockDeviceInfo,
                vsoContext: mockVsoContext
            });

            expect(result.success).toBe(false);
            expect(result.error).toBe(DeviceLinkError.CLAIM_SLOT_FAILED);
            expect(operatorSessionService.endSession).toHaveBeenCalledWith(operatorSessionId);
        });
    });
});
