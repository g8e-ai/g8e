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
import { OperatorAuthService } from '@vsod/services/operator/operator_auth_service.js';
import { AuthMode, AuthError, ApiKeyError, OperatorAuthError, DeviceLinkError, BEARER_PREFIX, OperatorSessionRole } from '@vsod/constants/auth.js';
import { OperatorStatus, OperatorType, CloudOperatorSubtype, operatorStatusToEventType } from '@vsod/constants/operator.js';
import { OperatorAuthResponse } from '@vsod/models/response_models.js';
import { SessionType } from '@vsod/constants/session.js';
import { OperatorDocument, SystemInfo } from '@vsod/models/operator_model.js';
import { OperatorStatusUpdatedEvent, OperatorStatusUpdatedData } from '@vsod/models/sse_models.js';
import { mockOperators, mockSystemInfo } from '@test/fixtures/operators.fixture.js';
import { HTTP_COOKIE_HEADER } from '../../../../constants/headers';

describe('OperatorAuthService', () => {
    let service;
    let mocks;

    beforeEach(() => {
        mocks = {
            apiKeyService: {
                validateKey: vi.fn(),
                recordUsage: vi.fn(),
                revokeKey: vi.fn(),
                issueKey: vi.fn(),
            },
            userService: {
                getUser: vi.fn(),
                updateUserOperator: vi.fn(),
            },
            operatorService: {
                getOperator: vi.fn(),
                getOperatorWithSessionContext: vi.fn(),
                relayRegisterOperatorSessionToG8ee: vi.fn(),
                claimOperatorSlot: vi.fn().mockResolvedValue(true),
                emit: vi.fn(),
            },
            operatorSessionService: {
                validateSession: vi.fn(),
                createOperatorSession: vi.fn(),
                endSession: vi.fn(),
            },
            bindingService: {
                bind: vi.fn(),
            },
            webSessionService: {
                getUserActiveSessions: vi.fn(),
            },
        };

        service = new OperatorAuthService(mocks);
    });

    describe('authenticateOperator', () => {
        it('should route to device link auth when auth_mode is OPERATOR_SESSION', async () => {
            const body = {
                auth_mode: AuthMode.OPERATOR_SESSION,
                operator_session_id: 'os-123',
                system_info: { hostname: 'test-host' },
            };
            const spy = vi.spyOn(service, '_authenticateViaDeviceLink').mockResolvedValue({ success: true });

            await service.authenticateOperator({ authorizationHeader: 'Bearer key', body });

            expect(spy).toHaveBeenCalledWith('os-123', body.system_info, 'Bearer key');
        });

        it('should route to api key auth by default', async () => {
            const body = {
                system_info: { hostname: 'test-host' },
                runtime_config: {},
            };
            const spy = vi.spyOn(service, '_authenticateViaApiKey').mockResolvedValue({ success: true });

            await service.authenticateOperator({ authorizationHeader: 'Bearer key', body });

            expect(spy).toHaveBeenCalledWith({
                authorizationHeader: 'Bearer key',
                system_info: body.system_info,
                runtime_config: body.runtime_config,
            });
        });
    });

    describe('_authenticateViaDeviceLink', () => {
        it('should fail if session is invalid', async () => {
            mocks.operatorSessionService.validateSession.mockResolvedValue(null);

            const result = await service._authenticateViaDeviceLink('invalid-session', {}, null);

            expect(result).toMatchObject({
                success: false,
                statusCode: 401,
                error: AuthError.INVALID_OR_EXPIRED_SESSION,
            });
        });

        it('should fail if user_id is missing from session', async () => {
            mocks.operatorSessionService.validateSession.mockResolvedValue({ operator_id: 'op-123' });

            const result = await service._authenticateViaDeviceLink('valid-session', {}, null);

            expect(result).toMatchObject({
                success: false,
                statusCode: 401,
                error: 'Invalid session',
            });
        });

        it('should fail if user not found', async () => {
            mocks.operatorSessionService.validateSession.mockResolvedValue({ user_id: 'u-123' });
            mocks.userService.getUser.mockResolvedValue(null);

            const result = await service._authenticateViaDeviceLink('valid-session', {}, null);

            expect(result).toMatchObject({
                success: false,
                statusCode: 404,
                error: ApiKeyError.USER_NOT_FOUND,
            });
        });

        it('should return bootstrap config on success', async () => {
            const session = { user_id: 'u-123', operator_id: 'op-123' };
            const user = { id: 'u-123', email: 'test@example.com' };
            mocks.operatorSessionService.validateSession.mockResolvedValue(session);
            mocks.userService.getUser.mockResolvedValue(user);
            mocks.operatorService.getOperator.mockResolvedValue({ api_key: 'op-api-key' });

            const result = await service._authenticateViaDeviceLink('valid-session', {}, 'Bearer header-key');

            expect(result.success).toBe(true);
            expect(result.response).toBeInstanceOf(OperatorAuthResponse);
            expect(result.response.api_key).toBe('header-key');
        });

        it('should fallback to operatorDoc api_key if no bearer token provided', async () => {
            const session = { user_id: 'u-123', operator_id: 'op-123' };
            const user = { id: 'u-123', email: 'test@example.com' };
            mocks.operatorSessionService.validateSession.mockResolvedValue(session);
            mocks.userService.getUser.mockResolvedValue(user);
            mocks.operatorService.getOperator.mockResolvedValue({ api_key: 'stored-api-key' });

            const result = await service._authenticateViaDeviceLink('valid-session', {}, null);

            expect(result.success).toBe(true);
            expect(result.response.api_key).toBe('stored-api-key');
        });
    });

    describe('_authenticateViaApiKey', () => {
        it('should fail if user not found for api key', async () => {
            mocks.apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: 'u-123', operator_id: 'op-123' }
            });
            mocks.userService.getUser.mockResolvedValue(null);

            const result = await service._authenticateViaApiKey({
                authorizationHeader: 'Bearer key',
                system_info: { system_fingerprint: 'fp-123' }
            });

            expect(result).toMatchObject({
                success: false,
                statusCode: 404,
                error: ApiKeyError.USER_NOT_FOUND,
            });
        });

        it('should fail if operator not found for api key', async () => {
            mocks.apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: 'u-123', operator_id: 'op-123' }
            });
            mocks.userService.getUser.mockResolvedValue({ id: 'u-123' });
            mocks.operatorService.getOperator.mockResolvedValue(null);

            const result = await service._authenticateViaApiKey({
                authorizationHeader: 'Bearer key',
                system_info: { system_fingerprint: 'fp-123' }
            });

            expect(result).toMatchObject({
                success: false,
                statusCode: 404,
                error: DeviceLinkError.OPERATOR_NOT_FOUND,
            });
        });

        it('should record usage and relay registration to g8ee on success', async () => {
            const operatorId = 'op-123';
            const userId = 'u-123';
            const organizationId = 'org-123';
            const apiKey = 'valid-key';
            const systemInfo = { system_fingerprint: 'fp-123' };

            mocks.apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: userId, organization_id: organizationId, operator_id: operatorId }
            });
            mocks.userService.getUser.mockResolvedValue({ 
                id: userId,
                email: 'test@example.com',
                name: 'Test User'
            });
            mocks.operatorService.getOperator.mockResolvedValue({ 
                operator_id: operatorId, 
                user_id: userId,
                web_session_id: 'ws-123',
                operator_type: 'system',
            });
            mocks.operatorSessionService.createOperatorSession.mockResolvedValue({ 
                id: 'os-123',
                expires_at: new Date(),
                created_at: new Date()
            });

            const result = await service._authenticateViaApiKey({
                authorizationHeader: `${BEARER_PREFIX}${apiKey}`,
                system_info: systemInfo
            });

            expect(result.success).toBe(true);
            expect(mocks.apiKeyService.recordUsage).toHaveBeenCalledWith(apiKey);

            expect(mocks.operatorService.claimOperatorSlot).toHaveBeenCalledWith(operatorId, expect.objectContaining({
                operator_session_id: 'os-123',
                web_session_id: 'ws-123',
            }));

            expect(mocks.userService.updateUserOperator).toHaveBeenCalledWith(
                userId, operatorId, OperatorStatus.ACTIVE
            );

            expect(mocks.operatorService.relayRegisterOperatorSessionToG8ee).toHaveBeenCalledWith(
                expect.objectContaining({
                    user_id: userId,
                    organization_id: organizationId,
                    bound_operators: expect.arrayContaining([
                        expect.objectContaining({
                            operator_id: operatorId,
                            operator_session_id: 'os-123',
                            status: OperatorStatus.ACTIVE,
                        })
                    ]),
                })
            );
            
            expect(result.response).toBeInstanceOf(OperatorAuthResponse);
            expect(result.response.operator_session_id).toBe('os-123');
        });

        it('should pass VSOHttpContext with BoundOperatorContext containing SystemInfo to g8ee relay', async () => {
            const operatorId = 'op-456';
            const userId = 'u-456';
            const rawSystemInfo = { system_fingerprint: 'fp-456', hostname: 'remote-host', os: 'linux' };

            mocks.apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: userId, organization_id: 'org-456', operator_id: operatorId }
            });
            mocks.userService.getUser.mockResolvedValue({ id: userId });
            mocks.operatorService.getOperator.mockResolvedValue({
                operator_id: operatorId,
                user_id: userId,
                web_session_id: 'ws-456',
                operator_type: 'cloud',
            });
            mocks.operatorSessionService.createOperatorSession.mockResolvedValue({ id: 'os-456' });

            await service._authenticateViaApiKey({
                authorizationHeader: `${BEARER_PREFIX}test-key`,
                system_info: rawSystemInfo,
            });

            const relayArg = mocks.operatorService.relayRegisterOperatorSessionToG8ee.mock.calls[0][0];
            expect(relayArg.bound_operators).toHaveLength(1);

            const boundOp = relayArg.bound_operators[0];
            expect(boundOp.operator_id).toBe(operatorId);
            expect(boundOp.operator_session_id).toBe('os-456');
            expect(boundOp.status).toBe(OperatorStatus.ACTIVE);
            expect(boundOp.operator_type).toBeUndefined();
            expect(boundOp.system_info).toBeUndefined();

            const claimArgs = mocks.operatorService.claimOperatorSlot.mock.calls[0];
            expect(claimArgs[0]).toBe(operatorId);
            expect(claimArgs[1].system_info).toBeInstanceOf(SystemInfo);
            expect(claimArgs[1].operator_type).toBe('cloud');
        });

        it('should use operator_session_id as web_session_id fallback when operator has no web session', async () => {
            mocks.apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: 'u-789', organization_id: null, operator_id: 'op-789' }
            });
            mocks.userService.getUser.mockResolvedValue({ id: 'u-789' });
            mocks.operatorService.getOperator.mockResolvedValue({
                operator_id: 'op-789',
                user_id: 'u-789',
                web_session_id: null,
            });
            mocks.operatorSessionService.createOperatorSession.mockResolvedValue({ id: 'os-789' });

            await service._authenticateViaApiKey({
                authorizationHeader: 'Bearer key',
                system_info: { system_fingerprint: 'fp-789' },
            });

            const relayArg = mocks.operatorService.relayRegisterOperatorSessionToG8ee.mock.calls[0][0];
            expect(relayArg.web_session_id).toBe('os-789');
        });

        it('should preserve BOUND status when operator is already bound', async () => {
            const operatorId = 'op-bound-1';
            const userId = 'u-bound-1';
            const apiKey = 'bound-key';

            mocks.apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: userId, organization_id: 'org-1', operator_id: operatorId }
            });
            mocks.userService.getUser.mockResolvedValue({ id: userId, email: 'b@test.com' });
            mocks.operatorService.getOperator.mockResolvedValue({
                operator_id: operatorId,
                user_id: userId,
                web_session_id: 'ws-bound',
                status: OperatorStatus.BOUND,
                operator_type: 'system',
            });
            mocks.operatorSessionService.createOperatorSession.mockResolvedValue({
                id: 'os-bound-1',
                expires_at: new Date(),
                created_at: new Date(),
            });

            const result = await service._authenticateViaApiKey({
                authorizationHeader: `${BEARER_PREFIX}${apiKey}`,
                system_info: { hostname: 'bound-host' },
            });

            expect(result.success).toBe(true);

            expect(mocks.operatorService.claimOperatorSlot).toHaveBeenCalledWith(
                operatorId,
                expect.objectContaining({ status: OperatorStatus.BOUND })
            );

            expect(mocks.userService.updateUserOperator).toHaveBeenCalledWith(
                userId, operatorId, OperatorStatus.BOUND
            );

            const relayArg = mocks.operatorService.relayRegisterOperatorSessionToG8ee.mock.calls[0][0];
            expect(relayArg.bound_operators[0].status).toBe(OperatorStatus.BOUND);
        });

        it('should set ACTIVE status when operator is not bound (e.g. AVAILABLE)', async () => {
            const operatorId = 'op-avail-1';
            const userId = 'u-avail-1';

            mocks.apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: userId, organization_id: 'org-1', operator_id: operatorId }
            });
            mocks.userService.getUser.mockResolvedValue({ id: userId });
            mocks.operatorService.getOperator.mockResolvedValue({
                operator_id: operatorId,
                user_id: userId,
                web_session_id: null,
                status: OperatorStatus.AVAILABLE,
            });
            mocks.operatorSessionService.createOperatorSession.mockResolvedValue({ id: 'os-avail-1' });

            await service._authenticateViaApiKey({
                authorizationHeader: 'Bearer avail-key',
                system_info: { hostname: 'avail-host' },
            });

            expect(mocks.operatorService.claimOperatorSlot).toHaveBeenCalledWith(
                operatorId,
                expect.objectContaining({ status: OperatorStatus.ACTIVE })
            );

            expect(mocks.userService.updateUserOperator).toHaveBeenCalledWith(
                userId, operatorId, OperatorStatus.ACTIVE
            );
        });

        it('should return success even if g8ee relay fails', async () => {
            mocks.apiKeyService.validateKey.mockResolvedValue({
                success: true,
                data: { user_id: 'u-123', organization_id: 'org-123', operator_id: 'op-123' }
            });
            mocks.userService.getUser.mockResolvedValue({ id: 'u-123' });
            mocks.operatorService.getOperator.mockResolvedValue({ operator_id: 'op-123', user_id: 'u-123' });
            mocks.operatorSessionService.createOperatorSession.mockResolvedValue({ id: 'os-123' });
            mocks.operatorService.relayRegisterOperatorSessionToG8ee.mockRejectedValue(new Error('g8ee Down'));

            const result = await service._authenticateViaApiKey({
                authorizationHeader: 'Bearer key',
                system_info: { system_fingerprint: 'fp-123' }
            });

            expect(result.success).toBe(true);
            expect(result.response.operator_session_id).toBe('os-123');
            expect(mocks.operatorService.claimOperatorSlot).toHaveBeenCalled();
            expect(mocks.userService.updateUserOperator).toHaveBeenCalled();
        });
    });
});
