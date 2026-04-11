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
import { OperatorRelayService } from '@vsod/services/operator/operator_relay_service.js';
import { getInternalHttpClient } from '@vsod/services/clients/internal_http_client.js';
import { VSOHttpContext } from '@vsod/models/request_models.js';
import { apiPaths } from '@vsod/constants/api_paths.js';

vi.mock('@vsod/services/clients/internal_http_client.js', () => ({
    getInternalHttpClient: vi.fn()
}));

describe('OperatorRelayService', () => {
    let service;
    let mockHttpClient;

    beforeEach(() => {
        mockHttpClient = {
            request: vi.fn().mockResolvedValue({ success: true })
        };
        vi.mocked(getInternalHttpClient).mockReturnValue(mockHttpClient);
        service = new OperatorRelayService();
    });

    const context = new VSOHttpContext({
        web_session_id: 'ws-123',
        user_id: 'u-123',
        case_id: 'c-123',
        investigation_id: 'i-123',
        task_id: 't-123',
        bound_operators: [{
            operator_id: 'op-123',
            operator_session_id: 'os-123'
        }]
    });

    describe('relayStopCommandToVse', () => {
        it('should relay stop request with correct parameters', async () => {
            await service.relayStopCommandToVse(context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('vse', apiPaths.vse.operatorsStop(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    operator_id: 'op-123',
                    operator_session_id: 'os-123',
                    user_id: 'u-123'
                }),
                vsoContext: context
            }));
        });

        it('should throw if vsoContext is missing', async () => {
            await expect(service.relayStopCommandToVse(null)).rejects.toThrow('ENFORCEMENT VIOLATION');
        });
    });

    describe('deregisterOperatorSessionInVse', () => {
        it('should relay deregistration request', async () => {
            await service.deregisterOperatorSessionInVse(context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('vse', apiPaths.vse.operatorsDeregisterSession(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    operator_id: 'op-123',
                    operator_session_id: 'os-123'
                }),
                vsoContext: context
            }));
        });
    });

    describe('relayDirectCommandToVse', () => {
        it('should relay direct command request', async () => {
            const commandData = {
                command: 'ls',
                execution_id: 'exec-123'
            };

            await service.relayDirectCommandToVse(commandData, context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('vse', apiPaths.vse.operatorDirectCommand(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    command: 'ls',
                    execution_id: 'exec-123'
                }),
                vsoContext: context
            }));
        });
    });

    describe('relayRegisterOperatorSessionToVse', () => {
        it('should relay registration request', async () => {
            await service.relayRegisterOperatorSessionToVse(context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('vse', apiPaths.vse.operatorsRegisterSession(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    operator_id: 'op-123',
                    operator_session_id: 'os-123'
                }),
                vsoContext: context
            }));
        });
    });

    describe('relayApprovalResponseToVse', () => {
        it('should relay approval response', async () => {
            const approvalData = {
                approval_id: 'app-123',
                approved: true,
                user_id: 'u-123',
                web_session_id: 'ws-123',
                operator_session_id: 'os-123',
                operator_id: 'op-123',
                case_id: 'c-123',
                investigation_id: 'i-123',
                task_id: 't-123'
            };

            await service.relayApprovalResponseToVse(approvalData, context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('vse', apiPaths.vse.operatorApprovalRespond(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    approval_id: 'app-123',
                    approved: true
                }),
                vsoContext: context
            }));
        });

        it('passes pre-validated data through without re-parsing (no double-parse)', async () => {
            const wireData = {
                approval_id: 'app-456',
                approved: false,
                user_id: 'u-456',
                web_session_id: 'ws-456',
                operator_session_id: 'os-456',
                operator_id: 'op-456',
                reason: 'denied by user',
                case_id: 'c-456',
                investigation_id: 'i-456',
                task_id: 't-456'
            };

            await service.relayApprovalResponseToVse(wireData, context);

            const callArgs = mockHttpClient.request.mock.calls[0][2];
            expect(callArgs.body).toBe(wireData);
        });
    });

    describe('relayPendingApprovalsFromVse', () => {
        it('should fetch pending approvals from VSE', async () => {
            await service.relayPendingApprovalsFromVse(context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('vse', apiPaths.vse.operatorApprovalPending(), expect.objectContaining({
                method: 'GET',
                vsoContext: context
            }));
        });

        it('should throw if vsoContext is missing', async () => {
            await expect(service.relayPendingApprovalsFromVse(null)).rejects.toThrow('ENFORCEMENT VIOLATION');
        });

        it('should pass context with case_id and investigation_id', async () => {
            const contextWithIds = new VSOHttpContext({
                web_session_id: 'ws-123',
                user_id: 'u-123',
                case_id: 'c-456',
                investigation_id: 'i-789',
                bound_operators: [{
                    operator_id: 'op-123',
                    operator_session_id: 'os-123'
                }]
            });

            await service.relayPendingApprovalsFromVse(contextWithIds);

            expect(mockHttpClient.request).toHaveBeenCalledWith('vse', apiPaths.vse.operatorApprovalPending(), expect.objectContaining({
                method: 'GET',
                vsoContext: contextWithIds
            }));
        });
    });
});
