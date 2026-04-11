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

import { ApprovalRespondRequest, DirectCommandRequest } from '../../models/request_models.js';
import express from 'express';
import { now } from '../../models/base.js';
import { ApprovalResponseEvent, DirectCommandResponseEvent } from '../../models/sse_models.js';
import { ErrorResponse } from '../../models/response_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { OperatorApprovalPaths } from '../../constants/api_paths.js';
import { OperatorRelayService } from '../../services/operator/operator_relay_service.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Service objects
 * @param {Object} options.services.bindingService - Binding service object
 * @param {Object} options.services.operatorSessionService - Operator session service object
 * @param {Object} options.services.internalHttpClient - Internal HTTP client object
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createOperatorApprovalRouter({ services, authMiddleware, rateLimiters }) {
    const { bindingService, operatorSessionService, internalHttpClient } = services;
    const { requireAuth, requireOperatorBinding, requireAtLeastOneOperator } = authMiddleware;
    const { apiRateLimiter } = rateLimiters;
    const relay = new OperatorRelayService({ internalHttpClient });
    const router = express.Router();

    router.post(OperatorApprovalPaths.RESPOND, requireAuth, requireOperatorBinding, requireAtLeastOneOperator, apiRateLimiter, async (req, res) => {
        try {
            const approvalRequest = ApprovalRespondRequest.parse(req.body);

            const { case_id, investigation_id, task_id } = req.body;
            if (!case_id || !investigation_id || !task_id) {
                return res.status(400).json(new ErrorResponse({
                    error: 'case_id, investigation_id, and task_id are required'
                }).forClient());
            }

            const boundOperators = req.boundOperators;

            logger.info('[OPERATOR-APPROVAL] Received approval response from user', {
                approval_id: approvalRequest.approval_id,
                approved: approvalRequest.approved,
                case_id,
                investigation_id,
                webSessionId: redactWebSessionId(req.webSessionId),
                operatorCount: boundOperators.length
            });

            const response = await relay.relayApprovalResponseToVse(approvalRequest.forWire(), req.vsoContext);

            logger.info('[OPERATOR-APPROVAL] Sent approval response to VSE via HTTP', {
                approval_id: approvalRequest.approval_id,
                approved: approvalRequest.approved,
                success: response.success,
                webSessionId: redactWebSessionId(req.webSessionId)
            });

            res.json(new ApprovalResponseEvent({
                success: true,
                approval_id: approvalRequest.approval_id,
                approved: approvalRequest.approved,
                timestamp: now()
            }).forClient());

        } catch (error) {
            logger.error('[OPERATOR-APPROVAL] Failed to process approval response', {
                error: error.message,
                stack: error.stack
            });

            res.status(500).json(new ErrorResponse({
                error: 'Failed to process approval response'
            }).forClient());
        }
    });

    router.post(OperatorApprovalPaths.DIRECT_COMMAND, requireAuth, requireOperatorBinding, requireAtLeastOneOperator, apiRateLimiter, async (req, res) => {
        try {
            const directCommandRequest = DirectCommandRequest.parse(req.body);

            const boundOperators = req.boundOperators;
            const operator_session_ids = await bindingService.getBoundOperatorSessionIds(req.webSessionId);

            const operator_session_id = operator_session_ids[0];

            const operatorSession = await operatorSessionService.validateSession(operator_session_id);
            if (!operatorSession) {
                return res.status(400).json(new ErrorResponse({
                    error: 'Operator session expired or invalid'
                }).forClient());
            }

            const execution_id = directCommandRequest.execution_id;

            logger.info('[OPERATOR-DIRECT] Received direct command from terminal', {
                command: directCommandRequest.command.substring(0, 100),
                execution_id,
                webSessionId: redactWebSessionId(req.webSessionId),
                operatorSessionId: redactWebSessionId(operator_session_id),
                operatorId: operatorSession.operator_id
            });

            const response = await relay.relayDirectCommandToVse(directCommandRequest.forWire(), req.vsoContext);

            logger.info('[OPERATOR-DIRECT] Sent direct command to VSE', {
                execution_id,
                success: response.success,
                webSessionId: redactWebSessionId(req.webSessionId),
                operatorId: operatorSession.operator_id
            });

            res.json(new DirectCommandResponseEvent({
                success: true,
                execution_id,
                message: 'Command sent to operator',
                timestamp: now()
            }).forClient());

        } catch (error) {
            logger.error('[OPERATOR-DIRECT] Failed to execute direct command', {
                error: error.message,
                stack: error.stack
            });

            res.status(500).json(new ErrorResponse({
                error: 'Failed to execute command'
            }).forClient());
        }
    });

    router.get(OperatorApprovalPaths.PENDING, requireAuth, requireOperatorBinding, requireAtLeastOneOperator, apiRateLimiter, async (req, res) => {
        try {
            const { case_id, investigation_id } = req.query;

            const boundOperators = req.boundOperators;

            logger.info('[OPERATOR-APPROVAL] Fetching pending approvals for user', {
                case_id,
                investigation_id,
                webSessionId: req.webSessionId,
                operatorCount: boundOperators.length
            });

            const response = await relay.relayPendingApprovalsFromVse(req.vsoContext);

            logger.info('[OPERATOR-APPROVAL] Retrieved pending approvals from VSE', {
                success: response.success,
                webSessionId: redactWebSessionId(req.webSessionId)
            });

            res.json(response);

        } catch (error) {
            logger.error('[OPERATOR-APPROVAL] Failed to fetch pending approvals', {
                error: error.message,
                stack: error.stack
            });

            res.status(500).json(new ErrorResponse({
                error: 'Failed to fetch pending approvals'
            }).forClient());
        }
    });

    return router;
}
