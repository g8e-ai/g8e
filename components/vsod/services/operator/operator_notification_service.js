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

import { logger } from '../../utils/logger.js';
import { OperatorStatus } from '../../constants/operator.js';
import { EventType } from '../../constants/events.js';
import { OperatorListUpdatedEvent } from '../../models/operator_model.js';
import { now } from '../../models/base.js';
import { redactWebSessionId } from '../../utils/security.js';

export class OperatorNotificationService {
    /**
     * @param {Object} options
     * @param {Object} options.webSessionService - WebSessionService instance
     * @param {Object} options.operatorDataService - OperatorDataService instance
     * @param {Object} options.sseService - SSEService instance
     */
    constructor({ webSessionService, operatorDataService, sseService }) {
        this.webSessionService = webSessionService;
        this.operatorDataService = operatorDataService;
        this.sseService = sseService;
    }

    async broadcastOperatorListToUser(userId, calculateSlotUsageFn) {
        try {
            const operators = await this.operatorDataService.queryOperators(
                [{ field: 'user_id', operator: '==', value: userId }]
            );

            const visibleOperators = operators.filter(op =>
                op.status !== OperatorStatus.UNAVAILABLE &&
                op.status !== OperatorStatus.TERMINATED
            );

            const enhancedOperators = visibleOperators.map((op) => {
                const s = op.status ?? OperatorStatus.OFFLINE;
                return { ...op, status_display: s, status_class: s === OperatorStatus.OFFLINE ? 'inactive' : s.toLowerCase() };
            });

            const activeCount = enhancedOperators.filter(op =>
                op.status === OperatorStatus.ACTIVE || op.status === OperatorStatus.BOUND
            ).length;

            const { usedSlots } = calculateSlotUsageFn(enhancedOperators);

            const userWebSessions = await this.webSessionService.getUserActiveSessions(userId);
            if (userWebSessions.length === 0) return;

            const event = OperatorListUpdatedEvent.parse({
                type: EventType.OPERATOR_PANEL_LIST_UPDATED,
                operators: enhancedOperators,
                total_count: enhancedOperators.length,
                active_count: activeCount,
                used_slots: usedSlots,
                max_slots: enhancedOperators.length,
                timestamp: now(),
            });

            for (const webSessionId of userWebSessions) {
                await this.sseService.publishEvent(webSessionId, event);
            }
        } catch (error) {
            logger.error('[OPERATOR-NOTIFY] Failed to broadcast Operator list', { userId, error: error.message });
        }
    }

    async broadcastOperatorListToSession(userId, webSessionId, calculateSlotUsageFn) {
        try {
            if (!webSessionId) return;
            if (!this.sseService) {
                logger.error('[OPERATOR-NOTIFY] sseService is missing in OperatorNotificationService');
                return;
            }

            const allOperators = await this.operatorDataService.queryOperators(
                [{ field: 'user_id', operator: '==', value: userId }]
            );

            const operators = allOperators.filter(op =>
                op.status !== OperatorStatus.UNAVAILABLE &&
                op.status !== OperatorStatus.TERMINATED
            );

            const enhancedOperators = operators.map((op) => {
                const s = op.status ?? OperatorStatus.OFFLINE;
                return { ...op, status_display: s, status_class: s === OperatorStatus.OFFLINE ? 'inactive' : s.toLowerCase() };
            });

            const activeCount = enhancedOperators.filter(op =>
                op.status === OperatorStatus.ACTIVE || op.status === OperatorStatus.BOUND
            ).length;

            const { usedSlots } = calculateSlotUsageFn(enhancedOperators);

            const event = OperatorListUpdatedEvent.parse({
                type: EventType.OPERATOR_PANEL_LIST_UPDATED,
                operators: enhancedOperators,
                total_count: enhancedOperators.length,
                active_count: activeCount,
                used_slots: usedSlots,
                max_slots: enhancedOperators.length,
                timestamp: now(),
            });

            await this.sseService.publishEvent(webSessionId, event);
        } catch (error) {
            logger.error('[OPERATOR-NOTIFY] Failed to broadcast to session', { userId, webSessionId: redactWebSessionId(webSessionId), error: error.message });
        }
    }
}
