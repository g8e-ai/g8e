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
import { DeviceLinkError } from '../../constants/auth.js';
import { OperatorStatus } from '../../constants/operator.js';
import { G8eHttpContext, BoundOperatorContext, UnbindOperatorsRequest } from '../../models/request_models.js';
import {
    BindOperatorsResponse,
    UnbindOperatorsResponse,
} from '../../models/operator_model.js';
import { now } from '../../models/base.js';

export class BindOperatorsService {
    /**
     * @param {Object} options
     * @param {Object} options.operatorService - OperatorService (Domain Layer) instance
     * @param {Object} options.bindingService - BoundSessionsService instance
     * @param {Object} options.operatorSessionService - OperatorSessionService instance
     * @param {Object} options.webSessionService - WebSessionService instance
     */
    constructor({ operatorService, bindingService, operatorSessionService, webSessionService }) {
        if (!operatorService) throw new Error('operatorService is required');
        if (!bindingService) throw new Error('bindingService is required');
        if (!operatorSessionService) throw new Error('operatorSessionService is required');
        if (!webSessionService) throw new Error('webSessionService is required');

        this.operatorService = operatorService;
        this.bindingService = bindingService;
        this.operatorSessionService = operatorSessionService;
        this.webSessionService = webSessionService;
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
            web_session_id: webSessionId.substring(0, 12) + '...',
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

                // 1. Update Operator document (status + web_session_id)
                await this.operatorService.operatorDataService.updateOperator(operatorId, { 
                    status: OperatorStatus.BOUND,
                    web_session_id: webSessionId 
                });

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
                                operator_id:         contextWrapper.operator_id,
                                operator_session_id: contextWrapper.operator_session_id,
                                status:              contextWrapper.status,
                                operator_type:       contextWrapper.operator_type,
                                system_info:         contextWrapper.system_info,
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

        if (bound.length > 0) {
            await this.operatorService.broadcastOperatorListToSession(userId, webSessionId).catch(() => {});
        }

        const success = bound.length > 0 || failed.length === 0;
        const statusCode = failed.length === operatorIds.length ? 400 : (failed.length > 0 ? 207 : 200);

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
            web_session_id: webSessionId.substring(0, 12) + '...',
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

                // 1. Update Operator document (status + web_session_id)
                await this.operatorService.operatorDataService.updateOperator(operatorId, { 
                    status: OperatorStatus.ACTIVE,
                    web_session_id: null 
                });

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
                                operator_id:         contextWrapper.operator_id,
                                operator_session_id: contextWrapper.operator_session_id,
                                status:              contextWrapper.status,
                                operator_type:       contextWrapper.operator_type,
                                system_info:         contextWrapper.system_info,
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

        if (unbound.length > 0) {
            await this.operatorService.broadcastOperatorListToSession(userId, webSessionId).catch(() => {});
        }

        const success = unbound.length > 0 || failed.length === 0;
        const statusCode = failed.length === operatorIds.length ? 400 : (failed.length > 0 ? 207 : 200);

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
