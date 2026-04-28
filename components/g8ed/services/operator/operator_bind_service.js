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
     */
    constructor({ operatorService, bindingService, webSessionService, sseService }) {
        if (!operatorService) throw new Error('operatorService is required');
        if (!bindingService) throw new Error('bindingService is required');
        if (!webSessionService) throw new Error('webSessionService is required');

        this.operatorService = operatorService;
        this.bindingService = bindingService;
        this.webSessionService = webSessionService;
        this.sseService = sseService;
    }

    async bindOperator(bindReq) {
        return this.bindOperators(bindReq);
    }

    async bindOperators(bindReq) {
        const { operator_ids: operatorIds, web_session_id: webSessionId, user_id: userId } = bindReq;
        const currentBoundSessionIds = await this.bindingService.getBoundOperatorSessionIds(webSessionId);

        logger.info('[OPERATOR-BIND-SERVICE] Starting bind operation', {
            user_id: userId,
            operator_count: operatorIds.length,
            web_session_id_tag: sessionIdTag(webSessionId),
        });

        const bound = [];
        const failed = [];
        const errors = [];

        for (const operatorId of operatorIds) {
            try {
                const operator = await this.operatorService.getOperator(operatorId);

                if (!operator) {
                    throw new Error(DeviceLinkError.OPERATOR_NOT_FOUND);
                }

                if (operator.user_id !== userId) {
                    throw new Error(DeviceLinkError.OPERATOR_WRONG_USER);
                }

                const operatorSessionId = operator.operator_session_id;
                if (!operatorSessionId) {
                    throw new Error('Operator has no active session');
                }

                if (currentBoundSessionIds.includes(operatorSessionId)) {
                    bound.push(operatorId);
                    continue;
                }

                // 1. Use g8ee for operator binding to enforce architectural boundary
                // g8ed should not write to operators after auth
                const g8eContext = G8eHttpContext.parse({
                    web_session_id: webSessionId,
                    user_id: userId,
                    organization_id: operator.organization_id,
                    source_component: SourceComponent.G8ED,
                });

                const relayParams = {
                    operator_ids: [operatorId],
                    web_session_id: webSessionId,
                    user_id: userId,
                };

                const response = await this.operatorService.relayBindOperatorsToG8ee(relayParams, g8eContext);
                
                if (!response.success || response.failed_count > 0) {
                    throw new Error(response.errors?.[0]?.error || 'Failed to bind operator via g8ee');
                }

                // 2. Update Web Session document
                await this.webSessionService.bindOperatorToWebSession(webSessionId, operatorId);

                // 3. Link sessions in KV & Durability
                await this.bindingService.bind(operatorSessionId, webSessionId, userId, operatorId);

                // 4. Relay to g8ee
                const contextWrapper = await this.operatorService.getOperatorWithSessionContext(operatorId);
                if (contextWrapper) {
                    const g8eContext = G8eHttpContext.parse({
                        web_session_id:      webSessionId,
                        user_id:             userId,
                        organization_id:     contextWrapper.organization_id,
                        case_id:             contextWrapper.case_id,
                        investigation_id:    contextWrapper.investigation_id,
                        task_id:             contextWrapper.task_id,
                        bound_operators: [
                            BoundOperatorContext.parse({
                                operator_id:         contextWrapper.id,
                                operator_session_id: contextWrapper.operator_session_id,
                                bound_web_session_id: contextWrapper.bound_web_session_id,
                                status:              contextWrapper.status,
                                operator_type:       contextWrapper.operator_type,
                            })
                        ]
                    });
                    await this.operatorService.relayRegisterOperatorSessionToG8ee(g8eContext).catch(() => {});
                }
                bound.push(operatorId);

            } catch (err) {
                logger.error('[OPERATOR-BIND-SERVICE] Failed to bind operator', {
                    operator_id: operatorId,
                    error: err.message,
                });
                failed.push(operatorId);
                errors.push({ operator_id: operatorId, error: err.message });
            }
        }

        const success = bound.length > 0 || failed.length === 0;
        const statusCode = failed.length === operatorIds.length ? 400 : (failed.length > 0 ? 207 : 200);

        // Emit updated operator list after successful bind
        if (bound.length > 0 && this.sseService) {
            try {
                const operatorList = await this.operatorService.getUserOperators(userId);
                await this.sseService.publishEvent(webSessionId, operatorList);
            } catch (error) {
                logger.warn('[OPERATOR-BIND-SERVICE] Failed to emit operator list after bind', { error: error.message });
            }
        }

        return new BindOperatorsResponse({
            success,
            statusCode,
            bound_count: bound.length,
            failed_count: failed.length,
            bound_operator_ids: bound,
            failed_operator_ids: failed,
            errors: errors.length > 0 ? errors : [],
        });
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
        
        logger.info('[OPERATOR-BIND-SERVICE] Starting unbind operation', {
            user_id: userId,
            operator_count: operatorIds.length,
            web_session_id_tag: sessionIdTag(webSessionId),
        });

        const unbound = [];
        const failed = [];
        const errors = [];

        for (const operatorId of operatorIds) {
            try {
                const operator = await this.operatorService.getOperator(operatorId);

                if (!operator) {
                    throw new Error(DeviceLinkError.OPERATOR_NOT_FOUND);
                }

                if (operator.user_id !== userId) {
                    throw new Error('Not authorized to unbind this operator');
                }

                const operatorSessionId = operator.operator_session_id;

                // 1. Use g8ee for operator unbinding to enforce architectural boundary
                // g8ed should not write to operators after auth
                const g8eContext = G8eHttpContext.parse({
                    web_session_id: webSessionId,
                    user_id: userId,
                    organization_id: operator.organization_id,
                    source_component: SourceComponent.G8ED,
                });

                const relayParams = {
                    operator_ids: [operatorId],
                    web_session_id: webSessionId,
                    user_id: userId,
                };

                const response = await this.operatorService.relayUnbindOperatorsToG8ee(relayParams, g8eContext);
                
                if (!response.success || response.failed_count > 0) {
                    throw new Error(response.errors?.[0]?.error || 'Failed to unbind operator via g8ee');
                }

                // 2. Update Web Session document
                await this.webSessionService.unbindOperatorFromWebSession(webSessionId, operatorId);

                // 3. Unlink sessions in KV & Durability
                if (operatorSessionId) {
                    await this.bindingService.unbind(operatorSessionId, webSessionId, operatorId);
                }
                
                // 4. Relay to g8ee
                const contextWrapper = await this.operatorService.getOperatorWithSessionContext(operatorId);
                if (contextWrapper) {
                    const g8eContext = G8eHttpContext.parse({
                        web_session_id:      webSessionId,
                        user_id:             userId,
                        organization_id:     contextWrapper.organization_id,
                        case_id:             contextWrapper.case_id,
                        investigation_id:    contextWrapper.investigation_id,
                        task_id:             contextWrapper.task_id,
                        bound_operators: [
                            BoundOperatorContext.parse({
                                operator_id:         contextWrapper.id,
                                operator_session_id: contextWrapper.operator_session_id,
                                bound_web_session_id: contextWrapper.bound_web_session_id,
                                status:              contextWrapper.status,
                                operator_type:       contextWrapper.operator_type,
                            })
                        ]
                    });
                    await this.operatorService.relayRegisterOperatorSessionToG8ee(g8eContext).catch(() => {});
                }
                
                unbound.push(operatorId);

            } catch (err) {
                logger.error('[OPERATOR-BIND-SERVICE] Failed to unbind operator', {
                    operator_id: operatorId,
                    error: err.message,
                });
                failed.push(operatorId);
                errors.push({ operator_id: operatorId, error: err.message });
            }
        }

        const success = unbound.length > 0 || failed.length === 0;
        const statusCode = failed.length === operatorIds.length ? 400 : (failed.length > 0 ? 207 : 200);

        // Emit updated operator list after successful unbind
        if (unbound.length > 0 && this.sseService) {
            try {
                const operatorList = await this.operatorService.getUserOperators(userId);
                await this.sseService.publishEvent(webSessionId, operatorList);
            } catch (error) {
                logger.warn('[OPERATOR-BIND-SERVICE] Failed to emit operator list after unbind', { error: error.message });
            }
        }

        return new UnbindOperatorsResponse({
            success,
            statusCode,
            unbound_count: unbound.length,
            failed_count: failed.length,
            unbound_operator_ids: unbound,
            failed_operator_ids: failed,
            errors: errors.length > 0 ? errors : [],
        });
    }
}
