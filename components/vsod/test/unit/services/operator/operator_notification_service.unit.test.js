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
import { OperatorNotificationService } from '@vsod/services/operator/operator_notification_service.js';
import { OperatorStatus } from '@vsod/constants/operator.js';
import { EventType } from '@vsod/constants/events.js';
import { OperatorDocument, OperatorListUpdatedEvent } from '@vsod/models/operator_model.js';

describe('OperatorNotificationService', () => {
    let service;
    let mocks;

    beforeEach(() => {
        mocks = {
            webSessionService: {
                getUserActiveSessions: vi.fn(),
            },
            operatorDataService: {
                queryOperators: vi.fn(),
            },
            sseService: {
                publishEvent: vi.fn().mockResolvedValue(true),
            },
        };

        service = new OperatorNotificationService(mocks);
    });

    const calculateSlotUsageFn = (ops) => ({ usedSlots: ops.filter(op => op.status === OperatorStatus.ACTIVE).length });

    describe('broadcastOperatorListToUser', () => {
        it('should broadcast updated list to all active user web sessions', async () => {
            const userId = 'u-123';
            const operators = [
                new OperatorDocument({ operator_id: 'op-1', status: OperatorStatus.ACTIVE, user_id: userId }),
                new OperatorDocument({ operator_id: 'op-2', status: OperatorStatus.OFFLINE, user_id: userId }),
                new OperatorDocument({ operator_id: 'op-3', status: OperatorStatus.TERMINATED, user_id: userId }),
            ];

            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);
            mocks.webSessionService.getUserActiveSessions.mockResolvedValue(['ws-1', 'ws-2']);

            await service.broadcastOperatorListToUser(userId, calculateSlotUsageFn);

            expect(mocks.sseService.publishEvent).toHaveBeenCalledTimes(2);
            expect(mocks.sseService.publishEvent).toHaveBeenCalledWith('ws-1', expect.any(OperatorListUpdatedEvent));

            const event = mocks.sseService.publishEvent.mock.calls[0][1];
            expect(event.operators).toHaveLength(2); // op-3 is filtered out
            expect(event.active_count).toBe(1);
        });

        it('should do nothing if user has no active sessions', async () => {
            mocks.operatorDataService.queryOperators.mockResolvedValue([]);
            mocks.webSessionService.getUserActiveSessions.mockResolvedValue([]);

            await service.broadcastOperatorListToUser('u-123', calculateSlotUsageFn);

            expect(mocks.sseService.publishEvent).not.toHaveBeenCalled();
        });

        it('should handle errors gracefully', async () => {
            mocks.operatorDataService.queryOperators.mockRejectedValue(new Error('DB Error'));
            
            await expect(service.broadcastOperatorListToUser('u-123', calculateSlotUsageFn)).resolves.not.toThrow();
        });
    });

    describe('broadcastOperatorListToSession', () => {
        it('should broadcast updated list to a specific web session', async () => {
            const userId = 'u-123';
            const webSessionId = 'ws-123';
            const operators = [
                new OperatorDocument({ operator_id: 'op-1', status: OperatorStatus.ACTIVE, user_id: userId }),
            ];

            mocks.operatorDataService.queryOperators.mockResolvedValue(operators);

            await service.broadcastOperatorListToSession(userId, webSessionId, calculateSlotUsageFn);

            expect(mocks.sseService.publishEvent).toHaveBeenCalledTimes(1);
            expect(mocks.sseService.publishEvent).toHaveBeenCalledWith(webSessionId, expect.any(OperatorListUpdatedEvent));
        });

        it('should not throw if sseService is missing (race condition safety)', async () => {
            const unstableService = new OperatorNotificationService({
                webSessionService: mocks.webSessionService,
                operatorDataService: mocks.operatorDataService,
                sseService: null
            });

            await expect(unstableService.broadcastOperatorListToSession('u-1', 'ws-1', calculateSlotUsageFn))
                .resolves.not.toThrow();
        });

        it('should handle missing webSessionId gracefully', async () => {
            await service.broadcastOperatorListToSession('u-1', null, calculateSlotUsageFn);
            expect(mocks.operatorDataService.queryOperators).not.toHaveBeenCalled();
        });
    });
});
