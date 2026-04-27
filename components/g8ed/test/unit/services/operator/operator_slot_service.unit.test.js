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
import { OperatorSlotService } from '@g8ed/services/operator/operator_slot_service.js';
import { OperatorStatus, OperatorType, CloudOperatorSubtype, DEFAULT_OPERATOR_SLOTS } from '@g8ed/constants/operator.js';
import { OperatorDocument, SystemInfo, CertInfo, OperatorSlotCreationResponse } from '@g8ed/models/operator_model.js';
import { OperatorRefreshKeyResponse } from '@g8ed/models/response_models.js';
import { SourceComponent } from '@g8ed/constants/ai.js';
import { ApiKeyStatus, ApiKeyClientName, ApiKeyPermission } from '@g8ed/constants/auth.js';

describe('OperatorSlotService', () => {
    let service;
    let mocks;

    beforeEach(() => {
        mocks = {
            operatorDataService: {
                queryOperators: vi.fn(),
                queryOperatorsFresh: vi.fn(),
                queryListedOperators: vi.fn().mockImplementation(async (filters, options) => {
                    // Get the operators and filter out TERMINATED
                    let operators;
                    if (options?.fresh) {
                        operators = await mocks.operatorDataService.queryOperatorsFresh(filters);
                    } else {
                        operators = await mocks.operatorDataService.queryOperators(filters);
                    }
                    // Filter out TERMINATED operators
                    return operators.filter(op => op.status !== OperatorStatus.TERMINATED);
                }),
                updateOperator: vi.fn(),
                createOperator: vi.fn(),
                getOperator: vi.fn(),
            },
            apiKeyService: {
                issueKey: vi.fn().mockResolvedValue({ success: true }),
                revokeKey: vi.fn().mockResolvedValue({ success: true }),
            },
            certificateService: {
                generateOperatorCertificate: vi.fn().mockResolvedValue({ success: true }),
                revokeCertificate: vi.fn().mockResolvedValue({ success: true }),
            },
            operatorSessionService: {
                endSession: vi.fn().mockResolvedValue({ success: true }),
            },
            operatorService: {
                relayCreateOperatorSlotToG8ee: vi.fn().mockResolvedValue({ success: true }),
                relayClaimOperatorSlotToG8ee: vi.fn().mockResolvedValue({ success: true }),
                relayTerminateOperatorToG8ee: vi.fn().mockResolvedValue({ success: true }),
                relayEndOperatorSessionToG8ee: vi.fn().mockResolvedValue({ success: true }),
                relayUpdateOperatorApiKeyToG8ee: vi.fn().mockResolvedValue({ success: true }),
                terminateOperator: vi.fn().mockResolvedValue({ success: true }),
            },
        };

        service = new OperatorSlotService(mocks);
    });

    describe('initializeOperatorSlots', () => {
        it('should create missing slots up to default count', async () => {
            const userId = 'u-123';
            const organizationId = 'org-123';
            const newOpId = `${userId}_operator_1_12345_abc123`;

            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([]);

            vi.spyOn(service, 'createOperatorSlot').mockResolvedValue({
                success: true,
                operator_id: newOpId
            });

            const result = await service.initializeOperatorSlots(userId, organizationId);

            expect(service.createOperatorSlot).toHaveBeenCalledTimes(DEFAULT_OPERATOR_SLOTS);
            expect(result).toContain(newOpId);
            expect(result).toHaveLength(DEFAULT_OPERATOR_SLOTS);
        });

        it('should only query operators once (no second round-trip)', async () => {
            const userId = 'u-123';
            const organizationId = 'org-123';

            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([]);
            vi.spyOn(service, 'createOperatorSlot').mockResolvedValue({
                success: true,
                operator_id: 'new-op'
            });

            await service.initializeOperatorSlots(userId, organizationId);

            expect(mocks.operatorDataService.queryOperatorsFresh).toHaveBeenCalledTimes(1);
        });

        it('should not re-issue API keys for any operators (no reindex)', async () => {
            const userId = 'u-123';
            const organizationId = 'org-123';
            const existingOp = {
                id: 'existing-op-1',
                user_id: userId,
                status: OperatorStatus.AVAILABLE,
                api_key: 'key-existing'
            };

            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([existingOp]);
            vi.spyOn(service, 'createOperatorSlot').mockResolvedValue({
                success: true,
                operator_id: 'new-op'
            });

            await service.initializeOperatorSlots(userId, organizationId);

            expect(mocks.apiKeyService.issueKey).not.toHaveBeenCalled();
        });

        it('should return combined list of existing and new operator IDs', async () => {
            const userId = 'u-123';
            const organizationId = 'org-123';
            const existingOp = {
                id: 'existing-op-1',
                user_id: userId,
                status: OperatorStatus.AVAILABLE,
                api_key: 'key-existing'
            };
            const newOpId = 'new-op-2';

            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([existingOp]);
            vi.spyOn(service, 'createOperatorSlot').mockResolvedValue({
                success: true,
                operator_id: newOpId
            });

            const result = await service.initializeOperatorSlots(userId, organizationId);

            expect(result).toContain('existing-op-1');
            expect(result).toContain(newOpId);
        });

        it('should exclude TERMINATED operators from the live count and returned IDs', async () => {
            const userId = 'u-123';
            const organizationId = 'org-123';
            const liveOps = Array.from({ length: DEFAULT_OPERATOR_SLOTS }, (_, i) => ({
                id: `op-${i}`,
                status: OperatorStatus.AVAILABLE
            }));
            const terminatedOp = {
                id: 'op-terminated',
                status: OperatorStatus.TERMINATED
            };

            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([...liveOps, terminatedOp]);

            const result = await service.initializeOperatorSlots(userId, organizationId);

            expect(result).not.toContain('op-terminated');
            expect(result).toHaveLength(DEFAULT_OPERATOR_SLOTS);
        });

        it('should not create slots if already at or above default count', async () => {
            const existing = Array.from({ length: DEFAULT_OPERATOR_SLOTS }, (_, i) => ({
                id: `op-${i}`,
                status: OperatorStatus.AVAILABLE
            }));
            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce(existing);

            await service.initializeOperatorSlots('u-1', 'org-1');

            expect(mocks.operatorDataService.createOperator).not.toHaveBeenCalled();
        });

        it('should assign G8E_POD subtype to the first created slot when no g8ep exists', async () => {
            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([]);

            const calls = [];
            vi.spyOn(service, 'createOperatorSlot').mockImplementation(async (args) => {
                calls.push(args);
                return { success: true, operator_id: `op-${args.slotNumber}` };
            });

            await service.initializeOperatorSlots('u-1', 'org-1');

            expect(calls[0].cloudSubtype).toBe(CloudOperatorSubtype.G8E_POD);
            expect(calls[0].isG8eNode).toBe(true);
            expect(calls[1].cloudSubtype).toBeNull();
            expect(calls[1].isG8eNode).toBe(false);
        });

        it('should not assign G8E_POD if an existing live operator already has is_g8ep', async () => {
            const existingG8eNode = {
                id: 'op-drop',
                status: OperatorStatus.AVAILABLE,
                is_g8ep: true,
            };
            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([existingG8eNode]);

            const calls = [];
            vi.spyOn(service, 'createOperatorSlot').mockImplementation(async (args) => {
                calls.push(args);
                return { success: true, operator_id: `op-${args.slotNumber}` };
            });

            await service.initializeOperatorSlots('u-1', 'org-1');

            const g8eNodeCalls = calls.filter(c => c.isG8eNode === true);
            expect(g8eNodeCalls).toHaveLength(0);
        });

        it('should assign G8E_POD to exactly one slot per user', async () => {
            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([]);

            const calls = [];
            vi.spyOn(service, 'createOperatorSlot').mockImplementation(async (args) => {
                calls.push(args);
                return { success: true, operator_id: `op-${args.slotNumber}` };
            });

            await service.initializeOperatorSlots('u-1', 'org-1');

            const g8eNodeCalls = calls.filter(c => c.isG8eNode === true);
            expect(g8eNodeCalls).toHaveLength(1);
            expect(g8eNodeCalls[0].cloudSubtype).toBe(CloudOperatorSubtype.G8E_POD);
        });

        it('should issue API keys for existing slots without keys', async () => {
            const userId = 'u-123';
            const organizationId = 'org-123';
            const existingOpWithoutKey = {
                id: 'existing-op-1',
                user_id: userId,
                status: OperatorStatus.AVAILABLE,
                api_key: null
            };

            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([existingOpWithoutKey]);
            mocks.operatorService.relayUpdateOperatorApiKeyToG8ee.mockResolvedValue({ success: true });

            await service.initializeOperatorSlots(userId, organizationId);

            expect(mocks.operatorService.relayUpdateOperatorApiKeyToG8ee).toHaveBeenCalledWith(
                existingOpWithoutKey.id,
                expect.any(String),
                expect.objectContaining({
                    user_id: userId,
                    organization_id: organizationId,
                    source_component: SourceComponent.G8ED
                })
            );
            expect(mocks.apiKeyService.issueKey).toHaveBeenCalledWith(
                expect.any(String),
                expect.objectContaining({
                    user_id: userId,
                    operator_id: existingOpWithoutKey.id,
                    client_name: ApiKeyClientName.OPERATOR
                })
            );
        });

        it('should not issue API keys for existing slots that already have keys', async () => {
            const userId = 'u-123';
            const organizationId = 'org-123';
            const existingOpWithKey = {
                id: 'existing-op-1',
                user_id: userId,
                status: OperatorStatus.AVAILABLE,
                api_key: 'existing-key'
            };

            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([existingOpWithKey]);

            await service.initializeOperatorSlots(userId, organizationId);

            expect(mocks.operatorService.relayUpdateOperatorApiKeyToG8ee).not.toHaveBeenCalled();
            expect(mocks.apiKeyService.issueKey).not.toHaveBeenCalled();
        });
    });

    describe('refreshOperatorApiKey', () => {
        it('should terminate old operator and create new one', async () => {
            const operatorId = 'op-old';
            const userId = 'u-123';
            const webSessionId = 'web-sess-123';
            const oldOperator = new OperatorDocument({
                id: operatorId,
                user_id: userId,
                organization_id: 'org-123',
                slot_number: 1,
                api_key: 'key-old',
                operator_cert_serial: 'serial-old',
                operator_session_id: 'os-old',
            });

            const broadcastFn = vi.fn();

            mocks.operatorDataService.getOperator.mockResolvedValue(oldOperator);
            mocks.operatorService.terminateOperator.mockResolvedValue({ success: true });
            mocks.operatorService.relayCreateOperatorSlotToG8ee.mockResolvedValue({
                success: true,
                operator_id: 'op-new',
                api_key: 'g8e_1a2b3c4d_' + '0'.repeat(64)
            });
            mocks.operatorSessionService.endSession.mockResolvedValue();
            mocks.apiKeyService.issueKey.mockResolvedValue({ success: true });
            mocks.apiKeyService.revokeKey.mockResolvedValue();
            mocks.certificateService.revokeCertificate.mockResolvedValue();

            const result = await service.refreshOperatorApiKey(operatorId, userId, webSessionId, broadcastFn);

            expect(result).toBeInstanceOf(OperatorRefreshKeyResponse);
            expect(result.success).toBe(true);
            expect(result.new_api_key).toBe('g8e_1a2b3c4d_' + '0'.repeat(64));
            expect(mocks.operatorService.terminateOperator).toHaveBeenCalledWith(operatorId);
            expect(mocks.operatorService.relayCreateOperatorSlotToG8ee).toHaveBeenCalled();
            expect(mocks.apiKeyService.issueKey).toHaveBeenCalled();
            expect(broadcastFn).toHaveBeenCalledWith(userId);
        });

        it('should return the real API key from the creation relay, not a status string', async () => {
            const operatorId = 'op-old';
            const userId = 'u-123';
            const webSessionId = 'web-sess-123';
            const newApiKey = 'g8e_1a2b3c4d_' + '0'.repeat(64);
            const newOperatorId = 'op-new';

            const oldOperator = new OperatorDocument({
                id: operatorId,
                user_id: userId,
                organization_id: 'org-123',
                slot_number: 1,
                api_key: 'key-old',
            });

            mocks.operatorDataService.getOperator.mockResolvedValue(oldOperator);
            mocks.operatorService.terminateOperator.mockResolvedValue({ success: true });
            
            // Mock relayCreateOperatorSlotToG8ee to return a real key
            mocks.operatorService.relayCreateOperatorSlotToG8ee.mockResolvedValue({
                success: true,
                operator_id: newOperatorId,
                api_key: newApiKey
            });

            const result = await service.refreshOperatorApiKey(operatorId, userId, webSessionId);

            expect(result.success).toBe(true);
            expect(result.new_api_key).toBe(newApiKey);
            expect(result.new_api_key).not.toBe('API key refreshed');
            expect(result.new_operator_id).toBe(newOperatorId);
            expect(mocks.operatorService.terminateOperator).toHaveBeenCalledWith(operatorId);
        });

        it('should fail if operator not found', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue(null);
            const result = await service.refreshOperatorApiKey('op-1', 'u-1', 'web-sess-123', vi.fn());
            expect(result).toBeInstanceOf(OperatorRefreshKeyResponse);
            expect(result.success).toBe(false);
        });

        it('should fail if unauthorized', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue(new OperatorDocument({ user_id: 'u-other', id: 'op-1' }));
            const result = await service.refreshOperatorApiKey('op-1', 'u-me', 'web-sess-123', vi.fn());
            expect(result).toBeInstanceOf(OperatorRefreshKeyResponse);
            expect(result.success).toBe(false);
            expect(result.message).toBe('Unauthorized');
        });
    });

    describe('createOperatorSlot', () => {
        it('should relay to operatorService and issue API key on success', async () => {
            const params = {
                userId: 'u-1',
                organizationId: 'org-1',
                slotNumber: 1,
                operatorType: OperatorType.CLOUD,
                cloudSubtype: CloudOperatorSubtype.G8E_POD,
                namePrefix: 'op',
                isG8eNode: true,
                webSessionId: 'web-sess-123'
            };

            const operatorId = 'new-op-id';
            const apiKey = 'g8e_test_key';
            mocks.operatorService.relayCreateOperatorSlotToG8ee.mockResolvedValue({
                success: true,
                operator_id: operatorId,
                api_key: apiKey
            });
            mocks.apiKeyService.issueKey.mockResolvedValue({ success: true });

            const result = await service.createOperatorSlot(params);

            expect(result).toBeInstanceOf(OperatorSlotCreationResponse);
            expect(result.success).toBe(true);
            expect(result.operator_id).toBe(operatorId);
            expect(result.api_key).toBe(apiKey);
            expect(mocks.operatorService.relayCreateOperatorSlotToG8ee).toHaveBeenCalled();
            expect(mocks.apiKeyService.issueKey).toHaveBeenCalled();
        });

        it('should return failure if relay fails', async () => {
            mocks.operatorService.relayCreateOperatorSlotToG8ee.mockResolvedValue({
                success: false,
                error: 'g8ee error'
            });

            const result = await service.createOperatorSlot({ userId: 'u-1', webSessionId: 'web-sess-123' });

            expect(result.success).toBe(false);
            expect(result.message).toBe('g8ee error');
            expect(mocks.apiKeyService.issueKey).not.toHaveBeenCalled();
        });

        it('should throw error if operatorService is missing', async () => {
            const serviceNoRelay = new OperatorSlotService({ ...mocks, operatorService: null });
            await expect(serviceNoRelay.createOperatorSlot({ userId: 'u-1', webSessionId: 'web-sess-123' })).rejects.toThrow('operatorService is required');
        });
    });

    describe('claimSlot', () => {
        it('should relay to operatorService and return success', async () => {
            const id = 'op-1';
            const params = {
                operator_session_id: 'os-1',
                bound_web_session_id: 'ws-1',
                system_info: { hostname: 'test' },
                operator_type: 'system',
            };

            mocks.operatorDataService.getOperator.mockResolvedValue({
                id,
                user_id: 'u-1',
                organization_id: 'org-1',
                operator_type: 'system'
            });
            mocks.operatorService.relayClaimOperatorSlotToG8ee.mockResolvedValue({ success: true });

            const result = await service.claimSlot(id, params);

            expect(result.success).toBe(true);
            expect(mocks.operatorService.relayClaimOperatorSlotToG8ee).toHaveBeenCalled();
        });

        it('should return failure if operator not found', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue(null);
            const result = await service.claimSlot('op-1', {});
            expect(result.success).toBe(false);
            expect(result.error).toBe('Operator not found');
        });

        it('should return failure if relay fails', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue({ id: 'op-1', user_id: 'u-1' });
            mocks.operatorService.relayClaimOperatorSlotToG8ee.mockResolvedValue({ success: false, error: 'relay error' });

            const result = await service.claimSlot('op-1', {});

            expect(result.success).toBe(false);
            expect(result.error).toBe('relay error');
            expect(mocks.operatorDataService.updateOperator).not.toHaveBeenCalled();
        });
    });

    describe('generateOperatorApiKey', () => {
        it('should generate key with correct prefix and suffix', () => {
            const operatorId = '550e8400-e29b-41d4-a716-446655440000';
            const key = service.generateOperatorApiKey(operatorId);

            expect(key).toMatch(/^g8e_[a-z0-9]{5,}_[a-f0-9]{64}$/);
        });
    });
});
