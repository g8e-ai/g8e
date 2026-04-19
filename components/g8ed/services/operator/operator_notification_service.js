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
import { OperatorPanelListUpdatedEvent } from '../../models/sse_models.js';
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
        // Keepalive now provides full operator list - this method is no-op
        // Individual operator updates use broadcastOperatorContext instead
        return;
    }

    async broadcastOperatorListToSession(userId, webSessionId, calculateSlotUsageFn) {
        // Keepalive now provides full operator list - this method is no-op
        // Individual operator updates use broadcastOperatorContext instead
        return;
    }

    async broadcastOperatorContext(webSessionId, operatorId, context) {
        try {
            if (!webSessionId || !this.sseService) return;

            const event = new OperatorPanelListUpdatedEvent({
                type: EventType.OPERATOR_PANEL_LIST_UPDATED,
                data: {
                    operator_id: operatorId,
                    case_id: context.case_id || null,
                    investigation_id: context.investigation_id || null,
                    task_id: context.task_id || null,
                    timestamp: now(),
                },
            });

            await this.sseService.publishEvent(webSessionId, event);
        } catch (error) {
            logger.error('[OPERATOR-NOTIFY] Failed to broadcast operator context', { 
                operatorId, 
                webSessionId: redactWebSessionId(webSessionId), 
                error: error.message 
            });
        }
    }
}
