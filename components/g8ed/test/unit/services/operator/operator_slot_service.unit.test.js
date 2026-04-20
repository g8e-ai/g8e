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
import { OperatorStatus, OperatorType, HistoryEventType, CloudOperatorSubtype, DEFAULT_OPERATOR_SLOTS } from '@g8ed/constants/operator.js';
import { OperatorDocument, SystemInfo, CertInfo, OperatorRefreshKeyResponse, OperatorSlotCreationResponse } from '@g8ed/models/operator_model.js';
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
                updateOperator: vi.fn(),
                createOperator: vi.fn(),
                getOperator: vi.fn(),
            },
            apiKeyService: {
                issueKey: vi.fn(),
                revokeKey: vi.fn(),
            },
            certificateService: {
                generateOperatorCertificate: vi.fn(),
                revokeCertificate: vi.fn(),
            },
            operatorSessionService: {
                endSession: vi.fn(),
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
                operator_id: 'existing-op-1',
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
                operator_id: 'existing-op-1',
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
                operator_id: `op-${i}`,
                status: OperatorStatus.AVAILABLE
            }));
            const terminatedOp = {
                operator_id: 'op-terminated',
                status: OperatorStatus.TERMINATED
            };

            mocks.operatorDataService.queryOperatorsFresh.mockResolvedValueOnce([...liveOps, terminatedOp]);

            const result = await service.initializeOperatorSlots(userId, organizationId);

            expect(result).not.toContain('op-terminated');
            expect(result).toHaveLength(DEFAULT_OPERATOR_SLOTS);
        });

        it('should not create slots if already at or above default count', async () => {
            const existing = Array.from({ length: DEFAULT_OPERATOR_SLOTS }, (_, i) => ({
                operator_id: `op-${i}`,
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
                operator_id: 'op-drop',
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
    });

    describe('refreshOperatorApiKey', () => {
        it('should terminate old operator and create new one', async () => {
            const operatorId = 'op-old';
            const userId = 'u-123';
            const oldOperator = new OperatorDocument({
                operator_id: operatorId,
                user_id: userId,
                organization_id: 'org-123',
                slot_number: 1,
                api_key: 'key-old',
                operator_cert_serial: 'serial-old',
                operator_session_id: 'os-old',
            });

            const broadcastFn = vi.fn();

            mocks.operatorDataService.getOperator.mockResolvedValue(oldOperator);
            mocks.operatorDataService.updateOperator.mockResolvedValue({ success: true });
            mocks.operatorDataService.createOperator.mockResolvedValue({ success: true });
            mocks.operatorSessionService.endSession.mockResolvedValue();
            mocks.apiKeyService.revokeKey.mockResolvedValue();
            mocks.certificateService.revokeCertificate.mockResolvedValue();

            const result = await service.refreshOperatorApiKey(operatorId, userId, broadcastFn);

            expect(result).toBeInstanceOf(OperatorRefreshKeyResponse);
            expect(result.success).toBe(true);
            expect(result.new_api_key).toBeDefined();
            expect(mocks.operatorDataService.updateOperator).toHaveBeenCalledWith(
                operatorId,
                expect.objectContaining({ status: OperatorStatus.TERMINATED })
            );
            expect(mocks.operatorDataService.createOperator).toHaveBeenCalled();
            expect(mocks.apiKeyService.issueKey).toHaveBeenCalled();
            expect(broadcastFn).toHaveBeenCalledWith(userId);
        });

        it('should fail if operator not found', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue(null);
            const result = await service.refreshOperatorApiKey('op-1', 'u-1', vi.fn());
            expect(result).toBeInstanceOf(OperatorRefreshKeyResponse);
            expect(result.success).toBe(false);
        });

        it('should fail if unauthorized', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue(new OperatorDocument({ user_id: 'u-other', operator_id: 'op-1' }));
            const result = await service.refreshOperatorApiKey('op-1', 'u-me', vi.fn());
            expect(result).toBeInstanceOf(OperatorRefreshKeyResponse);
            expect(result.success).toBe(false);
            expect(result.message).toBe('Unauthorized');
        });
    });

    describe('claimSlot', () => {
        it('should use provided status instead of defaulting to ACTIVE', async () => {
            mocks.operatorDataService.updateOperator.mockResolvedValue(true);

            await service.claimSlot('op-1', {
                operator_session_id: 'os-1',
                bound_web_session_id: 'ws-1',
                system_info: { hostname: 'test' },
                operator_type: 'system',
                status: OperatorStatus.BOUND,
            });

            const updateCall = mocks.operatorDataService.updateOperator.mock.calls[0];
            expect(updateCall[0]).toBe('op-1');
            expect(updateCall[1].status).toBe(OperatorStatus.BOUND);
        });

        it('should default to ACTIVE when no status is provided', async () => {
            mocks.operatorDataService.updateOperator.mockResolvedValue(true);

            await service.claimSlot('op-2', {
                operator_session_id: 'os-2',
                bound_web_session_id: 'ws-2',
                system_info: { hostname: 'test2' },
                operator_type: null,
            });

            const updateCall = mocks.operatorDataService.updateOperator.mock.calls[0];
            expect(updateCall[0]).toBe('op-2');
            expect(updateCall[1].status).toBe(OperatorStatus.ACTIVE);
        });
    });

    describe('generateOperatorApiKey', () => {
        it('should generate key with correct prefix and suffix', () => {
            const operatorId = 'user_operator_1_12345_abcde';
            const key = service.generateOperatorApiKey(operatorId);
            
            expect(key).toMatch(/^g8e_[a-z0-9]{5,}_[a-f0-9]{64}$/);
        });
    });
});
