// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { ApprovalRespondRequest, DirectCommandRequest, VSOHttpContext } from '../../models/request_models.js';
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
 * @param {Object} options.bindingService - BoundSessionsService instance
 * @param {Object} options.operatorSessionService - OperatorSessionService instance
 * @param {Object} options.internalHttpClient - InternalHttpClient instance
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createOperatorApprovalRouter({ bindingService, operatorSessionService, internalHttpClient, authMiddleware, rateLimiters }) {
    const { requireAuth } = authMiddleware;
    const { apiRateLimiter } = rateLimiters;
    const relay = new OperatorRelayService({ internalHttpClient });
    const router = express.Router();

    router.post(OperatorApprovalPaths.RESPOND, requireAuth, apiRateLimiter, async (req, res) => {
        try {
            const approvalRequest = ApprovalRespondRequest.parse(req.body);

            const { case_id, investigation_id, task_id } = req.body;
            if (!case_id || !investigation_id || !task_id) {
                return res.status(400).json(new ErrorResponse({
                    error: 'case_id, investigation_id, and task_id are required'
                }).forClient());
            }

            const boundOperators = await bindingService.resolveBoundOperators(req.webSessionId);

            if (boundOperators.length === 0) {
                logger.warn('[OPERATOR-APPROVAL] No Operator session found for user', {
                    approval_id: approvalRequest.approval_id,
                    webSessionId: redactWebSessionId(req.webSessionId)
                });
                return res.status(400).json(new ErrorResponse({
                    error: 'No active Operator session found'
                }).forClient());
            }

            logger.info('[OPERATOR-APPROVAL] Received approval response from user', {
                approval_id: approvalRequest.approval_id,
                approved: approvalRequest.approved,
                case_id,
                investigation_id,
                webSessionId: redactWebSessionId(req.webSessionId),
                operatorCount: boundOperators.length
            });

            const vsoContext = VSOHttpContext.parse({
                web_session_id: req.webSessionId,
                user_id: req.userId,
                organization_id: req.session.organization_id || req.session.user_data?.organization_id,
                case_id,
                investigation_id,
                task_id,
                bound_operators: boundOperators,
                execution_id: `req_approval_${approvalRequest.approval_id}`
            });

            const response = await relay.relayApprovalResponseToVse(approvalRequest.forWire(), vsoContext);

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

    router.post(OperatorApprovalPaths.DIRECT_COMMAND, requireAuth, apiRateLimiter, async (req, res) => {
        try {
            const directCommandRequest = DirectCommandRequest.parse(req.body);

            const operator_session_ids = await bindingService.getBoundOperatorSessionIds(req.webSessionId);

            if (operator_session_ids.length === 0) {
                logger.warn('[OPERATOR-DIRECT] No Operator session found for user', {
                    webSessionId: redactWebSessionId(req.webSessionId)
                });
                return res.status(400).json(new ErrorResponse({
                    error: 'No active Operator session found. Please bind an Operator first.'
                }).forClient());
            }

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

            const vsoContext = VSOHttpContext.parse({
                web_session_id: req.webSessionId,
                user_id: req.userId,
                organization_id: req.session.organization_id || req.session.user_data?.organization_id,
                case_id: req.body.case_id || null,
                investigation_id: req.body.investigation_id || null,
                bound_operators: await bindingService.resolveBoundOperators(req.webSessionId),
                execution_id: `req_direct_${execution_id}`
            });

            const response = await relay.relayDirectCommandToVse(directCommandRequest.forWire(), vsoContext);

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

    return router;
}
