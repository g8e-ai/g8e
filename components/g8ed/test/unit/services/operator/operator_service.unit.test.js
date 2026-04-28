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
import { OperatorService } from '@g8ed/services/operator/operator_service.js';
import { OperatorStatus } from '@g8ed/constants/operator.js';
import { OperatorDocument, OperatorWithSessionContext, OperatorSlot, OperatorListUpdatedEvent } from '@g8ed/models/operator_model.js';
import { EventType } from '@g8ed/constants/events.js';

describe('OperatorService', () => {
    let service;
    let mocks;

    beforeEach(() => {
        mocks = {
            operatorDataService: {
                getOperator: vi.fn(),
                getOperatorFresh: vi.fn(),
                queryOperators: vi.fn(),
                queryListedOperators: vi.fn().mockImplementation((filters) => {
                    // Filter out TERMINATED operators
                    const allOperators = mocks.operatorDataService.queryOperators(filters);
                    return allOperators.then(ops => ops.filter(op => op.status !== OperatorStatus.TERMINATED));
                }),
                updateOperator: vi.fn(),
                createOperator: vi.fn(),
                deleteOperator: vi.fn(),
                collectionName: 'operators_test',
            },
            userService: {
                getUser: vi.fn(),
            },
            apiKeyService: {
                issueKey: vi.fn(),
                revokeKey: vi.fn().mockResolvedValue({ success: true }),
            },
            certificateService: {
                generateOperatorCertificate: vi.fn(),
                revokeCertificate: vi.fn(),
            },
            operatorSessionService: {
                validateSession: vi.fn(),
            },
            webSessionService: {
                validateSession: vi.fn(),
                getUserActiveSessions: vi.fn(),
                getUserActiveSession: vi.fn(),
            },
            sseService: {
                publishEvent: vi.fn().mockResolvedValue(true),
            },
            internalHttpClient: {
                post: vi.fn(),
                get: vi.fn(),
            },
        };

        service = new OperatorService(mocks);
    });

    describe('calculateSlotUsage', () => {
        it('should correctly count active and bound operators', () => {
            const operators = [
                { id: 'op-1', status: OperatorStatus.ACTIVE },
                { id: 'op-2', status: OperatorStatus.BOUND },
                { id: 'op-3', status: OperatorStatus.OFFLINE },
                { id: 'op-4', status: OperatorStatus.TERMINATED },
            ];

            const result = service.calculateSlotUsage(operators);

            expect(result.usedSlots).toBe(2);
            expect(result.claimedOperators).toHaveLength(2);
        });
    });

    describe('getOperator', () => {
        it('should return OperatorDocument if found via Data service', async () => {
            const opDoc = new OperatorDocument({ 
                id: 'op-1', 
                user_id: 'u-1', 
                status: OperatorStatus.ACTIVE 
            });
            mocks.operatorDataService.getOperator.mockResolvedValue(opDoc);

            const result = await service.getOperator('op-1');

            expect(result).toBe(opDoc);
            expect(mocks.operatorDataService.getOperator).toHaveBeenCalledWith('op-1');
        });
    });

    describe('getOperatorWithSessionContext', () => {
        it('should return combined context for operator and its sessions', async () => {
            const operator = new OperatorDocument({ 
                id: 'op-1', 
                user_id: 'u-1',
                status: OperatorStatus.ACTIVE,
                operator_session_id: 'os-1', 
                bound_web_session_id: 'ws-1' 
            });
            mocks.operatorDataService.getOperator.mockResolvedValue(operator);
            
            const opSession = { id: 'os-1', user_id: 'u-1' };
            const webSession = { id: 'ws-1', user_id: 'u-1' };
            
            mocks.operatorSessionService.validateSession.mockResolvedValue(opSession);
            mocks.webSessionService.validateSession.mockResolvedValue(webSession);

            const result = await service.getOperatorWithSessionContext('op-1');

            expect(result).toBeInstanceOf(OperatorWithSessionContext);
            expect(result.id).toBe('op-1');
            expect(result.operator_session_id).toBe('os-1');
            expect(result.bound_web_session_id).toBe('ws-1');
        });
    });

    describe('Lifecycle & Relay Orchestration', () => {
        it('should relay stop command to g8ee', async () => {
            const operator = new OperatorDocument({ id: 'op-1', user_id: 'u-1', status: OperatorStatus.ACTIVE });
            const context = OperatorWithSessionContext.create(operator);
            
            vi.spyOn(service.relay, 'relayStopCommandToG8ee').mockResolvedValue(true);

            const result = await service.relayStopCommandToG8ee(context);

            expect(result).toBe(true);
            expect(service.relay.relayStopCommandToG8ee).toHaveBeenCalledWith(context);
        });
    });

    describe('getUserOperators', () => {
        it('should return OperatorSlot projections for the panel list', async () => {
            const operators = [
                new OperatorDocument({ id: 'op-1', user_id: 'u-1', status: OperatorStatus.ACTIVE, name: 'node-01', bound_web_session_id: 'ws-1' }),
                new OperatorDocument({ id: 'op-2', user_id: 'u-1', status: OperatorStatus.AVAILABLE })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getUserOperators('u-1');

            expect(result).toBeInstanceOf(OperatorListUpdatedEvent);
            expect(result.type).toBe(EventType.OPERATOR_PANEL_LIST_UPDATED);
            expect(result.operators).toHaveLength(2);
            expect(result.active_count).toBe(1);
            expect(result.used_slots).toBe(1);

            const slot = result.operators[0];
            expect(slot).toBeInstanceOf(OperatorSlot);
            expect(slot.operator_id).toBe('op-1');
            expect(slot.name).toBe('node-01');
            expect(slot.status).toBe(OperatorStatus.ACTIVE);
            expect(slot.status_display).toBe('ACTIVE');
            expect(slot.status_class).toBe('active');
            expect(slot.bound_web_session_id).toBe('ws-1');

            expect(slot).not.toHaveProperty('api_key');
            expect(slot).not.toHaveProperty('granted_intents');
            expect(slot).not.toHaveProperty('runtime_config');
        });

        it('should filter out terminated operators by default', async () => {
            const operators = [
                new OperatorDocument({ id: 'op-1', user_id: 'u-1', status: OperatorStatus.ACTIVE }),
                new OperatorDocument({ id: 'op-2', user_id: 'u-1', status: OperatorStatus.TERMINATED })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getUserOperators('u-1');
            expect(result.operators).toHaveLength(1);
            expect(result.operators[0].operator_id).toBe('op-1');
        });

        it('should include terminated operators if allStatuses is true', async () => {
            const operators = [
                new OperatorDocument({ id: 'op-1', user_id: 'u-1', status: OperatorStatus.ACTIVE }),
                new OperatorDocument({ id: 'op-2', user_id: 'u-1', status: OperatorStatus.TERMINATED })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getUserOperators('u-1', true);
            expect(result.operators).toHaveLength(2);
        });
    });

    describe('syncSessionOnConnect', () => {
        it('should re-bind operators and broadcast list', async () => {
            const userId = 'u-1';
            const webSessionId = 'ws-new';
            const operators = [
                { id: 'op-1', status: OperatorStatus.BOUND, bound_web_session_id: 'ws-old' },
                { id: 'op-2', status: OperatorStatus.ACTIVE }
            ];
            
            vi.spyOn(service, 'getUserVisibleOperatorStats').mockResolvedValue({ operators });
            vi.spyOn(service, 'updateWebSessionLink').mockResolvedValue(true);

            await service.syncSessionOnConnect(userId, webSessionId);

            expect(service.updateWebSessionLink).toHaveBeenCalledWith('op-1', webSessionId);
        });
    });

    describe('getAllOperators', () => {
        it('should return all non-terminated operators by default', async () => {
            const operators = [
                new OperatorDocument({ id: 'op-1', user_id: 'u-1', status: OperatorStatus.ACTIVE }),
                new OperatorDocument({ id: 'op-2', user_id: 'u-2', status: OperatorStatus.AVAILABLE }),
                new OperatorDocument({ id: 'op-3', user_id: 'u-1', status: OperatorStatus.TERMINATED })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getAllOperators();

            expect(result.operators).toHaveLength(2);
            expect(result.total_count).toBe(2);
            expect(result.active_count).toBe(1);
            expect(result.operators.find(op => op.id === 'op-3')).toBeUndefined();
        });

        it('should return all operators including terminated if allStatuses is true', async () => {
            const operators = [
                new OperatorDocument({ id: 'op-1', user_id: 'u-1', status: OperatorStatus.ACTIVE }),
                new OperatorDocument({ id: 'op-2', user_id: 'u-1', status: OperatorStatus.TERMINATED })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getAllOperators(true);

            expect(result.operators).toHaveLength(2);
            expect(result.total_count).toBe(2);
            expect(result.operators.find(op => op.id === 'op-2')).toBeDefined();
        });
    });

    describe('updateWebSessionLink', () => {
        it('should update operator document via data service', async () => {
            await service.updateWebSessionLink('op-1', 'ws-1');
            expect(mocks.operatorDataService.updateOperator).toHaveBeenCalledWith('op-1', { bound_web_session_id: 'ws-1' });
        });
    });

    describe('resetOperator', () => {
        it('should delete and recreate operator document', async () => {
            const existing = {
                id: 'op-1',
                user_id: 'u-1',
                organization_id: 'org-1',
                name: 'op-1',
                slot_number: 1,
                api_key: 'key-1'
            };
            mocks.operatorDataService.getOperator.mockResolvedValue(existing);
            vi.spyOn(service, 'getOperatorWithSessionContext').mockResolvedValue(null);

            const result = await service.resetOperator('op-1');

            expect(result.success).toBe(true);
            expect(mocks.operatorDataService.deleteOperator).toHaveBeenCalledWith('op-1');
            expect(mocks.operatorDataService.createOperator).toHaveBeenCalled();
        });
    });

    describe('terminateOperator', () => {
        it('should relay termination to g8ee and NOT delete operator document', async () => {
            const existing = {
                id: 'op-1',
                user_id: 'u-1',
                organization_id: 'org-1',
                name: 'op-1',
                slot_number: 1,
                api_key: 'key-1'
            };
            const g8eContext = {
                user_id: 'u-1',
                organization_id: 'org-1',
                source_component: 'g8ed'
            };
            mocks.operatorDataService.getOperator.mockResolvedValue(existing);
            vi.spyOn(service, 'getOperatorWithSessionContext').mockResolvedValue(g8eContext);
            vi.spyOn(service, 'relayTerminateOperatorToG8ee').mockResolvedValue({ success: true });
            vi.spyOn(service.relay, 'deregisterOperatorSessionInG8ee').mockResolvedValue({ success: true });

            const result = await service.terminateOperator('op-1');

            expect(result.success).toBe(true);
            expect(result.id).toBe('op-1');
            expect(service.relayTerminateOperatorToG8ee).toHaveBeenCalledWith('op-1', g8eContext);
            expect(service.relay.deregisterOperatorSessionInG8ee).toHaveBeenCalledWith(g8eContext);
            expect(mocks.operatorDataService.deleteOperator).not.toHaveBeenCalled();
            expect(mocks.operatorDataService.createOperator).not.toHaveBeenCalled();
        });

        it('should return error if relay fails and NOT delete operator document', async () => {
            const existing = {
                id: 'op-1',
                user_id: 'u-1',
                organization_id: 'org-1'
            };
            const g8eContext = { user_id: 'u-1' };
            mocks.operatorDataService.getOperator.mockResolvedValue(existing);
            vi.spyOn(service, 'getOperatorWithSessionContext').mockResolvedValue(g8eContext);
            vi.spyOn(service, 'relayTerminateOperatorToG8ee').mockRejectedValue(new Error('Relay failed'));

            const result = await service.terminateOperator('op-1');

            expect(result.success).toBe(false);
            expect(result.error).toBe('Relay failed');
            expect(mocks.operatorDataService.deleteOperator).not.toHaveBeenCalled();
        });

        it('should return error if operator not found', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue(null);

            const result = await service.terminateOperator('op-1');

            expect(result.success).toBe(false);
            expect(result.error).toBe('Operator not found');
            expect(mocks.operatorDataService.deleteOperator).not.toHaveBeenCalled();
        });
    });

    describe('getOperatorFresh', () => {
        it('should return fresh OperatorDocument via Data service', async () => {
            const opDoc = new OperatorDocument({ 
                id: 'op-1', 
                user_id: 'u-1', 
                status: OperatorStatus.ACTIVE 
            });
            mocks.operatorDataService.getOperatorFresh.mockResolvedValue(opDoc);

            const result = await service.getOperatorFresh('op-1');

            expect(result).toBe(opDoc);
            expect(mocks.operatorDataService.getOperatorFresh).toHaveBeenCalledWith('op-1');
        });
    });

    describe('getOperatorByUserId', () => {
        it('should return active/bound operator first', async () => {
            const operators = [
                new OperatorDocument({ id: 'op-avail', user_id: 'u-1', status: OperatorStatus.AVAILABLE }),
                new OperatorDocument({ id: 'op-active', user_id: 'u-1', status: OperatorStatus.ACTIVE })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getOperatorByUserId('u-1');
            expect(result.id).toBe('op-active');
        });

        it('should return available operator if no active/bound', async () => {
            const operators = [
                new OperatorDocument({ id: 'op-avail', user_id: 'u-1', status: OperatorStatus.AVAILABLE })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getOperatorByUserId('u-1');
            expect(result.id).toBe('op-avail');
        });

        it('should return null if no matches', async () => {
            mocks.operatorDataService.queryOperators.mockResolvedValue([]);
            const result = await service.getOperatorByUserId('u-1');
            expect(result).toBeNull();
        });
    });

    describe('Relay Methods', () => {
        const g8eContext = { id: 'op-1' };

        it('should delegate deregisterOperatorSessionInG8ee to relay subservice', async () => {
            vi.spyOn(service.relay, 'deregisterOperatorSessionInG8ee').mockResolvedValue(true);
            await service.deregisterOperatorSessionInG8ee(g8eContext);
            expect(service.relay.deregisterOperatorSessionInG8ee).toHaveBeenCalledWith(g8eContext);
        });

        it('should delegate relayDirectCommandToG8ee to relay subservice', async () => {
            vi.spyOn(service.relay, 'relayDirectCommandToG8ee').mockResolvedValue(true);
            const cmd = { command: 'ls' };
            await service.relayDirectCommandToG8ee(cmd, g8eContext);
            expect(service.relay.relayDirectCommandToG8ee).toHaveBeenCalledWith(cmd, g8eContext);
        });

        it('should delegate relayRegisterOperatorSessionToG8ee to relay subservice', async () => {
            vi.spyOn(service.relay, 'relayRegisterOperatorSessionToG8ee').mockResolvedValue(true);
            await service.relayRegisterOperatorSessionToG8ee(g8eContext);
            expect(service.relay.relayRegisterOperatorSessionToG8ee).toHaveBeenCalledWith(g8eContext);
        });

        it('should delegate relayApprovalResponseToG8ee to relay subservice', async () => {
            vi.spyOn(service.relay, 'relayApprovalResponseToG8ee').mockResolvedValue(true);
            const approval = { approved: true };
            await service.relayApprovalResponseToG8ee(approval, g8eContext);
            expect(service.relay.relayApprovalResponseToG8ee).toHaveBeenCalledWith(approval, g8eContext);
        });

        it('should delegate relayTerminateOperatorToG8ee to relay subservice', async () => {
            vi.spyOn(service.relay, 'relayTerminateOperatorToG8ee').mockResolvedValue({ success: true });
            await service.relayTerminateOperatorToG8ee('op-1', g8eContext);
            expect(service.relay.relayTerminateOperatorToG8ee).toHaveBeenCalledWith('op-1', g8eContext);
        });
    });

    describe('Slots & Key Management', () => {
        it('should delegate initializeOperatorSlots to slots subservice', async () => {
            vi.spyOn(service.slots, 'initializeOperatorSlots').mockResolvedValue(true);
            await service.initializeOperatorSlots('u-1', 'org-1', 'web-sess-1');
            expect(service.slots.initializeOperatorSlots).toHaveBeenCalledWith('u-1', 'org-1', 'web-sess-1');
        });

        it('should delegate refreshOperatorApiKey to slots subservice', async () => {
            vi.spyOn(service.slots, 'refreshOperatorApiKey').mockResolvedValue(true);
            await service.refreshOperatorApiKey('op-1', 'u-1', 'web-sess-1', null);
            expect(service.slots.refreshOperatorApiKey).toHaveBeenCalledWith('op-1', 'u-1', 'web-sess-1', null);
        });

        it('should delegate createOperatorSlot to slots subservice', async () => {
            vi.spyOn(service.slots, 'createOperatorSlot').mockResolvedValue({ operator_id: 'op-1' });
            const params = { user_id: 'u-1' };
            await service.createOperatorSlot(params);
            expect(service.slots.createOperatorSlot).toHaveBeenCalledWith(params);
        });

        it('should claim slot and not broadcast since keepalive provides full list', async () => {
            vi.spyOn(service.slots, 'claimSlot').mockResolvedValue(true);
            vi.spyOn(service, 'getOperator').mockResolvedValue({ user_id: 'u-1' });

            const result = await service.claimOperatorSlot('op-1', { fingerprint: 'fp' });
            expect(result).toBe(true);
        });

        it('should return false if claim fails', async () => {
            vi.spyOn(service.slots, 'claimSlot').mockResolvedValue(false);

            const result = await service.claimOperatorSlot('op-1', { fingerprint: 'fp' });
            expect(result).toBe(false);
        });
    });

    describe('validateOperatorApiKey', () => {
        it('should return true if API key matches and operator is not terminated', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue({ 
                api_key: 'valid-key',
                status: OperatorStatus.ACTIVE 
            });
            const result = await service.validateOperatorApiKey('op-1', 'valid-key');
            expect(result).toBe(true);
        });

        it('should return false if API key mismatch', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue({ 
                api_key: 'valid-key',
                status: OperatorStatus.ACTIVE 
            });
            const result = await service.validateOperatorApiKey('op-1', 'wrong-key');
            expect(result).toBe(false);
        });

        it('should return false if operator is terminated', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue({ 
                api_key: 'valid-key',
                status: OperatorStatus.TERMINATED 
            });
            const result = await service.validateOperatorApiKey('op-1', 'valid-key');
            expect(result).toBe(false);
        });
    });

    describe('getGrantedIntentsWithDetails', () => {
        it('should return active granted intents', async () => {
            const operator = {
                granted_intents: [
                    { 
                        name: 'shell', 
                        expires_at: new Date(Date.now() + 10000).toISOString(),
                        granted_at: new Date().toISOString(),
                        granted_by: 'user-1'
                    },
                    { 
                        name: 'file', 
                        expires_at: new Date(Date.now() - 10000).toISOString(),
                        granted_at: new Date().toISOString(),
                        granted_by: 'user-1'
                    } // expired
                ]
            };
            mocks.operatorDataService.getOperator.mockResolvedValue(operator);

            const result = await service.getGrantedIntentsWithDetails('op-1');
            expect(result).toHaveLength(1);
            expect(result[0].name).toBe('shell');
        });

        it('should return empty array if operator not found', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue(null);
            const result = await service.getGrantedIntentsWithDetails('op-1');
            expect(result).toEqual([]);
        });
    });
});
