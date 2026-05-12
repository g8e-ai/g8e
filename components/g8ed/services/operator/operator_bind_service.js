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
import { sessionIdTag } from '../../utils/session_log.js';
import { DeviceLinkError } from '../../constants/auth.js';
import { OperatorStatus } from '../../constants/operator.js';
import { EventType } from '../../constants/events.js';
import { SourceComponent } from '../../constants/ai.js';
import { G8eHttpContext, BoundOperatorContext, UnbindOperatorsRequest } from '../../models/request_models.js';
import {
    BindOperatorsResponse,
    UnbindOperatorsResponse,
} from '../../models/operator_model.js';

export class BindOperatorsService {
    /**
     * @param {Object} options
     * @param {Object} options.operatorService - OperatorService (Domain Layer) instance
     * @param {Object} options.bindingService - BoundSessionsService instance
     * @param {Object} options.webSessionService - WebSessionService instance
     * @param {Object} options.sseService - SSEService instance (optional)
     * @param {Object} options.internalHttpClient - InternalHttpClient instance
     */
    constructor({ operatorService, bindingService, webSessionService, sseService, internalHttpClient }) {
        if (!operatorService) throw new Error('operatorService is required');
        if (!bindingService) throw new Error('bindingService is required');
        if (!webSessionService) throw new Error('webSessionService is required');
        if (!internalHttpClient) throw new Error('internalHttpClient is required');

        this.operatorService = operatorService;
        this.bindingService = bindingService;
        this.webSessionService = webSessionService;
        this.sseService = sseService;
        this.internalHttpClient = internalHttpClient;
    }

    async bindOperator(bindReq) {
        return this.bindOperators(bindReq);
    }

    async bindOperators(bindReq) {
        const { operator_ids: operatorIds, web_session_id: webSessionId, user_id: userId } = bindReq;

        logger.info('[OPERATOR-BIND-SERVICE] Starting bind operation via substrate', {
            user_id: userId,
            operator_count: operatorIds.length,
            web_session_id_tag: sessionIdTag(webSessionId),
        });

        try {
            // Call substrate-owned binding route
            const response = await this.internalHttpClient.bindOperators(operatorIds, userId, webSessionId);
            
            if (!response.success) {
                throw new Error(response.error || 'Failed to bind operators via substrate');
            }

            // Sync local cache/state if needed
            // For now we still invalidate local cache to ensure next read reflects substrate state
            for (const operatorId of operatorIds) {
                await this.bindingService.evictDocument(this.bindingService.collection, webSessionId);
                await this.operatorService.evictOperator(operatorId);
            }

            // Emit updated operator list after successful bind
            if (this.sseService) {
                try {
                    const operatorList = await this.operatorService.getUserOperators(userId);
                    await this.sseService.publishEvent(webSessionId, operatorList);
                } catch (error) {
                    logger.warn('[OPERATOR-BIND-SERVICE] Failed to emit operator list after bind', { error: error.message });
                }
            }

            return new BindOperatorsResponse({
                success: response.success,
                statusCode: response.failed_count > 0 ? 207 : 200,
                bound_count: response.bound_count,
                failed_count: response.failed_count,
                bound_operator_ids: response.bound_operator_ids,
                failed_operator_ids: response.failed_operator_ids,
                errors: response.error ? [{ error: response.error }] : [],
            });

        } catch (err) {
            logger.error('[OPERATOR-BIND-SERVICE] Failed to bind operators via substrate', {
                error: err.message,
            });
            return new BindOperatorsResponse({
                success: false,
                statusCode: 500,
                errors: [{ error: err.message }],
            });
        }
    }

    async unbindOperator(unbindReq) {
        if (!(unbindReq instanceof UnbindOperatorsRequest)) {
            unbindReq = UnbindOperatorsRequest.parse(unbindReq);
        }
        return this.unbindOperators(unbindReq);
    }

    async unbindOperators(unbindReq) {
        if (!(unbindReq instanceof UnbindOperatorsRequest)) {
            unbindReq = UnbindOperatorsRequest.parse(unbindReq);
        }
        const { operator_ids: operatorIds, web_session_id: webSessionId, user_id: userId } = unbindReq;
        
        logger.info('[OPERATOR-BIND-SERVICE] Starting unbind operation via substrate', {
            user_id: userId,
            operator_count: operatorIds.length,
            web_session_id_tag: sessionIdTag(webSessionId),
        });

        try {
            // Call substrate-owned unbinding route
            const response = await this.internalHttpClient.unbindOperators(operatorIds, userId, webSessionId);
            
            if (!response.success) {
                throw new Error(response.error || 'Failed to unbind operators via substrate');
            }

            // Sync local cache/state if needed
            for (const operatorId of operatorIds) {
                await this.bindingService.evictDocument(this.bindingService.collection, webSessionId);
                await this.operatorService.evictOperator(operatorId);
            }

            // Emit updated operator list after successful unbind
            if (this.sseService) {
                try {
                    const operatorList = await this.operatorService.getUserOperators(userId);
                    await this.sseService.publishEvent(webSessionId, operatorList);
                } catch (error) {
                    logger.warn('[OPERATOR-BIND-SERVICE] Failed to emit operator list after unbind', { error: error.message });
                }
            }

            return new UnbindOperatorsResponse({
                success: response.success,
                statusCode: response.failed_count > 0 ? 207 : 200,
                unbound_count: response.unbound_count,
                failed_count: response.failed_count,
                unbound_operator_ids: response.unbound_operator_ids,
                failed_operator_ids: response.failed_operator_ids,
                errors: response.error ? [{ error: response.error }] : [],
            });

        } catch (err) {
            logger.error('[OPERATOR-BIND-SERVICE] Failed to unbind operators via substrate', {
                error: err.message,
            });
            return new UnbindOperatorsResponse({
                success: false,
                statusCode: 500,
                errors: [{ error: err.message }],
            });
        }
    }
}
