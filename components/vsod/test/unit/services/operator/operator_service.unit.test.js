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
import { OperatorService } from '@vsod/services/operator/operator_service.js';
import { OperatorStatus } from '@vsod/constants/operator.js';
import { OperatorDocument, OperatorWithSessionContext } from '@vsod/models/operator_model.js';
import { EventType } from '@vsod/constants/events.js';

describe('OperatorService', () => {
    let service;
    let mocks;

    beforeEach(() => {
        mocks = {
            operatorDataService: {
                getOperator: vi.fn(),
                getOperatorFresh: vi.fn(),
                queryOperators: vi.fn(),
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
                revokeKey: vi.fn(),
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
        };

        service = new OperatorService(mocks);
    });

    describe('calculateSlotUsage', () => {
        it('should correctly count active and bound operators', () => {
            const operators = [
                { operator_id: 'op-1', status: OperatorStatus.ACTIVE },
                { operator_id: 'op-2', status: OperatorStatus.BOUND },
                { operator_id: 'op-3', status: OperatorStatus.OFFLINE },
                { operator_id: 'op-4', status: OperatorStatus.TERMINATED },
            ];

            const result = service.calculateSlotUsage(operators);

            expect(result.usedSlots).toBe(2);
            expect(result.claimedOperators).toHaveLength(2);
        });
    });

    describe('getOperator', () => {
        it('should return OperatorDocument if found via Data service', async () => {
            const opDoc = new OperatorDocument({ 
                operator_id: 'op-1', 
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
                operator_id: 'op-1', 
                user_id: 'u-1',
                status: OperatorStatus.ACTIVE,
                operator_session_id: 'os-1', 
                web_session_id: 'ws-1' 
            });
            mocks.operatorDataService.getOperator.mockResolvedValue(operator);
            
            const opSession = { id: 'os-1', user_id: 'u-1' };
            const webSession = { id: 'ws-1', user_id: 'u-1' };
            
            mocks.operatorSessionService.validateSession.mockResolvedValue(opSession);
            mocks.webSessionService.validateSession.mockResolvedValue(webSession);

            const result = await service.getOperatorWithSessionContext('op-1');

            expect(result).toBeInstanceOf(OperatorWithSessionContext);
            expect(result.operator_id).toBe('op-1');
            expect(result.operator_session_id).toBe('os-1');
            expect(result.web_session_id).toBe('ws-1');
        });
    });

    describe('Lifecycle & Relay Orchestration', () => {
        it('should relay stop command to g8ee', async () => {
            const operator = new OperatorDocument({ operator_id: 'op-1', user_id: 'u-1', status: OperatorStatus.ACTIVE });
            const context = OperatorWithSessionContext.create(operator);
            
            vi.spyOn(service.relay, 'relayStopCommandToG8ee').mockResolvedValue(true);

            const result = await service.relayStopCommandToG8ee(context);

            expect(result).toBe(true);
            expect(service.relay.relayStopCommandToG8ee).toHaveBeenCalledWith(context);
        });
    });

    describe('getUserOperators', () => {
        it('should return enhanced operator list payload for UI', async () => {
            const operators = [
                new OperatorDocument({ operator_id: 'op-1', user_id: 'u-1', status: OperatorStatus.ACTIVE }),
                new OperatorDocument({ operator_id: 'op-2', user_id: 'u-1', status: OperatorStatus.AVAILABLE })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getUserOperators('u-1');

            expect(result.type).toBe(EventType.OPERATOR_PANEL_LIST_UPDATED);
            expect(result.operators).toHaveLength(2);
            expect(result.active_count).toBe(1);
            expect(result.used_slots).toBe(1);
            expect(result.operators[0].status_display).toBe(OperatorStatus.ACTIVE);
            expect(result.operators[0].status_class).toBe('active');
        });
    });

    describe('syncSessionOnConnect', () => {
        it('should re-bind operators and broadcast list', async () => {
            const userId = 'u-1';
            const webSessionId = 'ws-new';
            const operators = [
                { operator_id: 'op-1', status: OperatorStatus.BOUND, web_session_id: 'ws-old' },
                { operator_id: 'op-2', status: OperatorStatus.ACTIVE }
            ];
            
            vi.spyOn(service, 'getUserVisibleOperatorStats').mockResolvedValue({ operators });
            vi.spyOn(service, 'updateWebSessionLink').mockResolvedValue(true);
            vi.spyOn(service, 'broadcastOperatorListToSession').mockResolvedValue(true);

            await service.syncSessionOnConnect(userId, webSessionId);

            expect(service.updateWebSessionLink).toHaveBeenCalledWith('op-1', webSessionId, { skipBroadcast: true });
            expect(service.broadcastOperatorListToSession).toHaveBeenCalledWith(userId, webSessionId);
        });
    });

    describe('updateWebSessionLink', () => {
        it('should update operator document via data service', async () => {
            await service.updateWebSessionLink('op-1', 'ws-1');
            expect(mocks.operatorDataService.updateOperator).toHaveBeenCalledWith('op-1', { web_session_id: 'ws-1' });
        });
    });

    describe('resetOperator', () => {
        it('should delete and recreate operator document', async () => {
            const existing = {
                operator_id: 'op-1',
                user_id: 'u-1',
                organization_id: 'org-1',
                name: 'op-1',
                slot_number: 1,
                api_key: 'key-1'
            };
            mocks.operatorDataService.getOperator.mockResolvedValue(existing);
            vi.spyOn(service, 'getOperatorWithSessionContext').mockResolvedValue(null);
            vi.spyOn(service, '_broadcastOperatorListToUser').mockResolvedValue(true);

            const result = await service.resetOperator('op-1');

            expect(result.success).toBe(true);
            expect(mocks.operatorDataService.deleteOperator).toHaveBeenCalledWith('op-1');
            expect(mocks.operatorDataService.createOperator).toHaveBeenCalled();
            expect(service._broadcastOperatorListToUser).toHaveBeenCalledWith('u-1');
        });
    });

    describe('getOperatorFresh', () => {
        it('should return fresh OperatorDocument via Data service', async () => {
            const opDoc = new OperatorDocument({ 
                operator_id: 'op-1', 
                user_id: 'u-1', 
                status: OperatorStatus.ACTIVE 
            });
            mocks.operatorDataService.getOperatorFresh.mockResolvedValue(opDoc);

            const result = await service.getOperatorFresh('op-1');

            expect(result).toBe(opDoc);
            expect(mocks.operatorDataService.getOperatorFresh).toHaveBeenCalledWith('op-1');
        });
    });

    describe('getOperatorStatusInfo', () => {
        it('should return OperatorStatusInfo if operator exists', async () => {
            const opDoc = new OperatorDocument({ 
                operator_id: 'op-1', 
                user_id: 'u-1', 
                status: OperatorStatus.ACTIVE 
            });
            mocks.operatorDataService.getOperator.mockResolvedValue(opDoc);

            const result = await service.getOperatorStatusInfo('op-1');

            expect(result).not.toBeNull();
            expect(result.operator_id).toBe('op-1');
            expect(result.status).toBe(OperatorStatus.ACTIVE);
        });

        it('should return null if operator does not exist', async () => {
            mocks.operatorDataService.getOperator.mockResolvedValue(null);
            const result = await service.getOperatorStatusInfo('op-1');
            expect(result).toBeNull();
        });
    });

    describe('getOperatorByUserId', () => {
        it('should return active/bound operator first', async () => {
            const operators = [
                new OperatorDocument({ operator_id: 'op-avail', user_id: 'u-1', status: OperatorStatus.AVAILABLE }),
                new OperatorDocument({ operator_id: 'op-active', user_id: 'u-1', status: OperatorStatus.ACTIVE })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getOperatorByUserId('u-1');
            expect(result.operator_id).toBe('op-active');
        });

        it('should return available operator if no active/bound', async () => {
            const operators = [
                new OperatorDocument({ operator_id: 'op-avail', user_id: 'u-1', status: OperatorStatus.AVAILABLE })
            ];
            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            const result = await service.getOperatorByUserId('u-1');
            expect(result.operator_id).toBe('op-avail');
        });

        it('should return null if no matches', async () => {
            mocks.operatorDataService.queryOperators.mockResolvedValue([]);
            const result = await service.getOperatorByUserId('u-1');
            expect(result).toBeNull();
        });
    });

    describe('Relay Methods', () => {
        const vsoContext = { operator_id: 'op-1' };

        it('should delegate deregisterOperatorSessionInG8ee to relay subservice', async () => {
            vi.spyOn(service.relay, 'deregisterOperatorSessionInG8ee').mockResolvedValue(true);
            await service.deregisterOperatorSessionInG8ee(vsoContext);
            expect(service.relay.deregisterOperatorSessionInG8ee).toHaveBeenCalledWith(vsoContext);
        });

        it('should delegate relayDirectCommandToG8ee to relay subservice', async () => {
            vi.spyOn(service.relay, 'relayDirectCommandToG8ee').mockResolvedValue(true);
            const cmd = { command: 'ls' };
            await service.relayDirectCommandToG8ee(cmd, vsoContext);
            expect(service.relay.relayDirectCommandToG8ee).toHaveBeenCalledWith(cmd, vsoContext);
        });

        it('should delegate relayRegisterOperatorSessionToG8ee to relay subservice', async () => {
            vi.spyOn(service.relay, 'relayRegisterOperatorSessionToG8ee').mockResolvedValue(true);
            await service.relayRegisterOperatorSessionToG8ee(vsoContext);
            expect(service.relay.relayRegisterOperatorSessionToG8ee).toHaveBeenCalledWith(vsoContext);
        });

        it('should delegate relayApprovalResponseToG8ee to relay subservice', async () => {
            vi.spyOn(service.relay, 'relayApprovalResponseToG8ee').mockResolvedValue(true);
            const approval = { approved: true };
            await service.relayApprovalResponseToG8ee(approval, vsoContext);
            expect(service.relay.relayApprovalResponseToG8ee).toHaveBeenCalledWith(approval, vsoContext);
        });
    });

    describe('Notifications', () => {
        it('should delegate broadcastOperatorListToSession to notifications subservice', async () => {
            vi.spyOn(service.notifications, 'broadcastOperatorListToSession').mockResolvedValue(true);
            await service.broadcastOperatorListToSession('u-1', 'ws-1');
            expect(service.notifications.broadcastOperatorListToSession).toHaveBeenCalledWith('u-1', 'ws-1', expect.any(Function));
        });

        it('should delegate _broadcastOperatorListToUser to notifications subservice', async () => {
            vi.spyOn(service.notifications, 'broadcastOperatorListToUser').mockResolvedValue(true);
            await service._broadcastOperatorListToUser('u-1');
            expect(service.notifications.broadcastOperatorListToUser).toHaveBeenCalledWith('u-1', expect.any(Function));
        });
    });

    describe('Slots & Key Management', () => {
        it('should delegate initializeOperatorSlots to slots subservice', async () => {
            vi.spyOn(service.slots, 'initializeOperatorSlots').mockResolvedValue(true);
            await service.initializeOperatorSlots('u-1', 'org-1');
            expect(service.slots.initializeOperatorSlots).toHaveBeenCalledWith('u-1', 'org-1');
        });

        it('should delegate refreshOperatorApiKey to slots subservice', async () => {
            vi.spyOn(service.slots, 'refreshOperatorApiKey').mockResolvedValue(true);
            await service.refreshOperatorApiKey('op-1', 'u-1');
            expect(service.slots.refreshOperatorApiKey).toHaveBeenCalledWith('op-1', 'u-1', expect.any(Function));
        });

        it('should delegate createOperatorSlot to slots subservice', async () => {
            vi.spyOn(service.slots, 'createOperatorSlot').mockResolvedValue({ operator_id: 'op-1' });
            const params = { user_id: 'u-1' };
            await service.createOperatorSlot(params);
            expect(service.slots.createOperatorSlot).toHaveBeenCalledWith(params);
        });

        it('should claim slot and broadcast update on success', async () => {
            vi.spyOn(service.slots, 'claimSlot').mockResolvedValue(true);
            vi.spyOn(service, 'getOperator').mockResolvedValue({ user_id: 'u-1' });
            vi.spyOn(service, '_broadcastOperatorListToUser').mockResolvedValue(true);

            const result = await service.claimOperatorSlot('op-1', { fingerprint: 'fp' });
            expect(result).toBe(true);
            expect(service._broadcastOperatorListToUser).toHaveBeenCalledWith('u-1');
        });

        it('should not broadcast if claim fails', async () => {
            vi.spyOn(service.slots, 'claimSlot').mockResolvedValue(false);
            vi.spyOn(service, '_broadcastOperatorListToUser');

            const result = await service.claimOperatorSlot('op-1', { fingerprint: 'fp' });
            expect(result).toBe(false);
            expect(service._broadcastOperatorListToUser).not.toHaveBeenCalled();
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
