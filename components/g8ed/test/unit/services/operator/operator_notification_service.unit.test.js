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
import { OperatorNotificationService } from '@g8ed/services/operator/operator_notification_service.js';
import { OperatorStatus } from '@g8ed/constants/operator.js';
import { EventType } from '@g8ed/constants/events.js';
import { OperatorPanelListUpdatedEvent } from '@g8ed/models/sse_models.js';

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
        it('should be no-op since keepalive now provides full operator list', async () => {
            const userId = 'u-123';

            await service.broadcastOperatorListToUser(userId, calculateSlotUsageFn);

            expect(mocks.sseService.publishEvent).not.toHaveBeenCalled();
            expect(mocks.operatorDataService.queryOperators).not.toHaveBeenCalled();
            expect(mocks.webSessionService.getUserActiveSessions).not.toHaveBeenCalled();
        });
    });

    describe('broadcastOperatorListToSession', () => {
        it('should be no-op since keepalive now provides full operator list', async () => {
            const userId = 'u-123';
            const webSessionId = 'ws-123';

            await service.broadcastOperatorListToSession(userId, webSessionId, calculateSlotUsageFn);

            expect(mocks.sseService.publishEvent).not.toHaveBeenCalled();
            expect(mocks.operatorDataService.queryOperators).not.toHaveBeenCalled();
        });

        it('should handle missing webSessionId gracefully', async () => {
            await service.broadcastOperatorListToSession('u-1', null, calculateSlotUsageFn);
            expect(mocks.sseService.publishEvent).not.toHaveBeenCalled();
        });
    });

    describe('broadcastOperatorContext', () => {
        it('should broadcast operator context to web session', async () => {
            const webSessionId = 'ws-123';
            const operatorId = 'op-1';
            const context = {
                case_id: 'case-123',
                investigation_id: 'inv-456',
                task_id: 'task-789',
            };

            await service.broadcastOperatorContext(webSessionId, operatorId, context);

            expect(mocks.sseService.publishEvent).toHaveBeenCalledTimes(1);
            expect(mocks.sseService.publishEvent).toHaveBeenCalledWith(webSessionId, expect.any(OperatorPanelListUpdatedEvent));

            const event = mocks.sseService.publishEvent.mock.calls[0][1];
            expect(event.type).toBe(EventType.OPERATOR_PANEL_LIST_UPDATED);
            expect(event.data.operator_id).toBe(operatorId);
            expect(event.data.case_id).toBe(context.case_id);
            expect(event.data.investigation_id).toBe(context.investigation_id);
            expect(event.data.task_id).toBe(context.task_id);
            expect(event.data.timestamp).toBeInstanceOf(Date);
        });

        it('should handle missing webSessionId gracefully', async () => {
            await service.broadcastOperatorContext(null, 'op-1', {});
            expect(mocks.sseService.publishEvent).not.toHaveBeenCalled();
        });

        it('should handle missing sseService gracefully', async () => {
            const unstableService = new OperatorNotificationService({
                webSessionService: mocks.webSessionService,
                operatorDataService: mocks.operatorDataService,
                sseService: null
            });

            await expect(unstableService.broadcastOperatorContext('ws-1', 'op-1', {}))
                .resolves.not.toThrow();
        });

        it('should handle null context fields', async () => {
            const webSessionId = 'ws-123';
            const operatorId = 'op-1';

            await service.broadcastOperatorContext(webSessionId, operatorId, {});

            expect(mocks.sseService.publishEvent).toHaveBeenCalledTimes(1);
            const event = mocks.sseService.publishEvent.mock.calls[0][1];
            expect(event.data.operator_id).toBe(operatorId);
            expect(event.data.case_id).toBeNull();
            expect(event.data.investigation_id).toBeNull();
            expect(event.data.task_id).toBeNull();
        });
    });
});
