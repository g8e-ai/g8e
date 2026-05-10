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
            await service.broadcastOperatorListToUser('u-123', calculateSlotUsageFn);

            expect(mocks.sseService.publishEvent).not.toHaveBeenCalled();
            expect(mocks.operatorDataService.queryOperators).not.toHaveBeenCalled();
            expect(mocks.webSessionService.getUserActiveSessions).not.toHaveBeenCalled();
        });
    });

    describe('broadcastOperatorListToSession', () => {
        it('should be no-op since keepalive now provides full operator list', async () => {
            await service.broadcastOperatorListToSession('u-123', 'ws-123', calculateSlotUsageFn);

            expect(mocks.sseService.publishEvent).not.toHaveBeenCalled();
            expect(mocks.operatorDataService.queryOperators).not.toHaveBeenCalled();
        });

        it('should handle missing webSessionId gracefully', async () => {
            await service.broadcastOperatorListToSession('u-1', null, calculateSlotUsageFn);
            expect(mocks.sseService.publishEvent).not.toHaveBeenCalled();
        });
    });

    describe('no sparse OPERATOR_PANEL_LIST_UPDATED publisher', () => {
        it('does not expose broadcastOperatorContext (regression guard)', () => {
            // The removed broadcastOperatorContext published a sparse
            // {operator_id, case_id, investigation_id} payload under
            // OPERATOR_PANEL_LIST_UPDATED, colliding with the full-list
            // shape and wiping the operator panel on every heartbeat.
            expect(service.broadcastOperatorContext).toBeUndefined();
        });
    });
});
