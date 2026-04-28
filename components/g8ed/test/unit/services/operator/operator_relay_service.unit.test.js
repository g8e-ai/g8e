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
import { OperatorRelayService } from '@g8ed/services/operator/operator_relay_service.js';
import { G8eHttpContext } from '@g8ed/models/request_models.js';
import { ApiPaths } from '@g8ed/constants/api_paths.js';

describe('OperatorRelayService', () => {
    let service;
    let mockHttpClient;

    beforeEach(() => {
        mockHttpClient = {
            request: vi.fn().mockResolvedValue({ success: true })
        };
        service = new OperatorRelayService({
            internalHttpClient: mockHttpClient
        });
    });

    const context = new G8eHttpContext({
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

    describe('relayStopCommandToG8ee', () => {
        it('should relay stop request with correct parameters', async () => {
            await service.relayStopCommandToG8ee(context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('g8ee', ApiPaths.g8ee.operatorsStop(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    operator_id: 'op-123',
                    operator_session_id: 'os-123',
                    user_id: 'u-123'
                }),
                g8eContext: context
            }));
        });

        it('should throw if g8eContext is missing', async () => {
            await expect(service.relayStopCommandToG8ee(null)).rejects.toThrow('ENFORCEMENT VIOLATION');
        });
    });

    describe('deregisterOperatorSessionInG8ee', () => {
        it('should relay deregistration request', async () => {
            await service.deregisterOperatorSessionInG8ee(context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('g8ee', ApiPaths.g8ee.operatorsDeregisterSession(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    operator_id: 'op-123',
                    operator_session_id: 'os-123'
                }),
                g8eContext: context
            }));
        });
    });

    describe('relayDirectCommandToG8ee', () => {
        it('should relay direct command request', async () => {
            const commandData = {
                command: 'ls',
                execution_id: 'exec-123'
            };

            await service.relayDirectCommandToG8ee(commandData, context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('g8ee', ApiPaths.g8ee.operatorDirectCommand(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    command: 'ls',
                    execution_id: 'exec-123'
                }),
                g8eContext: context
            }));
        });
    });

    describe('relayRegisterOperatorSessionToG8ee', () => {
        it('should relay registration request', async () => {
            await service.relayRegisterOperatorSessionToG8ee(context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('g8ee', ApiPaths.g8ee.operatorsRegisterSession(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    operator_id: 'op-123',
                    operator_session_id: 'os-123'
                }),
                g8eContext: context
            }));
        });
    });

    describe('relayApprovalResponseToG8ee', () => {
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

            await service.relayApprovalResponseToG8ee(approvalData, context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('g8ee', ApiPaths.g8ee.operatorApprovalRespond(), expect.objectContaining({
                method: 'POST',
                body: expect.objectContaining({
                    approval_id: 'app-123',
                    approved: true
                }),
                g8eContext: context
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

            await service.relayApprovalResponseToG8ee(wireData, context);

            const callArgs = mockHttpClient.request.mock.calls[0][2];
            expect(callArgs.body).toBe(wireData);
        });
    });

    describe('relayPendingApprovalsFromG8ee', () => {
        it('should fetch pending approvals from g8ee', async () => {
            await service.relayPendingApprovalsFromG8ee(context);

            expect(mockHttpClient.request).toHaveBeenCalledWith('g8ee', ApiPaths.g8ee.operatorApprovalPending(), expect.objectContaining({
                method: 'GET',
                g8eContext: context
            }));
        });

        it('should throw if g8eContext is missing', async () => {
            await expect(service.relayPendingApprovalsFromG8ee(null)).rejects.toThrow('ENFORCEMENT VIOLATION');
        });

        it('should pass context with case_id and investigation_id', async () => {
            const contextWithIds = new G8eHttpContext({
                web_session_id: 'ws-123',
                user_id: 'u-123',
                case_id: 'c-456',
                investigation_id: 'i-789',
                bound_operators: [{
                    operator_id: 'op-123',
                    operator_session_id: 'os-123'
                }]
            });

            await service.relayPendingApprovalsFromG8ee(contextWithIds);

            expect(mockHttpClient.request).toHaveBeenCalledWith('g8ee', ApiPaths.g8ee.operatorApprovalPending(), expect.objectContaining({
                method: 'GET',
                g8eContext: contextWithIds
            }));
        });
    });
});
